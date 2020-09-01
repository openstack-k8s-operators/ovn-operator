package stubs

import (
	"io/ioutil"
	"os"
	"path"

	corev1 "k8s.io/api/core/v1"
	yaml "sigs.k8s.io/yaml"

	ovncentralv1alpha1 "github.com/openstack-k8s-operators/ovn-central-operator/api/v1alpha1"
)

var (
	ovsdb_container_tpl corev1.Container
	pvc_tpl             corev1.PersistentVolumeClaim
)

func init() {
	yamlsPath := os.Getenv("OPERATOR_YAMLS")
	if yamlsPath == "" {
		panic("Environment variable OPERATOR_YAMLS is not set")
	}

	yamls := map[string]interface{}{
		"ovsdb_container.yaml": &ovsdb_container_tpl,
		"pvc.yaml":             &pvc_tpl,
	}

	for file, obj := range yamls {
		contents, err := ioutil.ReadFile(path.Join(yamlsPath, file))
		if err != nil {
			panic(err)
		}

		err = yaml.UnmarshalStrict(contents, obj)
		if err != nil {
			panic(err)
		}
	}
}

func OVSDBContainer(cr *ovncentralv1alpha1.OVNCentralSpec,
	containerName string, containerCommand []string) *corev1.Container {

	container := ovsdb_container_tpl.DeepCopy()
	container.Image = cr.Image
	container.Name = containerName
	container.Command = containerCommand

	return container
}

func PVC(cr *ovncentralv1alpha1.OVNCentral, name string) *corev1.PersistentVolumeClaim {
	pvc := pvc_tpl.DeepCopy()

	pvc.Name = name
	pvc.Namespace = cr.Namespace
	pvc.Spec.Resources.Requests = corev1.ResourceList{corev1.ResourceStorage: cr.Spec.StorageSize}
	pvc.Spec.StorageClassName = cr.Spec.StorageClass

	return pvc
}
