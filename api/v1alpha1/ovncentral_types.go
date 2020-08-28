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
	"github.com/operator-framework/operator-lib/status"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Important: Run "make" to regenerate code after modifying this file

// OVNCentralServer defines the observed state of a member of cluster
type OVNCentralServerStatus struct {
	NB *OVNCentralServerDatabaseStatus `json:"nb,omitempty"`
	SB *OVNCentralServerDatabaseStatus `json:"sb,omitempty"`
}

type OVNCentralServerDatabaseStatus struct {
	ServerID     string `json:"serverID"`
	LocalAddress string `json:"localAddress"`
}

// OVNCentralSpec defines the desired state of OVNCentral
type OVNCentralSpec struct {
	Replicas         int    `json:"replicas"`
	Image            string `json:"image"`
	StorageClass     string `json:"storageClass,omitempty"`
	ConnectionConfig string `json:"connectionConfig,omitempty"`
	ConnectionCA     string `json:"connectionCA,omitempty"`
	ConnectionCert   string `json:"connectionCert,omitempty"`
	NBSchemaVersion  string `json:"nbSchemaVersion,omitempty"`
	SBSchemaVersion  string `json:"sbSchemaVersion,omitempty"`
}

// OVNCentralStatus defines the observed state of OVNCentral
type OVNCentralStatus struct {
	Conditions      status.Conditions                 `json:"conditions"`
	Replicas        int                               `json:"replicas"`
	NBClusterID     string                            `json:"nbClusterID,omitempty"`
	SBClusterID     string                            `json:"sbClusterID,omitempty"`
	NBSchemaVersion string                            `json:"nbSchemaVersion,omitEmpty"`
	SBSchemaVersion string                            `json:"sbSchemaVersion,omitEmpty"`
	Servers         map[string]OVNCentralServerStatus `json:"servers"`
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
