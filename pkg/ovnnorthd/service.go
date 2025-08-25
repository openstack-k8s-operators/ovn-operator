package ovnnorthd

import (
	ovnv1 "github.com/openstack-k8s-operators/ovn-operator/api/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// MetricsService - Service for ovnnorthd metrics per pod
func MetricsService(
	serviceName string,
	instance *ovnv1.OVNNorthd,
	serviceLabels map[string]string,
	selectorLabels map[string]string,
) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceName,
			Namespace: instance.Namespace,
			Labels:    serviceLabels,
		},
		Spec: corev1.ServiceSpec{
			Selector: selectorLabels,
			Ports: []corev1.ServicePort{
				{
					Name:     "metrics",
					Port:     1981,
					Protocol: corev1.ProtocolTCP,
				},
			},
		},
	}
}
