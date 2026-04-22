package ovndbbackup

import (
	ovnv1 "github.com/openstack-k8s-operators/ovn-operator/api/v1beta1"
	ovn_common "github.com/openstack-k8s-operators/ovn-operator/internal/common"
	corev1 "k8s.io/api/core/v1"
)

func tlsVolumesAndMounts(cluster *ovnv1.OVNDBCluster) ([]corev1.Volume, []corev1.VolumeMount) {
	var volumes []corev1.Volume
	var mounts []corev1.VolumeMount

	if cluster.Spec.TLS.SecretName != nil {
		volumes = append(volumes, corev1.Volume{
			Name: "tls-certs",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: *cluster.Spec.TLS.SecretName,
				},
			},
		})
		mounts = append(mounts,
			corev1.VolumeMount{
				Name:      "tls-certs",
				MountPath: ovn_common.OVNDbCertPath,
				SubPath:   "tls.crt",
				ReadOnly:  true,
			},
			corev1.VolumeMount{
				Name:      "tls-certs",
				MountPath: ovn_common.OVNDbKeyPath,
				SubPath:   "tls.key",
				ReadOnly:  true,
			},
		)
	}

	if cluster.Spec.TLS.CaBundleSecretName != "" {
		volumes = append(volumes, corev1.Volume{
			Name: "tls-ca-bundle",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: cluster.Spec.TLS.CaBundleSecretName,
				},
			},
		})
		mounts = append(mounts, corev1.VolumeMount{
			Name:      "tls-ca-bundle",
			MountPath: ovn_common.OVNDbCaCertPath,
			SubPath:   "tls-ca-bundle.pem",
			ReadOnly:  true,
		})
	}

	return volumes, mounts
}
