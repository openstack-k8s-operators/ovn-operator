/*
Copyright 2020 Red Hat

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

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/openstack-k8s-operators/lib-common/modules/common/env"
	ovnv1 "github.com/openstack-k8s-operators/ovn-operator/api/v1beta1"
	util "github.com/openstack-k8s-operators/ovn-operator/pkg/common"
)

const (
	// OVNCentralLabel - ovn central label
	OVNCentralLabel = "ovn-central"
)

// OVNCentralReconciler reconciles a OVNCentral object
type OVNCentralReconciler struct {
	client.Client
	Kclient kubernetes.Interface
	Log     logr.Logger
	Scheme  *runtime.Scheme
}

// GetClient -
func (r *OVNCentralReconciler) GetClient() client.Client {
	return r.Client
}

// GetLogger -
func (r *OVNCentralReconciler) GetLogger() logr.Logger {
	return r.Log
}

// +kubebuilder:rbac:groups=ovn.openstack.org,resources=ovncentrals,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=ovn.openstack.org,resources=ovncentrals/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=ovn.openstack.org,resources=ovncentrals/finalizers,verbs=update
// +kubebuilder:rbac:groups=ovn.openstack.org,resources=ovsdbclusters,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=ovn.openstack.org,resources=ovsdbclusters/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;delete
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch

// Reconcile - reconcile OVN Central API requests
func (r *OVNCentralReconciler) Reconcile(ctx context.Context, req ctrl.Request) (result ctrl.Result, err error) {
	_ = r.Log.WithValues("ovncentral", req.NamespacedName)
	_ = context.Background()

	//
	// Fetch the instance
	//

	central := &ovnv1.OVNCentral{}
	if err := r.Client.Get(ctx, req.NamespacedName, central); err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile
			// request.  Owned objects are automatically garbage collected.  For
			// additional cleanup logic use finalizers. Return and don't requeue.
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		err = WrapErrorForObject("Get central", central, err)
		return ctrl.Result{}, err
	}

	//
	// Snapshot the original status object and ensure we save any changes to it on return
	//

	origStatus := central.Status.DeepCopy()
	statusChanged := func() bool {
		return !equality.Semantic.DeepEqual(&central.Status, origStatus)
	}

	defer func() {
		if statusChanged() {
			if updateErr := r.Client.Status().Update(ctx, central); updateErr != nil {
				if err == nil {
					err = WrapErrorForObject(
						"Update Status", central, updateErr)
				} else {
					LogErrorForObject(r, updateErr, "Update status", central)
				}
			}
		}
	}()

	//
	// Set Default values
	//

	if updated := r.setDefaultValues(ctx, central); updated {
		if err := r.Client.Update(ctx, central); err != nil {
			err = WrapErrorForObject("Update", central, err)
			return ctrl.Result{}, err
		}

		LogForObject(r, "Updated default values", central)
		return ctrl.Result{}, nil
	}

	nbCluster, err := r.clusterApply(ctx, central, ovnv1.DBTypeNB)
	if err != nil {
		return ctrl.Result{}, err
	}

	sbCluster, err := r.clusterApply(ctx, central, ovnv1.DBTypeSB)
	if err != nil {
		return ctrl.Result{}, err
	}

	northd, err := r.northdApply(ctx, central)
	if err != nil {
		return ctrl.Result{}, err
	}

	getAvailability := func() bool {
		if !util.IsAvailable(nbCluster) {
			return false
		}
		if !util.IsAvailable(sbCluster) {
			return false
		}

		if northd == nil {
			return false
		}

		for i := 0; i < len(northd.Status.Conditions); i++ {
			cond := &northd.Status.Conditions[i]
			if cond.Type == appsv1.DeploymentAvailable && cond.Status == corev1.ConditionTrue {
				return true
			}
		}

		return false
	}

	if getAvailability() {
		if !util.IsAvailable(central) {
			LogForObject(r, "OVN Central became available", central)
		}
		util.SetAvailable(central)
	} else {
		if util.IsAvailable(central) {
			LogForObject(r, "OVN Central became unavailable", central)
		}
		util.UnsetAvailable(central)
	}

	return ctrl.Result{}, nil
}

// SetupWithManager -
func (r *OVNCentralReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// Schedule a reconcile on ovncentral for northd if the client configmap owned by either of
	// our 2 clusters is updated
	clusterConfigMapWatcher := handler.EnqueueRequestsFromMapFunc(func(source client.Object) []reconcile.Request {
		labels := source.GetLabels()
		cluster, ok := labels[OVNCentralLabel]
		if !ok {
			return []reconcile.Request{}
		}

		return []reconcile.Request{
			{NamespacedName: types.NamespacedName{
				Name:      cluster,
				Namespace: source.GetNamespace(),
			}},
		}
	})

	return ctrl.NewControllerManagedBy(mgr).
		For(&ovnv1.OVNCentral{}).
		Owns(&ovnv1.OVSDBServer{}).
		Watches(&source.Kind{Type: &corev1.ConfigMap{}}, clusterConfigMapWatcher).
		Owns(&appsv1.Deployment{}).
		Complete(r)
}

func (r *OVNCentralReconciler) setDefaultValues(
	ctx context.Context,
	central *ovnv1.OVNCentral,
) bool {
	logDefault := func(field string, value interface{}) {
		r.Log.Info("Defaulting field", "field", field, "value", value)
	}

	var updated bool
	if central.Spec.NBClientConfig == "" {
		central.Spec.NBClientConfig = fmt.Sprintf("%s-nb", central.Name)
		logDefault("NBClientConfig", central.Spec.NBClientConfig)

		updated = true
	}
	if central.Spec.SBClientConfig == "" {
		central.Spec.SBClientConfig = fmt.Sprintf("%s-sb", central.Name)
		logDefault("SBClientConfig", central.Spec.SBClientConfig)

		updated = true
	}

	return updated
}

func (r *OVNCentralReconciler) clusterApply(
	ctx context.Context,
	central *ovnv1.OVNCentral,
	dbType ovnv1.DBType,
) (*ovnv1.OVSDBCluster, error) {
	cluster := &ovnv1.OVSDBCluster{}
	cluster.Name = fmt.Sprintf("%s-%s", central.Name, strings.ToLower(string(dbType)))
	cluster.Namespace = central.Namespace

	apply := func() error {
		util.InitLabelMap(&cluster.Labels)
		cluster.Labels[OVNCentralLabel] = central.Name

		cluster.Spec.Image = central.Spec.Image
		cluster.Spec.ServerStorageSize = central.Spec.StorageSize
		cluster.Spec.ServerStorageClass = central.Spec.StorageClass

		cluster.Spec.DBType = dbType
		switch dbType {
		case ovnv1.DBTypeNB:
			cluster.Spec.Replicas = central.Spec.NBReplicas
			cluster.Spec.ClientConfig = &central.Spec.NBClientConfig
		case ovnv1.DBTypeSB:
			cluster.Spec.Replicas = central.Spec.SBReplicas
			cluster.Spec.ClientConfig = &central.Spec.SBClientConfig
		}

		return controllerutil.SetControllerReference(central, cluster, r.Scheme)
	}
	op, err := controllerutil.CreateOrUpdate(ctx, r.Client, cluster, apply)
	if err != nil {
		err = WrapErrorForObject(fmt.Sprintf("Updating %s cluster", string(dbType)), cluster, err)
		return nil, err
	}

	if op != controllerutil.OperationResultNone {
		LogForObject(r, string(op), cluster)
	}
	return cluster, nil
}

func (r *OVNCentralReconciler) northdApply(
	ctx context.Context,
	central *ovnv1.OVNCentral,
) (*appsv1.Deployment, error) {
	getClientConfig := func(cmName string) (*string, error) {
		cm := &corev1.ConfigMap{}
		namespaced := types.NamespacedName{Name: cmName, Namespace: central.Namespace}
		if err := r.Client.Get(ctx, namespaced, cm); err != nil {
			if errors.IsNotFound(err) {
				r.Log.Info("Cluster client config not found", "configmap", cmName)
				return nil, nil
			}
			return nil, fmt.Errorf("Get ConfigMap %s", cmName)
		}

		if config, ok := cm.Data["connection"]; ok {
			return &config, nil
		}

		LogForObject(r, "ConfigMap does not contain connection", cm)
		return nil, nil
	}

	nbConfig, err := getClientConfig(central.Spec.NBClientConfig)
	if nbConfig == nil {
		return nil, err
	}
	sbConfig, err := getClientConfig(central.Spec.SBClientConfig)
	if sbConfig == nil {
		return nil, err
	}

	northd := &appsv1.Deployment{}
	northd.Name = fmt.Sprintf("%s-northd", central.Name)
	northd.Namespace = central.Namespace

	apply := func() error {
		util.InitLabelMap(&northd.Labels)
		northd.Labels[OVNCentralLabel] = central.Name

		podLabels := map[string]string{
			"app":           "ovn-northd",
			OVNCentralLabel: central.Name,
		}

		northd.Spec.Replicas = &central.Spec.Replicas
		northd.Spec.Selector = &metav1.LabelSelector{
			MatchLabels: podLabels,
		}
		northd.Spec.Strategy.Type = appsv1.RollingUpdateDeploymentStrategyType
		maxUnavailable := intstr.FromInt(1)
		northd.Spec.Strategy.RollingUpdate = &appsv1.RollingUpdateDeployment{
			MaxUnavailable: &maxUnavailable,
		}

		template := &northd.Spec.Template
		util.InitLabelMap(&template.Labels)
		for k, v := range podLabels {
			template.Labels[k] = v
		}

		if len(template.Spec.Containers) != 1 {
			template.Spec.Containers = make([]corev1.Container, 1)
		}
		container := &template.Spec.Containers[0]
		container.Name = "ovn-northd"
		container.Image = central.Spec.Image
		container.Command = []string{"/northd"}
		container.Resources.Requests = map[corev1.ResourceName]resource.Quantity{
			corev1.ResourceCPU:    *resource.NewScaledQuantity(10, resource.Milli),
			corev1.ResourceMemory: *resource.NewScaledQuantity(300, resource.Mega),
		}
		envVars := map[string]env.Setter{}
		envVars["OVN_LOG_LEVEL"] = env.SetValue("info")
		envVars["OVN_RUNDIR"] = env.SetValue(ovnRunDir)
		envVars["OVN_NB_DB"] = env.SetValue(*nbConfig)
		envVars["OVN_SB_DB"] = env.SetValue(*sbConfig)
		container.Env = env.MergeEnvs(container.Env, envVars)

		// XXX: Dev only
		container.ImagePullPolicy = corev1.PullAlways

		return controllerutil.SetControllerReference(central, northd, r.Scheme)
	}
	op, err := controllerutil.CreateOrUpdate(ctx, r.Client, northd, apply)
	if err != nil {
		return nil, WrapErrorForObject("Updating northd deployment", northd, err)
	}
	if op != controllerutil.OperationResultNone {
		LogForObject(r, string(op), northd)
	}
	return northd, nil
}
