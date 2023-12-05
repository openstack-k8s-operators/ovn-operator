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
	"github.com/openstack-k8s-operators/lib-common/modules/common"
	"github.com/openstack-k8s-operators/lib-common/modules/common/env"
	"github.com/openstack-k8s-operators/ovn-operator/api/v1beta1"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// DaemonSet func
func DaemonSet(
	instance *v1beta1.OVNController,
	configHash string,
	labels map[string]string,
	annotations map[string]string,
) (*appsv1.DaemonSet, error) {

	runAsUser := int64(0)
	privileged := true

	//
	// https://kubernetes.io/docs/tasks/configure-pod-container/configure-liveness-readiness-startup-probes/
	//
	ovsDbLivenessProbe := &corev1.Probe{
		// TODO might need tuning
		TimeoutSeconds:      5,
		PeriodSeconds:       3,
		InitialDelaySeconds: 3,
	}

	ovsVswitchdLivenessProbe := &corev1.Probe{
		// TODO might need tuning
		TimeoutSeconds:      5,
		PeriodSeconds:       3,
		InitialDelaySeconds: 3,
	}

	noopCmd := []string{
		"/bin/true",
	}

	var ovsDbPreStopCmd []string
	var ovsDbCmd []string
	var ovsDbArgs []string

	var ovsVswitchdCmd []string
	var ovsVswitchdArgs []string
	var ovsVswitchdPreStopCmd []string

	var ovnControllerArgs []string
	var ovnControllerPreStopCmd []string

	if instance.Spec.Debug.Service {
		ovsDbLivenessProbe.Exec = &corev1.ExecAction{
			Command: noopCmd,
		}
		ovsDbCmd = []string{
			common.DebugCommand,
		}
		ovsDbArgs = []string{}
		ovsDbPreStopCmd = noopCmd
		ovsVswitchdLivenessProbe.Exec = &corev1.ExecAction{
			Command: noopCmd,
		}
		ovsVswitchdCmd = []string{
			common.DebugCommand,
		}
		ovsVswitchdArgs = noopCmd
		ovsVswitchdPreStopCmd = noopCmd

		ovnControllerArgs = []string{
			common.DebugCommand,
		}
		ovnControllerPreStopCmd = noopCmd
	} else {
		ovsDbLivenessProbe.Exec = &corev1.ExecAction{
			Command: []string{
				"/usr/bin/ovs-vsctl",
				"show",
			},
		}
		ovsDbCmd = []string{
			"/usr/bin/dumb-init",
		}
		ovsDbArgs = []string{
			"--single-child", "--", "/usr/local/bin/container-scripts/start-ovsdb-server.sh",
		}
		// sleep is required as workaround for https://github.com/kubernetes/kubernetes/issues/39170
		ovsDbPreStopCmd = []string{
			"/usr/share/openvswitch/scripts/ovs-ctl", "stop", "--no-ovs-vswitchd", ";", "sleep", "2",
		}

		ovsVswitchdLivenessProbe.Exec = &corev1.ExecAction{
			Command: []string{
				"/usr/bin/ovs-appctl",
				"bond/show",
			},
		}
		ovsVswitchdCmd = []string{
			"/usr/sbin/ovs-vswitchd",
		}
		ovsVswitchdArgs = []string{
			"--pidfile", "--mlockall",
		}
		// sleep is required as workaround for https://github.com/kubernetes/kubernetes/issues/39170
		ovsVswitchdPreStopCmd = []string{
			"/usr/share/openvswitch/scripts/ovs-ctl", "stop", "--no-ovsdb-server", ";", "sleep", "2",
		}

		ovnControllerArgs = []string{
			"/usr/local/bin/container-scripts/net_setup.sh && ovn-controller --pidfile unix:/run/openvswitch/db.sock",
		}
		// sleep is required as workaround for https://github.com/kubernetes/kubernetes/issues/39170
		ovnControllerPreStopCmd = []string{
			"/usr/share/ovn/scripts/ovn-ctl", "stop_controller", ";", "sleep", "2",
		}
	}

	envVars := map[string]env.Setter{}
	envVars["CONFIG_HASH"] = env.SetValue(configHash)

	daemonset := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ServiceName,
			Namespace: instance.Namespace,
		},
		Spec: appsv1.DaemonSetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels:      labels,
					Annotations: annotations,
				},
				Spec: corev1.PodSpec{
					ServiceAccountName: instance.RbacResourceName(),
					Containers: []corev1.Container{
						// ovsdb-server container
						{
							Name:    "ovsdb-server",
							Command: ovsDbCmd,
							Args:    ovsDbArgs,
							Lifecycle: &corev1.Lifecycle{
								PreStop: &corev1.LifecycleHandler{
									Exec: &corev1.ExecAction{
										Command: ovsDbPreStopCmd,
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
							Env:                      env.MergeEnvs([]corev1.EnvVar{}, envVars),
							VolumeMounts:             GetOvsDbVolumeMounts(),
							Resources:                instance.Spec.Resources,
							LivenessProbe:            ovsDbLivenessProbe,
							TerminationMessagePolicy: corev1.TerminationMessageFallbackToLogsOnError,
						}, {
							// ovs-vswitchd container
							Name:    "ovs-vswitchd",
							Command: ovsVswitchdCmd,
							Args:    ovsVswitchdArgs,
							Lifecycle: &corev1.Lifecycle{
								PreStop: &corev1.LifecycleHandler{
									Exec: &corev1.ExecAction{
										Command: ovsVswitchdPreStopCmd,
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
							Env:                      env.MergeEnvs([]corev1.EnvVar{}, envVars),
							VolumeMounts:             GetVswitchdVolumeMounts(),
							Resources:                instance.Spec.Resources,
							LivenessProbe:            ovsVswitchdLivenessProbe,
							TerminationMessagePolicy: corev1.TerminationMessageFallbackToLogsOnError,
						}, {
							// ovn-controller container
							// NOTE(slaweq): for some reason, when ovn-controller is started without
							// bash shell, it fails with error "unrecognized option --pidfile"
							Name: "ovn-controller",
							Command: []string{
								"/bin/bash", "-c",
							},
							Args: ovnControllerArgs,
							Lifecycle: &corev1.Lifecycle{
								PreStop: &corev1.LifecycleHandler{
									Exec: &corev1.ExecAction{
										Command: ovnControllerPreStopCmd,
									},
								},
							},
							Image: instance.Spec.OvnContainerImage,
							// TODO(slaweq): to check if ovn-controller really needs such security contexts
							SecurityContext: &corev1.SecurityContext{
								Capabilities: &corev1.Capabilities{
									Add:  []corev1.Capability{"NET_ADMIN", "SYS_ADMIN", "SYS_NICE"},
									Drop: []corev1.Capability{},
								},
								RunAsUser:  &runAsUser,
								Privileged: &privileged,
							},
							Env:                      env.MergeEnvs([]corev1.EnvVar{}, envVars),
							VolumeMounts:             GetOvnControllerVolumeMounts(),
							Resources:                instance.Spec.Resources,
							TerminationMessagePolicy: corev1.TerminationMessageFallbackToLogsOnError,
						},
					},
				},
			},
		},
	}
	daemonset.Spec.Template.Spec.Volumes = GetVolumes(instance.Name, instance.Namespace)

	if instance.Spec.NodeSelector != nil && len(instance.Spec.NodeSelector) > 0 {
		daemonset.Spec.Template.Spec.NodeSelector = instance.Spec.NodeSelector
	}

	return daemonset, nil

}
