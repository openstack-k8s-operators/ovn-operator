/*
Copyright 2022.

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
	"fmt"

	. "github.com/onsi/ginkgo/v2" //revive:disable:dot-imports
	. "github.com/onsi/gomega"    //revive:disable:dot-imports

	//revive:disable-next-line:dot-imports
	topologyv1 "github.com/openstack-k8s-operators/infra-operator/apis/topology/v1beta1"
	. "github.com/openstack-k8s-operators/lib-common/modules/common/test/helpers"

	condition "github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

var _ = Describe("OVNNorthd controller", func() {

	When("A OVNNorthd instance is created", func() {
		var ovnNorthdName types.NamespacedName
		BeforeEach(func() {
			ovnNorthdName = ovn.CreateOVNNorthd(nil, namespace, GetDefaultOVNNorthdSpec())
			DeferCleanup(ovn.DeleteOVNNorthd, ovnNorthdName)
		})

		It("should have the Spec fields initialized", func() {
			OVNNorthd := ovn.GetOVNNorthd(ovnNorthdName)
			Expect(*(OVNNorthd.Spec.Replicas)).Should(Equal(int32(1)))
		})

		It("should have the Status fields initialized", func() {
			OVNNorthd := ovn.GetOVNNorthd(ovnNorthdName)
			Expect(OVNNorthd.Status.ReadyCount).To(Equal(int32(0)))
		})

		It("should have a finalizer", func() {
			// the reconciler loop adds the finalizer so we have to wait for
			// it to run
			Eventually(func() []string {
				return ovn.GetOVNNorthd(ovnNorthdName).Finalizers
			}, timeout, interval).Should(ContainElement("openstack.org/ovnnorthd"))
		})

		It("should be in input ready condition", func() {
			th.ExpectCondition(
				ovnNorthdName,
				ConditionGetterFunc(OVNNorthdConditionGetter),
				condition.InputReadyCondition,
				corev1.ConditionTrue,
			)
		})

		When("OVNDBCluster instances are available", func() {
			It("should create a Deployment with the ovn connection CLI args set based on the OVNDBCluster", func() {
				OVNNorthd := ovn.GetOVNNorthd(ovnNorthdName)
				dbs := CreateOVNDBClusters(namespace, map[string][]string{}, 1)
				DeferCleanup(DeleteOVNDBClusters, dbs)

				deplName := types.NamespacedName{
					Namespace: namespace,
					Name:      "ovn-northd",
				}

				depl := th.GetDeployment(deplName)
				Expect(depl.Spec.Template.Spec.Containers[0].Args).To(Equal([]string{
					"-vfile:off",
					"-vconsole:info",
					fmt.Sprintf("--n-threads=%d", *OVNNorthd.Spec.NThreads),
					"--ovnnb-db=tcp:ovsdbserver-nb-0." + namespace + ".svc.cluster.local:6641",
					"--ovnsb-db=tcp:ovsdbserver-sb-0." + namespace + ".svc.cluster.local:6642",
				}))
			})
		})

	})

	When("A OVNNorthd instance is created with 0 replicas", func() {
		var ovnNorthdName types.NamespacedName
		BeforeEach(func() {
			dbs := CreateOVNDBClusters(namespace, map[string][]string{}, 1)
			DeferCleanup(DeleteOVNDBClusters, dbs)
			spec := GetDefaultOVNNorthdSpec()
			replicas := int32(0)
			spec.Replicas = &replicas
			ovnNorthdName = ovn.CreateOVNNorthd(nil, namespace, spec)
			DeferCleanup(ovn.DeleteOVNNorthd, ovnNorthdName)
		})

		It("should create a Deployment with 0 replicas", func() {
			deplName := types.NamespacedName{
				Namespace: namespace,
				Name:      "ovn-northd",
			}

			depl := th.GetDeployment(deplName)
			Expect(*(depl.Spec.Replicas)).Should(Equal(int32(0)))
		})

		It("should not have deploy ready condition", func() {
			Eventually(func(g Gomega) {
				ovnNorthd := GetOVNNorthd(ovnNorthdName)
				g.Expect(ovnNorthd.Status.Conditions.Has(condition.DeploymentReadyCondition)).To(BeFalse())
			}, timeout, interval).Should(Succeed())
		})

		It("should be in ready condition", func() {
			th.ExpectCondition(
				ovnNorthdName,
				ConditionGetterFunc(OVNNorthdConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionTrue,
			)
		})
	})

	When("A OVNNorthd Deployment rollout is progressing", func() {
		var ovnNorthdName types.NamespacedName
		var deploymentName types.NamespacedName
		BeforeEach(func() {
			dbs := CreateOVNDBClusters(namespace, map[string][]string{}, 1)
			DeferCleanup(DeleteOVNDBClusters, dbs)
			spec := GetDefaultOVNNorthdSpec()
			ovnNorthdName = ovn.CreateOVNNorthd(nil, namespace, spec)
			DeferCleanup(ovn.DeleteOVNNorthd, ovnNorthdName)
			deploymentName = types.NamespacedName{
				Namespace: namespace,
				Name:      "ovn-northd",
			}
			th.SimulateDeploymentProgressing(deploymentName)
		})

		It("shows the deployment progressing in DeploymentReadyCondition", func() {
			th.ExpectConditionWithDetails(
				ovnNorthdName,
				ConditionGetterFunc(OVNNorthdConditionGetter),
				condition.DeploymentReadyCondition,
				corev1.ConditionFalse,
				condition.RequestedReason,
				condition.DeploymentReadyRunningMessage,
			)
			th.ExpectCondition(
				ovnNorthdName,
				ConditionGetterFunc(OVNNorthdConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionFalse,
			)
		})

		It("still shows the deployment progressing in DeploymentReadyCondition when rollout hits ProgressDeadlineExceeded", func() {
			th.SimulateDeploymentProgressDeadlineExceeded(deploymentName)
			th.ExpectConditionWithDetails(
				ovnNorthdName,
				ConditionGetterFunc(OVNNorthdConditionGetter),
				condition.DeploymentReadyCondition,
				corev1.ConditionFalse,
				condition.RequestedReason,
				condition.DeploymentReadyRunningMessage,
			)
			th.ExpectCondition(
				ovnNorthdName,
				ConditionGetterFunc(OVNNorthdConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionFalse,
			)
		})

		It("reaches Ready when deployment rollout finished", func() {
			th.ExpectConditionWithDetails(
				ovnNorthdName,
				ConditionGetterFunc(OVNNorthdConditionGetter),
				condition.DeploymentReadyCondition,
				corev1.ConditionFalse,
				condition.RequestedReason,
				condition.DeploymentReadyRunningMessage,
			)
			th.ExpectCondition(
				ovnNorthdName,
				ConditionGetterFunc(OVNNorthdConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionFalse,
			)

			th.SimulateDeploymentReplicaReady(deploymentName)
			th.ExpectCondition(
				ovnNorthdName,
				ConditionGetterFunc(OVNNorthdConditionGetter),
				condition.DeploymentReadyCondition,
				corev1.ConditionTrue,
			)

			th.ExpectCondition(
				ovnNorthdName,
				ConditionGetterFunc(OVNNorthdConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionTrue,
			)
		})
	})

	When("OVNNorthd is created with nodeSelector", func() {
		var ovnNorthdName types.NamespacedName
		var deploymentName types.NamespacedName

		BeforeEach(func() {
			dbs := CreateOVNDBClusters(namespace, map[string][]string{}, 1)
			DeferCleanup(DeleteOVNDBClusters, dbs)
			spec := GetDefaultOVNNorthdSpec()
			nodeSelector := map[string]string{
				"foo": "bar",
			}
			spec.NodeSelector = &nodeSelector
			ovnNorthdName = ovn.CreateOVNNorthd(nil, namespace, spec)
			DeferCleanup(ovn.DeleteOVNNorthd, ovnNorthdName)
			deploymentName = types.NamespacedName{
				Namespace: namespace,
				Name:      "ovn-northd",
			}
			th.SimulateDeploymentReplicaReady(deploymentName)
		})

		It("sets nodeSelector in resource specs", func() {
			Eventually(func(g Gomega) {
				g.Expect(th.GetDeployment(deploymentName).Spec.Template.Spec.NodeSelector).To(Equal(map[string]string{"foo": "bar"}))
			}, timeout, interval).Should(Succeed())
		})

		It("updates nodeSelector in resource specs when changed", func() {
			Eventually(func(g Gomega) {
				g.Expect(th.GetDeployment(deploymentName).Spec.Template.Spec.NodeSelector).To(Equal(map[string]string{"foo": "bar"}))
			}, timeout, interval).Should(Succeed())

			Eventually(func(g Gomega) {
				northd := ovn.GetOVNNorthd(ovnNorthdName)
				newNodeSelector := map[string]string{
					"foo2": "bar2",
				}
				northd.Spec.NodeSelector = &newNodeSelector
				g.Expect(k8sClient.Update(ctx, northd)).Should(Succeed())
			}, timeout, interval).Should(Succeed())

			Eventually(func(g Gomega) {
				g.Expect(th.GetDeployment(deploymentName).Spec.Template.Spec.NodeSelector).To(Equal(map[string]string{"foo2": "bar2"}))
			}, timeout, interval).Should(Succeed())
		})

		It("removes nodeSelector from resource specs when cleared", func() {
			Eventually(func(g Gomega) {
				g.Expect(th.GetDeployment(deploymentName).Spec.Template.Spec.NodeSelector).To(Equal(map[string]string{"foo": "bar"}))
			}, timeout, interval).Should(Succeed())

			Eventually(func(g Gomega) {
				northd := ovn.GetOVNNorthd(ovnNorthdName)
				emptyNodeSelector := map[string]string{}
				northd.Spec.NodeSelector = &emptyNodeSelector
				g.Expect(k8sClient.Update(ctx, northd)).Should(Succeed())
			}, timeout, interval).Should(Succeed())

			Eventually(func(g Gomega) {
				g.Expect(th.GetDeployment(deploymentName).Spec.Template.Spec.NodeSelector).To(BeNil())
			}, timeout, interval).Should(Succeed())
		})

		It("removes nodeSelector from resource specs when nilled", func() {
			Eventually(func(g Gomega) {
				g.Expect(th.GetDeployment(deploymentName).Spec.Template.Spec.NodeSelector).To(Equal(map[string]string{"foo": "bar"}))
			}, timeout, interval).Should(Succeed())

			Eventually(func(g Gomega) {
				northd := ovn.GetOVNNorthd(ovnNorthdName)
				northd.Spec.NodeSelector = nil
				g.Expect(k8sClient.Update(ctx, northd)).Should(Succeed())
			}, timeout, interval).Should(Succeed())

			Eventually(func(g Gomega) {
				g.Expect(th.GetDeployment(deploymentName).Spec.Template.Spec.NodeSelector).To(BeNil())
			}, timeout, interval).Should(Succeed())
		})

		It("should create a ConfigMap for status_check.sh", func() {
			scriptsCM := types.NamespacedName{
				Namespace: ovnNorthdName.Namespace,
				Name:      fmt.Sprintf("%s-%s", ovnNorthdName.Name, "scripts"),
			}
			Eventually(func() corev1.ConfigMap {
				return *th.GetConfigMap(scriptsCM)
			}, timeout, interval).ShouldNot(BeNil())

			Expect(th.GetConfigMap(scriptsCM).ObjectMeta.OwnerReferences[0].Name).To(Equal(ovnNorthdName.Name))
			Expect(th.GetConfigMap(scriptsCM).ObjectMeta.OwnerReferences[0].Kind).To(Equal("OVNNorthd"))
		})

	})

	When("OVNNorthd is created with TLS", func() {
		var ovnNorthdName types.NamespacedName

		BeforeEach(func() {
			dbs := CreateOVNDBClusters(namespace, map[string][]string{}, 1)
			DeferCleanup(DeleteOVNDBClusters, dbs)
			spec := GetTLSOVNNorthdSpec()
			ovnNorthdName = ovn.CreateOVNNorthd(nil, namespace, spec)
			DeferCleanup(ovn.DeleteOVNNorthd, ovnNorthdName)
		})

		It("reports that the CA secret is missing", func() {
			th.ExpectConditionWithDetails(
				ovnNorthdName,
				ConditionGetterFunc(OVNNorthdConditionGetter),
				condition.TLSInputReadyCondition,
				corev1.ConditionFalse,
				condition.RequestedReason,
				fmt.Sprintf("TLSInput is missing: %s", CABundleSecretName),
			)
			th.ExpectCondition(
				ovnNorthdName,
				ConditionGetterFunc(OVNNorthdConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionFalse,
			)
		})

		It("reports that the cert secret is missing", func() {
			DeferCleanup(k8sClient.Delete, ctx, th.CreateCABundleSecret(types.NamespacedName{
				Name:      CABundleSecretName,
				Namespace: namespace,
			}))
			th.ExpectConditionWithDetails(
				ovnNorthdName,
				ConditionGetterFunc(OVNNorthdConditionGetter),
				condition.TLSInputReadyCondition,
				corev1.ConditionFalse,
				condition.RequestedReason,
				fmt.Sprintf("TLSInput is missing: secrets \"%s in namespace %s\" not found",
					OvnDbCertSecretName, namespace),
			)
			th.ExpectCondition(
				ovnNorthdName,
				ConditionGetterFunc(OVNNorthdConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionFalse,
			)
		})

		It("creates a Deployment with TLS certs attached", func() {
			DeferCleanup(k8sClient.Delete, ctx, th.CreateCABundleSecret(types.NamespacedName{
				Name:      CABundleSecretName,
				Namespace: namespace,
			}))
			DeferCleanup(k8sClient.Delete, ctx, th.CreateCertSecret(types.NamespacedName{
				Name:      OvnDbCertSecretName,
				Namespace: namespace,
			}))

			deploymentName := types.NamespacedName{
				Namespace: namespace,
				Name:      "ovn-northd",
			}
			th.SimulateDeploymentReadyWithPods(
				deploymentName, map[string][]string{},
			)

			d := th.GetDeployment(deploymentName)

			//  check TLS volumes
			th.AssertVolumeExists(CABundleSecretName, d.Spec.Template.Spec.Volumes)
			th.AssertVolumeExists("ovn-northd-tls-certs", d.Spec.Template.Spec.Volumes)

			svcC := d.Spec.Template.Spec.Containers[0]

			// check TLS volume mounts
			th.AssertVolumeMountExists(CABundleSecretName, "tls-ca-bundle.pem", svcC.VolumeMounts)
			th.AssertVolumeMountExists("ovn-northd-tls-certs", "tls.key", svcC.VolumeMounts)
			th.AssertVolumeMountExists("ovn-northd-tls-certs", "tls.crt", svcC.VolumeMounts)
			th.AssertVolumeMountExists("ovn-northd-tls-certs", "ca.crt", svcC.VolumeMounts)

			// check cli args
			Expect(svcC.Args).To(And(
				ContainElement(ContainSubstring("--private-key=")),
				ContainElement(ContainSubstring("--certificate=")),
				ContainElement(ContainSubstring("--ca-cert=")),
			))

			th.ExpectCondition(
				ovnNorthdName,
				ConditionGetterFunc(OVNNorthdConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionTrue,
			)
		})

		It("reconfigures the pods when CA bundle changes", func() {
			DeferCleanup(k8sClient.Delete, ctx, th.CreateCABundleSecret(types.NamespacedName{
				Name:      CABundleSecretName,
				Namespace: namespace,
			}))
			DeferCleanup(k8sClient.Delete, ctx, th.CreateCertSecret(types.NamespacedName{
				Name:      OvnDbCertSecretName,
				Namespace: namespace,
			}))

			deploymentName := types.NamespacedName{
				Namespace: namespace,
				Name:      "ovn-northd",
			}
			th.SimulateDeploymentReadyWithPods(
				deploymentName, map[string][]string{},
			)

			originalHash := GetEnvVarValue(
				th.GetDeployment(deploymentName).Spec.Template.Spec.Containers[0].Env,
				"tls-ca-bundle.pem",
				"",
			)
			Expect(originalHash).NotTo(BeEmpty())

			// Change the content of the CA secret
			th.UpdateSecret(types.NamespacedName{
				Name:      CABundleSecretName,
				Namespace: namespace,
			},
				"tls-ca-bundle.pem",
				[]byte("DifferentCAData"),
			)

			// Assert that the pod is updated
			Eventually(func(g Gomega) {
				newHash := GetEnvVarValue(
					th.GetDeployment(deploymentName).Spec.Template.Spec.Containers[0].Env,
					"tls-ca-bundle.pem",
					"",
				)
				g.Expect(newHash).NotTo(BeEmpty())
				g.Expect(newHash).NotTo(Equal(originalHash))
			}, timeout, interval).Should(Succeed())
		})

		It("reconfigures the pods when cert changes", func() {
			DeferCleanup(k8sClient.Delete, ctx, th.CreateCABundleSecret(types.NamespacedName{
				Name:      CABundleSecretName,
				Namespace: namespace,
			}))
			DeferCleanup(k8sClient.Delete, ctx, th.CreateCertSecret(types.NamespacedName{
				Name:      OvnDbCertSecretName,
				Namespace: namespace,
			}))

			deploymentName := types.NamespacedName{
				Namespace: namespace,
				Name:      "ovn-northd",
			}
			th.SimulateDeploymentReadyWithPods(
				deploymentName, map[string][]string{},
			)

			originalHash := GetEnvVarValue(
				th.GetDeployment(deploymentName).Spec.Template.Spec.Containers[0].Env,
				"certs",
				"",
			)
			Expect(originalHash).NotTo(BeEmpty())

			// Change the content of the cert secret
			th.UpdateSecret(types.NamespacedName{
				Name:      OvnDbCertSecretName,
				Namespace: namespace,
			},
				"tls.crt",
				[]byte("DifferentCrtData"),
			)

			// Assert that the pod is updated
			Eventually(func(g Gomega) {
				newHash := GetEnvVarValue(
					th.GetDeployment(deploymentName).Spec.Template.Spec.Containers[0].Env,
					"certs",
					"",
				)
				g.Expect(newHash).NotTo(BeEmpty())
				g.Expect(newHash).NotTo(Equal(originalHash))
			}, timeout, interval).Should(Succeed())
		})
	})
	When("OVNNorthd is created with topologyref", func() {
		var ovnNorthdName types.NamespacedName
		var deploymentName types.NamespacedName
		var ovnTopologies []types.NamespacedName
		var topologyRef, topologyRefAlt *topologyv1.TopoRef

		BeforeEach(func() {
			ovnNorthdName = types.NamespacedName{
				Name:      "ovn-northd-0",
				Namespace: namespace,
			}
			ovnTopologies = []types.NamespacedName{
				{
					Namespace: namespace,
					Name:      "ovn-topology",
				},
				{
					Namespace: namespace,
					Name:      "ovn-topology-alt",
				},
			}
			// Define the two topology references used in this test
			topologyRef = &topologyv1.TopoRef{
				Name:      ovnTopologies[0].Name,
				Namespace: ovnTopologies[0].Namespace,
			}
			topologyRefAlt = &topologyv1.TopoRef{
				Name:      ovnTopologies[1].Name,
				Namespace: ovnTopologies[1].Namespace,
			}

			// Create Test Topology
			for _, t := range ovnTopologies {
				// Build the topology Spec
				topologySpec, _ := GetSampleTopologySpec(ovnNorthdName.Name)
				infra.CreateTopology(t, topologySpec)
			}
			dbs := CreateOVNDBClusters(namespace, map[string][]string{}, 1)
			DeferCleanup(DeleteOVNDBClusters, dbs)
			spec := GetDefaultOVNNorthdSpec()
			spec.TopologyRef = topologyRef

			ovn.CreateOVNNorthd(&ovnNorthdName.Name, namespace, spec)

			deploymentName = types.NamespacedName{
				Namespace: namespace,
				Name:      "ovn-northd",
			}
			th.SimulateDeploymentReplicaReady(deploymentName)
		})
		It("sets topologyref in both .Status CR and resources", func() {
			Eventually(func(g Gomega) {
				tp := infra.GetTopology(types.NamespacedName{
					Name:      topologyRef.Name,
					Namespace: topologyRef.Namespace,
				})
				finalizers := tp.GetFinalizers()
				g.Expect(finalizers).To(HaveLen(1))
				northd := ovn.GetOVNNorthd(ovnNorthdName)
				g.Expect(northd.Status.LastAppliedTopology).NotTo(BeNil())
				g.Expect(northd.Status.LastAppliedTopology).To(Equal(topologyRef))
				g.Expect(finalizers).To(ContainElement(
					fmt.Sprintf("openstack.org/ovnnorthd-%s", ovnNorthdName.Name)))
			}, timeout, interval).Should(Succeed())

			Eventually(func(g Gomega) {
				_, topologySpecObj := GetSampleTopologySpec(ovnNorthdName.Name)
				g.Expect(th.GetDeployment(deploymentName).Spec.Template.Spec.TopologySpreadConstraints).ToNot(BeNil())
				g.Expect(th.GetDeployment(deploymentName).Spec.Template.Spec.TopologySpreadConstraints).To(Equal(topologySpecObj))
				g.Expect(th.GetDeployment(deploymentName).Spec.Template.Spec.Affinity).To(BeNil())
			}, timeout, interval).Should(Succeed())
		})

		It("updates topology when the reference changes", func() {
			Eventually(func(g Gomega) {
				northd := ovn.GetOVNNorthd(ovnNorthdName)
				northd.Spec.TopologyRef.Name = topologyRefAlt.Name
				g.Expect(k8sClient.Update(ctx, northd)).To(Succeed())
			}, timeout, interval).Should(Succeed())

			Eventually(func(g Gomega) {
				tp := infra.GetTopology(types.NamespacedName{
					Name:      topologyRefAlt.Name,
					Namespace: topologyRefAlt.Namespace,
				})
				finalizers := tp.GetFinalizers()
				g.Expect(finalizers).To(HaveLen(1))
				northd := ovn.GetOVNNorthd(ovnNorthdName)
				g.Expect(northd.Status.LastAppliedTopology).NotTo(BeNil())
				g.Expect(northd.Status.LastAppliedTopology).To(Equal(topologyRefAlt))
				g.Expect(finalizers).To(ContainElement(
					fmt.Sprintf("openstack.org/ovnnorthd-%s", ovnNorthdName.Name)))
			}, timeout, interval).Should(Succeed())

			Eventually(func(g Gomega) {
				_, topologySpecObj := GetSampleTopologySpec(ovnNorthdName.Name)
				g.Expect(th.GetDeployment(deploymentName).Spec.Template.Spec.TopologySpreadConstraints).ToNot(BeNil())
				g.Expect(th.GetDeployment(deploymentName).Spec.Template.Spec.TopologySpreadConstraints).To(Equal(topologySpecObj))
				g.Expect(th.GetDeployment(deploymentName).Spec.Template.Spec.Affinity).To(BeNil())
			}, timeout, interval).Should(Succeed())

			Eventually(func(g Gomega) {
				// Verify the previous referenced topology has no finalizers
				tp := infra.GetTopology(types.NamespacedName{
					Name:      topologyRef.Name,
					Namespace: topologyRef.Namespace,
				})
				finalizers := tp.GetFinalizers()
				g.Expect(finalizers).To(BeEmpty())
			}, timeout, interval).Should(Succeed())
		})
		It("removes topologyRef from the spec", func() {
			Eventually(func(g Gomega) {
				northd := ovn.GetOVNNorthd(ovnNorthdName)
				// Remove the TopologyRef from the existing .Spec
				northd.Spec.TopologyRef = nil
				g.Expect(k8sClient.Update(ctx, northd)).To(Succeed())
			}, timeout, interval).Should(Succeed())

			Eventually(func(g Gomega) {
				northd := ovn.GetOVNNorthd(ovnNorthdName)
				g.Expect(northd.Status.LastAppliedTopology).Should(BeNil())
			}, timeout, interval).Should(Succeed())

			Eventually(func(g Gomega) {
				// Both Affinity and TopologySpreadConstraints are not set
				g.Expect(th.GetDeployment(deploymentName).Spec.Template.Spec.TopologySpreadConstraints).To(BeNil())
				g.Expect(th.GetDeployment(deploymentName).Spec.Template.Spec.Affinity).ToNot(BeNil())
			}, timeout, interval).Should(Succeed())

			// Verify the existing topologies have no finalizer anymore
			Eventually(func(g Gomega) {
				for _, topology := range ovnTopologies {
					tp := infra.GetTopology(types.NamespacedName{
						Name:      topology.Name,
						Namespace: topology.Namespace,
					})
					finalizers := tp.GetFinalizers()
					g.Expect(finalizers).To(BeEmpty())
				}
			}, timeout, interval).Should(Succeed())
		})
	})
	It("rejects a wrong topologyRef on a different namespace", func() {
		spec := map[string]interface{}{}
		// Inject a topologyRef that points to a different namespace
		spec["topologyRef"] = map[string]interface{}{
			"name":      "foo",
			"namespace": "bar",
		}
		raw := map[string]interface{}{
			"apiVersion": "ovn.openstack.org/v1beta1",
			"kind":       "OVNNorthd",
			"metadata": map[string]interface{}{
				"name":      "ovnnorthd-sample",
				"namespace": namespace,
			},
			"spec": spec,
		}
		unstructuredObj := &unstructured.Unstructured{Object: raw}
		_, err := controllerutil.CreateOrPatch(
			th.Ctx, th.K8sClient, unstructuredObj, func() error { return nil })
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(
			ContainSubstring(
				"spec.topologyRef.namespace: Invalid value: \"namespace\": Customizing namespace field is not supported"),
		)
	})
})
