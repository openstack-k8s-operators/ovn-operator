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

package controllers

import (
	corev1 "k8s.io/api/core/v1"

	ovnv1alpha1 "github.com/openstack-k8s-operators/ovn-operator/api/v1alpha1"
	"github.com/openstack-k8s-operators/ovn-operator/util"
)

func pvcName(server *ovnv1alpha1.OVSDBServer) string {
	return server.Name
}

func pvcShell(server *ovnv1alpha1.OVSDBServer) *corev1.PersistentVolumeClaim {

	pvc := &corev1.PersistentVolumeClaim{}
	pvc.Name = pvcName(server)
	pvc.Namespace = server.Namespace

	return pvc
}

func pvcApply(pvc *corev1.PersistentVolumeClaim, server *ovnv1alpha1.OVSDBServer) {
	util.InitLabelMap(&pvc.Labels)

	pvc.Spec.Resources.Requests = corev1.ResourceList{
		corev1.ResourceStorage: server.Spec.StorageSize,
	}

	// StorageClassName will be defaulted server-side if we
	// originally passed an empty one, so don't try to overwrite
	// it.
	if server.Spec.StorageClass != nil {
		pvc.Spec.StorageClassName = server.Spec.StorageClass
	}

	pvc.Spec.AccessModes = []corev1.PersistentVolumeAccessMode{
		corev1.ReadWriteOnce,
	}

	volumeMode := corev1.PersistentVolumeFilesystem
	pvc.Spec.VolumeMode = &volumeMode
}
