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
	"math/rand"
	"sort"
	"strings"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	ovnv1 "github.com/openstack-k8s-operators/ovn-operator/api/v1beta1"
	util "github.com/openstack-k8s-operators/ovn-operator/pkg/common"
)

// OVSDBClusterReconciler reconciles a OVSDBCluster object
type OVSDBClusterReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

// ReconcilerCommon

// GetClient -
func (r *OVSDBClusterReconciler) GetClient() client.Client {
	return r.Client
}

// GetLogger -
func (r *OVSDBClusterReconciler) GetLogger() logr.Logger {
	return r.Log
}

const (
	// OVSDBClusterLabel - cluster label
	OVSDBClusterLabel = "ovsdb-cluster"
	// OVSDBServerLabel - server label
	OVSDBServerLabel = "ovsdb-server"

	// OVSDBClusterFinalizer - cluster finalizer
	OVSDBClusterFinalizer = "ovsdbcluster.ovn.openstack.org"
)

type clusterKickError struct{}

func (e clusterKickError) Error() string { return "" }

// +kubebuilder:rbac:groups=ovn.openstack.org,resources=ovsdbclusters,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=ovn.openstack.org,resources=ovsdbclusters/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=ovn.openstack.org,resources=ovsdbservers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch;create;update;delete
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;create;update;delete

// Reconcile reconcile OVSDBCluster API requests
func (r *OVSDBClusterReconciler) Reconcile(ctx context.Context, req ctrl.Request) (result ctrl.Result, err error) {
	_ = context.Background()
	_ = r.Log.WithValues("ovsdbcluster", req.NamespacedName)

	//
	// Fetch the cluster object
	//

	cluster := &ovnv1.OVSDBCluster{}
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

	// Add a finalizer to ourselves
	if !controllerutil.ContainsFinalizer(cluster, OVSDBClusterFinalizer) {
		controllerutil.AddFinalizer(cluster, OVSDBClusterFinalizer)
		if err := r.Client.Update(ctx, cluster); err != nil {
			err = WrapErrorForObject("Add finalizer to cluster", cluster, err)
			return ctrl.Result{}, err
		}

		r.Log.Info("Added cluster finalizer")
		return ctrl.Result{}, nil
	}

	//
	// Snapshot the original status object and ensure we save any changes to it on return
	//

	origStatus := cluster.Status.DeepCopy()
	statusChanged := func() bool {
		return !equality.Semantic.DeepEqual(&cluster.Status, origStatus)
	}

	defer func() {
		if statusChanged() {
			if updateErr := r.Client.Status().Update(ctx, cluster); updateErr != nil {
				if err == nil {
					err = WrapErrorForObject(
						"Update Status", cluster, updateErr)
				} else {
					LogErrorForObject(r, updateErr, "Update status", cluster)
				}
			}
		}
	}()

	//
	// Get all the OVSDBServers we manage
	//

	servers, err := r.getServers(ctx, cluster)
	if err != nil {
		return ctrl.Result{}, err
	}

	//
	// Get all the DB server Pods we manage
	//

	serverPods, err := r.getServerPods(ctx, cluster)
	if err != nil {
		return ctrl.Result{}, err
	}

	//
	// Finalize cluster if it has been deleted
	//

	if util.IsDeleted(cluster) {
		return r.finalizeCluster(ctx, cluster, servers)
	}

	//
	// Update status based on the observed state of servers and pods
	//

	r.updateClusterStatus(cluster, servers, serverPods)
	if statusChanged() {
		// Status will be saved automatically
		return ctrl.Result{}, nil
	}

	//
	// Update servers
	//

	if err := r.updateServers(ctx, cluster, servers); err != nil {
		return ctrl.Result{}, err
	}

	//
	// Ensure we have a pod for each available server
	//

	if err := r.updateServerPods(ctx, cluster, servers, serverPods); err != nil {
		return ctrl.Result{}, err
	}

	//
	// Finalize servers
	//

	if err := r.finalizeServers(ctx, cluster, servers, serverPods); err != nil {
		if _, ok := err.(clusterKickError); ok {
			return ctrl.Result{RequeueAfter: time.Duration(10) * time.Second}, nil
		}

		return ctrl.Result{}, err
	}

	//
	// Write ConfigMap
	//

	if err := r.writeConfigMap(ctx, cluster, servers); err != nil {
		return ctrl.Result{}, err
	}

	// FIN
	return ctrl.Result{}, nil
}

func findServer(name string, servers []ovnv1.OVSDBServer) *ovnv1.OVSDBServer {
	for i := 0; i < len(servers); i++ {
		if servers[i].Name == name {
			return &servers[i]
		}
	}
	return nil
}

func findPod(name string, serverPods []corev1.Pod) *corev1.Pod {
	for i := 0; i < len(serverPods); i++ {
		if serverPods[i].Name == name {
			return &serverPods[i]
		}
	}
	return nil
}

func nextServerName(
	cluster *ovnv1.OVSDBCluster,
	servers []ovnv1.OVSDBServer,
) string {

	for i := 0; ; i++ {
		name := fmt.Sprintf("%s-%d", cluster.Name, i)
		if findServer(name, servers) == nil {
			return name
		}
	}
}

func (r *OVSDBClusterReconciler) getServers(
	ctx context.Context,
	cluster *ovnv1.OVSDBCluster,
) ([]ovnv1.OVSDBServer, error) {

	serverList := &ovnv1.OVSDBServerList{}
	serverListOpts := &client.ListOptions{Namespace: cluster.Namespace}
	client.MatchingLabels{
		OVSDBClusterLabel: cluster.Name,
	}.ApplyToList(serverListOpts)
	if err := r.Client.List(ctx, serverList, serverListOpts); err != nil {
		err = fmt.Errorf("Error listing servers for cluster %s: %w", cluster.Name, err)
		return nil, err
	}

	servers := serverList.Items
	sort.Slice(servers, func(i, j int) bool {
		return servers[i].Name < servers[j].Name
	})

	return servers, nil
}

func (r *OVSDBClusterReconciler) getServerPods(
	ctx context.Context,
	cluster *ovnv1.OVSDBCluster,
) ([]corev1.Pod, error) {

	podList := &corev1.PodList{}
	podListOpts := &client.ListOptions{Namespace: cluster.Namespace}
	client.MatchingLabels{
		"app":             "ovsdb-server",
		OVSDBClusterLabel: cluster.Name,
	}.ApplyToList(podListOpts)
	if err := r.Client.List(ctx, podList, podListOpts); err != nil {
		err = fmt.Errorf("Error listing server pods for cluster %s: %w", cluster.Name, err)
		return nil, err
	}

	pods := podList.Items
	sort.Slice(pods, func(i, j int) bool {
		return pods[i].Name < pods[j].Name
	})

	return pods, nil
}

// SetupWithManager -
func (r *OVSDBClusterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&ovnv1.OVSDBCluster{}).
		Owns(&ovnv1.OVSDBServer{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&corev1.Pod{}).
		Complete(r)
}

func (r *OVSDBClusterReconciler) finalizeCluster(
	ctx context.Context,
	cluster *ovnv1.OVSDBCluster,
	servers []ovnv1.OVSDBServer,
) (ctrl.Result, error) {
	// Remove finalizer from all managed servers
	for i := 0; i < len(servers); i++ {
		server := &servers[i]
		if controllerutil.ContainsFinalizer(server, OVSDBClusterFinalizer) {
			controllerutil.RemoveFinalizer(server, OVSDBClusterFinalizer)

			if err := r.Client.Update(ctx, server); err != nil {
				err = WrapErrorForObject(
					"Remove server finalizer", server, err)
				return ctrl.Result{}, err
			}

			LogForObject(r, "Removed finalizer from server", server)
		}
	}

	controllerutil.RemoveFinalizer(cluster, OVSDBClusterFinalizer)
	if err := r.Client.Update(ctx, cluster); err != nil {
		err = WrapErrorForObject(
			"Remove cluster finalizer", cluster, err)
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *OVSDBClusterReconciler) finalizeServers(
	ctx context.Context,
	cluster *ovnv1.OVSDBCluster,
	servers []ovnv1.OVSDBServer,
	serverPods []corev1.Pod,
) error {
	for i := 0; i < len(servers); i++ {
		server := &servers[i]
		if util.IsDeleted(server) &&
			controllerutil.ContainsFinalizer(server, OVSDBClusterFinalizer) {

			// Pick an available server pod to kick the deleted server
			var kickerPod *corev1.Pod
			kickerPod = findPod(server.Name, serverPods)
			for i := rand.Intn(len(serverPods)); i < len(serverPods); i++ {
				pod := &serverPods[i%len(serverPods)]
				if util.IsPodReady(&serverPods[i]) {
					kickerPod = pod
					break
				}
			}
			if kickerPod == nil {
				LogForObject(r, "No server pod available to finalize server", server)
				// No point trying to finalize anything else right now
				break
			}

			r.Log.Info("Kicking server from cluster",
				"server", server.Name, "exec pod", kickerPod.Name)

			result, err := util.PodExec(kickerPod, DBServerContainerName,
				[]string{"/cluster-kick", *server.Status.ServerID}, false)
			if err != nil {
				return WrapErrorForObject("Sending exec for cluster kick", kickerPod, err)
			}

			if result.ExitStatus != 0 {
				r.Log.Info("Failed to kick server from cluster",
					"server", server.Name, "exec pod", kickerPod.Name)

				return clusterKickError{}
			}

			serverPod := findPod(server.Name, serverPods)
			if serverPod != nil {
				if err := r.Client.Delete(ctx, serverPod); err != nil {
					return WrapErrorForObject("Delete db pod during finalizer", serverPod, err)
				}
			}

			controllerutil.RemoveFinalizer(server, OVSDBClusterFinalizer)
			if err := r.Client.Update(ctx, server); err != nil {
				return WrapErrorForObject("Remove cluster finalizer from server", server, err)
			}
		}
	}

	return nil
}

func (r *OVSDBClusterReconciler) updateClusterStatus(
	cluster *ovnv1.OVSDBCluster,
	servers []ovnv1.OVSDBServer,
	serverPods []corev1.Pod,
) {
	// Cluster size is the basis of the quorum calculation. It is the number of
	// servers which have been added to the cluster, not the target number of
	// replicas.
	clusterSize := 0
	for i := 0; i < len(servers); i++ {
		if util.IsInitialized(&servers[i]) {
			clusterSize++
		}
	}
	clusterQuorum := int(math.Ceil(float64(clusterSize) / 2))

	nAvailable := 0
	for i := 0; i < len(serverPods); i++ {
		if util.IsPodReady(&serverPods[i]) {
			nAvailable++
		}
	}

	// We're Available iff a quorum of server pods are Ready
	if nAvailable >= clusterQuorum && nAvailable > 0 {
		util.SetAvailable(cluster)
	} else {
		util.UnsetAvailable(cluster)
	}

	cluster.Status.AvailableServers = nAvailable
	cluster.Status.ClusterSize = clusterSize
	cluster.Status.ClusterQuorum = clusterQuorum

	// We're failed iff any of our servers have failed
	var failedServers []string

	// Set ClusterID from server ClusterIDs
	for i := 0; i < len(servers); i++ {
		server := &servers[i]
		if util.IsFailed(server) {
			failedServers = append(failedServers, server.Name)
		}

		if cluster.Status.ClusterID == nil {
			cluster.Status.ClusterID = server.Status.ClusterID
		}

		// We will update all servers with this clusterID. If any contain a
		// different database they will mark themselves failed.
	}

	// If any servers have failed, also set failed on the cluster
	if len(failedServers) > 0 {
		msg := fmt.Sprintf("The following servers have failed to intialize: %s",
			strings.Join(failedServers, ", "))
		if !util.IsFailed(cluster) {
			LogForObject(r, msg, cluster)
		}
		util.SetFailed(cluster, ovnv1.OVSDBClusterServers, msg)
	} else {
		util.UnsetFailed(cluster)
	}
}

func (r *OVSDBClusterReconciler) updateServers(
	ctx context.Context,
	cluster *ovnv1.OVSDBCluster,
	servers []ovnv1.OVSDBServer,
) error {

	// A list of all servers we're going to update or add
	var updateServers []*ovnv1.OVSDBServer

	// Servers to be updated
	for i := 0; i < len(servers); i++ {
		server := &servers[i]
		if !util.IsDeleted(server) {
			updateServers = append(updateServers, server)
		}
	}

	// Create new servers if required

	targetServers := cluster.Spec.Replicas
	if targetServers == 0 {
		// The cluster needs at least one server as a datastore, even if it's not running
		targetServers = 1
	}
	if cluster.Status.ClusterID == nil {
		// Create exactly one server if we're not bootstrapped
		targetServers = 1
	}

	for i := len(servers); int32(i) < targetServers; i++ {
		name := nextServerName(cluster, servers)
		updateServers = append(updateServers, serverShell(cluster, name))
	}

	for _, server := range updateServers {
		apply := func() error {
			serverApply(cluster, server, servers)
			applyClusterLabels(cluster, server.Labels)

			controllerutil.AddFinalizer(server, OVSDBClusterFinalizer)
			err := controllerutil.SetControllerReference(
				cluster, server, r.Scheme)
			if err != nil {
				return WrapErrorForObject(
					"Set controller reference for server", server, err)
			}

			return err
		}
		op, err := controllerutil.CreateOrUpdate(ctx, r.Client, server, apply)
		if err != nil {
			return err
		}

		if op != controllerutil.OperationResultNone {
			LogForObject(r, "Created/Updated server", server)
		}
	}

	return nil
}

func (r *OVSDBClusterReconciler) updateServerPods(
	ctx context.Context,
	cluster *ovnv1.OVSDBCluster,
	servers []ovnv1.OVSDBServer,
	serverPods []corev1.Pod,
) error {
	status := &cluster.Status

	if cluster.Spec.Replicas == 0 {
		// If we scale down to zero we'll still have 1 server, but we don't want any
		// running pods.
		for i := 0; i < len(serverPods); i++ {
			serverPod := &serverPods[i]
			if err := r.Delete(ctx, serverPod); err != nil {
				return WrapErrorForObject("Delete server pod", &serverPods[i], err)
			}
			LogForObject(r, "Deleted server pod", serverPod)
		}
	} else {
		for i := 0; i < len(servers); i++ {
			server := &servers[i]

			// Ignore if server is bootstrapping or deleting
			if !util.IsInitialized(server) || util.IsDeleted(server) {
				continue
			}

			serverPod := findPod(server.Name, serverPods)

			// If the server is Failed, ensure that it is not running
			if util.IsFailed(server) {
				if serverPod != nil {
					if err := r.Client.Delete(ctx, serverPod); err != nil {
						if !errors.IsNotFound(err) {
							return WrapErrorForObject("Delete pod of failed server", serverPod, err)
						}
					}
					LogForObject(r, "Deleted pod of failed server", serverPod)
				}

				continue
			}

			// If the pod already exists, updating could potentially cause an outage of
			// it, so check we have a super-quorum.
			if serverPod != nil {
				// Updating a cluster with less than 3 servers will always cause
				// loss of quorum, so ignore it.
				if status.ClusterSize >= 3 && status.AvailableServers <= status.ClusterQuorum {
					continue
				}
			} else {
				serverPod = dbServerShell(server)
			}

			apply := func() error {
				dbServerApply(serverPod, server, cluster)
				applyServerLabels(server, serverPod.Labels)

				if err := controllerutil.SetControllerReference(
					cluster, serverPod, r.Scheme); err != nil {

					err = WrapErrorForObject(
						"Set controller reference for server pod",
						serverPod, err)
					return err
				}
				return nil
			}
			op, err := CreateOrDelete(ctx, r, serverPod, apply)
			if err != nil {
				err = WrapErrorForObject("Update server pod", serverPod, err)
				return err
			}
			if op != controllerutil.OperationResultNone {
				status.AvailableServers--
				LogForObject(r, string(op), serverPod)
			}
		}
	}

	return nil
}

func (r *OVSDBClusterReconciler) writeConfigMap(
	ctx context.Context,
	cluster *ovnv1.OVSDBCluster,
	servers []ovnv1.OVSDBServer,
) error {
	if cluster.Spec.ClientConfig == nil || cluster.Status.ClusterID == nil {
		return nil
	}

	configMap := &corev1.ConfigMap{}
	configMap.Name = *cluster.Spec.ClientConfig
	configMap.Namespace = cluster.Namespace

	apply := func() error {
		util.InitLabelMap(&configMap.Labels)
		applyClusterLabels(cluster, configMap.Labels)

		var methods []string = make([]string, 0, len(servers)+1)
		methods = append(methods, fmt.Sprintf("cid:%s", *cluster.Status.ClusterID))

		for i := 0; i < len(servers); i++ {
			server := &servers[i]
			if util.IsInitialized(server) {
				methods = append(methods, *server.Status.DBAddress)
			}
		}

		util.InitLabelMap(&configMap.Data)
		configMap.Data["connection"] = strings.Join(methods, ",")

		return controllerutil.SetControllerReference(cluster, configMap, r.Scheme)
	}
	op, err := controllerutil.CreateOrUpdate(ctx, r.Client, configMap, apply)
	if err != nil {
		return WrapErrorForObject("Create or update config map", configMap, err)
	}
	if op != controllerutil.OperationResultNone {
		LogForObject(r, string(op), configMap, "cluster", cluster.Name)
	}

	return nil
}

func serverShell(
	cluster *ovnv1.OVSDBCluster,
	name string,
) *ovnv1.OVSDBServer {

	server := &ovnv1.OVSDBServer{}
	server.Name = name
	server.Namespace = cluster.Namespace
	return server
}

func serverApply(
	cluster *ovnv1.OVSDBCluster,
	server *ovnv1.OVSDBServer,
	servers []ovnv1.OVSDBServer,
) {

	var initPeers []string
	for i := 0; i < len(servers); i++ {
		peer := &servers[i]

		if peer.Name != server.Name && peer.Status.RaftAddress != nil {
			initPeers = append(initPeers, *peer.Status.RaftAddress)
		}
	}

	util.InitLabelMap(&server.Labels)

	server.Spec.DBType = cluster.Spec.DBType
	server.Spec.ClusterID = cluster.Status.ClusterID
	server.Spec.ClusterName = cluster.Name
	server.Spec.InitPeers = initPeers

	server.Spec.StorageSize = cluster.Spec.ServerStorageSize
	server.Spec.StorageClass = cluster.Spec.ServerStorageClass
}

func applyClusterLabels(cluster *ovnv1.OVSDBCluster, labels map[string]string) {
	labels[OVNCentralLabel] = cluster.Labels[OVNCentralLabel]
	labels[OVSDBClusterLabel] = cluster.Name
}
