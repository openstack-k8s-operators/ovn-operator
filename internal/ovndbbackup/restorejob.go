package ovndbbackup

import (
	"fmt"
	"strings"

	"github.com/openstack-k8s-operators/lib-common/modules/common/env"
	ovnv1 "github.com/openstack-k8s-operators/ovn-operator/api/v1beta1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// RestoreJob returns a Job that restores an OVN DB from a backup.
// It mounts pod-0's PVC and the backup PVC, copies the standalone backup file,
// then lets ovn-ctl convert it to a RAFT cluster on pod startup.
func RestoreJob(
	restore *ovnv1.OVNDBRestore,
	backup *ovnv1.OVNDBBackup,
	cluster *ovnv1.OVNDBCluster,
	labels map[string]string,
) *batchv1.Job {
	var scriptsVolumeDefaultMode int32 = 0755
	var backoffLimit int32 = 2

	pod0PVCName := ClusterPod0PVCName(cluster)

	envVars := map[string]env.Setter{
		"DB_TYPE": env.SetValue(strings.ToLower(cluster.Spec.DBType)),
	}
	if restore.Spec.BackupTimestamp != "" {
		envVars["BACKUP_TIMESTAMP"] = env.SetValue(restore.Spec.BackupTimestamp)
	}

	return &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      RestoreJobName(restore),
			Namespace: restore.Namespace,
			Labels:    labels,
		},
		Spec: batchv1.JobSpec{
			BackoffLimit: &backoffLimit,
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					RestartPolicy:      corev1.RestartPolicyOnFailure,
					ServiceAccountName: restore.RbacResourceName(),
					Containers: []corev1.Container{
						{
							Name:    "ovndb-restore",
							Image:   cluster.Spec.ContainerImage,
							Command: []string{"/bin/bash", BackupScriptsMountPath + "/restore_ovndb"},
							Env:     env.MergeEnvs([]corev1.EnvVar{}, envVars),
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      DBDataVolumeName,
									MountPath: DBDataMountPath,
								},
								{
									Name:      BackupVolumeName,
									MountPath: BackupDataMountPath,
									ReadOnly:  true,
								},
								{
									Name:      BackupScriptsVolumeName,
									MountPath: BackupScriptsMountPath,
									ReadOnly:  true,
								},
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: DBDataVolumeName,
							VolumeSource: corev1.VolumeSource{
								PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: pod0PVCName,
								},
							},
						},
						{
							Name: BackupVolumeName,
							VolumeSource: corev1.VolumeSource{
								PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: BackupPVCName(backup),
									ReadOnly:  true,
								},
							},
						},
						{
							Name: BackupScriptsVolumeName,
							VolumeSource: corev1.VolumeSource{
								ConfigMap: &corev1.ConfigMapVolumeSource{
									DefaultMode: &scriptsVolumeDefaultMode,
									LocalObjectReference: corev1.LocalObjectReference{
										Name: fmt.Sprintf("%s-restore-scripts", restore.Name),
									},
								},
							},
						},
					},
				},
			},
		},
	}
}
