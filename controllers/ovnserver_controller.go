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
	k8s_errors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	ovncentralv1alpha1 "github.com/openstack-k8s-operators/ovn-central-operator/api/v1alpha1"
	"github.com/openstack-k8s-operators/ovn-central-operator/util"
)

// OVNServerReconciler reconciles a OVNServer object
type OVNServerReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

func (r *OVNServerReconciler) GetClient() client.Client {
	return r.Client
}

func (r *OVNServerReconciler) GetLogger() logr.Logger {
	return r.Log
}

// +kubebuilder:rbac:groups=ovn-central.openstack.org,resources=ovnservers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=ovn-central.openstack.org,resources=ovnservers/status,verbs=get;update;patch

func (r *OVNServerReconciler) Reconcile(req ctrl.Request) (res ctrl.Result, err error) {
	ctx := context.Background()
	_ = r.Log.WithValues("ovnserver", req.NamespacedName)

	//
	// Fetch the server object
	//

	server := &ovncentralv1alpha1.OVNServer{}
	if err = r.Client.Get(ctx, req.NamespacedName, server); err != nil {
		if k8s_errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request. Owned
			// objects are automatically garbage collected. For additional cleanup logic use
			// finalizers.  Return and don't requeue.
			return ctrl.Result{}, nil
		}
		err = WrapErrorForObject("Get server", server, err)
		return ctrl.Result{}, err
	}

	//
	// Set the Available condition to false until reconciliation completes
	//

	origAvailable := server.IsAvailable()
	if origAvailable {
		server.SetAvailable(false)
	}
	origFailed := server.IsFailed()
	if origFailed {
		server.SetFailed(false, "", nil)
	}

	defer func() {
		if server.IsAvailable() != origAvailable || server.IsFailed() != origFailed {
			updateErr := r.Client.Status().Update(ctx, server)
			if updateErr != nil {
				if err == nil {
					// Return the update error if Reconcile() isn't already
					// returning an error

					err = WrapErrorForObject("Update Status", server, updateErr)
				} else {
					// Otherwise log the update error and leave the original
					// error unchanged

					LogErrorForObject(r, updateErr, "Update", server)
				}
			}
		}
	}()

	//
	// Ensure Service exists
	//

	service, op, err := r.Service(ctx, server)
	if err != nil {
		return ctrl.Result{}, err
	}
	if op != controllerutil.OperationResultNone {
		LogForObject(r, string(op), service)
		// Modified a watched object. Wait for reconcile.
		return ctrl.Result{}, nil
	}
	serviceName := fmt.Sprintf("%s.%s.svc.cluster.local", service.Name, service.Namespace)

	//
	// Ensure PVC exists
	//

	pvc, op, err := r.PVC(ctx, server)
	if err != nil {
		return ctrl.Result{}, err
	}
	if op != controllerutil.OperationResultNone {
		LogForObject(r, string(op), pvc)
		// Modified a watched object. Wait for reconcile.
		return ctrl.Result{}, nil
	}

	//
	// Bootstrap the database if clusterID is not set
	//

	if server.Status.ClusterID == "" {
		return r.bootstrapDB(ctx, server, serviceName, pvc)
	}

	//
	// Ensure server pod exists
	//

	// Delete the bootstrap pod if it exists
	bootstrapPod := bootstrapPodShell(server)
	if err := DeleteIfExists(r, ctx, bootstrapPod); err != nil {
		return ctrl.Result{}, err
	}

	dbPod := dbPodShell(server)
	_, err = CreateOrDelete(r, ctx, dbPod, func() error {
		return r.dbPod(dbPod, server, pvc, serviceName)
	})

	//
	// Update DB Status
	//
	// If the pod is initialised, read DB status from the dbstatus
	// initcontainer and update if necessary
	//

	if util.IsPodConditionSet(corev1.PodInitialized, dbPod) {
		updated, err := r.updateDBStatus(ctx, server, dbPod, dbStatusContainerName)
		if updated || err != nil {
			return ctrl.Result{}, nil
		}

		// Pod initialized, status is uptodate
	}

	//
	// Mark server available if pod is Ready
	//

	if !util.IsPodConditionSet(corev1.PodReady, dbPod) {
		// Wait until pod is ready
		return ctrl.Result{}, nil
	}

	server.SetAvailable(true)

	// FIN
	return ctrl.Result{}, nil
}

func (r *OVNServerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&ovncentralv1alpha1.OVNServer{}).
		Owns(&corev1.Service{}).
		Owns(&corev1.PersistentVolumeClaim{}).
		Owns(&corev1.Pod{}).
		Complete(r)
}

func (r *OVNServerReconciler) updateDBStatus(
	ctx context.Context,
	server *ovncentralv1alpha1.OVNServer,
	pod *corev1.Pod,
	container string) (bool, error) {

	logReader, err := util.GetLogStream(ctx, pod, container, 1024)
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
				container, pod.Name, err)
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

func (r *OVNServerReconciler) bootstrapDB(
	ctx context.Context, server *ovncentralv1alpha1.OVNServer,
	serviceName string, pvc *corev1.PersistentVolumeClaim) (ctrl.Result, error) {

	// Ensure the DB pod isn't running
	dbPod := dbPodShell(server)
	if err := DeleteIfExists(r, ctx, dbPod); err != nil {
		return ctrl.Result{}, err
	}

	// Ensure the bootstrap pod is running
	bootstrapPod := bootstrapPodShell(server)
	_, err := CreateOrDelete(r, ctx, bootstrapPod, func() error {
		return r.bootstrapPod(bootstrapPod, server, pvc, serviceName)
	})

	// Set failed condition if bootstrap failed
	if bootstrapPod.Status.Phase == corev1.PodFailed {
		err = fmt.Errorf("Bootstrap pod %s failed. See pod logs for details",
			bootstrapPod.Name)
		server.SetFailed(true, "BootstrapFailed", err)

		return ctrl.Result{}, err
	}

	if bootstrapPod.Status.Phase != corev1.PodSucceeded {
		// Wait for bootstrap to complete
		return ctrl.Result{}, nil
	}

	// Read DB state from the status container and update server status
	_, err = r.updateDBStatus(ctx, server, bootstrapPod, dbStatusContainerName)
	return ctrl.Result{}, err
}

func (r *OVNServerReconciler) Service(
	ctx context.Context,
	server *ovncentralv1alpha1.OVNServer) (
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

func (r *OVNServerReconciler) PVC(
	ctx context.Context,
	server *ovncentralv1alpha1.OVNServer) (
	*corev1.PersistentVolumeClaim, controllerutil.OperationResult, error) {

	pvc := &corev1.PersistentVolumeClaim{}
	pvc.Name = server.Name
	pvc.Namespace = server.Namespace

	op, err := controllerutil.CreateOrUpdate(ctx, r.Client, pvc, func() error {
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
	})

	return pvc, op, err
}

const (
	hostsVolumeName = "hosts"
	runVolumeName   = "pod-run"
	dataVolumeName  = "data"

	ovsDBDir  = "/var/lib/openvswitch"
	ovsRunDir = "/pod-run"

	dbStatusContainerName = "dbstatus"
)

func dbPodVolumes(volumes *[]corev1.Volume, pvc *corev1.PersistentVolumeClaim) {
	for _, vol := range []corev1.Volume{
		{Name: dataVolumeName, VolumeSource: corev1.VolumeSource{
			PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
				ClaimName: pvc.Name}},
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

func dbPodVolumeMounts(mounts []corev1.VolumeMount) []corev1.VolumeMount {
	return util.MergeVolumeMounts(mounts, util.MountSetterMap{
		hostsVolumeName: util.VolumeMountWithSubpath("/etc/hosts", "hosts"),
		runVolumeName:   util.VolumeMount(ovsRunDir),
		dataVolumeName:  util.VolumeMount(ovsDBDir),
	})
}

// Define a local entry for the service in /hosts pointing to the pod IP. This
// allows ovsdb-server to bind to the 'service ip' on startup.
func hostsInitContainer(container *corev1.Container, server *ovncentralv1alpha1.OVNServer,
	serviceName string) {

	const hostsTmpMount = "/hosts-new"
	container.Name = "override-local-service-ip"
	container.Image = server.Spec.Image
	container.Command = []string{"/add_service_to_hosts"}
	container.VolumeMounts = util.MergeVolumeMounts(container.VolumeMounts, util.MountSetterMap{
		hostsVolumeName: util.VolumeMount(hostsTmpMount),
	})
	container.Env = util.MergeEnvs(container.Env, util.EnvSetterMap{
		"POD_IP":       util.EnvDownwardAPI("status.podIP"),
		"SVC_NAME":     util.EnvValue(serviceName),
		"HOSTS_VOLUME": util.EnvValue(hostsTmpMount),
	})
}

func dbStatusContainer(container *corev1.Container, server *ovncentralv1alpha1.OVNServer, serviceName string) {
	container.Name = dbStatusContainerName
	container.Image = server.Spec.Image
	container.Command = []string{"/dbstatus"}
	container.VolumeMounts = dbPodVolumeMounts(container.VolumeMounts)
	container.Env = util.MergeEnvs(container.Env, util.EnvSetterMap{
		"DB_TYPE":    util.EnvValue("NB"),
		"SVC_NAME":   util.EnvValue(serviceName),
		"OVS_DBDIR":  util.EnvValue(ovsDBDir),
		"OVS_RUNDIR": util.EnvValue(ovsRunDir),
	})
}

func dbPodShell(server *ovncentralv1alpha1.OVNServer) *corev1.Pod {
	pod := &corev1.Pod{}
	pod.Name = server.Name
	pod.Namespace = server.Namespace

	return pod
}

func (r *OVNServerReconciler) dbPod(
	pod *corev1.Pod,
	server *ovncentralv1alpha1.OVNServer,
	pvc *corev1.PersistentVolumeClaim,
	serviceName string) error {

	util.InitLabelMap(&pod.Labels)
	setCommonLabels(server, pod.Labels)

	pod.Spec.RestartPolicy = corev1.RestartPolicyAlways

	// TODO
	// pod.Spec.Affinity

	dbPodVolumes(&pod.Spec.Volumes, pvc)

	if len(pod.Spec.InitContainers) != 2 {
		pod.Spec.InitContainers = make([]corev1.Container, 2)
	}
	hostsInitContainer(&pod.Spec.InitContainers[0], server, serviceName)
	dbStatusContainer(&pod.Spec.InitContainers[1], server, serviceName)

	if len(pod.Spec.Containers) != 1 {
		pod.Spec.Containers = make([]corev1.Container, 1)
	}

	dbContainer := &pod.Spec.Containers[0]
	dbContainer.Name = "ovsdb-server"
	dbContainer.Image = server.Spec.Image
	dbContainer.VolumeMounts = dbPodVolumeMounts(dbContainer.VolumeMounts)
	dbContainer.Env = util.MergeEnvs(dbContainer.Env, util.EnvSetterMap{
		"DB_TYPE":       util.EnvValue("NB"),
		"SVC_NAME":      util.EnvValue(serviceName),
		"OVS_DBDIR":     util.EnvValue(ovsDBDir),
		"OVS_RUNDIR":    util.EnvValue(ovsRunDir),
		"OVN_LOG_LEVEL": util.EnvValue("info"),
	})

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

func bootstrapPodShell(server *ovncentralv1alpha1.OVNServer) *corev1.Pod {
	pod := &corev1.Pod{}
	pod.Name = fmt.Sprintf("%s-bootstrap", server.Name)
	pod.Namespace = server.Namespace

	return pod
}

func (r *OVNServerReconciler) bootstrapPod(
	pod *corev1.Pod,
	server *ovncentralv1alpha1.OVNServer,
	pvc *corev1.PersistentVolumeClaim,
	serviceName string) error {

	util.InitLabelMap(&pod.Labels)
	setCommonLabels(server, pod.Labels)

	pod.Spec.RestartPolicy = corev1.RestartPolicyNever

	// TODO
	// pod.Spec.Affinity
	// We should ensure the bootstrap pod has the same affinity as the db
	// pod to better support late binding PVCs.

	dbPodVolumes(&pod.Spec.Volumes, pvc)

	if len(pod.Spec.InitContainers) != 2 {
		pod.Spec.InitContainers = make([]corev1.Container, 2)
	}
	hostsInitContainer(&pod.Spec.InitContainers[0], server, serviceName)
	dbInitContainer := &pod.Spec.InitContainers[1]
	dbInitContainer.Name = "dbinit"
	dbInitContainer.Image = server.Spec.Image
	dbInitContainer.Command = []string{"/dbinit"}
	dbInitContainer.VolumeMounts = dbPodVolumeMounts(dbInitContainer.VolumeMounts)
	dbInitContainer.Env = util.MergeEnvs(dbInitContainer.Env, util.EnvSetterMap{
		"DB_TYPE":    util.EnvValue("NB"),
		"SVC_NAME":   util.EnvValue(serviceName),
		"OVS_DBDIR":  util.EnvValue(ovsDBDir),
		"OVS_RUNDIR": util.EnvValue(ovsRunDir),
		"BOOTSTRAP":  util.EnvValue("true"),
	})

	if len(pod.Spec.Containers) != 1 {
		pod.Spec.Containers = make([]corev1.Container, 1)
	}
	dbStatusContainer(&pod.Spec.Containers[0], server, serviceName)

	controllerutil.SetControllerReference(server, pod, r.Scheme)

	return nil
}

// Set labels which all objects owned by this server will have
func setCommonLabels(server *ovncentralv1alpha1.OVNServer, labels map[string]string) {
	labels["app"] = "ovsdb-server"
	labels["ovsdb-server"] = server.Name
	// TODO: Add ovn-central label
}
