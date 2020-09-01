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

type reconcilerReturn struct {
	result ctrl.Result
	err    error
}

const dataVolumeName = "data"

func getBootstrapPVCName(instance *ovncentralv1alpha1.OVNCentral) string {
	return fmt.Sprintf("%s-%s-0", dataVolumeName, instance.Name)
}

// +kubebuilder:rbac:groups=ovn-central.openstack.org,resources=ovncentrals,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=ovn-central.openstack.org,resources=ovncentrals/status,verbs=get;update;patch

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
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected.
			// For additional cleanup logic use finalizers. Return and don't requeue.
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return ctrl.Result{}, err
	}

	//
	// Set Default values
	//

	instanceUpdated := r.setDefaultValues(ctx, instance)
	if instanceUpdated {
		err := r.Client.Update(ctx, instance)
		if err != nil {
			return ctrl.Result{}, err
		}

		return ctrl.Result{Requeue: true}, nil
	}

	//
	// Check the bootstrap PVC exists
	//
	// The bootstrap PVC is the PVC of the first pod in the statefulset. We
	// manually initialise it with a database before starting the
	// statefulset.
	//

	bootstrapPVCName := getBootstrapPVCName(instance)
	bootstrapPVC := &corev1.PersistentVolumeClaim{}
	err = r.Client.Get(ctx, types.NamespacedName{Name: bootstrapPVCName, Namespace: instance.Namespace}, bootstrapPVC)
	if err != nil {
		if k8s_errors.IsNotFound(err) {
			return r.createBootstrapPVC(ctx, instance, bootstrapPVCName)
		}
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *OVNCentralReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&ovncentralv1alpha1.OVNCentral{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&corev1.PersistentVolumeClaim{}).
		Owns(&corev1.Secret{}).
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

// +kubebuilder:rbac:groups=core,resources=persistentvolumeclaims,verbs=get;list;create;update;delete
func (r *OVNCentralReconciler) createBootstrapPVC(ctx context.Context,
	instance *ovncentralv1alpha1.OVNCentral, bootstrapPVCName string) (ctrl.Result, error) {

	bootstrapPVC := stubs.PVC(instance, bootstrapPVCName)
	err := r.Client.Create(ctx, bootstrapPVC)
	if err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{Requeue: true}, nil
}

// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;create;update;delete
