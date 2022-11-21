package ovndbcluster

import (
	ovnv1 "github.com/openstack-k8s-operators/ovn-operator/api/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Service - Service for conductor pod services
func Service(
	serviceName string,
	instance *ovnv1.OVNDBCluster,
	serviceLabels map[string]string,
) *corev1.Service {
	dbPortName := "north"
	raftPortName := "north-raft"
	var dbPort int32 = 6641
	var raftPort int32 = 6643
	if instance.Spec.DBType == "SB" {
		dbPortName = "south"
		raftPortName = "south-raft"
		dbPort = 6642
		raftPort = 6644
	}
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceName,
			Namespace: instance.Namespace,
			Labels:    serviceLabels,
		},
		Spec: corev1.ServiceSpec{
			Selector: serviceLabels,
			Ports: []corev1.ServicePort{
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
			},
		},
	}
}
