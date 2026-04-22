// Package ovndbbackup provides functionality for managing OVN database backups and restores
package ovndbbackup

const (
	// BackupDataMountPath is the mount path for backup data in pods
	BackupDataMountPath = "/backup/data"

	// BackupVolumeName is the name of the backup PVC volume
	BackupVolumeName = "backup-data"

	// BackupScriptsVolumeName is the name of the backup scripts ConfigMap volume
	BackupScriptsVolumeName = "backup-scripts"

	// BackupScriptsMountPath is the mount path for backup scripts
	BackupScriptsMountPath = "/usr/local/bin/backup-scripts"

	// DBDataVolumeName is the name of the OVN DB data volume (for restore)
	DBDataVolumeName = "ovn-db-data"

	// DBDataMountPath is the mount path for OVN DB data (for restore)
	DBDataMountPath = "/etc/ovn"
)
