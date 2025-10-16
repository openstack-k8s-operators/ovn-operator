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
	"github.com/openstack-k8s-operators/lib-common/modules/common/util"
	ovnv1 "github.com/openstack-k8s-operators/ovn-operator/api/v1beta1"
	ovn_common "github.com/openstack-k8s-operators/ovn-operator/pkg/common"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

// CreateOVNDaemonSet creates a DaemonSet for OVN controller pods
func CreateOVNDaemonSet(
	instance *ovnv1.OVNController,
	configHash string,
	labels map[string]string,
	topology *topologyv1.Topology,
) *appsv1.DaemonSet {
	volumes := GetOVNControllerVolumes(instance.Name, instance.Namespace, false)
	mounts := GetOVNControllerVolumeMounts(false)

	cmd := []string{
		"ovn-controller", "--pidfile", "unix:/run/openvswitch/db.sock",
	}

	// add OVN dbs cert and CA
	if instance.Spec.TLS.Enabled() {
		svc := tls.Service{
			SecretName: *instance.Spec.TLS.SecretName,
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

// CreateOVSDaemonSet creates a DaemonSet for OVS (Open vSwitch) pods
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
			VolumeMounts: GetOVSDbVolumeMounts(),
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
			VolumeMounts: GetOVSDbVolumeMounts(),
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
			VolumeMounts: GetVswitchdVolumeMounts(),
			// TODO: consider the fact that resources are now double booked
			Resources:                instance.Spec.Resources,
			LivenessProbe:            ovsVswitchdLivenessProbe,
			ReadinessProbe:           ovsVswitchdReadinessProbe,
			TerminationMessagePolicy: corev1.TerminationMessageFallbackToLogsOnError,
		},
	}

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
					InitContainers:     initContainers,
					Containers:         containers,
					Volumes:            GetOVSVolumes(instance.Name, instance.Namespace),
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
		daemonset.Spec.Template.Annotations = annotations
	}

	return daemonset
}

// CreateMetricsDaemonSet creates a DaemonSet for metrics pods
func CreateMetricsDaemonSet(
	instance *ovnv1.OVNController,
	configHash string,
	labels map[string]string,
	topology *topologyv1.Topology,
) *appsv1.DaemonSet {
	// Create metrics configuration volume and mount
	volumes := []corev1.Volume{
		{
			Name: "ovs-rundir",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: fmt.Sprintf("/var/home/core/%s/var/run/openvswitch", instance.Namespace),
					Type: &[]corev1.HostPathType{corev1.HostPathDirectoryOrCreate}[0],
				},
			},
		},
		{
			Name: "ovn-rundir",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: fmt.Sprintf("/var/home/core/%s/var/run/ovn", instance.Namespace),
					Type: &[]corev1.HostPathType{corev1.HostPathDirectoryOrCreate}[0],
				},
			},
		},
		{
			Name: "config",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: fmt.Sprintf("%s-metrics-config", instance.Name),
					},
				},
			},
		},
	}

	volumeMounts := []corev1.VolumeMount{
		{
			Name:      "ovs-rundir",
			MountPath: "/var/run/openvswitch",
			ReadOnly:  true,
		},
		{
			Name:      "ovn-rundir",
			MountPath: "/var/run/ovn",
			ReadOnly:  true,
		},
		{
			Name:      "config",
			MountPath: "/etc/config",
			ReadOnly:  true,
		},
	}

	// add TLS volume mounts if TLS is enabled - use dedicated metrics cert secret
	if instance.Spec.TLS.Enabled() {
		// cleanup fallback once openstack-operator provides it
		metricsCertSecretName := "cert-ovn-metrics" //nolint:gosec // G101: Not actual credentials, just secret name constants
		if instance.Spec.MetricsTLS.SecretName != nil && *instance.Spec.MetricsTLS.SecretName != "" {
			metricsCertSecretName = *instance.Spec.MetricsTLS.SecretName
		}

		metricsSvc := tls.Service{
			SecretName: metricsCertSecretName,
			CertMount:  ptr.To(ovn_common.OVNMetricsCertPath),
			KeyMount:   ptr.To(ovn_common.OVNMetricsKeyPath),
			CaMount:    ptr.To(ovn_common.OVNDbCaCertPath), // Use the same CA for now
		}
		// Add the metrics certificate volume to the main volumes list
		// Use "metrics-certs" as volume name to stay within 63 char limit
		volumes = append(volumes, metricsSvc.CreateVolume("metrics-certs"))
		volumeMounts = append(volumeMounts, metricsSvc.CreateVolumeMounts("metrics-certs")...)

		// add CA bundle if defined
		if instance.Spec.TLS.CaBundleSecretName != "" {
			volumes = append(volumes, instance.Spec.TLS.CreateVolume())
			volumeMounts = append(volumeMounts, instance.Spec.TLS.CreateVolumeMounts(nil)...)
		}
	}

	envVars := map[string]env.Setter{}
	envVars["CONFIG_HASH"] = env.SetValue(configHash)

	runAsUser := int64(0)
	privileged := true

	containers := []corev1.Container{
		{
			Name:    "openstack-network-exporter",
			Image:   instance.Spec.ExporterImage,
			Command: []string{"/app/openstack-network-exporter"},
			SecurityContext: &corev1.SecurityContext{
				Capabilities: &corev1.Capabilities{
					Add:  []corev1.Capability{"NET_ADMIN", "SYS_ADMIN", "SYS_NICE"},
					Drop: []corev1.Capability{},
				},
				RunAsUser:  &runAsUser,
				Privileged: &privileged,
			},
			Env: env.MergeEnvs([]corev1.EnvVar{
				{
					Name:  "OPENSTACK_NETWORK_EXPORTER_YAML",
					Value: "/etc/config/openstack-network-exporter.yaml",
				},
			}, envVars),
			VolumeMounts:             volumeMounts,
			TerminationMessagePolicy: corev1.TerminationMessageFallbackToLogsOnError,
		},
	}

	daemonset := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ovnv1.ServiceNameOVNControllerMetrics,
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

// GetMetricsConfigMap creates the metrics configuration template
func GetMetricsConfigMap(
	instance *ovnv1.OVNController,
) util.Template {
	templateParameters := make(map[string]any)

	templateParameters["TLS"] = instance.Spec.TLS.Enabled()

	// Add TLS configuration if enabled
	if instance.Spec.TLS.Enabled() {
		templateParameters["OVN_METRICS_CERT_PATH"] = ovn_common.OVNMetricsCertPath
		templateParameters["OVN_METRICS_KEY_PATH"] = ovn_common.OVNMetricsKeyPath
	}

	return util.Template{
		Name:      fmt.Sprintf("%s-metrics-config", instance.Name),
		Namespace: instance.Namespace,
		Type:      util.TemplateTypeNone,
		AdditionalTemplate: map[string]string{
			"openstack-network-exporter.yaml": "/ovncontroller/config/openstack-network-exporter.yaml",
		},
		InstanceType:  instance.Kind,
		ConfigOptions: templateParameters,
	}
}
