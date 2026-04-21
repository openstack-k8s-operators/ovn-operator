/*

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

package controller

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/openstack-k8s-operators/lib-common/modules/common"
	"github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	"github.com/openstack-k8s-operators/lib-common/modules/common/configmap"
	"github.com/openstack-k8s-operators/lib-common/modules/common/env"
	"github.com/openstack-k8s-operators/lib-common/modules/common/helper"
	common_rbac "github.com/openstack-k8s-operators/lib-common/modules/common/rbac"
	"github.com/openstack-k8s-operators/lib-common/modules/common/rsh"
	"github.com/openstack-k8s-operators/lib-common/modules/common/util"
	ovnv1 "github.com/openstack-k8s-operators/ovn-operator/api/v1beta1"
	"github.com/openstack-k8s-operators/ovn-operator/internal/ovndbbackup"
	"github.com/openstack-k8s-operators/ovn-operator/internal/ovndbcluster"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	k8s_errors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// OVNDBRestoreReconciler reconciles a OVNDBRestore object
type OVNDBRestoreReconciler struct {
	client.Client
	Kclient    kubernetes.Interface
	RestConfig *rest.Config
	Scheme     *runtime.Scheme
}

// GetClient -
func (r *OVNDBRestoreReconciler) GetClient() client.Client {
	return r.Client
}

// GetKClient -
func (r *OVNDBRestoreReconciler) GetKClient() kubernetes.Interface {
	return r.Kclient
}

// GetScheme -
func (r *OVNDBRestoreReconciler) GetScheme() *runtime.Scheme {
	return r.Scheme
}

// GetLogger returns a logger object with a prefix of "controller.name" and additional controller context fields
func (r *OVNDBRestoreReconciler) GetLogger(ctx context.Context) logr.Logger {
	return log.FromContext(ctx).WithName("Controllers").WithName("OVNDBRestore")
}

//+kubebuilder:rbac:groups=ovn.openstack.org,resources=ovndbrestores,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=ovn.openstack.org,resources=ovndbrestores/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=ovn.openstack.org,resources=ovndbrestores/finalizers,verbs=update;patch
//+kubebuilder:rbac:groups=ovn.openstack.org,resources=ovndbbackups,verbs=get;list;watch;update;patch
//+kubebuilder:rbac:groups=ovn.openstack.org,resources=ovndbclusters,verbs=get;list;watch;update;patch
//+kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=get;list;watch
//+kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch;create;delete
//+kubebuilder:rbac:groups=core,resources=persistentvolumeclaims,verbs=get;list;delete

// Reconcile - OVN DB Restore
func (r *OVNDBRestoreReconciler) Reconcile(ctx context.Context, req ctrl.Request) (result ctrl.Result, _err error) {
	Log := r.GetLogger(ctx)

	instance := &ovnv1.OVNDBRestore{}
	err := r.Get(ctx, req.NamespacedName, instance)
	if err != nil {
		if k8s_errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
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

	if instance.Status.Conditions == nil {
		instance.Status.Conditions = condition.Conditions{}
	}

	savedConditions := instance.Status.Conditions.DeepCopy()

	cl := condition.CreateList(
		condition.UnknownCondition(ovnv1.OVNDBBackupReadyCondition, condition.InitReason, ovnv1.OVNDBBackupReadyInitMessage),
		condition.UnknownCondition(ovnv1.OVNDBClusterReadyCondition, condition.InitReason, ovnv1.OVNDBClusterReadyInitMessage),
		condition.UnknownCondition(ovnv1.RestoreJobReadyCondition, condition.InitReason, ovnv1.RestoreJobReadyInitMessage),
		condition.UnknownCondition(condition.ServiceAccountReadyCondition, condition.InitReason, condition.ServiceAccountReadyInitMessage),
		condition.UnknownCondition(condition.RoleReadyCondition, condition.InitReason, condition.RoleReadyInitMessage),
		condition.UnknownCondition(condition.RoleBindingReadyCondition, condition.InitReason, condition.RoleBindingReadyInitMessage),
	)
	instance.Status.Conditions.Init(&cl)
	instance.Status.ObservedGeneration = instance.Generation

	if instance.Status.Hash == nil {
		instance.Status.Hash = map[string]string{}
	}

	defer func() {
		if r := recover(); r != nil {
			Log.Info(fmt.Sprintf("panic during reconcile %v\n", r))
			panic(r)
		}
		if instance.Status.Conditions.AllSubConditionIsTrue() {
			instance.Status.Conditions.MarkTrue(
				condition.ReadyCondition, condition.ReadyMessage)
		} else {
			instance.Status.Conditions.MarkUnknown(
				condition.ReadyCondition, condition.InitReason, condition.ReadyInitMessage)
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

	if instance.DeletionTimestamp.IsZero() && controllerutil.AddFinalizer(instance, helper.GetFinalizer()) {
		return ctrl.Result{}, nil
	}

	if !instance.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, instance, helper)
	}

	return r.reconcileNormal(ctx, instance, helper)
}

func (r *OVNDBRestoreReconciler) reconcileDelete(ctx context.Context, instance *ovnv1.OVNDBRestore, helper *helper.Helper) (ctrl.Result, error) {
	Log := r.GetLogger(ctx)
	Log.Info("Reconciling OVNDBRestore delete")

	// Remove finalizer from backup if we added one
	backup := &ovnv1.OVNDBBackup{}
	err := r.Get(ctx, types.NamespacedName{
		Name:      instance.Spec.BackupSource,
		Namespace: instance.Namespace,
	}, backup)
	if err != nil {
		if !k8s_errors.IsNotFound(err) {
			return ctrl.Result{}, err
		}
	} else {
		finalizerName := "ovn.openstack.org/restore-" + instance.Name
		if controllerutil.RemoveFinalizer(backup, finalizerName) {
			if err := r.Update(ctx, backup); err != nil {
				return ctrl.Result{}, err
			}
		}

		// Remove restore annotation from OVNDBCluster if present
		cluster := &ovnv1.OVNDBCluster{}
		clusterErr := r.Get(ctx, types.NamespacedName{
			Name:      backup.Spec.DatabaseInstance,
			Namespace: instance.Namespace,
		}, cluster)
		if clusterErr == nil {
			if _, ok := cluster.Annotations[ovnv1.RestoreInProgressAnnotation]; ok {
				patch := client.MergeFrom(cluster.DeepCopy())
				delete(cluster.Annotations, ovnv1.RestoreInProgressAnnotation)
				if patchErr := r.Patch(ctx, cluster, patch); patchErr != nil {
					return ctrl.Result{}, patchErr
				}
			}
		} else if !k8s_errors.IsNotFound(clusterErr) {
			return ctrl.Result{}, clusterErr
		}
	}

	controllerutil.RemoveFinalizer(instance, helper.GetFinalizer())
	Log.Info("Reconciled OVNDBRestore delete successfully")

	return ctrl.Result{}, nil
}

func (r *OVNDBRestoreReconciler) reconcileNormal(ctx context.Context, instance *ovnv1.OVNDBRestore, helper *helper.Helper) (ctrl.Result, error) {
	Log := r.GetLogger(ctx)
	Log.Info("Reconciling OVNDBRestore", "phase", instance.Status.Phase)

	// RBAC
	rbacRules := []rbacv1.PolicyRule{
		{
			APIGroups:     []string{"security.openshift.io"},
			ResourceNames: []string{"restricted-v2"},
			Resources:     []string{"securitycontextconstraints"},
			Verbs:         []string{"use"},
		},
	}
	rbacResult, err := common_rbac.ReconcileRbac(ctx, helper, instance, rbacRules)
	if err != nil {
		return rbacResult, err
	} else if (rbacResult != ctrl.Result{}) {
		return rbacResult, nil
	}

	// If completed, nothing to do
	if instance.Status.Phase == ovnv1.OVNDBRestorePhaseCompleted {
		instance.Status.Conditions.MarkTrue(ovnv1.OVNDBBackupReadyCondition, ovnv1.OVNDBBackupReadyMessage)
		instance.Status.Conditions.MarkTrue(ovnv1.OVNDBClusterReadyCondition, ovnv1.OVNDBClusterReadyMessage)
		instance.Status.Conditions.MarkTrue(ovnv1.RestoreJobReadyCondition, ovnv1.RestoreJobReadyMessage)
		return ctrl.Result{}, nil
	}

	// Lookup OVNDBBackup
	backup := &ovnv1.OVNDBBackup{}
	err = r.Get(ctx, types.NamespacedName{
		Name:      instance.Spec.BackupSource,
		Namespace: instance.Namespace,
	}, backup)
	if err != nil {
		if k8s_errors.IsNotFound(err) {
			instance.Status.Conditions.Set(condition.FalseCondition(
				ovnv1.OVNDBBackupReadyCondition,
				condition.RequestedReason,
				condition.SeverityWarning,
				ovnv1.OVNDBBackupReadyErrorMessage,
				fmt.Sprintf("OVNDBBackup %s not found", instance.Spec.BackupSource)))
			return ctrl.Result{RequeueAfter: time.Second * 10}, nil
		}
		return ctrl.Result{}, err
	}

	if !backup.IsReady() {
		instance.Status.Conditions.Set(condition.FalseCondition(
			ovnv1.OVNDBBackupReadyCondition,
			condition.RequestedReason,
			condition.SeverityInfo,
			ovnv1.OVNDBBackupReadyInitMessage))
		return ctrl.Result{RequeueAfter: time.Second * 10}, nil
	}

	// Add finalizer to backup to prevent deletion during restore
	finalizerName := "ovn.openstack.org/restore-" + instance.Name
	if controllerutil.AddFinalizer(backup, finalizerName) {
		if err := r.Update(ctx, backup); err != nil {
			return ctrl.Result{}, err
		}
	}
	instance.Status.Conditions.MarkTrue(ovnv1.OVNDBBackupReadyCondition, ovnv1.OVNDBBackupReadyMessage)

	// Lookup OVNDBCluster
	cluster := &ovnv1.OVNDBCluster{}
	err = r.Get(ctx, types.NamespacedName{
		Name:      backup.Spec.DatabaseInstance,
		Namespace: instance.Namespace,
	}, cluster)
	if err != nil {
		if k8s_errors.IsNotFound(err) {
			instance.Status.Conditions.Set(condition.FalseCondition(
				ovnv1.OVNDBClusterReadyCondition,
				condition.RequestedReason,
				condition.SeverityWarning,
				ovnv1.OVNDBClusterReadyErrorMessage,
				fmt.Sprintf("OVNDBCluster %s not found", backup.Spec.DatabaseInstance)))
			return ctrl.Result{RequeueAfter: time.Second * 10}, nil
		}
		return ctrl.Result{}, err
	}
	instance.Status.Conditions.MarkTrue(ovnv1.OVNDBClusterReadyCondition, ovnv1.OVNDBClusterReadyMessage)

	// Run the phase-based state machine
	switch instance.Status.Phase {
	case "", ovnv1.OVNDBRestorePhaseValidating:
		return r.phaseValidate(ctx, instance, cluster, Log)
	case ovnv1.OVNDBRestorePhaseScalingDown:
		return r.phaseScaleDown(ctx, instance, cluster, Log)
	case ovnv1.OVNDBRestorePhaseRestoring:
		return r.phaseRestore(ctx, instance, backup, cluster, helper, Log)
	case ovnv1.OVNDBRestorePhaseScalingUp:
		return r.phaseScaleUp(ctx, instance, cluster, Log)
	case ovnv1.OVNDBRestorePhaseFailed:
		return ctrl.Result{}, nil
	}

	return ctrl.Result{}, nil
}

func (r *OVNDBRestoreReconciler) phaseValidate(
	_ context.Context,
	instance *ovnv1.OVNDBRestore,
	cluster *ovnv1.OVNDBCluster,
	Log logr.Logger,
) (ctrl.Result, error) {
	Log.Info("Phase: Validating")

	// Save original replica count
	if instance.Status.OriginalReplicas == nil {
		replicas := int32(1)
		if cluster.Spec.Replicas != nil {
			replicas = *cluster.Spec.Replicas
		}
		instance.Status.OriginalReplicas = &replicas
	}

	instance.Status.Phase = ovnv1.OVNDBRestorePhaseScalingDown
	return ctrl.Result{Requeue: true}, nil
}

// setRestoreAnnotation sets the restore annotation on the OVNDBCluster with the
// desired replica count. The OVNDBCluster controller uses this value to override
// the StatefulSet replicas, preventing higher-level operators from interfering.
func (r *OVNDBRestoreReconciler) setRestoreAnnotation(
	ctx context.Context,
	cluster *ovnv1.OVNDBCluster,
	desiredReplicas int32,
) error {
	value := fmt.Sprintf("%d", desiredReplicas)
	if cluster.Annotations != nil && cluster.Annotations[ovnv1.RestoreInProgressAnnotation] == value {
		return nil
	}
	patch := client.MergeFrom(cluster.DeepCopy())
	if cluster.Annotations == nil {
		cluster.Annotations = map[string]string{}
	}
	cluster.Annotations[ovnv1.RestoreInProgressAnnotation] = value
	return r.Patch(ctx, cluster, patch)
}

func (r *OVNDBRestoreReconciler) removeRestoreAnnotation(
	ctx context.Context,
	cluster *ovnv1.OVNDBCluster,
) error {
	if _, ok := cluster.Annotations[ovnv1.RestoreInProgressAnnotation]; !ok {
		return nil
	}
	patch := client.MergeFrom(cluster.DeepCopy())
	delete(cluster.Annotations, ovnv1.RestoreInProgressAnnotation)
	return r.Patch(ctx, cluster, patch)
}

func (r *OVNDBRestoreReconciler) phaseScaleDown(
	ctx context.Context,
	instance *ovnv1.OVNDBRestore,
	cluster *ovnv1.OVNDBCluster,
	Log logr.Logger,
) (ctrl.Result, error) {
	Log.Info("Phase: ScalingDown")

	// Set restore annotation to 0 — this tells the OVNDBCluster controller to
	// force the StatefulSet to 0 replicas regardless of what higher-level
	// operators set in the spec.
	if err := r.setRestoreAnnotation(ctx, cluster, 0); err != nil {
		return ctrl.Result{}, err
	}

	// Force-delete all pods for this StatefulSet. The preStop hooks try to
	// leave the RAFT cluster gracefully, but that hangs when all pods are
	// terminating simultaneously. Since we're restoring the DB from backup,
	// RAFT membership state is irrelevant.
	podList := &corev1.PodList{}
	if err := r.List(ctx, podList,
		client.InNamespace(cluster.Namespace),
		client.MatchingLabels{common.AppSelector: ovndbbackup.StatefulSetName(cluster)},
	); err != nil {
		return ctrl.Result{}, err
	}

	gracePeriod := int64(0)
	for i := range podList.Items {
		Log.Info("Force-deleting pod", "pod", podList.Items[i].Name)
		if err := r.Delete(ctx, &podList.Items[i], &client.DeleteOptions{
			GracePeriodSeconds: &gracePeriod,
		}); err != nil && !k8s_errors.IsNotFound(err) {
			return ctrl.Result{}, err
		}
	}

	// Wait for all pods to be gone
	if len(podList.Items) > 0 {
		Log.Info("Waiting for pods to terminate", "remaining", len(podList.Items))
		return ctrl.Result{RequeueAfter: time.Second * 3}, nil
	}

	// Delete ALL PVCs so pods start fresh without stale RAFT state.
	// Pod-0's PVC is also deleted because:
	// 1. Its data will be overwritten by the backup anyway
	// 2. With local-storage, pod-0's PVC may be bound to a different node
	//    than the backup PVC, causing a volume node affinity conflict when
	//    the restore Job tries to mount both
	// The PVC will be recreated in phaseRestore before the restore Job.
	originalReplicas := int32(1)
	if instance.Status.OriginalReplicas != nil {
		originalReplicas = *instance.Status.OriginalReplicas
	}
	stsName := ovndbbackup.StatefulSetName(cluster)
	for i := int32(0); i < originalReplicas; i++ {
		pvcName := fmt.Sprintf("%s%s-%s-%d",
			cluster.Name, ovndbcluster.PVCSuffixEtcOVN, stsName, i)
		pvc := &corev1.PersistentVolumeClaim{}
		err := r.Get(ctx, types.NamespacedName{
			Name:      pvcName,
			Namespace: cluster.Namespace,
		}, pvc)
		if err != nil {
			if k8s_errors.IsNotFound(err) {
				continue
			}
			return ctrl.Result{}, err
		}
		Log.Info("Deleting PVC", "pvc", pvcName)
		if err := r.Delete(ctx, pvc); err != nil && !k8s_errors.IsNotFound(err) {
			return ctrl.Result{}, err
		}
	}

	Log.Info("All pods terminated and PVCs cleaned up")
	instance.Status.Phase = ovnv1.OVNDBRestorePhaseRestoring
	return ctrl.Result{Requeue: true}, nil
}

func (r *OVNDBRestoreReconciler) phaseRestore(
	ctx context.Context,
	instance *ovnv1.OVNDBRestore,
	backup *ovnv1.OVNDBBackup,
	cluster *ovnv1.OVNDBCluster,
	h *helper.Helper,
	Log logr.Logger,
) (ctrl.Result, error) {
	Log.Info("Phase: Restoring")

	serviceLabels := map[string]string{
		"app":          "ovndbrestore",
		"ovndbrestore": instance.Name,
	}

	// Create restore scripts ConfigMap
	err := r.generateRestoreConfigMaps(ctx, h, instance, cluster, serviceLabels)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to create restore ConfigMap: %w", err)
	}

	// Ensure pod-0's PVC exists. It was deleted in phaseScaleDown to avoid
	// volume node affinity conflicts with local-storage: recreating it here
	// lets WaitForFirstConsumer bind it to a PV on the same node as the
	// backup PVC.
	pod0PVCName := ovndbbackup.ClusterPod0PVCName(cluster)
	pod0PVC := &corev1.PersistentVolumeClaim{}
	err = r.Get(ctx, types.NamespacedName{Name: pod0PVCName, Namespace: cluster.Namespace}, pod0PVC)
	if err == nil {
		if pod0PVC.DeletionTimestamp != nil {
			Log.Info("Waiting for old pod-0 PVC to be fully deleted", "pvc", pod0PVCName)
			return ctrl.Result{RequeueAfter: time.Second * 3}, nil
		}
	} else if k8s_errors.IsNotFound(err) {
		storageRequest, parseErr := resource.ParseQuantity(cluster.Spec.StorageRequest)
		if parseErr != nil {
			return ctrl.Result{}, fmt.Errorf("failed to parse StorageRequest: %w", parseErr)
		}
		pod0PVC = &corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:      pod0PVCName,
				Namespace: cluster.Namespace,
			},
			Spec: corev1.PersistentVolumeClaimSpec{
				AccessModes: []corev1.PersistentVolumeAccessMode{
					corev1.ReadWriteOnce,
				},
				StorageClassName: &cluster.Spec.StorageClass,
				Resources: corev1.VolumeResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceStorage: storageRequest,
					},
				},
			},
		}
		Log.Info("Creating pod-0 PVC for restore", "pvc", pod0PVCName)
		if err = r.Create(ctx, pod0PVC); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to create pod-0 PVC: %w", err)
		}
	} else {
		return ctrl.Result{}, err
	}

	// Create or check restore Job
	restoreJob := ovndbbackup.RestoreJob(instance, backup, cluster, serviceLabels)
	foundJob := &batchv1.Job{}
	err = r.Get(ctx, types.NamespacedName{Name: restoreJob.Name, Namespace: restoreJob.Namespace}, foundJob)
	if err != nil {
		if k8s_errors.IsNotFound(err) {
			if err := controllerutil.SetControllerReference(instance, restoreJob, r.Scheme); err != nil {
				return ctrl.Result{}, err
			}
			Log.Info("Creating restore Job", "Job.Name", restoreJob.Name)
			if err := r.Create(ctx, restoreJob); err != nil {
				instance.Status.Conditions.Set(condition.FalseCondition(
					ovnv1.RestoreJobReadyCondition,
					condition.ErrorReason,
					condition.SeverityWarning,
					ovnv1.RestoreJobReadyErrorMessage,
					err.Error()))
				return ctrl.Result{}, err
			}
			return ctrl.Result{RequeueAfter: time.Second * 5}, nil
		}
		return ctrl.Result{}, err
	}

	// Check Job status
	if foundJob.Status.Succeeded > 0 {
		Log.Info("Restore Job completed successfully")
		instance.Status.Phase = ovnv1.OVNDBRestorePhaseScalingUp
		return ctrl.Result{Requeue: true}, nil
	}

	if foundJob.Status.Failed > 0 {
		Log.Error(nil, "Restore Job failed")
		instance.Status.Phase = ovnv1.OVNDBRestorePhaseFailed
		instance.Status.Conditions.Set(condition.FalseCondition(
			ovnv1.RestoreJobReadyCondition,
			condition.ErrorReason,
			condition.SeverityError,
			ovnv1.RestoreJobReadyErrorMessage,
			"restore Job failed"))
		return ctrl.Result{}, nil
	}

	Log.Info("Waiting for restore Job to complete")
	return ctrl.Result{RequeueAfter: time.Second * 5}, nil
}

func (r *OVNDBRestoreReconciler) phaseScaleUp(
	ctx context.Context,
	instance *ovnv1.OVNDBRestore,
	cluster *ovnv1.OVNDBCluster,
	Log logr.Logger,
) (ctrl.Result, error) {
	Log.Info("Phase: ScalingUp")

	originalReplicas := int32(1)
	if instance.Status.OriginalReplicas != nil {
		originalReplicas = *instance.Status.OriginalReplicas
	}

	_, annotationPresent := cluster.Annotations[ovnv1.RestoreInProgressAnnotation]

	if annotationPresent {
		// Annotation still set — we need pod-0 to bootstrap first.
		if err := r.setRestoreAnnotation(ctx, cluster, 1); err != nil {
			return ctrl.Result{}, err
		}

		sts := &appsv1.StatefulSet{}
		if err := r.Get(ctx, types.NamespacedName{
			Name:      ovndbbackup.StatefulSetName(cluster),
			Namespace: cluster.Namespace,
		}, sts); err != nil {
			return ctrl.Result{}, err
		}

		if sts.Status.ReadyReplicas < 1 {
			Log.Info("Waiting for pod-0 to become ready")
			return ctrl.Result{RequeueAfter: time.Second * 10}, nil
		}

		r.verifyRestoredDB(ctx, instance, cluster, Log)

		if err := r.removeRestoreAnnotation(ctx, cluster); err != nil {
			return ctrl.Result{}, err
		}
		Log.Info("Removed restore annotation, cluster will scale to original replicas")
		return ctrl.Result{RequeueAfter: time.Second * 10}, nil
	}

	// Annotation already removed — wait for all replicas without re-setting it.
	sts := &appsv1.StatefulSet{}
	if err := r.Get(ctx, types.NamespacedName{
		Name:      ovndbbackup.StatefulSetName(cluster),
		Namespace: cluster.Namespace,
	}, sts); err != nil {
		return ctrl.Result{}, err
	}

	if sts.Status.ReadyReplicas < originalReplicas {
		Log.Info("Waiting for all replicas to become ready",
			"ready", sts.Status.ReadyReplicas, "desired", originalReplicas)
		return ctrl.Result{RequeueAfter: time.Second * 10}, nil
	}

	Log.Info("All replicas are ready, restore completed")
	instance.Status.Phase = ovnv1.OVNDBRestorePhaseCompleted
	instance.Status.Conditions.MarkTrue(ovnv1.RestoreJobReadyCondition, ovnv1.RestoreJobReadyMessage)
	return ctrl.Result{}, nil
}

func (r *OVNDBRestoreReconciler) verifyRestoredDB(
	ctx context.Context,
	instance *ovnv1.OVNDBRestore,
	cluster *ovnv1.OVNDBCluster,
	Log logr.Logger,
) {
	if r.RestConfig == nil {
		Log.Info("RestConfig not available, skipping DB verification")
		return
	}

	pod0Name := ovndbbackup.Pod0Name(cluster)
	dbType := strings.ToLower(cluster.Spec.DBType)
	dbName := "OVN_Northbound"
	if cluster.Spec.DBType == ovnv1.SBDBType {
		dbName = "OVN_Southbound"
	}

	containerName := ovndbbackup.ServiceName(cluster)
	err := rsh.ExecInPod(ctx, r.Kclient, r.RestConfig,
		types.NamespacedName{Namespace: instance.Namespace, Name: pod0Name},
		containerName,
		[]string{"ovsdb-client", "get-schema-version",
			fmt.Sprintf("unix:/run/ovn/ovn%s_db.sock", dbType), dbName},
		func(stdout, _ *bytes.Buffer) error {
			Log.Info("Restored DB schema version", "version", strings.TrimSpace(stdout.String()))
			return nil
		},
	)
	if err != nil {
		Log.Info("DB verification failed (non-fatal)", "error", err)
	}
}

func (r *OVNDBRestoreReconciler) generateRestoreConfigMaps(
	ctx context.Context,
	h *helper.Helper,
	instance *ovnv1.OVNDBRestore,
	cluster *ovnv1.OVNDBCluster,
	cmLabels map[string]string,
) error {
	templateParameters := make(map[string]any)
	templateParameters["DB_TYPE"] = strings.ToLower(cluster.Spec.DBType)

	cms := []util.Template{
		{
			Name:          fmt.Sprintf("%s-restore-scripts", instance.Name),
			Namespace:     instance.Namespace,
			Type:          util.TemplateTypeScripts,
			InstanceType:  "ovndbrestore",
			Labels:        cmLabels,
			ConfigOptions: templateParameters,
		},
	}
	return configmap.EnsureConfigMaps(ctx, h, instance, cms, &map[string]env.Setter{})
}

// SetupWithManager sets up the controller with the Manager.
func (r *OVNDBRestoreReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&ovnv1.OVNDBRestore{}).
		Owns(&batchv1.Job{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&corev1.ServiceAccount{}).
		Owns(&rbacv1.Role{}).
		Owns(&rbacv1.RoleBinding{}).
		Complete(r)
}
