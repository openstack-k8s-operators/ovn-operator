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

// OVNDBBackupSpec defines the desired state of OVNDBBackup
type OVNDBBackupSpec struct {
	// +kubebuilder:validation:Required
	// DatabaseInstance - Name of the OVNDBCluster CR to back up
	DatabaseInstance string `json:"databaseInstance"`

	// +kubebuilder:validation:Optional
	// StorageClass for the backup PVC (defaults to the OVNDBCluster's StorageClass)
	StorageClass string `json:"storageClass,omitempty"`

	// +kubebuilder:validation:Optional
	// StorageRequest for the backup PVC (defaults to the OVNDBCluster's StorageRequest)
	StorageRequest string `json:"storageRequest,omitempty"`

	// +kubebuilder:validation:Required
	// +kubebuilder:default="@daily"
	// Schedule in Cron format for periodic backups
	Schedule string `json:"schedule"`

	// +kubebuilder:validation:Optional
	// Retention - duration after which old backups are cleaned up from disk
	Retention *metav1.Duration `json:"retention,omitempty"`
}

// OVNDBBackupStatus defines the observed state of OVNDBBackup
type OVNDBBackupStatus struct {
	// Map of hashes to track input changes
	Hash map[string]string `json:"hash,omitempty"`

	// Conditions
	Conditions condition.Conditions `json:"conditions,omitempty" optional:"true"`

	// ObservedGeneration - the most recent generation observed
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.conditions[0].status",description="Status"
//+kubebuilder:printcolumn:name="Message",type="string",JSONPath=".status.conditions[0].message",description="Message"

// OVNDBBackup is the Schema for the ovndbbackups API
type OVNDBBackup struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   OVNDBBackupSpec   `json:"spec,omitempty"`
	Status OVNDBBackupStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// OVNDBBackupList contains a list of OVNDBBackup
type OVNDBBackupList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []OVNDBBackup `json:"items"`
}

func init() {
	SchemeBuilder.Register(&OVNDBBackup{}, &OVNDBBackupList{})
}

// IsReady - returns true if backup is ready
func (instance OVNDBBackup) IsReady() bool {
	return instance.Status.Conditions.IsTrue(condition.ReadyCondition)
}

// RbacConditionsSet - set the conditions for the rbac object
func (instance OVNDBBackup) RbacConditionsSet(c *condition.Condition) {
	instance.Status.Conditions.Set(c)
}

// RbacNamespace - return the namespace
func (instance OVNDBBackup) RbacNamespace() string {
	return instance.Namespace
}

// RbacResourceName - return the name to be used for rbac objects (serviceaccount, role, rolebinding)
func (instance OVNDBBackup) RbacResourceName() string {
	return "ovndbbackup-" + instance.Name
}
