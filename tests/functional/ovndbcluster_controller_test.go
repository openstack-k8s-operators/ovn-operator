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
	"strings"

	networkv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	condition "github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	. "github.com/openstack-k8s-operators/lib-common/modules/common/test/helpers"
	"github.com/openstack-k8s-operators/ovn-operator/api/v1beta1"
	ovnv1 "github.com/openstack-k8s-operators/ovn-operator/api/v1beta1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
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

					Eventually(func(g Gomega) int {
						listDns := GetDNSDataList(statefulSetName, "ovsdbserver-"+DnsName)
						return len(listDns.Items)
					}).Should(BeNumerically("==", 3))

					// Scale down to 1
					Eventually(func(g Gomega) {
						c := GetOVNDBCluster(clusterName)
						*c.Spec.Replicas = 1
						g.Expect(k8sClient.Update(ctx, c)).Should(Succeed())
					}).Should(Succeed())

					// Check if dnsdata CR is down to 1
					Eventually(func() int {
						targetedDnsOcurrences := 0
						listDns := GetDNSDataList(statefulSetName, "ovsdbserver-"+DnsName)
						// This calls CreateOVNDBClusters at the BeforeEach, which will
						// create NB and SB cluster, both belonging at the same namespace
						// GetDNSDataList will return ALL occurrences of DNSData (will include
						// the NB and the SB occurences). Since both clusters were created with 3
						// replicas and only the targeted one has been scaled down to 1 if we don't
						// filter by name the result would be 4 (3+1)
						for _, d := range listDns.Items {
							if strings.Contains(d.Name, DnsName) {
								targetedDnsOcurrences++
							}
						}
						return targetedDnsOcurrences
					}).Should(BeNumerically("==", 1))
				},
				Entry("DNS entry NB", "sb"),
				Entry("DNS entry SB", "nb"),
			)
			DescribeTable("Should update DNSData IP if pod IP changes",
				func(DNSEntryName string) {
					var clusterName types.NamespacedName
					var OVNSBDBClusterName types.NamespacedName
					var cluster *ovnv1.OVNDBCluster
					var dbs []types.NamespacedName

					_ = th.CreateNetworkAttachmentDefinition(types.NamespacedName{Namespace: namespace, Name: "internalapi"})
					dbs = CreateOVNDBClusters(namespace, map[string][]string{namespace + "/internalapi": {"10.0.0.1"}}, 3)
					OVNSBDBClusterName = types.NamespacedName{Name: dbs[1].Name, Namespace: dbs[1].Namespace}
					cluster = GetOVNDBCluster(OVNSBDBClusterName)
					clusterName = OVNSBDBClusterName
					fullDNSEntryName := DNSEntryName + "." + cluster.Namespace + ".svc"

					// Check that DNSData info has been created with correct IP (10.0.0.1)
					Eventually(func(g Gomega) {
						ip := GetDNSDataHostnameIP("ovsdbserver-sb-0", cluster.Namespace, fullDNSEntryName)
						g.Expect(ip).Should(Equal("10.0.0.1"))
					}).Should(Succeed())

					// Modify pod IP section
					pod := GetPod(types.NamespacedName{Name: "ovsdbserver-sb-0", Namespace: cluster.Namespace})
					// Create new pod NAD and add it to POD
					netStatus := []networkv1.NetworkStatus{
						networkv1.NetworkStatus{
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
						*&c.Spec.ElectionTimer += 1000
						g.Expect(k8sClient.Update(ctx, c)).Should(Succeed())
					}).Should(Succeed())

					// Check that DNSData info has been modified with correct IP (10.0.0.10)
					Eventually(func(g Gomega) {
						ip := GetDNSDataHostnameIP("ovsdbserver-sb-0", cluster.Namespace, fullDNSEntryName)
						g.Expect(ip).Should(Equal("10.0.0.10"))
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
			Expect(OVNDBCluster.Spec.DBType).Should(Equal(v1beta1.NBDBType))
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
			}, timeout, interval).Should(ContainElement("OVNDBCluster"))
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

	When("OVNDBCluster is created with networkAttachments", func() {
		var OVNDBClusterName types.NamespacedName
		BeforeEach(func() {
			spec := GetDefaultOVNDBClusterSpec()
			spec.NetworkAttachment = "internalapi"
			spec.DBType = v1beta1.SBDBType
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
				g.Expect(len(serviceListWithoutTypeLabel.Items)).To(Equal(2))
			}).Should(Succeed())

			Eventually(func(g Gomega) {
				serviceListWithClusterType := GetServicesListWithLabel(namespace, map[string]string{"type": "cluster"})
				g.Expect(len(serviceListWithClusterType.Items)).To(Equal(1))
				g.Expect(serviceListWithClusterType.Items[0].Name).To(Equal("ovsdbserver-sb-0"))
			}).Should(Succeed())

			Eventually(func(g Gomega) {
				serviceListWithHeadlessType := GetServicesListWithLabel(namespace, map[string]string{"type": "headless"})
				g.Expect(len(serviceListWithHeadlessType.Items)).To(Equal(1))
				g.Expect(serviceListWithHeadlessType.Items[0].Name).To(Equal("ovsdbserver-sb"))
			}).Should(Succeed())
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
	})
})
