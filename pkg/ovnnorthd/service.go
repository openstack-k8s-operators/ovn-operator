package ovnnorthd

import (
	"fmt"

	"github.com/openstack-k8s-operators/lib-common/modules/common/util"
	ovnv1 "github.com/openstack-k8s-operators/ovn-operator/api/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// MetricsServiceForPod - Service for ovnnorthd metrics endpoint for a specific pod
func MetricsServiceForPod(
	instance *ovnv1.OVNNorthd,
	serviceLabels map[string]string,
	podName string,
) *corev1.Service {
	// Add type label for metrics service and prometheus discovery labels
	svcLabels := util.MergeMaps(serviceLabels, map[string]string{
		"type": "metrics",
		"pod":  podName,
	})

	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-metrics", podName),
			Namespace: instance.Namespace,
			Labels:    svcLabels,
		},
		Spec: corev1.ServiceSpec{
			// No selector - we'll manually create endpoints for this service
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
