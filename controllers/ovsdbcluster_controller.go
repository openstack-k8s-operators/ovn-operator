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
	"sort"

	"github.com/go-logr/logr"
	k8s_errors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	ovncentralv1alpha1 "github.com/openstack-k8s-operators/ovn-central-operator/api/v1alpha1"
	"github.com/openstack-k8s-operators/ovn-central-operator/util"
)

// OVSDBClusterReconciler reconciles a OVSDBCluster object
type OVSDBClusterReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

// ReconcilerCommon

func (r *OVSDBClusterReconciler) GetClient() client.Client {
	return r.Client
}

func (r *OVSDBClusterReconciler) GetLogger() logr.Logger {
	return r.Log
}

const (
	clusterLabel = "ovsdb-cluster"
)

// +kubebuilder:rbac:groups=ovn-central.openstack.org,resources=ovsdbclusters,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=ovn-central.openstack.org,resources=ovsdbclusters/status,verbs=get;update;patch

func (r *OVSDBClusterReconciler) Reconcile(req ctrl.Request) (result ctrl.Result, err error) {
	ctx := context.Background()
	_ = r.Log.WithValues("ovsdbcluster", req.NamespacedName)

	//
	// Fetch the cluster object
	//

	cluster := &ovncentralv1alpha1.OVSDBCluster{}
	if err := r.Client.Get(ctx, req.NamespacedName, cluster); err != nil {
		if k8s_errors.IsNotFound(err) {
			// Request object not found, could have been deleted after
			// reconcile request. Owned objects are automatically garbage
			// collected. For additional cleanup logic use finalizers.
			// Return and don't requeue.
			return ctrl.Result{}, nil
		}
		err = WrapErrorForObject("Get cluster", cluster, err)
		return ctrl.Result{}, err
	}

	//
	// Reset conditions
	//

	origConditions := util.DeepCopyConditions(cluster.Status.Conditions)
	util.UnsetAvailable(cluster)
	util.UnsetFailed(cluster)
	defer CheckConditions(r, ctx, cluster, origConditions, &err)

	//
	// Get a list of the OVSDBServers we manage
	//

	servers := &ovncentralv1alpha1.OVSDBServerList{}
	serverListOpts := &client.ListOptions{Namespace: cluster.Namespace}
	client.MatchingLabels{
		clusterLabel: cluster.Name,
	}.ApplyToList(serverListOpts)
	if err := r.Client.List(ctx, servers, serverListOpts); err != nil {
		err = fmt.Errorf("Error listing servers for cluster %s: %w", cluster.Name, err)
		return ctrl.Result{}, err
	}

	//
	// Set ClusterID from server ClusterIDs
	//

	clusterIDUpdated := false
	for _, server := range servers.Items {
		if cluster.Status.ClusterID == nil {
			cluster.Status.ClusterID = server.Status.ClusterID
			clusterIDUpdated = true
		} else if server.Status.ClusterID != nil {
			if *cluster.Status.ClusterID != *server.Status.ClusterID {
				msg := fmt.Sprintf("Server %s has inconsistent ClusterID %s. "+
					"Expected ClusterID %s",
					server.Name, *server.Status.ClusterID, *cluster.Status.ClusterID)
				util.SetFailed(
					cluster,
					ovncentralv1alpha1.OVSDBClusterInconsistent, msg)
				return ctrl.Result{}, fmt.Errorf(msg)
			}

		}
	}
	if clusterIDUpdated {
		if err := r.Client.Status().Update(ctx, cluster); err != nil {
			err = WrapErrorForObject("Set ClusterID", cluster, err)
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	//
	// Bring up the first server in the cluster, which will bootstrap if necessary
	//

	server, _, err := r.Server(ctx, cluster, servers.Items, 0)
	if err != nil {
		return ctrl.Result{}, err
	}

	if cluster.Status.ClusterID == nil && util.IsFailed(server) {
		msg := fmt.Sprintf("Cluster bootstrapping failed. "+
			"See ovsdbserver/%s for details", server.Name)
		util.SetFailed(cluster, ovncentralv1alpha1.OVSDBClusterBootstrap, msg)
		return ctrl.Result{}, fmt.Errorf(msg)
	}

	if cluster.Status.ClusterID == nil {
		// Wait until bootstrap is finished
		return ctrl.Result{}, nil
	}

	util.SetAvailable(cluster)

	// FIN
	return ctrl.Result{}, nil
}

func (r *OVSDBClusterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&ovncentralv1alpha1.OVSDBCluster{}).
		Owns(&ovncentralv1alpha1.OVSDBServer{}).
		Complete(r)
}

func (r *OVSDBClusterReconciler) Server(
	ctx context.Context,
	cluster *ovncentralv1alpha1.OVSDBCluster,
	servers []ovncentralv1alpha1.OVSDBServer,
	index int) (*ovncentralv1alpha1.OVSDBServer, controllerutil.OperationResult, error) {

	server := &ovncentralv1alpha1.OVSDBServer{}
	server.Name = fmt.Sprintf("%s-%d", cluster.Name, index)
	server.Namespace = cluster.Namespace

	var initPeers []string
	for _, peer := range servers {
		if peer.Name != server.Name && peer.Status.RaftAddress != nil {
			initPeers = append(initPeers, *server.Status.RaftAddress)
		}
	}
	sort.Strings(initPeers)

	op, err := controllerutil.CreateOrUpdate(ctx, r.Client, server, func() error {
		util.InitLabelMap(&server.Labels)
		server.Labels["app"] = clusterLabel
		server.Labels[clusterLabel] = cluster.Name

		server.Spec.DBType = cluster.Spec.DBType
		server.Spec.ClusterID = cluster.Status.ClusterID
		server.Spec.InitPeers = initPeers
		server.Spec.Image = cluster.Spec.Image

		server.Spec.StorageSize = cluster.Spec.ServerStorageSize
		server.Spec.StorageClass = cluster.Spec.ServerStorageClass

		err := controllerutil.SetControllerReference(cluster, server, r.Scheme)
		if err != nil {
			return WrapErrorForObject("SetControllerReference", server, err)
		}

		return nil
	})

	if err != nil {
		err = WrapErrorForObject("CreateOrUpdate Server", server, err)
	}

	return server, op, err
}
