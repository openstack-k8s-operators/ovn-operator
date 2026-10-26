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
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// OVNControllerDefaults -
type OVNControllerDefaults struct {
	OVSContainerImageURL           string
	OVNControllerContainerImageURL string
	ExporterImageURL               string
}

var ovnDefaults OVNControllerDefaults

// log is for logging in this package.
var ovncontrollerlog = logf.Log.WithName("ovncontroller-resource")

// SetupOVNControllerDefaults - initialize OVNController spec defaults for use with either internal or external webhooks
func SetupOVNControllerDefaults(defaults OVNControllerDefaults) {
	ovnDefaults = defaults
	ovncontrollerlog.Info("OVNController defaults initialized", "defaults", defaults)
}

// Default sets default values for the OVNController resource
func (r *OVNController) Default() {
	ovncontrollerlog.Info("default", "name", r.Name)

	r.Spec.Default()
}

// Default - set defaults for this OVNController spec
func (spec *OVNControllerSpec) Default() {
	if spec.OvsContainerImage == "" {
		spec.OvsContainerImage = ovnDefaults.OVSContainerImageURL
	}
	if spec.OvnContainerImage == "" {
		spec.OvnContainerImage = ovnDefaults.OVNControllerContainerImageURL
	}
	if spec.ExporterImage == "" {
		spec.ExporterImage = ovnDefaults.ExporterImageURL
	}
	spec.OVNControllerSpecCore.Default()
}

// Default - set defaults for this OVNController core spec (this version is called by OpenStackControlplane webhooks)
func (spec *OVNControllerSpecCore) Default() {
	// nothing here yet
}

// ValidateCreate validates the OVNController resource on creation
func (r *OVNController) ValidateCreate() (admission.Warnings, error) {
	ovncontrollerlog.Info("validate create", "name", r.Name)
	errors := field.ErrorList{}

	errors = r.Spec.ValidateCreate(field.NewPath("spec"), r.Namespace)
	if len(errors) != 0 {
		ovncontrollerlog.Info("validation failed", "name", r.Name)
		return nil, apierrors.NewInvalid(
			schema.GroupKind{Group: "ovn.openstack.org", Kind: "OVNController"},
			r.Name, errors)
	}
	return nil, nil
}

func (r OVNControllerSpec) ValidateCreate(basePath *field.Path, namespace string) field.ErrorList {
	return r.OVNControllerSpecCore.ValidateCreate(basePath, namespace)
}

func (r *OVNControllerSpecCore) ValidateCreate(basePath *field.Path, namespace string) field.ErrorList {
	errors := field.ErrorList{}

	// When a TopologyRef CR is referenced, fail if a different Namespace is
	// referenced because is not supported
	errors = append(errors, r.ValidateTopology(basePath, namespace)...)
	errors = append(errors, r.ValidateProbes(basePath.Child("override", "probes"))...)

	return errors
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *OVNController) ValidateUpdate(old runtime.Object) (admission.Warnings, error) {
	ovncontrollerlog.Info("validate update", "name", r.Name)

	errors := r.Spec.OVNControllerSpecCore.ValidateCreate(field.NewPath("spec"), r.Namespace)

	if len(errors) != 0 {
		return nil, apierrors.NewInvalid(
			schema.GroupKind{Group: "ovn.openstack.org", Kind: "OVNController"},
			r.Name, errors)
	}
	return nil, nil
}

func (r OVNControllerSpec) ValidateUpdate(old OVNControllerSpec, basePath *field.Path, namespace string) field.ErrorList {
	return r.OVNControllerSpecCore.ValidateUpdate(old, basePath, namespace)
}

func (r *OVNControllerSpecCore) ValidateUpdate(old OVNControllerSpec, basePath *field.Path, namespace string) field.ErrorList {
	return r.ValidateCreate(basePath, namespace)
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *OVNController) ValidateDelete() (admission.Warnings, error) {
	ovncontrollerlog.Info("validate delete", "name", r.Name)

	return nil, nil
}
