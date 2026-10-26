/*
Copyright 2026 Red Hat Inc.

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

package functional_test

import (
	. "github.com/onsi/ginkgo/v2" //revive:disable:dot-imports
	. "github.com/onsi/gomega"    //revive:disable:dot-imports
	"github.com/openstack-k8s-operators/lib-common/modules/common/probes"
	ovnv1 "github.com/openstack-k8s-operators/ovn-operator/api/v1beta1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

var _ = Describe("OVNController probe overrides", func() {
	It("preserves defaults, applies independent overrides, and restores defaults when removed", func() {
		instance := CreateOVNController(namespace, GetDefaultOVNControllerSpec())
		DeferCleanup(th.DeleteInstance, instance)
		name := types.NamespacedName{Name: instance.GetName(), Namespace: namespace}
		dsName := types.NamespacedName{Name: "ovn-controller", Namespace: namespace}
		container := GetDaemonSet(dsName).Spec.Template.Spec.Containers[0]
		Expect(container.LivenessProbe.TimeoutSeconds).To(Equal(int32(5)))
		Expect(container.LivenessProbe.PeriodSeconds).To(Equal(int32(5)))
		Expect(container.LivenessProbe.InitialDelaySeconds).To(Equal(int32(30)))
		Expect(container.LivenessProbe.FailureThreshold).To(Equal(int32(3)))
		Expect(container.ReadinessProbe.TimeoutSeconds).To(Equal(int32(5)))
		Expect(container.StartupProbe).To(BeNil())

		Eventually(func(g Gomega) {
			cr := GetOVNController(name)
			cr.Spec.Override.Probes = probes.OverrideSpec{
				LivenessProbes:  &probes.ProbeConf{TimeoutSeconds: 30, FailureThreshold: 6},
				ReadinessProbes: &probes.ProbeConf{Type: probes.ProbeHandlerHTTP, Port: 8080, Path: "/ready"},
				StartupProbes:   &probes.ProbeConf{FailureThreshold: 60},
			}
			g.Expect(k8sClient.Update(ctx, cr)).To(Succeed())
		}, timeout, interval).Should(Succeed())
		Eventually(func(g Gomega) {
			c := GetDaemonSet(dsName).Spec.Template.Spec.Containers[0]
			g.Expect(c.LivenessProbe.TimeoutSeconds).To(Equal(int32(30)))
			g.Expect(c.LivenessProbe.FailureThreshold).To(Equal(int32(6)))
			g.Expect(c.LivenessProbe.Exec.Command).To(Equal(container.LivenessProbe.Exec.Command))
			g.Expect(c.ReadinessProbe.HTTPGet.Path).To(Equal("/ready"))
			g.Expect(c.ReadinessProbe.TimeoutSeconds).To(Equal(int32(5)))
			g.Expect(c.StartupProbe).NotTo(BeNil())
			g.Expect(c.StartupProbe.FailureThreshold).To(Equal(int32(60)))
			g.Expect(c.StartupProbe.Exec.Command).To(Equal(container.LivenessProbe.Exec.Command))
		}, timeout, interval).Should(Succeed())
		Eventually(func(g Gomega) {
			cr := GetOVNController(name)
			cr.Spec.Override = ovnv1.OVNControllerOverrideSpec{}
			g.Expect(k8sClient.Update(ctx, cr)).To(Succeed())
		}, timeout, interval).Should(Succeed())
		Eventually(func(g Gomega) {
			c := GetDaemonSet(dsName).Spec.Template.Spec.Containers[0]
			g.Expect(c.LivenessProbe).To(Equal(container.LivenessProbe))
			g.Expect(c.ReadinessProbe).To(Equal(container.ReadinessProbe))
			g.Expect(c.StartupProbe).To(BeNil())
		}, timeout, interval).Should(Succeed())
		// The override applies only to the ovn-controller container.
		ovs := GetDaemonSet(types.NamespacedName{Name: "ovn-controller-ovs", Namespace: namespace})
		for _, c := range ovs.Spec.Template.Spec.Containers {
			Expect(c.LivenessProbe.TimeoutSeconds).To(Equal(int32(5)))
			Expect(c.StartupProbe).To(BeNil())
		}
	})

	DescribeTable("rejects invalid effective configurations on create and update",
		func(conf probes.ProbeConf) {
			cr := &ovnv1.OVNController{
				ObjectMeta: metav1.ObjectMeta{Name: "invalid-probe", Namespace: namespace},
				Spec:       GetDefaultOVNControllerSpec(),
			}
			cr.Spec.Override.Probes.LivenessProbes = &conf
			Expect(apierrors.IsInvalid(k8sClient.Create(ctx, cr))).To(BeTrue())
			cr.Spec.Override = ovnv1.OVNControllerOverrideSpec{}
			Expect(k8sClient.Create(ctx, cr)).To(Succeed())
			DeferCleanup(th.DeleteInstance, cr)
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: cr.Name, Namespace: namespace}, cr)).To(Succeed())
				cr.Spec.Override.Probes.LivenessProbes = &conf
				g.Expect(apierrors.IsInvalid(k8sClient.Update(ctx, cr))).To(BeTrue())
			}, timeout, interval).Should(Succeed())
		},
		Entry("HTTP handler without a port", probes.ProbeConf{Type: probes.ProbeHandlerHTTP}),
		Entry("negative timeout", probes.ProbeConf{TimeoutSeconds: -1}),
		Entry("negative delay", probes.ProbeConf{InitialDelaySeconds: -1}),
	)

	It("allows an explicit Exec type to inherit the default command", func() {
		spec := GetDefaultOVNControllerSpec()
		spec.Override.Probes.ReadinessProbes = &probes.ProbeConf{Type: probes.ProbeHandlerExec, TimeoutSeconds: 15}
		instance := CreateOVNController(namespace, spec)
		DeferCleanup(th.DeleteInstance, instance)
		c := GetDaemonSet(types.NamespacedName{Name: "ovn-controller", Namespace: namespace}).Spec.Template.Spec.Containers[0]
		Expect(c.ReadinessProbe.Exec).NotTo(BeNil())
		Expect(c.ReadinessProbe.TimeoutSeconds).To(Equal(int32(15)))
		Expect(c.ReadinessProbe.Exec.Command).To(ContainElement("/usr/local/bin/container-scripts/ovn_controller_readiness.sh"))
	})
})
