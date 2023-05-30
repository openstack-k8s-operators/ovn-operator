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

	"github.com/google/uuid"
	networkv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/openstack-k8s-operators/lib-common/modules/test/helpers"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	condition "github.com/openstack-k8s-operators/lib-common/modules/common/condition"
)

var _ = Describe("OVNNorthd controller", func() {

	When("A OVNNorthd instance is created", func() {
		var OVNNorthdName types.NamespacedName
		BeforeEach(func() {
			name := fmt.Sprintf("ovnnorthd-%s", uuid.New().String())
			instance := CreateOVNNorthd(namespace, name, GetDefaultOVNNorthdSpec())
			OVNNorthdName = types.NamespacedName{Name: instance.GetName(), Namespace: instance.GetNamespace()}
			DeferCleanup(th.DeleteInstance, instance)
		})

		It("should have the Spec fields initialized", func() {
			OVNNorthd := GetOVNNorthd(OVNNorthdName)
			Expect(OVNNorthd.Spec.Replicas).Should(Equal(int32(1)))
		})

		It("should have the Status fields initialized", func() {
			OVNNorthd := GetOVNNorthd(OVNNorthdName)
			Expect(OVNNorthd.Status.Hash).To(BeEmpty())
			Expect(OVNNorthd.Status.ReadyCount).To(Equal(int32(0)))
		})

		It("should have a finalizer", func() {
			// the reconciler loop adds the finalizer so we have to wait for
			// it to run
			Eventually(func() []string {
				return GetOVNNorthd(OVNNorthdName).Finalizers
			}, timeout, interval).Should(ContainElement("OVNNorthd"))
		})

		It("should not create a config map", func() {
			Eventually(func() []corev1.ConfigMap {
				return th.ListConfigMaps(fmt.Sprintf("%s-%s", OVNNorthdName.Name, "config-data")).Items
			}, timeout, interval).Should(BeEmpty())
		})

		It("should be in input ready condition", func() {
			th.ExpectCondition(
				OVNNorthdName,
				ConditionGetterFunc(OVNNorthdConditionGetter),
				condition.InputReadyCondition,
				corev1.ConditionTrue,
			)
		})

		When("OVNDBCluster instance is not available", func() {
			It("should not create a config map", func() {
				Eventually(func() []corev1.ConfigMap {
					return th.ListConfigMaps(fmt.Sprintf("%s-%s", OVNNorthdName.Name, "config-data")).Items
				}, timeout, interval).Should(BeEmpty())
			})
			It("should not set ServiceConfigReadyCondition condition", func() {
				th.ExpectCondition(
					OVNNorthdName,
					ConditionGetterFunc(OVNNorthdConditionGetter),
					condition.ServiceConfigReadyCondition,
					corev1.ConditionFalse,
				)
			})
		})

		When("OVNDBCluster instances are available", func() {
			It("should create a ConfigMap for ovn-northd.json with the ovn connection config option set based on the OVNDBCluster", func() {
				dbs := CreateOVNDBClusters(namespace)
				DeferCleanup(DeleteOVNDBClusters, dbs)
				configataCM := types.NamespacedName{
					Namespace: OVNNorthdName.Namespace,
					Name:      fmt.Sprintf("%s-%s", OVNNorthdName.Name, "config-data"),
				}

				Eventually(func() corev1.ConfigMap {
					return *th.GetConfigMap(configataCM)
				}, timeout, interval).ShouldNot(BeNil())
				for _, db := range dbs {
					ovndb := GetOVNDBCluster(db)
					Expect(th.GetConfigMap(configataCM).Data["ovn-northd.json"]).Should(
						ContainSubstring("ovn%s-db=%s", strings.ToLower(string(ovndb.Spec.DBType)), ovndb.Status.DBAddress))
				}

				th.ExpectCondition(
					OVNNorthdName,
					ConditionGetterFunc(OVNNorthdConditionGetter),
					condition.ServiceConfigReadyCondition,
					corev1.ConditionTrue,
				)
			})
		})

		When("OVNNorthd CR is deleted", func() {
			It("removes the Config MAP", func() {
				DeferCleanup(DeleteOVNDBClusters, CreateOVNDBClusters(namespace))
				configataCM := types.NamespacedName{
					Namespace: OVNNorthdName.Namespace,
					Name:      fmt.Sprintf("%s-%s", OVNNorthdName.Name, "config-data"),
				}

				Eventually(func() corev1.ConfigMap {
					return *th.GetConfigMap(configataCM)
				}, timeout, interval).ShouldNot(BeNil())

				th.DeleteInstance(GetOVNNorthd(OVNNorthdName))

				Eventually(func() []corev1.ConfigMap {
					return th.ListConfigMaps(configataCM.Name).Items
				}, timeout, interval).Should(BeEmpty())
			})
		})

	})

	When("OVNNorthd is created with networkAttachments", func() {
		var OVNNorthdName types.NamespacedName
		BeforeEach(func() {
			dbs := CreateOVNDBClusters(namespace)
			DeferCleanup(DeleteOVNDBClusters, dbs)
			name := fmt.Sprintf("ovnnorthd-%s", uuid.New().String())
			spec := GetDefaultOVNNorthdSpec()
			spec["networkAttachment"] = "internalapi"
			instance := CreateOVNNorthd(namespace, name, spec)
			OVNNorthdName = types.NamespacedName{Name: instance.GetName(), Namespace: instance.GetNamespace()}
			DeferCleanup(th.DeleteInstance, instance)
		})

		It("reports that the definition is missing", func() {
			th.ExpectConditionWithDetails(
				OVNNorthdName,
				ConditionGetterFunc(OVNNorthdConditionGetter),
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
				Name:      "ovn-northd",
			}
			depl := th.GetDeployment(statefulSetName)

			expectedAnnotation, err := json.Marshal(
				[]networkv1.NetworkSelectionElement{
					{
						Name:             "internalapi",
						Namespace:        namespace,
						InterfaceRequest: "internalapi",
					}})
			Expect(err).ShouldNot(HaveOccurred())
			Expect(depl.Spec.Template.ObjectMeta.Annotations).To(
				HaveKeyWithValue("k8s.v1.cni.cncf.io/networks", string(expectedAnnotation)),
			)

			// We don't add network attachment status annotations to the Pods
			// to simulate that the network attachments are missing.
			//SimulateDeploymentReplicaReadyWithPods(statefulSetName, map[string][]string{})

			th.ExpectConditionWithDetails(
				OVNNorthdName,
				ConditionGetterFunc(OVNNorthdConditionGetter),
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
				Name:      "ovn-northd",
			}
			depl := th.GetDeployment(statefulSetName)

			expectedAnnotation, err := json.Marshal(
				[]networkv1.NetworkSelectionElement{
					{
						Name:             "internalapi",
						Namespace:        namespace,
						InterfaceRequest: "internalapi",
					}})
			Expect(err).ShouldNot(HaveOccurred())
			Expect(depl.Spec.Template.ObjectMeta.Annotations).To(
				HaveKeyWithValue("k8s.v1.cni.cncf.io/networks", string(expectedAnnotation)),
			)

			// We simulate that there is no IP associated with the internalapi
			// network attachment
			th.SimulateDeploymentReadyWithPods(
				statefulSetName,
				map[string][]string{namespace + "/internalapi": {}},
			)

			th.ExpectConditionWithDetails(
				OVNNorthdName,
				ConditionGetterFunc(OVNNorthdConditionGetter),
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
				Name:      "ovn-northd",
			}
			th.SimulateDeploymentReadyWithPods(
				statefulSetName,
				map[string][]string{namespace + "/internalapi": {"10.0.0.1"}},
			)

			th.ExpectCondition(
				OVNNorthdName,
				ConditionGetterFunc(OVNNorthdConditionGetter),
				condition.NetworkAttachmentsReadyCondition,
				corev1.ConditionTrue,
			)

			Eventually(func(g Gomega) {
				OVNNorthd := GetOVNNorthd(OVNNorthdName)
				g.Expect(OVNNorthd.Status.NetworkAttachments).To(
					Equal(map[string][]string{namespace + "/internalapi": {"10.0.0.1"}}))

			}, timeout, interval).Should(Succeed())
		})
	})

})
