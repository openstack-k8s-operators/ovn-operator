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
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	k8s_errors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	ovncentralv1alpha1 "github.com/openstack-k8s-operators/ovn-central-operator/api/v1alpha1"
	"github.com/openstack-k8s-operators/ovn-central-operator/util"
)

// OVNCentralReconciler reconciles a OVNCentral object
type OVNCentralReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

func (r *OVNCentralReconciler) GetClient() client.Client {
	return r.Client
}

func (r *OVNCentralReconciler) GetLogger() logr.Logger {
	return r.Log
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
			// Request object not found, could have been deleted after reconcile
			// request.  Owned objects are automatically garbage collected.  For
			// additional cleanup logic use finalizers. Return and don't requeue.
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		err = WrapErrorForObject("Get instance", instance, err)
		return ctrl.Result{}, err
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
	// Create the bootstrap server
	//

	apply := func(server *ovncentralv1alpha1.OVSDBServer) (*ovncentralv1alpha1.OVSDBServer, error) {
		fetched := &ovncentralv1alpha1.OVSDBServer{}
		err := r.Client.Get(ctx,
			types.NamespacedName{Name: server.Name, Namespace: server.Namespace},
			fetched)
		if err != nil {
			if k8s_errors.IsNotFound(err) {
				err = r.Client.Create(ctx, server)
				if err != nil {
					err = WrapErrorForObject("Create", server, err)
					return nil, err
				}
				LogForObject(r, "Create", server)

				return server, nil
			}

			err = WrapErrorForObject("Get", server, err)
			return nil, err
		}

		if !equality.Semantic.DeepDerivative(server, fetched) {
			server.ResourceVersion = fetched.ResourceVersion
			err = r.Client.Update(ctx, server)
			if err != nil {
				err = WrapErrorForObject("Update", server, err)
				return nil, err
			}
			LogForObject(r, "Update", server)
			return server, nil
		}

		return fetched, nil
	}

	var servers []corev1.LocalObjectReference

	server := r.Server(instance, 0)
	server, err = apply(server)
	if err != nil {
		return ctrl.Result{}, err
	}

	switch {
	case server.IsFailed():
		err = fmt.Errorf("Bootstrap server %s failed", server.Name)
		if condErr := r.setFailed(ctx, instance, "BootstrapFailed", err); condErr != nil {
			return ctrl.Result{}, condErr
		}
		return ctrl.Result{}, err
	case server.IsAvailable():
		// Continue
	default:
		// Wait for server to become either Failed or Available
		return ctrl.Result{}, nil
	}

	servers = append(servers, corev1.LocalObjectReference{Name: server.Name})
	instance.Status.Servers = servers
	err = r.Client.Status().Update(ctx, instance)
	if err != nil {
		err = WrapErrorForObject("Update Status", instance, err)
		return ctrl.Result{}, nil
	}

	return ctrl.Result{}, nil
}

func (r *OVNCentralReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&ovncentralv1alpha1.OVNCentral{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&corev1.Secret{}).
		Owns(&ovncentralv1alpha1.OVSDBServer{}).
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

func (r *OVNCentralReconciler) Server(
	central *ovncentralv1alpha1.OVNCentral,
	index int) *ovncentralv1alpha1.OVSDBServer {

	server := &ovncentralv1alpha1.OVSDBServer{}
	server.Name = fmt.Sprintf("%s-%d", central.Name, index)
	server.Namespace = central.Namespace

	server.Spec.DBType = ovncentralv1alpha1.DBTypeNB
	server.Spec.Image = central.Spec.Image
	server.Spec.StorageSize = central.Spec.StorageSize
	server.Spec.StorageClass = central.Spec.StorageClass

	controllerutil.SetControllerReference(central, server, r.Scheme)
	return server
}

func (r *OVNCentralReconciler) setFailed(
	ctx context.Context, instance *ovncentralv1alpha1.OVNCentral,
	reason status.ConditionReason, conditionErr error) error {

	util.SetFailed(instance, reason, conditionErr.Error())
	if err := r.Client.Status().Update(ctx, instance); err != nil {
		err = WrapErrorForObject("Update status", instance, err)
		return err
	}

	return nil
}

// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;create;update;delete
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;create;update;delete
