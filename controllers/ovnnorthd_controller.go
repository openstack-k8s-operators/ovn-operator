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
	"sort"
	"time"

	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/go-logr/logr"
	topologyv1 "github.com/openstack-k8s-operators/infra-operator/apis/topology/v1beta1"
	"github.com/openstack-k8s-operators/lib-common/modules/common"
	"github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	"github.com/openstack-k8s-operators/lib-common/modules/common/configmap"
	"github.com/openstack-k8s-operators/lib-common/modules/common/deployment"
	"github.com/openstack-k8s-operators/lib-common/modules/common/env"
	"github.com/openstack-k8s-operators/lib-common/modules/common/helper"
	"github.com/openstack-k8s-operators/lib-common/modules/common/labels"
	common_rbac "github.com/openstack-k8s-operators/lib-common/modules/common/rbac"
	"github.com/openstack-k8s-operators/lib-common/modules/common/service"
	"github.com/openstack-k8s-operators/lib-common/modules/common/tls"
	"github.com/openstack-k8s-operators/lib-common/modules/common/util"
	ovnv1 "github.com/openstack-k8s-operators/ovn-operator/api/v1beta1"
	ovn_common "github.com/openstack-k8s-operators/ovn-operator/pkg/common"
	"github.com/openstack-k8s-operators/ovn-operator/pkg/ovnnorthd"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	k8s_errors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// OVNNorthdReconciler reconciles a OVNNorthd object
type OVNNorthdReconciler struct {
	client.Client
	Kclient kubernetes.Interface
	Scheme  *runtime.Scheme
}

// GetClient -
func (r *OVNNorthdReconciler) GetClient() client.Client {
	return r.Client
}

// GetScheme -
func (r *OVNNorthdReconciler) GetScheme() *runtime.Scheme {
	return r.Scheme
}

// getlog returns a logger object with a prefix of "conroller.name" and aditional controller context fields
func (r *OVNNorthdReconciler) GetLogger(ctx context.Context) logr.Logger {
	return log.FromContext(ctx).WithName("Controllers").WithName("OVNNorthd")
}

//+kubebuilder:rbac:groups=ovn.openstack.org,resources=ovnnorthds,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=ovn.openstack.org,resources=ovnnorthds/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=ovn.openstack.org,resources=ovnnorthds/finalizers,verbs=update;patch
//+kubebuilder:rbac:groups=ovn.openstack.org,resources=ovndbclusters,verbs=get;list;watch;
//+kubebuilder:rbac:groups=ovn.openstack.org,resources=ovndbclusters/status,verbs=get;list;watch;
//+kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;create;update;patch;delete;
//+kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch;delete;
//+kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete;
//+kubebuilder:rbac:groups=core,resources=endpoints,verbs=get;list;watch;create;update;patch;delete;
//+kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;patch;update;delete;
//+kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;

// service account, role, rolebinding
// +kubebuilder:rbac:groups="",resources=serviceaccounts,verbs=get;list;watch;create;update;patch
// +kubebuilder:rbac:groups="rbac.authorization.k8s.io",resources=roles,verbs=get;list;watch;create;update;patch
// +kubebuilder:rbac:groups="rbac.authorization.k8s.io",resources=rolebindings,verbs=get;list;watch;create;update;patch
// service account permissions that are needed to grant permission to the above
// +kubebuilder:rbac:groups="security.openshift.io",resourceNames=restricted-v2,resources=securitycontextconstraints,verbs=use
// +kubebuilder:rbac:groups="",resources=pods,verbs=create;delete;get;list;patch;update;watch
// +kubebuilder:rbac:groups=topology.openstack.org,resources=topologies,verbs=get;list;watch;update

// Reconcile - OVN Northd
func (r *OVNNorthdReconciler) Reconcile(ctx context.Context, req ctrl.Request) (result ctrl.Result, _err error) {
	Log := r.GetLogger(ctx)

	// Fetch the OVNNorthd instance
	instance := &ovnv1.OVNNorthd{}
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

	helper, err := helper.NewHelper(
		instance,
		r.Client,
		r.Kclient,
		r.Scheme,
		Log,
	)
	if err != nil {
		return ctrl.Result{}, err
	}

	//
	// initialize status
	//
	if instance.Status.Conditions == nil {
		instance.Status.Conditions = condition.Conditions{}
	}

	// Save a copy of the condtions so that we can restore the LastTransitionTime
	// when a condition's state doesn't change.
	savedConditions := instance.Status.Conditions.DeepCopy()

	// initialize conditions used later as Status=Unknown
	cl := condition.CreateList(
		condition.UnknownCondition(condition.InputReadyCondition, condition.InitReason, condition.InputReadyInitMessage),
		condition.UnknownCondition(condition.DeploymentReadyCondition, condition.InitReason, condition.DeploymentReadyInitMessage),
		condition.UnknownCondition(condition.ServiceAccountReadyCondition, condition.InitReason, condition.ServiceAccountReadyInitMessage),
		condition.UnknownCondition(condition.RoleReadyCondition, condition.InitReason, condition.RoleReadyInitMessage),
		condition.UnknownCondition(condition.RoleBindingReadyCondition, condition.InitReason, condition.RoleBindingReadyInitMessage),
		condition.UnknownCondition(condition.TLSInputReadyCondition, condition.InitReason, condition.InputReadyInitMessage),
	)

	// Init Topology condition if there's a reference
	if instance.Spec.TopologyRef != nil {
		c := condition.UnknownCondition(condition.TopologyReadyCondition, condition.InitReason, condition.TopologyReadyInitMessage)
		cl.Set(c)
	}

	instance.Status.Conditions.Init(&cl)
	instance.Status.ObservedGeneration = instance.Generation

	// Always patch the instance status when exiting this function so we can persist any changes.
	defer func() {
		// Don't update the status, if reconciler Panics
		if r := recover(); r != nil {
			Log.Info(fmt.Sprintf("panic during reconcile %v\n", r))
			panic(r)
		}
		// update the Ready condition based on the sub conditions
		if instance.Status.Conditions.AllSubConditionIsTrue() {
			instance.Status.Conditions.MarkTrue(
				condition.ReadyCondition, condition.ReadyMessage)
		} else {
			// something is not ready so reset the Ready condition
			instance.Status.Conditions.MarkUnknown(
				condition.ReadyCondition, condition.InitReason, condition.ReadyInitMessage)
			// and recalculate it based on the state of the rest of the conditions
			instance.Status.Conditions.Set(
				instance.Status.Conditions.Mirror(condition.ReadyCondition))
		}
		condition.RestoreLastTransitionTimes(&instance.Status.Conditions, savedConditions)
		err := helper.PatchInstance(ctx, instance)
		if err != nil {
			_err = err
			return
		}
	}()

	// If we're not deleting this and the service object doesn't have our finalizer, add it.
	if instance.DeletionTimestamp.IsZero() && controllerutil.AddFinalizer(instance, helper.GetFinalizer()) {
		return ctrl.Result{}, nil
	}

	// Handle service delete
	if !instance.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, instance, helper)
	}

	// Handle non-deleted clusters
	return r.reconcileNormal(ctx, instance, helper)
}

// SetupWithManager sets up the controller with the Manager.
func (r *OVNNorthdReconciler) SetupWithManager(mgr ctrl.Manager) error {
	crs := &ovnv1.OVNNorthdList{}
	// index caBundleSecretNameField
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &ovnv1.OVNNorthd{}, caBundleSecretNameField, func(rawObj client.Object) []string {
		// Extract the secret name from the spec, if one is provided
		cr := rawObj.(*ovnv1.OVNNorthd)
		if cr.Spec.TLS.CaBundleSecretName == "" {
			return nil
		}
		return []string{cr.Spec.TLS.CaBundleSecretName}
	}); err != nil {
		return err
	}

	// index tlsField
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &ovnv1.OVNNorthd{}, tlsField, func(rawObj client.Object) []string {
		// Extract the secret name from the spec, if one is provided
		cr := rawObj.(*ovnv1.OVNNorthd)
		if cr.Spec.TLS.SecretName == nil {
			return nil
		}
		return []string{*cr.Spec.TLS.SecretName}
	}); err != nil {
		return err
	}

	// index topologyField
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &ovnv1.OVNNorthd{}, topologyField, func(rawObj client.Object) []string {
		// Extract the topology name from the spec, if one is provided
		cr := rawObj.(*ovnv1.OVNNorthd)
		if cr.Spec.TopologyRef == nil {
			return nil
		}
		return []string{cr.Spec.TopologyRef.Name}
	}); err != nil {
		return err
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&ovnv1.OVNNorthd{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Owns(&corev1.Endpoints{}).
		Owns(&corev1.ServiceAccount{}).
		Owns(&rbacv1.Role{}).
		Owns(&rbacv1.RoleBinding{}).
		Watches(&ovnv1.OVNDBCluster{}, handler.EnqueueRequestsFromMapFunc(ovnv1.OVNCRNamespaceMapFunc(crs, mgr.GetClient()))).
		Watches(
			&corev1.Secret{},
			handler.EnqueueRequestsFromMapFunc(r.findObjectsForSrc),
			builder.WithPredicates(predicate.ResourceVersionChangedPredicate{}),
		).
		Watches(&topologyv1.Topology{},
			handler.EnqueueRequestsFromMapFunc(r.findObjectsForSrc),
			builder.WithPredicates(predicate.GenerationChangedPredicate{})).
		Complete(r)
}

func (r *OVNNorthdReconciler) findObjectsForSrc(ctx context.Context, src client.Object) []reconcile.Request {
	requests := []reconcile.Request{}

	Log := r.GetLogger(ctx)

	for _, field := range allWatchFields {
		crList := &ovnv1.OVNNorthdList{}
		listOps := &client.ListOptions{
			FieldSelector: fields.OneTermEqualSelector(field, src.GetName()),
			Namespace:     src.GetNamespace(),
		}
		err := r.Client.List(ctx, crList, listOps)
		if err != nil {
			Log.Error(err, fmt.Sprintf("listing %s for field: %s - %s", crList.GroupVersionKind().Kind, field, src.GetNamespace()))
			return requests
		}

		for _, item := range crList.Items {
			Log.Info(fmt.Sprintf("input source %s changed, reconcile: %s - %s", src.GetName(), item.GetName(), item.GetNamespace()))

			requests = append(requests,
				reconcile.Request{
					NamespacedName: types.NamespacedName{
						Name:      item.GetName(),
						Namespace: item.GetNamespace(),
					},
				},
			)
		}
	}

	return requests
}

func (r *OVNNorthdReconciler) reconcileDelete(ctx context.Context, instance *ovnv1.OVNNorthd, helper *helper.Helper) (ctrl.Result, error) {
	Log := r.GetLogger(ctx)

	Log.Info("Reconciling Service delete")
	// Remove finalizer on the Topology CR
	if ctrlResult, err := topologyv1.EnsureDeletedTopologyRef(
		ctx,
		helper,
		instance.Status.LastAppliedTopology,
		instance.Name,
	); err != nil {
		return ctrlResult, err
	}
	// Service is deleted so remove the finalizer.
	controllerutil.RemoveFinalizer(instance, helper.GetFinalizer())
	Log.Info("Reconciled Service delete successfully")
	return ctrl.Result{}, nil
}

func (r *OVNNorthdReconciler) reconcileUpdate(ctx context.Context) (ctrl.Result, error) {
	Log := r.GetLogger(ctx)

	Log.Info("Reconciling Service update")

	Log.Info("Reconciled Service update successfully")
	return ctrl.Result{}, nil
}

func (r *OVNNorthdReconciler) reconcileUpgrade(ctx context.Context) (ctrl.Result, error) {
	Log := r.GetLogger(ctx)

	Log.Info("Reconciling Service upgrade")

	Log.Info("Reconciled Service upgrade successfully")
	return ctrl.Result{}, nil
}

func (r *OVNNorthdReconciler) reconcileNormal(ctx context.Context, instance *ovnv1.OVNNorthd, helper *helper.Helper) (ctrl.Result, error) {
	Log := r.GetLogger(ctx)

	Log.Info("Reconciling Service")

	// Service account, role, binding
	rbacRules := []rbacv1.PolicyRule{
		{
			APIGroups:     []string{"security.openshift.io"},
			ResourceNames: []string{"restricted-v2"},
			Resources:     []string{"securitycontextconstraints"},
			Verbs:         []string{"use"},
		},
		{
			APIGroups: []string{""},
			Resources: []string{"pods"},
			Verbs:     []string{"create", "get", "list", "watch", "update", "patch", "delete"},
		},
	}
	rbacResult, err := common_rbac.ReconcileRbac(ctx, helper, instance, rbacRules)
	if err != nil {
		return rbacResult, err
	} else if (rbacResult != ctrl.Result{}) {
		return rbacResult, nil
	}

	instance.Status.Conditions.MarkTrue(condition.InputReadyCondition, condition.InputReadyMessage)

	//
	// TODO check when/if Init, Update, or Upgrade should/could be skipped
	//

	serviceLabels := map[string]string{
		common.AppSelector: ovnv1.ServiceNameOVNNorthd,
	}

	// Handle service update
	ctrlResult, err := r.reconcileUpdate(ctx)
	if err != nil {
		return ctrlResult, err
	} else if (ctrlResult != ctrl.Result{}) {
		return ctrlResult, nil
	}

	// Handle service upgrade
	ctrlResult, err = r.reconcileUpgrade(ctx)
	if err != nil {
		return ctrlResult, err
	} else if (ctrlResult != ctrl.Result{}) {
		return ctrlResult, nil
	}

	nbEndpoint, err := getInternalEndpoint(ctx, helper, instance, ovnv1.NBDBType)
	if err != nil {
		return ctrlResult, err
	}
	sbEndpoint, err := getInternalEndpoint(ctx, helper, instance, ovnv1.SBDBType)
	if err != nil {
		return ctrlResult, err
	}

	envVars := make(map[string]env.Setter)

	//
	// TLS input validation
	//
	// Validate the CA cert secret if provided
	if instance.Spec.TLS.CaBundleSecretName != "" {
		hash, err := tls.ValidateCACertSecret(
			ctx,
			helper.GetClient(),
			types.NamespacedName{
				Name:      instance.Spec.TLS.CaBundleSecretName,
				Namespace: instance.Namespace,
			},
		)
		if err != nil {
			if k8s_errors.IsNotFound(err) {
				instance.Status.Conditions.Set(condition.FalseCondition(
					condition.TLSInputReadyCondition,
					condition.RequestedReason,
					condition.SeverityInfo,
					fmt.Sprintf(condition.TLSInputReadyWaitingMessage, instance.Spec.TLS.CaBundleSecretName)))
				return ctrl.Result{}, nil
			}
			instance.Status.Conditions.Set(condition.FalseCondition(
				condition.TLSInputReadyCondition,
				condition.ErrorReason,
				condition.SeverityWarning,
				condition.TLSInputErrorMessage,
				err.Error()))
			return ctrl.Result{}, err
		}

		if hash != "" {
			envVars[tls.CABundleKey] = env.SetValue(hash)
		}
	}

	// Validate service cert secret
	if instance.Spec.TLS.Enabled() {
		hash, err := instance.Spec.TLS.ValidateCertSecret(ctx, helper, instance.Namespace)
		if err != nil {
			if k8s_errors.IsNotFound(err) {
				instance.Status.Conditions.Set(condition.FalseCondition(
					condition.TLSInputReadyCondition,
					condition.RequestedReason,
					condition.SeverityInfo,
					fmt.Sprintf(condition.TLSInputReadyWaitingMessage, err.Error())))
				return ctrl.Result{}, nil
			}
			instance.Status.Conditions.Set(condition.FalseCondition(
				condition.TLSInputReadyCondition,
				condition.ErrorReason,
				condition.SeverityWarning,
				condition.TLSInputErrorMessage,
				err.Error()))
			return ctrl.Result{}, err
		}
		envVars[tls.TLSHashName] = env.SetValue(hash)
	}
	// all cert input checks out so report InputReady
	instance.Status.Conditions.MarkTrue(condition.TLSInputReadyCondition, condition.InputReadyMessage)

	//
	// create Configmap required for the Northd service
	// - %-scripts configmap holding scripts required for the Northd service
	//
	err = r.generateServiceConfigMaps(ctx, helper, instance, &envVars, ovnv1.ServiceNameOVNNorthd)
	if err != nil {
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.ServiceConfigReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			condition.ServiceConfigReadyErrorMessage,
			err.Error()))
		return ctrl.Result{}, err
	}
	// Create ConfigMaps - end

	instance.Status.Conditions.MarkTrue(condition.ServiceConfigReadyCondition, condition.ServiceConfigReadyMessage)

	//
	// Handle Topology
	//
	topology, err := ensureTopology(
		ctx,
		helper,
		instance,      // topologyHandler
		instance.Name, // finalizer
		&instance.Status.Conditions,
		labels.GetLabelSelector(serviceLabels),
	)
	if err != nil {
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.TopologyReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			condition.TopologyReadyErrorMessage,
			err.Error()))
		return ctrl.Result{}, fmt.Errorf("waiting for Topology requirements: %w", err)
	}

	// Define a new Deployment object
	depl := deployment.NewDeployment(
		ovnnorthd.Deployment(instance, serviceLabels, nbEndpoint, sbEndpoint, envVars, topology),
		time.Duration(5)*time.Second,
	)

	ctrlResult, err = depl.CreateOrPatch(ctx, helper)
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

	deploy := depl.GetDeployment()
	if deploy.Generation == deploy.Status.ObservedGeneration {
		instance.Status.ReadyCount = deploy.Status.ReadyReplicas
	}

	// Mark the Deployment as Ready only if the number of Replicas is equals
	// to the Deployed instances (ReadyCount), and the the Status.Replicas
	// match Status.ReadyReplicas. If a deployment update is in progress,
	// Replicas > ReadyReplicas.
	// In addition, make sure the controller sees the last Generation
	// by comparing it with the ObservedGeneration.
	if deployment.IsReady(deploy) {
		instance.Status.Conditions.MarkTrue(condition.DeploymentReadyCondition, condition.DeploymentReadyMessage)
	} else if *instance.Spec.Replicas == 0 {
		instance.Status.Conditions.Remove(condition.DeploymentReadyCondition)
	} else {
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.DeploymentReadyCondition,
			condition.RequestedReason,
			condition.SeverityInfo,
			condition.DeploymentReadyRunningMessage))
	}
	// create Deployment - end

	// Create per-pod metrics services if metrics are enabled
	if instance.Spec.MetricsEnabled == nil || *instance.Spec.MetricsEnabled {
		ctrlResult, err = r.reconcileMetricsServices(ctx, helper, instance, serviceLabels)
		if err != nil {
			Log.Error(err, "Failed to reconcile metrics services")
			return ctrlResult, err
		} else if (ctrlResult != ctrl.Result{}) {
			Log.Info("Metrics services reconciliation in progress")
			return ctrlResult, nil
		}
	} else {
		// Metrics are explicitly disabled, clean up any existing metrics services
		r.cleanupExtraStaticServices(ctx, instance, 0, Log)
	}

	Log.Info("Reconciled Service successfully")
	return ctrl.Result{}, nil
}

func getInternalEndpoint(
	ctx context.Context,
	h *helper.Helper,
	instance *ovnv1.OVNNorthd,
	dbType string,
) (string, error) {
	cluster, err := ovnv1.GetDBClusterByType(ctx, h, instance.Namespace, map[string]string{}, dbType)
	if err != nil {
		return "", err
	}
	internalEndpoint, err := cluster.GetInternalEndpoint()
	if err != nil {
		return "", err
	}
	return internalEndpoint, nil
}

func (r *OVNNorthdReconciler) generateServiceConfigMaps(
	ctx context.Context,
	h *helper.Helper,
	instance *ovnv1.OVNNorthd,
	envVars *map[string]env.Setter,
	serviceName string,
) error {

	cmLabels := labels.GetLabels(instance, labels.GetGroupLabel(serviceName), map[string]string{})

	templateParameters := make(map[string]interface{})
	templateParameters["TLS"] = instance.Spec.TLS.Enabled()
	templateParameters["OVN_METRICS_CERT_PATH"] = ovn_common.OVNMetricsCertPath
	templateParameters["OVN_METRICS_KEY_PATH"] = ovn_common.OVNMetricsKeyPath

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
		// ConfigConfigMap for network exporter
		{
			Name:          fmt.Sprintf("%s-config", instance.Name),
			Namespace:     instance.Namespace,
			Type:          util.TemplateTypeConfig,
			InstanceType:  instance.Kind,
			Labels:        cmLabels,
			ConfigOptions: templateParameters,
		},
	}
	return configmap.EnsureConfigMaps(ctx, h, instance, cms, envVars)
}

// reconcileMetricsServices creates static metrics services with consistent pod assignment
func (r *OVNNorthdReconciler) reconcileMetricsServices(
	ctx context.Context,
	helper *helper.Helper,
	instance *ovnv1.OVNNorthd,
	serviceLabels map[string]string,
) (ctrl.Result, error) {
	Log := r.GetLogger(ctx)

	// Get the desired number of replicas from the instance
	desiredReplicas := int32(0)
	if instance.Spec.Replicas != nil {
		desiredReplicas = *instance.Spec.Replicas
	}

	// List running pods for this OVN Northd instance
	podList := &corev1.PodList{}
	listOpts := []client.ListOption{
		client.InNamespace(instance.Namespace),
		client.MatchingLabels(map[string]string{
			common.AppSelector: ovnv1.ServiceNameOVNNorthd,
		}),
	}
	err := r.Client.List(ctx, podList, listOpts...)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to list pods: %w", err)
	}

	// Get running pods (exclude terminating pods and pods without IP)
	runningPods := make([]*corev1.Pod, 0, len(podList.Items))
	for i := range podList.Items {
		pod := &podList.Items[i]
		isTerminating := pod.DeletionTimestamp != nil
		isReady := false
		// Check if pod is ready by examining readiness conditions
		for _, condition := range pod.Status.Conditions {
			if condition.Type == corev1.PodReady && condition.Status == corev1.ConditionTrue {
				isReady = true
				break
			}
		}
		if pod.Status.Phase == corev1.PodRunning && !isTerminating && isReady && pod.Status.PodIP != "" {
			runningPods = append(runningPods, pod)
		}
	}

	// Sort pods by creation timestamp for consistent assignment
	sort.Slice(runningPods, func(i, j int) bool {
		return runningPods[i].CreationTimestamp.Before(&runningPods[j].CreationTimestamp)
	})

	// Create static metrics services based on desired replica count
	for i := int32(0); i < desiredReplicas; i++ {
		serviceName := fmt.Sprintf("ovn-northd-metrics-%d", i)

		// Create service without selector (headless service)
		staticService := r.createStaticMetricsService(instance, serviceLabels, serviceName, i)

		metricsService, err := service.NewService(
			staticService,
			time.Duration(5)*time.Second,
			nil,
		)
		if err != nil {
			Log.Error(err, fmt.Sprintf("Failed to create static metrics service %s", serviceName))
			continue
		}
		ctrlResult, err := metricsService.CreateOrPatch(ctx, helper)
		if err != nil {
			Log.Error(err, fmt.Sprintf("Failed to create or patch static metrics service %s", serviceName))
			// Return error to trigger requeue instead of continuing, allowing rate limiter to back off
			return ctrl.Result{}, err
		} else if (ctrlResult != ctrl.Result{}) {
			Log.Info(fmt.Sprintf("Static metrics service creation in progress: %s", serviceName))
			return ctrlResult, nil
		}

		// Create endpoints for this service pointing to the assigned pod (if exists)
		err = r.createStaticServiceEndpoints(ctx, instance, serviceName, runningPods, int(i), Log)
		if err != nil {
			Log.Error(err, fmt.Sprintf("Failed to create endpoints for static metrics service %s", serviceName))
			// Check if this is a forbidden error due to network policies
			if k8s_errors.IsForbidden(err) {
				Log.Info(fmt.Sprintf("Endpoint creation forbidden for service %s, likely due to network policies - skipping", serviceName))
				continue
			}
			// For other errors, return to trigger requeue with rate limiting
			return ctrl.Result{}, err
		}

		Log.Info(fmt.Sprintf("Successfully reconciled static metrics service: %s", serviceName))
	}

	// Clean up extra services if we scaled down
	r.cleanupExtraStaticServices(ctx, instance, desiredReplicas, Log)

	return ctrl.Result{}, nil
}

// createStaticMetricsService creates a static metrics service with predictable naming
func (r *OVNNorthdReconciler) createStaticMetricsService(
	instance *ovnv1.OVNNorthd,
	serviceLabels map[string]string,
	serviceName string,
	replicaIndex int32,
) *corev1.Service {
	// Add type label for metrics service and replica index
	svcLabels := util.MergeMaps(serviceLabels, map[string]string{
		"type":          "metrics",
		"replica-index": fmt.Sprintf("%d", replicaIndex),
	})

	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceName,
			Namespace: instance.Namespace,
			Labels:    svcLabels,
		},
		Spec: corev1.ServiceSpec{
			// No selector - we'll manually create endpoints for this service
			Ports: []corev1.ServicePort{
				{
					Name:     "metrics",
					Port:     1981,
					Protocol: corev1.ProtocolTCP,
				},
			},
		},
	}
}

// createStaticServiceEndpoints creates endpoints for a static service pointing to assigned pod
func (r *OVNNorthdReconciler) createStaticServiceEndpoints(
	ctx context.Context,
	instance *ovnv1.OVNNorthd,
	serviceName string,
	runningPods []*corev1.Pod,
	replicaIndex int,
	Log logr.Logger,
) error {
	// Create endpoints object pointing to assigned pod (if exists)
	endpoints := &corev1.Endpoints{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceName,
			Namespace: instance.Namespace,
			Labels: map[string]string{
				"type":          "metrics",
				"replica-index": fmt.Sprintf("%d", replicaIndex),
				"service":       "ovn-northd",
			},
		},
		Subsets: []corev1.EndpointSubset{},
	}

	// Assign pod to this static service if available
	if replicaIndex < len(runningPods) {
		pod := runningPods[replicaIndex]
		// Double-check pod readiness and IP availability
		if pod.Status.PodIP != "" && pod.Status.Phase == corev1.PodRunning {
			// Verify pod is still ready
			isReady := false
			for _, condition := range pod.Status.Conditions {
				if condition.Type == corev1.PodReady && condition.Status == corev1.ConditionTrue {
					isReady = true
					break
				}
			}
			if isReady {
				endpoints.Subsets = []corev1.EndpointSubset{
					{
						Addresses: []corev1.EndpointAddress{
							{
								IP: pod.Status.PodIP,
								TargetRef: &corev1.ObjectReference{
									Kind:      "Pod",
									Name:      pod.Name,
									Namespace: pod.Namespace,
									UID:       pod.UID,
								},
							},
						},
						Ports: []corev1.EndpointPort{
							{
								Name:     "metrics",
								Port:     1981,
								Protocol: corev1.ProtocolTCP,
							},
						},
					},
				}
			} else {
				Log.Info(fmt.Sprintf("Pod %s not ready for endpoint assignment to service %s", pod.Name, serviceName))
			}
		} else {
			Log.Info(fmt.Sprintf("Pod %s not running or missing IP for endpoint assignment to service %s", pod.Name, serviceName))
		}
	}

	// Set controller reference
	err := controllerutil.SetControllerReference(instance, endpoints, r.Scheme)
	if err != nil {
		return fmt.Errorf("failed to set controller reference for endpoints %s: %w", serviceName, err)
	}

	// Create or update endpoints
	foundEndpoints := &corev1.Endpoints{}
	err = r.Client.Get(ctx, types.NamespacedName{Name: serviceName, Namespace: instance.Namespace}, foundEndpoints)
	if err != nil && k8s_errors.IsNotFound(err) {
		// Create new endpoints
		err = r.Client.Create(ctx, endpoints)
		if err != nil {
			return fmt.Errorf("failed to create endpoints for service %s: %w", serviceName, err)
		}
		Log.Info(fmt.Sprintf("Created endpoints for static service %s", serviceName))
	} else if err != nil {
		return fmt.Errorf("failed to get endpoints for service %s: %w", serviceName, err)
	} else {
		// Update existing endpoints only if they changed
		if !endpointsEqual(foundEndpoints.Subsets, endpoints.Subsets) {
			foundEndpoints.Subsets = endpoints.Subsets
			err = r.Client.Update(ctx, foundEndpoints)
			if err != nil {
				return fmt.Errorf("failed to update endpoints for service %s: %w", serviceName, err)
			}
			Log.Info(fmt.Sprintf("Updated endpoints for static service %s", serviceName))
		}
	}

	return nil
}

// endpointsEqual compares two endpoint subsets to determine if they are equal
func endpointsEqual(existing, desired []corev1.EndpointSubset) bool {
	if len(existing) != len(desired) {
		return false
	}

	for i, existingSubset := range existing {
		if i >= len(desired) {
			return false
		}
		desiredSubset := desired[i]

		// Compare addresses
		if len(existingSubset.Addresses) != len(desiredSubset.Addresses) {
			return false
		}
		for j, existingAddr := range existingSubset.Addresses {
			if j >= len(desiredSubset.Addresses) {
				return false
			}
			desiredAddr := desiredSubset.Addresses[j]
			if existingAddr.IP != desiredAddr.IP {
				return false
			}
		}

		// Compare ports
		if len(existingSubset.Ports) != len(desiredSubset.Ports) {
			return false
		}
		for j, existingPort := range existingSubset.Ports {
			if j >= len(desiredSubset.Ports) {
				return false
			}
			desiredPort := desiredSubset.Ports[j]
			if existingPort.Port != desiredPort.Port || existingPort.Protocol != desiredPort.Protocol {
				return false
			}
		}
	}

	return true
}

// cleanupExtraStaticServices removes services beyond the desired replica count
func (r *OVNNorthdReconciler) cleanupExtraStaticServices(
	ctx context.Context,
	instance *ovnv1.OVNNorthd,
	desiredReplicas int32,
	Log logr.Logger,
) {
	// Look for services with indices >= desiredReplicas
	for i := desiredReplicas; i < 32; i++ { // Check up to max replicas (32)
		serviceName := fmt.Sprintf("ovn-northd-metrics-%d", i)

		// Try to delete service
		svc := &corev1.Service{}
		err := r.Client.Get(ctx, types.NamespacedName{Name: serviceName, Namespace: instance.Namespace}, svc)
		if err == nil {
			// Service exists, delete it
			Log.Info(fmt.Sprintf("Deleting extra static service %s (scaled down)", serviceName))
			err = r.Client.Delete(ctx, svc)
			if err != nil && !k8s_errors.IsNotFound(err) {
				Log.Error(err, fmt.Sprintf("Failed to delete extra service %s", serviceName))
			}
		}

		// Try to delete endpoints
		ep := &corev1.Endpoints{}
		err = r.Client.Get(ctx, types.NamespacedName{Name: serviceName, Namespace: instance.Namespace}, ep)
		if err == nil {
			// Endpoints exist, delete them
			Log.Info(fmt.Sprintf("Deleting extra static endpoints %s (scaled down)", serviceName))
			err = r.Client.Delete(ctx, ep)
			if err != nil && !k8s_errors.IsNotFound(err) {
				Log.Error(err, fmt.Sprintf("Failed to delete extra endpoints %s", serviceName))
			}
		}
	}
}
