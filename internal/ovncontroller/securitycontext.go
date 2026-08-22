package ovncontroller

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"
)

var privilegedOVSCapabilities = []corev1.Capability{
	"NET_ADMIN",
	"SYS_ADMIN",
	"SYS_NICE",
}

func getPrivilegedDaemonSetPodSecurityContext() *corev1.PodSecurityContext {
	return &corev1.PodSecurityContext{
		SeccompProfile: &corev1.SeccompProfile{
			Type: corev1.SeccompProfileTypeRuntimeDefault,
		},
	}
}

func getPrivilegedHostPathSecurityContext(capabilities []corev1.Capability) *corev1.SecurityContext {
	securityContext := &corev1.SecurityContext{
		RunAsUser:                ptr.To(int64(0)),
		Privileged:               ptr.To(true),
		AllowPrivilegeEscalation: ptr.To(true),
		ReadOnlyRootFilesystem:   ptr.To(false),
		SeccompProfile: &corev1.SeccompProfile{
			Type: corev1.SeccompProfileTypeRuntimeDefault,
		},
	}

	if len(capabilities) > 0 {
		securityContext.Capabilities = &corev1.Capabilities{
			Add:  capabilities,
			Drop: []corev1.Capability{},
		}
	}

	return securityContext
}

func getLegacyOVSPodSecurityContext() *corev1.PodSecurityContext {
	return nil
}

func getLegacyOVSHostPathSecurityContext(capabilities []corev1.Capability) *corev1.SecurityContext {
	securityContext := &corev1.SecurityContext{
		RunAsUser:  ptr.To(int64(0)),
		Privileged: ptr.To(true),
	}

	if len(capabilities) > 0 {
		securityContext.Capabilities = &corev1.Capabilities{
			Add:  capabilities,
			Drop: []corev1.Capability{},
		}
	}

	return securityContext
}

func getOVSPodSecurityContext(hardened bool) *corev1.PodSecurityContext {
	if hardened {
		return getPrivilegedDaemonSetPodSecurityContext()
	}
	return getLegacyOVSPodSecurityContext()
}

func getOVSContainerSecurityContext(hardened bool, capabilities []corev1.Capability) *corev1.SecurityContext {
	if hardened {
		return getPrivilegedHostPathSecurityContext(capabilities)
	}
	return getLegacyOVSHostPathSecurityContext(capabilities)
}
