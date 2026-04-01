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

// Package ovncontroller provides functionality for managing OVN controller components
package ovncontroller

import (
	"context"
	"strings"

	"github.com/openstack-k8s-operators/lib-common/modules/common/env"
	ovnv1 "github.com/openstack-k8s-operators/ovn-operator/api/v1beta1"
	ovn_common "github.com/openstack-k8s-operators/ovn-operator/internal/common"
	ovndbcluster "github.com/openstack-k8s-operators/ovn-operator/internal/ovndbcluster"
	"sigs.k8s.io/controller-runtime/pkg/client"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ConfigJob - prepare job to configure ovn-controller
func ConfigJob(
	ctx context.Context,
	k8sClient client.Client,
	instance *ovnv1.OVNController,
	sbCluster *ovnv1.OVNDBCluster,
	labels map[string]string,
) ([]*batchv1.Job, error) {

	var jobs []*batchv1.Job
	runAsUser := int64(0)
	privileged := true
	// NOTE(slaweq): set TTLSecondsAfterFinished=0 will clean done
	// configuration job automatically right after it will be finished
	jobTTLAfterFinished := int32(0)

	ovnPods, err := getOVNControllerPods(
		ctx,
		k8sClient,
		instance,
	)
	if err != nil {
		return nil, err
	}

	internalEndpoint, err := sbCluster.GetInternalEndpoint()
	if err != nil {
		return nil, err
	}

	envVars := map[string]env.Setter{}
	envVars["OVNBridge"] = env.SetValue(instance.Spec.ExternalIDS.OvnBridge)
	envVars["OVNRemote"] = env.SetValue(internalEndpoint)
	envVars["OVNEncapType"] = env.SetValue(instance.Spec.ExternalIDS.OvnEncapType)
	envVars["OVNEncapTos"] = env.SetValue(instance.Spec.ExternalIDS.OvnEncapTos)
	envVars["OVNAvailabilityZones"] = env.SetValue(strings.Join(instance.Spec.ExternalIDS.OvnAvailabilityZones, ":"))
	envVars["PhysicalNetworks"] = env.SetValue(getPhysicalNetworks(instance))
	envVars["OVNHostName"] = env.DownwardAPI("spec.nodeName")
	envVars["OVNLogLevel"] = env.SetValue(instance.Spec.OVNLogLevel)
	envVars["OVSLogLevel"] = env.SetValue(instance.Spec.OVSLogLevel)

	// Prepare volumes and mounts for config job
	volumes := GetOVNControllerVolumes(instance.Name, instance.Namespace, true)
	volumeMounts := GetOVNControllerVolumeMounts(true)

	// When TLS is enabled, mount the RBAC PKI CA secret so the config job
	// can generate and sign per-node ovn-controller certificates.
	if instance.Spec.TLS.Enabled() {
		volumes = append(volumes, corev1.Volume{
			Name: "ovn-rbac-pki-ca",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: ovndbcluster.OVNRbacPkiCaSecret,
				},
			},
		})
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      "ovn-rbac-pki-ca",
			MountPath: ovn_common.OVNRbacPkiCaMountPath,
			ReadOnly:  true,
		})
		// Also mount etc-ovs to persist the generated certificates
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      "etc-ovs",
			MountPath: OVNControllerCertDir,
			ReadOnly:  false,
		})

		envVars["OVNControllerCertDir"] = env.SetValue(OVNControllerCertDir)
		envVars["OVN_RBAC_CA_CERT"] = env.SetValue(ovn_common.OVNRbacPkiCaCertPath)
		envVars["OVN_RBAC_CA_KEY"] = env.SetValue(ovn_common.OVNRbacPkiCaKeyPath)
	}

	for _, ovnPod := range ovnPods.Items {
		commands := []string{
			"/usr/local/bin/container-scripts/init.sh",
			"&&",
			"/usr/local/bin/additional-scripts/configure-ovn.sh",
		}

		jobs = append(
			jobs,
			&batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{
					Name:      ovnPod.Name + "-config",
					Namespace: instance.Namespace,
					Labels:    labels,
				},
				Spec: batchv1.JobSpec{
					TTLSecondsAfterFinished: &jobTTLAfterFinished,
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							RestartPolicy:      corev1.RestartPolicyOnFailure,
							ServiceAccountName: instance.RbacResourceName(),
							Containers: []corev1.Container{
								{
									Name:  "ovn-config",
									Image: instance.Spec.OvnContainerImage,
									Command: []string{
										"/bin/bash", "-c", strings.Join(commands, " "),
									},
									Args: []string{},
									SecurityContext: &corev1.SecurityContext{
										RunAsUser:  &runAsUser,
										Privileged: &privileged,
									},
									Env:          env.MergeEnvs([]corev1.EnvVar{}, envVars),
									VolumeMounts: volumeMounts,
									Resources:    instance.Spec.Resources,
								},
							},
							Volumes:  volumes,
							NodeName: ovnPod.Spec.NodeName,
							// ^ NodeSelector not required
						},
					},
				},
			},
		)
	}

	return jobs, nil
}
