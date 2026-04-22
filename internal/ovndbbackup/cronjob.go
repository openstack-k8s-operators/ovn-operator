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

// BackupCronJob returns the CronJob definition for scheduled backups
func BackupCronJob(
	backup *ovnv1.OVNDBBackup,
	cluster *ovnv1.OVNDBCluster,
	labels map[string]string,
	configHash string,
) *batchv1.CronJob {
	var scriptsVolumeDefaultMode int32 = 0755
	concurrencyPolicy := batchv1.ForbidConcurrent

	retentionMinutes := ""
	if backup.Spec.Retention != nil {
		retentionMinutes = fmt.Sprintf("%d", int(backup.Spec.Retention.Minutes()))
	}

	envVars := map[string]env.Setter{
		"DB_TYPE":      env.SetValue(strings.ToLower(cluster.Spec.DBType)),
		"DB_PORT":      env.SetValue(fmt.Sprintf("%d", DBPort(cluster))),
		"SERVICE_NAME": env.SetValue(ServiceName(cluster)),
		"NAMESPACE":    env.SetValue(backup.Namespace),
		"BACKUP_DIR":   env.SetValue(BackupDataMountPath),
		"RETENTION":    env.SetValue(retentionMinutes),
		"CONFIG_HASH":  env.SetValue(configHash),
	}

	volumes := []corev1.Volume{
		{
			Name: BackupVolumeName,
			VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
					ClaimName: BackupPVCName(backup),
				},
			},
		},
		{
			Name: BackupScriptsVolumeName,
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					DefaultMode: &scriptsVolumeDefaultMode,
					LocalObjectReference: corev1.LocalObjectReference{
						Name: BackupScriptsConfigMapName(backup),
					},
				},
			},
		},
	}

	volumeMounts := []corev1.VolumeMount{
		{
			Name:      BackupVolumeName,
			MountPath: BackupDataMountPath,
		},
		{
			Name:      BackupScriptsVolumeName,
			MountPath: BackupScriptsMountPath,
			ReadOnly:  true,
		},
	}

	if cluster.Spec.TLS.Enabled() {
		tlsVolumes, tlsVolumeMounts := tlsVolumesAndMounts(cluster)
		volumes = append(volumes, tlsVolumes...)
		volumeMounts = append(volumeMounts, tlsVolumeMounts...)
	}

	return &batchv1.CronJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      BackupCronJobName(backup),
			Namespace: backup.Namespace,
			Labels:    labels,
		},
		Spec: batchv1.CronJobSpec{
			Schedule:          backup.Spec.Schedule,
			ConcurrencyPolicy: concurrencyPolicy,
			JobTemplate: batchv1.JobTemplateSpec{
				Spec: batchv1.JobSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							RestartPolicy:      corev1.RestartPolicyOnFailure,
							ServiceAccountName: backup.RbacResourceName(),
							Containers: []corev1.Container{
								{
									Name:         "ovndb-backup",
									Image:        cluster.Spec.ContainerImage,
									Command:      []string{"/bin/bash", BackupScriptsMountPath + "/backup_ovndb"},
									Env:          env.MergeEnvs([]corev1.EnvVar{}, envVars),
									VolumeMounts: volumeMounts,
								},
							},
							Volumes: volumes,
						},
					},
				},
			},
		},
	}
}
