package ovnnorthd

import corev1 "k8s.io/api/core/v1"

// GetNorthdVolumes -
// TODO: merge to GetVolumes when other controllers also switched to current config
// mechanism.
func GetNorthdVolumes(name string) []corev1.Volume {
	var config0640AccessMode int32 = 0640

	return []corev1.Volume{
		{
			Name: "config-data",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					DefaultMode: &config0640AccessMode,
					LocalObjectReference: corev1.LocalObjectReference{
						Name: name + "-config-data",
					},
				},
			},
		},
	}

}

// GetNorthdVolumeMounts - OVN Northd VolumeMounts
func GetNorthdVolumeMounts() []corev1.VolumeMount {
	return []corev1.VolumeMount{
		{
			Name:      "config-data",
			MountPath: "/var/lib/config-data",
			ReadOnly:  false,
		},
	}

}
