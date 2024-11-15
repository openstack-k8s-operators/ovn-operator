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
	"fmt"

	"github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	"github.com/openstack-k8s-operators/lib-common/modules/common/service"
	"github.com/openstack-k8s-operators/lib-common/modules/common/tls"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// NBDBType - Northbound database type
	NBDBType      = "NB"
	ServiceNameNB = "ovsdbserver-nb"
	// SBDBType - Southbound database type
	SBDBType      = "SB"
	ServiceNameSB = "ovsdbserver-sb"

	// ServiceHeadlessType - Constant to identify Headless services
	ServiceHeadlessType = "headless"
	// ServiceClusterType - Constant to identify Cluster services
	ServiceClusterType = "cluster"

	// DNSSuffix : hardcoded value on how DNSCore domain is configured
	DNSSuffix = "cluster.local"
	// TODO: retrieve it from environment

	// Container image fall-back defaults

	// OVNNBContainerImage is the fall-back container image for OVNDBCluster NB
	OVNNBContainerImage = "quay.io/podified-antelope-centos9/openstack-ovn-nb-db-server:current-podified"
	// OVNSBContainerImage is the fall-back container image for OVNDBCluster SB
	OVNSBContainerImage = "quay.io/podified-antelope-centos9/openstack-ovn-sb-db-server:current-podified"
)

// OVNDBClusterSpec defines the desired state of OVNDBCluster
type OVNDBClusterSpec struct {
	// +kubebuilder:validation:Required
	// ContainerImage - Container Image URL (will be set to environmental default if empty)
	ContainerImage string `json:"containerImage"`

	OVNDBClusterSpecCore `json:",inline"`
}

// OVNDBClusterSpecCore -
type OVNDBClusterSpecCore struct {

	// +kubebuilder:validation:Required
	// +kubebuilder:default="NB"
	// +kubebuilder:validation:Pattern="^NB|SB$"
	// DBType - NB or SB
	DBType string `json:"dbType"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default=1
	// +kubebuilder:validation:Maximum=32
	// +kubebuilder:validation:Minimum=0
	// Replicas of OVN DBCluster to run
	Replicas *int32 `json:"replicas"`

	// +kubebuilder:validation:Optional
	// NodeSelector to target subset of worker nodes running this service
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default=info
	// LogLevel - Set log level info, dbg, emer etc
	LogLevel string `json:"logLevel,omitempty"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default=10000
	// OVN Northbound and Southbound RAFT db election timer to use on db creation (in milliseconds)
	ElectionTimer int32 `json:"electionTimer"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default=60000
	// Probe interval for the OVSDB session (in milliseconds)
	InactivityProbe int32 `json:"inactivityProbe"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default=60000
	// Active probe interval from standby to active ovsdb-server remote
	ProbeIntervalToActive int32 `json:"probeIntervalToActive"`

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

	// +kubebuilder:validation:Optional
	// NetworkAttachment is a NetworkAttachment resource name to expose the service to the given network.
	// If specified the IP address of this network is used as the dbAddress connection.
	NetworkAttachment string `json:"networkAttachment"`

	// +kubebuilder:validation:Optional
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	// TLS - Parameters related to TLS
	TLS tls.SimpleService `json:"tls,omitempty"`

	// +kubebuilder:validation:Optional
	// Override, provides the ability to override the generated manifest of several child resources.
	Override OVNDBClusterOverrideSpec `json:"override,omitempty"`
}

// OVNDBClusterOverrideSpec to override the generated manifest of several child resources.
type OVNDBClusterOverrideSpec struct {
	// Override configuration for the Service created to serve traffic to the cluster.
	Service *service.OverrideSpec `json:"service,omitempty"`
}

// OVNDBClusterStatus defines the observed state of OVNDBCluster
type OVNDBClusterStatus struct {
	// ReadyCount of OVN DBCluster instances
	ReadyCount int32 `json:"readyCount,omitempty"`

	// Map of hashes to track e.g. job status
	Hash map[string]string `json:"hash,omitempty"`

	// Conditions
	Conditions condition.Conditions `json:"conditions,omitempty" optional:"true"`

	// DBAddress - DB IP address used by external nodes
	DBAddress string `json:"dbAddress,omitempty"`

	// InternalDBAddress - DB IP address used by other Pods in the cluster
	InternalDBAddress string `json:"internalDbAddress,omitempty"`

	// NetworkAttachments status of the deployment pods
	NetworkAttachments map[string][]string `json:"networkAttachments,omitempty"`

	//ObservedGeneration - the most recent generation observed for this service. If the observed generation is less than the spec generation, then the controller has not processed the latest changes.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:printcolumn:name="NetworkAttachments",type="string",JSONPath=".status.networkAttachments",description="NetworkAttachments"
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
	// OVNDBCluster is reconciled successfully
	return instance.Status.Conditions.IsTrue(condition.ReadyCondition)
}

// RbacConditionsSet - set the conditions for the rbac object
func (instance OVNDBCluster) RbacConditionsSet(c *condition.Condition) {
	instance.Status.Conditions.Set(c)
}

// RbacNamespace - return the namespace
func (instance OVNDBCluster) RbacNamespace() string {
	return instance.Namespace
}

// RbacResourceName - return the name to be used for rbac objects (serviceaccount, role, rolebinding)
func (instance OVNDBCluster) RbacResourceName() string {
	return "ovncluster-" + instance.Name
}

// GetInternalEndpoint - return the DNS name that openshift coreDNS can resolve
func (instance OVNDBCluster) GetInternalEndpoint() (string, error) {
	if instance.Status.InternalDBAddress == "" {
		return "", fmt.Errorf("internal DBEndpoint not ready yet for %s", instance.Spec.DBType)
	}
	return instance.Status.InternalDBAddress, nil
}

// GetExternalEndpoint - return the DNS that openstack dnsmasq can resolve
func (instance OVNDBCluster) GetExternalEndpoint() (string, error) {
	if (instance.Spec.NetworkAttachment != "" || instance.Spec.Override.Service != nil) && instance.Status.DBAddress == "" {
		return "", fmt.Errorf("external DBEndpoint not ready yet for %s", instance.Spec.DBType)
	}
	return instance.Status.DBAddress, nil
}
