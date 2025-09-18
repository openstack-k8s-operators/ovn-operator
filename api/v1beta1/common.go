/*
Copyright 2023.

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

import "github.com/openstack-k8s-operators/lib-common/modules/common/util"

// SetupDefaults - initializes any CRD field defaults based on environment variables (the defaulting mechanism itself is implemented via webhooks)
func SetupDefaults() {
	// Acquire environmental defaults and initialize OVNDBCluster defaults with them
	ovnDbClusterDefaults := OVNDBClusterDefaults{
		NBContainerImageURL: util.GetEnvVar("RELATED_IMAGE_OVN_NB_DBCLUSTER_IMAGE_URL_DEFAULT", OVNNBContainerImage),
		SBContainerImageURL: util.GetEnvVar("RELATED_IMAGE_OVN_SB_DBCLUSTER_IMAGE_URL_DEFAULT", OVNSBContainerImage),
		ExporterImageURL:    util.GetEnvVar("RELATED_IMAGE_OPENSTACK_NETWORK_EXPORTER_IMAGE_URL_DEFAULT", OpenstackNetworkExporterImage),
	}

	SetupOVNDBClusterDefaults(ovnDbClusterDefaults)

	// Acquire environmental defaults and initialize OVNNorthd defaults with them
	ovnNorthdDefaults := OVNNorthdDefaults{
		ContainerImageURL: util.GetEnvVar("RELATED_IMAGE_OVN_NORTHD_IMAGE_URL_DEFAULT", OVNNorthdContainerImage),
		ExporterImageURL:  util.GetEnvVar("RELATED_IMAGE_OPENSTACK_NETWORK_EXPORTER_IMAGE_URL_DEFAULT", OpenstackNetworkExporterImage),
	}

	SetupOVNNorthdDefaults(ovnNorthdDefaults)

	// Acquire environmental defaults and initialize OVNController defaults with them
	ovnControllerDefaults := OVNControllerDefaults{
		OVSContainerImageURL:           util.GetEnvVar("RELATED_IMAGE_OVN_CONTROLLER_OVS_IMAGE_URL_DEFAULT", OVNControllerOVSContainerImage),
		OVNControllerContainerImageURL: util.GetEnvVar("RELATED_IMAGE_OVN_CONTROLLER_IMAGE_URL_DEFAULT", OVNControllerContainerImage),
		ExporterImageURL:               util.GetEnvVar("RELATED_IMAGE_OPENSTACK_NETWORK_EXPORTER_IMAGE_URL_DEFAULT", OpenstackNetworkExporterImage),
	}

	SetupOVNControllerDefaults(ovnControllerDefaults)
}
