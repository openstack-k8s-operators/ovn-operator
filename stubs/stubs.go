package stubs

import (
	"io/ioutil"
	corev1 "k8s.io/api/core/v1"
	"os"
	"path"
	yaml "sigs.k8s.io/yaml"

	ovncentralv1alpha1 "github.com/openstack-k8s-operators/ovn-central-operator/api/v1alpha1"
)

var (
	ovsdb_container corev1.Container
)

func init() {
	yamlsPath := os.Getenv("OPERATOR_YAMLS")
	if yamlsPath == "" {
		panic("Environment variable OPERATOR_YAMLS is not set")
	}

	yamls := map[string]interface{}{
		"ovsdb_container.yaml": &ovsdb_container,
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

func OVSDBContainer(cr ovncentralv1alpha1.OVNCentralSpec,
	containerName string, containerCommand []string) *corev1.Container {

	container := ovsdb_container.DeepCopy()
	container.Image = cr.Image
	container.Name = containerName
	container.Command = containerCommand

	return container
}
