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
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ClusterID
type ClusterID string

func (cid *ClusterID) String() string {
	return string(*cid)
}

// ServerID
type ServerID string

func (sid *ServerID) String() string {
	return string(*sid)
}

// DatabaseName
type DatabaseName string

func (name *DatabaseName) String() string {
	return string(*name)
}

// ServerAddress
type ServerAddress string

func (sa *ServerAddress) String() string {
	return string(*sa)
}

const (
	OVNServerAvailable status.ConditionType = "Available"
	OVNServerFailed    status.ConditionType = "Failed"
)

// OVNServerSpec defines the desired state of OVNServer
type OVNServerSpec struct {
	NBClusterID *ClusterID      `json:"nbClusterID,omitempty"`
	SBClusterID *ClusterID      `json:"sbClusterID,omitempty"`
	InitPeers   []ServerAddress `json:"initPeers,omitempty"`

	Image        string            `json:"image"`
	StorageSize  resource.Quantity `json:"storageSize,omitempty"`
	StorageClass *string           `json:"storageClass,omitempty"`
}

type DatabaseStatus struct {
	ClusterID ClusterID     `json:"clusterID"`
	ServerID  ServerID      `json:"serverID"`
	Name      DatabaseName  `json:"name"`
	Address   ServerAddress `json:"address"`
}

// OVNServerStatus defines the observed state of OVNServer
type OVNServerStatus struct {
	Conditions status.Conditions `json:"conditions,omitempty"`
	NBDatabase *DatabaseStatus   `json:"nbDatabase,omitEmpty"`
	SBDatabase *DatabaseStatus   `json:"sbDatabase,omitEmpty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// OVNServer is the Schema for the servers API
type OVNServer struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   OVNServerSpec   `json:"spec,omitempty"`
	Status OVNServerStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// OVNServerList contains a list of OVNServer
type OVNServerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []OVNServer `json:"items"`
}

func init() {
	SchemeBuilder.Register(&OVNServer{}, &OVNServerList{})
}

func (server *OVNServer) SetAvailable() {
	condition := status.Condition{
		Type:   OVNServerAvailable,
		Status: corev1.ConditionTrue,
	}

	server.Status.Conditions.SetCondition(condition)
}

func (server *OVNServer) IsAvailable() bool {
	return server.Status.Conditions.IsTrueFor(OVNServerAvailable)
}

func (server *OVNServer) SetFailed(reason status.ConditionReason, err error) {
	condition := status.Condition{
		Type:    OVNServerFailed,
		Status:  corev1.ConditionTrue,
		Reason:  reason,
		Message: err.Error(),
	}

	server.Status.Conditions.SetCondition(condition)
}

func (server *OVNServer) IsFailed() bool {
	return server.Status.Conditions.IsTrueFor(OVNServerFailed)
}
