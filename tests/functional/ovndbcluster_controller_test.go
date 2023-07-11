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

	"github.com/google/uuid"
	networkv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	condition "github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	. "github.com/openstack-k8s-operators/lib-common/modules/common/test/helpers"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

var _ = Describe("OVNDBCluster controller", func() {

	When("A OVNDBCluster instance is created", func() {
		var OVNDBClusterName types.NamespacedName

		BeforeEach(func() {
			name := fmt.Sprintf("ovndbcluster-%s", uuid.New().String())
			instance := CreateOVNDBCluster(namespace, name, GetDefaultOVNDBClusterSpec())
			OVNDBClusterName = types.NamespacedName{Name: instance.GetName(), Namespace: instance.GetNamespace()}
			DeferCleanup(th.DeleteInstance, instance)
		})

		It("should have the Spec fields initialized", func() {
			OVNDBCluster := GetOVNDBCluster(OVNDBClusterName)
			Expect(OVNDBCluster.Spec.Replicas).Should(Equal(int32(1)))
			Expect(OVNDBCluster.Spec.LogLevel).Should(Equal("info"))
			Expect(OVNDBCluster.Spec.DBType).Should(Equal("NB"))
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
				Eventually(func() []corev1.ConfigMap {
					return th.ListConfigMaps(fmt.Sprintf("%s-%s", OVNDBClusterName.Name, cmName)).Items
				}, timeout, interval).Should(BeEmpty())
			},
			Entry("config-data CM", "config-data"),
			Entry("scripts CM", "scripts"),
		)
		DescribeTable("should eventually create the config maps",
			func(cmName string) {
				Eventually(func() corev1.ConfigMap {
					cm := types.NamespacedName{
						Namespace: OVNDBClusterName.Namespace,
						Name:      fmt.Sprintf("%s-%s", OVNDBClusterName.Name, cmName),
					}
					return *th.GetConfigMap(cm)
				}, timeout, interval).ShouldNot(BeNil())
			},
			Entry("config-data CM", "config-data"),
			Entry("scripts CM", "scripts"),
		)
		When("OVNDBCluster CR is deleted, config maps should be removed", func() {
			BeforeEach(func() {
				th.DeleteInstance(GetOVNDBCluster(OVNDBClusterName))
			})

			DescribeTable("should delete the config map",
				func(cmName string) {
					Eventually(func() []corev1.ConfigMap {
						cm := types.NamespacedName{
							Namespace: OVNDBClusterName.Namespace,
							Name:      fmt.Sprintf("%s-%s", OVNDBClusterName.Name, cmName),
						}
						return th.ListConfigMaps(cm.Name).Items
					}, timeout, interval).Should(BeEmpty())
				},
				Entry("config-data CM", "config-data"),
				Entry("scripts CM", "scripts"),
			)
		})

	})

	When("OVNDBCluster is created with networkAttachments", func() {
		var OVNDBClusterName types.NamespacedName
		BeforeEach(func() {
			name := fmt.Sprintf("ovndbcluster-%s", uuid.New().String())
			spec := GetDefaultOVNDBClusterSpec()
			spec["networkAttachment"] = "internalapi"
			spec["dbType"] = "SB"
			instance := CreateOVNDBCluster(namespace, name, spec)
			OVNDBClusterName = types.NamespacedName{Name: instance.GetName(), Namespace: instance.GetNamespace()}
			DeferCleanup(th.DeleteInstance, instance)
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
