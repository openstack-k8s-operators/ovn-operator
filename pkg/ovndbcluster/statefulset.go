/*
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

package ovndbcluster

import (
	"github.com/openstack-k8s-operators/lib-common/modules/common"
	"github.com/openstack-k8s-operators/lib-common/modules/common/affinity"
	"github.com/openstack-k8s-operators/lib-common/modules/common/env"
	"github.com/openstack-k8s-operators/lib-common/modules/common/tls"
	ovnv1 "github.com/openstack-k8s-operators/ovn-operator/api/v1beta1"
	ovn_common "github.com/openstack-k8s-operators/ovn-operator/pkg/common"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

const (
	// ServiceCommand -
	ServiceCommand = "/usr/local/bin/container-scripts/setup.sh"

	// PvcSuffixEtcOvn -
	PvcSuffixEtcOvn = "-etc-ovn"
)

// StatefulSet func
func StatefulSet(
	instance *ovnv1.OVNDBCluster,
	configHash string,
	labels map[string]string,
	annotations map[string]string,
) *appsv1.StatefulSet {
	livenessProbe := &corev1.Probe{
		// TODO might need tuning
		TimeoutSeconds:      5,
		PeriodSeconds:       3,
		InitialDelaySeconds: 3,
	}
	readinessProbe := &corev1.Probe{
		// TODO might need tuning
		TimeoutSeconds:      5,
		PeriodSeconds:       5,
		InitialDelaySeconds: 5,
	}

	var preStopCmd []string
	var postStartCmd []string
	cmd := []string{"/usr/bin/dumb-init"}
	args := []string{"--single-child", "--", "/bin/bash", "-c", ServiceCommand}
	//
	// https://kubernetes.io/docs/tasks/configure-pod-container/configure-liveness-readiness-startup-probes/
	//
	livenessProbe.Exec = &corev1.ExecAction{
		Command: []string{
			"/usr/bin/pidof", "ovsdb-server",
		},
	}
	readinessProbe.Exec = livenessProbe.Exec

	postStartCmd = []string{
		"/usr/local/bin/container-scripts/settings.sh",
	}
	preStopCmd = []string{
		"/usr/local/bin/container-scripts/cleanup.sh",
	}

	lifecycle := &corev1.Lifecycle{
		PostStart: &corev1.LifecycleHandler{
			Exec: &corev1.ExecAction{
				Command: postStartCmd,
			},
		},
		PreStop: &corev1.LifecycleHandler{
			Exec: &corev1.ExecAction{
				Command: preStopCmd,
			},
		},
	}
	serviceName := ovnv1.ServiceNameNB
	if instance.Spec.DBType == ovnv1.SBDBType {
		serviceName = ovnv1.ServiceNameSB
	}
	envVars := map[string]env.Setter{}
	envVars["CONFIG_HASH"] = env.SetValue(configHash)
	// TODO: Make confs customizable
	envVars["OVN_RUNDIR"] = env.SetValue("/tmp")
	// we have to set LOGDIR even though we don't want to log to file. This is
	// because ovsdb-server will still attempt to write a line into the file
	// before seizing file logging, and the default log file location is not
	// available for write
	envVars["OVN_LOGDIR"] = env.SetValue("/tmp")

	// create Volume and VolumeMounts
	volumes := GetDBClusterVolumes(instance.Name)
	volumeMounts := GetDBClusterVolumeMounts(instance.Name + PvcSuffixEtcOvn)

	// add CA bundle if defined
	if instance.Spec.TLS.CaBundleSecretName != "" {
		volumes = append(volumes, instance.Spec.TLS.CreateVolume())
		volumeMounts = append(volumeMounts, instance.Spec.TLS.CreateVolumeMounts(nil)...)
	}

	// add OVN dbs cert and CA
	if instance.Spec.TLS.Enabled() {
		svc := tls.Service{
			SecretName: *instance.Spec.TLS.GenericService.SecretName,
			CertMount:  ptr.To(ovn_common.OVNDbCertPath),
			KeyMount:   ptr.To(ovn_common.OVNDbKeyPath),
			CaMount:    ptr.To(ovn_common.OVNDbCaCertPath),
		}
		volumes = append(volumes, svc.CreateVolume(serviceName))
		volumeMounts = append(volumeMounts, svc.CreateVolumeMounts(serviceName)...)
	}

	statefulset := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceName,
			Namespace: instance.Namespace,
		},
		Spec: appsv1.StatefulSetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			ServiceName:         serviceName,
			PodManagementPolicy: appsv1.ParallelPodManagement,
			Replicas:            instance.Spec.Replicas,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: annotations,
					Labels:      labels,
				},
				Spec: corev1.PodSpec{
					ServiceAccountName: instance.RbacResourceName(),
					Containers: []corev1.Container{
						{
							Name:                     serviceName,
							Command:                  cmd,
							Args:                     args,
							Image:                    instance.Spec.ContainerImage,
							Env:                      env.MergeEnvs([]corev1.EnvVar{}, envVars),
							VolumeMounts:             volumeMounts,
							Resources:                instance.Spec.Resources,
							ReadinessProbe:           readinessProbe,
							LivenessProbe:            livenessProbe,
							Lifecycle:                lifecycle,
							TerminationMessagePolicy: corev1.TerminationMessageFallbackToLogsOnError,
						},
					},
				},
			},
		},
	}

	// https://kubernetes.io/docs/concepts/workloads/controllers/statefulset/#persistentvolumeclaim-retention
	statefulset.Spec.PersistentVolumeClaimRetentionPolicy = &appsv1.StatefulSetPersistentVolumeClaimRetentionPolicy{
		WhenDeleted: appsv1.DeletePersistentVolumeClaimRetentionPolicyType,
		WhenScaled:  appsv1.RetainPersistentVolumeClaimRetentionPolicyType,
	}

	blockOwnerDeletion := false
	ownerRef := metav1.NewControllerRef(instance, instance.GroupVersionKind())
	ownerRef.BlockOwnerDeletion = &blockOwnerDeletion

	statefulset.Spec.VolumeClaimTemplates = []corev1.PersistentVolumeClaim{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:            instance.Name + PvcSuffixEtcOvn,
				Namespace:       instance.Namespace,
				Labels:          labels,
				OwnerReferences: []metav1.OwnerReference{*ownerRef},
			},
			Spec: corev1.PersistentVolumeClaimSpec{
				AccessModes: []corev1.PersistentVolumeAccessMode{
					corev1.ReadWriteOnce,
				},
				StorageClassName: &instance.Spec.StorageClass,
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceStorage: resource.MustParse(instance.Spec.StorageRequest),
					},
				},
			},
		},
	}
	statefulset.Spec.Template.Spec.Volumes = volumes
	// If possible two pods of the same service should not
	// run on the same worker node. If this is not possible
	// the get still created on the same worker node.
	statefulset.Spec.Template.Spec.Affinity = affinity.DistributePods(
		common.AppSelector,
		[]string{
			serviceName,
		},
		corev1.LabelHostname,
	)
	if instance.Spec.NodeSelector != nil && len(instance.Spec.NodeSelector) > 0 {
		statefulset.Spec.Template.Spec.NodeSelector = instance.Spec.NodeSelector
	}

	return statefulset
}
