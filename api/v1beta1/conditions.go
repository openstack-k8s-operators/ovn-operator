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

	// OVNDBClusterReadyCondition indicates when the referenced OVNDBCluster exists and is available
	OVNDBClusterReadyCondition condition.Type = "OVNDBClusterReady"

	// PersistentVolumeClaimReadyCondition indicates when the backup PVC is ready
	PersistentVolumeClaimReadyCondition condition.Type = "PersistentVolumeClaimReady"

	// CronJobReadyCondition indicates when the backup CronJob is ready
	CronJobReadyCondition condition.Type = "CronJobReady"

	// OVNDBBackupReadyCondition indicates when the referenced OVNDBBackup is ready (used by restore)
	OVNDBBackupReadyCondition condition.Type = "OVNDBBackupReady"

	// RestoreJobReadyCondition indicates when the restore Job has completed
	RestoreJobReadyCondition condition.Type = "RestoreJobReady"
)

// Common messages used by API objects.
const (
	// ExternalConfigInitMessage is the init message for ExternalConfigReadyCondition
	ExternalConfigInitMessage = "External config generation is not started"

	// ExternalConfigErrorMessage is the error message format for ExternalConfigReadyCondition
	ExternalConfigErrorMessage = "External config generation error: %s"

	// OVNDBClusterReadyInitMessage
	OVNDBClusterReadyInitMessage = "OVNDBCluster not yet available"

	// OVNDBClusterReadyMessage
	OVNDBClusterReadyMessage = "OVNDBCluster is available"

	// OVNDBClusterReadyErrorMessage
	OVNDBClusterReadyErrorMessage = "OVNDBCluster error occurred %s"

	// PersistentVolumeClaimReadyInitMessage
	PersistentVolumeClaimReadyInitMessage = "PersistentVolumeClaim not yet created"

	// PersistentVolumeClaimReadyMessage
	PersistentVolumeClaimReadyMessage = "PersistentVolumeClaim created"

	// PersistentVolumeClaimReadyErrorMessage
	PersistentVolumeClaimReadyErrorMessage = "PersistentVolumeClaim error occurred %s"

	// CronJobReadyInitMessage
	CronJobReadyInitMessage = "CronJob not yet created"

	// CronJobReadyMessage
	CronJobReadyMessage = "CronJob created"

	// CronJobReadyErrorMessage
	CronJobReadyErrorMessage = "CronJob error occurred %s"

	// OVNDBBackupReadyInitMessage
	OVNDBBackupReadyInitMessage = "OVNDBBackup not yet available"

	// OVNDBBackupReadyMessage
	OVNDBBackupReadyMessage = "OVNDBBackup is available"

	// OVNDBBackupReadyErrorMessage
	OVNDBBackupReadyErrorMessage = "OVNDBBackup error occurred %s"

	// RestoreJobReadyInitMessage
	RestoreJobReadyInitMessage = "Restore not yet started"

	// RestoreJobReadyMessage
	RestoreJobReadyMessage = "Restore completed"

	// RestoreJobReadyErrorMessage
	RestoreJobReadyErrorMessage = "Restore error occurred %s"
)
