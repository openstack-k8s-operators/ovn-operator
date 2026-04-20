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
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

var _ = Describe("OVNDBRestore controller", func() {

	When("OVNDBRestore CR is created with non-existent OVNDBBackup", func() {
		var restoreName types.NamespacedName

		BeforeEach(func() {
			spec := GetDefaultOVNDBRestoreSpec()
			spec.BackupSource = "nonexistent-backup"
			instance := CreateOVNDBRestore(namespace, spec)
			restoreName = types.NamespacedName{Name: instance.GetName(), Namespace: instance.GetNamespace()}
			DeferCleanup(th.DeleteInstance, instance)
		})

		It("should set OVNDBBackupReady condition to False", func() {
			th.ExpectConditionWithDetails(
				restoreName,
				ConditionGetterFunc(OVNDBRestoreConditionGetter),
				ovnv1.OVNDBBackupReadyCondition,
				corev1.ConditionFalse,
				condition.RequestedReason,
				fmt.Sprintf(ovnv1.OVNDBBackupReadyErrorMessage, "OVNDBBackup nonexistent-backup not found"),
			)
		})

		It("should not be ready", func() {
			th.ExpectCondition(
				restoreName,
				ConditionGetterFunc(OVNDBRestoreConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionFalse,
			)
		})
	})

	When("OVNDBRestore CR is created with a ready OVNDBBackup", func() {
		var restoreName types.NamespacedName

		BeforeEach(func() {
			// Create OVNDBCluster
			dbSpec := GetDefaultOVNDBClusterSpec()
			dbCluster := CreateOVNDBCluster(namespace, dbSpec)
			DeferCleanup(th.DeleteInstance, dbCluster)

			// Create OVNDBBackup referencing the cluster
			backupSpec := GetDefaultOVNDBBackupSpec()
			backupSpec.DatabaseInstance = dbCluster.GetName()
			backupInstance := CreateOVNDBBackup(namespace, backupSpec)
			DeferCleanup(th.DeleteInstance, backupInstance)

			// Wait for backup to be ready
			backupNN := types.NamespacedName{Name: backupInstance.GetName(), Namespace: backupInstance.GetNamespace()}
			th.ExpectCondition(
				backupNN,
				ConditionGetterFunc(OVNDBBackupConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionTrue,
			)

			spec := ovnv1.OVNDBRestoreSpec{
				BackupSource: backupInstance.GetName(),
			}
			instance := CreateOVNDBRestore(namespace, spec)
			restoreName = types.NamespacedName{Name: instance.GetName(), Namespace: instance.GetNamespace()}
			DeferCleanup(th.DeleteInstance, instance)
		})

		It("should mark OVNDBBackupReady as True", func() {
			th.ExpectCondition(
				restoreName,
				ConditionGetterFunc(OVNDBRestoreConditionGetter),
				ovnv1.OVNDBBackupReadyCondition,
				corev1.ConditionTrue,
			)
		})

		It("should mark OVNDBClusterReady as True", func() {
			th.ExpectCondition(
				restoreName,
				ConditionGetterFunc(OVNDBRestoreConditionGetter),
				ovnv1.OVNDBClusterReadyCondition,
				corev1.ConditionTrue,
			)
		})

		It("should save original replicas and move to ScalingDown phase", func() {
			Eventually(func(g Gomega) {
				restore := GetOVNDBRestore(restoreName)
				g.Expect(restore.Status.OriginalReplicas).NotTo(BeNil())
				g.Expect(restore.Status.Phase).To(BeElementOf(
					ovnv1.OVNDBRestorePhaseScalingDown,
					ovnv1.OVNDBRestorePhaseRestoring,
				))
			}, timeout, interval).Should(Succeed())
		})

		It("should add a finalizer to the backup CR", func() {
			restore := GetOVNDBRestore(restoreName)
			backupName := types.NamespacedName{
				Name:      restore.Spec.BackupSource,
				Namespace: namespace,
			}
			Eventually(func(g Gomega) {
				backup := GetOVNDBBackup(backupName)
				finalizerName := "ovn.openstack.org/restore-" + restore.Name
				g.Expect(backup.Finalizers).To(ContainElement(finalizerName))
			}, timeout, interval).Should(Succeed())
		})
	})
})
