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

package ovncontroller

import (
	"fmt"

	topologyv1 "github.com/openstack-k8s-operators/infra-operator/apis/topology/v1beta1"
	"github.com/openstack-k8s-operators/lib-common/modules/common/env"
	"github.com/openstack-k8s-operators/lib-common/modules/common/tls"
	ovnv1 "github.com/openstack-k8s-operators/ovn-operator/api/v1beta1"
	ovn_common "github.com/openstack-k8s-operators/ovn-operator/pkg/common"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
)

func CreateOVNDaemonSet(
	instance *ovnv1.OVNController,
	configHash string,
	labels map[string]string,
	topology *topologyv1.Topology,
) *appsv1.DaemonSet {
	volumes := GetOVNControllerVolumes(instance.Name, instance.Namespace)
	mounts := GetOVNControllerVolumeMounts()

	cmd := []string{
		"ovn-controller", "--pidfile", "unix:/run/openvswitch/db.sock",
	}

	// add OVN dbs cert and CA
	if instance.Spec.TLS.Enabled() {
		svc := tls.Service{
			SecretName: *instance.Spec.TLS.GenericService.SecretName,
			CertMount:  ptr.To(ovn_common.OVNDbCertPath),
			KeyMount:   ptr.To(ovn_common.OVNDbKeyPath),
			CaMount:    ptr.To(ovn_common.OVNDbCaCertPath),
		}
		volumes = append(volumes, svc.CreateVolume(ovnv1.ServiceNameOVNController))
		mounts = append(mounts, svc.CreateVolumeMounts(ovnv1.ServiceNameOVNController)...)

		// add CA bundle if defined
		if instance.Spec.TLS.CaBundleSecretName != "" {
			volumes = append(volumes, instance.Spec.TLS.CreateVolume())
			mounts = append(mounts, instance.Spec.TLS.CreateVolumeMounts(nil)...)
		}

		cmd = append(cmd, []string{
			fmt.Sprintf("--certificate=%s", ovn_common.OVNDbCertPath),
			fmt.Sprintf("--private-key=%s", ovn_common.OVNDbKeyPath),
			fmt.Sprintf("--ca-cert=%s", ovn_common.OVNDbCaCertPath),
		}...)
	}

	ovnControllerLivenessProbe := &corev1.Probe{
		// TODO might need tuning
		TimeoutSeconds:      5,
		PeriodSeconds:       5,
		InitialDelaySeconds: 30,
	}

	ovnControllerReadinessProbe := &corev1.Probe{
		// TODO might need tuning
		TimeoutSeconds:      5,
		PeriodSeconds:       5,
		InitialDelaySeconds: 30,
	}

	ovnControllerLivenessProbe.Exec = &corev1.ExecAction{
		Command: []string{
			"/usr/local/bin/container-scripts/ovn_controller_liveness.sh",
		},
	}

	ovnControllerReadinessProbe.Exec = &corev1.ExecAction{
		Command: []string{
			"/usr/local/bin/container-scripts/ovn_controller_readiness.sh",
		},
	}

	runAsUser := int64(0)
	privileged := true

	envVars := map[string]env.Setter{}
	envVars["CONFIG_HASH"] = env.SetValue(configHash)

	containers := []corev1.Container{
		{
			Name:    "ovn-controller",
			Command: cmd,
			Lifecycle: &corev1.Lifecycle{
				PreStop: &corev1.LifecycleHandler{
					Exec: &corev1.ExecAction{
						Command: []string{"/usr/share/ovn/scripts/ovn-ctl", "stop_controller"},
					},
				},
			},
			Image: instance.Spec.OvnContainerImage,
			SecurityContext: &corev1.SecurityContext{
				Capabilities: &corev1.Capabilities{
					Add:  []corev1.Capability{"NET_ADMIN", "SYS_ADMIN", "SYS_NICE"},
					Drop: []corev1.Capability{},
				},
				RunAsUser:  &runAsUser,
				Privileged: &privileged,
			},
			Env:            env.MergeEnvs([]corev1.EnvVar{}, envVars),
			VolumeMounts:   mounts,
			ReadinessProbe: ovnControllerReadinessProbe,
			LivenessProbe:  ovnControllerLivenessProbe,
			// TODO: consider the fact that resources are now double booked
			Resources:                instance.Spec.Resources,
			TerminationMessagePolicy: corev1.TerminationMessageFallbackToLogsOnError,
		},
	}

	daemonset := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ovnv1.ServiceNameOVNController,
			Namespace: instance.Namespace,
		},
		Spec: appsv1.DaemonSetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					ServiceAccountName: instance.RbacResourceName(),
					Containers:         containers,
					Volumes:            volumes,
				},
			},
		},
	}

	if instance.Spec.NodeSelector != nil {
		daemonset.Spec.Template.Spec.NodeSelector = *instance.Spec.NodeSelector
	}
	// DaemonSet automatically place one Pod per node that matches the
	// node selector, but topology spread constraints and PodAffinity/PodAntiaffinity
	// rules are ignored. However, NodeAffinity, part of the Topology interface,
	// might be used to influence Pod scheduling.
	// More details about DaemonSetSpec Pod scheduling in:
	// https://github.com/kubernetes/kubernetes/blob/master/pkg/controller/daemon/daemon_controller.go#L1018
	// https://github.com/kubernetes/kubernetes/blob/master/pkg/controller/daemon/util/daemonset_util.go#L226
	if topology != nil {
		// Get the Topology .Spec
		ts := topology.Spec
		// Process Affinity if defined in the referenced Topology
		if ts.Affinity != nil {
			daemonset.Spec.Template.Spec.Affinity = ts.Affinity
		}
	}

	return daemonset
}

func CreateOVSDaemonSet(
	instance *ovnv1.OVNController,
	configHash string,
	labels map[string]string,
	annotations map[string]string,
	topology *topologyv1.Topology,
) *appsv1.DaemonSet {
	//
	// https://kubernetes.io/docs/tasks/configure-pod-container/configure-liveness-readiness-startup-probes/
	//
	ovsDbLivenessProbe := &corev1.Probe{
		// TODO might need tuning
		TimeoutSeconds:      5,
		PeriodSeconds:       5,
		InitialDelaySeconds: 30,
	}

	ovsVswitchdLivenessProbe := &corev1.Probe{
		// TODO might need tuning
		TimeoutSeconds:      5,
		PeriodSeconds:       5,
		InitialDelaySeconds: 30,
	}

	ovsDbReadinessProbe := &corev1.Probe{
		// TODO might need tuning
		TimeoutSeconds:      5,
		PeriodSeconds:       5,
		InitialDelaySeconds: 30,
	}

	ovsVswitchdReadinessProbe := &corev1.Probe{
		// TODO might need tuning
		TimeoutSeconds:      5,
		PeriodSeconds:       5,
		InitialDelaySeconds: 30,
	}

	ovsDbLivenessProbe.Exec = &corev1.ExecAction{
		Command: []string{
			"/usr/local/bin/container-scripts/ovsdb_server_liveness.sh",
		},
	}
	ovsVswitchdLivenessProbe.Exec = &corev1.ExecAction{
		Command: []string{
			"/usr/local/bin/container-scripts/vswitchd_liveness.sh",
		},
	}

	ovsDbReadinessProbe.Exec = &corev1.ExecAction{
		Command: []string{
			"/usr/local/bin/container-scripts/ovsdb_server_readiness.sh",
		},
	}
	ovsVswitchdReadinessProbe.Exec = &corev1.ExecAction{
		Command: []string{
			"/usr/local/bin/container-scripts/vswitchd_readiness.sh",
		},
	}

	runAsUser := int64(0)
	privileged := true

	envVars := map[string]env.Setter{}
	envVars["CONFIG_HASH"] = env.SetValue(configHash)

	volumes := []corev1.Volume{}
	mounts := []corev1.VolumeMount{}

	// add OVN dbs cert and CA
	if instance.Spec.TLS.Enabled() {
		svc := tls.Service{
			SecretName: *instance.Spec.TLS.GenericService.SecretName,
			CertMount:  ptr.To(ovn_common.OVNDbCertPath),
			KeyMount:   ptr.To(ovn_common.OVNDbKeyPath),
			CaMount:    ptr.To(ovn_common.OVNDbCaCertPath),
		}
		volumes = append(volumes, svc.CreateVolume(ovnv1.ServiceNameOVS))
		mounts = append(mounts, svc.CreateVolumeMounts(ovnv1.ServiceNameOVS)...)

		// add CA bundle if defined
		if instance.Spec.TLS.CaBundleSecretName != "" {
			volumes = append(volumes, instance.Spec.TLS.CreateVolume())
			mounts = append(mounts, instance.Spec.TLS.CreateVolumeMounts(nil)...)
		}
	}

	initContainers := []corev1.Container{
		{
			Name:    "ovsdb-server-init",
			Command: []string{"/usr/local/bin/container-scripts/init-ovsdb-server.sh"},
			Image:   instance.Spec.OvsContainerImage,
			SecurityContext: &corev1.SecurityContext{
				Capabilities: &corev1.Capabilities{
					Add:  []corev1.Capability{"NET_ADMIN", "SYS_ADMIN", "SYS_NICE"},
					Drop: []corev1.Capability{},
				},
				RunAsUser:  &runAsUser,
				Privileged: &privileged,
			},
			Env:          env.MergeEnvs([]corev1.EnvVar{}, envVars),
			VolumeMounts: append(GetOVSDbVolumeMounts(), mounts...),
		},
	}

	containers := []corev1.Container{
		{
			Name:    "ovsdb-server",
			Command: []string{"/usr/bin/dumb-init"},
			Args:    []string{"--single-child", "--", "/usr/local/bin/container-scripts/start-ovsdb-server.sh"},
			Lifecycle: &corev1.Lifecycle{
				PreStop: &corev1.LifecycleHandler{
					Exec: &corev1.ExecAction{
						Command: []string{"/usr/local/bin/container-scripts/stop-ovsdb-server.sh"},
					},
				},
			},
			Image: instance.Spec.OvsContainerImage,
			SecurityContext: &corev1.SecurityContext{
				Capabilities: &corev1.Capabilities{
					Add:  []corev1.Capability{"NET_ADMIN", "SYS_ADMIN", "SYS_NICE"},
					Drop: []corev1.Capability{},
				},
				RunAsUser:  &runAsUser,
				Privileged: &privileged,
			},
			Env:          env.MergeEnvs([]corev1.EnvVar{}, envVars),
			VolumeMounts: append(GetOVSDbVolumeMounts(), mounts...),
			// TODO: consider the fact that resources are now double booked
			Resources:                instance.Spec.Resources,
			LivenessProbe:            ovsDbLivenessProbe,
			ReadinessProbe:           ovsDbReadinessProbe,
			TerminationMessagePolicy: corev1.TerminationMessageFallbackToLogsOnError,
		},
		{
			Name:    "ovs-vswitchd",
			Command: []string{"/usr/local/bin/container-scripts/start-vswitchd.sh"},
			Lifecycle: &corev1.Lifecycle{
				PreStop: &corev1.LifecycleHandler{
					Exec: &corev1.ExecAction{
						Command: []string{"/usr/local/bin/container-scripts/stop-vswitchd.sh"},
					},
				},
			},
			Image: instance.Spec.OvsContainerImage,
			SecurityContext: &corev1.SecurityContext{
				Capabilities: &corev1.Capabilities{
					Add:  []corev1.Capability{"NET_ADMIN", "SYS_ADMIN", "SYS_NICE"},
					Drop: []corev1.Capability{},
				},
				RunAsUser:  &runAsUser,
				Privileged: &privileged,
			},
			Env:          env.MergeEnvs([]corev1.EnvVar{}, envVars),
			VolumeMounts: append(GetVswitchdVolumeMounts(), mounts...),
			// TODO: consider the fact that resources are now double booked
			Resources:                instance.Spec.Resources,
			LivenessProbe:            ovsVswitchdLivenessProbe,
			ReadinessProbe:           ovsVswitchdReadinessProbe,
			TerminationMessagePolicy: corev1.TerminationMessageFallbackToLogsOnError,
		},
	}

	maxUnavailable := intstr.FromInt32(0)
	maxSurge := intstr.FromInt32(1)

	daemonset := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ovnv1.ServiceNameOVS,
			Namespace: instance.Namespace,
		},
		Spec: appsv1.DaemonSetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					ServiceAccountName: instance.RbacResourceName(),
					HostPID:            true,
					InitContainers:     initContainers,
					Containers:         containers,
					Volumes:            append(GetOVSVolumes(instance.Name, instance.Namespace), volumes...),
				},
			},
			UpdateStrategy: appsv1.DaemonSetUpdateStrategy{
				Type: appsv1.RollingUpdateDaemonSetStrategyType,
				RollingUpdate: &appsv1.RollingUpdateDaemonSet{
					MaxUnavailable: &maxUnavailable,
					MaxSurge:       &maxSurge,
				},
			},
		},
	}

	if instance.Spec.NodeSelector != nil {
		daemonset.Spec.Template.Spec.NodeSelector = *instance.Spec.NodeSelector
	}
	// DaemonSet automatically place one Pod per node that matches the
	// node selector, but topology spread constraints and PodAffinity/PodAntiaffinity
	// rules are ignored. However, NodeAffinity, part of the Topology interface,
	// might be used to influence Pod scheduling.
	// More details about DaemonSetSpec Pod scheduling in:
	// https://github.com/kubernetes/kubernetes/blob/master/pkg/controller/daemon/daemon_controller.go#L1018
	// https://github.com/kubernetes/kubernetes/blob/master/pkg/controller/daemon/util/daemonset_util.go#L226
	if topology != nil {
		topology.ApplyTo(&daemonset.Spec.Template)
	}

	if len(annotations) > 0 {
		daemonset.Spec.Template.ObjectMeta.Annotations = annotations
	}

	return daemonset
}
