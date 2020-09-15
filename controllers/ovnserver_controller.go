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
	"crypto/sha256"
	"encoding/json"
	"fmt"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	k8s_errors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/rand"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	ovncentralv1alpha1 "github.com/openstack-k8s-operators/ovn-central-operator/api/v1alpha1"
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

const (
	dbInitContainerName = "db-init"
)

// +kubebuilder:rbac:groups=ovn-central.openstack.org,resources=ovnservers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=ovn-central.openstack.org,resources=ovnservers/status,verbs=get;update;patch

func (r *OVNServerReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()
	_ = r.Log.WithValues("ovnserver", req.NamespacedName)

	//
	// Fetch the server object
	//

	server := &ovncentralv1alpha1.OVNServer{}
	if err := r.Client.Get(ctx, req.NamespacedName, server); err != nil {
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
	// Ensure server pod exists
	//

	podIntent := r.Pod(ctx, server, pvc, service)

	// Compute a hash of the intended pod state
	jsonBytes, err := json.Marshal(podIntent)
	if err != nil {
		err = WrapErrorForObject("JSON Encode", podIntent, err)
		return ctrl.Result{}, err
	}
	hashBytes := sha256.Sum256(jsonBytes)
	lastAppliedHash := rand.SafeEncodeString(string(hashBytes[:]))
	lastAppliedName := fmt.Sprintf("%s-last-applied", server.Name)

	podIntent.Annotations[lastAppliedName] = lastAppliedHash

	podCurrent := &corev1.Pod{}
	err = r.Client.Get(ctx, getNamespacedName(podIntent), podCurrent)
	if err != nil {
		if k8s_errors.IsNotFound(err) {
			err = r.Client.Create(ctx, podIntent)
			if err != nil {
				err = WrapErrorForObject("Create", podIntent, err)
				return ctrl.Result{}, err
			}
			LogForObject(r, "Create", podIntent)
			// Created a watched object. Wait for reconcile.
			return ctrl.Result{}, err
		}

		// Error getting object
		err = WrapErrorForObject("Get", podIntent, err)
		return ctrl.Result{}, err
	}

	currentAppliedHash, ok := podCurrent.Annotations[lastAppliedName]
	if !ok || currentAppliedHash != lastAppliedHash {
		if err = r.Client.Delete(ctx, podCurrent); err != nil {
			err = WrapErrorForObject("Delete", podCurrent, err)
			return ctrl.Result{}, nil
		}
		LogForObject(r, "Delete", podCurrent)
		// Deleted a watched object. Wait for reconcile.
		return ctrl.Result{}, nil
	}

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

func (r *OVNServerReconciler) Service(
	ctx context.Context,
	server *ovncentralv1alpha1.OVNServer) (
	*corev1.Service, controllerutil.OperationResult, error) {

	service := &corev1.Service{}
	service.Name = server.Name
	service.Namespace = server.Namespace
	op, err := controllerutil.CreateOrUpdate(ctx, r.Client, service, func() error {
		initLabelMap(&service.Labels)

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
		initLabelMap(&pvc.Labels)
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

func (r *OVNServerReconciler) Pod(
	ctx context.Context, server *ovncentralv1alpha1.OVNServer,
	pvc *corev1.PersistentVolumeClaim,
	svc *corev1.Service) *corev1.Pod {

	serviceName := fmt.Sprintf("%s.%s.svc.cluster.local", svc.Name, svc.Namespace)

	pod := &corev1.Pod{}
	pod.Name = server.Name
	pod.Namespace = server.Namespace

	pod.Annotations = make(map[string]string)
	pod.Labels = make(map[string]string)
	setCommonLabels(server, pod.Labels)

	pod.Spec.RestartPolicy = corev1.RestartPolicyAlways

	// TODO
	// pod.Spec.Affinity

	const (
		hostsVolumeName = "hosts"
		runVolumeName   = "pod-run"
		dataVolumeName  = "data"
	)

	const (
		ovsDBDir  = "/var/lib/openvswitch"
		ovsRunDir = "/pod-run"
	)

	pod.Spec.Volumes = []corev1.Volume{
		{Name: dataVolumeName, VolumeSource: corev1.VolumeSource{
			PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
				ClaimName: pvc.Name}},
		},
		{Name: runVolumeName, VolumeSource: emptyDirVol},
		{Name: hostsVolumeName, VolumeSource: emptyDirVol},
	}

	// Define a local entry for the service in /hosts pointing to the pod
	// IP. This allows ovsdb-server to bind to the 'service ip' on startup.
	const hostsTmpMount = "/hosts-new"
	hostsInitContainer := corev1.Container{
		Name:  "override-local-service-ip",
		Image: server.Spec.Image,
		Env: []corev1.EnvVar{
			envDownwardAPI("POD_IP", "status.podIP"),
			envValue("SVC_NAME", serviceName),
			envValue("HOSTS_VOLUME", hostsTmpMount),
		},
		Command: []string{"/add_service_to_hosts.sh"},
		VolumeMounts: []corev1.VolumeMount{
			volumeMount(hostsVolumeName, hostsTmpMount),
		},
		// XXX: Dev only
		ImagePullPolicy: corev1.PullAlways,
	}

	dbInitContainer := corev1.Container{
		Name:  dbInitContainerName,
		Image: server.Spec.Image,
		Env: []corev1.EnvVar{
			envValue("DB_TYPE", "NB"),
			envValue("SVC_NAME", serviceName),
			envValue("OVS_DBDIR", ovsDBDir),
			envValue("OVS_RUNDIR", ovsRunDir),
			envValue("BOOTSTRAP", "true"),
		},
		Command: []string{"/dbinit.sh"},
		VolumeMounts: []corev1.VolumeMount{
			corev1.VolumeMount{
				Name: hostsVolumeName, MountPath: "/etc/hosts", SubPath: "hosts",
			},
			volumeMount(runVolumeName, ovsRunDir),
			volumeMount(dataVolumeName, ovsDBDir),
		},
	}

	pod.Spec.InitContainers = []corev1.Container{
		hostsInitContainer,
		dbInitContainer,
	}

	dbContainer := corev1.Container{
		Name:  "ovsdb-server",
		Image: server.Spec.Image,
		Env: []corev1.EnvVar{
			envValue("DB_TYPE", "NB"),
			envValue("SVC_NAME", serviceName),
			envValue("OVS_DBDIR", ovsDBDir),
			envValue("OVS_RUNDIR", ovsRunDir),
			envValue("OVN_LOG_LEVEL", "info"),
		},
		VolumeMounts: []corev1.VolumeMount{
			corev1.VolumeMount{
				Name: hostsVolumeName, MountPath: "/etc/hosts", SubPath: "hosts",
			},
			volumeMount(runVolumeName, ovsRunDir),
			volumeMount(dataVolumeName, ovsDBDir),
		},
	}
	dbContainer.ReadinessProbe = execProbe("/is_ready.sh")
	dbContainer.LivenessProbe = execProbe("/is_live.sh")
	dbContainer.LivenessProbe.InitialDelaySeconds = 60

	pod.Spec.Containers = []corev1.Container{
		dbContainer,
	}

	controllerutil.SetControllerReference(server, pod, r.Scheme)

	return pod
}

// Set labels which all objects owned by this server will have
func setCommonLabels(server *ovncentralv1alpha1.OVNServer, labels map[string]string) {
	labels["app"] = "ovsdb-server"
	labels["ovsdb-server"] = server.Name
	// TODO: Add ovn-central label
}

// Inititialise a label map to an empty map if it is nil.
// Unlike some other maps and slices, we don't blindly overwrite this because
// we want to support other tools setting arbitrary labels.
func initLabelMap(m *map[string]string) {
	if *m == nil {
		*m = make(map[string]string)
	}
}

// Syntactic sugar variables and functions

func getNamespacedName(obj metav1.Object) types.NamespacedName {
	return types.NamespacedName{
		Name:      obj.GetName(),
		Namespace: obj.GetNamespace(),
	}
}

var (
	uid0        int64               = 0
	emptyDirVol corev1.VolumeSource = corev1.VolumeSource{
		EmptyDir: &corev1.EmptyDirVolumeSource{},
	}
)

func envDownwardAPI(name, field string) corev1.EnvVar {
	return corev1.EnvVar{
		Name: name,
		ValueFrom: &corev1.EnvVarSource{
			FieldRef: &corev1.ObjectFieldSelector{
				FieldPath: field,
			},
		},
	}
}

func envValue(name, value string) corev1.EnvVar {
	return corev1.EnvVar{Name: name, Value: value}
}

func volumeMount(name, mountPath string) corev1.VolumeMount {
	return corev1.VolumeMount{Name: name, MountPath: mountPath}
}

func execProbe(command ...string) *corev1.Probe {
	return &corev1.Probe{
		Handler: corev1.Handler{Exec: &corev1.ExecAction{Command: command}},
	}
}
