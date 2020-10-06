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
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	OVSDBServerBootstrapFailed  status.ConditionReason = "BootstrapFailed"
	OVSDBServerBootstrapInvalid status.ConditionReason = "BootstrapInvalid"
	OVSDBServerInconsistent     status.ConditionReason = "InconsistentClusterID"
)

// OVSDBServerSpec defines the desired state of OVSDBServer
type OVSDBServerSpec struct {
	ClusterName string   `json:"clusterName"`
	DBType      DBType   `json:"dbType"`
	ClusterID   *string  `json:"clusterID,omitempty"`
	InitPeers   []string `json:"initPeers,omitempty"`

	StorageSize  resource.Quantity `json:"storageSize"`
	StorageClass *string           `json:"storageClass,omitempty"`
}

type DatabaseStatus struct {
	ClusterID   *string `json:"clusterID,omitempty"`
	ServerID    *string `json:"serverID,omitempty"`
	Name        *string `json:"name,omitempty"`
	RaftAddress *string `json:"raftAddress,omitempty"`
	DBAddress   *string `json:"dbAddress,omitempty"`
}

// OVSDBServerStatus defines the observed state of OVSDBServer
type OVSDBServerStatus struct {
	Conditions     status.Conditions `json:"conditions,omitempty"`
	DatabaseStatus `json:"databaseStatus,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// OVSDBServer represents the storage and network identity of an ovsdb-server in
// a raft cluster. It is the Schema for the ovsdbservers API.
type OVSDBServer struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   OVSDBServerSpec   `json:"spec,omitempty"`
	Status OVSDBServerStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// OVSDBServerList contains a list of OVSDBServer
type OVSDBServerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []OVSDBServer `json:"items"`
}

func init() {
	SchemeBuilder.Register(&OVSDBServer{}, &OVSDBServerList{})
}

// ObjectWithConditions

func (server *OVSDBServer) GetConditions() *status.Conditions {
	return &server.Status.Conditions
}
