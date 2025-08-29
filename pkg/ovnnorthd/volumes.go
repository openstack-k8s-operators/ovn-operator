package ovnnorthd

import corev1 "k8s.io/api/core/v1"

// GetNorthdVolumes - OVN Northd Volumes
func GetNorthdVolumes(name string) []corev1.Volume {
	var scriptsVolumeDefaultMode int32 = 0755

	return []corev1.Volume{
		{
			Name: "scripts",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					DefaultMode: &scriptsVolumeDefaultMode,
					LocalObjectReference: corev1.LocalObjectReference{
						Name: name + "-scripts",
					},
				},
			},
		},
		{
			Name: "ovn-rundir",
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		},
		{
			Name: "config",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					DefaultMode: &scriptsVolumeDefaultMode,
					LocalObjectReference: corev1.LocalObjectReference{
						Name: name + "-config",
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
			Name:      "scripts",
			MountPath: "/usr/local/bin/container-scripts",
			ReadOnly:  true,
		},
		{
			Name:      "ovn-rundir",
			MountPath: "/tmp",
		},
	}

}
