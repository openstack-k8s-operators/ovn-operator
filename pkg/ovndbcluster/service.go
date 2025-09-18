package ovndbcluster

import (
	ovnv1 "github.com/openstack-k8s-operators/ovn-operator/api/v1beta1"
	"github.com/openstack-k8s-operators/ovn-operator/pkg/common"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Service - Service for ovndbcluster per pod
func Service(
	serviceName string,
	instance *ovnv1.OVNDBCluster,
	serviceLabels map[string]string,
	selectorLabels map[string]string,
) *corev1.Service {
	dbPortName := "north"
	raftPortName := "north-raft"
	dbPort := DbPortNB
	raftPort := RaftPortNB
	if instance.Spec.DBType == ovnv1.SBDBType {
		dbPortName = "south"
		raftPortName = "south-raft"
		dbPort = DbPortSB
		raftPort = RaftPortSB
	}

	ports := []corev1.ServicePort{
		{
			Name:     dbPortName,
			Port:     dbPort,
			Protocol: corev1.ProtocolTCP,
		},
		{
			Name:     raftPortName,
			Port:     raftPort,
			Protocol: corev1.ProtocolTCP,
		},
	}

	// Add metrics port if metrics are enabled and exporter image is specified
	if instance.Spec.ExporterImage != "" && (instance.Spec.MetricsEnabled == nil || *instance.Spec.MetricsEnabled) {
		ports = append(ports, corev1.ServicePort{
			Name:     "metrics",
			Port:     common.MetricsPort,
			Protocol: corev1.ProtocolTCP,
		})
	}

	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceName,
			Namespace: instance.Namespace,
			Labels:    serviceLabels,
		},
		Spec: corev1.ServiceSpec{
			Selector: selectorLabels,
			Ports:    ports,
		},
	}
}

// HeadlessService - Headless Service for ovndbcluster pods to get DNS names in pods
func HeadlessService(
	serviceName string,
	instance *ovnv1.OVNDBCluster,
	serviceLabels map[string]string,
	selectorLabels map[string]string,
) *corev1.Service {
	raftPortName := "north-raft"
	raftPort := RaftPortNB
	if instance.Spec.DBType == ovnv1.SBDBType {
		raftPortName = "south-raft"
		raftPort = RaftPortSB
	}
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
					Name:     raftPortName,
					Port:     raftPort,
					Protocol: corev1.ProtocolTCP,
				},
			},
			ClusterIP: "None",
		},
	}
}
