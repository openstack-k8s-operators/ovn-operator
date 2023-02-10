/*
Copyright 2022.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllers

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/openstack-k8s-operators/lib-common/modules/common"
	"github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	"github.com/openstack-k8s-operators/lib-common/modules/common/configmap"
	"github.com/openstack-k8s-operators/lib-common/modules/common/env"
	"github.com/openstack-k8s-operators/lib-common/modules/common/helper"
	"github.com/openstack-k8s-operators/lib-common/modules/common/labels"
	"github.com/openstack-k8s-operators/lib-common/modules/common/service"
	"github.com/openstack-k8s-operators/lib-common/modules/common/statefulset"
	"github.com/openstack-k8s-operators/lib-common/modules/common/util"
	ovnv1 "github.com/openstack-k8s-operators/ovn-operator/api/v1beta1"
	"github.com/openstack-k8s-operators/ovn-operator/pkg/ovndbcluster"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	k8s_errors "k8s.io/apimachinery/pkg/api/errors"
)

// OVNDBClusterReconciler reconciles a OVNDBCluster object
type OVNDBClusterReconciler struct {
	client.Client
	Kclient kubernetes.Interface
	Log     logr.Logger
	Scheme  *runtime.Scheme
}

// GetClient -
func (r *OVNDBClusterReconciler) GetClient() client.Client {
	return r.Client
}

// GetKClient -
func (r *OVNDBClusterReconciler) GetKClient() kubernetes.Interface {
	return r.Kclient
}

// GetLogger -
func (r *OVNDBClusterReconciler) GetLogger() logr.Logger {
	return r.Log
}

// GetScheme -
func (r *OVNDBClusterReconciler) GetScheme() *runtime.Scheme {
	return r.Scheme
}

//+kubebuilder:rbac:groups=ovn.openstack.org,resources=ovndbclusters,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=ovn.openstack.org,resources=ovndbclusters/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=ovn.openstack.org,resources=ovndbclusters/finalizers,verbs=update
//+kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;create;update;patch;delete;
//+kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=get;list;watch;create;patch;update;delete;
//+kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;patch;update;delete;

// Reconcile - OVN DBCluster
func (r *OVNDBClusterReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = context.Background()
	_ = r.Log.WithValues("ovndbcluster", req.NamespacedName)

	// Fetch the OVNDBCluster instance
	instance := &ovnv1.OVNDBCluster{}
	err := r.Client.Get(ctx, req.NamespacedName, instance)
	if err != nil {
		if k8s_errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected.
			// For additional cleanup logic use finalizers. Return and don't requeue.
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return ctrl.Result{}, err
	}
	//
	// initialize status
	//
	if instance.Status.Conditions == nil {
		instance.Status.Conditions = condition.Conditions{}
		// initialize conditions used later as Status=Unknown
		cl := condition.CreateList(
			condition.UnknownCondition(condition.DeploymentReadyCondition, condition.InitReason, condition.DeploymentReadyInitMessage),
		)

		instance.Status.Conditions.Init(&cl)

		// Register overall status immediately to have an early feedback e.g. in the cli
		if err := r.Status().Update(ctx, instance); err != nil {
			return ctrl.Result{}, err
		}
	}
	if instance.Status.Hash == nil {
		instance.Status.Hash = map[string]string{}
	}

	helper, err := helper.NewHelper(
		instance,
		r.Client,
		r.Kclient,
		r.Scheme,
		r.Log,
	)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Always patch the instance status when exiting this function so we can persist any changes.
	defer func() {
		// update the overall status condition if service is ready
		if instance.IsReady() {
			instance.Status.Conditions.MarkTrue(condition.ReadyCondition, condition.ReadyMessage)
		}

		if err := helper.SetAfter(instance); err != nil {
			util.LogErrorForObject(helper, err, "Set after and calc patch/diff", instance)
		}

		if changed := helper.GetChanges()["status"]; changed {
			patch := client.MergeFrom(helper.GetBeforeObject())

			if err := r.Status().Patch(ctx, instance, patch); err != nil && !k8s_errors.IsNotFound(err) {
				util.LogErrorForObject(helper, err, "Update status", instance)
			}
		}
	}()

	// Handle service delete
	if !instance.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, instance, helper)
	}

	// Handle non-deleted clusters
	return r.reconcileNormal(ctx, instance, helper)
}

// SetupWithManager sets up the controller with the Manager.
func (r *OVNDBClusterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&ovnv1.OVNDBCluster{}).
		Owns(&corev1.Service{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&appsv1.StatefulSet{}).
		Complete(r)
}

func (r *OVNDBClusterReconciler) reconcileDelete(ctx context.Context, instance *ovnv1.OVNDBCluster, helper *helper.Helper) (ctrl.Result, error) {
	r.Log.Info("Reconciling Service delete")

	// Service is deleted so remove the finalizer.
	controllerutil.RemoveFinalizer(instance, helper.GetFinalizer())
	r.Log.Info("Reconciled Service delete successfully")
	if err := r.Update(ctx, instance); err != nil && !k8s_errors.IsNotFound(err) {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *OVNDBClusterReconciler) reconcileUpdate(ctx context.Context, instance *ovnv1.OVNDBCluster, helper *helper.Helper) (ctrl.Result, error) {
	r.Log.Info("Reconciling Service update")

	// TODO: should have minor update tasks if required
	// - delete dbsync hash from status to rerun it?

	r.Log.Info("Reconciled Service update successfully")
	return ctrl.Result{}, nil
}

func (r *OVNDBClusterReconciler) reconcileUpgrade(ctx context.Context, instance *ovnv1.OVNDBCluster, helper *helper.Helper) (ctrl.Result, error) {
	r.Log.Info("Reconciling Service upgrade")

	// TODO: should have major version upgrade tasks
	// -delete dbsync hash from status to rerun it?

	r.Log.Info("Reconciled Service upgrade successfully")
	return ctrl.Result{}, nil
}

func (r *OVNDBClusterReconciler) reconcileNormal(ctx context.Context, instance *ovnv1.OVNDBCluster, helper *helper.Helper) (ctrl.Result, error) {
	r.Log.Info("Reconciling Service")

	if !controllerutil.ContainsFinalizer(instance, helper.GetFinalizer()) {
		// If the service object doesn't have our finalizer, add it.
		controllerutil.AddFinalizer(instance, helper.GetFinalizer())
		// Register the finalizer immediately to avoid orphaning resources on delete
		err := r.Update(ctx, instance)

		return ctrl.Result{}, err
	}
	serviceName := ovndbcluster.ServiceNameNB
	if instance.Spec.DBType == "SB" {
		serviceName = ovndbcluster.ServiceNameSB
	}
	serviceLabels := map[string]string{
		common.AppSelector: serviceName,
	}

	// ConfigMap
	configMapVars := make(map[string]env.Setter)

	instance.Status.Conditions.MarkTrue(condition.InputReadyCondition, condition.InputReadyMessage)

	//
	// Create ConfigMaps and Secrets required as input for the Service and calculate an overall hash of hashes
	//

	//
	// create Configmap required for dbcluster input
	// - %-config configmap holding minimal dbcluster config required to get the service up
	//
	err := r.generateServiceConfigMaps(ctx, helper, instance, &configMapVars, serviceName)
	if err != nil {
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.ServiceConfigReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			condition.ServiceConfigReadyErrorMessage,
			err.Error()))
		return ctrl.Result{}, err
	}

	//
	// create hash over all the different input resources to identify if any those changed
	// and a restart/recreate is required.
	//
	inputHash, err := r.createHashOfInputHashes(ctx, instance, configMapVars)
	if err != nil {
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.ServiceConfigReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			condition.ServiceConfigReadyErrorMessage,
			err.Error()))
		return ctrl.Result{}, err
	}
	// Create ConfigMaps and Secrets - end

	instance.Status.Conditions.MarkTrue(condition.ServiceConfigReadyCondition, condition.ServiceConfigReadyMessage)

	//
	// TODO check when/if Init, Update, or Upgrade should/could be skipped
	//

	// Handle service update
	ctrlResult, err := r.reconcileUpdate(ctx, instance, helper)
	if err != nil {
		return ctrlResult, err
	} else if (ctrlResult != ctrl.Result{}) {
		return ctrlResult, nil
	}

	// Handle service upgrade
	ctrlResult, err = r.reconcileUpgrade(ctx, instance, helper)
	if err != nil {
		return ctrlResult, err
	} else if (ctrlResult != ctrl.Result{}) {
		return ctrlResult, nil
	}
	// Define a new Statefulset object
	sfset := statefulset.NewStatefulSet(
		ovndbcluster.StatefulSet(instance, inputHash, serviceLabels),
		time.Duration(5)*time.Second,
	)

	ctrlResult, err = sfset.CreateOrPatch(ctx, helper)
	if err != nil {
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.DeploymentReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			condition.DeploymentReadyErrorMessage,
			err.Error()))
		return ctrlResult, err
	} else if (ctrlResult != ctrl.Result{}) {
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.DeploymentReadyCondition,
			condition.RequestedReason,
			condition.SeverityInfo,
			condition.DeploymentReadyRunningMessage))
		return ctrlResult, nil
	}

	instance.Status.ReadyCount = sfset.GetStatefulSet().Status.ReadyReplicas

	// create Statefulset - end
	// Handle service init
	ctrlResult, err = r.reconcileServices(ctx, instance, helper, serviceLabels, serviceName)
	if err != nil {
		return ctrlResult, err
	} else if (ctrlResult != ctrl.Result{}) {
		return ctrlResult, nil
	}
	svcList, err := service.GetServicesListWithLabel(
		ctx,
		helper,
		helper.GetBeforeObject().GetNamespace(),
		serviceLabels,
	)
	if err == nil && instance.Status.ReadyCount > 0 && len(svcList.Items) > 0 {
		instance.Status.Conditions.MarkTrue(condition.DeploymentReadyCondition, condition.DeploymentReadyMessage)
		dbAddress := []string{}
		raftAddress := []string{}
		for _, svc := range svcList.Items {
			// Filter out headless services
			if svc.Spec.ClusterIP != "None" {
				dbAddress = append(dbAddress, fmt.Sprintf("tcp:%s:%d", svc.Spec.ClusterIP, svc.Spec.Ports[0].Port))
				raftAddress = append(raftAddress, fmt.Sprintf("tcp:%s:%d", svc.Spec.ClusterIP, svc.Spec.Ports[1].Port))
			}
		}
		// Set DBAddress
		instance.Status.DBAddress = strings.Join(dbAddress, ",")
		// Set RaftAddress
		instance.Status.RaftAddress = strings.Join(raftAddress, ",")
	}
	r.Log.Info("Reconciled Service successfully")
	return ctrl.Result{}, nil
}

func (r *OVNDBClusterReconciler) reconcileServices(
	ctx context.Context,
	instance *ovnv1.OVNDBCluster,
	helper *helper.Helper,
	serviceLabels map[string]string,
	serviceName string,
) (ctrl.Result, error) {
	r.Log.Info("Reconciling OVN DB Cluster Service")

	//
	// Ensure the ovndbcluster headless service Exists
	//

	headlesssvc := service.NewService(
		ovndbcluster.HeadlessService(serviceName, instance, serviceLabels),
		serviceLabels,
		time.Duration(5)*time.Second,
	)

	ctrlResult, err := headlesssvc.CreateOrPatch(ctx, helper)
	if err != nil {
		return ctrl.Result{}, err
	} else if (ctrlResult != ctrl.Result{}) {
		return ctrl.Result{}, nil
	}

	podList, err := ovndbcluster.OVNDBPods(ctx, instance, helper, serviceLabels)
	if err != nil {
		return ctrl.Result{}, err
	}

	for _, ovnPod := range podList.Items {
		//
		// Create the ovndbcluster pod service if none exists
		//
		ovndbServiceLabels := map[string]string{
			common.AppSelector:                   serviceName,
			"statefulset.kubernetes.io/pod-name": ovnPod.Name,
		}
		svc := service.NewService(
			ovndbcluster.Service(ovnPod.Name, instance, ovndbServiceLabels),
			ovndbServiceLabels,
			time.Duration(5)*time.Second,
		)
		ctrlResult, err := svc.CreateOrPatch(ctx, helper)
		if err != nil {
			return ctrl.Result{}, err
		} else if (ctrlResult != ctrl.Result{}) {
			return ctrl.Result{}, nil
		}
		// create service - end
	}

	// Delete any extra services left after scale down
	svcList, err := service.GetServicesListWithLabel(
		ctx,
		helper,
		helper.GetBeforeObject().GetNamespace(),
		serviceLabels,
	)
	if err == nil && len(svcList.Items) > int(instance.Spec.Replicas) {
		for i := len(svcList.Items) - 1; i >= int(instance.Spec.Replicas); i-- {
			svcLabels := map[string]string{
				common.AppSelector:                   serviceName,
				"statefulset.kubernetes.io/pod-name": serviceName + fmt.Sprintf("-%d", i),
			}
			err = service.DeleteServicesWithLabel(
				ctx,
				helper,
				instance,
				svcLabels,
			)
			if err != nil {
				return ctrl.Result{}, err
			}
		}
	}
	r.Log.Info("Reconciled OVN DB Cluster Service successfully")
	return ctrl.Result{}, nil
}

// generateServiceConfigMaps - create create configmaps which hold service configuration
func (r *OVNDBClusterReconciler) generateServiceConfigMaps(
	ctx context.Context,
	h *helper.Helper,
	instance *ovnv1.OVNDBCluster,
	envVars *map[string]env.Setter,
	serviceName string,
) error {
	// Create/update configmaps from templates
	cmLabels := labels.GetLabels(instance, labels.GetGroupLabel(serviceName), map[string]string{})

	templateParameters := make(map[string]interface{})

	templateParameters["OVN_LOG_LEVEL"] = instance.Spec.LogLevel
	templateParameters["SERVICE_NAME"] = serviceName
	templateParameters["DB_TYPE"] = strings.ToLower(instance.Spec.DBType)
	templateParameters["DB_PORT"] = 6641
	templateParameters["RAFT_PORT"] = 6643
	if instance.Spec.DBType == "SB" {
		templateParameters["DB_PORT"] = 6642
		templateParameters["RAFT_PORT"] = 6644
	}
	templateParameters["OVN_ELECTION_TIMER"] = instance.Spec.ElectionTimer
	templateParameters["OVN_PROBE_INTERVAL_TO_ACTIVE"] = instance.Spec.ProbeIntervalToActive
	cms := []util.Template{
		// ScriptsConfigMap
		{
			Name:          fmt.Sprintf("%s-scripts", instance.Name),
			Namespace:     instance.Namespace,
			Type:          util.TemplateTypeScripts,
			InstanceType:  instance.Kind,
			Labels:        cmLabels,
			ConfigOptions: templateParameters,
		},
		// ConfigMap
		{
			Name:          fmt.Sprintf("%s-config-data", instance.Name),
			Namespace:     instance.Namespace,
			Type:          util.TemplateTypeConfig,
			InstanceType:  instance.Kind,
			Labels:        cmLabels,
			ConfigOptions: templateParameters,
		},
	}
	return configmap.EnsureConfigMaps(ctx, h, instance, cms, envVars)
}

// createHashOfInputHashes - creates a hash of hashes which gets added to the resources which requires a restart
// if any of the input resources change, like configs, passwords, ...
func (r *OVNDBClusterReconciler) createHashOfInputHashes(
	ctx context.Context,
	instance *ovnv1.OVNDBCluster,
	envVars map[string]env.Setter,
) (string, error) {
	mergedMapVars := env.MergeEnvs([]corev1.EnvVar{}, envVars)
	hash, err := util.ObjectHash(mergedMapVars)
	if err != nil {
		return hash, err
	}
	if hashMap, changed := util.SetHash(instance.Status.Hash, common.InputHashName, hash); changed {
		instance.Status.Hash = hashMap
		if err := r.Client.Status().Update(ctx, instance); err != nil {
			return hash, err
		}
		r.Log.Info(fmt.Sprintf("Input maps hash %s - %s", common.InputHashName, hash))
	}
	return hash, nil
}
