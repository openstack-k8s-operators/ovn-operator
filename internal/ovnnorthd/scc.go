package ovnnorthd

import corev1 "k8s.io/api/core/v1"

func getOVNNorthdSecurityContext() *corev1.SecurityContext {
	falseVal := false
	trueVal := true

	return &corev1.SecurityContext{
		RunAsNonRoot:             &trueVal,
		AllowPrivilegeEscalation: &falseVal,
		Capabilities: &corev1.Capabilities{
			Drop: []corev1.Capability{
				"ALL",
			},
		},
	}
}
