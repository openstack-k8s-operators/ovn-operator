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
	"encoding/json"
	"fmt"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	ovnv1 "github.com/openstack-k8s-operators/ovn-operator/api/v1beta1"
	util "github.com/openstack-k8s-operators/ovn-operator/pkg/common"
)

// OVSDBServerReconciler reconciles a OVSDBServer object
type OVSDBServerReconciler struct {
	client.Client
	Kclient kubernetes.Interface
	Log     logr.Logger
	Scheme  *runtime.Scheme
}

// GetClient -
func (r *OVSDBServerReconciler) GetClient() client.Client {
	return r.Client
}

// GetLogger -
func (r *OVSDBServerReconciler) GetLogger() logr.Logger {
	return r.Log
}

// +kubebuilder:rbac:groups=ovn.openstack.org,resources=ovsdbservers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=ovn.openstack.org,resources=ovsdbservers/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=ovn.openstack.org,resources=ovsdbservers/finalizers,verbs=update
// +kubebuilder:rbac:groups=ovn.openstack.org,resources=ovsdbclusters,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;delete
// +kubebuilder:rbac:groups=core,resources=persistentvolumeclaims,verbs=get;list;watch;create;update;delete
// +kubebuilder:rbac:groups=core,resources=pods/log,verbs=get;list

// Reconcile reconcile OVSDBServer API requests
func (r *OVSDBServerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (res ctrl.Result, err error) {
	_ = context.Background()
	_ = r.Log.WithValues("ovsdbserver", req.NamespacedName)

	//
	// Fetch the server object
	//

	server := &ovnv1.OVSDBServer{}
	if err = r.Client.Get(ctx, req.NamespacedName, server); err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile
			// request. Owned objects are automatically garbage collected. For
			// additional cleanup logic use finalizers. Return and don't requeue.
			return ctrl.Result{}, nil
		}
		err = WrapErrorForObject("Get server", server, err)
		return ctrl.Result{}, err
	}

	//
	// Snapshot the original status object and ensure we save any changes to it on return
	//

	origStatus := server.Status.DeepCopy()
	statusChanged := func() bool {
		return !equality.Semantic.DeepEqual(&server.Status, origStatus)
	}

	defer func() {
		if statusChanged() {
			if updateErr := r.Client.Status().Update(ctx, server); updateErr != nil {
				if err == nil {
					err = WrapErrorForObject(
						"Update Status", server, updateErr)
				} else {
					LogErrorForObject(r, updateErr, "Update status", server)
				}
			}
		}
	}()

	//
	// Fetch the cluster object
	//

	cluster := &ovnv1.OVSDBCluster{}
	err = r.Client.Get(ctx,
		types.NamespacedName{Name: server.Spec.ClusterName, Namespace: server.Namespace}, cluster)
	if err != nil {
		err = WrapErrorForObject("Get cluster for server", server, err)
		return ctrl.Result{}, err
	}

	//
	// Ensure Service exists
	//

	service := serviceShell(server)
	op, err := CreateOrDelete(ctx, r, service, func() error {
		serviceApply(service, server)
		applyServerLabels(server, service.Labels)

		err := controllerutil.SetOwnerReference(server, service, r.Scheme)
		if err != nil {
			return WrapErrorForObject("Set controller reference for service", service, err)
		}
		return nil
	})
	if err != nil {
		return ctrl.Result{}, err
	}
	if op != controllerutil.OperationResultNone {
		LogForObject(r, "Updated service", service)
	}

	//
	// Ensure PVC exists
	//

	pvc := pvcShell(server)
	op, err = controllerutil.CreateOrUpdate(ctx, r.Client, pvc, func() error {
		pvcApply(pvc, server)
		applyServerLabels(server, pvc.Labels)

		err := controllerutil.SetOwnerReference(server, pvc, r.Scheme)
		if err != nil {
			return WrapErrorForObject("Set controller reference for PVC", pvc, err)
		}
		return nil
	})
	if err != nil {
		return ctrl.Result{}, err
	}
	if op != controllerutil.OperationResultNone {
		LogForObject(r, "Updated PVC", pvc)
	}

	//
	// Bootstrap the database if clusterID is not set
	//

	if server.Status.ClusterID == nil {
		return r.bootstrapDB(ctx, server, cluster)
	}

	util.SetInitialized(server)

	if server.Spec.ClusterID != nil && *server.Spec.ClusterID != *server.Status.ClusterID {
		msg := fmt.Sprintf("Expected ClusterID %s but found %s",
			*server.Spec.ClusterID, *server.Status.ClusterID)
		if !util.IsFailed(server) {
			LogForObject(r, msg, server)
		}
		util.SetFailed(server, ovnv1.OVSDBServerInconsistent, msg)
	} else {
		util.UnsetFailed(server)
	}

	// FIN
	return ctrl.Result{}, nil
}

// SetupWithManager -
func (r *OVSDBServerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&ovnv1.OVSDBServer{}).
		Owns(&corev1.Service{}).
		Owns(&corev1.PersistentVolumeClaim{}).
		Owns(&corev1.Pod{}).
		Complete(r)
}

func (r *OVSDBServerReconciler) bootstrapDB(
	ctx context.Context, server *ovnv1.OVSDBServer,
	cluster *ovnv1.OVSDBCluster,
) (ctrl.Result, error) {
	if server.Spec.ClusterID != nil && len(server.Spec.InitPeers) == 0 {
		msg := fmt.Sprintf("Unable to bootstrap server %s into cluster %s without InitPeers",
			server.Name, *server.Spec.ClusterID)
		if !util.IsFailed(server) {
			LogForObject(r, msg, server)
		}
		util.SetFailed(server, ovnv1.OVSDBServerBootstrapInvalid, msg)
		return ctrl.Result{}, nil
	}

	// Ensure the bootstrap pod is running
	bootstrapPod := bootstrapPodShell(server)
	_, err := CreateOrDelete(ctx, r, bootstrapPod, func() error {
		bootstrapPodApply(bootstrapPod, server, cluster)
		applyServerLabels(server, bootstrapPod.Labels)

		err := controllerutil.SetControllerReference(server, bootstrapPod, r.Scheme)
		if err != nil {
			err = WrapErrorForObject(
				"Set controller reference for bootstrap pod", bootstrapPod, err)
		}
		return err

	})
	if err != nil {
		return ctrl.Result{}, WrapErrorForObject("Create bootstrap pod", bootstrapPod, err)
	}

	// Set failed condition if bootstrap failed
	// XXX: This test doesn't work with RestartPolicyOnFailure
	if bootstrapPod.Status.Phase == corev1.PodFailed {
		msg := fmt.Sprintf("Bootstrap pod %s failed. See pod logs for details",
			bootstrapPod.Name)
		if !util.IsFailed(server) {
			LogForObject(r, msg, server)
		}
		util.SetFailed(server, ovnv1.OVSDBServerBootstrapFailed, msg)

		return ctrl.Result{}, nil
	}

	if bootstrapPod.Status.Phase != corev1.PodSucceeded {
		// Wait for bootstrap to complete
		return ctrl.Result{}, nil
	}

	// Read the db status from the completed pod
	logReader, err := util.GetLogStream(ctx, bootstrapPod, dbStatusContainerName, 1024)
	if err != nil {
		return ctrl.Result{}, err
	}
	defer logReader.Close()

	dbStatus := &server.Status.DatabaseStatus
	jsonReader := json.NewDecoder(logReader)
	err = jsonReader.Decode(dbStatus)
	if err != nil {
		return ctrl.Result{},
			fmt.Errorf("Decode database status from container %s in pod/%s logs: %w",
				dbStatusContainerName, bootstrapPod.Name, err)
	}

	if err := r.Client.Delete(ctx, bootstrapPod); err != nil {
		err = WrapErrorForObject("Delete server bootstrap pod", bootstrapPod, err)
		return ctrl.Result{}, err
	}

	util.SetInitialized(server)

	if err := r.Client.Status().Update(ctx, server); err != nil {
		err = WrapErrorForObject("Update db status", server, err)
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func applyServerLabels(server *ovnv1.OVSDBServer, labels map[string]string) {
	labels[OVNCentralLabel] = server.Labels[OVNCentralLabel]
	labels[OVSDBClusterLabel] = server.Labels[OVSDBClusterLabel]
	labels[OVSDBServerLabel] = server.Name
}
