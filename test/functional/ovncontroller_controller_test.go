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
	topologyv1 "github.com/openstack-k8s-operators/infra-operator/apis/topology/v1beta1"
	condition "github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	ovnv1 "github.com/openstack-k8s-operators/ovn-operator/api/v1beta1"
	ovn_common "github.com/openstack-k8s-operators/ovn-operator/internal/common"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	k8s_errors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
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

		It("should have a liveness probe configured", func() {

			daemonSetName := types.NamespacedName{
				Namespace: namespace,
				Name:      "ovn-controller",
			}
			ds := GetDaemonSet(daemonSetName)

			Expect(ds.Spec.Template.Spec.Containers[0].LivenessProbe).ShouldNot(BeNil())

			daemonSetNameOVS := types.NamespacedName{
				Namespace: namespace,
				Name:      "ovn-controller-ovs",
			}
			ds = GetDaemonSet(daemonSetNameOVS)
			Expect(ds.Spec.Template.Spec.Containers[0].LivenessProbe).ShouldNot(BeNil())
			Expect(ds.Spec.Template.Spec.Containers[1].LivenessProbe).ShouldNot(BeNil())

		})
		It("should have a readiness probe configured", func() {

			daemonSetName := types.NamespacedName{
				Namespace: namespace,
				Name:      "ovn-controller",
			}
			ds := GetDaemonSet(daemonSetName)

			Expect(ds.Spec.Template.Spec.Containers[0].ReadinessProbe).ShouldNot(BeNil())

			daemonSetNameOVS := types.NamespacedName{
				Namespace: namespace,
				Name:      "ovn-controller-ovs",
			}
			ds = GetDaemonSet(daemonSetNameOVS)
			Expect(ds.Spec.Template.Spec.Containers[0].ReadinessProbe).ShouldNot(BeNil())
			Expect(ds.Spec.Template.Spec.Containers[1].ReadinessProbe).ShouldNot(BeNil())
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

		})

		When("OVNDBCluster instances with networkAttachments are available", func() {
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
		It("should have a liveness probe configured", func() {

			daemonSetName := types.NamespacedName{
				Namespace: namespace,
				Name:      "ovn-controller",
			}
			ds := GetDaemonSet(daemonSetName)

			Expect(ds.Spec.Template.Spec.Containers[0].LivenessProbe).ShouldNot(BeNil())

			daemonSetNameOVS := types.NamespacedName{
				Namespace: namespace,
				Name:      "ovn-controller-ovs",
			}
			ds = GetDaemonSet(daemonSetNameOVS)
			Expect(ds.Spec.Template.Spec.Containers[0].LivenessProbe).ShouldNot(BeNil())
			Expect(ds.Spec.Template.Spec.Containers[1].LivenessProbe).ShouldNot(BeNil())
		})
		It("should have a readiness probe configured", func() {

			daemonSetName := types.NamespacedName{
				Namespace: namespace,
				Name:      "ovn-controller",
			}
			ds := GetDaemonSet(daemonSetName)

			Expect(ds.Spec.Template.Spec.Containers[0].ReadinessProbe).ShouldNot(BeNil())

			daemonSetNameOVS := types.NamespacedName{
				Namespace: namespace,
				Name:      "ovn-controller-ovs",
			}
			ds = GetDaemonSet(daemonSetNameOVS)
			Expect(ds.Spec.Template.Spec.Containers[0].ReadinessProbe).ShouldNot(BeNil())
			Expect(ds.Spec.Template.Spec.Containers[1].ReadinessProbe).ShouldNot(BeNil())
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
				condition.ErrorReason,
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
		It("should have a liveness probe configured", func() {

			daemonSetName := types.NamespacedName{
				Namespace: namespace,
				Name:      "ovn-controller",
			}
			ds := GetDaemonSet(daemonSetName)

			Expect(ds.Spec.Template.Spec.Containers[0].LivenessProbe).ShouldNot(BeNil())

			daemonSetNameOVS := types.NamespacedName{
				Namespace: namespace,
				Name:      "ovn-controller-ovs",
			}
			ds = GetDaemonSet(daemonSetNameOVS)
			Expect(ds.Spec.Template.Spec.Containers[0].LivenessProbe).ShouldNot(BeNil())
			Expect(ds.Spec.Template.Spec.Containers[1].LivenessProbe).ShouldNot(BeNil())
		})
		It("should have a readiness probe configured", func() {

			daemonSetName := types.NamespacedName{
				Namespace: namespace,
				Name:      "ovn-controller",
			}
			ds := GetDaemonSet(daemonSetName)

			Expect(ds.Spec.Template.Spec.Containers[0].ReadinessProbe).ShouldNot(BeNil())

			daemonSetNameOVS := types.NamespacedName{
				Namespace: namespace,
				Name:      "ovn-controller-ovs",
			}
			ds = GetDaemonSet(daemonSetNameOVS)
			Expect(ds.Spec.Template.Spec.Containers[0].ReadinessProbe).ShouldNot(BeNil())
			Expect(ds.Spec.Template.Spec.Containers[1].ReadinessProbe).ShouldNot(BeNil())
		})
	})

	When("OVNController is created with invalid nic mappings", func() {
		var OVNControllerName types.NamespacedName
		BeforeEach(func() {
			dbs := CreateOVNDBClusters(namespace, map[string][]string{}, 1)
			DeferCleanup(DeleteOVNDBClusters, dbs)
			spec := GetDefaultOVNControllerSpec()
			spec.NicMappings = map[string]string{
				"<invalid>": "<nic>",
			}
			instance := CreateOVNController(namespace, spec)
			OVNControllerName = types.NamespacedName{Name: instance.GetName(), Namespace: instance.GetNamespace()}
			DeferCleanup(th.DeleteInstance, instance)
		})

		It("reports error for invalid nicMappings", func() {
			Eventually(func(g Gomega) {
				getter := ConditionGetterFunc(OVNControllerConditionGetter)
				conditions := getter.GetConditions(OVNControllerName)
				g.Expect(conditions).NotTo(
					BeNil(), "Status.Conditions in nil")
				netCondition := conditions.Get(condition.NetworkAttachmentsReadyCondition)
				g.Expect(netCondition.Message).Should(
					ContainSubstring("a lowercase RFC 1123 subdomain must consist of lower case alphanumeric characters, '-' or '.'"))
			}, timeout, interval).Should(Succeed())
		})

		It("reports success when nicMappings updated to valid value", func() {
			// Update Interface in NicMappings
			Eventually(func(g Gomega) {
				ovnController := GetOVNController(OVNControllerName)
				ovnController.Spec.NicMappings = map[string]string{
					"validnet": "enp3s0.100",
				}
				g.Expect(k8sClient.Update(ctx, ovnController)).Should(Succeed())
			}, timeout, interval).Should(Succeed())

			nad := types.NamespacedName{
				Namespace: OVNControllerName.Namespace,
				Name:      "validnet",
			}

			// Ensure OwnerReferences set correctly for the updated Network Attachment
			Eventually(func(g Gomega) {
				g.Expect(GetNAD(nad).ObjectMeta.OwnerReferences[0].Name).To(Equal(
					OVNControllerName.Name))
			}, timeout, interval).Should(Succeed())

			// Ensure NetworkCondition ready
			th.ExpectCondition(
				OVNControllerName,
				ConditionGetterFunc(OVNControllerConditionGetter),
				condition.NetworkAttachmentsReadyCondition,
				corev1.ConditionTrue,
			)
			// Ensure Interface updated in the Network Attachment
			Eventually(func(g Gomega) {
				g.Expect(GetNAD(nad).Spec.Config).Should(
					ContainSubstring("enp3s0.100"))
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
			Expect(ds.Spec.Template.Spec.Containers[0].LivenessProbe).ShouldNot(BeNil())
			Expect(ds.Spec.Template.Spec.Containers[0].ReadinessProbe).ShouldNot(BeNil())

			ds = GetDaemonSet(daemonSetNameOVS)
			Expect(ds.Spec.Template.Spec.Containers[0].LivenessProbe).ShouldNot(BeNil())
			Expect(ds.Spec.Template.Spec.Containers[0].ReadinessProbe).ShouldNot(BeNil())
			Expect(ds.Spec.Template.Spec.Containers[1].LivenessProbe).ShouldNot(BeNil())
			Expect(ds.Spec.Template.Spec.Containers[1].ReadinessProbe).ShouldNot(BeNil())
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
			Expect(ovnController.Spec.ExternalIDS.OvnEncapType).To(Equal("geneve"))
			Expect(ovnController.Spec.ExternalIDS.OvnBridge).To(Equal("br-int"))
			Expect(ovnController.Spec.ExternalIDS.SystemID).To(Equal("random"))
		})
		It("should have a liveness probe configured", func() {

			daemonSetName := types.NamespacedName{
				Namespace: namespace,
				Name:      "ovn-controller",
			}
			ds := GetDaemonSet(daemonSetName)

			Expect(ds.Spec.Template.Spec.Containers[0].LivenessProbe).ShouldNot(BeNil())

			daemonSetNameOVS := types.NamespacedName{
				Namespace: namespace,
				Name:      "ovn-controller-ovs",
			}
			ds = GetDaemonSet(daemonSetNameOVS)
			Expect(ds.Spec.Template.Spec.Containers[0].LivenessProbe).ShouldNot(BeNil())
			Expect(ds.Spec.Template.Spec.Containers[1].LivenessProbe).ShouldNot(BeNil())
		})
		It("should have a readiness probe configured", func() {

			daemonSetName := types.NamespacedName{
				Namespace: namespace,
				Name:      "ovn-controller",
			}
			ds := GetDaemonSet(daemonSetName)

			Expect(ds.Spec.Template.Spec.Containers[0].ReadinessProbe).ShouldNot(BeNil())

			daemonSetNameOVS := types.NamespacedName{
				Namespace: namespace,
				Name:      "ovn-controller-ovs",
			}
			ds = GetDaemonSet(daemonSetNameOVS)
			Expect(ds.Spec.Template.Spec.Containers[0].ReadinessProbe).ShouldNot(BeNil())
			Expect(ds.Spec.Template.Spec.Containers[1].ReadinessProbe).ShouldNot(BeNil())
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
				condition.ErrorReason,
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
				fmt.Sprintf("TLSInput is missing: secrets \"%s in namespace %s\" not found",
					OvnDbCertSecretName, namespace),
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

			ds := GetDaemonSet(daemonSetName)
			Expect(ds.Spec.Template.Spec.Containers[0].LivenessProbe).ShouldNot(BeNil())
			Expect(ds.Spec.Template.Spec.Containers[0].ReadinessProbe).ShouldNot(BeNil())

			ds = GetDaemonSet(daemonSetNameOVS)

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

			// Create metrics certificate secret for TLS metrics
			DeferCleanup(k8sClient.Delete, ctx, th.CreateCertSecret(types.NamespacedName{
				Name:      "cert-ovn-metrics",
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

			daemonSetNameOVNMetrics := types.NamespacedName{
				Namespace: namespace,
				Name:      "ovn-controller-metrics",
			}

			SimulateDaemonsetNumberReady(daemonSetNameOVNMetrics)

			ds := GetDaemonSet(daemonSetName)
			Expect(ds.Spec.Template.Spec.Containers[0].LivenessProbe).ShouldNot(BeNil())
			Expect(ds.Spec.Template.Spec.Containers[0].ReadinessProbe).ShouldNot(BeNil())

			ds = GetDaemonSet(daemonSetName)

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
			Expect(svcC.Command).To(And(
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

	When("OVNController is created with nodeSelector", func() {
		var ovnControllerName types.NamespacedName
		var daemonSetName types.NamespacedName
		var daemonSetNameOVS types.NamespacedName

		BeforeEach(func() {
			dbs := CreateOVNDBClusters(namespace, map[string][]string{}, 1)
			DeferCleanup(DeleteOVNDBClusters, dbs)

			spec := GetDefaultOVNControllerSpec()
			nodeSelector := map[string]string{
				"foo": "bar",
			}
			spec.NodeSelector = &nodeSelector
			instance := CreateOVNController(namespace, spec)
			DeferCleanup(th.DeleteInstance, instance)

			ovnControllerName = types.NamespacedName{Name: instance.GetName(), Namespace: instance.GetNamespace()}

			daemonSetName = types.NamespacedName{
				Namespace: namespace,
				Name:      "ovn-controller",
			}

			SimulateDaemonsetNumberReady(daemonSetName)

			daemonSetNameOVS = types.NamespacedName{
				Namespace: namespace,
				Name:      "ovn-controller-ovs",
			}

			SimulateDaemonsetNumberReady(daemonSetNameOVS)
		})

		It("sets nodeSelector in resource specs", func() {
			Eventually(func(g Gomega) {
				g.Expect(GetDaemonSet(daemonSetName).Spec.Template.Spec.NodeSelector).To(Equal(map[string]string{"foo": "bar"}))
				g.Expect(GetDaemonSet(daemonSetNameOVS).Spec.Template.Spec.NodeSelector).To(Equal(map[string]string{"foo": "bar"}))
			}, timeout, interval).Should(Succeed())
		})

		It("updates nodeSelector in resource specs when changed", func() {
			Eventually(func(g Gomega) {
				g.Expect(GetDaemonSet(daemonSetName).Spec.Template.Spec.NodeSelector).To(Equal(map[string]string{"foo": "bar"}))
				g.Expect(GetDaemonSet(daemonSetNameOVS).Spec.Template.Spec.NodeSelector).To(Equal(map[string]string{"foo": "bar"}))
			}, timeout, interval).Should(Succeed())

			Eventually(func(g Gomega) {
				ovnController := GetOVNController(ovnControllerName)
				newNodeSelector := map[string]string{
					"foo2": "bar2",
				}
				ovnController.Spec.NodeSelector = &newNodeSelector
				g.Expect(k8sClient.Update(ctx, ovnController)).Should(Succeed())
			}, timeout, interval).Should(Succeed())

			Eventually(func(g Gomega) {
				g.Expect(GetDaemonSet(daemonSetName).Spec.Template.Spec.NodeSelector).To(Equal(map[string]string{"foo2": "bar2"}))
				g.Expect(GetDaemonSet(daemonSetNameOVS).Spec.Template.Spec.NodeSelector).To(Equal(map[string]string{"foo2": "bar2"}))
			}, timeout, interval).Should(Succeed())
		})

		It("removes nodeSelector from resource specs when cleared", func() {
			Eventually(func(g Gomega) {
				g.Expect(GetDaemonSet(daemonSetName).Spec.Template.Spec.NodeSelector).To(Equal(map[string]string{"foo": "bar"}))
				g.Expect(GetDaemonSet(daemonSetNameOVS).Spec.Template.Spec.NodeSelector).To(Equal(map[string]string{"foo": "bar"}))
			}, timeout, interval).Should(Succeed())

			Eventually(func(g Gomega) {
				ovnController := GetOVNController(ovnControllerName)
				emptyNodeSelector := map[string]string{}
				ovnController.Spec.NodeSelector = &emptyNodeSelector
				g.Expect(k8sClient.Update(ctx, ovnController)).Should(Succeed())
			}, timeout, interval).Should(Succeed())

			Eventually(func(g Gomega) {
				g.Expect(GetDaemonSet(daemonSetName).Spec.Template.Spec.NodeSelector).To(BeNil())
				g.Expect(GetDaemonSet(daemonSetNameOVS).Spec.Template.Spec.NodeSelector).To(BeNil())
			}, timeout, interval).Should(Succeed())
		})

		It("removes nodeSelector from resource specs when nilled", func() {
			Eventually(func(g Gomega) {
				g.Expect(GetDaemonSet(daemonSetName).Spec.Template.Spec.NodeSelector).To(Equal(map[string]string{"foo": "bar"}))
				g.Expect(GetDaemonSet(daemonSetNameOVS).Spec.Template.Spec.NodeSelector).To(Equal(map[string]string{"foo": "bar"}))
			}, timeout, interval).Should(Succeed())

			Eventually(func(g Gomega) {
				ovnController := GetOVNController(ovnControllerName)
				ovnController.Spec.NodeSelector = nil
				g.Expect(k8sClient.Update(ctx, ovnController)).Should(Succeed())
			}, timeout, interval).Should(Succeed())

			Eventually(func(g Gomega) {
				g.Expect(GetDaemonSet(daemonSetName).Spec.Template.Spec.NodeSelector).To(BeNil())
				g.Expect(GetDaemonSet(daemonSetNameOVS).Spec.Template.Spec.NodeSelector).To(BeNil())
			}, timeout, interval).Should(Succeed())
		})
	})

	When("OVNController is created with topologyref", func() {
		var ovnControllerName types.NamespacedName
		var daemonSetName types.NamespacedName
		var daemonSetNameOVS types.NamespacedName
		var ovnTopologies []types.NamespacedName
		var topologyRef, topologyRefAlt *topologyv1.TopoRef

		BeforeEach(func() {
			ovnControllerName = types.NamespacedName{
				Name:      "ovn-controller-0",
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
				topologySpec, _ := GetSampleTopologySpec(ovnControllerName.Name)
				infra.CreateTopology(t, topologySpec)
			}

			dbs := CreateOVNDBClusters(namespace, map[string][]string{}, 1)
			DeferCleanup(DeleteOVNDBClusters, dbs)

			spec := GetDefaultOVNControllerSpec()
			spec.TopologyRef = topologyRef

			//ovn.CreateOVNControllerWithName("ovn-controller-0", namespace, spec)
			ovn.CreateOVNController(&ovnControllerName.Name, namespace, spec)

			daemonSetName = types.NamespacedName{
				Namespace: namespace,
				Name:      "ovn-controller",
			}

			SimulateDaemonsetNumberReady(daemonSetName)

			daemonSetNameOVS = types.NamespacedName{
				Namespace: namespace,
				Name:      "ovn-controller-ovs",
			}

			SimulateDaemonsetNumberReady(daemonSetNameOVS)
		})

		It("sets topologyref in both .Status CR and resources", func() {
			Eventually(func(g Gomega) {
				tp := infra.GetTopology(types.NamespacedName{
					Name:      topologyRef.Name,
					Namespace: topologyRef.Namespace,
				})
				finalizers := tp.GetFinalizers()
				g.Expect(finalizers).To(HaveLen(1))
				ovn := GetOVNController(ovnControllerName)
				g.Expect(ovn.Status.LastAppliedTopology).NotTo(BeNil())
				g.Expect(ovn.Status.LastAppliedTopology).To(Equal(topologyRef))
				g.Expect(finalizers).To(ContainElement(
					fmt.Sprintf("openstack.org/ovncontroller-%s", ovnControllerName.Name)))
			}, timeout, interval).Should(Succeed())

			Eventually(func(g Gomega) {
				g.Expect(GetDaemonSet(daemonSetName).Spec.Template.Spec.TopologySpreadConstraints).To(BeNil())
				g.Expect(GetDaemonSet(daemonSetNameOVS).Spec.Template.Spec.Affinity).To(BeNil())
			}, timeout, interval).Should(Succeed())
		})

		It("updates topology when the reference changes", func() {
			Eventually(func(g Gomega) {
				ovn := GetOVNController(ovnControllerName)
				ovn.Spec.TopologyRef.Name = ovnTopologies[1].Name
				g.Expect(k8sClient.Update(ctx, ovn)).To(Succeed())
			}, timeout, interval).Should(Succeed())

			Eventually(func(g Gomega) {
				tp := infra.GetTopology(types.NamespacedName{
					Name:      topologyRefAlt.Name,
					Namespace: topologyRefAlt.Namespace,
				})
				finalizers := tp.GetFinalizers()
				g.Expect(finalizers).To(HaveLen(1))
				ovn := GetOVNController(ovnControllerName)
				g.Expect(ovn.Status.LastAppliedTopology).NotTo(BeNil())
				g.Expect(ovn.Status.LastAppliedTopology).To(Equal(topologyRefAlt))
				g.Expect(finalizers).To(ContainElement(
					fmt.Sprintf("openstack.org/ovncontroller-%s", ovnControllerName.Name)))
				g.Expect(GetDaemonSet(daemonSetName).Spec.Template.Spec.TopologySpreadConstraints).To(BeNil())
				g.Expect(GetDaemonSet(daemonSetNameOVS).Spec.Template.Spec.Affinity).To(BeNil())

				// Verify the previous referenced topology has no finalizers
				tp = infra.GetTopology(types.NamespacedName{
					Name:      topologyRef.Name,
					Namespace: topologyRef.Namespace,
				})
				finalizers = tp.GetFinalizers()
				g.Expect(finalizers).To(BeEmpty())
			}, timeout, interval).Should(Succeed())
		})
		It("removes topologyRef from the spec", func() {
			Eventually(func(g Gomega) {
				ovn := GetOVNController(ovnControllerName)
				// Remove the TopologyRef from the existing .Spec
				ovn.Spec.TopologyRef = nil
				g.Expect(k8sClient.Update(ctx, ovn)).To(Succeed())
			}, timeout, interval).Should(Succeed())

			Eventually(func(g Gomega) {
				ovn := GetOVNController(ovnControllerName)
				g.Expect(ovn.Status.LastAppliedTopology).Should(BeNil())
			}, timeout, interval).Should(Succeed())

			Eventually(func(g Gomega) {
				g.Expect(GetDaemonSet(daemonSetName).Spec.Template.Spec.TopologySpreadConstraints).To(BeNil())
				g.Expect(GetDaemonSet(daemonSetName).Spec.Template.Spec.Affinity).To(BeNil())
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
		spec := map[string]any{}
		// Inject a topologyRef that points to a different namespace
		spec["topologyRef"] = map[string]any{
			"name":      "foo",
			"namespace": "bar",
		}
		raw := map[string]any{
			"apiVersion": "ovn.openstack.org/v1beta1",
			"kind":       "OVNController",
			"metadata": map[string]any{
				"name":      "ovncontroller-sample",
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

	When("A OVNController instance with metrics enabled is created", func() {
		var OVNControllerName types.NamespacedName
		BeforeEach(func() {
			spec := GetDefaultOVNControllerSpec()
			spec.ExporterImage = "quay.io/openstack-k8s-operators/openstack-network-exporter:current-podified"
			metricsEnabled := true
			spec.MetricsEnabled = &metricsEnabled
			instance := CreateOVNController(namespace, spec)
			OVNControllerName = types.NamespacedName{Name: instance.GetName(), Namespace: instance.GetNamespace()}
			DeferCleanup(th.DeleteInstance, instance)
		})

		It("should create a metrics ConfigMap", func() {
			metricsCM := types.NamespacedName{
				Namespace: OVNControllerName.Namespace,
				Name:      fmt.Sprintf("%s-metrics-config", OVNControllerName.Name),
			}
			Eventually(func() corev1.ConfigMap {
				return *th.GetConfigMap(metricsCM)
			}, timeout, interval).ShouldNot(BeNil())

			Expect(th.GetConfigMap(metricsCM).Data["openstack-network-exporter.yaml"]).Should(
				And(
					ContainSubstring("ovs-rundir: /var/run/openvswitch"),
					ContainSubstring("ovn-rundir: /var/run/ovn"),
					ContainSubstring("collectors: ['ovn', 'vswitch', 'bridge', 'coverage', 'datapath', 'iface', 'memory']"),
				))
		})

		It("should create a metrics DaemonSet", func() {
			metricsDaemonSetName := types.NamespacedName{
				Namespace: namespace,
				Name:      "ovn-controller-metrics",
			}

			ds := GetDaemonSet(metricsDaemonSetName)
			Expect(ds.Spec.Template.Spec.Containers).To(HaveLen(1))
			Expect(ds.Spec.Template.Spec.Containers[0].Name).To(Equal("openstack-network-exporter"))
			Expect(ds.Spec.Template.Spec.Containers[0].Image).To(Equal("quay.io/openstack-k8s-operators/openstack-network-exporter:current-podified"))
		})

		It("should create a metrics Service", func() {
			metricsServiceName := types.NamespacedName{
				Namespace: namespace,
				Name:      "ovn-controller-metrics",
			}

			svc := th.GetService(metricsServiceName)
			Expect(svc.Spec.Ports).To(HaveLen(1))
			Expect(svc.Spec.Ports[0].Port).To(Equal(ovn_common.MetricsPort))
			Expect(svc.Spec.Ports[0].Name).To(Equal("metrics"))
		})
	})

	When("A OVNController instance with metrics disabled is created", func() {
		var OVNControllerName types.NamespacedName
		BeforeEach(func() {
			spec := GetDefaultOVNControllerSpec()
			spec.ExporterImage = "quay.io/openstack-k8s-operators/openstack-network-exporter:current-podified"
			metricsEnabled := false
			spec.MetricsEnabled = &metricsEnabled
			instance := CreateOVNController(namespace, spec)
			OVNControllerName = types.NamespacedName{Name: instance.GetName(), Namespace: instance.GetNamespace()}
			DeferCleanup(th.DeleteInstance, instance)
		})

		It("should not create a metrics DaemonSet", func() {
			metricsDaemonSetName := types.NamespacedName{
				Namespace: namespace,
				Name:      "ovn-controller-metrics",
			}

			Eventually(func() error {
				ds := &appsv1.DaemonSet{}
				return k8sClient.Get(ctx, metricsDaemonSetName, ds)
			}, timeout, interval).Should(MatchError(k8s_errors.IsNotFound, "metrics DaemonSet should not exist"))
		})

		It("should not create a metrics Service", func() {
			metricsServiceName := types.NamespacedName{
				Namespace: namespace,
				Name:      "ovn-controller-metrics",
			}

			Eventually(func() error {
				svc := &corev1.Service{}
				return k8sClient.Get(ctx, metricsServiceName, svc)
			}, timeout, interval).Should(MatchError(k8s_errors.IsNotFound, "metrics Service should not exist"))
		})

		It("should not create a metrics ConfigMap", func() {
			metricsCM := types.NamespacedName{
				Namespace: OVNControllerName.Namespace,
				Name:      fmt.Sprintf("%s-metrics-config", OVNControllerName.Name),
			}

			Eventually(func() error {
				cm := &corev1.ConfigMap{}
				return k8sClient.Get(ctx, metricsCM, cm)
			}, timeout, interval).Should(MatchError(k8s_errors.IsNotFound, "metrics ConfigMap should not exist"))
		})
	})

	When("A OVNController instance with metrics enabled is updated to disabled", func() {
		var OVNControllerName types.NamespacedName
		var instance client.Object
		BeforeEach(func() {
			spec := GetDefaultOVNControllerSpec()
			spec.ExporterImage = "quay.io/openstack-k8s-operators/openstack-network-exporter:current-podified"
			metricsEnabled := true
			spec.MetricsEnabled = &metricsEnabled
			instance = CreateOVNController(namespace, spec)
			OVNControllerName = types.NamespacedName{Name: instance.GetName(), Namespace: instance.GetNamespace()}
			DeferCleanup(th.DeleteInstance, instance)

			// Wait for metrics resources to be created first
			metricsDaemonSetName := types.NamespacedName{
				Namespace: namespace,
				Name:      "ovn-controller-metrics",
			}
			Eventually(func() *appsv1.DaemonSet {
				return GetDaemonSet(metricsDaemonSetName)
			}, timeout, interval).ShouldNot(BeNil())
		})

		It("should cleanup metrics resources when MetricsEnabled is set to false", func() {
			// Update the instance to disable metrics
			Eventually(func(g Gomega) {
				updatedInstance := &ovnv1.OVNController{}
				g.Expect(k8sClient.Get(ctx, OVNControllerName, updatedInstance)).Should(Succeed())
				metricsEnabled := false
				updatedInstance.Spec.MetricsEnabled = &metricsEnabled
				g.Expect(k8sClient.Update(ctx, updatedInstance)).Should(Succeed())
			}, timeout, interval).Should(Succeed())

			// Verify that metrics DaemonSet is deleted
			metricsDaemonSetName := types.NamespacedName{
				Namespace: namespace,
				Name:      "ovn-controller-metrics",
			}
			Eventually(func() error {
				ds := &appsv1.DaemonSet{}
				return k8sClient.Get(ctx, metricsDaemonSetName, ds)
			}, timeout, interval).Should(MatchError(k8s_errors.IsNotFound, "metrics DaemonSet should be deleted"))

			// Verify that metrics Service is deleted
			metricsServiceName := types.NamespacedName{
				Namespace: namespace,
				Name:      "ovn-controller-metrics",
			}
			Eventually(func() error {
				svc := &corev1.Service{}
				return k8sClient.Get(ctx, metricsServiceName, svc)
			}, timeout, interval).Should(MatchError(k8s_errors.IsNotFound, "metrics Service should be deleted"))

			// Verify that metrics ConfigMap is deleted
			metricsCM := types.NamespacedName{
				Namespace: OVNControllerName.Namespace,
				Name:      fmt.Sprintf("%s-metrics-config", OVNControllerName.Name),
			}
			Eventually(func() error {
				cm := &corev1.ConfigMap{}
				return k8sClient.Get(ctx, metricsCM, cm)
			}, timeout, interval).Should(MatchError(k8s_errors.IsNotFound, "metrics ConfigMap should be deleted"))
		})
	})
})
