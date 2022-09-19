/*
Copyright 2020 Red Hat

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

package v1alpha1

import (
	"github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Important: Run "make" to regenerate code after modifying this file

// OVNCentralSpec defines the desired state of OVNCentral
type OVNCentralSpec struct {
	// Required properties

	NBReplicas  int               `json:"nbReplicas"`
	SBReplicas  int               `json:"sbReplicas"`
	Image       string            `json:"image"`
	StorageSize resource.Quantity `json:"storageSize,omitempty"`

	// Required properties with default values

	NBClientConfig string `json:"nbClientConfig,omitempty"`
	SBClientConfig string `json:"sbClientConfig,omitempty"`

	// Optional properties

	StorageClass *string `json:"storageClass,omitempty"`
}

// OVNCentralStatus defines the observed state of OVNCentral
type OVNCentralStatus struct {
	Conditions condition.Conditions `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// OVNCentral is the Schema for the ovncentrals API
type OVNCentral struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   OVNCentralSpec   `json:"spec,omitempty"`
	Status OVNCentralStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// OVNCentralList contains a list of OVNCentral
type OVNCentralList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []OVNCentral `json:"items"`
}

func init() {
	SchemeBuilder.Register(&OVNCentral{}, &OVNCentralList{})
}

// ObjectWithConditions

func (cluster *OVNCentral) GetConditions() *condition.Conditions {
	return &cluster.Status.Conditions
}
