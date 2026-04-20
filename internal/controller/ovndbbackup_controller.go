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
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	"github.com/openstack-k8s-operators/lib-common/modules/common/configmap"
	"github.com/openstack-k8s-operators/lib-common/modules/common/env"
	"github.com/openstack-k8s-operators/lib-common/modules/common/helper"
	common_rbac "github.com/openstack-k8s-operators/lib-common/modules/common/rbac"
	"github.com/openstack-k8s-operators/lib-common/modules/common/util"
	ovnv1 "github.com/openstack-k8s-operators/ovn-operator/api/v1beta1"
	ovn_common "github.com/openstack-k8s-operators/ovn-operator/internal/common"
	"github.com/openstack-k8s-operators/ovn-operator/internal/ovndbbackup"
	"github.com/openstack-k8s-operators/ovn-operator/internal/ovndbcluster"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	k8s_errors "k8s.io/apimachinery/pkg/api/errors"
)

// OVNDBBackupReconciler reconciles a OVNDBBackup object
type OVNDBBackupReconciler struct {
	client.Client
	Kclient kubernetes.Interface
	Scheme  *runtime.Scheme
}

// GetClient -
func (r *OVNDBBackupReconciler) GetClient() client.Client {
	return r.Client
}

// GetKClient -
func (r *OVNDBBackupReconciler) GetKClient() kubernetes.Interface {
	return r.Kclient
}

// GetScheme -
func (r *OVNDBBackupReconciler) GetScheme() *runtime.Scheme {
	return r.Scheme
}

// GetLogger returns a logger object with a prefix of "controller.name" and additional controller context fields
func (r *OVNDBBackupReconciler) GetLogger(ctx context.Context) logr.Logger {
	return log.FromContext(ctx).WithName("Controllers").WithName("OVNDBBackup")
}

//+kubebuilder:rbac:groups=ovn.openstack.org,resources=ovndbbackups,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=ovn.openstack.org,resources=ovndbbackups/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=ovn.openstack.org,resources=ovndbbackups/finalizers,verbs=update;patch
//+kubebuilder:rbac:groups=ovn.openstack.org,resources=ovndbclusters,verbs=get;list;watch
//+kubebuilder:rbac:groups=core,resources=persistentvolumeclaims,verbs=get;list;watch;create;update;patch
//+kubebuilder:rbac:groups=batch,resources=cronjobs,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;create;update;patch;delete;

// Reconcile - OVN DB Backup
func (r *OVNDBBackupReconciler) Reconcile(ctx context.Context, req ctrl.Request) (result ctrl.Result, _err error) {
	Log := r.GetLogger(ctx)

	instance := &ovnv1.OVNDBBackup{}
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
		condition.UnknownCondition(ovnv1.OVNDBClusterReadyCondition, condition.InitReason, ovnv1.OVNDBClusterReadyInitMessage),
		condition.UnknownCondition(condition.ServiceConfigReadyCondition, condition.InitReason, condition.ServiceConfigReadyInitMessage),
		condition.UnknownCondition(ovnv1.PersistentVolumeClaimReadyCondition, condition.InitReason, ovnv1.PersistentVolumeClaimReadyInitMessage),
		condition.UnknownCondition(ovnv1.CronJobReadyCondition, condition.InitReason, ovnv1.CronJobReadyInitMessage),
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

func (r *OVNDBBackupReconciler) reconcileDelete(_ context.Context, instance *ovnv1.OVNDBBackup, helper *helper.Helper) (ctrl.Result, error) {
	Log := r.GetLogger(context.Background())
	Log.Info("Reconciling OVNDBBackup delete")

	controllerutil.RemoveFinalizer(instance, helper.GetFinalizer())
	Log.Info("Reconciled OVNDBBackup delete successfully")

	return ctrl.Result{}, nil
}

func (r *OVNDBBackupReconciler) reconcileNormal(ctx context.Context, instance *ovnv1.OVNDBBackup, helper *helper.Helper) (ctrl.Result, error) {
	Log := r.GetLogger(ctx)
	Log.Info("Reconciling OVNDBBackup")

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

	// Lookup referenced OVNDBCluster
	cluster := &ovnv1.OVNDBCluster{}
	err = r.Get(ctx, types.NamespacedName{
		Name:      instance.Spec.DatabaseInstance,
		Namespace: instance.Namespace,
	}, cluster)
	if err != nil {
		if k8s_errors.IsNotFound(err) {
			instance.Status.Conditions.Set(condition.FalseCondition(
				ovnv1.OVNDBClusterReadyCondition,
				condition.RequestedReason,
				condition.SeverityWarning,
				ovnv1.OVNDBClusterReadyErrorMessage,
				fmt.Sprintf("OVNDBCluster %s not found", instance.Spec.DatabaseInstance)))
			return ctrl.Result{RequeueAfter: time.Second * 10}, nil
		}
		return ctrl.Result{}, err
	}
	instance.Status.Conditions.MarkTrue(ovnv1.OVNDBClusterReadyCondition, ovnv1.OVNDBClusterReadyMessage)

	serviceName := ovnv1.ServiceNameNB
	if cluster.Spec.DBType == ovnv1.SBDBType {
		serviceName = ovnv1.ServiceNameSB
	}
	serviceLabels := map[string]string{
		"app":         "ovndbbackup",
		"ovndbbackup": instance.Name,
		"service":     serviceName,
	}

	// Generate backup scripts ConfigMap
	configMapVars := make(map[string]env.Setter)
	err = r.generateBackupConfigMaps(ctx, helper, instance, cluster, &configMapVars, serviceLabels)
	if err != nil {
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.ServiceConfigReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			condition.ServiceConfigReadyErrorMessage,
			err.Error()))
		return ctrl.Result{}, err
	}

	inputHash, err := r.createHashOfInputHashes(instance, configMapVars)
	if err != nil {
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.ServiceConfigReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			condition.ServiceConfigReadyErrorMessage,
			err.Error()))
		return ctrl.Result{}, err
	}
	instance.Status.Conditions.MarkTrue(condition.ServiceConfigReadyCondition, condition.ServiceConfigReadyMessage)

	// Create backup PVC (NOT owned by the CR)
	backupPVC := ovndbbackup.BackupPVC(instance, cluster)
	foundPVC := &corev1.PersistentVolumeClaim{}
	err = r.Get(ctx, types.NamespacedName{Name: backupPVC.Name, Namespace: backupPVC.Namespace}, foundPVC)
	if err != nil {
		if k8s_errors.IsNotFound(err) {
			Log.Info("Creating backup PVC", "PVC.Name", backupPVC.Name)
			err = r.Create(ctx, backupPVC)
			if err != nil {
				instance.Status.Conditions.Set(condition.FalseCondition(
					ovnv1.PersistentVolumeClaimReadyCondition,
					condition.ErrorReason,
					condition.SeverityWarning,
					ovnv1.PersistentVolumeClaimReadyErrorMessage,
					err.Error()))
				return ctrl.Result{}, err
			}
		} else {
			return ctrl.Result{}, err
		}
	}
	instance.Status.Conditions.MarkTrue(ovnv1.PersistentVolumeClaimReadyCondition, ovnv1.PersistentVolumeClaimReadyMessage)

	// Create or update CronJob (owned by the CR)
	cronJob := ovndbbackup.BackupCronJob(instance, cluster, serviceLabels, inputHash)
	op, err := controllerutil.CreateOrPatch(ctx, r.Client, cronJob, func() error {
		err := controllerutil.SetControllerReference(instance, cronJob, r.Scheme)
		if err != nil {
			return err
		}
		cronJob.Spec.Schedule = instance.Spec.Schedule
		cronJob.Spec.JobTemplate.Spec.Template.Spec.Containers[0].Image = cluster.Spec.ContainerImage
		return nil
	})
	if err != nil {
		instance.Status.Conditions.Set(condition.FalseCondition(
			ovnv1.CronJobReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			ovnv1.CronJobReadyErrorMessage,
			err.Error()))
		return ctrl.Result{}, err
	}
	if op != controllerutil.OperationResultNone {
		Log.Info("CronJob operationResult", "CronJob.Name", cronJob.Name, "result", op)
	}
	instance.Status.Conditions.MarkTrue(ovnv1.CronJobReadyCondition, ovnv1.CronJobReadyMessage)

	Log.Info("Reconciled OVNDBBackup successfully")
	return ctrl.Result{}, nil
}

func (r *OVNDBBackupReconciler) generateBackupConfigMaps(
	ctx context.Context,
	h *helper.Helper,
	instance *ovnv1.OVNDBBackup,
	cluster *ovnv1.OVNDBCluster,
	envVars *map[string]env.Setter,
	cmLabels map[string]string,
) error {
	templateParameters := make(map[string]any)
	templateParameters["TLS"] = cluster.Spec.TLS.Enabled()
	templateParameters["OVNDB_CERT_PATH"] = ovn_common.OVNDbCertPath
	templateParameters["OVNDB_KEY_PATH"] = ovn_common.OVNDbKeyPath
	templateParameters["OVNDB_CACERT_PATH"] = ovn_common.OVNDbCaCertPath
	templateParameters["SERVICE_NAME"] = ovndbbackup.ServiceName(cluster)
	templateParameters["NAMESPACE"] = instance.GetNamespace()
	templateParameters["DB_TYPE"] = strings.ToLower(cluster.Spec.DBType)
	templateParameters["DB_PORT"] = ovndbcluster.DbPortNB
	if cluster.Spec.DBType == ovnv1.SBDBType {
		templateParameters["DB_PORT"] = ovndbcluster.DbPortSB
	}

	cms := []util.Template{
		{
			Name:          ovndbbackup.BackupScriptsConfigMapName(instance),
			Namespace:     instance.Namespace,
			Type:          util.TemplateTypeScripts,
			InstanceType:  "OVNDBBackup",
			Labels:        cmLabels,
			ConfigOptions: templateParameters,
		},
	}
	return configmap.EnsureConfigMaps(ctx, h, instance, cms, envVars)
}

func (r *OVNDBBackupReconciler) createHashOfInputHashes(
	instance *ovnv1.OVNDBBackup,
	envVars map[string]env.Setter,
) (string, error) {
	mergedMapVars := env.MergeEnvs([]corev1.EnvVar{}, envVars)
	hash, err := util.ObjectHash(mergedMapVars)
	if err != nil {
		return hash, err
	}
	instance.Status.Hash[string(condition.InputReadyCondition)] = hash
	return hash, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *OVNDBBackupReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&ovnv1.OVNDBBackup{}).
		Owns(&batchv1.CronJob{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&corev1.ServiceAccount{}).
		Owns(&rbacv1.Role{}).
		Owns(&rbacv1.RoleBinding{}).
		Complete(r)
}
