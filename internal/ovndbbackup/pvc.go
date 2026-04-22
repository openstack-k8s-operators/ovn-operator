package ovndbbackup

import (
	"github.com/openstack-k8s-operators/lib-common/modules/common/backup"
	"github.com/openstack-k8s-operators/lib-common/modules/common/util"
	ovnv1 "github.com/openstack-k8s-operators/ovn-operator/api/v1beta1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// BackupPVC returns the PVC definition for storing backups.
// The PVC is intentionally NOT owned by the OVNDBBackup CR so that
// backup data survives CR deletion.
func BackupPVC(instance *ovnv1.OVNDBBackup, cluster *ovnv1.OVNDBCluster) *corev1.PersistentVolumeClaim {
	storageClass := instance.Spec.StorageClass
	if storageClass == "" {
		storageClass = cluster.Spec.StorageClass
	}

	storageRequest := instance.Spec.StorageRequest
	if storageRequest == "" {
		storageRequest = cluster.Spec.StorageRequest
	}

	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      BackupPVCName(instance),
			Namespace: instance.Namespace,
			Labels: util.MergeStringMaps(
				map[string]string{
					"app":                         "ovndbbackup",
					"ovndbbackup":                 instance.Name,
					"ovn.openstack.org/dbcluster": instance.Spec.DatabaseInstance,
				},
				backup.GetBackupLabels(backup.CategoryControlPlane),
				backup.GetRestoreLabels(backup.RestoreOrder00, backup.CategoryControlPlane),
			),
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{
				corev1.ReadWriteOnce,
			},
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: resource.MustParse(storageRequest),
				},
			},
		},
	}

	if storageClass != "" {
		pvc.Spec.StorageClassName = &storageClass
	}

	return pvc
}
