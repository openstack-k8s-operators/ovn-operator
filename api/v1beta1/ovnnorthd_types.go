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

package v1beta1

import (
	"github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	"github.com/openstack-k8s-operators/lib-common/modules/common/tls"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// Container image fall-back defaults

	// OVNNorthdContainerImage is the fall-back container image for OVNNorthd
	OVNNorthdContainerImage = "quay.io/podified-antelope-centos9/openstack-ovn-northd:current-podified"
	// ServiceNameOVNNorthd -
	ServiceNameOVNNorthd = "ovn-northd"
)

// OVNNorthdSpec defines the desired state of OVNNorthd
type OVNNorthdSpec struct {
	// +kubebuilder:validation:Required
	// ContainerImage - Container Image URL (will be set to environmental default if empty)
	ContainerImage string `json:"containerImage"`

	OVNNorthdSpecCore `json:",inline"`
}

// OVNNorthdSpecCore -
type OVNNorthdSpecCore struct {

	// +kubebuilder:validation:Optional
	// +kubebuilder:default=1
	// +kubebuilder:validation:Maximum=32
	// +kubebuilder:validation:Minimum=0
	// Replicas of OVN Northd to run
	Replicas *int32 `json:"replicas"`

	// +kubebuilder:validation:Optional
	// NodeSelector to target subset of worker nodes running this service
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default=info
	// LogLevel - Set log level info, dbg, emer etc
	LogLevel string `json:"logLevel,omitempty"`

	// +kubebuilder:validation:Optional
	// Resources - Compute Resources required by this service (Limits/Requests).
	// https://kubernetes.io/docs/concepts/configuration/manage-resources-containers/
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`

	// +kubebuilder:validation:Optional
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	// TLS - Parameters related to TLS
	TLS tls.SimpleService `json:"tls,omitempty"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default=1
	// NThreads sets number of threads used for building logical flows
	NThreads *int32 `json:"nThreads"`
}

// OVNNorthdStatus defines the observed state of OVNNorthd
type OVNNorthdStatus struct {
	// ReadyCount of OVN Northd instances
	ReadyCount int32 `json:"readyCount,omitempty"`

	// Conditions
	Conditions condition.Conditions `json:"conditions,omitempty" optional:"true"`

	//ObservedGeneration - the most recent generation observed for this service. If the observed generation is less than the spec generation, then the controller has not processed the latest changes.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.conditions[0].status",description="Status"
//+kubebuilder:printcolumn:name="Message",type="string",JSONPath=".status.conditions[0].message",description="Message"

// OVNNorthd is the Schema for the ovnnorthds API
type OVNNorthd struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   OVNNorthdSpec   `json:"spec,omitempty"`
	Status OVNNorthdStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// OVNNorthdList contains a list of OVNNorthd
type OVNNorthdList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []OVNNorthd `json:"items"`
}

func init() {
	SchemeBuilder.Register(&OVNNorthd{}, &OVNNorthdList{})
}

// IsReady - returns true if service is ready to server requests
func (instance OVNNorthd) IsReady() bool {
	// Ready when:
	// OVNNorthd is reconciled successfully
	return instance.Status.Conditions.IsTrue(condition.ReadyCondition)
}

// RbacConditionsSet - set the conditions for the rbac object
func (instance OVNNorthd) RbacConditionsSet(c *condition.Condition) {
	instance.Status.Conditions.Set(c)
}

// RbacNamespace - return the namespace
func (instance OVNNorthd) RbacNamespace() string {
	return instance.Namespace
}

// RbacResourceName - return the name to be used for rbac objects (serviceaccount, role, rolebinding)
func (instance OVNNorthd) RbacResourceName() string {
	return "ovnnorthd-" + instance.Name
}
