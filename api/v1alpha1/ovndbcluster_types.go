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

package v1alpha1

import (
	"context"
	"fmt"

	"github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	"github.com/openstack-k8s-operators/lib-common/modules/common/helper"
	"sigs.k8s.io/controller-runtime/pkg/client"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	k8s_errors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

//
// GetDBEndpoints - get DB Endpoints
//
func GetDBEndpoints(
	ctx context.Context,
	h *helper.Helper,
	namespace string,
	labelSelector map[string]string,
) (map[string]string, error) {
	ovnDBList := &OVNDBClusterList{}

	listOpts := []client.ListOption{
		client.InNamespace(namespace),
	}

	if len(labelSelector) > 0 {
		labels := client.MatchingLabels(labelSelector)
		listOpts = append(listOpts, labels)
	}

	err := h.GetClient().List(ctx, ovnDBList, listOpts...)
	if err != nil {
		return nil, err
	}

	if len(ovnDBList.Items) > 2 {
		return nil, fmt.Errorf("more then two OVNDBCluster object found in namespace %s", namespace)
	}

	if len(ovnDBList.Items) == 0 {
		return nil, k8s_errors.NewNotFound(
			appsv1.Resource("OVNDBCluster"),
			fmt.Sprintf("No OVNDBCluster object found in namespace %s", namespace),
		)
	}
	DBEndpointsMap := make(map[string]string)
	for _, ovndb := range ovnDBList.Items {
		DBEndpointsMap[ovndb.Spec.DBType] = ovndb.Status.DBAddress
	}
	return DBEndpointsMap, nil
}

// OVNDBClusterSpec defines the desired state of OVNDBCluster
type OVNDBClusterSpec struct {
	ContainerImage string `json:"containerImage,omitempty"`

	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Pattern="^NB|SB$"
	// DBType - NB or SB
	DBType string `json:"dbType"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default=1
	// +kubebuilder:validation:Maximum=1
	// +kubebuilder:validation:Minimum=0
	// Replicas of OVN DBCluster to run
	Replicas int32 `json:"replicas"`

	// +kubebuilder:validation:Optional
	// NodeSelector to target subset of worker nodes running this service
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default=info
	// LogLevel - Set log level info, dbg, emer etc
	LogLevel string `json:"logLevel"`

	// +kubebuilder:validation:Optional
	// Resources - Compute Resources required by this service (Limits/Requests).
	// https://kubernetes.io/docs/concepts/configuration/manage-resources-containers/
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`

	// +kubebuilder:validation:Optional
	// StorageClass
	StorageClass string `json:"storageClass,omitempty"`

	// +kubebuilder:validation:Required
	// StorageRequest
	StorageRequest string `json:"storageRequest"`
}

// OVNDBClusterStatus defines the observed state of OVNDBCluster
type OVNDBClusterStatus struct {
	// ReadyCount of OVN DBCluster instances
	ReadyCount int32 `json:"readyCount,omitempty"`

	// Map of hashes to track e.g. job status
	Hash map[string]string `json:"hash,omitempty"`

	// Conditions
	Conditions condition.Conditions `json:"conditions,omitempty" optional:"true"`

	// RaftAddress -
	RaftAddress string `json:"raftAddress,omitempty"`

	// DBAddress -
	DBAddress string `json:"dbAddress,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.conditions[0].status",description="Status"
//+kubebuilder:printcolumn:name="Message",type="string",JSONPath=".status.conditions[0].message",description="Message"

// OVNDBCluster is the Schema for the ovndbclusters API
type OVNDBCluster struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   OVNDBClusterSpec   `json:"spec,omitempty"`
	Status OVNDBClusterStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// OVNDBClusterList contains a list of OVNDBCluster
type OVNDBClusterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []OVNDBCluster `json:"items"`
}

func init() {
	SchemeBuilder.Register(&OVNDBCluster{}, &OVNDBClusterList{})
}

// IsReady - returns true if service is ready to server requests
func (instance OVNDBCluster) IsReady() bool {
	// Ready when:
	// there is at least a single pod to serve the OVN DBCluster
	return instance.Status.ReadyCount >= 1
}
