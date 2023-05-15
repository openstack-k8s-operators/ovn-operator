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
	"context"
	"fmt"

	"github.com/openstack-k8s-operators/lib-common/modules/common/env"
	"github.com/openstack-k8s-operators/lib-common/modules/common/helper"
	"github.com/openstack-k8s-operators/ovs-operator/api/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	ovnclient "github.com/openstack-k8s-operators/ovn-operator/api/v1beta1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ConfigJob - prepare job to configure ovn-controller
func ConfigJob(
	ctx context.Context,
	h *helper.Helper,
	k8sClient client.Client,
	instance *v1beta1.OVS,
	labels map[string]string,
) ([]*batchv1.Job, error) {

	var jobs []*batchv1.Job
	runAsUser := int64(0)
	privileged := true
	// NOTE(slaweq): set TTLSecondsAfterFinished=0 will clean done
	// configuration job automatically right after it will be finished
	jobTTLAfterFinished := int32(0)

	dbmap, err := ovnclient.GetDBEndpoints(ctx, h, instance.Namespace, map[string]string{})
	if err != nil {
		return nil, err
	}

	ovsNodes, err := getOvsPodsNodes(
		ctx,
		k8sClient,
		instance,
	)
	if err != nil {
		return nil, err
	}

	envVars := map[string]env.Setter{}
	envVars["KOLLA_CONFIG_FILE"] = env.SetValue(KollaConfigAPI)
	envVars["KOLLA_CONFIG_STRATEGY"] = env.SetValue("COPY_ALWAYS")
	envVars["OvnBridge"] = env.SetValue(instance.Spec.ExternalIDS.OvnBridge)
	envVars["OvnRemote"] = env.SetValue(dbmap["internal-SB"])
	envVars["OvnEncapType"] = env.SetValue(instance.Spec.ExternalIDS.OvnEncapType)
	envVars["PodNamespace"] = env.SetValue(instance.Namespace)
	envVars["PodNetworksStatus"] = EnvDownwardAPI("metadata.annotations['k8s.v1.cni.cncf.io/networks-status']")
	envVars["OvnEncapNetwork"] = env.SetValue(instance.Spec.NetworkAttachment)
	envVars["OvnEncapIP"] = EnvDownwardAPI("status.podIP")
	envVars["EnableChassisAsGateway"] = env.SetValue(fmt.Sprintf("%t", instance.Spec.ExternalIDS.EnableChassisAsGateway))
	envVars["PhysicalNetworks"] = env.SetValue(getPhysicalNetworks(instance))
	envVars["OvnHostName"] = EnvDownwardAPI("spec.nodeName")

	for _, ovsNode := range ovsNodes {
		jobs = append(
			jobs,
			&batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{
					Name:      instance.Name + "-configuration-" + ovsNode,
					Namespace: instance.Namespace,
					Labels:    labels,
				},
				Spec: batchv1.JobSpec{
					TTLSecondsAfterFinished: &jobTTLAfterFinished,
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							RestartPolicy:      "OnFailure",
							ServiceAccountName: instance.RbacResourceName(),
							Containers: []corev1.Container{
								{
									Name:  instance.Name + "-ovn-configuration-sync",
									Image: instance.Spec.OvnContainerImage,
									Command: []string{
										"/usr/local/bin/container-scripts/init.sh",
									},
									Args: []string{},
									SecurityContext: &corev1.SecurityContext{
										RunAsUser:  &runAsUser,
										Privileged: &privileged,
									},
									Env:          env.MergeEnvs([]corev1.EnvVar{}, envVars),
									VolumeMounts: GetOvnVolumeMounts(),
									Resources:    instance.Spec.Resources,
								},
							},
							Volumes:  GetVolumes(instance.Name),
							NodeName: ovsNode,
						},
					},
				},
			},
		)
	}

	return jobs, nil
}
