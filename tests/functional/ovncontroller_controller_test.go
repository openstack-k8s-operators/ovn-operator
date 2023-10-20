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
	. "github.com/openstack-k8s-operators/lib-common/modules/common/test/helpers"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	condition "github.com/openstack-k8s-operators/lib-common/modules/common/condition"
)

var _ = Describe("OVNController controller", func() {

	When("A OVNController instance is created", func() {
		var OVNControllerName types.NamespacedName
		BeforeEach(func() {
			name := fmt.Sprintf("ovn-controller-%s", uuid.New().String())
			instance := CreateOVNController(namespace, name, GetDefaultOVNControllerSpec())
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
			}, timeout, interval).Should(ContainElement("OVNController"))
		})

		It("should create a ConfigMap for net_setup.sh with eth0 as Interface Name", func() {
			scriptsCM := types.NamespacedName{
				Namespace: OVNControllerName.Namespace,
				Name:      fmt.Sprintf("%s-%s", OVNControllerName.Name, "scripts"),
			}
			Eventually(func() corev1.ConfigMap {
				return *th.GetConfigMap(scriptsCM)
			}, timeout, interval).ShouldNot(BeNil())

			Expect(th.GetConfigMap(scriptsCM).Data["net_setup.sh"]).Should(
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
				scriptsCM = types.NamespacedName{
					Namespace: OVNControllerName.Namespace,
					Name:      fmt.Sprintf("%s-%s", OVNControllerName.Name, "scripts"),
				}
			})

			It("should create a config job", func() {
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
			})

			It("should create a ConfigMap for net_setup.sh with eth0 as Interface Name", func() {
				Eventually(func() corev1.ConfigMap {
					return *th.GetConfigMap(scriptsCM)
				}, timeout, interval).ShouldNot(BeNil())

				Expect(th.GetConfigMap(scriptsCM).Data["net_setup.sh"]).Should(
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

	When("A OVNController instance is created with debug on", func() {
		BeforeEach(func() {
			dbs := CreateOVNDBClusters(namespace, map[string][]string{}, 1)
			DeferCleanup(DeleteOVNDBClusters, dbs)
			name := fmt.Sprintf("ovn-controller-%s", uuid.New().String())
			spec := GetDefaultOVNControllerSpec()
			spec["debug"] = map[string]interface{}{
				"service": true,
			}
			instance := CreateOVNController(namespace, name, spec)
			DeferCleanup(th.DeleteInstance, instance)
		})

		It("Container commands to include debug commands", func() {
			dsName := types.NamespacedName{
				Namespace: namespace,
				Name:      "ovn-controller",
			}
			ds := GetDaemonSet(dsName)
			Expect(ds.Spec.Template.Spec.Containers).To(HaveLen(3))
			Expect(ds.Spec.Template.Spec.Containers[0].LivenessProbe.Exec.Command).To(
				Equal([]string{"/bin/true"}))
			Expect(ds.Spec.Template.Spec.Containers[0].Command[0]).Should(ContainSubstring("sleep infinity"))
			Expect(ds.Spec.Template.Spec.Containers[0].Lifecycle.PreStop.Exec.Command).To(
				Equal([]string{"/bin/true"}))

			Expect(ds.Spec.Template.Spec.Containers[1].LivenessProbe.Exec.Command).To(
				Equal([]string{"/bin/true"}))
			Expect(ds.Spec.Template.Spec.Containers[1].Command[0]).Should(ContainSubstring("sleep infinity"))
			Expect(ds.Spec.Template.Spec.Containers[1].Lifecycle.PreStop.Exec.Command).To(
				Equal([]string{"/bin/true"}))

			Expect(ds.Spec.Template.Spec.Containers[2].Args[0]).Should(ContainSubstring("sleep infinity"))
			Expect(ds.Spec.Template.Spec.Containers[2].Lifecycle.PreStop.Exec.Command).To(
				Equal([]string{"/bin/true"}))
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
			name := fmt.Sprintf("ovn-controller-%s", uuid.New().String())
			spec := GetDefaultOVNControllerSpec()
			spec["networkAttachment"] = "internalapi"
			instance := CreateOVNController(namespace, name, spec)
			OVNControllerName = types.NamespacedName{Name: instance.GetName(), Namespace: instance.GetNamespace()}
			DeferCleanup(th.DeleteInstance, instance)
		})

		It("should create a config job", func() {
			daemonSetName := types.NamespacedName{
				Namespace: namespace,
				Name:      "ovn-controller",
			}

			SimulateDaemonsetNumberReadyWithPods(
				daemonSetName,
				map[string][]string{namespace + "/internalapi": {"10.0.0.1"}},
			)
			configJob := types.NamespacedName{
				Namespace: OVNControllerName.Namespace,
				Name:      daemonSetName.Name + "-config",
			}
			Eventually(func() batchv1.Job {
				return *th.GetJob(configJob)
			}, timeout, interval).ShouldNot(BeNil())
		})

		It("reports that network attachment is missing", func() {

			daemonSetName := types.NamespacedName{
				Namespace: namespace,
				Name:      "ovn-controller",
			}
			ds := GetDaemonSet(daemonSetName)

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
					"not all pods have interfaces with ips as configured in NetworkAttachments: [internalapi]",
			)
		})
		It("reports that an IP is missing", func() {

			daemonSetName := types.NamespacedName{
				Namespace: namespace,
				Name:      "ovn-controller",
			}
			ds := GetDaemonSet(daemonSetName)

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
					"not all pods have interfaces with ips as configured in NetworkAttachments: [internalapi]",
			)
		})
		It("reports NetworkAttachmentsReady if the Pods got the proper annotations", func() {

			daemonSetName := types.NamespacedName{
				Namespace: namespace,
				Name:      "ovn-controller",
			}
			SimulateDaemonsetNumberReadyWithPods(
				daemonSetName,
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
		It("should create a ConfigMap for net_setup.sh with nic name as Network Attachment and OwnerReferences set", func() {

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
			Expect(th.GetConfigMap(scriptsCM).Data["net_setup.sh"]).Should(
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
			name := fmt.Sprintf("ovn-controller-%s", uuid.New().String())
			spec := GetDefaultOVNControllerSpec()
			spec["networkAttachment"] = "internalapi"
			instance := CreateOVNController(namespace, name, spec)
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
			name := fmt.Sprintf("ovn-controller-%s", uuid.New().String())
			spec := GetDefaultOVNControllerSpec()
			spec["nicMappings"] = map[string]interface{}{
				"physnet1": "enp2s0.100",
			}
			instance := CreateOVNController(namespace, name, spec)
			OVNControllerName = types.NamespacedName{Name: instance.GetName(), Namespace: instance.GetNamespace()}
			DeferCleanup(th.DeleteInstance, instance)
		})

		It("reports that the networkattachment definition is created based on nic configs", func() {
			daemonSetName := types.NamespacedName{
				Namespace: namespace,
				Name:      "ovn-controller",
			}
			ds := GetDaemonSet(daemonSetName)

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
			daemonSetName := types.NamespacedName{
				Namespace: namespace,
				Name:      "ovn-controller",
			}
			SimulateDaemonsetNumberReadyWithPods(
				daemonSetName,
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
			name := fmt.Sprintf("ovn-controller-%s", uuid.New().String())
			spec := GetDefaultOVNControllerSpec()
			spec["networkAttachment"] = "internalapi"
			spec["nicMappings"] = map[string]interface{}{
				"physnet1": "enp2s0.100",
			}
			instance := CreateOVNController(namespace, name, spec)
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
			ds := GetDaemonSet(daemonSetName)

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

	When("OVNController is created with networkAttachments and nic configs", func() {
		BeforeEach(func() {
			dbs := CreateOVNDBClusters(namespace, map[string][]string{}, 1)
			DeferCleanup(DeleteOVNDBClusters, dbs)
			name := fmt.Sprintf("ovn-controller-%s", uuid.New().String())
			spec := GetDefaultOVNControllerSpec()
			spec["networkAttachments"] = []string{"internalapi"}
			spec["nicMappings"] = map[string]interface{}{
				"physnet1": "enp2s0.100",
			}
			instance := CreateOVNController(namespace, name, spec)
			DeferCleanup(th.DeleteInstance, instance)
		})

		It("reports that daemonset has annotations for both Networkattachment and nic-configs", func() {
			internalAPINADName := types.NamespacedName{Namespace: namespace, Name: "internalapi"}
			nad := th.CreateNetworkAttachmentDefinition(internalAPINADName)
			DeferCleanup(th.DeleteInstance, nad)

			daemonSetName := types.NamespacedName{
				Namespace: namespace,
				Name:      "ovn-controller",
			}
			ds := GetDaemonSet(daemonSetName)

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

	When("OVNController is created with old networkAttachment and new networkAttachments and nic configs", func() {
		BeforeEach(func() {
			dbs := CreateOVNDBClusters(namespace, map[string][]string{}, 1)
			DeferCleanup(DeleteOVNDBClusters, dbs)
			name := fmt.Sprintf("ovn-controller-%s", uuid.New().String())
			spec := GetDefaultOVNControllerSpec()
			spec["networkAttachment"] = "tenant"
			spec["networkAttachments"] = []string{"internalapi"}
			spec["nicMappings"] = map[string]interface{}{
				"physnet1": "enp2s0.100",
			}
			instance := CreateOVNController(namespace, name, spec)
			DeferCleanup(th.DeleteInstance, instance)
		})

		It("reports that daemonset has annotations for both Networkattachment and nic-configs", func() {
			internalAPINADName := types.NamespacedName{Namespace: namespace, Name: "internalapi"}
			nad := th.CreateNetworkAttachmentDefinition(internalAPINADName)
			DeferCleanup(th.DeleteInstance, nad)
			tenantNADName := types.NamespacedName{Namespace: namespace, Name: "tenant"}
			nad = th.CreateNetworkAttachmentDefinition(tenantNADName)
			DeferCleanup(th.DeleteInstance, nad)

			daemonSetName := types.NamespacedName{
				Namespace: namespace,
				Name:      "ovn-controller",
			}
			ds := GetDaemonSet(daemonSetName)

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
					{
						Name:             "tenant",
						Namespace:        namespace,
						InterfaceRequest: "tenant",
					},
				})
			Expect(err).ShouldNot(HaveOccurred())
			Expect(ds.Spec.Template.ObjectMeta.Annotations).To(
				HaveKeyWithValue("k8s.v1.cni.cncf.io/networks", string(expectedAnnotation)),
			)
		})
	})

	When("OVNController is created with networkAttachments and nic configs", func() {
		BeforeEach(func() {
			dbs := CreateOVNDBClusters(namespace, map[string][]string{}, 1)
			DeferCleanup(DeleteOVNDBClusters, dbs)
			name := fmt.Sprintf("ovn-controller-%s", uuid.New().String())
			spec := GetDefaultOVNControllerSpec()
			spec["networkAttachments"] = []string{"internalapi", "tenant"}
			spec["nicMappings"] = map[string]interface{}{
				"physnet1": "enp2s0.100",
			}
			instance := CreateOVNController(namespace, name, spec)
			DeferCleanup(th.DeleteInstance, instance)
		})

		It("reports that daemonset has annotations for both Networkattachment and nic-configs", func() {
			internalAPINADName := types.NamespacedName{Namespace: namespace, Name: "internalapi"}
			nad := th.CreateNetworkAttachmentDefinition(internalAPINADName)
			DeferCleanup(th.DeleteInstance, nad)
			tenantNADName := types.NamespacedName{Namespace: namespace, Name: "tenant"}
			nad = th.CreateNetworkAttachmentDefinition(tenantNADName)
			DeferCleanup(th.DeleteInstance, nad)

			daemonSetName := types.NamespacedName{
				Namespace: namespace,
				Name:      "ovn-controller",
			}
			ds := GetDaemonSet(daemonSetName)

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
					{
						Name:             "tenant",
						Namespace:        namespace,
						InterfaceRequest: "tenant",
					},
				})
			Expect(err).ShouldNot(HaveOccurred())
			Expect(ds.Spec.Template.ObjectMeta.Annotations).To(
				HaveKeyWithValue("k8s.v1.cni.cncf.io/networks", string(expectedAnnotation)),
			)
		})
	})

	When("OVNController is created with old networkAttachment and new networkAttachments (shared value) and nic configs", func() {
		BeforeEach(func() {
			dbs := CreateOVNDBClusters(namespace, map[string][]string{}, 1)
			DeferCleanup(DeleteOVNDBClusters, dbs)
			name := fmt.Sprintf("ovn-controller-%s", uuid.New().String())
			spec := GetDefaultOVNControllerSpec()
			spec["networkAttachment"] = "tenant"
			spec["networkAttachments"] = []string{"internalapi", "tenant"}
			spec["nicMappings"] = map[string]interface{}{
				"physnet1": "enp2s0.100",
			}
			instance := CreateOVNController(namespace, name, spec)
			DeferCleanup(th.DeleteInstance, instance)
		})

		It("reports that daemonset has annotations for both Networkattachment  and nic-configs", func() {
			internalAPINADName := types.NamespacedName{Namespace: namespace, Name: "internalapi"}
			nad := th.CreateNetworkAttachmentDefinition(internalAPINADName)
			DeferCleanup(th.DeleteInstance, nad)
			tenantNADName := types.NamespacedName{Namespace: namespace, Name: "tenant"}
			nad = th.CreateNetworkAttachmentDefinition(tenantNADName)
			DeferCleanup(th.DeleteInstance, nad)

			daemonSetName := types.NamespacedName{
				Namespace: namespace,
				Name:      "ovn-controller",
			}
			ds := GetDaemonSet(daemonSetName)

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
					{
						Name:             "tenant",
						Namespace:        namespace,
						InterfaceRequest: "tenant",
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
			name := fmt.Sprintf("ovn-controller-%s", uuid.New().String())
			ovnControllerName = types.NamespacedName{Namespace: namespace, Name: name}

			instance := CreateOVNController(namespace, name, map[string]interface{}{})
			DeferCleanup(th.DeleteInstance, instance)
		})

		It("applies meaningful defaults", func() {
			ovnController := GetOVNController(ovnControllerName)
			Expect(ovnController.Spec.ExternalIDS.OvnEncapType).To(Equal("geneve"))
			Expect(ovnController.Spec.ExternalIDS.OvnBridge).To(Equal("br-int"))
			Expect(ovnController.Spec.ExternalIDS.SystemID).To(Equal("random"))
		})
	})
})
