/*
Copyright 2020 Red Hat

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllers

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	ovncentralv1alpha1 "github.com/openstack-k8s-operators/ovn-central-operator/api/v1alpha1"
	"github.com/openstack-k8s-operators/ovn-central-operator/util"
)

func serviceName(server *ovncentralv1alpha1.OVSDBServer) string {
	return fmt.Sprintf("%s.%s.svc.cluster.local", server.Name, server.Namespace)
}

func serviceShell(server *ovncentralv1alpha1.OVSDBServer) *corev1.Service {
	service := &corev1.Service{}
	service.Name = server.Name
	service.Namespace = server.Namespace

	return service
}

func serviceApply(
	service *corev1.Service,
	server *ovncentralv1alpha1.OVSDBServer) {

	util.InitLabelMap(&service.Labels)

	service.Spec.Selector = make(map[string]string)
	service.Spec.Selector["app"] = OVSDBServerApp
	service.Spec.Selector[OVSDBServerLabel] = server.Name

	makePort := func(name string, port int32) corev1.ServicePort {
		return corev1.ServicePort{
			Name:       name,
			Port:       port,
			Protocol:   corev1.ProtocolTCP,
			TargetPort: intstr.FromInt(int(port)),
		}
	}

	service.Spec.Ports = []corev1.ServicePort{
		makePort("north", 6641),
		makePort("south", 6642),
		makePort("north-raft", 6643),
		makePort("south-raft", 6644),
	}

	service.Spec.Type = corev1.ServiceTypeClusterIP

	// There are 2 reasons we need this.
	//
	// 1. The raft cluster communicates using this service. If we
	//    don't add the pod to the service until it becomes ready,
	//    it can never become ready.
	//
	// 2. A potential client race. A client attempting a
	//    leader-only connection must be able to connect to the
	//    leader at the time. Any delay in making the leader
	//    available for connections could result in incorrect
	//    behaviour.
	service.Spec.PublishNotReadyAddresses = true
}
