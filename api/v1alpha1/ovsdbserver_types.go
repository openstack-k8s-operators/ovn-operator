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

const (
	DBTypeNB = "NB"
	DBTypeSB = "SB"
)

const (
	OVSDBServerAvailable status.ConditionType = "Available"
	OVSDBServerFailed    status.ConditionType = "Failed"
)

// OVSDBServerSpec defines the desired state of OVSDBServer
type OVSDBServerSpec struct {
	ClusterID *string  `json:"sbClusterID,omitempty"`
	InitPeers []string `json:"initPeers,omitempty"`
	DBType    string   `json:"dbType,omitempty"`

	Image        string            `json:"image"`
	StorageSize  resource.Quantity `json:"storageSize,omitempty"`
	StorageClass *string           `json:"storageClass,omitempty"`
}

type DatabaseStatus struct {
	ClusterID   string `json:"clusterID,omitempty"`
	ServerID    string `json:"serverID,omitempty"`
	Name        string `json:"name,omitempty"`
	RaftAddress string `json:"raftAddress,omitempty"`
	DBAddress   string `json:"dbAddress,omitempty"`
}

// OVSDBServerStatus defines the observed state of OVSDBServer
type OVSDBServerStatus struct {
	DatabaseStatus `json:"databaseStatus"`
	Conditions     status.Conditions `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// OVSDBServer is the Schema for the servers API
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

func conditionStatus(status bool) corev1.ConditionStatus {
	if status {
		return corev1.ConditionTrue
	} else {
		return corev1.ConditionFalse
	}
}

func (server *OVSDBServer) SetAvailable(available bool) {
	condition := status.Condition{
		Type:   OVSDBServerAvailable,
		Status: conditionStatus(available),
	}

	server.Status.Conditions.SetCondition(condition)
}

func (server *OVSDBServer) IsAvailable() bool {
	return server.Status.Conditions.IsTrueFor(OVSDBServerAvailable)
}

func (server *OVSDBServer) SetFailed(failed bool, reason status.ConditionReason, err error) {
	msg := ""
	if err != nil {
		msg = err.Error()
	}
	condition := status.Condition{
		Type:    OVSDBServerFailed,
		Status:  conditionStatus(failed),
		Reason:  reason,
		Message: msg,
	}

	server.Status.Conditions.SetCondition(condition)
}

func (server *OVSDBServer) IsFailed() bool {
	return server.Status.Conditions.IsTrueFor(OVSDBServerFailed)
}
