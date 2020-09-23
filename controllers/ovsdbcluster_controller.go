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
	"k8s.io/apimachinery/pkg/api/errors"
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
		if errors.IsNotFound(err) {
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

	serverList := &ovncentralv1alpha1.OVSDBServerList{}
	serverListOpts := &client.ListOptions{Namespace: cluster.Namespace}
	client.MatchingLabels{
		clusterLabel: cluster.Name,
	}.ApplyToList(serverListOpts)
	if err := r.Client.List(ctx, serverList, serverListOpts); err != nil {
		err = fmt.Errorf("Error listing servers for cluster %s: %w", cluster.Name, err)
		return ctrl.Result{}, err
	}
	servers := serverList.Items
	sort.Slice(servers, func(i, j int) bool {
		return servers[i].Name < servers[j].Name
	})

	//
	// We're Available iff a quorum of servers are Available
	//
	// Quorum is based on the number of servers which have been initialised into the cluster,
	// not the target number of replicas.
	//

	var nAvailable, nInitialised int
	for _, server := range servers {
		if util.IsAvailable(&server) {
			nAvailable++
		} else if util.IsInitialised(&server) {
			nInitialised++
		}
	}

	clusterSize := nAvailable + nInitialised
	clusterQuorum := int(math.Ceil(float64(clusterSize) / 2))

	if cluster.Status.AvailableServers != nAvailable ||
		cluster.Status.ClusterSize != clusterSize ||
		cluster.Status.ClusterQuorum != clusterQuorum {

		cluster.Status.AvailableServers = nAvailable
		cluster.Status.ClusterSize = clusterSize
		cluster.Status.ClusterQuorum = clusterQuorum

		return UpdateStatus(r, ctx, cluster, "Update cluster stats in status")
	}

	origAvailable := util.IsAvailable(cluster)
	if nAvailable >= clusterQuorum && nAvailable > 0 {
		util.SetAvailable(cluster)
	} else {
		util.UnsetAvailable(cluster)
	}
	if origAvailable != util.IsAvailable(cluster) {
		return UpdateStatus(r, ctx, cluster, "Update Available condition")
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
	for _, server := range servers {
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
				return ctrl.Result{}, err
			}

		}
	}
	if clusterIDUpdated {
		return UpdateStatus(r, ctx, cluster, "Set ClusterID")
	}

	//
	// Bootstrap the cluster if necessary
	//

	if len(servers) == 0 {
		if cluster.Status.ClusterID != nil {
			err := fmt.Errorf("Cannot re-bootstrap a previously initialised " +
				"cluster with no remaining servers")
			util.SetFailed(cluster,
				ovncentralv1alpha1.OVSDBClusterInvalid, err.Error())
			return ctrl.Result{}, err
		}

		bootstrapServerName := serverNameForIndex(cluster, 0)

		bootstrapServer := serverShell(cluster, bootstrapServerName)
		_, err := controllerutil.CreateOrUpdate(ctx, r.Client, bootstrapServer,
			func() error {
				return r.serverApply(cluster, bootstrapServer, servers, true)
			})
		if err != nil {
			return ctrl.Result{}, err
		}

		return ctrl.Result{}, nil
	}

	// Wait for bootstrap to complete
	if cluster.Status.ClusterID == nil {
		// It's never going to complete if the bootstrap server failed
		progressing := false
		for _, server := range servers {
			if !util.IsFailed(&server) {
				progressing = true
				break
			}
		}
		if !progressing {
			err := fmt.Errorf("Cluster bootstrapping failed. "+
				"See ovsdbserver/%s for details", servers[0].Name)
			util.SetFailed(cluster,
				ovncentralv1alpha1.OVSDBClusterBootstrap, err.Error())
			return ctrl.Result{}, err
		}

		return ctrl.Result{}, nil
	}

	//
	// Check in progress operations
	//

	findServer := func(name string) *ovncentralv1alpha1.OVSDBServer {
		n := sort.Search(len(servers), func(i int) bool {
			return servers[i].Name >= name
		})

		if n == len(servers) {
			return nil
		}
		return &servers[n]
	}

	var inProgress []ovncentralv1alpha1.OVSDBServerOperation
	for _, operation := range cluster.Status.Operations {
		server := findServer(operation.Name)
		if operation.Terminate {
			if server == nil {
				r.Log.Info("Observed deletion of server", "server", operation.Name)
			} else if server.DeletionTimestamp == nil {
				if err := r.Client.Delete(ctx, server); err != nil {
					err = WrapErrorForObject("Delete server", server, err)
					return ctrl.Result{}, err
				}

				LogForObject(r, "Deleted server", server)
				inProgress = append(inProgress, operation)
			}
		} else {
			switch {
			case server != nil && operation.UID == nil:
				r.Log.Info("Observed creation of server", "server", operation.Name)
			case server != nil && operation.UID != nil && server.UID != *operation.UID:
				r.Log.Info("Object scheduled for update was deleted")
				inProgress = append(inProgress, operation)
				inProgress[len(inProgress)-1].TargetGeneration = 0
			case server != nil &&
				operation.TargetGeneration > 0 &&
				server.Status.ObservedGeneration >= operation.TargetGeneration:
				r.Log.Info("Observed update of server", "server", operation.Name)
			default:
				server = serverShell(cluster, operation.Name)
				_, err = controllerutil.CreateOrUpdate(ctx, r.Client, server,
					func() error {
						return r.serverApply(cluster, server, servers, false)
					})
				if err != nil {
					return ctrl.Result{}, err
				}
				inProgress = append(inProgress, operation)
				inProgress[len(inProgress)-1].TargetGeneration = server.Generation
			}
		}
	}
	if !equality.Semantic.DeepEqual(cluster.Status.Operations, inProgress) {
		cluster.Status.Operations = inProgress
		return UpdateStatus(r, ctx, cluster, "Remove completed operations")
	}

	//
	// Wait for in progress operations to complete before continuing
	//

	if len(cluster.Status.Operations) > 0 {
		r.Log.Info("Waiting for in progress server operations")
		return ctrl.Result{}, nil
	}

	//
	// Refresh existing servers
	//
	// Only allow one in progress update at a time, and only if we have quorum+1 available
	// servers.
	//

	// Ignore quorum if cluster size is less than 3. Updating a cluster of this size will
	// always result in loss of quorum.
	if clusterSize >= 3 && nAvailable < clusterQuorum+1 {
		r.Log.Info("Waiting for quorum")
		return ctrl.Result{}, nil
	}

	for _, server := range servers {
		update, err := NeedsUpdate(r, ctx, &server, func() error {
			return r.serverApply(cluster, &server, servers, false)
		})
		if err != nil {
			return ctrl.Result{}, err
		}

		if update {
			cluster.Status.Operations = append(cluster.Status.Operations,
				ovncentralv1alpha1.OVSDBServerOperation{
					Name: server.Name,
					UID:  &server.UID,
				})
			return UpdateStatus(r, ctx, cluster, "Schedule server update",
				"server", server.Name)
		}
	}

	//
	// Add servers
	//
	// Only if we have a quorum
	//

	if nAvailable < clusterQuorum {
		r.Log.Info("Waiting for quorum")
		return ctrl.Result{}, nil
	}

	if len(servers) < cluster.Spec.Replicas {
		nextIndex := 0
		for i := len(servers); i < cluster.Spec.Replicas; i++ {
			name := ""
			for {
				name = serverNameForIndex(cluster, nextIndex)
				if findServer(name) == nil {
					break
				}
				nextIndex++
			}

			cluster.Status.Operations = append(cluster.Status.Operations,
				ovncentralv1alpha1.OVSDBServerOperation{
					Name: name,
				})
			nextIndex++
		}

		return UpdateStatus(r, ctx, cluster, "Add create server operations")
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

func serverShell(
	cluster *ovncentralv1alpha1.OVSDBCluster,
	name string) *ovncentralv1alpha1.OVSDBServer {

	server := &ovncentralv1alpha1.OVSDBServer{}
	server.Name = name
	server.Namespace = cluster.Namespace
	return server
}

func (r *OVSDBClusterReconciler) serverApply(
	cluster *ovncentralv1alpha1.OVSDBCluster,
	server *ovncentralv1alpha1.OVSDBServer,
	peers []ovncentralv1alpha1.OVSDBServer,
	stopped bool) error {

	var initPeers []string
	for _, peer := range peers {
		if peer.Name != server.Name && peer.Status.RaftAddress != nil {
			initPeers = append(initPeers, *peer.Status.RaftAddress)
		}
	}

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

	return err
}
