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
	"encoding/json"
	"fmt"

	. "github.com/onsi/ginkgo/v2" //revive:disable:dot-imports
	. "github.com/onsi/gomega"    //revive:disable:dot-imports

	//revive:disable-next-line:dot-imports
	"github.com/openstack-k8s-operators/lib-common/modules/common/service"
	. "github.com/openstack-k8s-operators/lib-common/modules/common/test/helpers"

	networkv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	topologyv1 "github.com/openstack-k8s-operators/infra-operator/apis/topology/v1beta1"
	condition "github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	ovnv1 "github.com/openstack-k8s-operators/ovn-operator/api/v1beta1"
	ovn_common "github.com/openstack-k8s-operators/ovn-operator/pkg/common"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

var _ = Describe("OVNDBCluster controller", func() {
	When("OVNDBCluster CR is created with replicas, dns info should be present",
		func() {
			DescribeTable("should create and delete dnsdata CR",
				func(DnsName string) {
					var clusterName types.NamespacedName
					var OVNSBDBClusterName types.NamespacedName
					var OVNNBDBClusterName types.NamespacedName
					var cluster *ovnv1.OVNDBCluster
					var dbs []types.NamespacedName
					_ = th.CreateNetworkAttachmentDefinition(types.NamespacedName{Namespace: namespace, Name: "internalapi"})
					dbs = CreateOVNDBClusters(namespace, map[string][]string{namespace + "/internalapi": {"10.0.0.1"}}, 3)
					OVNNBDBClusterName = types.NamespacedName{Name: dbs[0].Name, Namespace: dbs[0].Namespace}
					OVNSBDBClusterName = types.NamespacedName{Name: dbs[1].Name, Namespace: dbs[1].Namespace}
					cluster = GetOVNDBCluster(OVNNBDBClusterName)
					clusterName = OVNNBDBClusterName
					if DnsName == "sb" {
						cluster = GetOVNDBCluster(OVNSBDBClusterName)
						clusterName = OVNSBDBClusterName
					}
					statefulSetName := types.NamespacedName{
						Namespace: cluster.Namespace,
						Name:      "ovsdbserver-" + DnsName,
					}

					Eventually(func(_ Gomega) int {
						DNSHostsList := GetDNSDataHostsList(statefulSetName.Namespace, "ovsdbserver-"+DnsName)
						return len(DNSHostsList)
					}).Should(BeNumerically("==", 3))

					// Scale down to 1
					Eventually(func(g Gomega) {
						c := GetOVNDBCluster(clusterName)
						*c.Spec.Replicas = 1
						g.Expect(k8sClient.Update(ctx, c)).Should(Succeed())
					}).Should(Succeed())

					// Check if dnsdata CR is down to 1
					Eventually(func() int {
						listDNS := GetDNSDataHostsList(statefulSetName.Namespace, "ovsdbserver-"+DnsName)
						return len(listDNS)
					}).Should(BeNumerically("==", 1))
				},
				Entry("DNS entry NB", "nb"),
				Entry("DNS entry SB", "sb"),
			)
			DescribeTable("Should update DNSData IP if pod IP changes",
				func(_ string) {
					var clusterName types.NamespacedName
					var OVNSBDBClusterName types.NamespacedName
					var cluster *ovnv1.OVNDBCluster
					var dbs []types.NamespacedName

					_ = th.CreateNetworkAttachmentDefinition(types.NamespacedName{Namespace: namespace, Name: "internalapi"})
					dbs = CreateOVNDBClusters(namespace, map[string][]string{namespace + "/internalapi": {"10.0.0.1"}}, 3)
					OVNSBDBClusterName = types.NamespacedName{Name: dbs[1].Name, Namespace: dbs[1].Namespace}
					cluster = GetOVNDBCluster(OVNSBDBClusterName)
					clusterName = OVNSBDBClusterName

					// Check that DNSData info has been created with correct IP (10.0.0.1)
					Eventually(func(g Gomega) {
						g.Expect(CheckDNSDataContainsIP(cluster.Namespace, "ovsdbserver-sb", "10.0.0.1")).Should(BeTrue())
					}).Should(Succeed())

					// Modify pod IP section
					pod := GetPod(types.NamespacedName{Name: "ovsdbserver-sb-0", Namespace: cluster.Namespace})
					// Create new pod NAD and add it to POD
					netStatus := []networkv1.NetworkStatus{
						{
							Name: cluster.Namespace + "/internalapi",
							IPs:  []string{"10.0.0.10"},
						},
					}
					netStatusAnnotation, err := json.Marshal(netStatus)
					Expect(err).NotTo(HaveOccurred())
					pod.Annotations[networkv1.NetworkStatusAnnot] = string(netStatusAnnotation)
					UpdatePod(pod)
					// End modify pod IP section

					// Call reconcile loop
					Eventually(func(g Gomega) {
						c := GetOVNDBCluster(clusterName)
						// Change something just to call reconcile loop
						c.Spec.ElectionTimer += 1000
						g.Expect(k8sClient.Update(ctx, c)).Should(Succeed())
					}).Should(Succeed())

					// Check that DNSData info has been modified with correct IP (10.0.0.10)
					Eventually(func(g Gomega) {
						g.Expect(CheckDNSDataContainsIP(cluster.Namespace, "ovsdbserver-sb", "10.0.0.1")).Should(BeTrue())
					}).Should(Succeed())

				},
				Entry("DNS CName entry", "ovsdbserver-sb"),
			)
		})

	When("A OVNDBCluster instance is created", func() {
		var OVNDBClusterName types.NamespacedName

		BeforeEach(func() {
			instance := CreateOVNDBCluster(namespace, GetDefaultOVNDBClusterSpec())
			OVNDBClusterName = types.NamespacedName{Name: instance.GetName(), Namespace: instance.GetNamespace()}
			DeferCleanup(th.DeleteInstance, instance)
		})

		It("should have the Spec fields initialized", func() {
			OVNDBCluster := GetOVNDBCluster(OVNDBClusterName)
			Expect(*(OVNDBCluster.Spec.Replicas)).Should(Equal(int32(1)))
			Expect(OVNDBCluster.Spec.LogLevel).Should(Equal("info"))
			Expect(OVNDBCluster.Spec.DBType).Should(Equal(ovnv1.NBDBType))
		})

		It("should have the StatefulSet with podManagementPolicy set to Parallel", func() {
			statefulSetName := types.NamespacedName{
				Namespace: namespace,
				Name:      "ovsdbserver-nb",
			}
			ss := th.GetStatefulSet(statefulSetName)

			Expect(ss.Spec.PodManagementPolicy).Should(Equal(appsv1.ParallelPodManagement))
		})

		It("should have the Status fields initialized", func() {
			OVNDBCluster := GetOVNDBCluster(OVNDBClusterName)
			Expect(OVNDBCluster.Status.Hash).To(BeEmpty())
			Expect(OVNDBCluster.Status.ReadyCount).To(Equal(int32(0)))
		})

		It("should have a finalizer", func() {
			// the reconciler loop adds the finalizer so we have to wait for
			// it to run
			Eventually(func() []string {
				return GetOVNDBCluster(OVNDBClusterName).Finalizers
			}, timeout, interval).Should(ContainElement("openstack.org/ovndbcluster"))
		})

		It("should not create an external config map", func() {
			externalCM := types.NamespacedName{
				Namespace: OVNDBClusterName.Namespace,
				Name:      "ovncontroller-config",
			}
			th.AssertConfigMapDoesNotExist(externalCM)
		})

		DescribeTable("should not create the config map",
			func(cmName string) {
				cm := types.NamespacedName{
					Namespace: namespace,
					Name:      fmt.Sprintf("%s-%s", OVNDBClusterName.Name, cmName),
				}
				th.AssertConfigMapDoesNotExist(cm)
			},
			Entry("scripts CM", "scripts"),
		)
		DescribeTable("should eventually create the config map with OwnerReferences set",
			func(cmName string) {
				cm := types.NamespacedName{
					Namespace: OVNDBClusterName.Namespace,
					Name:      fmt.Sprintf("%s-%s", OVNDBClusterName.Name, cmName),
				}
				Eventually(func() corev1.ConfigMap {

					return *th.GetConfigMap(cm)
				}, timeout, interval).ShouldNot(BeNil())
				// Check OwnerReferences set correctly for the Config Map
				Expect(th.GetConfigMap(cm).ObjectMeta.OwnerReferences[0].Name).To(Equal(OVNDBClusterName.Name))
				Expect(th.GetConfigMap(cm).ObjectMeta.OwnerReferences[0].Kind).To(Equal("OVNDBCluster"))
			},
			Entry("scripts CM", "scripts"),
		)

		It("should create a scripts ConfigMap with namespace from CR", func() {
			cm := types.NamespacedName{
				Namespace: namespace,
				Name:      fmt.Sprintf("%s-%s", OVNDBClusterName.Name, "scripts"),
			}
			Eventually(func() corev1.ConfigMap {
				return *th.GetConfigMap(cm)
			}, timeout, interval).ShouldNot(BeNil())

			Expect(th.GetConfigMap(cm).Data["setup.sh"]).Should(
				ContainSubstring(fmt.Sprintf("NAMESPACE=\"%s\"", namespace)))
		})
	})

	When("A OVNDBCluster instance is created with metrics enabled", func() {
		var OVNDBClusterName types.NamespacedName
		var statefulSetName types.NamespacedName

		BeforeEach(func() {
			spec := GetDefaultOVNDBClusterSpec()
			// Ensure ExporterImage is set to enable metrics
			spec.ExporterImage = "test-exporter:latest"
			instance := CreateOVNDBCluster(namespace, spec)
			OVNDBClusterName = types.NamespacedName{Name: instance.GetName(), Namespace: instance.GetNamespace()}
			DeferCleanup(th.DeleteInstance, instance)

			statefulSetName = types.NamespacedName{
				Namespace: OVNDBClusterName.Namespace,
				Name:      "ovsdbserver-nb", // Use NB as default
			}
		})

		It("creates a StatefulSet with metrics sidecar container", func() {
			sts := th.GetStatefulSet(statefulSetName)

			// Verify both main and metrics containers exist
			Expect(sts.Spec.Template.Spec.Containers).To(HaveLen(2))

			// Check main container
			mainC := sts.Spec.Template.Spec.Containers[0]
			Expect(mainC.Name).To(Equal("ovsdbserver-nb"))

			// Check metrics container
			metricsC := sts.Spec.Template.Spec.Containers[1]
			Expect(metricsC.Name).To(Equal("openstack-network-exporter"))
			Expect(metricsC.Command).To(Equal([]string{"/app/openstack-network-exporter"}))

			// Check metrics container environment
			Expect(metricsC.Env).To(ContainElement(corev1.EnvVar{
				Name:  "OPENSTACK_NETWORK_EXPORTER_YAML",
				Value: "/etc/config/openstack-network-exporter.yaml",
			}))

			// Check metrics container volume mounts
			th.AssertVolumeMountExists("config", "", metricsC.VolumeMounts)
			th.AssertVolumeMountExists("ovsdb-rundir", "", metricsC.VolumeMounts)
		})

		It("creates the required volumes for metrics", func() {
			sts := th.GetStatefulSet(statefulSetName)

			// Verify required volumes exist
			th.AssertVolumeExists("ovsdb-rundir", sts.Spec.Template.Spec.Volumes)
			th.AssertVolumeExists("config", sts.Spec.Template.Spec.Volumes)
		})

		It("creates the metrics config ConfigMap", func() {
			configMapName := types.NamespacedName{
				Namespace: OVNDBClusterName.Namespace,
				Name:      fmt.Sprintf("%s-config", OVNDBClusterName.Name),
			}

			Eventually(func() corev1.ConfigMap {
				return *th.GetConfigMap(configMapName)
			}, timeout, interval).ShouldNot(BeNil())
		})

		It("creates services with metrics labels and ports", func() {
			// Simulate StatefulSet ready with pods to trigger service creation
			th.SimulateStatefulSetReplicaReadyWithPods(statefulSetName, map[string][]string{})

			// Look for services with metrics enabled label
			Eventually(func(g Gomega) {
				services := GetServicesListWithLabel(OVNDBClusterName.Namespace, map[string]string{"metrics": "enabled"})
				g.Expect(services.Items).ToNot(BeEmpty())

				// Verify services still have cluster type and metrics port
				for _, svc := range services.Items {
					// Should have cluster type
					g.Expect(svc.Labels["type"]).To(Equal("cluster"))

					// Should have metrics port
					var hasMetricsPort bool
					for _, port := range svc.Spec.Ports {
						if port.Name == "metrics" && port.Port == ovn_common.MetricsPort {
							hasMetricsPort = true
							break
						}
					}
					g.Expect(hasMetricsPort).To(BeTrue(), fmt.Sprintf("Service %s should have metrics port", svc.Name))
				}
			}, timeout, interval).Should(Succeed())
		})
	})

	When("A OVNDBCluster instance is created with metrics disabled", func() {
		var OVNDBClusterName types.NamespacedName
		var statefulSetName types.NamespacedName

		BeforeEach(func() {
			spec := GetDefaultOVNDBClusterSpec()
			metricsEnabled := false
			spec.MetricsEnabled = &metricsEnabled
			// Set ExporterImage but disable metrics
			spec.ExporterImage = "test-exporter:latest"
			instance := CreateOVNDBCluster(namespace, spec)
			OVNDBClusterName = types.NamespacedName{Name: instance.GetName(), Namespace: instance.GetNamespace()}
			DeferCleanup(th.DeleteInstance, instance)

			statefulSetName = types.NamespacedName{
				Namespace: OVNDBClusterName.Namespace,
				Name:      "ovsdbserver-nb", // Use NB as default
			}
		})

		It("creates a StatefulSet without metrics sidecar container", func() {
			sts := th.GetStatefulSet(statefulSetName)

			// Verify only main container exists (no metrics sidecar)
			Expect(sts.Spec.Template.Spec.Containers).To(HaveLen(1))
			Expect(sts.Spec.Template.Spec.Containers[0].Name).To(Equal("ovsdbserver-nb"))
		})

		It("does not create the metrics config ConfigMap", func() {
			configMapName := types.NamespacedName{
				Namespace: OVNDBClusterName.Namespace,
				Name:      fmt.Sprintf("%s-config", OVNDBClusterName.Name),
			}

			th.AssertConfigMapDoesNotExist(configMapName)
		})

		It("creates services without metrics labels and ports", func() {
			// Simulate StatefulSet ready with pods to trigger service creation
			th.SimulateStatefulSetReplicaReadyWithPods(statefulSetName, map[string][]string{})

			// Verify no services have metrics enabled label
			Eventually(func(g Gomega) {
				metricsServices := GetServicesListWithLabel(OVNDBClusterName.Namespace, map[string]string{"metrics": "enabled"})
				g.Expect(metricsServices.Items).To(BeEmpty())

				// Check that cluster services exist but don't have metrics ports or metrics labels
				clusterServices := GetServicesListWithLabel(OVNDBClusterName.Namespace, map[string]string{"type": "cluster"})
				for _, svc := range clusterServices.Items {
					// Should not have metrics label
					g.Expect(svc.Labels["metrics"]).To(BeEmpty())

					// Services should not have metrics port
					for _, port := range svc.Spec.Ports {
						g.Expect(port.Name).ToNot(Equal("metrics"))
					}
				}
			}, timeout, interval).Should(Succeed())
		})
	})

	When("A OVNDBCluster instance is created with nodeSelector", func() {
		var OVNDBClusterName types.NamespacedName
		var statefulSetName types.NamespacedName

		BeforeEach(func() {
			spec := GetDefaultOVNDBClusterSpec()
			nodeSelector := map[string]string{
				"foo": "bar",
			}
			spec.NodeSelector = &nodeSelector
			instance := CreateOVNDBCluster(namespace, spec)
			OVNDBClusterName = types.NamespacedName{Name: instance.GetName(), Namespace: instance.GetNamespace()}
			DeferCleanup(th.DeleteInstance, instance)

			statefulSetName = types.NamespacedName{
				Namespace: namespace,
				Name:      "ovsdbserver-nb",
			}
			th.SimulateStatefulSetReplicaReady(statefulSetName)
		})

		It("sets nodeSelector in resource specs", func() {
			Eventually(func(g Gomega) {
				g.Expect(th.GetStatefulSet(statefulSetName).Spec.Template.Spec.NodeSelector).To(Equal(map[string]string{"foo": "bar"}))
			}, timeout, interval).Should(Succeed())
		})

		It("updates nodeSelector in resource specs when changed", func() {
			Eventually(func(g Gomega) {
				g.Expect(th.GetStatefulSet(statefulSetName).Spec.Template.Spec.NodeSelector).To(Equal(map[string]string{"foo": "bar"}))
			}, timeout, interval).Should(Succeed())

			Eventually(func(g Gomega) {
				dbCluster := GetOVNDBCluster(OVNDBClusterName)
				newNodeSelector := map[string]string{
					"foo2": "bar2",
				}
				dbCluster.Spec.NodeSelector = &newNodeSelector
				g.Expect(k8sClient.Update(ctx, dbCluster)).Should(Succeed())
			}, timeout, interval).Should(Succeed())

			Eventually(func(g Gomega) {
				g.Expect(th.GetStatefulSet(statefulSetName).Spec.Template.Spec.NodeSelector).To(Equal(map[string]string{"foo2": "bar2"}))
			}, timeout, interval).Should(Succeed())
		})

		It("removes nodeSelector from resource specs when cleared", func() {
			Eventually(func(g Gomega) {
				g.Expect(th.GetStatefulSet(statefulSetName).Spec.Template.Spec.NodeSelector).To(Equal(map[string]string{"foo": "bar"}))
			}, timeout, interval).Should(Succeed())

			Eventually(func(g Gomega) {
				dbCluster := GetOVNDBCluster(OVNDBClusterName)
				emptyNodeSelector := map[string]string{}
				dbCluster.Spec.NodeSelector = &emptyNodeSelector
				g.Expect(k8sClient.Update(ctx, dbCluster)).Should(Succeed())
			}, timeout, interval).Should(Succeed())

			Eventually(func(g Gomega) {
				g.Expect(th.GetStatefulSet(statefulSetName).Spec.Template.Spec.NodeSelector).To(BeNil())
			}, timeout, interval).Should(Succeed())
		})

		It("removes nodeSelector from resource specs when nilled", func() {
			Eventually(func(g Gomega) {
				g.Expect(th.GetStatefulSet(statefulSetName).Spec.Template.Spec.NodeSelector).To(Equal(map[string]string{"foo": "bar"}))
			}, timeout, interval).Should(Succeed())

			Eventually(func(g Gomega) {
				dbCluster := GetOVNDBCluster(OVNDBClusterName)
				dbCluster.Spec.NodeSelector = nil
				g.Expect(k8sClient.Update(ctx, dbCluster)).Should(Succeed())
			}, timeout, interval).Should(Succeed())

			Eventually(func(g Gomega) {
				g.Expect(th.GetStatefulSet(statefulSetName).Spec.Template.Spec.NodeSelector).To(BeNil())
			}, timeout, interval).Should(Succeed())
		})
	})

	When("OVNDBClusters are created with networkAttachments", func() {
		It("does not break if pods are not created yet", func() {
			// Create OVNDBCluster with 1 replica
			spec := GetDefaultOVNDBClusterSpec()
			spec.NetworkAttachment = "internalapi"
			internalAPINADName := types.NamespacedName{Namespace: namespace, Name: "internalapi"}
			nad := th.CreateNetworkAttachmentDefinition(internalAPINADName)
			DeferCleanup(th.DeleteInstance, nad)
			dbs := CreateOVNDBClusters(namespace, map[string][]string{namespace + "/internalapi": {"10.0.0.1"}}, 1)

			// Increase replica to 3
			Eventually(func(g Gomega) {
				c := GetOVNDBCluster(dbs[0])
				*c.Spec.Replicas = 3
				g.Expect(k8sClient.Update(ctx, c)).Should(Succeed())
			}).Should(Succeed())

			//Check that error occurs
			Eventually(func(g Gomega) {
				conditions := GetOVNDBCluster(dbs[0]).Status.Conditions
				cond := conditions.Get(condition.ExposeServiceReadyCondition)
				g.Expect(cond.Status).To(Equal(corev1.ConditionFalse))
			}).Should(Succeed())

			// Decrease replicas back to 1
			Eventually(func(g Gomega) {
				c := GetOVNDBCluster(dbs[0])
				*c.Spec.Replicas = 1
				g.Expect(k8sClient.Update(ctx, c)).Should(Succeed())
			}).Should(Succeed())

			//Check that error doesn't happen and instance is ready
			Eventually(func(g Gomega) {
				conditions := GetOVNDBCluster(dbs[0]).Status.Conditions
				cond := conditions.Get(condition.DeploymentReadyCondition)
				g.Expect(cond.Status).To(Equal(corev1.ConditionTrue))
			}).Should(Succeed())

		})

		It("should create an external config map", func() {
			var instance *ovnv1.OVNDBCluster
			spec := GetDefaultOVNDBClusterSpec()
			spec.NetworkAttachment = "internalapi"
			internalAPINADName := types.NamespacedName{Namespace: namespace, Name: "internalapi"}
			nad := th.CreateNetworkAttachmentDefinition(internalAPINADName)
			DeferCleanup(th.DeleteInstance, nad)
			dbs := CreateOVNDBClusters(namespace, map[string][]string{namespace + "/internalapi": {"10.0.0.1"}}, 1)
			if GetOVNDBCluster(dbs[0]).Spec.DBType == ovnv1.SBDBType {
				instance = GetOVNDBCluster(dbs[0])
			} else {
				instance = GetOVNDBCluster(dbs[1])
			}
			configCM := types.NamespacedName{
				Namespace: instance.GetNamespace(),
				Name:      "ovncontroller-config",
			}
			Eventually(func() corev1.ConfigMap {
				return *th.GetConfigMap(configCM)
			}, timeout, interval).ShouldNot(BeNil())
		})
	})

	When("OVNDBCluster is created with networkAttachments", func() {
		var OVNDBClusterName types.NamespacedName
		BeforeEach(func() {
			spec := GetDefaultOVNDBClusterSpec()
			spec.NetworkAttachment = "internalapi"
			spec.DBType = ovnv1.SBDBType
			// Disable metrics for network attachment tests to avoid sidecar container complications
			metricsEnabled := false
			spec.MetricsEnabled = &metricsEnabled
			instance := CreateOVNDBCluster(namespace, spec)
			OVNDBClusterName = types.NamespacedName{Name: instance.GetName(), Namespace: instance.GetNamespace()}
			DeferCleanup(th.DeleteInstance, instance)
		})

		It("should create services", func() {
			internalAPINADName := types.NamespacedName{Namespace: namespace, Name: "internalapi"}
			nad := th.CreateNetworkAttachmentDefinition(internalAPINADName)
			DeferCleanup(th.DeleteInstance, nad)

			statefulSetName := types.NamespacedName{
				Namespace: namespace,
				Name:      "ovsdbserver-sb",
			}
			th.SimulateStatefulSetReplicaReadyWithPods(
				statefulSetName,
				map[string][]string{namespace + "/internalapi": {"10.0.0.1"}},
			)
			// Cluster is created with 1 replica, serviceListWithoutTypeLabel should be 2:
			// - ovsdbserver-sb   (headless type)
			// - ovsdbserver-sb-0 (cluster type)
			Eventually(func(g Gomega) {
				serviceListWithoutTypeLabel := GetServicesListWithLabel(namespace)
				g.Expect(serviceListWithoutTypeLabel.Items).To(HaveLen(2))
			}).Should(Succeed())

			Eventually(func(g Gomega) {
				serviceListWithClusterType := GetServicesListWithLabel(namespace, map[string]string{"type": "cluster"})
				g.Expect(serviceListWithClusterType.Items).To(HaveLen(1))
				g.Expect(serviceListWithClusterType.Items[0].Name).To(Equal("ovsdbserver-sb-0"))
			}).Should(Succeed())

			Eventually(func(g Gomega) {
				serviceListWithHeadlessType := GetServicesListWithLabel(namespace, map[string]string{"type": "headless"})
				g.Expect(serviceListWithHeadlessType.Items).To(HaveLen(1))
				g.Expect(serviceListWithHeadlessType.Items[0].Name).To(Equal("ovsdbserver-sb"))
			}).Should(Succeed())
		})

		It("should create an external ConfigMap with expected key-value pairs and OwnerReferences set", func() {
			internalAPINADName := types.NamespacedName{Namespace: namespace, Name: "internalapi"}
			nad := th.CreateNetworkAttachmentDefinition(internalAPINADName)
			DeferCleanup(th.DeleteInstance, nad)

			statefulSetName := types.NamespacedName{
				Namespace: namespace,
				Name:      "ovsdbserver-sb",
			}
			th.SimulateStatefulSetReplicaReadyWithPods(
				statefulSetName,
				map[string][]string{namespace + "/internalapi": {"10.0.0.1"}},
			)

			externalCM := types.NamespacedName{
				Namespace: OVNDBClusterName.Namespace,
				Name:      "ovncontroller-config",
			}

			ExpectedExternalSBEndpoint := "tcp:ovsdbserver-sb." + namespace + ".svc:6642"

			Eventually(func() corev1.ConfigMap {
				return *th.GetConfigMap(externalCM)
			}, timeout, interval).ShouldNot(BeNil())

			// Check OwnerReferences set correctly for the Config Map
			Expect(th.GetConfigMap(externalCM).ObjectMeta.OwnerReferences[0].Name).To(Equal(OVNDBClusterName.Name))
			Expect(th.GetConfigMap(externalCM).ObjectMeta.OwnerReferences[0].Kind).To(Equal("OVNDBCluster"))

			Eventually(func(g Gomega) {
				g.Expect(th.GetConfigMap(externalCM).Data["ovsdb-config"]).Should(
					ContainSubstring("ovn-remote: %s", ExpectedExternalSBEndpoint))
			}, timeout, interval).Should(Succeed())
		})

		It("should create an external ConfigMap with ovn-encap-type and ovn-encap-tos if OVNController is configured", func() {
			ExpectedEncapType := "vxlan"
			ExpectedEncapTos := "inherit"
			// Spawn OVNController with vxlan as ExternalIDs.OvnEncapType
			ovncontrollerSpec := GetDefaultOVNControllerSpec()
			ovncontrollerSpec.ExternalIDS.OvnEncapType = ExpectedEncapType
			ovncontrollerSpec.ExternalIDS.OvnEncapTos = ExpectedEncapTos
			ovnController := CreateOVNController(namespace, ovncontrollerSpec)
			DeferCleanup(th.DeleteInstance, ovnController)
			internalAPINADName := types.NamespacedName{Namespace: namespace, Name: "internalapi"}
			nad := th.CreateNetworkAttachmentDefinition(internalAPINADName)
			DeferCleanup(th.DeleteInstance, nad)

			statefulSetName := types.NamespacedName{
				Namespace: namespace,
				Name:      "ovsdbserver-sb",
			}
			th.SimulateStatefulSetReplicaReadyWithPods(
				statefulSetName,
				map[string][]string{namespace + "/internalapi": {"10.0.0.1"}},
			)

			externalCM := types.NamespacedName{
				Namespace: OVNDBClusterName.Namespace,
				Name:      "ovncontroller-config",
			}

			ExpectedExternalSBEndpoint := "tcp:ovsdbserver-sb." + namespace + ".svc:6642"

			Eventually(func() corev1.ConfigMap {
				return *th.GetConfigMap(externalCM)
			}, timeout, interval).ShouldNot(BeNil())

			// Check OwnerReferences set correctly for the Config Map
			Expect(th.GetConfigMap(externalCM).ObjectMeta.OwnerReferences[0].Name).To(Equal(OVNDBClusterName.Name))
			Expect(th.GetConfigMap(externalCM).ObjectMeta.OwnerReferences[0].Kind).To(Equal("OVNDBCluster"))

			Eventually(func(g Gomega) {
				g.Expect(th.GetConfigMap(externalCM).Data["ovsdb-config"]).Should(
					ContainSubstring("ovn-remote: %s", ExpectedExternalSBEndpoint))
			}, timeout, interval).Should(Succeed())
			Eventually(func(g Gomega) {
				g.Expect(th.GetConfigMap(externalCM).Data["ovsdb-config"]).Should(
					ContainSubstring("ovn-encap-type: %s", ExpectedEncapType))
			}, timeout, interval).Should(Succeed())
			Eventually(func(g Gomega) {
				g.Expect(th.GetConfigMap(externalCM).Data["ovsdb-config"]).Should(
					ContainSubstring("ovn-encap-tos: %s", ExpectedEncapTos))
			}, timeout, interval).Should(Succeed())
		})

		It("should remove ovnEncapType if OVNController gets deleted", func() {
			ExpectedEncapType := "vxlan"
			ExpectedEncapTos := "inherit"
			// Spawn OVNController with vxlan as ExternalIDs.OvnEncapType
			ovncontrollerSpec := GetDefaultOVNControllerSpec()
			ovncontrollerSpec.ExternalIDS.OvnEncapType = ExpectedEncapType
			ovncontrollerSpec.ExternalIDS.OvnEncapTos = ExpectedEncapTos
			ovnController := CreateOVNController(namespace, ovncontrollerSpec)
			//DeferCleanup(th.DeleteInstance, ovnController)
			internalAPINADName := types.NamespacedName{Namespace: namespace, Name: "internalapi"}
			nad := th.CreateNetworkAttachmentDefinition(internalAPINADName)
			DeferCleanup(th.DeleteInstance, nad)

			statefulSetName := types.NamespacedName{
				Namespace: namespace,
				Name:      "ovsdbserver-sb",
			}
			th.SimulateStatefulSetReplicaReadyWithPods(
				statefulSetName,
				map[string][]string{namespace + "/internalapi": {"10.0.0.1"}},
			)

			externalCM := types.NamespacedName{
				Namespace: OVNDBClusterName.Namespace,
				Name:      "ovncontroller-config",
			}

			ExpectedExternalSBEndpoint := "tcp:ovsdbserver-sb." + namespace + ".svc:6642"

			Eventually(func() corev1.ConfigMap {
				return *th.GetConfigMap(externalCM)
			}, timeout, interval).ShouldNot(BeNil())

			// Check OwnerReferences set correctly for the Config Map
			Expect(th.GetConfigMap(externalCM).ObjectMeta.OwnerReferences[0].Name).To(Equal(OVNDBClusterName.Name))
			Expect(th.GetConfigMap(externalCM).ObjectMeta.OwnerReferences[0].Kind).To(Equal("OVNDBCluster"))

			Eventually(func(g Gomega) {
				g.Expect(th.GetConfigMap(externalCM).Data["ovsdb-config"]).Should(
					ContainSubstring("ovn-remote: %s", ExpectedExternalSBEndpoint))
			}, timeout, interval).Should(Succeed())
			Eventually(func(g Gomega) {
				g.Expect(th.GetConfigMap(externalCM).Data["ovsdb-config"]).Should(
					ContainSubstring("ovn-encap-type: %s", ExpectedEncapType))
			}, timeout, interval).Should(Succeed())
			Eventually(func(g Gomega) {
				g.Expect(th.GetConfigMap(externalCM).Data["ovsdb-config"]).Should(
					ContainSubstring("ovn-encap-tos: %s", ExpectedEncapTos))
			}, timeout, interval).Should(Succeed())

			// This should trigger an OVNDBCluster reconcile and update config map
			// without ovn-encap-type and ovn-encap-tos
			DeleteOVNController(types.NamespacedName{Name: ovnController.GetName(), Namespace: namespace})

			Eventually(func() corev1.ConfigMap {
				return *th.GetConfigMap(externalCM)
			}, timeout, interval).ShouldNot(BeNil())
			Eventually(func(g Gomega) {
				g.Expect(th.GetConfigMap(externalCM).Data["ovsdb-config"]).Should(
					ContainSubstring("ovn-remote: %s", ExpectedExternalSBEndpoint))
			}, timeout, interval).Should(Succeed())
			Eventually(func(g Gomega) {
				g.Expect(th.GetConfigMap(externalCM).Data["ovsdb-config"]).ShouldNot(
					ContainSubstring("ovn-encap-type: %s", ExpectedEncapType))
			}, timeout, interval).Should(Succeed())
			Eventually(func(g Gomega) {
				g.Expect(th.GetConfigMap(externalCM).Data["ovsdb-config"]).ShouldNot(
					ContainSubstring("ovn-encap-tos: %s", ExpectedEncapTos))
			}, timeout, interval).Should(Succeed())
		})

		It("should delete an external ConfigMap once SB DBCluster is detached from NAD", func() {
			internalAPINADName := types.NamespacedName{Namespace: namespace, Name: "internalapi"}
			nad := th.CreateNetworkAttachmentDefinition(internalAPINADName)
			DeferCleanup(th.DeleteInstance, nad)

			statefulSetName := types.NamespacedName{
				Namespace: namespace,
				Name:      "ovsdbserver-sb",
			}
			th.SimulateStatefulSetReplicaReadyWithPods(
				statefulSetName,
				map[string][]string{namespace + "/internalapi": {"10.0.0.1"}},
			)

			externalCM := types.NamespacedName{
				Namespace: OVNDBClusterName.Namespace,
				Name:      "ovncontroller-config",
			}

			// Should exist externalCM
			Eventually(func() corev1.ConfigMap {
				return *th.GetConfigMap(externalCM)
			}, timeout, interval).ShouldNot(BeNil())

			// Detach SBCluster from NAD
			Eventually(func(g Gomega) {
				ovndbcluster := GetOVNDBCluster(OVNDBClusterName)
				ovndbcluster.Spec.NetworkAttachment = ""
				g.Expect(k8sClient.Update(ctx, ovndbcluster)).Should(Succeed())
			}, timeout, interval).Should(Succeed())
			th.AssertConfigMapDoesNotExist(externalCM)
		})

		It("reports that the definition is missing", func() {
			th.ExpectConditionWithDetails(
				OVNDBClusterName,
				ConditionGetterFunc(OVNDBClusterConditionGetter),
				condition.NetworkAttachmentsReadyCondition,
				corev1.ConditionFalse,
				condition.RequestedReason,
				"NetworkAttachment resources missing: internalapi",
			)
		})
		It("reports that network attachment is missing", func() {
			internalAPINADName := types.NamespacedName{Namespace: namespace, Name: "internalapi"}
			nad := th.CreateNetworkAttachmentDefinition(internalAPINADName)
			DeferCleanup(th.DeleteInstance, nad)

			statefulSetName := types.NamespacedName{
				Namespace: namespace,
				Name:      "ovsdbserver-sb",
			}
			ss := th.GetStatefulSet(statefulSetName)

			expectedAnnotation, err := json.Marshal(
				[]networkv1.NetworkSelectionElement{
					{
						Name:             "internalapi",
						Namespace:        namespace,
						InterfaceRequest: "internalapi",
					}})
			Expect(err).ShouldNot(HaveOccurred())
			Expect(ss.Spec.Template.ObjectMeta.Annotations).To(
				HaveKeyWithValue("k8s.v1.cni.cncf.io/networks", string(expectedAnnotation)),
			)

			// We don't add network attachment status annotations to the Pods
			// to simulate that the network attachments are missing.
			th.SimulateStatefulSetReplicaReadyWithPods(statefulSetName, map[string][]string{})

			th.ExpectConditionWithDetails(
				OVNDBClusterName,
				ConditionGetterFunc(OVNDBClusterConditionGetter),
				condition.NetworkAttachmentsReadyCondition,
				corev1.ConditionFalse,
				condition.ErrorReason,
				"NetworkAttachments error occurred "+
					"not all pods have interfaces with ips as configured in NetworkAttachments: internalapi",
			)
		})
		It("reports that an IP is missing", func() {
			internalAPINADName := types.NamespacedName{Namespace: namespace, Name: "internalapi"}
			nad := th.CreateNetworkAttachmentDefinition(internalAPINADName)
			DeferCleanup(th.DeleteInstance, nad)

			statefulSetName := types.NamespacedName{
				Namespace: namespace,
				Name:      "ovsdbserver-sb",
			}
			ss := th.GetStatefulSet(statefulSetName)

			expectedAnnotation, err := json.Marshal(
				[]networkv1.NetworkSelectionElement{
					{
						Name:             "internalapi",
						Namespace:        namespace,
						InterfaceRequest: "internalapi",
					}})
			Expect(err).ShouldNot(HaveOccurred())
			Expect(ss.Spec.Template.ObjectMeta.Annotations).To(
				HaveKeyWithValue("k8s.v1.cni.cncf.io/networks", string(expectedAnnotation)),
			)

			// We simulate that there is no IP associated with the internalapi
			// network attachment
			th.SimulateStatefulSetReplicaReadyWithPods(
				statefulSetName,
				map[string][]string{namespace + "/internalapi": {}},
			)

			th.ExpectConditionWithDetails(
				OVNDBClusterName,
				ConditionGetterFunc(OVNDBClusterConditionGetter),
				condition.NetworkAttachmentsReadyCondition,
				corev1.ConditionFalse,
				condition.ErrorReason,
				"NetworkAttachments error occurred "+
					"not all pods have interfaces with ips as configured in NetworkAttachments: internalapi",
			)
		})
		It("reports NetworkAttachmentsReady if the Pods got the proper annotations", func() {
			internalAPINADName := types.NamespacedName{Namespace: namespace, Name: "internalapi"}
			nad := th.CreateNetworkAttachmentDefinition(internalAPINADName)
			DeferCleanup(th.DeleteInstance, nad)

			statefulSetName := types.NamespacedName{
				Namespace: namespace,
				Name:      "ovsdbserver-sb",
			}
			th.SimulateStatefulSetReplicaReadyWithPods(
				statefulSetName,
				map[string][]string{namespace + "/internalapi": {"10.0.0.1"}},
			)

			th.ExpectCondition(
				OVNDBClusterName,
				ConditionGetterFunc(OVNDBClusterConditionGetter),
				condition.NetworkAttachmentsReadyCondition,
				corev1.ConditionTrue,
			)

			Eventually(func(g Gomega) {
				OVNDBCluster := GetOVNDBCluster(OVNDBClusterName)
				g.Expect(OVNDBCluster.Status.NetworkAttachments).To(
					Equal(map[string][]string{namespace + "/internalapi": {"10.0.0.1"}}))

			}, timeout, interval).Should(Succeed())

			th.ExpectCondition(
				OVNDBClusterName,
				ConditionGetterFunc(OVNDBClusterConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionTrue,
			)
		})

		When("OVNDBCluster gets migrated to use service override", func() {
			BeforeEach(func() {
				internalAPINADName := types.NamespacedName{Namespace: namespace, Name: "internalapi"}
				nad := th.CreateNetworkAttachmentDefinition(internalAPINADName)
				DeferCleanup(th.DeleteInstance, nad)

				statefulSetName := types.NamespacedName{
					Namespace: namespace,
					Name:      "ovsdbserver-sb",
				}
				th.SimulateStatefulSetReplicaReadyWithPods(
					statefulSetName,
					map[string][]string{namespace + "/internalapi": {"10.0.0.1"}},
				)

				th.ExpectCondition(
					OVNDBClusterName,
					ConditionGetterFunc(OVNDBClusterConditionGetter),
					condition.NetworkAttachmentsReadyCondition,
					corev1.ConditionTrue,
				)

				Eventually(func(g Gomega) {
					OVNDBCluster := GetOVNDBCluster(OVNDBClusterName)
					g.Expect(OVNDBCluster.Status.NetworkAttachments).To(
						Equal(map[string][]string{namespace + "/internalapi": {"10.0.0.1"}}))

				}, timeout, interval).Should(Succeed())

				th.ExpectCondition(
					OVNDBClusterName,
					ConditionGetterFunc(OVNDBClusterConditionGetter),
					condition.ReadyCondition,
					corev1.ConditionTrue,
				)
			})

			It("creates a loadbalancer service for accessing the SB DB", func() {
				ovnDB := GetOVNDBCluster(OVNDBClusterName)
				Expect(ovnDB).ToNot(BeNil())

				_, err := controllerutil.CreateOrPatch(
					th.Ctx, th.K8sClient, ovnDB, func() error {
						ovnDB.Spec.Override.Service = &service.OverrideSpec{
							EmbeddedLabelsAnnotations: &service.EmbeddedLabelsAnnotations{
								Annotations: map[string]string{
									"metallb.universe.tf/address-pool":    "osp-internalapi",
									"metallb.universe.tf/allow-shared-ip": "osp-internalapi",
									"metallb.universe.tf/loadBalancerIPs": "internal-lb-ip-1,internal-lb-ip-2",
								},
							},
							Spec: &service.OverrideServiceSpec{
								Type: corev1.ServiceTypeLoadBalancer,
							},
						}
						ovnDB.Spec.NetworkAttachment = ""
						return nil
					})
				Expect(err).ToNot(HaveOccurred())
				th.SimulateLoadBalancerServiceIP(types.NamespacedName{Namespace: namespace, Name: "ovsdbserver-sb"})

				// Cluster is created with 1 replica, serviceListWithoutTypeLabel should be 2:
				// - ovsdbserver-sb   (loadbalancer type)
				// - ovsdbserver-sb-0 (cluster type)
				Eventually(func(g Gomega) {
					serviceListWithoutTypeLabel := GetServicesListWithLabel(namespace)
					g.Expect(serviceListWithoutTypeLabel.Items).To(HaveLen(2))
				}).Should(Succeed())

				Eventually(func(g Gomega) {
					serviceListWithClusterType := GetServicesListWithLabel(namespace, map[string]string{"type": "cluster"})
					g.Expect(serviceListWithClusterType.Items).To(HaveLen(1))
					g.Expect(serviceListWithClusterType.Items[0].Name).To(Equal("ovsdbserver-sb-0"))
				}).Should(Succeed())

				Eventually(func(g Gomega) {
					serviceListWithLoadBalancerType := GetServicesListWithLabel(namespace, map[string]string{"type": "loadbalancer"})
					g.Expect(serviceListWithLoadBalancerType.Items).To(HaveLen(1))
					g.Expect(serviceListWithLoadBalancerType.Items[0].Name).To(Equal("ovsdbserver-sb"))
					g.Expect(serviceListWithLoadBalancerType.Items[0].Spec.Type).To(Equal(corev1.ServiceTypeLoadBalancer))
				}).Should(Succeed())
			})
		})
	})

	When("OVNDBCluster is created with a service override", func() {
		var OVNDBClusterName types.NamespacedName
		BeforeEach(func() {
			spec := GetDefaultOVNDBClusterSpec()
			spec.Override.Service = &service.OverrideSpec{
				EmbeddedLabelsAnnotations: &service.EmbeddedLabelsAnnotations{
					Annotations: map[string]string{
						"metallb.universe.tf/address-pool":    "osp-internalapi",
						"metallb.universe.tf/allow-shared-ip": "osp-internalapi",
						"metallb.universe.tf/loadBalancerIPs": "internal-lb-ip-1,internal-lb-ip-2",
					},
				},
				Spec: &service.OverrideServiceSpec{
					Type: corev1.ServiceTypeLoadBalancer,
				},
			}

			spec.DBType = ovnv1.SBDBType
			instance := CreateOVNDBCluster(namespace, spec)
			OVNDBClusterName = types.NamespacedName{Name: instance.GetName(), Namespace: instance.GetNamespace()}
			DeferCleanup(th.DeleteInstance, instance)
		})

		It("should create services", func() {
			statefulSetName := types.NamespacedName{
				Namespace: namespace,
				Name:      "ovsdbserver-sb",
			}
			th.SimulateStatefulSetReplicaReadyWithPods(
				statefulSetName,
				map[string][]string{},
			)
			th.SimulateLoadBalancerServiceIP(types.NamespacedName{Namespace: namespace, Name: "ovsdbserver-sb"})
			// Cluster is created with 1 replica, serviceListWithoutTypeLabel should be 2:
			// - ovsdbserver-sb   (loadbalancer type)
			// - ovsdbserver-sb-0 (cluster type)
			Eventually(func(g Gomega) {
				serviceListWithoutTypeLabel := GetServicesListWithLabel(namespace)
				g.Expect(serviceListWithoutTypeLabel.Items).To(HaveLen(2))
			}).Should(Succeed())

			Eventually(func(g Gomega) {
				serviceListWithClusterType := GetServicesListWithLabel(namespace, map[string]string{"type": "cluster"})
				g.Expect(serviceListWithClusterType.Items).To(HaveLen(1))
				g.Expect(serviceListWithClusterType.Items[0].Name).To(Equal("ovsdbserver-sb-0"))
			}).Should(Succeed())

			Eventually(func(g Gomega) {
				serviceListWithLoadBalancerType := GetServicesListWithLabel(namespace, map[string]string{"type": "loadbalancer"})
				g.Expect(serviceListWithLoadBalancerType.Items).To(HaveLen(1))
				g.Expect(serviceListWithLoadBalancerType.Items[0].Name).To(Equal("ovsdbserver-sb"))
			}).Should(Succeed())
		})

		It("should create an external ConfigMap with expected key-value pairs and OwnerReferences set", func() {
			statefulSetName := types.NamespacedName{
				Namespace: namespace,
				Name:      "ovsdbserver-sb",
			}
			th.SimulateStatefulSetReplicaReadyWithPods(
				statefulSetName,
				map[string][]string{},
			)
			th.SimulateLoadBalancerServiceIP(types.NamespacedName{Namespace: namespace, Name: "ovsdbserver-sb"})

			externalCM := types.NamespacedName{
				Namespace: OVNDBClusterName.Namespace,
				Name:      "ovncontroller-config",
			}

			ExpectedExternalSBEndpoint := "tcp:ovsdbserver-sb." + namespace + ".svc:6642"

			Eventually(func() corev1.ConfigMap {
				return *th.GetConfigMap(externalCM)
			}, timeout, interval).ShouldNot(BeNil())

			// Check OwnerReferences set correctly for the Config Map
			Expect(th.GetConfigMap(externalCM).ObjectMeta.OwnerReferences[0].Name).To(Equal(OVNDBClusterName.Name))
			Expect(th.GetConfigMap(externalCM).ObjectMeta.OwnerReferences[0].Kind).To(Equal("OVNDBCluster"))

			Eventually(func(g Gomega) {
				g.Expect(th.GetConfigMap(externalCM).Data["ovsdb-config"]).Should(
					ContainSubstring("ovn-remote: %s", ExpectedExternalSBEndpoint))
			}, timeout, interval).Should(Succeed())
		})

		It("should create an external ConfigMap with ovn-encap-type and ovn-encap-tos if OVNController is configured", func() {
			ExpectedEncapType := "vxlan"
			ExpectedEncapTos := "inherit"
			// Spawn OVNController with vxlan as ExternalIDs.OvnEncapType
			ovncontrollerSpec := GetDefaultOVNControllerSpec()
			ovncontrollerSpec.ExternalIDS.OvnEncapType = ExpectedEncapType
			ovncontrollerSpec.ExternalIDS.OvnEncapTos = ExpectedEncapTos
			ovnController := CreateOVNController(namespace, ovncontrollerSpec)
			DeferCleanup(th.DeleteInstance, ovnController)

			statefulSetName := types.NamespacedName{
				Namespace: namespace,
				Name:      "ovsdbserver-sb",
			}
			th.SimulateStatefulSetReplicaReadyWithPods(
				statefulSetName,
				map[string][]string{},
			)
			th.SimulateLoadBalancerServiceIP(types.NamespacedName{Namespace: namespace, Name: "ovsdbserver-sb"})

			externalCM := types.NamespacedName{
				Namespace: OVNDBClusterName.Namespace,
				Name:      "ovncontroller-config",
			}

			ExpectedExternalSBEndpoint := "tcp:ovsdbserver-sb." + namespace + ".svc:6642"

			Eventually(func() corev1.ConfigMap {
				return *th.GetConfigMap(externalCM)
			}, timeout, interval).ShouldNot(BeNil())

			// Check OwnerReferences set correctly for the Config Map
			Expect(th.GetConfigMap(externalCM).ObjectMeta.OwnerReferences[0].Name).To(Equal(OVNDBClusterName.Name))
			Expect(th.GetConfigMap(externalCM).ObjectMeta.OwnerReferences[0].Kind).To(Equal("OVNDBCluster"))

			Eventually(func(g Gomega) {
				g.Expect(th.GetConfigMap(externalCM).Data["ovsdb-config"]).Should(
					ContainSubstring("ovn-remote: %s", ExpectedExternalSBEndpoint))
			}, timeout, interval).Should(Succeed())
			Eventually(func(g Gomega) {
				g.Expect(th.GetConfigMap(externalCM).Data["ovsdb-config"]).Should(
					ContainSubstring("ovn-encap-type: %s", ExpectedEncapType))
			}, timeout, interval).Should(Succeed())
			Eventually(func(g Gomega) {
				g.Expect(th.GetConfigMap(externalCM).Data["ovsdb-config"]).Should(
					ContainSubstring("ovn-encap-tos: %s", ExpectedEncapTos))
			}, timeout, interval).Should(Succeed())
		})

		It("should remove ovnEncapType and ovnEncapTos if OVNController gets deleted", func() {
			ExpectedEncapType := "vxlan"
			ExpectedEncapTos := "inherit"
			// Spawn OVNController with vxlan as ExternalIDs.OvnEncapType
			ovncontrollerSpec := GetDefaultOVNControllerSpec()
			ovncontrollerSpec.ExternalIDS.OvnEncapType = ExpectedEncapType
			ovncontrollerSpec.ExternalIDS.OvnEncapTos = ExpectedEncapTos
			ovnController := CreateOVNController(namespace, ovncontrollerSpec)

			statefulSetName := types.NamespacedName{
				Namespace: namespace,
				Name:      "ovsdbserver-sb",
			}
			th.SimulateStatefulSetReplicaReadyWithPods(
				statefulSetName,
				map[string][]string{},
			)
			th.SimulateLoadBalancerServiceIP(types.NamespacedName{Namespace: namespace, Name: "ovsdbserver-sb"})

			externalCM := types.NamespacedName{
				Namespace: OVNDBClusterName.Namespace,
				Name:      "ovncontroller-config",
			}

			ExpectedExternalSBEndpoint := "tcp:ovsdbserver-sb." + namespace + ".svc:6642"

			Eventually(func() corev1.ConfigMap {
				return *th.GetConfigMap(externalCM)
			}, timeout, interval).ShouldNot(BeNil())

			// Check OwnerReferences set correctly for the Config Map
			Expect(th.GetConfigMap(externalCM).ObjectMeta.OwnerReferences[0].Name).To(Equal(OVNDBClusterName.Name))
			Expect(th.GetConfigMap(externalCM).ObjectMeta.OwnerReferences[0].Kind).To(Equal("OVNDBCluster"))

			Eventually(func(g Gomega) {
				g.Expect(th.GetConfigMap(externalCM).Data["ovsdb-config"]).Should(
					ContainSubstring("ovn-remote: %s", ExpectedExternalSBEndpoint))
			}, timeout, interval).Should(Succeed())
			Eventually(func(g Gomega) {
				g.Expect(th.GetConfigMap(externalCM).Data["ovsdb-config"]).Should(
					ContainSubstring("ovn-encap-type: %s", ExpectedEncapType))
			}, timeout, interval).Should(Succeed())
			Eventually(func(g Gomega) {
				g.Expect(th.GetConfigMap(externalCM).Data["ovsdb-config"]).Should(
					ContainSubstring("ovn-encap-tos: %s", ExpectedEncapTos))
			}, timeout, interval).Should(Succeed())

			// This should trigger an OVNDBCluster reconcile and update config map
			// without ovn-encap-type and ovn-encap-tos
			DeleteOVNController(types.NamespacedName{Name: ovnController.GetName(), Namespace: namespace})

			Eventually(func() corev1.ConfigMap {
				return *th.GetConfigMap(externalCM)
			}, timeout, interval).ShouldNot(BeNil())
			Eventually(func(g Gomega) {
				g.Expect(th.GetConfigMap(externalCM).Data["ovsdb-config"]).Should(
					ContainSubstring("ovn-remote: %s", ExpectedExternalSBEndpoint))
			}, timeout, interval).Should(Succeed())
			Eventually(func(g Gomega) {
				g.Expect(th.GetConfigMap(externalCM).Data["ovsdb-config"]).ShouldNot(
					ContainSubstring("ovn-encap-type: %s", ExpectedEncapType))
			}, timeout, interval).Should(Succeed())
			Eventually(func(g Gomega) {
				g.Expect(th.GetConfigMap(externalCM).Data["ovsdb-config"]).ShouldNot(
					ContainSubstring("ovn-encap-tos: %s", ExpectedEncapTos))
			}, timeout, interval).Should(Succeed())
		})

		It("should delete an external ConfigMap once SB DBCluster service override is removed", func() {
			statefulSetName := types.NamespacedName{
				Namespace: namespace,
				Name:      "ovsdbserver-sb",
			}
			th.SimulateStatefulSetReplicaReadyWithPods(
				statefulSetName,
				map[string][]string{},
			)
			th.SimulateLoadBalancerServiceIP(types.NamespacedName{Namespace: namespace, Name: "ovsdbserver-sb"})

			externalCM := types.NamespacedName{
				Namespace: OVNDBClusterName.Namespace,
				Name:      "ovncontroller-config",
			}

			// Should exist externalCM
			Eventually(func() corev1.ConfigMap {
				return *th.GetConfigMap(externalCM)
			}, timeout, interval).ShouldNot(BeNil())

			// Detach SBCluster from NAD
			Eventually(func(g Gomega) {
				ovndbcluster := GetOVNDBCluster(OVNDBClusterName)
				ovndbcluster.Spec.Override.Service = nil
				g.Expect(k8sClient.Update(ctx, ovndbcluster)).Should(Succeed())
			}, timeout, interval).Should(Succeed())
			th.AssertConfigMapDoesNotExist(externalCM)
		})
	})

	When("OVNDBCluster is created with TLS", func() {
		var OVNDBClusterName types.NamespacedName
		BeforeEach(func() {
			spec := GetTLSOVNDBClusterSpec()
			spec.NetworkAttachment = "internalapi"
			spec.DBType = ovnv1.SBDBType
			instance := CreateOVNDBCluster(namespace, spec)
			OVNDBClusterName = types.NamespacedName{Name: instance.GetName(), Namespace: instance.GetNamespace()}
			DeferCleanup(th.DeleteInstance, instance)
			internalAPINADName := types.NamespacedName{Namespace: namespace, Name: "internalapi"}
			nad := th.CreateNetworkAttachmentDefinition(internalAPINADName)
			DeferCleanup(th.DeleteInstance, nad)
		})

		It("reports that the CA secret is missing", func() {
			th.ExpectConditionWithDetails(
				OVNDBClusterName,
				ConditionGetterFunc(OVNDBClusterConditionGetter),
				condition.TLSInputReadyCondition,
				corev1.ConditionFalse,
				condition.RequestedReason,
				fmt.Sprintf("TLSInput is missing: %s", CABundleSecretName),
			)
			th.ExpectCondition(
				OVNDBClusterName,
				ConditionGetterFunc(OVNDBClusterConditionGetter),
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
				OVNDBClusterName,
				ConditionGetterFunc(OVNDBClusterConditionGetter),
				condition.TLSInputReadyCondition,
				corev1.ConditionFalse,
				condition.RequestedReason,
				fmt.Sprintf("TLSInput is missing: secrets \"%s in namespace %s\" not found",
					OvnDbCertSecretName, namespace),
			)
			th.ExpectCondition(
				OVNDBClusterName,
				ConditionGetterFunc(OVNDBClusterConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionFalse,
			)
		})

		It("creates a Statefulset with TLS certs attached", func() {
			DeferCleanup(k8sClient.Delete, ctx, th.CreateCABundleSecret(types.NamespacedName{
				Name:      CABundleSecretName,
				Namespace: namespace,
			}))
			DeferCleanup(k8sClient.Delete, ctx, th.CreateCertSecret(types.NamespacedName{
				Name:      OvnDbCertSecretName,
				Namespace: namespace,
			}))

			statefulSetName := types.NamespacedName{
				Namespace: namespace,
				Name:      "ovsdbserver-sb",
			}
			th.SimulateStatefulSetReplicaReadyWithPods(statefulSetName,
				map[string][]string{namespace + "/internalapi": {"10.0.0.1"}},
			)

			ss := th.GetStatefulSet(statefulSetName)

			//  check TLS volumes
			th.AssertVolumeExists(CABundleSecretName, ss.Spec.Template.Spec.Volumes)
			th.AssertVolumeExists("ovsdbserver-sb-tls-certs", ss.Spec.Template.Spec.Volumes)

			svcC := ss.Spec.Template.Spec.Containers[0]

			// check TLS volume mounts
			th.AssertVolumeMountExists(CABundleSecretName, "tls-ca-bundle.pem", svcC.VolumeMounts)
			th.AssertVolumeMountExists("ovsdbserver-sb-tls-certs", "tls.key", svcC.VolumeMounts)
			th.AssertVolumeMountExists("ovsdbserver-sb-tls-certs", "tls.crt", svcC.VolumeMounts)
			th.AssertVolumeMountExists("ovsdbserver-sb-tls-certs", "ca.crt", svcC.VolumeMounts)

			// check DB url schema
			Eventually(func(g Gomega) {
				OVNDBCluster := GetOVNDBCluster(OVNDBClusterName)
				g.Expect(OVNDBCluster.Status.DBAddress).To(HavePrefix("ssl:"))
				g.Expect(OVNDBCluster.Status.InternalDBAddress).To(HavePrefix("ssl:"))
			}, timeout, interval).Should(Succeed())

			// check scripts configure TLS
			scriptsCM := types.NamespacedName{
				Namespace: OVNDBClusterName.Namespace,
				Name:      fmt.Sprintf("%s-%s", OVNDBClusterName.Name, "scripts"),
			}
			Eventually(func() corev1.ConfigMap {
				return *th.GetConfigMap(scriptsCM)
			}, timeout, interval).ShouldNot(BeNil())

			Expect(th.GetConfigMap(scriptsCM).Data["setup.sh"]).Should(
				ContainSubstring("DB_SCHEME=\"pssl\""))
			Expect(th.GetConfigMap(scriptsCM).Data["setup.sh"]).Should(And(
				ContainSubstring("-db-ssl-key="),
				ContainSubstring("-db-ssl-cert="),
				ContainSubstring("-db-ssl-ca-cert="),
				ContainSubstring("-cluster-remote-proto=ssl"),
			))

			th.ExpectCondition(
				OVNDBClusterName,
				ConditionGetterFunc(OVNDBClusterConditionGetter),
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

			statefulSetName := types.NamespacedName{
				Namespace: namespace,
				Name:      "ovsdbserver-sb",
			}
			th.SimulateStatefulSetReplicaReadyWithPods(statefulSetName,
				map[string][]string{namespace + "/internalapi": {"10.0.0.1"}},
			)

			originalHash := GetEnvVarValue(
				th.GetStatefulSet(statefulSetName).Spec.Template.Spec.Containers[0].Env,
				"CONFIG_HASH",
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
					th.GetStatefulSet(statefulSetName).Spec.Template.Spec.Containers[0].Env,
					"CONFIG_HASH",
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

			statefulSetName := types.NamespacedName{
				Namespace: namespace,
				Name:      "ovsdbserver-sb",
			}
			th.SimulateStatefulSetReplicaReadyWithPods(statefulSetName,
				map[string][]string{namespace + "/internalapi": {"10.0.0.1"}},
			)

			originalHash := GetEnvVarValue(
				th.GetStatefulSet(statefulSetName).Spec.Template.Spec.Containers[0].Env,
				"CONFIG_HASH",
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
					th.GetStatefulSet(statefulSetName).Spec.Template.Spec.Containers[0].Env,
					"CONFIG_HASH",
					"",
				)
				g.Expect(newHash).NotTo(BeEmpty())
				g.Expect(newHash).NotTo(Equal(originalHash))
			}, timeout, interval).Should(Succeed())
		})

	})

	When("OVNDB is created with topologyref", func() {
		var OVNDBClusterName types.NamespacedName
		var statefulSetName types.NamespacedName
		var ovnTopologies []types.NamespacedName
		var topologyRef, topologyRefAlt *topologyv1.TopoRef

		BeforeEach(func() {
			OVNDBClusterName = types.NamespacedName{
				Name:      "ovn-db-0",
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
				topologySpec, _ := GetSampleTopologySpec(OVNDBClusterName.Name)
				infra.CreateTopology(t, topologySpec)
			}

			spec := GetDefaultOVNDBClusterSpec()
			spec.TopologyRef = topologyRef
			ovn.CreateOVNDBCluster(&OVNDBClusterName.Name, OVNDBClusterName.Namespace, spec)

			statefulSetName = types.NamespacedName{
				Namespace: namespace,
				Name:      "ovsdbserver-nb",
			}
			th.SimulateStatefulSetReplicaReady(statefulSetName)
		})

		It("sets topologyref in both .Status CR and resources", func() {
			Eventually(func(g Gomega) {
				tp := infra.GetTopology(types.NamespacedName{
					Name:      topologyRef.Name,
					Namespace: topologyRef.Namespace,
				})
				finalizers := tp.GetFinalizers()
				g.Expect(finalizers).To(HaveLen(1))
				ovndb := GetOVNDBCluster(OVNDBClusterName)
				g.Expect(ovndb.Status.LastAppliedTopology).NotTo(BeNil())
				g.Expect(ovndb.Status.LastAppliedTopology).To(Equal(topologyRef))
				g.Expect(finalizers).To(ContainElement(
					fmt.Sprintf("openstack.org/ovndbcluster-%s", OVNDBClusterName.Name)))
			}, timeout, interval).Should(Succeed())
			Eventually(func(g Gomega) {
				_, topologySpecObj := GetSampleTopologySpec(OVNDBClusterName.Name)
				g.Expect(th.GetStatefulSet(statefulSetName).Spec.Template.Spec.TopologySpreadConstraints).ToNot(BeNil())
				g.Expect(th.GetStatefulSet(statefulSetName).Spec.Template.Spec.TopologySpreadConstraints).To(Equal(topologySpecObj))
				g.Expect(th.GetStatefulSet(statefulSetName).Spec.Template.Spec.Affinity).To(BeNil())
			}, timeout, interval).Should(Succeed())
		})

		It("updates topology when the reference changes", func() {
			Eventually(func(g Gomega) {
				ovndb := GetOVNDBCluster(OVNDBClusterName)
				ovndb.Spec.TopologyRef.Name = ovnTopologies[1].Name
				g.Expect(k8sClient.Update(ctx, ovndb)).To(Succeed())
			}, timeout, interval).Should(Succeed())

			Eventually(func(g Gomega) {
				tp := infra.GetTopology(types.NamespacedName{
					Name:      topologyRefAlt.Name,
					Namespace: topologyRefAlt.Namespace,
				})
				finalizers := tp.GetFinalizers()
				g.Expect(finalizers).To(HaveLen(1))
				ovndb := GetOVNDBCluster(OVNDBClusterName)
				g.Expect(ovndb.Status.LastAppliedTopology).NotTo(BeNil())
				g.Expect(ovndb.Status.LastAppliedTopology).To(Equal(topologyRefAlt))
				g.Expect(finalizers).To(ContainElement(
					fmt.Sprintf("openstack.org/ovndbcluster-%s", OVNDBClusterName.Name)))
			}, timeout, interval).Should(Succeed())

			Eventually(func(g Gomega) {
				_, topologySpecObj := GetSampleTopologySpec(OVNDBClusterName.Name)
				g.Expect(th.GetStatefulSet(statefulSetName).Spec.Template.Spec.TopologySpreadConstraints).ToNot(BeNil())
				g.Expect(th.GetStatefulSet(statefulSetName).Spec.Template.Spec.TopologySpreadConstraints).To(Equal(topologySpecObj))
				g.Expect(th.GetStatefulSet(statefulSetName).Spec.Template.Spec.Affinity).To(BeNil())
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
				ovndb := GetOVNDBCluster(OVNDBClusterName)
				// Remove the TopologyRef from the existing .Spec
				ovndb.Spec.TopologyRef = nil
				g.Expect(k8sClient.Update(ctx, ovndb)).To(Succeed())
			}, timeout, interval).Should(Succeed())

			Eventually(func(g Gomega) {
				ovndb := GetOVNDBCluster(OVNDBClusterName)
				g.Expect(ovndb.Status.LastAppliedTopology).Should(BeNil())
			}, timeout, interval).Should(Succeed())

			Eventually(func(g Gomega) {
				g.Expect(th.GetStatefulSet(statefulSetName).Spec.Template.Spec.TopologySpreadConstraints).To(BeNil())
				g.Expect(th.GetStatefulSet(statefulSetName).Spec.Template.Spec.Affinity).ToNot(BeNil())
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

		It("rejects a wrong topologyRef on a different namespace", func() {
			spec := map[string]any{}
			// Inject a topologyRef that points to a different namespace
			spec["topologyRef"] = map[string]any{
				"name":      "foo",
				"namespace": "bar",
			}
			raw := map[string]any{
				"apiVersion": "ovn.openstack.org/v1beta1",
				"kind":       "OVNDBCluster",
				"metadata": map[string]any{
					"name":      "ovndbcluster-sample",
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
})
