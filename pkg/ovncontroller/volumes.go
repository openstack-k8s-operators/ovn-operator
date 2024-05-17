package ovncontroller

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
)

func GetOVNControllerVolumes(name string, namespace string) []corev1.Volume {

	var scriptsVolumeDefaultMode int32 = 0755
	directoryOrCreate := corev1.HostPathDirectoryOrCreate

	//source_type := corev1.HostPathDirectoryOrCreate
	return []corev1.Volume{
		{
			Name: "etc-ovs",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: fmt.Sprintf("/var/home/core/%s/etc/ovs", namespace),
					Type: &directoryOrCreate,
				},
			},
		},
		{
			Name: "var-run",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: fmt.Sprintf("/var/home/core/%s/var/run/openvswitch", namespace),
					Type: &directoryOrCreate,
				},
			},
		},
		{
			Name: "var-log",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: fmt.Sprintf("/var/home/core/%s/var/log/openvswitch", namespace),
					Type: &directoryOrCreate,
				},
			},
		},
		{
			Name: "var-lib",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: fmt.Sprintf("/var/home/core/%s/var/lib/openvswitch", namespace),
					Type: &directoryOrCreate,
				},
			},
		},
		{
			Name: "var-run-ovn",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: fmt.Sprintf("/var/home/core/%s/var/run/ovn", namespace),
					Type: &directoryOrCreate,
				},
			},
		},
		{
			Name: "var-log-ovn",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: fmt.Sprintf("/var/home/core/%s/var/log/ovn", namespace),
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

func GetOVSVolumes(name string, namespace string) []corev1.Volume {

	var scriptsVolumeDefaultMode int32 = 0755
	directoryOrCreate := corev1.HostPathDirectoryOrCreate

	//source_type := corev1.HostPathDirectoryOrCreate
	return []corev1.Volume{
		{
			Name: "etc-ovs",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: fmt.Sprintf("/var/home/core/%s/etc/ovs", namespace),
					Type: &directoryOrCreate,
				},
			},
		},
		{
			Name: "var-run",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: fmt.Sprintf("/var/home/core/%s/var/run/openvswitch", namespace),
					Type: &directoryOrCreate,
				},
			},
		},
		{
			Name: "var-log",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: fmt.Sprintf("/var/home/core/%s/var/log/openvswitch", namespace),
					Type: &directoryOrCreate,
				},
			},
		},
		{
			Name: "var-lib",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: fmt.Sprintf("/var/home/core/%s/var/lib/openvswitch", namespace),
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

// GetOVSDbVolumeMounts - ovsdb-server VolumeMounts
func GetOVSDbVolumeMounts() []corev1.VolumeMount {
	return []corev1.VolumeMount{
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

// GetOVNControllerVolumeMounts - ovn-controller VolumeMounts
func GetOVNControllerVolumeMounts() []corev1.VolumeMount {
	return []corev1.VolumeMount{
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
