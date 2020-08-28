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
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	ovncentralv1alpha1 "github.com/openstack-k8s-operators/ovn-central-operator/api/v1alpha1"
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

// +kubebuilder:rbac:groups=ovn-central.openstack.org,resources=ovncentrals,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=ovn-central.openstack.org,resources=ovncentrals/status,verbs=get;update;patch

func (r *OVNCentralReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	_ = r.Log.WithValues("ovncentral", req.NamespacedName)
	ctx := context.Background()

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

	ret := r.setDefaultValues(req, ctx, instance)
	if ret != nil {
		return ret.result, ret.err
	}

	return ctrl.Result{}, nil
}

func (r *OVNCentralReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&ovncentralv1alpha1.OVNCentral{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&corev1.Secret{}).
		Complete(r)
}

func (r *OVNCentralReconciler) setDefaultValues(req ctrl.Request, ctx context.Context,
	instance *ovncentralv1alpha1.OVNCentral) *reconcilerReturn {

	// Check that ConnectionConfig, ConnectionCA, and ConnectionCert have
	// non-empty values. Default them if they are not set.
	var updatedInstance bool
	if instance.Spec.ConnectionConfig == "" {
		instance.Spec.ConnectionConfig = fmt.Sprintf("%s-connection", instance.Name)
		r.Log.Info("Defaulting empty ConnectionConfig", "value", instance.Spec.ConnectionConfig)

		updatedInstance = true
	}
	if instance.Spec.ConnectionCA == "" {
		instance.Spec.ConnectionCA = fmt.Sprintf("%s-ca", instance.Name)
		r.Log.Info("Defaulting empty ConnectionCA", "value", instance.Spec.ConnectionCA)
		updatedInstance = true
	}
	if instance.Spec.ConnectionCert == "" {
		instance.Spec.ConnectionCert = fmt.Sprintf("%s-cert", instance.Name)
		r.Log.Info("Defaulting empty ConnectionCert", "value", instance.Spec.ConnectionCert)
		updatedInstance = true
	}

	if updatedInstance {
		err := r.Client.Update(ctx, instance)
		if err != nil {
			return &reconcilerReturn{ctrl.Result{}, err}
		}

		return &reconcilerReturn{ctrl.Result{Requeue: true}, nil}
	}

	return nil
}

// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;create;update;delete
