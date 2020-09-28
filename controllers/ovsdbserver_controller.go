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
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	ovncentralv1alpha1 "github.com/openstack-k8s-operators/ovn-central-operator/api/v1alpha1"
	"github.com/openstack-k8s-operators/ovn-central-operator/util"
)

const (
	OVSDBServerFinalizer = "ovsdbserver.ovn-central.openstack.org"
)

// OVSDBServerReconciler reconciles a OVSDBServer object
type OVSDBServerReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

func (r *OVSDBServerReconciler) GetClient() client.Client {
	return r.Client
}

func (r *OVSDBServerReconciler) GetLogger() logr.Logger {
	return r.Log
}

// +kubebuilder:rbac:groups=ovn-central.openstack.org,resources=ovsdbservers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=ovn-central.openstack.org,resources=ovsdbservers/status,verbs=get;update;patch

func (r *OVSDBServerReconciler) Reconcile(req ctrl.Request) (res ctrl.Result, err error) {
	ctx := context.Background()
	_ = r.Log.WithValues("ovsdbserver", req.NamespacedName)

	//
	// Fetch the server object
	//

	server := &ovncentralv1alpha1.OVSDBServer{}
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
	// Unset Failed and Available conditions unless they are re-set explicitly
	//

	origConditions := util.DeepCopyConditions(server.Status.Conditions)
	util.UnsetFailed(server)
	util.UnsetAvailable(server)
	defer CheckConditions(r, ctx, server, origConditions, &err)

	//
	// Run the finalizer if we're deleting.
	// Add it if it's not present.
	//

	if !server.DeletionTimestamp.IsZero() {
		return r.finalizer(ctx, server)
	}

	if !controllerutil.ContainsFinalizer(server, OVSDBServerFinalizer) {
		controllerutil.AddFinalizer(server, OVSDBServerFinalizer)
		if err := r.Client.Update(ctx, server); err != nil {
			err = WrapErrorForObject("Add finalizer", server, err)
			return ctrl.Result{}, err
		}

		LogForObject(r, "Added finalizer to server", server)
		return ctrl.Result{}, nil
	}

	//
	// Ensure Service exists
	//

	service, op, err := r.service(ctx, server)
	if err != nil {
		return ctrl.Result{}, err
	}
	if op != controllerutil.OperationResultNone {
		LogForObject(r, "Updated service", service)
		// Modified a watched object. Wait for reconcile.
		return ctrl.Result{}, nil
	}

	//
	// Ensure PVC exists
	//

	pvc := pvcShell(server)
	op, err = controllerutil.CreateOrUpdate(ctx, r.Client, pvc, func() error {
		return r.pvcApply(pvc, server)
	})
	if err != nil {
		return ctrl.Result{}, err
	}
	if op != controllerutil.OperationResultNone {
		LogForObject(r, "Updated PVC", pvc)
		// Modified a watched object. Wait for reconcile.
		return ctrl.Result{}, nil
	}

	//
	// Bootstrap the database if clusterID is not set
	//

	if server.Status.ClusterID == nil {
		return r.bootstrapDB(ctx, server)
	}

	//
	// Delete the bootstrap pod if it exists
	//

	bootstrapPod := bootstrapPodShell(server)
	if err := DeleteIfExists(r, ctx, bootstrapPod); err != nil {
		return ctrl.Result{}, err
	}

	//
	// Ensure server pod is not running if Stopped is set
	//

	dbPod := dbPodShell(server)
	if server.Spec.Stopped {
		if err := DeleteIfExists(r, ctx, dbPod); err != nil {
			return ctrl.Result{}, err
		}

		return r.reconcileComplete(ctx, server)
	}

	//
	// Ensure server pod exists
	//

	op, err = CreateOrDelete(r, ctx, dbPod, func() error {
		return r.dbPodApply(dbPod, server)
	})
	if op != controllerutil.OperationResultNone {
		LogForObject(r, "Updated server pod", dbPod)
		return ctrl.Result{}, nil
	}

	//
	// Update DB Status
	//
	// If the pod is initialised, read DB status from the dbstatus
	// initcontainer and update if necessary
	//

	dbPod, err = r.getDBPod(ctx, server)
	if err != nil {
		err = WrapErrorForObject("Get DB Pod", dbPod, err)
		return ctrl.Result{}, nil
	}

	if !dbPod.DeletionTimestamp.IsZero() {
		LogForObject(r, "Waiting for pod to terminate", dbPod)
		return ctrl.Result{}, nil
	}

	if !util.IsPodConditionSet(corev1.PodInitialized, dbPod) {
		LogForObject(r, "Waiting for pod to initialise", server)
		return ctrl.Result{}, nil
	}
	util.SetInitialised(server)
	updated, err := r.updateDBStatus(ctx, server, dbPod, dbStatusContainerName)
	if updated || err != nil {
		return ctrl.Result{}, err
	}

	if !util.IsPodConditionSet(corev1.PodReady, dbPod) {
		LogForObject(r, "Wait for pod to become ready", server)
		return ctrl.Result{}, nil
	}

	util.SetAvailable(server)

	// FIN
	return r.reconcileComplete(ctx, server)
}

func (r *OVSDBServerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&ovncentralv1alpha1.OVSDBServer{}).
		Owns(&corev1.Service{}).
		Owns(&corev1.PersistentVolumeClaim{}).
		Owns(&corev1.Pod{}).
		Complete(r)
}

func (r *OVSDBServerReconciler) reconcileComplete(
	ctx context.Context,
	server *ovncentralv1alpha1.OVSDBServer) (ctrl.Result, error) {

	if server.Generation != server.Status.ObservedGeneration {
		server.Status.ObservedGeneration = server.Generation
		if err := r.Client.Status().Update(ctx, server); err != nil {
			err = WrapErrorForObject("Update ObservedGeneration", server, err)
			return ctrl.Result{}, err
		}
	}

	LogForObject(r, "Successfully reconciled server", server, "Generation", server.Generation)

	return ctrl.Result{}, nil
}

func serviceName(server *ovncentralv1alpha1.OVSDBServer) string {
	return fmt.Sprintf("%s.%s.svc.cluster.local", server.Name, server.Namespace)
}

func pvcName(server *ovncentralv1alpha1.OVSDBServer) string {
	return server.Name
}

func (r *OVSDBServerReconciler) getDBPod(
	ctx context.Context,
	server *ovncentralv1alpha1.OVSDBServer) (*corev1.Pod, error) {

	dbPod := dbPodShell(server)
	dbPodKey, err := client.ObjectKeyFromObject(dbPod)
	if err != nil {
		err = WrapErrorForObject("ObjectKeyFromObject db pod", dbPod, err)
		return dbPod, err
	}
	if err := r.Client.Get(ctx, dbPodKey, dbPod); err != nil {
		return dbPod, err
	}
	return dbPod, nil
}

func (r *OVSDBServerReconciler) finalizer(
	ctx context.Context,
	server *ovncentralv1alpha1.OVSDBServer) (ctrl.Result, error) {

	if !controllerutil.ContainsFinalizer(server, OVSDBServerFinalizer) {
		LogForObject(r, "Waiting for another finalizer", server)
		return ctrl.Result{}, nil
	}

	// Ensure the DB pod isn't running
	dbPod := dbPodShell(server)
	if err := DeleteIfExists(r, ctx, dbPod); err != nil {
		return ctrl.Result{}, err
	}

	finalizerPod := finalizerPodShell(server)
	op, err := CreateOrDelete(r, ctx, finalizerPod, func() error {
		return r.finalizerPodApply(finalizerPod, server)
	})
	if err != nil {
		err = WrapErrorForObject("Create finalizer pod", finalizerPod, err)
		return ctrl.Result{}, err
	}
	if op == controllerutil.OperationResultCreated {
		LogForObject(r, "Created finalizer pod", server)
	}
	if finalizerPod.Status.Phase != corev1.PodSucceeded {
		LogForObject(r, "Waiting for finalizer to succeed", server)
		return ctrl.Result{}, err
	}
	if err := r.Client.Delete(ctx, finalizerPod); err != nil {
		err = WrapErrorForObject("Delete finalizer pod", finalizerPod, err)
		return ctrl.Result{}, err
	}

	// Ensure we've deleted the PVC before deleting the server. This avoids a race with the
	// garbage collector if we immediately create a new server. This server has been removed
	// from the cluster, so attempting to re-use its data will fail.
	pvc := pvcShell(server)
	if err := DeleteIfExists(r, ctx, pvc); err != nil {
		err = WrapErrorForObject("Delete PVC", pvc, err)
		return ctrl.Result{}, err
	}

	controllerutil.RemoveFinalizer(server, OVSDBServerFinalizer)
	if err := r.Client.Update(ctx, server); err != nil {
		err = WrapErrorForObject("Remove finalizer for server", server, err)
		return ctrl.Result{}, err
	}
	LogForObject(r, "Finalize server completed successfully", server)

	return ctrl.Result{}, nil
}

func (r *OVSDBServerReconciler) updateDBStatus(
	ctx context.Context,
	server *ovncentralv1alpha1.OVSDBServer,
	pod *corev1.Pod,
	statusContainerName string) (bool, error) {

	logReader, err := util.GetLogStream(ctx, pod, statusContainerName, 1024)
	if err != nil {
		return false, err
	}
	defer logReader.Close()

	dbStatus := &ovncentralv1alpha1.DatabaseStatus{}
	jsonReader := json.NewDecoder(logReader)
	err = jsonReader.Decode(dbStatus)
	if err != nil {
		return false,
			fmt.Errorf("Decode database status from container %s in pod/%s logs: %w",
				statusContainerName, pod.Name, err)
	}

	if !equality.Semantic.DeepDerivative(dbStatus, &server.Status.DatabaseStatus) {
		r.Log.Info(fmt.Sprintf("Read db status: %v", *dbStatus))
		server.Status.DatabaseStatus = *dbStatus
		err = r.Client.Status().Update(ctx, server)
		if err != nil {
			return false, WrapErrorForObject("Update Status", server, err)
		}

		return true, nil
	}

	return false, nil
}

func (r *OVSDBServerReconciler) bootstrapDB(
	ctx context.Context, server *ovncentralv1alpha1.OVSDBServer) (ctrl.Result, error) {

	// Ensure the DB pod isn't running
	dbPod := dbPodShell(server)
	if err := DeleteIfExists(r, ctx, dbPod); err != nil {
		return ctrl.Result{}, err
	}

	// Ensure the bootstrap pod is running
	bootstrapPod := bootstrapPodShell(server)
	_, err := CreateOrDelete(r, ctx, bootstrapPod, func() error {
		return r.bootstrapPodApply(bootstrapPod, server)
	})

	// Set failed condition if bootstrap failed
	if bootstrapPod.Status.Phase == corev1.PodFailed {
		err := fmt.Errorf("Bootstrap pod %s failed. See pod logs for details",
			bootstrapPod.Name)
		util.SetFailed(server, ovncentralv1alpha1.OVSDBServerBootstrapFailed, err.Error())

		return ctrl.Result{}, err
	}

	if bootstrapPod.Status.Phase != corev1.PodSucceeded {
		// Wait for bootstrap to complete
		return ctrl.Result{}, nil
	}

	// If we created a new DB we're now initialised. If not we need to wait until we've
	// actually started the db server and synced with another server.
	if server.Spec.ClusterID == nil {
		util.SetInitialised(server)
	}

	// Read DB state from the status container and update server status
	_, err = r.updateDBStatus(ctx, server, bootstrapPod, dbStatusContainerName)
	return ctrl.Result{}, err
}

func (r *OVSDBServerReconciler) service(
	ctx context.Context,
	server *ovncentralv1alpha1.OVSDBServer) (
	*corev1.Service, controllerutil.OperationResult, error) {

	service := &corev1.Service{}
	service.Name = server.Name
	service.Namespace = server.Namespace
	op, err := controllerutil.CreateOrUpdate(ctx, r.Client, service, func() error {
		util.InitLabelMap(&service.Labels)

		setCommonLabels(server, service.Labels)

		// XXX: Selector is immutable. If we ever changed common labels
		// we'd need to delete the Service to update this. Should
		// probably use a minimal set instead.
		service.Spec.Selector = make(map[string]string)
		setCommonLabels(server, service.Spec.Selector)

		makePort := func(name string, port int32) corev1.ServicePort {
			return corev1.ServicePort{
				Name:       name,
				Port:       port,
				Protocol:   corev1.ProtocolTCP,
				TargetPort: intstr.FromInt(int(port)),
			}
		}

		service.Spec.Ports = []corev1.ServicePort{
			makePort("north", 6641),
			makePort("south", 6642),
			makePort("north-raft", 6643),
			makePort("south-raft", 6644),
		}

		service.Spec.Type = corev1.ServiceTypeClusterIP

		// There are 2 reasons we need this.
		//
		// 1. The raft cluster communicates using this service. If we
		//    don't add the pod to the service until it becomes ready,
		//    it can never become ready.
		//
		// 2. A potential client race. A client attempting a
		//    leader-only connection must be able to connect to the
		//    leader at the time. Any delay in making the leader
		//    available for connections could result in incorrect
		//    behaviour.
		service.Spec.PublishNotReadyAddresses = true

		err := controllerutil.SetControllerReference(server, service, r.Scheme)
		if err != nil {
			return WrapErrorForObject("SetControllerReference", service, err)
		}

		return nil
	})

	return service, op, err
}

func pvcShell(
	server *ovncentralv1alpha1.OVSDBServer) *corev1.PersistentVolumeClaim {

	pvc := &corev1.PersistentVolumeClaim{}
	pvc.Name = pvcName(server)
	pvc.Namespace = server.Namespace

	return pvc
}

func (r *OVSDBServerReconciler) pvcApply(
	pvc *corev1.PersistentVolumeClaim,
	server *ovncentralv1alpha1.OVSDBServer) error {

	util.InitLabelMap(&pvc.Labels)
	setCommonLabels(server, pvc.Labels)

	pvc.Spec.Resources.Requests = corev1.ResourceList{
		corev1.ResourceStorage: server.Spec.StorageSize,
	}

	// StorageClassName will be defaulted server-side if we
	// originally passed an empty one, so don't try to overwrite
	// it.
	if server.Spec.StorageClass != nil {
		pvc.Spec.StorageClassName = server.Spec.StorageClass
	}

	pvc.Spec.AccessModes = []corev1.PersistentVolumeAccessMode{
		corev1.ReadWriteOnce,
	}

	volumeMode := corev1.PersistentVolumeFilesystem
	pvc.Spec.VolumeMode = &volumeMode

	err := controllerutil.SetControllerReference(server, pvc, r.Scheme)
	if err != nil {
		return WrapErrorForObject("SetControllerReference", pvc, err)
	}

	return nil
}

const (
	hostsVolumeName = "hosts"
	runVolumeName   = "pod-run"
	dataVolumeName  = "data"

	ovnDBDir  = "/var/lib/openvswitch"
	ovnRunDir = "/ovn-run"

	dbStatusContainerName = "dbstatus"
)

func dbPodVolumes(volumes *[]corev1.Volume, server *ovncentralv1alpha1.OVSDBServer) {
	for _, vol := range []corev1.Volume{
		{Name: dataVolumeName, VolumeSource: corev1.VolumeSource{
			PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
				ClaimName: pvcName(server)}},
		},
		{Name: runVolumeName, VolumeSource: util.EmptyDirVol()},
		{Name: hostsVolumeName, VolumeSource: util.EmptyDirVol()},
	} {
		updated := false
		for i := 0; i < len(*volumes); i++ {
			if (*volumes)[i].Name == vol.Name {
				(*volumes)[i] = vol
				updated = true
				break
			}
		}
		if !updated {
			*volumes = append(*volumes, vol)
		}
	}
}

func dbContainerVolumeMounts(mounts []corev1.VolumeMount) []corev1.VolumeMount {
	return util.MergeVolumeMounts(mounts, util.MountSetterMap{
		hostsVolumeName: util.VolumeMountWithSubpath("/etc/hosts", "hosts"),
		runVolumeName:   util.VolumeMount(ovnRunDir),
		dataVolumeName:  util.VolumeMount(ovnDBDir),
	})
}

func dbContainerEnv(envs []corev1.EnvVar, server *ovncentralv1alpha1.OVSDBServer) []corev1.EnvVar {
	return util.MergeEnvs(envs, util.EnvSetterMap{
		"DB_TYPE":       util.EnvValue(server.Spec.DBType),
		"SERVER_NAME":   util.EnvValue(serviceName(server)),
		"OVN_DBDIR":     util.EnvValue(ovnDBDir),
		"OVN_RUNDIR":    util.EnvValue(ovnRunDir),
		"OVN_LOG_LEVEL": util.EnvValue("info"),
	})
}

// Define a local entry for the service in /hosts pointing to the pod IP. This
// allows ovsdb-server to bind to the 'service ip' on startup.
func hostsInitContainerApply(container *corev1.Container, server *ovncentralv1alpha1.OVSDBServer) {

	const hostsTmpMount = "/hosts-new"
	container.Name = "override-local-service-ip"
	container.Image = server.Spec.Image
	container.Command = []string{
		"/bin/bash",
		"-c",
		"cp /etc/hosts $HOSTS_VOLUME/hosts; " +
			"echo \"$POD_IP $SERVER_NAME\" >> $HOSTS_VOLUME/hosts",
	}
	container.VolumeMounts = util.MergeVolumeMounts(container.VolumeMounts, util.MountSetterMap{
		hostsVolumeName: util.VolumeMount(hostsTmpMount),
	})
	container.Env = util.MergeEnvs(container.Env, util.EnvSetterMap{
		"POD_IP":       util.EnvDownwardAPI("status.podIP"),
		"SERVER_NAME":  util.EnvValue(serviceName(server)),
		"HOSTS_VOLUME": util.EnvValue(hostsTmpMount),
	})

	// XXX: Dev only. Both pods use this container, so this ensures we
	// always pull the latest image.
	container.ImagePullPolicy = corev1.PullAlways
}

func dbStatusContainerApply(
	container *corev1.Container,
	server *ovncentralv1alpha1.OVSDBServer) {

	container.Name = dbStatusContainerName
	container.Image = server.Spec.Image
	container.Command = []string{"/dbstatus"}
	container.VolumeMounts = dbContainerVolumeMounts(container.VolumeMounts)
	container.Env = dbContainerEnv(container.Env, server)
}

func dbServerContainerApply(
	container *corev1.Container,
	server *ovncentralv1alpha1.OVSDBServer) {

	container.Name = "ovsdb-server"
	container.Image = server.Spec.Image
	container.Command = []string{"/dbserver"}
	container.VolumeMounts = dbContainerVolumeMounts(container.VolumeMounts)
	container.Env = dbContainerEnv(container.Env, server)
}

func dbPodShell(server *ovncentralv1alpha1.OVSDBServer) *corev1.Pod {
	pod := &corev1.Pod{}
	pod.Name = server.Name
	pod.Namespace = server.Namespace

	return pod
}

func (r *OVSDBServerReconciler) dbPodApply(
	pod *corev1.Pod,
	server *ovncentralv1alpha1.OVSDBServer) error {

	util.InitLabelMap(&pod.Labels)
	setCommonLabels(server, pod.Labels)

	pod.Spec.RestartPolicy = corev1.RestartPolicyAlways

	// TODO
	// pod.Spec.Affinity

	dbPodVolumes(&pod.Spec.Volumes, server)

	if len(pod.Spec.InitContainers) != 2 {
		pod.Spec.InitContainers = make([]corev1.Container, 2)
	}
	hostsInitContainerApply(&pod.Spec.InitContainers[0], server)
	dbStatusContainerApply(&pod.Spec.InitContainers[1], server)

	if len(pod.Spec.Containers) != 1 {
		pod.Spec.Containers = make([]corev1.Container, 1)
	}

	dbContainer := &pod.Spec.Containers[0]
	dbServerContainerApply(dbContainer, server)

	dbContainer.ReadinessProbe = util.ExecProbe("/is_ready")
	dbContainer.ReadinessProbe.PeriodSeconds = 10
	dbContainer.ReadinessProbe.SuccessThreshold = 1
	dbContainer.ReadinessProbe.FailureThreshold = 1
	dbContainer.ReadinessProbe.TimeoutSeconds = 60

	dbContainer.LivenessProbe = util.ExecProbe("/is_live")
	dbContainer.LivenessProbe.InitialDelaySeconds = 60
	dbContainer.LivenessProbe.PeriodSeconds = 10
	dbContainer.LivenessProbe.SuccessThreshold = 1
	dbContainer.LivenessProbe.FailureThreshold = 3
	dbContainer.LivenessProbe.TimeoutSeconds = 10

	controllerutil.SetControllerReference(server, pod, r.Scheme)

	return nil
}

func bootstrapPodShell(server *ovncentralv1alpha1.OVSDBServer) *corev1.Pod {
	pod := &corev1.Pod{}
	pod.Name = fmt.Sprintf("%s-bootstrap", server.Name)
	pod.Namespace = server.Namespace

	return pod
}

func (r *OVSDBServerReconciler) bootstrapPodApply(
	pod *corev1.Pod,
	server *ovncentralv1alpha1.OVSDBServer) error {

	util.InitLabelMap(&pod.Labels)
	setCommonLabels(server, pod.Labels)

	pod.Spec.RestartPolicy = corev1.RestartPolicyOnFailure

	// TODO
	// pod.Spec.Affinity
	// We should ensure the bootstrap pod has the same affinity as the db
	// pod to better support late binding PVCs.

	dbPodVolumes(&pod.Spec.Volumes, server)

	if len(pod.Spec.InitContainers) != 2 {
		pod.Spec.InitContainers = make([]corev1.Container, 2)
	}
	hostsInitContainerApply(&pod.Spec.InitContainers[0], server)
	dbInitContainer := &pod.Spec.InitContainers[1]
	dbInitContainer.Name = "dbinit"
	dbInitContainer.Image = server.Spec.Image
	dbInitContainer.VolumeMounts = dbContainerVolumeMounts(dbInitContainer.VolumeMounts)
	dbInitContainer.Env = dbContainerEnv(dbInitContainer.Env, server)

	if server.Spec.ClusterID == nil {
		dbInitContainer.Command = []string{"/cluster-create"}
	} else {
		if len(server.Spec.InitPeers) == 0 {
			err := fmt.Errorf("Unable to bootstrap server %s into cluster %s: "+
				"no InitPeers defined", server.Name, *server.Spec.ClusterID)
			util.SetFailed(server,
				ovncentralv1alpha1.OVSDBServerBootstrapInvalid, err.Error())
			return err
		}
		dbInitContainer.Command = []string{"/cluster-join", *server.Spec.ClusterID}
		dbInitContainer.Command = append(dbInitContainer.Command, server.Spec.InitPeers...)
	}

	if len(pod.Spec.Containers) != 1 {
		pod.Spec.Containers = make([]corev1.Container, 1)
	}
	dbStatusContainerApply(&pod.Spec.Containers[0], server)

	controllerutil.SetControllerReference(server, pod, r.Scheme)

	return nil
}

func finalizerPodShell(server *ovncentralv1alpha1.OVSDBServer) *corev1.Pod {
	pod := &corev1.Pod{}
	pod.Name = fmt.Sprintf("%s-finalizer", server.Name)
	pod.Namespace = server.Namespace

	return pod
}

func (r *OVSDBServerReconciler) finalizerPodApply(
	pod *corev1.Pod,
	server *ovncentralv1alpha1.OVSDBServer) error {

	util.InitLabelMap(&pod.Labels)
	setCommonLabels(server, pod.Labels)
	pod.Spec.RestartPolicy = corev1.RestartPolicyOnFailure

	dbPodVolumes(&pod.Spec.Volumes, server)

	if len(pod.Spec.InitContainers) != 1 {
		pod.Spec.InitContainers = make([]corev1.Container, 1)
	}

	hostsInitContainerApply(&pod.Spec.InitContainers[0], server)

	if len(pod.Spec.Containers) != 1 {
		pod.Spec.Containers = make([]corev1.Container, 1)
	}

	clusterLeaveContainer := &pod.Spec.Containers[0]
	clusterLeaveContainer.Name = "cluster-leave"
	clusterLeaveContainer.Image = server.Spec.Image
	clusterLeaveContainer.VolumeMounts = dbContainerVolumeMounts(
		clusterLeaveContainer.VolumeMounts)
	clusterLeaveContainer.Env = dbContainerEnv(clusterLeaveContainer.Env, server)
	clusterLeaveContainer.Command = []string{"/cluster-leave"}

	controllerutil.SetControllerReference(server, pod, r.Scheme)

	return nil
}

// Set labels which all objects owned by this server will have
func setCommonLabels(server *ovncentralv1alpha1.OVSDBServer, labels map[string]string) {
	labels["app"] = "ovsdb-server"
	labels["ovsdb-server"] = server.Name
	// TODO: Add ovn-central label
}
