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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

	infranetworkv1 "github.com/openstack-k8s-operators/infra-operator/apis/network/v1beta1"
	"github.com/openstack-k8s-operators/lib-common/modules/common"
	"github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	"github.com/openstack-k8s-operators/lib-common/modules/common/configmap"
	"github.com/openstack-k8s-operators/lib-common/modules/common/env"
	"github.com/openstack-k8s-operators/lib-common/modules/common/helper"
	"github.com/openstack-k8s-operators/lib-common/modules/common/labels"
	nad "github.com/openstack-k8s-operators/lib-common/modules/common/networkattachment"
	common_rbac "github.com/openstack-k8s-operators/lib-common/modules/common/rbac"
	"github.com/openstack-k8s-operators/lib-common/modules/common/service"
	"github.com/openstack-k8s-operators/lib-common/modules/common/statefulset"
	"github.com/openstack-k8s-operators/lib-common/modules/common/tls"
	"github.com/openstack-k8s-operators/lib-common/modules/common/util"
	ovnv1 "github.com/openstack-k8s-operators/ovn-operator/api/v1beta1"
	ovn_common "github.com/openstack-k8s-operators/ovn-operator/pkg/common"
	"github.com/openstack-k8s-operators/ovn-operator/pkg/ovndbcluster"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	k8s_errors "k8s.io/apimachinery/pkg/api/errors"
)

// OVNDBClusterReconciler reconciles a OVNDBCluster object
type OVNDBClusterReconciler struct {
	client.Client
	Kclient kubernetes.Interface
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

// GetScheme -
func (r *OVNDBClusterReconciler) GetScheme() *runtime.Scheme {
	return r.Scheme
}

// getlog returns a logger object with a prefix of "conroller.name" and aditional controller context fields
func (r *OVNDBClusterReconciler) GetLogger(ctx context.Context) logr.Logger {
	return log.FromContext(ctx).WithName("Controllers").WithName("OVNDBCluster")
}

//+kubebuilder:rbac:groups=ovn.openstack.org,resources=ovndbclusters,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=ovn.openstack.org,resources=ovncontroller,verbs=watch;
//+kubebuilder:rbac:groups=ovn.openstack.org,resources=ovndbclusters/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=ovn.openstack.org,resources=ovndbclusters/finalizers,verbs=update;patch
//+kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;create;update;patch;delete;
//+kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch;delete;
//+kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=get;list;watch;create;patch;update;delete;
//+kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;patch;update;delete;
//+kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;
//+kubebuilder:rbac:groups=k8s.cni.cncf.io,resources=network-attachment-definitions,verbs=get;list;watch
//+kubebuilder:rbac:groups=network.openstack.org,resources=dnsdata,verbs=get;list;watch;create;update;patch;delete

// service account, role, rolebinding
// +kubebuilder:rbac:groups="",resources=serviceaccounts,verbs=get;list;watch;create;update;patch
// +kubebuilder:rbac:groups="rbac.authorization.k8s.io",resources=roles,verbs=get;list;watch;create;update;patch
// +kubebuilder:rbac:groups="rbac.authorization.k8s.io",resources=rolebindings,verbs=get;list;watch;create;update;patch
// service account permissions that are needed to grant permission to the above
// +kubebuilder:rbac:groups="security.openshift.io",resourceNames=restricted-v2,resources=securitycontextconstraints,verbs=use
// +kubebuilder:rbac:groups="",resources=pods,verbs=create;delete;get;list;patch;update;watch

// Reconcile - OVN DBCluster
func (r *OVNDBClusterReconciler) Reconcile(ctx context.Context, req ctrl.Request) (result ctrl.Result, _err error) {
	Log := r.GetLogger(ctx)

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
		condition.UnknownCondition(condition.ExposeServiceReadyCondition, condition.InitReason, condition.ExposeServiceReadyInitMessage),
		condition.UnknownCondition(condition.ServiceConfigReadyCondition, condition.InitReason, condition.ServiceConfigReadyInitMessage),
		condition.UnknownCondition(condition.NetworkAttachmentsReadyCondition, condition.InitReason, condition.NetworkAttachmentsReadyInitMessage),
		condition.UnknownCondition(condition.DeploymentReadyCondition, condition.InitReason, condition.DeploymentReadyInitMessage),
		condition.UnknownCondition(condition.ServiceAccountReadyCondition, condition.InitReason, condition.ServiceAccountReadyInitMessage),
		condition.UnknownCondition(condition.RoleReadyCondition, condition.InitReason, condition.RoleReadyInitMessage),
		condition.UnknownCondition(condition.RoleBindingReadyCondition, condition.InitReason, condition.RoleBindingReadyInitMessage),
		condition.UnknownCondition(condition.TLSInputReadyCondition, condition.InitReason, condition.InputReadyInitMessage),
	)

	instance.Status.Conditions.Init(&cl)
	instance.Status.ObservedGeneration = instance.Generation

	if instance.Status.Hash == nil {
		instance.Status.Hash = map[string]string{}
	}
	if instance.Status.NetworkAttachments == nil {
		instance.Status.NetworkAttachments = map[string][]string{}
	}

	// Always patch the instance status when exiting this function so we can persist any changes.
	defer func() {
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
func (r *OVNDBClusterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	crs := &ovnv1.OVNDBClusterList{}
	// index caBundleSecretNameField
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &ovnv1.OVNDBCluster{}, caBundleSecretNameField, func(rawObj client.Object) []string {
		// Extract the secret name from the spec, if one is provided
		cr := rawObj.(*ovnv1.OVNDBCluster)
		if cr.Spec.TLS.CaBundleSecretName == "" {
			return nil
		}
		return []string{cr.Spec.TLS.CaBundleSecretName}
	}); err != nil {
		return err
	}

	// index tlsField
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &ovnv1.OVNDBCluster{}, tlsField, func(rawObj client.Object) []string {
		// Extract the secret name from the spec, if one is provided
		cr := rawObj.(*ovnv1.OVNDBCluster)
		if cr.Spec.TLS.SecretName == nil {
			return nil
		}
		return []string{*cr.Spec.TLS.SecretName}
	}); err != nil {
		return err
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&ovnv1.OVNDBCluster{}).
		Owns(&corev1.Service{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&appsv1.StatefulSet{}).
		Owns(&corev1.ServiceAccount{}).
		Owns(&rbacv1.Role{}).
		Owns(&rbacv1.RoleBinding{}).
		Owns(&infranetworkv1.DNSData{}).
		Watches(&ovnv1.OVNController{}, handler.EnqueueRequestsFromMapFunc(ovnv1.OVNCRNamespaceMapFunc(crs, mgr.GetClient()))).
		Watches(
			&corev1.Secret{},
			handler.EnqueueRequestsFromMapFunc(r.findObjectsForSrc),
			builder.WithPredicates(predicate.ResourceVersionChangedPredicate{}),
		).
		Complete(r)
}

func (r *OVNDBClusterReconciler) findObjectsForSrc(ctx context.Context, src client.Object) []reconcile.Request {
	requests := []reconcile.Request{}

	Log := r.GetLogger(ctx)

	for _, field := range allWatchFields {
		crList := &ovnv1.OVNDBClusterList{}
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

func (r *OVNDBClusterReconciler) reconcileDelete(ctx context.Context, instance *ovnv1.OVNDBCluster, helper *helper.Helper) (ctrl.Result, error) {
	Log := r.GetLogger(ctx)

	Log.Info("Reconciling Service delete")

	// Service is deleted so remove the finalizer.
	controllerutil.RemoveFinalizer(instance, helper.GetFinalizer())
	Log.Info("Reconciled Service delete successfully")

	return ctrl.Result{}, nil
}

func (r *OVNDBClusterReconciler) reconcileUpdate(ctx context.Context) (ctrl.Result, error) {
	Log := r.GetLogger(ctx)

	Log.Info("Reconciling Service update")

	// TODO: should have minor update tasks if required
	// - delete dbsync hash from status to rerun it?

	Log.Info("Reconciled Service update successfully")
	return ctrl.Result{}, nil
}

func (r *OVNDBClusterReconciler) reconcileUpgrade(ctx context.Context) (ctrl.Result, error) {
	Log := r.GetLogger(ctx)

	Log.Info("Reconciling Service upgrade")

	// TODO: should have major version upgrade tasks
	// -delete dbsync hash from status to rerun it?

	Log.Info("Reconciled Service upgrade successfully")
	return ctrl.Result{}, nil
}

func (r *OVNDBClusterReconciler) reconcileNormal(ctx context.Context, instance *ovnv1.OVNDBCluster, helper *helper.Helper) (ctrl.Result, error) {
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

	serviceName := ovnv1.ServiceNameNB
	if instance.Spec.DBType == ovnv1.SBDBType {
		serviceName = ovnv1.ServiceNameSB
	}
	serviceLabels := map[string]string{
		common.AppSelector: serviceName,
	}

	// network to attach to
	networkAttachments := []string{}
	if instance.Spec.NetworkAttachment != "" {
		networkAttachments = append(networkAttachments, instance.Spec.NetworkAttachment)

		_, err := nad.GetNADWithName(ctx, helper, instance.Spec.NetworkAttachment, instance.Namespace)
		if err != nil {
			if k8s_errors.IsNotFound(err) {
				Log.Info(fmt.Sprintf("network-attachment-definition %s not found", instance.Spec.NetworkAttachment))
				instance.Status.Conditions.Set(condition.FalseCondition(
					condition.NetworkAttachmentsReadyCondition,
					condition.RequestedReason,
					condition.SeverityInfo,
					condition.NetworkAttachmentsReadyWaitingMessage,
					instance.Spec.NetworkAttachment))
				return ctrl.Result{RequeueAfter: time.Second * 10}, nil
			}
			instance.Status.Conditions.Set(condition.FalseCondition(
				condition.NetworkAttachmentsReadyCondition,
				condition.ErrorReason,
				condition.SeverityWarning,
				condition.NetworkAttachmentsReadyErrorMessage,
				err.Error()))
			return ctrl.Result{}, err
		}
	} else if instance.Spec.DBType == ovnv1.SBDBType {
		// This config map was created by the SB and it only needs to be deleted once
		// since this reconcile loop can be done by the SB and the NB, filtering so only
		// one deletes it.
		Log.Info("NetworkAttachment is empty, deleting external config map")
		err = r.deleteExternalConfigMaps(ctx, helper, instance.Namespace)
		if err != nil {
			return ctrl.Result{}, err
		}
	}

	serviceAnnotations, err := nad.CreateNetworksAnnotation(instance.Namespace, networkAttachments)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed create network annotation from %s: %w",
			instance.Spec.NetworkAttachment, err)
	}

	// ConfigMap
	configMapVars := make(map[string]env.Setter)

	instance.Status.Conditions.MarkTrue(condition.InputReadyCondition, condition.InputReadyMessage)

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
			configMapVars[tls.CABundleKey] = env.SetValue(hash)
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
		configMapVars[tls.TLSHashName] = env.SetValue(hash)
	}
	// all cert input checks out so report InputReady
	instance.Status.Conditions.MarkTrue(condition.TLSInputReadyCondition, condition.InputReadyMessage)

	//
	// Create ConfigMaps and Secrets required as input for the Service and calculate an overall hash of hashes
	//

	//
	// create Configmap required for dbcluster input
	// - %-config configmap holding minimal dbcluster config required to get the service up
	//
	err = r.generateServiceConfigMaps(ctx, helper, instance, &configMapVars, serviceName)
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
	// Define a new Statefulset object
	sfset := statefulset.NewStatefulSet(
		ovndbcluster.StatefulSet(instance, inputHash, serviceLabels, serviceAnnotations),
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

	// verify if network attachment matches expectations
	networkReady, networkAttachmentStatus, err := nad.VerifyNetworkStatusFromAnnotation(ctx, helper, networkAttachments, serviceLabels, instance.Status.ReadyCount)
	if err != nil {
		return ctrl.Result{}, err
	}

	instance.Status.NetworkAttachments = networkAttachmentStatus
	if networkReady {
		instance.Status.Conditions.MarkTrue(condition.NetworkAttachmentsReadyCondition, condition.NetworkAttachmentsReadyMessage)
	} else {
		err := fmt.Errorf("not all pods have interfaces with ips as configured in NetworkAttachments: %s", instance.Spec.NetworkAttachment)
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.NetworkAttachmentsReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			condition.NetworkAttachmentsReadyErrorMessage,
			err.Error()))

		return ctrlResult, nil
	}

	// create Statefulset - end
	// Handle service init
	ctrlResult, err = r.reconcileServices(ctx, instance, helper, serviceLabels, serviceName)
	if err != nil {
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.ExposeServiceReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			condition.ExposeServiceReadyErrorMessage,
			err.Error()))
		return ctrlResult, err
	} else if (ctrlResult != ctrl.Result{}) {
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.ExposeServiceReadyCondition,
			condition.RequestedReason,
			condition.SeverityInfo,
			condition.ExposeServiceReadyRunningMessage))
		return ctrlResult, err
	}

	svcList, err := service.GetServicesListWithLabel(
		ctx,
		helper,
		helper.GetBeforeObject().GetNamespace(),
		serviceLabels,
	)
	if err != nil {
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.ExposeServiceReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			condition.ExposeServiceReadyErrorMessage,
			err.Error()))
		return ctrl.Result{}, err
	}

	if instance.Status.ReadyCount > 0 && len(svcList.Items) > 0 {
		instance.Status.Conditions.MarkTrue(condition.DeploymentReadyCondition, condition.DeploymentReadyMessage)
		instance.Status.Conditions.MarkTrue(condition.ExposeServiceReadyCondition, condition.ExposeServiceReadyMessage)
		internalDbAddress := []string{}
		var svcPort int32
		scheme := "tcp"
		if instance.Spec.TLS.Enabled() {
			scheme = "ssl"
		}
		for _, svc := range svcList.Items {
			svcPort = svc.Spec.Ports[0].Port

			// Filter out headless services
			if svc.Spec.ClusterIP != "None" {
				internalDbAddress = append(internalDbAddress, fmt.Sprintf("%s:%s.%s.svc.%s:%d", scheme, svc.Name, svc.Namespace, ovnv1.DNSSuffix, svcPort))
			}
		}

		// Note setting this to the singular headless service address (e.g ssl:ovsdbserver-sb...) "works" but will not
		// load-balance on OCP (https://issues.redhat.com/browse/RFE-2838)
		// Since the clients are connecting to the pod dns names directly the TLS certs will need to include
		// all (potential) pod names or use a wildcard

		// Set DB Address
		instance.Status.InternalDBAddress = strings.Join(internalDbAddress, ",")
		if instance.Spec.DBType == ovnv1.SBDBType && instance.Spec.NetworkAttachment != "" {
			// This config map will populate the sb db address to edpm, can't use the nb
			// If there's no networkAttachments the configMap is not needed
			configMapVars := make(map[string]env.Setter)
			err = r.generateExternalConfigMaps(ctx, helper, instance, serviceName, &configMapVars)
			if err != nil {
				Log.Info(fmt.Sprintf("Error while generating external config map: %v", err))
			}
		}

	}
	Log.Info("Reconciled Service successfully")
	return ctrl.Result{}, nil
}

func getPodIPInNetwork(ovnPod corev1.Pod, namespace string, networkAttachment string) (string, error) {
	netStat, err := nad.GetNetworkStatusFromAnnotation(ovnPod.Annotations)
	if err != nil {
		err = fmt.Errorf("error while getting the Network Status for pod %s: %w", ovnPod.Name, err)
		return "", err
	}
	for _, v := range netStat {
		if v.Name == namespace+"/"+networkAttachment {
			for _, ip := range v.IPs {
				return ip, nil
			}
		}
	}
	// If this is reached it means that no IP was found, construct error and return
	err = fmt.Errorf("error while getting IP address from pod %s in network %s, IP is empty", ovnPod.Name, networkAttachment)
	return "", err
}

func (r *OVNDBClusterReconciler) reconcileServices(
	ctx context.Context,
	instance *ovnv1.OVNDBCluster,
	helper *helper.Helper,
	serviceLabels map[string]string,
	serviceName string,
) (ctrl.Result, error) {
	Log := r.GetLogger(ctx)

	Log.Info("Reconciling OVN DB Cluster Service")

	//
	// Ensure the ovndbcluster headless service Exists
	//
	headlessServiceLabels := util.MergeMaps(serviceLabels, map[string]string{"type": ovnv1.ServiceHeadlessType})

	headlesssvc, err := service.NewService(
		ovndbcluster.HeadlessService(serviceName, instance, headlessServiceLabels, serviceLabels),
		time.Duration(5)*time.Second,
		nil,
	)
	if err != nil {
		return ctrl.Result{}, err
	}

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
		ovndbSelectorLabels := map[string]string{
			common.AppSelector:                   serviceName,
			"statefulset.kubernetes.io/pod-name": ovnPod.Name,
		}
		ovndbServiceLabels := util.MergeMaps(ovndbSelectorLabels, map[string]string{"type": ovnv1.ServiceClusterType})
		svc, err := service.NewService(
			ovndbcluster.Service(ovnPod.Name, instance, ovndbServiceLabels, ovndbSelectorLabels),
			time.Duration(5)*time.Second,
			nil,
		)
		if err != nil {
			return ctrl.Result{}, err
		}
		ctrlResult, err := svc.CreateOrPatch(ctx, helper)
		if err != nil {
			return ctrl.Result{}, err
		} else if (ctrlResult != ctrl.Result{}) {
			return ctrl.Result{}, nil
		}
		// create service - end
	}

	// Delete any extra services left after scale down
	clusterServiceLabels := util.MergeMaps(serviceLabels, map[string]string{"type": ovnv1.ServiceClusterType})
	svcList, err := service.GetServicesListWithLabel(
		ctx,
		helper,
		helper.GetBeforeObject().GetNamespace(),
		clusterServiceLabels,
	)
	if err == nil && len(svcList.Items) > int(*(instance.Spec.Replicas)) {
		for i := len(svcList.Items) - 1; i >= int(*(instance.Spec.Replicas)); i-- {
			fullServiceName := fmt.Sprintf("%s-%d", serviceName, i)
			svcLabels := map[string]string{
				common.AppSelector:                   serviceName,
				"statefulset.kubernetes.io/pod-name": fullServiceName,
			}
			err = service.DeleteServicesWithLabel(
				ctx,
				helper,
				instance,
				svcLabels,
			)
			if err != nil {
				err = fmt.Errorf("error while deleting service with name %s: %w", fullServiceName, err)
				return ctrl.Result{}, err
			}
		}
	}

	var svc *corev1.Service

	// When the cluster is attached to an external network, create DNS record for every
	// cluster member so it can be resolved from outside cluster (edpm nodes)
	if instance.Spec.NetworkAttachment != "" {
		var dnsIPsList []string
		// TODO(averdagu): use built in Min once go1.21 is used
		minLen := ovn_common.Min(len(podList.Items), int(*(instance.Spec.Replicas)))
		for _, ovnPod := range podList.Items[:minLen] {
			svc, err = service.GetServiceWithName(
				ctx,
				helper,
				ovnPod.Name,
				ovnPod.Namespace,
			)
			if err != nil {
				return ctrl.Result{}, err
			}

			dnsIP, err := getPodIPInNetwork(ovnPod, instance.Namespace, instance.Spec.NetworkAttachment)
			dnsIPsList = append(dnsIPsList, dnsIP)
			if err != nil {
				return ctrl.Result{}, err
			}

		}
		// DNSData info is called every reconcile loop to ensure that even if a pod gets
		// restarted and it's IP has changed, the DNSData CR will have the correct info.
		// If nothing changed this won't modify the current dnsmasq pod.
		err = ovndbcluster.DNSData(
			ctx,
			helper,
			serviceName,
			dnsIPsList,
			instance,
			serviceLabels,
		)
		if err != nil {
			return ctrl.Result{}, err
		}
		// It can be possible that not all pods are ready, so DNSData won't
		// have complete information, return error to retrigger reconcile loop
		// Returning here instead of at the beggining of the for is done to
		// expose the already created pods to other services/dataplane nodes
		if len(podList.Items) < int(*(instance.Spec.Replicas)) {
			Log.Info(fmt.Sprintf("not all pods are yet created, number of expected pods: %v, current pods: %v", *(instance.Spec.Replicas), len(podList.Items)))
			return ctrl.Result{RequeueAfter: 1 * time.Second}, nil
		}
	}
	// dbAddress will contain ovsdbserver-(nb|sb).openstack.svc or empty
	scheme := "tcp"
	if instance.Spec.TLS.Enabled() {
		scheme = "ssl"
	}
	instance.Status.DBAddress = ovndbcluster.GetDBAddress(svc, serviceName, instance.Namespace, scheme)

	Log.Info("Reconciled OVN DB Cluster Service successfully")
	return ctrl.Result{}, nil
}

// generateServiceConfigMaps - create create configmaps which hold service configuration
func (r *OVNDBClusterReconciler) generateExternalConfigMaps(
	ctx context.Context,
	h *helper.Helper,
	instance *ovnv1.OVNDBCluster,
	serviceName string,
	envVars *map[string]env.Setter,
) error {
	// Create/update configmaps from templates
	cmLabels := labels.GetLabels(instance, labels.GetGroupLabel(serviceName), map[string]string{})
	log := r.GetLogger(ctx)

	externalEndpoint, err := instance.GetExternalEndpoint()
	if err != nil {
		return err
	}

	externalTemplateParameters := make(map[string]interface{})
	externalTemplateParameters["OVNRemote"] = externalEndpoint

	ovnController, err := ovnv1.GetOVNController(ctx, h, instance.Namespace)
	if err != nil {
		log.Info(fmt.Sprintf("Error on getting OVNController: %v", err))
		return err
	}
	if ovnController != nil {
		externalTemplateParameters["OVNEncapType"] = ovnController.Spec.ExternalIDS.OvnEncapType
	}

	cms := []util.Template{
		// EDP ConfigMap
		{
			Name:          "ovncontroller-config",
			Namespace:     instance.Namespace,
			Type:          util.TemplateTypeConfig,
			InstanceType:  instance.Kind,
			Labels:        cmLabels,
			ConfigOptions: externalTemplateParameters,
		},
	}
	return configmap.EnsureConfigMaps(ctx, h, instance, cms, envVars)
}

func (r *OVNDBClusterReconciler) deleteExternalConfigMaps(
	ctx context.Context,
	h *helper.Helper,
	namespace string,
) error {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ovncontroller-config",
			Namespace: namespace,
		},
	}

	err := h.GetClient().Delete(ctx, cm)
	if err != nil && !k8s_errors.IsNotFound(err) {
		return fmt.Errorf("error deleting external config map %s: %w", cm.Name, err)
	}
	return nil
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
	templateParameters["NAMESPACE"] = instance.GetNamespace()
	templateParameters["DB_TYPE"] = strings.ToLower(instance.Spec.DBType)
	templateParameters["DB_PORT"] = ovndbcluster.DbPortNB
	templateParameters["RAFT_PORT"] = ovndbcluster.RaftPortNB
	if instance.Spec.DBType == ovnv1.SBDBType {
		templateParameters["DB_PORT"] = ovndbcluster.DbPortSB
		templateParameters["RAFT_PORT"] = ovndbcluster.RaftPortSB
	}
	templateParameters["OVN_ELECTION_TIMER"] = instance.Spec.ElectionTimer
	templateParameters["OVN_INACTIVITY_PROBE"] = instance.Spec.InactivityProbe
	templateParameters["OVN_PROBE_INTERVAL_TO_ACTIVE"] = instance.Spec.ProbeIntervalToActive
	templateParameters["TLS"] = instance.Spec.TLS.Enabled()
	templateParameters["OVNDB_CERT_PATH"] = ovn_common.OVNDbCertPath
	templateParameters["OVNDB_KEY_PATH"] = ovn_common.OVNDbKeyPath
	templateParameters["OVNDB_CACERT_PATH"] = ovn_common.OVNDbCaCertPath

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
	Log := r.GetLogger(ctx)

	mergedMapVars := env.MergeEnvs([]corev1.EnvVar{}, envVars)
	hash, err := util.ObjectHash(mergedMapVars)
	if err != nil {
		return hash, err
	}
	if hashMap, changed := util.SetHash(instance.Status.Hash, common.InputHashName, hash); changed {
		instance.Status.Hash = hashMap
		Log.Info(fmt.Sprintf("Input maps hash %s - %s", common.InputHashName, hash))
	}
	return hash, nil
}
