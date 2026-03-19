/*
Copyright 2026.

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
	condition "github.com/openstack-k8s-operators/lib-common/modules/common/condition"
)

// OVN Condition Types used by API objects.
const (
	// ExternalConfigReadyCondition indicates when the external config (e.g. ovncontroller-config ConfigMap) is ready
	ExternalConfigReadyCondition condition.Type = "External Config Ready"
)

// Common messages used by API objects.
const (
	// ExternalConfigInitMessage is the init message for ExternalConfigReadyCondition
	ExternalConfigInitMessage = "External config generation is not started"

	// ExternalConfigErrorMessage is the error message format for ExternalConfigReadyCondition
	ExternalConfigErrorMessage = "External config generation error: %s"
)
