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

type DBType string

const (
	DBTypeNB DBType = "NB"
	DBTypeSB DBType = "SB"
)

const (
	OVSDBClusterServers status.ConditionReason = "FailedServers"
)

// OVSDBClusterSpec defines the desired state of OVSDBCluster
type OVSDBClusterSpec struct {
	DBType       DBType  `json:"dbType"`
	Replicas     int     `json:"replicas"`
	ClientConfig *string `json:"clientConfig,omitempty"`

	Image              string            `json:"image"`
	ServerStorageSize  resource.Quantity `json:"serverStorageSize"`
	ServerStorageClass *string           `json:"serverStorageClass,omitempty"`
}

// OVSDBClusterStatus defines the observed state of OVSDBCluster
type OVSDBClusterStatus struct {
	Conditions       status.Conditions `json:"conditions,omitempty"`
	ClusterID        *string           `json:"clusterID,omitempty"`
	AvailableServers int               `json:"availableServers"`
	ClusterSize      int               `json:"clusterSize"`
	ClusterQuorum    int               `json:"clusterQuorum"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// OVSDBCluster is the Schema for the ovsdbclusters API
type OVSDBCluster struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   OVSDBClusterSpec   `json:"spec,omitempty"`
	Status OVSDBClusterStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// OVSDBClusterList contains a list of OVSDBCluster
type OVSDBClusterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []OVSDBCluster `json:"items"`
}

func init() {
	SchemeBuilder.Register(&OVSDBCluster{}, &OVSDBClusterList{})
}

// ObjectWithConditions

func (cluster *OVSDBCluster) GetConditions() *status.Conditions {
	return &cluster.Status.Conditions
}
