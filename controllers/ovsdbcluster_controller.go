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
	"math"
	"sort"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/api/equality"
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
	sort.Slice(servers.Items, func(i, j int) bool {
		return servers.Items[i].Name < servers.Items[j].Name
	})

	//
	// Update server availability in status
	//
	// This is important for concurrency. By doing this we know that if we ever execute this
	// reconcile concurrently, all continuing threads will have the same view of available and
	// unavailable servers, and will therefore make the same scale up/down decisions. This is
	// defensive, as we do not intend this reconcile loop to be executed concurrently.
	//

	var nAvailable, nInitialised int
	serverSummarys := make([]ovncentralv1alpha1.OVSDBServerSummary, 0, len(servers.Items))
	for _, server := range servers.Items {
		var state ovncentralv1alpha1.OVSDBServerSummaryState
		if util.IsAvailable(&server) {
			state = ovncentralv1alpha1.SummaryStateAvailable
			nAvailable++
		} else if util.IsInitialised(&server) {
			state = ovncentralv1alpha1.SummaryStateInitialised
			nInitialised++
		}

		serverSummarys = append(serverSummarys, ovncentralv1alpha1.OVSDBServerSummary{
			Name:  server.Name,
			State: state,
		})
	}

	if !equality.Semantic.DeepEqual(serverSummarys, cluster.Status.Servers) {
		cluster.Status.Servers = serverSummarys
		if err := r.Client.Status().Update(ctx, cluster); err != nil {
			err = WrapErrorForObject("Update Status.Servers", cluster, err)
			return ctrl.Result{}, err
		}

		return ctrl.Result{}, nil
	}

	//
	// We're Available iff a quorum of servers are Available
	//
	// Quorum is based on the number of servers which currently exist, not the target scale
	//

	origAvailable := util.IsAvailable(cluster)
	nClusterMembers := nAvailable + nInitialised
	quorum := int(math.Ceil(float64(nClusterMembers) / 2))
	if nAvailable >= quorum && nAvailable > 0 {
		util.SetAvailable(cluster)
	} else {
		util.UnsetAvailable(cluster)
	}
	if origAvailable != util.IsAvailable(cluster) {
		if err := r.Client.Status().Update(ctx, cluster); err != nil {
			err = WrapErrorForObject("Update Available condition", cluster, err)
			return ctrl.Result{}, err
		}

		return ctrl.Result{}, nil
	}

	//
	// Unset Failed condition unless it's re-set explicitly
	//

	origConditions := util.DeepCopyConditions(cluster.Status.Conditions)
	util.UnsetFailed(cluster)
	defer CheckConditions(r, ctx, cluster, origConditions, &err)

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
				err = fmt.Errorf("Server %s has inconsistent ClusterID %s. "+
					"Expected ClusterID %s",
					server.Name, *server.Status.ClusterID,
					*cluster.Status.ClusterID)
				util.SetFailed(
					cluster,
					ovncentralv1alpha1.OVSDBClusterInconsistent, err.Error())
				return ctrl.Result{Requeue: false}, err
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
	// Bootstrap the cluster if necessary
	//

	if len(servers.Items) == 0 {
		if cluster.Status.ClusterID != nil {
			err := fmt.Errorf("Cannot re-bootstrap a previously initialised " +
				"cluster with no remaining servers")
			util.SetFailed(cluster,
				ovncentralv1alpha1.OVSDBClusterInvalid, err.Error())
			return ctrl.Result{Requeue: false}, err
		}

		bootstrapServerName := serverNameForIndex(cluster, 0)

		_, _, err := r.Server(ctx, cluster, servers.Items, bootstrapServerName, true)
		if err != nil {
			return ctrl.Result{}, err
		}

		return ctrl.Result{}, nil
	}

	// Wait for bootstrap to complete
	if cluster.Status.ClusterID == nil {
		// It's never going to complete if the bootstrap server failed
		progressing := false
		for _, server := range servers.Items {
			if !util.IsFailed(&server) {
				progressing = true
				break
			}
		}
		if !progressing {
			err := fmt.Errorf("Cluster bootstrapping failed. "+
				"See ovsdbserver/%s for details", servers.Items[0].Name)
			util.SetFailed(cluster,
				ovncentralv1alpha1.OVSDBClusterBootstrap, err.Error())
			return ctrl.Result{Requeue: false}, err
		}

		return ctrl.Result{}, nil
	}

	//
	// Refresh all servers which are running but not currently available
	//
	// These aren't currently part of a quorum and may even be broken, so just update them all
	//

	updated := false
	for _, summary := range serverSummarys {
		if summary.State == ovncentralv1alpha1.SummaryStateAvailable {
			continue
		}

		server, op, err := r.Server(ctx, cluster, servers.Items, summary.Name, false)
		if err != nil {
			err = WrapErrorForObject("Create or update server", server, err)
			return ctrl.Result{}, err
		}

		if op != controllerutil.OperationResultNone {
			updated = true
		}
	}
	if updated {
		return ctrl.Result{}, nil
	}

	//
	// Add servers
	//
	// Only if we have a quorum
	//

	if nAvailable < quorum {
		r.Log.Info("Waiting for quorum")
		return ctrl.Result{}, nil
	}

	if len(serverSummarys) < cluster.Spec.Scale {
		nextIndex := 0
		newServerNames := make([]string, 0, cluster.Spec.Scale-len(serverSummarys))
		for i := len(serverSummarys); i < cluster.Spec.Scale; i++ {
			name := ""
			for {
				name = serverNameForIndex(cluster, nextIndex)
				conflict := false
				for _, server := range serverSummarys {
					if server.Name == name {
						conflict = true
						break
					}
				}
				if !conflict {
					break
				}
				nextIndex++
			}
			newServerNames = append(newServerNames, name)
			serverSummarys = append(serverSummarys,
				ovncentralv1alpha1.OVSDBServerSummary{Name: name})
		}

		sort.Slice(serverSummarys, func(i, j int) bool {
			return serverSummarys[i].Name < serverSummarys[j].Name
		})

		// Update server summaries before creating new servers. This will prevent
		// concurrent runs of this reconciler from creating conflicting plans.
		cluster.Status.Servers = serverSummarys
		if err := r.Client.Status().Update(ctx, cluster); err != nil {
			err = WrapErrorForObject("Update status to add servers", cluster, err)
			return ctrl.Result{}, err
		}

		for _, name := range newServerNames {
			_, _, err := r.Server(ctx, cluster, servers.Items, name, false)
			if err != nil {
				return ctrl.Result{}, nil
			}
		}

		return ctrl.Result{}, nil
	}

	// FIN
	return ctrl.Result{}, nil
}

func serverNameForIndex(cluster *ovncentralv1alpha1.OVSDBCluster, index int) string {
	return fmt.Sprintf("%s-%d", cluster.Name, index)
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
	name string,
	stopped bool) (*ovncentralv1alpha1.OVSDBServer, controllerutil.OperationResult, error) {

	server := &ovncentralv1alpha1.OVSDBServer{}
	server.Name = name
	server.Namespace = cluster.Namespace

	var initPeers []string
	for _, peer := range servers {
		if peer.Name != server.Name && peer.Status.RaftAddress != nil {
			initPeers = append(initPeers, *peer.Status.RaftAddress)
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
		server.Spec.Stopped = stopped

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
