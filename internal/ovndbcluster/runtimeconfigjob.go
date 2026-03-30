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

// Package ovndbcluster provides functionality for managing OVN database cluster components
package ovndbcluster

import (
	"context"
	"fmt"
	"strings"

	"github.com/openstack-k8s-operators/lib-common/modules/common/env"
	ovnv1 "github.com/openstack-k8s-operators/ovn-operator/api/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// RuntimeConfigJobs - create single job per pod that handles all runtime configuration changes
func RuntimeConfigJobs(
	ctx context.Context,
	k8sClient client.Client,
	instance *ovnv1.OVNDBCluster,
	labels map[string]string,
	serviceName string,
	changedConfigs map[string]interface{}, // Map of changed configuration parameters
) ([]*batchv1.Job, error) {

	var jobs []*batchv1.Job
	// NOTE: set TTLSecondsAfterFinished=0 will clean done
	// configuration job automatically right after it will be finished
	jobTTLAfterFinished := int32(0)

	// Get all pods in the StatefulSet
	ovnPods, err := getOVNDBClusterPods(
		ctx,
		k8sClient,
		instance,
		serviceName,
	)
	if err != nil {
		return nil, err
	}

	// Create a single consolidated job per pod
	jobs = createConsolidatedConfigJobs(instance, ovnPods.Items, labels, jobTTLAfterFinished, changedConfigs)

	return jobs, nil
}

// createConsolidatedConfigJobs - create single job per pod that handles all runtime config changes
func createConsolidatedConfigJobs(instance *ovnv1.OVNDBCluster, pods []corev1.Pod, labels map[string]string, jobTTL int32, changedConfigs map[string]interface{}) []*batchv1.Job {
	var jobs []*batchv1.Job

	for _, pod := range pods {
		// More inclusive pod status check - skip only clearly unavailable pods
		if pod.Status.Phase == corev1.PodFailed || pod.Status.Phase == corev1.PodSucceeded || pod.DeletionTimestamp != nil {
			continue
		}

		// Create list of configuration changes to process
		var configFlags []string
		if _, exists := changedConfigs["ElectionTimer"]; exists {
			configFlags = append(configFlags, "ELECTION_TIMER")
		}
		if _, exists := changedConfigs["LogLevel"]; exists {
			configFlags = append(configFlags, "LOG_LEVEL")
		}
		if _, exists := changedConfigs["InactivityProbe"]; exists {
			configFlags = append(configFlags, "INACTIVITY_PROBE")
		}

		// Skip if no changes detected
		if len(configFlags) == 0 {
			continue
		}

		// Create environment variables for the script (minimal set)
		envVars := map[string]env.Setter{
			"ELECTION_TIMER":   env.SetValue(fmt.Sprintf("%d", instance.Spec.ElectionTimer)),
			"LOG_LEVEL":        env.SetValue(instance.Spec.LogLevel),
			"INACTIVITY_PROBE": env.SetValue(fmt.Sprintf("%d", instance.Spec.InactivityProbe)),
			"CONFIG_FLAGS":     env.SetValue(strings.Join(configFlags, " ")),
		}

		// Use the runtime-config script from ConfigMap volume
		commands := []string{
			"/bin/bash",
			"/etc/config/runtime-config.sh",
		}

		// Use timestamp to ensure unique job names and avoid conflicts
		jobName := fmt.Sprintf("%s-config-%d", pod.Name, metav1.Now().Unix())

		job := &batchv1.Job{
			ObjectMeta: metav1.ObjectMeta{
				Name:      jobName,
				Namespace: instance.Namespace,
				Labels:    labels,
			},
			Spec: batchv1.JobSpec{
				TTLSecondsAfterFinished: &jobTTL,
				Template: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						RestartPolicy:      corev1.RestartPolicyOnFailure,
						ServiceAccountName: instance.RbacResourceName(),
						Containers: []corev1.Container{
							{
								Name:    "ovn-config",
								Image:   instance.Spec.ContainerImage,
								Command: commands,
								Env:     env.MergeEnvs([]corev1.EnvVar{}, envVars),
								VolumeMounts: []corev1.VolumeMount{
									{
										Name:      "ovn-db-data",
										MountPath: "/etc/ovn",
									},
									{
										Name:      "config-scripts",
										MountPath: "/etc/config",
										ReadOnly:  true,
									},
								},
							},
						},
						Volumes: []corev1.Volume{
							{
								Name: "ovn-db-data",
								VolumeSource: corev1.VolumeSource{
									PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
										ClaimName: fmt.Sprintf("%s%s-%s", instance.Name, PVCSuffixEtcOVN, pod.Name),
									},
								},
							},
							{
								Name: "config-scripts",
								VolumeSource: corev1.VolumeSource{
									ConfigMap: &corev1.ConfigMapVolumeSource{
										LocalObjectReference: corev1.LocalObjectReference{
											Name: fmt.Sprintf("%s-runtime-config", instance.Name),
										},
										DefaultMode: func() *int32 { mode := int32(0755); return &mode }(),
									},
								},
							},
						},
						NodeName: pod.Spec.NodeName,
					},
				},
			},
		}
		jobs = append(jobs, job)
	}

	return jobs
}

// getOVNDBClusterPods returns the pods belonging to the OVNDBCluster StatefulSet
func getOVNDBClusterPods(
	ctx context.Context,
	k8sClient client.Client,
	instance *ovnv1.OVNDBCluster,
	serviceName string,
) (*corev1.PodList, error) {
	ovnPods := &corev1.PodList{}

	listOpts := []client.ListOption{
		client.InNamespace(instance.Namespace),
		client.MatchingLabels{
			"service": serviceName,
		},
	}

	err := k8sClient.List(ctx, ovnPods, listOpts...)
	if err != nil {
		return nil, err
	}

	return ovnPods, nil
}
