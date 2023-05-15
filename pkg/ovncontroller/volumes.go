package ovncontroller

import (
	corev1 "k8s.io/api/core/v1"
)

// GetVolumes -
func GetVolumes(name string) []corev1.Volume {

	var scriptsVolumeDefaultMode int32 = 0755
	directoryOrCreate := corev1.HostPathDirectoryOrCreate

	//source_type := corev1.HostPathDirectoryOrCreate
	return []corev1.Volume{
		{
			Name: "etc-machine-id",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: "/etc/machine-id",
				},
			},
		},
		{
			Name: "etc-localtime",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: "/etc/localtime",
				},
			},
		},
		{
			Name: "etc-ovs",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: "/var/home/core/openstack/etc/ovs",
					Type: &directoryOrCreate,
				},
			},
		},
		{
			Name: "var-run",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: "/var/home/core/openstack/var/run/openvswitch",
					Type: &directoryOrCreate,
				},
			},
		},
		{
			Name: "var-log",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: "/var/home/core/openstack/var/log/openvswitch",
					Type: &directoryOrCreate,
				},
			},
		},
		{
			Name: "var-lib",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: "/var/home/core/openstack/var/lib/openvswitch",
					Type: &directoryOrCreate,
				},
			},
		},
		{
			Name: "var-run-ovn",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: "/var/home/core/openstack/var/run/ovn",
					Type: &directoryOrCreate,
				},
			},
		},
		{
			Name: "var-log-ovn",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: "/var/home/core/openstack/var/log/ovn",
					Type: &directoryOrCreate,
				},
			},
		},
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
	}

}

// GetOvsDbVolumeMounts - ovsdb-server VolumeMounts
func GetOvsDbVolumeMounts() []corev1.VolumeMount {
	return []corev1.VolumeMount{
		{
			Name:      "etc-machine-id",
			MountPath: "/etc/machine-id",
			ReadOnly:  true,
		},
		{
			Name:      "etc-localtime",
			MountPath: "/etc/localtime",
			ReadOnly:  true,
		},
		{
			Name:      "etc-ovs",
			MountPath: "/etc/openvswitch",
			ReadOnly:  false,
		},
		{
			Name:      "var-run",
			MountPath: "/var/run/openvswitch",
			ReadOnly:  false,
		},
		{
			Name:      "var-log",
			MountPath: "/var/log/openvswitch",
			ReadOnly:  false,
		},
		{
			Name:      "var-lib",
			MountPath: "/var/lib/openvswitch",
			ReadOnly:  false,
		},
		{
			Name:      "scripts",
			MountPath: "/usr/local/bin/container-scripts",
			ReadOnly:  true,
		},
	}
}

// GetVswitchdVolumeMounts - ovs-vswitchd VolumeMounts
func GetVswitchdVolumeMounts() []corev1.VolumeMount {
	return []corev1.VolumeMount{
		{
			Name:      "etc-machine-id",
			MountPath: "/etc/machine-id",
			ReadOnly:  true,
		},
		{
			Name:      "etc-localtime",
			MountPath: "/etc/localtime",
			ReadOnly:  true,
		},
		{
			Name:      "var-run",
			MountPath: "/var/run/openvswitch",
			ReadOnly:  false,
		},
		{
			Name:      "var-log",
			MountPath: "/var/log/openvswitch",
			ReadOnly:  false,
		},
		{
			Name:      "var-lib",
			MountPath: "/var/lib/openvswitch",
			ReadOnly:  false,
		},
	}
}

// GetOvnVolumeMounts - ovn-controller VolumeMounts
func GetOvnVolumeMounts() []corev1.VolumeMount {
	return []corev1.VolumeMount{
		{
			Name:      "etc-machine-id",
			MountPath: "/etc/machine-id",
			ReadOnly:  true,
		},
		{
			Name:      "etc-localtime",
			MountPath: "/etc/localtime",
			ReadOnly:  true,
		},
		{
			Name:      "var-run",
			MountPath: "/var/run/openvswitch",
			ReadOnly:  false,
		},
		{
			Name:      "var-run-ovn",
			MountPath: "/var/run/ovn",
			ReadOnly:  false,
		},
		{
			Name:      "var-log-ovn",
			MountPath: "/var/log/ovn",
			ReadOnly:  false,
		},
		{
			Name:      "scripts",
			MountPath: "/usr/local/bin/container-scripts",
			ReadOnly:  true,
		},
	}
}
