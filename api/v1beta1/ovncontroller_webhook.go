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
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// OVNControllerDefaults -
type OVNControllerDefaults struct {
	OVSContainerImageURL           string
	OVNControllerContainerImageURL string
}

var ovnDefaults OVNControllerDefaults

// log is for logging in this package.
var ovncontrollerlog = logf.Log.WithName("ovncontroller-resource")

// SetupOVNControllerDefaults - initialize OVNController spec defaults for use with either internal or external webhooks
func SetupOVNControllerDefaults(defaults OVNControllerDefaults) {
	ovnDefaults = defaults
	ovncontrollerlog.Info("OVNController defaults initialized", "defaults", defaults)
}

func (r *OVNController) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		Complete()
}

//+kubebuilder:webhook:path=/mutate-ovn-openstack-org-v1beta1-ovncontroller,mutating=true,failurePolicy=fail,sideEffects=None,groups=ovn.openstack.org,resources=ovncontrollers,verbs=create;update,versions=v1beta1,name=movncontroller.kb.io,admissionReviewVersions=v1

var _ webhook.Defaulter = &OVNController{}

// Default implements webhook.Defaulter so a webhook will be registered for the type
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
	spec.OVNControllerSpecCore.Default()
}

// Default - set defaults for this OVNController core spec (this version is called by OpenStackControlplane webhooks)
func (spec *OVNControllerSpecCore) Default() {
	// nothing here yet
}

//+kubebuilder:webhook:path=/validate-ovn-openstack-org-v1beta1-ovncontroller,mutating=false,failurePolicy=fail,sideEffects=None,groups=ovn.openstack.org,resources=ovncontrollers,verbs=create;update,versions=v1beta1,name=vovncontroller.kb.io,admissionReviewVersions=v1

var _ webhook.Validator = &OVNController{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
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

	return errors
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *OVNController) ValidateUpdate(old runtime.Object) (admission.Warnings, error) {
	ovncontrollerlog.Info("validate update", "name", r.Name)

	errors := field.ErrorList{}
	basePath := field.NewPath("spec")

	// When a TopologyRef CR is referenced, fail if a different Namespace is
	// referenced because is not supported
	errors = append(errors, r.Spec.ValidateTopology(basePath, r.Namespace)...)

	if len(errors) != 0 {
		return nil, apierrors.NewInvalid(
			schema.GroupKind{Group: "ovn.openstack.org", Kind: "OVNController"},
			r.Name, errors)
	}
	return nil, nil
}

func (r OVNControllerSpec) ValidateUpdate(old OVNControllerSpec, basePath *field.Path, namespace string) field.ErrorList {
	return r.OVNControllerSpecCore.ValidateCreate(basePath, namespace)
}

func (r *OVNControllerSpecCore) ValidateUpdate(old OVNControllerSpec, basePath *field.Path, namespace string) field.ErrorList {
	allErrs := field.ErrorList{}

	// When a TopologyRef CR is referenced, fail if a different Namespace is
	// referenced because is not supported
	allErrs = append(allErrs, r.ValidateTopology(basePath, namespace)...)

	return allErrs
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *OVNController) ValidateDelete() (admission.Warnings, error) {
	ovncontrollerlog.Info("validate delete", "name", r.Name)

	return nil, nil
}
