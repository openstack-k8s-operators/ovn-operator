package ovndbcluster

import (
	labels "github.com/openstack-k8s-operators/lib-common/modules/common/labels"
	ovnv1 "github.com/openstack-k8s-operators/ovn-operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// Service func
func Service(instance *ovnv1.OVNDBCluster, scheme *runtime.Scheme) *corev1.Service {

	serviceName := ServiceNameNB
	dbPortName := "north"
	raftPortName := "north-raft"
	var dbPort int32 = 6641
	var raftPort int32 = 6643
	if instance.Spec.DBType == "SB" {
		serviceName = ServiceNameSB
		dbPortName = "south"
		raftPortName = "south-raft"
		dbPort = 6642
		raftPort = 6644
	}
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      instance.Name,
			Namespace: instance.Namespace,
			Labels:    labels.GetLabels(instance, labels.GetGroupLabel(serviceName), map[string]string{}),
		},
		Spec: corev1.ServiceSpec{
			// (ykarel) TODO - Add pod based slector
			Selector: map[string]string{"service": serviceName},
			Ports: []corev1.ServicePort{
				{Name: dbPortName, Port: dbPort, Protocol: corev1.ProtocolTCP},
				{Name: raftPortName, Port: raftPort, Protocol: corev1.ProtocolTCP},
			},
		},
	}
	return svc
}
