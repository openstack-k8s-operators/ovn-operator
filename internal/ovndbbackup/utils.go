package ovndbbackup

import (
	"crypto/sha256"
	"fmt"

	ovnv1 "github.com/openstack-k8s-operators/ovn-operator/api/v1beta1"
	"github.com/openstack-k8s-operators/ovn-operator/internal/ovndbcluster"
)

func truncateName(name string, maxLen int) string {
	if len(name) <= maxLen {
		return name
	}
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(name)))
	return name[:maxLen-9] + "-" + hash[:8]
}

// BackupCronJobName returns the name for the backup CronJob (max 52 chars for CronJobs)
func BackupCronJobName(backup *ovnv1.OVNDBBackup) string {
	return truncateName(backup.Name+"-cronjob", 52)
}

// BackupPVCName returns the name for the backup PVC
func BackupPVCName(backup *ovnv1.OVNDBBackup) string {
	return truncateName(backup.Name+"-backup", 63)
}

// RestoreJobName returns the name for the restore Job
func RestoreJobName(restore *ovnv1.OVNDBRestore) string {
	return truncateName(restore.Name+"-restore", 63)
}

// BackupScriptsConfigMapName returns the name for the backup scripts ConfigMap
func BackupScriptsConfigMapName(backup *ovnv1.OVNDBBackup) string {
	return backup.Name + "-backup-scripts"
}

// ClusterPod0PVCName returns the PVC name for pod-0 of a given OVNDBCluster.
// StatefulSet PVCs follow the pattern: <volumeClaimTemplate>-<statefulset>-<ordinal>.
// The volume claim template is named <cluster.Name>-etc-ovn and the StatefulSet
// uses the service name (ovsdbserver-nb / ovsdbserver-sb), not the cluster CR name.
func ClusterPod0PVCName(cluster *ovnv1.OVNDBCluster) string {
	stsName := ServiceName(cluster)
	return cluster.Name + ovndbcluster.PVCSuffixEtcOVN + "-" + stsName + "-0"
}

// Pod0Name returns the pod-0 name for a given OVNDBCluster.
// The StatefulSet is named after the service (ovsdbserver-nb / ovsdbserver-sb).
func Pod0Name(cluster *ovnv1.OVNDBCluster) string {
	return ServiceName(cluster) + "-0"
}

// StatefulSetName returns the StatefulSet name for a given OVNDBCluster.
// The StatefulSet is named after the service (ovsdbserver-nb / ovsdbserver-sb).
func StatefulSetName(cluster *ovnv1.OVNDBCluster) string {
	return ServiceName(cluster)
}

// ServiceName returns the headless service name for a given OVNDBCluster
func ServiceName(cluster *ovnv1.OVNDBCluster) string {
	if cluster.Spec.DBType == ovnv1.SBDBType {
		return ovnv1.ServiceNameSB
	}
	return ovnv1.ServiceNameNB
}

// DBPort returns the DB port for a given OVNDBCluster
func DBPort(cluster *ovnv1.OVNDBCluster) int32 {
	if cluster.Spec.DBType == ovnv1.SBDBType {
		return ovndbcluster.DbPortSB
	}
	return ovndbcluster.DbPortNB
}
