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
	. "github.com/openstack-k8s-operators/lib-common/modules/common/test/helpers"

	networkv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	condition "github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	ovnv1 "github.com/openstack-k8s-operators/ovn-operator/api/v1beta1"
	ovn_common "github.com/openstack-k8s-operators/ovn-operator/pkg/common"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

var _ = Describe("OVNController controller", func() {

	When("A OVNController instance is created", func() {
		var OVNControllerName types.NamespacedName
		BeforeEach(func() {
			instance := CreateOVNController(namespace, GetDefaultOVNControllerSpec())
			OVNControllerName = types.NamespacedName{Name: instance.GetName(), Namespace: instance.GetNamespace()}
			DeferCleanup(th.DeleteInstance, instance)
		})

		It("should have the Spec fields initialized", func() {
			OVNController := GetOVNController(OVNControllerName)
			Expect(OVNController.Spec.OvsContainerImage).Should(Equal("quay.io/podified-antelope-centos9/openstack-ovn-base:current-podified"))
			Expect(OVNController.Spec.OvnContainerImage).Should(Equal("quay.io/podified-antelope-centos9/openstack-ovn-controller:current-podified"))
		})

		It("should have the Status fields initialized", func() {
			OVNController := GetOVNController(OVNControllerName)
			Expect(OVNController.Status.Hash).To(BeEmpty())
			Expect(OVNController.Status.NumberReady).To(Equal(int32(0)))
		})

		It("should have a finalizer", func() {
			// the reconciler loop adds the finalizer so we have to wait for
			// it to run
			Eventually(func() []string {
				return GetOVNController(OVNControllerName).Finalizers
			}, timeout, interval).Should(ContainElement("openstack.org/ovncontroller"))
		})

		It("should create a ConfigMap for start-vswitchd.sh with eth0 as Interface Name", func() {
			scriptsCM := types.NamespacedName{
				Namespace: OVNControllerName.Namespace,
				Name:      fmt.Sprintf("%s-%s", OVNControllerName.Name, "scripts"),
			}
			Eventually(func() corev1.ConfigMap {
				return *th.GetConfigMap(scriptsCM)
			}, timeout, interval).ShouldNot(BeNil())

			Expect(th.GetConfigMap(scriptsCM).Data["start-vswitchd.sh"]).Should(
				ContainSubstring("addr show dev eth0"))

			th.ExpectCondition(
				OVNControllerName,
				ConditionGetterFunc(OVNControllerConditionGetter),
				condition.ServiceConfigReadyCondition,
				corev1.ConditionTrue,
			)
		})

		It("should not create an external config map", func() {
			externalCM := types.NamespacedName{
				Namespace: OVNControllerName.Namespace,
				Name:      fmt.Sprintf("%s-%s", OVNControllerName.Name, "config"),
			}
			th.AssertConfigMapDoesNotExist(externalCM)
		})

		It("should not create a config job", func() {
			daemonSetName := types.NamespacedName{
				Namespace: namespace,
				Name:      "ovn-controller",
			}
			configJob := types.NamespacedName{
				Namespace: OVNControllerName.Namespace,
				Name:      daemonSetName.Name + "-config",
			}
			th.AssertJobDoesNotExist(configJob)

			daemonSetNameOVS := types.NamespacedName{
				Namespace: namespace,
				Name:      "ovn-controller-ovs",
			}
			configJobOVS := types.NamespacedName{
				Namespace: OVNControllerName.Namespace,
				Name:      daemonSetNameOVS.Name + "-config",
			}
			th.AssertJobDoesNotExist(configJobOVS)
		})

		// TODO(ihar) introduce a new condition for the external config?
		It("should be in input ready condition", func() {
			th.ExpectCondition(
				OVNControllerName,
				ConditionGetterFunc(OVNControllerConditionGetter),
				condition.InputReadyCondition,
				corev1.ConditionTrue,
			)
		})

		When("OVNDBCluster instances are available without networkAttachments", func() {
			var scriptsCM types.NamespacedName
			var dbs []types.NamespacedName
			BeforeEach(func() {
				dbs = CreateOVNDBClusters(namespace, map[string][]string{}, 1)
				DeferCleanup(DeleteOVNDBClusters, dbs)
				daemonSetName := types.NamespacedName{
					Namespace: namespace,
					Name:      "ovn-controller",
				}
				SimulateDaemonsetNumberReady(daemonSetName)
				daemonSetNameOVS := types.NamespacedName{
					Namespace: namespace,
					Name:      "ovn-controller-ovs",
				}
				SimulateDaemonsetNumberReady(daemonSetNameOVS)
				scriptsCM = types.NamespacedName{
					Namespace: OVNControllerName.Namespace,
					Name:      fmt.Sprintf("%s-%s", OVNControllerName.Name, "scripts"),
				}
			})

			It("should create a config job for ovn-controller and not for ovn-controller-ovs", func() {
				daemonSetName := types.NamespacedName{
					Namespace: namespace,
					Name:      "ovn-controller",
				}
				SimulateDaemonsetNumberReadyWithPods(
					daemonSetName,
					map[string][]string{},
				)
				configJob := types.NamespacedName{
					Namespace: OVNControllerName.Namespace,
					Name:      daemonSetName.Name + "-config",
				}
				Eventually(func() batchv1.Job {
					return *th.GetJob(configJob)
				}, timeout, interval).ShouldNot(BeNil())

				daemonSetNameOVS := types.NamespacedName{
					Namespace: namespace,
					Name:      "ovn-controller-ovs",
				}
				SimulateDaemonsetNumberReadyWithPods(
					daemonSetNameOVS,
					map[string][]string{},
				)
				configJobOVS := types.NamespacedName{
					Namespace: OVNControllerName.Namespace,
					Name:      daemonSetNameOVS.Name + "-config",
				}
				th.AssertJobDoesNotExist(configJobOVS)
			})

			It("should create a ConfigMap for start-vswitchd.sh with eth0 as Interface Name", func() {
				Eventually(func() corev1.ConfigMap {
					return *th.GetConfigMap(scriptsCM)
				}, timeout, interval).ShouldNot(BeNil())

				Expect(th.GetConfigMap(scriptsCM).Data["start-vswitchd.sh"]).Should(
					ContainSubstring("addr show dev eth0"))

				th.ExpectCondition(
					OVNControllerName,
					ConditionGetterFunc(OVNControllerConditionGetter),
					condition.ServiceConfigReadyCondition,
					corev1.ConditionTrue,
				)
			})

			It("should not create an external config map", func() {
				externalCM := types.NamespacedName{
					Namespace: OVNControllerName.Namespace,
					Name:      fmt.Sprintf("%s-%s", OVNControllerName.Name, "config"),
				}
				th.AssertConfigMapDoesNotExist(externalCM)
			})
		})

		When("OVNDBCluster instances with networkAttachments are available", func() {
			var configCM types.NamespacedName
			var daemonSetName types.NamespacedName
			var daemonSetNameOVS types.NamespacedName
			var dbs []types.NamespacedName
			BeforeEach(func() {
				internalAPINADName := types.NamespacedName{Namespace: namespace, Name: "internalapi"}
				nad := th.CreateNetworkAttachmentDefinition(internalAPINADName)
				DeferCleanup(th.DeleteInstance, nad)
				dbs = CreateOVNDBClusters(namespace, map[string][]string{namespace + "/internalapi": {"10.0.0.0"}}, 1)
				DeferCleanup(DeleteOVNDBClusters, dbs)
				daemonSetName = types.NamespacedName{
					Namespace: namespace,
					Name:      "ovn-controller",
				}
				SimulateDaemonsetNumberReadyWithPods(
					daemonSetName,
					map[string][]string{},
				)
				daemonSetNameOVS = types.NamespacedName{
					Namespace: namespace,
					Name:      "ovn-controller-ovs",
				}
				SimulateDaemonsetNumberReadyWithPods(
					daemonSetNameOVS,
					map[string][]string{},
				)
				configCM = types.NamespacedName{
					Namespace: OVNControllerName.Namespace,
					Name:      fmt.Sprintf("%s-%s", OVNControllerName.Name, "config"),
				}
			})

			It("should create a config job", func() {
				configJob := types.NamespacedName{
					Namespace: OVNControllerName.Namespace,
					Name:      daemonSetName.Name + "-config",
				}
				Eventually(func() batchv1.Job {
					return *th.GetJob(configJob)
				}, timeout, interval).ShouldNot(BeNil())
			})

			It("should not create a config job", func() {
				configJob := types.NamespacedName{
					Namespace: OVNControllerName.Namespace,
					Name:      daemonSetNameOVS.Name + "-config",
				}
				th.AssertJobDoesNotExist(configJob)
			})
			It("should create an external config map", func() {
				Eventually(func() corev1.ConfigMap {
					return *th.GetConfigMap(configCM)
				}, timeout, interval).ShouldNot(BeNil())
			})

			It("should delete the external config map when networkAttachment is detached from SB DB", func() {
				Eventually(func() corev1.ConfigMap {
					return *th.GetConfigMap(configCM)
				}, timeout, interval).ShouldNot(BeNil())
				Eventually(func(g Gomega) {
					ovndbcluster := GetOVNDBCluster(dbs[1])
					ovndbcluster.Spec.NetworkAttachment = ""
					g.Expect(k8sClient.Update(ctx, ovndbcluster)).Should(Succeed())
				}, timeout, interval).Should(Succeed())
				th.AssertConfigMapDoesNotExist(configCM)
			})
		})
	})

	When("OVNController and OVNDBClusters are created with networkAttachments", func() {
		var OVNControllerName types.NamespacedName
		var dbs []types.NamespacedName

		BeforeEach(func() {
			internalAPINADName := types.NamespacedName{Namespace: namespace, Name: "internalapi"}
			nad := th.CreateNetworkAttachmentDefinition(internalAPINADName)
			DeferCleanup(th.DeleteInstance, nad)
			dbs = CreateOVNDBClusters(namespace, map[string][]string{namespace + "/internalapi": {"10.0.0.1"}}, 1)
			for _, db := range dbs {
				DeferCleanup(th.DeleteInstance, GetOVNDBCluster(db))
			}
			spec := GetDefaultOVNControllerSpec()
			spec.NetworkAttachment = "internalapi"
			instance := CreateOVNController(namespace, spec)
			OVNControllerName = types.NamespacedName{Name: instance.GetName(), Namespace: instance.GetNamespace()}
			DeferCleanup(th.DeleteInstance, instance)
		})

		It("should create a config job for ovn-controller and not for ovn-controller-ovs", func() {
			daemonSetName := types.NamespacedName{
				Namespace: namespace,
				Name:      "ovn-controller",
			}

			SimulateDaemonsetNumberReadyWithPods(
				daemonSetName,
				make(map[string][]string),
			)
			daemonSetNameOVS := types.NamespacedName{
				Namespace: namespace,
				Name:      "ovn-controller-ovs",
			}
			SimulateDaemonsetNumberReadyWithPods(
				daemonSetNameOVS,
				map[string][]string{namespace + "/internalapi": {"10.0.0.1"}},
			)
			configJob := types.NamespacedName{
				Namespace: OVNControllerName.Namespace,
				Name:      daemonSetName.Name + "-config",
			}
			configJobOVS := types.NamespacedName{
				Namespace: OVNControllerName.Namespace,
				Name:      daemonSetNameOVS.Name + "-config",
			}
			Eventually(func() batchv1.Job {
				return *th.GetJob(configJob)
			}, timeout, interval).ShouldNot(BeNil())
			th.AssertJobDoesNotExist(configJobOVS)
		})
		It("reports that network attachment is missing", func() {

			daemonSetName := types.NamespacedName{
				Namespace: namespace,
				Name:      "ovn-controller",
			}
			ds := GetDaemonSet(daemonSetName)
			Expect(ds.Spec.Template.ObjectMeta.Annotations).To(BeNil())

			daemonSetName = types.NamespacedName{
				Namespace: namespace,
				Name:      "ovn-controller-ovs",
			}
			ds = GetDaemonSet(daemonSetName)

			expectedAnnotation, err := json.Marshal(
				[]networkv1.NetworkSelectionElement{
					{
						Name:             "internalapi",
						Namespace:        namespace,
						InterfaceRequest: "internalapi",
					},
				})
			Expect(err).ShouldNot(HaveOccurred())
			Expect(ds.Spec.Template.ObjectMeta.Annotations).To(
				HaveKeyWithValue("k8s.v1.cni.cncf.io/networks", string(expectedAnnotation)),
			)

			// We don't add network attachment status annotations to the Pods
			// to simulate that the network attachments are missing.
			//SimulateDaemonsetNumberReadyWithPods(daemonSetName, map[string][]string{})

			th.ExpectConditionWithDetails(
				OVNControllerName,
				ConditionGetterFunc(OVNControllerConditionGetter),
				condition.NetworkAttachmentsReadyCondition,
				corev1.ConditionFalse,
				condition.ErrorReason,
				"NetworkAttachments error occurred "+
					"not all pods have interfaces with ips as configured in NetworkAttachments: internalapi",
			)
		})
		It("reports that an IP is missing", func() {

			daemonSetName := types.NamespacedName{
				Namespace: namespace,
				Name:      "ovn-controller",
			}
			ds := GetDaemonSet(daemonSetName)

			Expect(ds.Spec.Template.ObjectMeta.Annotations).To(BeNil())

			daemonSetName = types.NamespacedName{
				Namespace: namespace,
				Name:      "ovn-controller-ovs",
			}
			ds = GetDaemonSet(daemonSetName)

			expectedAnnotation, err := json.Marshal(
				[]networkv1.NetworkSelectionElement{
					{
						Name:             "internalapi",
						Namespace:        namespace,
						InterfaceRequest: "internalapi",
					},
				})
			Expect(err).ShouldNot(HaveOccurred())
			Expect(ds.Spec.Template.ObjectMeta.Annotations).To(
				HaveKeyWithValue("k8s.v1.cni.cncf.io/networks", string(expectedAnnotation)),
			)

			// We simulate that there is no IP associated with the internalapi
			// network attachment
			SimulateDaemonsetNumberReadyWithPods(
				daemonSetName,
				map[string][]string{namespace + "/internalapi": {}},
			)

			th.ExpectConditionWithDetails(
				OVNControllerName,
				ConditionGetterFunc(OVNControllerConditionGetter),
				condition.NetworkAttachmentsReadyCondition,
				corev1.ConditionFalse,
				condition.ErrorReason,
				"NetworkAttachments error occurred "+
					"not all pods have interfaces with ips as configured in NetworkAttachments: internalapi",
			)
		})
		It("reports NetworkAttachmentsReady if the Pods got the proper annotations", func() {

			daemonSetName := types.NamespacedName{
				Namespace: namespace,
				Name:      "ovn-controller",
			}
			SimulateDaemonsetNumberReadyWithPods(
				daemonSetName,
				make(map[string][]string),
			)
			daemonSetNameOVS := types.NamespacedName{
				Namespace: namespace,
				Name:      "ovn-controller-ovs",
			}
			SimulateDaemonsetNumberReadyWithPods(
				daemonSetNameOVS,
				map[string][]string{namespace + "/internalapi": {"10.0.0.1"}},
			)

			th.ExpectCondition(
				OVNControllerName,
				ConditionGetterFunc(OVNControllerConditionGetter),
				condition.NetworkAttachmentsReadyCondition,
				corev1.ConditionTrue,
			)

			Eventually(func(g Gomega) {
				OVNController := GetOVNController(OVNControllerName)
				g.Expect(OVNController.Status.NetworkAttachments).To(
					Equal(map[string][]string{namespace + "/internalapi": {"10.0.0.1"}}))

			}, timeout, interval).Should(Succeed())
		})
		It("should create a ConfigMap for start-vswitchd.sh with nic name as Network Attachment and OwnerReferences set", func() {

			scriptsCM := types.NamespacedName{
				Namespace: OVNControllerName.Namespace,
				Name:      fmt.Sprintf("%s-%s", OVNControllerName.Name, "scripts"),
			}

			Eventually(func() corev1.ConfigMap {
				return *th.GetConfigMap(scriptsCM)
			}, timeout, interval).ShouldNot(BeNil())

			// Check OwnerReferences set correctly for the Config Map
			Expect(th.GetConfigMap(scriptsCM).ObjectMeta.OwnerReferences[0].Name).To(Equal(OVNControllerName.Name))
			Expect(th.GetConfigMap(scriptsCM).ObjectMeta.OwnerReferences[0].Kind).To(Equal("OVNController"))

			ovncontroller := GetOVNController(OVNControllerName)
			Expect(th.GetConfigMap(scriptsCM).Data["start-vswitchd.sh"]).Should(
				ContainSubstring("addr show dev %s", ovncontroller.Spec.NetworkAttachment))
		})
		It("should create an external ConfigMap with expected key-value pairs and OwnerReferences set", func() {

			externalCM := types.NamespacedName{
				Namespace: OVNControllerName.Namespace,
				Name:      fmt.Sprintf("%s-%s", OVNControllerName.Name, "config"),
			}

			daemonSetName := types.NamespacedName{
				Namespace: namespace,
				Name:      "ovn-controller",
			}
			SimulateDaemonsetNumberReadyWithPods(
				daemonSetName,
				make(map[string][]string),
			)
			daemonSetName = types.NamespacedName{
				Namespace: namespace,
				Name:      "ovn-controller-ovs",
			}
			SimulateDaemonsetNumberReadyWithPods(
				daemonSetName,
				map[string][]string{namespace + "/internalapi": {"10.0.0.1"}},
			)
			ExpectedExternalSBEndpoint := "tcp:ovsdbserver-sb." + namespace + ".svc:6642"

			Eventually(func() corev1.ConfigMap {
				return *th.GetConfigMap(externalCM)
			}, timeout, interval).ShouldNot(BeNil())

			// Check OwnerReferences set correctly for the Config Map
			Expect(th.GetConfigMap(externalCM).ObjectMeta.OwnerReferences[0].Name).To(Equal(OVNControllerName.Name))
			Expect(th.GetConfigMap(externalCM).ObjectMeta.OwnerReferences[0].Kind).To(Equal("OVNController"))

			Eventually(func(g Gomega) {
				g.Expect(th.GetConfigMap(externalCM).Data["ovsdb-config"]).Should(
					ContainSubstring("ovn-remote: %s", ExpectedExternalSBEndpoint))
			}, timeout, interval).Should(Succeed())
			Eventually(func(g Gomega) {
				g.Expect(th.GetConfigMap(externalCM).Data["ovsdb-config"]).Should(
					ContainSubstring("ovn-encap-type: %s", "geneve"))
			}, timeout, interval).Should(Succeed())
		})

		It("should delete an external ConfigMap once SB DBCluster is deleted", func() {

			externalCM := types.NamespacedName{
				Namespace: OVNControllerName.Namespace,
				Name:      fmt.Sprintf("%s-%s", OVNControllerName.Name, "config"),
			}

			daemonSetName := types.NamespacedName{
				Namespace: namespace,
				Name:      "ovn-controller",
			}
			SimulateDaemonsetNumberReadyWithPods(
				daemonSetName,
				make(map[string][]string),
			)

			daemonSetNameOVS := types.NamespacedName{
				Namespace: namespace,
				Name:      "ovn-controller-ovs",
			}
			SimulateDaemonsetNumberReadyWithPods(
				daemonSetNameOVS,
				map[string][]string{namespace + "/internalapi": {"10.0.0.1"}},
			)

			Eventually(func() corev1.ConfigMap {
				return *th.GetConfigMap(externalCM)
			}, timeout, interval).ShouldNot(BeNil())

			DeleteOVNDBClusters(dbs)
			th.AssertConfigMapDoesNotExist(externalCM)
		})

		It("should delete an external ConfigMap once SB DBCluster is detached from NAD", func() {

			externalCM := types.NamespacedName{
				Namespace: OVNControllerName.Namespace,
				Name:      fmt.Sprintf("%s-%s", OVNControllerName.Name, "config"),
			}

			daemonSetName := types.NamespacedName{
				Namespace: namespace,
				Name:      "ovn-controller",
			}
			SimulateDaemonsetNumberReadyWithPods(
				daemonSetName,
				make(map[string][]string),
			)
			daemonSetNameOVS := types.NamespacedName{
				Namespace: namespace,
				Name:      "ovn-controller-ovs",
			}
			SimulateDaemonsetNumberReadyWithPods(
				daemonSetNameOVS,
				map[string][]string{namespace + "/internalapi": {"10.0.0.1"}},
			)

			Eventually(func() corev1.ConfigMap {
				return *th.GetConfigMap(externalCM)
			}, timeout, interval).ShouldNot(BeNil())

			// Detach SBCluster from NAD
			Eventually(func(g Gomega) {
				ovndbcluster := GetOVNDBCluster(dbs[1])
				ovndbcluster.Spec.NetworkAttachment = ""
				g.Expect(k8sClient.Update(ctx, ovndbcluster)).Should(Succeed())
			}, timeout, interval).Should(Succeed())
			th.AssertConfigMapDoesNotExist(externalCM)
		})
	})

	When("OVNController is created with missing networkAttachment", func() {
		var OVNControllerName types.NamespacedName
		var dbs []types.NamespacedName

		BeforeEach(func() {
			dbs = CreateOVNDBClusters(namespace, map[string][]string{}, 1)
			for _, db := range dbs {
				DeferCleanup(th.DeleteInstance, GetOVNDBCluster(db))
			}
			spec := GetDefaultOVNControllerSpec()
			spec.NetworkAttachment = "internalapi"
			instance := CreateOVNController(namespace, spec)
			OVNControllerName = types.NamespacedName{Name: instance.GetName(), Namespace: instance.GetNamespace()}
			DeferCleanup(th.DeleteInstance, instance)
		})

		It("reports that the definition is missing", func() {
			th.ExpectConditionWithDetails(
				OVNControllerName,
				ConditionGetterFunc(OVNControllerConditionGetter),
				condition.NetworkAttachmentsReadyCondition,
				corev1.ConditionFalse,
				condition.RequestedReason,
				"NetworkAttachment resources missing: internalapi",
			)
		})
	})

	When("OVNController is created with nic configs", func() {
		var OVNControllerName types.NamespacedName
		BeforeEach(func() {
			dbs := CreateOVNDBClusters(namespace, map[string][]string{}, 1)
			DeferCleanup(DeleteOVNDBClusters, dbs)
			spec := GetDefaultOVNControllerSpec()
			spec.NicMappings = map[string]string{
				"physnet1": "enp2s0.100",
			}
			instance := CreateOVNController(namespace, spec)
			OVNControllerName = types.NamespacedName{Name: instance.GetName(), Namespace: instance.GetNamespace()}
			DeferCleanup(th.DeleteInstance, instance)
		})

		It("reports that the networkattachment definition is created with OwnerReferences set", func() {
			nad := types.NamespacedName{
				Namespace: OVNControllerName.Namespace,
				Name:      "physnet1",
			}
			// Ensure OwnerReferences set correctly for the created Network Attachment
			Eventually(func(g Gomega) {
				g.Expect(GetNAD(nad).ObjectMeta.OwnerReferences[0].Name).To(Equal(
					OVNControllerName.Name))
			}, timeout, interval).Should(Succeed())
		})

		It("reports that the networkattachment definition is updated with nicMappings update", func() {
			nad := types.NamespacedName{
				Namespace: OVNControllerName.Namespace,
				Name:      "physnet1",
			}
			// Ensure NAD exists with defined interface
			Eventually(func(g Gomega) {
				g.Expect(GetNAD(nad).Spec.Config).Should(
					ContainSubstring("enp2s0.100"))
			}, timeout, interval).Should(Succeed())

			// Update Interface in NicMappings
			Eventually(func(g Gomega) {
				ovnController := GetOVNController(OVNControllerName)
				ovnController.Spec.NicMappings = map[string]string{
					"physnet1": "enp3s0.100",
				}
				g.Expect(k8sClient.Update(ctx, ovnController)).Should(Succeed())
			}, timeout, interval).Should(Succeed())

			// Ensure OwnerReferences set correctly for the updated Network Attachment
			Eventually(func(g Gomega) {
				g.Expect(GetNAD(nad).ObjectMeta.OwnerReferences[0].Name).To(Equal(
					OVNControllerName.Name))
			}, timeout, interval).Should(Succeed())

			// Ensure Interface updated in the Network Attachment
			Eventually(func(g Gomega) {
				g.Expect(GetNAD(nad).Spec.Config).Should(
					ContainSubstring("enp3s0.100"))
			}, timeout, interval).Should(Succeed())
		})

		It("should not update the networkattachment definition created externally", func() {
			nad := types.NamespacedName{
				Namespace: OVNControllerName.Namespace,
				Name:      "physnet1",
			}
			// Ensure NAD exists with defined interface
			Eventually(func(g Gomega) {
				g.Expect(GetNAD(nad).Spec.Config).Should(
					ContainSubstring("enp2s0.100"))
			}, timeout, interval).Should(Succeed())

			extNADName := types.NamespacedName{Namespace: namespace, Name: "external"}
			nadInstance := CreateNAD(extNADName)
			DeferCleanup(th.DeleteInstance, nadInstance)

			// Update NicMappings to use existing Network Attachment
			Eventually(func(g Gomega) {
				ovnController := GetOVNController(OVNControllerName)
				ovnController.Spec.NicMappings = map[string]string{
					"external": "enp3s0.100",
				}
				g.Expect(k8sClient.Update(ctx, ovnController)).Should(Succeed())
			}, timeout, interval).Should(Succeed())

			// Ensure OwnerReferences not added to not managed Network Attachment
			Eventually(func(g Gomega) {
				g.Expect(GetNAD(extNADName).ObjectMeta.OwnerReferences).Should(BeNil())
			}, timeout, interval).Should(Succeed())

			// Ensure Interface not updated in not managed Network Attachment
			Eventually(func(g Gomega) {
				g.Expect(GetNAD(extNADName).Spec.Config).ShouldNot(
					ContainSubstring("enp3s0.100"))
			}, timeout, interval).Should(Succeed())
		})

		It("reports that the networkattachment definition is created based on nic configs", func() {
			daemonSetName := types.NamespacedName{
				Namespace: namespace,
				Name:      "ovn-controller",
			}
			daemonSetNameOVS := types.NamespacedName{
				Namespace: namespace,
				Name:      "ovn-controller-ovs",
			}

			ds := GetDaemonSet(daemonSetName)
			Expect(ds.Spec.Template.ObjectMeta.Annotations).To(BeNil())

			ds = GetDaemonSet(daemonSetNameOVS)
			expectedAnnotation, err := json.Marshal(
				[]networkv1.NetworkSelectionElement{
					{
						Name:             "physnet1",
						Namespace:        namespace,
						InterfaceRequest: "physnet1",
					},
				})
			Expect(err).ShouldNot(HaveOccurred())
			Expect(ds.Spec.Template.ObjectMeta.Annotations).To(
				HaveKeyWithValue("k8s.v1.cni.cncf.io/networks", string(expectedAnnotation)),
			)

			th.ExpectCondition(
				OVNControllerName,
				ConditionGetterFunc(OVNControllerConditionGetter),
				condition.NetworkAttachmentsReadyCondition,
				corev1.ConditionTrue,
			)
		})

		It("reports IP to not exist in Status for nic-configs", func() {
			daemonSetNameOVS := types.NamespacedName{
				Namespace: namespace,
				Name:      "ovn-controller-ovs",
			}
			SimulateDaemonsetNumberReadyWithPods(
				daemonSetNameOVS,
				map[string][]string{namespace + "/physnet1": {"10.0.0.1"}},
			)

			th.ExpectCondition(
				OVNControllerName,
				ConditionGetterFunc(OVNControllerConditionGetter),
				condition.NetworkAttachmentsReadyCondition,
				corev1.ConditionTrue,
			)

			Eventually(func(g Gomega) {
				OVNController := GetOVNController(OVNControllerName)
				g.Expect(OVNController.Status.NetworkAttachments).ToNot(
					Equal(map[string][]string{namespace + "/physnet1": {"10.0.0.1"}}))

			}, timeout, interval).Should(Succeed())
		})
	})

	When("OVNController is created with networkAttachment and nic configs", func() {
		BeforeEach(func() {
			dbs := CreateOVNDBClusters(namespace, map[string][]string{}, 1)
			DeferCleanup(DeleteOVNDBClusters, dbs)
			spec := GetDefaultOVNControllerSpec()
			spec.NetworkAttachment = "internalapi"
			spec.NicMappings = map[string]string{
				"physnet1": "enp2s0.100",
			}
			instance := CreateOVNController(namespace, spec)
			DeferCleanup(th.DeleteInstance, instance)
		})

		It("reports that daemonset have annotations for both Networkattachment and nic-configs", func() {
			internalAPINADName := types.NamespacedName{Namespace: namespace, Name: "internalapi"}
			nad := th.CreateNetworkAttachmentDefinition(internalAPINADName)
			DeferCleanup(th.DeleteInstance, nad)

			daemonSetName := types.NamespacedName{
				Namespace: namespace,
				Name:      "ovn-controller",
			}
			daemonSetNameOVS := types.NamespacedName{
				Namespace: namespace,
				Name:      "ovn-controller-ovs",
			}

			ds := GetDaemonSet(daemonSetName)
			Expect(ds.Spec.Template.ObjectMeta.Annotations).To(BeNil())

			ds = GetDaemonSet(daemonSetNameOVS)
			expectedAnnotation, err := json.Marshal(
				[]networkv1.NetworkSelectionElement{
					{
						Name:             "internalapi",
						Namespace:        namespace,
						InterfaceRequest: "internalapi",
					},
					{
						Name:             "physnet1",
						Namespace:        namespace,
						InterfaceRequest: "physnet1",
					},
				})
			Expect(err).ShouldNot(HaveOccurred())
			Expect(ds.Spec.Template.ObjectMeta.Annotations).To(
				HaveKeyWithValue("k8s.v1.cni.cncf.io/networks", string(expectedAnnotation)),
			)
		})
	})

	When("OVNController is created with empty spec", func() {
		var ovnControllerName types.NamespacedName

		BeforeEach(func() {
			dbs := CreateOVNDBClusters(namespace, map[string][]string{}, 1)
			DeferCleanup(DeleteOVNDBClusters, dbs)
			instance := CreateOVNController(namespace, ovnv1.OVNControllerSpec{})
			DeferCleanup(th.DeleteInstance, instance)

			ovnControllerName = types.NamespacedName{Name: instance.GetName(), Namespace: instance.GetNamespace()}
		})

		It("applies meaningful defaults", func() {
			ovnController := GetOVNController(ovnControllerName)
			Expect(*ovnController.Spec.ExternalIDS.EnableChassisAsGateway).To(BeTrue())
			Expect(ovnController.Spec.ExternalIDS.OvnEncapType).To(Equal("geneve"))
			Expect(ovnController.Spec.ExternalIDS.OvnBridge).To(Equal("br-int"))
			Expect(ovnController.Spec.ExternalIDS.SystemID).To(Equal("random"))
		})
	})

	When("OVNController is created with TLS", func() {
		var ovnControllerName types.NamespacedName

		BeforeEach(func() {
			dbs := CreateOVNDBClusters(namespace, map[string][]string{}, 1)
			DeferCleanup(DeleteOVNDBClusters, dbs)
			instance := CreateOVNController(namespace, GetTLSOVNControllerSpec())
			DeferCleanup(th.DeleteInstance, instance)

			ovnControllerName = types.NamespacedName{Name: instance.GetName(), Namespace: instance.GetNamespace()}
		})

		It("reports that the CA secret is missing", func() {
			th.ExpectConditionWithDetails(
				ovnControllerName,
				ConditionGetterFunc(OVNControllerConditionGetter),
				condition.TLSInputReadyCondition,
				corev1.ConditionFalse,
				condition.RequestedReason,
				fmt.Sprintf("TLSInput is missing: %s", CABundleSecretName),
			)
			th.ExpectCondition(
				ovnControllerName,
				ConditionGetterFunc(OVNControllerConditionGetter),
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
				ovnControllerName,
				ConditionGetterFunc(OVNControllerConditionGetter),
				condition.TLSInputReadyCondition,
				corev1.ConditionFalse,
				condition.RequestedReason,
				fmt.Sprintf(condition.TLSInputReadyWaitingMessage, "one or more cert secrets"),
			)
			th.ExpectCondition(
				ovnControllerName,
				ConditionGetterFunc(OVNControllerConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionFalse,
			)
		})

		It("OVS Daemonset is created with 3 containers including an init container", func() {
			DeferCleanup(k8sClient.Delete, ctx, th.CreateCABundleSecret(types.NamespacedName{
				Name:      CABundleSecretName,
				Namespace: namespace,
			}))
			DeferCleanup(k8sClient.Delete, ctx, th.CreateCertSecret(types.NamespacedName{
				Name:      OvnDbCertSecretName,
				Namespace: namespace,
			}))

			daemonSetName := types.NamespacedName{
				Namespace: namespace,
				Name:      "ovn-controller",
			}

			SimulateDaemonsetNumberReady(daemonSetName)

			daemonSetNameOVS := types.NamespacedName{
				Namespace: namespace,
				Name:      "ovn-controller-ovs",
			}

			SimulateDaemonsetNumberReady(daemonSetNameOVS)

			ds := GetDaemonSet(daemonSetNameOVS)

			Expect(ds.Spec.Template.Spec.InitContainers).To(HaveLen(1))
			Expect(ds.Spec.Template.Spec.Containers).To(HaveLen(2))
		})

		It("creates a Daemonset with TLS certs attached", func() {
			DeferCleanup(k8sClient.Delete, ctx, th.CreateCABundleSecret(types.NamespacedName{
				Name:      CABundleSecretName,
				Namespace: namespace,
			}))
			DeferCleanup(k8sClient.Delete, ctx, th.CreateCertSecret(types.NamespacedName{
				Name:      OvnDbCertSecretName,
				Namespace: namespace,
			}))

			daemonSetName := types.NamespacedName{
				Namespace: namespace,
				Name:      "ovn-controller",
			}

			SimulateDaemonsetNumberReady(daemonSetName)

			daemonSetNameOVS := types.NamespacedName{
				Namespace: namespace,
				Name:      "ovn-controller-ovs",
			}

			SimulateDaemonsetNumberReady(daemonSetNameOVS)

			ds := GetDaemonSet(daemonSetName)

			//  check TLS volumes
			th.AssertVolumeExists(CABundleSecretName, ds.Spec.Template.Spec.Volumes)
			th.AssertVolumeExists("ovn-controller-tls-certs", ds.Spec.Template.Spec.Volumes)

			svcC := ds.Spec.Template.Spec.Containers[0]

			// check TLS volume mounts
			th.AssertVolumeMountExists(CABundleSecretName, "tls-ca-bundle.pem", svcC.VolumeMounts)
			th.AssertVolumeMountExists("ovn-controller-tls-certs", "tls.key", svcC.VolumeMounts)
			th.AssertVolumeMountExists("ovn-controller-tls-certs", "tls.crt", svcC.VolumeMounts)
			th.AssertVolumeMountExists("ovn-controller-tls-certs", "ca.crt", svcC.VolumeMounts)

			// check cli args
			Expect(svcC.Args).To(And(
				ContainElement(ContainSubstring(fmt.Sprintf("--private-key=%s", ovn_common.OVNDbKeyPath))),
				ContainElement(ContainSubstring(fmt.Sprintf("--certificate=%s", ovn_common.OVNDbCertPath))),
				ContainElement(ContainSubstring(fmt.Sprintf("--ca-cert=%s", ovn_common.OVNDbCaCertPath))),
			))

			th.ExpectCondition(
				ovnControllerName,
				ConditionGetterFunc(OVNControllerConditionGetter),
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

			daemonSetName := types.NamespacedName{
				Namespace: namespace,
				Name:      "ovn-controller",
			}

			SimulateDaemonsetNumberReady(daemonSetName)

			daemonSetNameOVS := types.NamespacedName{
				Namespace: namespace,
				Name:      "ovn-controller-ovs",
			}

			SimulateDaemonsetNumberReady(daemonSetNameOVS)

			originalHash := GetEnvVarValue(
				GetDaemonSet(daemonSetName).Spec.Template.Spec.Containers[0].Env,
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
					GetDaemonSet(daemonSetName).Spec.Template.Spec.Containers[0].Env,
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

			daemonSetName := types.NamespacedName{
				Namespace: namespace,
				Name:      "ovn-controller",
			}

			SimulateDaemonsetNumberReady(daemonSetName)

			daemonSetNameOVS := types.NamespacedName{
				Namespace: namespace,
				Name:      "ovn-controller-ovs",
			}

			SimulateDaemonsetNumberReady(daemonSetNameOVS)

			originalHash := GetEnvVarValue(
				GetDaemonSet(daemonSetName).Spec.Template.Spec.Containers[0].Env,
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
					GetDaemonSet(daemonSetName).Spec.Template.Spec.Containers[0].Env,
					"CONFIG_HASH",
					"",
				)
				g.Expect(newHash).NotTo(BeEmpty())
				g.Expect(newHash).NotTo(Equal(originalHash))
			}, timeout, interval).Should(Succeed())
		})
	})
})
