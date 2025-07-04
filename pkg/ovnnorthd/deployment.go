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

package ovnnorthd

import (
	"fmt"

	topologyv1 "github.com/openstack-k8s-operators/infra-operator/apis/topology/v1beta1"
	"github.com/openstack-k8s-operators/lib-common/modules/common"
	"github.com/openstack-k8s-operators/lib-common/modules/common/affinity"
	"github.com/openstack-k8s-operators/lib-common/modules/common/env"
	"github.com/openstack-k8s-operators/lib-common/modules/common/tls"
	ovnv1 "github.com/openstack-k8s-operators/ovn-operator/api/v1beta1"
	ovn_common "github.com/openstack-k8s-operators/ovn-operator/pkg/common"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

const (
	// ServiceCommand -
	ServiceCommand = "/usr/bin/ovn-northd"
)

// Deployment func
func Deployment(
	instance *ovnv1.OVNNorthd,
	labels map[string]string,
	nbEndpoint string,
	sbEndpoint string,
	envVars map[string]env.Setter,
	topology *topologyv1.Topology,
) *appsv1.Deployment {

	livenessProbe := &corev1.Probe{
		TimeoutSeconds:      1,
		PeriodSeconds:       5,
		InitialDelaySeconds: 10,
	}
	readinessProbe := &corev1.Probe{
		TimeoutSeconds:      1,
		PeriodSeconds:       5,
		InitialDelaySeconds: 10,
	}
	cmd := []string{ServiceCommand}
	args := []string{
		"-vfile:off",
		fmt.Sprintf("-vconsole:%s", instance.Spec.LogLevel),
		fmt.Sprintf("--n-threads=%d", *instance.Spec.NThreads),
		fmt.Sprintf("--ovnnb-db=%s", nbEndpoint),
		fmt.Sprintf("--ovnsb-db=%s", sbEndpoint),
	}

	// create Volume and VolumeMounts
	volumes := GetNorthdVolumes(instance.Name)
	volumeMounts := GetNorthdVolumeMounts()

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
		volumes = append(volumes, svc.CreateVolume(ovnv1.ServiceNameOVNNorthd))
		volumeMounts = append(volumeMounts, svc.CreateVolumeMounts(ovnv1.ServiceNameOVNNorthd)...)

		args = append(args,
			fmt.Sprintf("--certificate=%s", ovn_common.OVNDbCertPath),
			fmt.Sprintf("--private-key=%s", ovn_common.OVNDbKeyPath),
			fmt.Sprintf("--ca-cert=%s", ovn_common.OVNDbCaCertPath),
		)
	}

	//
	// https://kubernetes.io/docs/tasks/configure-pod-container/configure-liveness-readiness-startup-probes/
	//
	livenessProbe.Exec = &corev1.ExecAction{
		Command: []string{
			"/usr/local/bin/container-scripts/status_check.sh",
		},
	}
	readinessProbe.Exec = livenessProbe.Exec

	// TODO: Make confs customizable
	envVars["OVN_RUNDIR"] = env.SetValue("/tmp")

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ovnv1.ServiceNameOVNNorthd,
			Namespace: instance.Namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Replicas: instance.Spec.Replicas,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					ServiceAccountName: instance.RbacResourceName(),
					Containers: []corev1.Container{
						{
							Name:                     ovnv1.ServiceNameOVNNorthd,
							Command:                  cmd,
							Args:                     args,
							Image:                    instance.Spec.ContainerImage,
							SecurityContext:          getOVNNorthdSecurityContext(),
							Env:                      env.MergeEnvs([]corev1.EnvVar{}, envVars),
							Resources:                instance.Spec.Resources,
							ReadinessProbe:           readinessProbe,
							LivenessProbe:            livenessProbe,
							TerminationMessagePolicy: corev1.TerminationMessageFallbackToLogsOnError,
							VolumeMounts:             volumeMounts,
						},
					},
					Volumes: volumes,
				},
			},
		},
	}
	if instance.Spec.NodeSelector != nil {
		deployment.Spec.Template.Spec.NodeSelector = *instance.Spec.NodeSelector
	}
	if topology != nil {
		topology.ApplyTo(&deployment.Spec.Template)
	} else {
		// If possible two pods of the same service should not
		// run on the same worker node. If this is not possible
		// the get still created on the same worker node.
		deployment.Spec.Template.Spec.Affinity = affinity.DistributePods(
			common.AppSelector,
			[]string{
				ovnv1.ServiceNameOVNNorthd,
			},
			corev1.LabelHostname,
		)
	}

	return deployment
}
