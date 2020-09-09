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

	"github.com/go-logr/logr"
	"github.com/operator-framework/operator-lib/status"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	k8s_errors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	ovncentralv1alpha1 "github.com/openstack-k8s-operators/ovn-central-operator/api/v1alpha1"
	"github.com/openstack-k8s-operators/ovn-central-operator/stubs"
)

// OVNCentralReconciler reconciles a OVNCentral object
type OVNCentralReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=ovn-central.openstack.org,resources=ovncentrals,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=ovn-central.openstack.org,resources=ovncentrals/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=core,resources=persistentvolumeclaims,verbs=get;list;create;update;delete

func (r *OVNCentralReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	_ = r.Log.WithValues("ovncentral", req.NamespacedName)
	ctx := context.Background()

	//
	// Fetch the instance
	//

	instance := &ovncentralv1alpha1.OVNCentral{}
	err := r.Client.Get(ctx, req.NamespacedName, instance)
	if err != nil {
		if k8s_errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile
			// request.  Owned objects are automatically garbage collected.  For
			// additional cleanup logic use finalizers. Return and don't requeue.
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return ctrl.Result{}, fmt.Errorf("Getting %v: %w", req.NamespacedName, err)
	}

	//
	// Set Default values
	//

	updatedInstance := r.setDefaultValues(ctx, instance)
	if updatedInstance {
		err := r.Client.Update(ctx, instance)
		if err != nil {
			err = WrapErrorForObject("Update", instance, err)
			return ctrl.Result{}, err
		}

		return ctrl.Result{}, nil
	}

	//
	// Create a Service for each server
	//

	var services []*corev1.Service
	for i := 0; i < instance.Spec.Replicas; i++ {
		service := stubs.ServerService(instance, r.Scheme, i)
		service, err := r.ApplyService(ctx, service)
		if err != nil {
			return ctrl.Result{}, err
		}
		services = append(services, service)
	}

	//
	// Create a PVC for each server
	//
	// The stateful set would do this for us, but as we need to do it for the bootstrap server,
	// we do it for all servers for consistency.
	//
	// We must ensure we follow the statefulset's naming pattern.
	//

	var pvcs []*corev1.PersistentVolumeClaim
	for i := 0; i < instance.Spec.Replicas; i++ {
		pvc := stubs.PVC(instance, r.Scheme, i)
		pvc, err := r.ApplyPVC(ctx, pvc)
		if err != nil {
			return ctrl.Result{}, err
		}
		pvcs = append(pvcs, pvc)
	}

	//
	// Check we have cluster IDs for the NB and SB databases
	//

	if instance.Status.NBClusterID == nil || instance.Status.SBClusterID == nil {
		// Try to get cluster IDs from servers
		nbClusterID, sbClusterID, err := r.getClusterIDsFromServers(ctx, instance)
		if err != nil {
			return ctrl.Result{}, err
		}

		if nbClusterID != nil || sbClusterID != nil {
			instance.Status.NBClusterID = nbClusterID
			instance.Status.SBClusterID = sbClusterID

			r.LogForObject("Set ClusterIDs", instance,
				"NBClusterID", instance.Status.NBClusterID,
				"SBClusterID", instance.Status.SBClusterID)

			err := r.Client.Status().Update(ctx, instance)
			if err != nil {
				err = WrapErrorForObject("Update status", instance, err)
				return ctrl.Result{}, err
			}
			return ctrl.Result{}, nil
		}
	}

	return ctrl.Result{}, nil
}

func (r *OVNCentralReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&ovncentralv1alpha1.OVNCentral{}).
		Owns(&batchv1.Job{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&corev1.PersistentVolumeClaim{}).
		Owns(&corev1.Secret{}).
		Owns(&corev1.Service{}).
		Complete(r)
}

func (r *OVNCentralReconciler) setDefaultValues(ctx context.Context,
	instance *ovncentralv1alpha1.OVNCentral) bool {

	logDefault := func(field string, value interface{}) {
		r.Log.Info(fmt.Sprintf("Defaulting empty %s", field), "value", value)
	}

	var updatedInstance bool
	if instance.Spec.ConnectionConfig == "" {
		instance.Spec.ConnectionConfig = fmt.Sprintf("%s-connection", instance.Name)
		logDefault("ConnectionConfig", instance.Spec.ConnectionConfig)

		updatedInstance = true
	}
	if instance.Spec.ConnectionCA == "" {
		instance.Spec.ConnectionCA = fmt.Sprintf("%s-ca", instance.Name)
		logDefault("ConnectionCA", instance.Spec.ConnectionCA)
		updatedInstance = true
	}
	if instance.Spec.ConnectionCert == "" {
		instance.Spec.ConnectionCert = fmt.Sprintf("%s-cert", instance.Name)
		logDefault("ConnectionCert", instance.Spec.ConnectionCert)
		updatedInstance = true
	}

	return updatedInstance
}

func getAllClusterIDs(servers []ovncentralv1alpha1.OVNCentralServerStatus) ([]string, []string) {

	reduce := func(servers []ovncentralv1alpha1.OVNCentralServerStatus,
		f func(*ovncentralv1alpha1.OVNCentralServerStatus) string) []string {

		var values []string

		seen := func(value string) bool {
			for _, i := range values {
				if i == value {
					return true
				}
			}
			return false
		}

		for _, i := range servers {
			value := f(&i)
			if !seen(value) {
				values = append(values, value)
			}
		}
		return values
	}

	getNB := func(server *ovncentralv1alpha1.OVNCentralServerStatus) string {
		return server.NB.ClusterID
	}

	getSB := func(server *ovncentralv1alpha1.OVNCentralServerStatus) string {
		return server.SB.ClusterID
	}

	return reduce(servers, getNB), reduce(servers, getSB)
}

func (r *OVNCentralReconciler) getClusterIDsFromServers(ctx context.Context,
	instance *ovncentralv1alpha1.OVNCentral) (nbClusterID, sbClusterID *string, err error) {

	// First look to see if we can extract cluster ID from values reported by
	// running servers. This would be useful recovering lost Status, but
	// would be an unusual situation.

	nbClusterIDs, sbClusterIDs := getAllClusterIDs(instance.Status.Servers)

	// We can't recover if servers are reporting multiple cluster IDs
	if len(nbClusterIDs) > 1 || len(sbClusterIDs) > 1 {
		err = r.setFailed(ctx, instance, ovncentralv1alpha1.OVNCentralInconsistentCluster,
			"Not all cluster members are reporting the same cluster id")
		return nil, nil, err
	}

	if len(nbClusterIDs) == 1 {
		nbClusterID = &nbClusterIDs[0]
	}
	if len(sbClusterIDs) == 1 {
		sbClusterID = &sbClusterIDs[0]
	}

	return nbClusterID, sbClusterID, err
}

func (r *OVNCentralReconciler) createBootstrapper(
	ctx context.Context, instance *ovncentralv1alpha1.OVNCentral,
	bootstrapPVC *corev1.PersistentVolumeClaim) (updated bool, err error) {

	bootstrapPod := stubs.BootstrapPod(instance, r.Scheme, bootstrapPVC)
	fetched := &corev1.Pod{}

	// Check if the pod already exists
	err = r.Client.Get(ctx,
		types.NamespacedName{Name: bootstrapPod.Name, Namespace: bootstrapPod.Namespace},
		fetched)
	if err != nil {
		if k8s_errors.IsNotFound(err) {
			// The job isn't running. Create it.
			err = r.Client.Create(ctx, bootstrapPod)
			if err != nil {
				err = WrapErrorForObject("Create", bootstrapPod, err)
				return false, err
			}
			return true, nil
		}
		err = WrapErrorForObject("Get", bootstrapPod, err)
		return false, err
	}

	switch fetched.Status.Phase {
	case corev1.PodSucceeded:

	case corev1.PodFailed:
	default:
	}

	// Job is still running and hasn't failed, so keep waiting
	return false, nil
}

func (r *OVNCentralReconciler) setFailed(
	ctx context.Context, instance *ovncentralv1alpha1.OVNCentral,
	reason status.ConditionReason, message string) error {

	condition := status.Condition{
		Type:    ovncentralv1alpha1.OVNCentralFailed,
		Status:  corev1.ConditionTrue,
		Reason:  reason,
		Message: message,
	}
	instance.Status.Conditions.SetCondition(condition)
	err := r.Client.Status().Update(ctx, instance)
	if err != nil {
		err = WrapErrorForObject("Update status", instance, err)
		return err
	}

	return fmt.Errorf(message)
}

// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;create;update;delete
