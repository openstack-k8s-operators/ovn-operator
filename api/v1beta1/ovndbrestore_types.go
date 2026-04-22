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

package v1beta1

import (
	"github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// OVNDBRestorePhase represents the current phase of the restore operation
type OVNDBRestorePhase string

const (
	// OVNDBRestorePhaseValidating - validating backup source and cluster
	OVNDBRestorePhaseValidating OVNDBRestorePhase = "Validating"
	// OVNDBRestorePhaseScalingDown - scaling the OVNDBCluster to 0
	OVNDBRestorePhaseScalingDown OVNDBRestorePhase = "ScalingDown"
	// OVNDBRestorePhaseRestoring - running the restore Job
	OVNDBRestorePhaseRestoring OVNDBRestorePhase = "Restoring"
	// OVNDBRestorePhaseScalingUp - scaling the OVNDBCluster back up
	OVNDBRestorePhaseScalingUp OVNDBRestorePhase = "ScalingUp"
	// OVNDBRestorePhaseCompleted - restore completed successfully
	OVNDBRestorePhaseCompleted OVNDBRestorePhase = "Completed"
	// OVNDBRestorePhaseFailed - restore failed
	OVNDBRestorePhaseFailed OVNDBRestorePhase = "Failed"
)

// OVNDBRestoreSpec defines the desired state of OVNDBRestore
type OVNDBRestoreSpec struct {
	// +kubebuilder:validation:Required
	// BackupSource - Name of the OVNDBBackup CR to restore from
	BackupSource string `json:"backupSource"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Pattern=`^\d{8}-\d{6}$`
	// BackupTimestamp - specific backup timestamp to restore (format: YYYYMMDD-HHMMSS).
	// Must match the timestamp prefix of a backup file on the backup PVC.
	// If empty, the most recent backup is used.
	BackupTimestamp string `json:"backupTimestamp,omitempty"`
}

// OVNDBRestoreStatus defines the observed state of OVNDBRestore
type OVNDBRestoreStatus struct {
	// Map of hashes to track input changes
	Hash map[string]string `json:"hash,omitempty"`

	// Conditions
	Conditions condition.Conditions `json:"conditions,omitempty" optional:"true"`

	// ObservedGeneration - the most recent generation observed
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// OriginalReplicas - the replica count saved before scale-down
	OriginalReplicas *int32 `json:"originalReplicas,omitempty"`

	// Phase - current phase of the restore operation
	Phase OVNDBRestorePhase `json:"phase,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:printcolumn:name="Phase",type="string",JSONPath=".status.phase",description="Phase"
//+kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.conditions[0].status",description="Status"
//+kubebuilder:printcolumn:name="Message",type="string",JSONPath=".status.conditions[0].message",description="Message"

// OVNDBRestore is the Schema for the ovndbrestores API
type OVNDBRestore struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   OVNDBRestoreSpec   `json:"spec,omitempty"`
	Status OVNDBRestoreStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// OVNDBRestoreList contains a list of OVNDBRestore
type OVNDBRestoreList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []OVNDBRestore `json:"items"`
}

func init() {
	SchemeBuilder.Register(&OVNDBRestore{}, &OVNDBRestoreList{})
}

// RbacConditionsSet - set the conditions for the rbac object
func (instance OVNDBRestore) RbacConditionsSet(c *condition.Condition) {
	instance.Status.Conditions.Set(c)
}

// RbacNamespace - return the namespace
func (instance OVNDBRestore) RbacNamespace() string {
	return instance.Namespace
}

// RbacResourceName - return the name to be used for rbac objects (serviceaccount, role, rolebinding)
func (instance OVNDBRestore) RbacResourceName() string {
	return "ovndbrestore-" + instance.Name
}
