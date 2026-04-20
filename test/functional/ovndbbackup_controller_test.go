/*

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
	. "github.com/openstack-k8s-operators/lib-common/modules/common/test/helpers"

	condition "github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	ovnv1 "github.com/openstack-k8s-operators/ovn-operator/api/v1beta1"
	"github.com/openstack-k8s-operators/ovn-operator/internal/ovndbbackup"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

var _ = Describe("OVNDBBackup controller", func() {

	When("OVNDBBackup CR is created with non-existent OVNDBCluster", func() {
		var backupName types.NamespacedName

		BeforeEach(func() {
			spec := GetDefaultOVNDBBackupSpec()
			spec.DatabaseInstance = "nonexistent-cluster"
			instance := CreateOVNDBBackup(namespace, spec)
			backupName = types.NamespacedName{Name: instance.GetName(), Namespace: instance.GetNamespace()}
			DeferCleanup(th.DeleteInstance, instance)
		})

		It("should set OVNDBClusterReady condition to False", func() {
			th.ExpectConditionWithDetails(
				backupName,
				ConditionGetterFunc(OVNDBBackupConditionGetter),
				ovnv1.OVNDBClusterReadyCondition,
				corev1.ConditionFalse,
				condition.RequestedReason,
				fmt.Sprintf(ovnv1.OVNDBClusterReadyErrorMessage, "OVNDBCluster nonexistent-cluster not found"),
			)
		})

		It("should not be ready", func() {
			th.ExpectCondition(
				backupName,
				ConditionGetterFunc(OVNDBBackupConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionFalse,
			)
		})
	})

	When("OVNDBBackup CR is created with an existing OVNDBCluster", func() {
		var backupName types.NamespacedName

		BeforeEach(func() {
			dbSpec := GetDefaultOVNDBClusterSpec()
			dbCluster := CreateOVNDBCluster(namespace, dbSpec)
			DeferCleanup(th.DeleteInstance, dbCluster)

			spec := GetDefaultOVNDBBackupSpec()
			spec.DatabaseInstance = dbCluster.GetName()
			instance := CreateOVNDBBackup(namespace, spec)
			backupName = types.NamespacedName{Name: instance.GetName(), Namespace: instance.GetNamespace()}
			DeferCleanup(th.DeleteInstance, instance)
		})

		It("should mark OVNDBClusterReady condition as True", func() {
			th.ExpectCondition(
				backupName,
				ConditionGetterFunc(OVNDBBackupConditionGetter),
				ovnv1.OVNDBClusterReadyCondition,
				corev1.ConditionTrue,
			)
		})

		It("should create a backup PVC", func() {
			backup := GetOVNDBBackup(backupName)
			pvcName := types.NamespacedName{
				Namespace: namespace,
				Name:      ovndbbackup.BackupPVCName(backup),
			}
			Eventually(func(g Gomega) {
				pvc := &corev1.PersistentVolumeClaim{}
				g.Expect(k8sClient.Get(ctx, pvcName, pvc)).Should(Succeed())
				g.Expect(pvc.Spec.AccessModes).To(ContainElement(corev1.ReadWriteOnce))
			}, timeout, interval).Should(Succeed())
		})

		It("should mark PersistentVolumeClaimReady condition as True", func() {
			th.ExpectCondition(
				backupName,
				ConditionGetterFunc(OVNDBBackupConditionGetter),
				ovnv1.PersistentVolumeClaimReadyCondition,
				corev1.ConditionTrue,
			)
		})

		It("should create a CronJob", func() {
			backup := GetOVNDBBackup(backupName)
			cronJobName := types.NamespacedName{
				Namespace: namespace,
				Name:      ovndbbackup.BackupCronJobName(backup),
			}
			Eventually(func(g Gomega) {
				cronJob := &batchv1.CronJob{}
				g.Expect(k8sClient.Get(ctx, cronJobName, cronJob)).Should(Succeed())
				g.Expect(cronJob.Spec.Schedule).To(Equal("@daily"))
				g.Expect(cronJob.Spec.ConcurrencyPolicy).To(Equal(batchv1.ForbidConcurrent))
			}, timeout, interval).Should(Succeed())
		})

		It("should mark CronJobReady condition as True", func() {
			th.ExpectCondition(
				backupName,
				ConditionGetterFunc(OVNDBBackupConditionGetter),
				ovnv1.CronJobReadyCondition,
				corev1.ConditionTrue,
			)
		})

		It("should become Ready when all conditions are met", func() {
			th.ExpectCondition(
				backupName,
				ConditionGetterFunc(OVNDBBackupConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionTrue,
			)
		})

		It("should create a CronJob owned by the CR", func() {
			backup := GetOVNDBBackup(backupName)
			cronJobName := types.NamespacedName{
				Namespace: namespace,
				Name:      ovndbbackup.BackupCronJobName(backup),
			}
			Eventually(func(g Gomega) {
				cronJob := &batchv1.CronJob{}
				g.Expect(k8sClient.Get(ctx, cronJobName, cronJob)).Should(Succeed())
				g.Expect(cronJob.OwnerReferences).To(HaveLen(1))
				g.Expect(cronJob.OwnerReferences[0].Name).To(Equal(backup.Name))
			}, timeout, interval).Should(Succeed())
		})

		It("should create a PVC NOT owned by the CR", func() {
			backup := GetOVNDBBackup(backupName)
			pvcName := types.NamespacedName{
				Namespace: namespace,
				Name:      ovndbbackup.BackupPVCName(backup),
			}
			Eventually(func(g Gomega) {
				pvc := &corev1.PersistentVolumeClaim{}
				g.Expect(k8sClient.Get(ctx, pvcName, pvc)).Should(Succeed())
				g.Expect(pvc.OwnerReferences).To(BeEmpty())
			}, timeout, interval).Should(Succeed())
		})
	})

	When("OVNDBBackup CR uses storage defaults from OVNDBCluster", func() {
		var backupName types.NamespacedName

		BeforeEach(func() {
			dbSpec := GetDefaultOVNDBClusterSpec()
			dbCluster := CreateOVNDBCluster(namespace, dbSpec)
			DeferCleanup(th.DeleteInstance, dbCluster)

			spec := ovnv1.OVNDBBackupSpec{
				DatabaseInstance: dbCluster.GetName(),
				Schedule:         "@daily",
			}
			instance := CreateOVNDBBackup(namespace, spec)
			backupName = types.NamespacedName{Name: instance.GetName(), Namespace: instance.GetNamespace()}
			DeferCleanup(th.DeleteInstance, instance)
		})

		It("should create a PVC using the cluster's storage settings", func() {
			backup := GetOVNDBBackup(backupName)
			pvcName := types.NamespacedName{
				Namespace: namespace,
				Name:      ovndbbackup.BackupPVCName(backup),
			}
			Eventually(func(g Gomega) {
				pvc := &corev1.PersistentVolumeClaim{}
				g.Expect(k8sClient.Get(ctx, pvcName, pvc)).Should(Succeed())
				storageReq := pvc.Spec.Resources.Requests[corev1.ResourceStorage]
				g.Expect(storageReq.String()).To(Equal("1G"))
			}, timeout, interval).Should(Succeed())
		})
	})

	When("OVNDBBackup CR is deleted", func() {
		var backupName types.NamespacedName

		BeforeEach(func() {
			dbSpec := GetDefaultOVNDBClusterSpec()
			dbCluster := CreateOVNDBCluster(namespace, dbSpec)
			DeferCleanup(th.DeleteInstance, dbCluster)

			spec := GetDefaultOVNDBBackupSpec()
			spec.DatabaseInstance = dbCluster.GetName()
			instance := CreateOVNDBBackup(namespace, spec)
			backupName = types.NamespacedName{Name: instance.GetName(), Namespace: instance.GetNamespace()}

			// Wait for it to be ready
			th.ExpectCondition(
				backupName,
				ConditionGetterFunc(OVNDBBackupConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionTrue,
			)
		})

		It("should preserve the PVC after CR deletion", func() {
			backup := GetOVNDBBackup(backupName)
			pvcName := types.NamespacedName{
				Namespace: namespace,
				Name:      ovndbbackup.BackupPVCName(backup),
			}

			th.DeleteInstance(backup)

			// PVC should still exist (not owned by CR)
			pvc := &corev1.PersistentVolumeClaim{}
			Expect(k8sClient.Get(ctx, pvcName, pvc)).Should(Succeed())
			Expect(pvc.OwnerReferences).To(BeEmpty())
		})
	})

	When("OVNDBBackup CR configmap is created", func() {
		var backupName types.NamespacedName

		BeforeEach(func() {
			dbSpec := GetDefaultOVNDBClusterSpec()
			dbCluster := CreateOVNDBCluster(namespace, dbSpec)
			DeferCleanup(th.DeleteInstance, dbCluster)

			spec := GetDefaultOVNDBBackupSpec()
			spec.DatabaseInstance = dbCluster.GetName()
			instance := CreateOVNDBBackup(namespace, spec)
			backupName = types.NamespacedName{Name: instance.GetName(), Namespace: instance.GetNamespace()}
			DeferCleanup(th.DeleteInstance, instance)
		})

		It("should create a scripts ConfigMap", func() {
			backup := GetOVNDBBackup(backupName)
			cmName := types.NamespacedName{
				Namespace: namespace,
				Name:      ovndbbackup.BackupScriptsConfigMapName(backup),
			}
			Eventually(func(g Gomega) {
				cm := &corev1.ConfigMap{}
				g.Expect(k8sClient.Get(ctx, cmName, cm)).Should(Succeed())
			}, timeout, interval).Should(Succeed())
		})

		It("should mark ServiceConfigReady as True", func() {
			th.ExpectCondition(
				backupName,
				ConditionGetterFunc(OVNDBBackupConditionGetter),
				condition.ServiceConfigReadyCondition,
				corev1.ConditionTrue,
			)
		})
	})
})
