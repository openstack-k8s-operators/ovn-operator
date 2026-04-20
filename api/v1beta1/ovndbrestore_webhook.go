/*

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
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

var ovndbrestorelog = logf.Log.WithName("ovndbrestore-resource")

var _ webhook.Defaulter = &OVNDBRestore{}

// Default implements webhook.Defaulter so a webhook will be registered for the type
func (r *OVNDBRestore) Default() {
	ovndbrestorelog.Info("default", "name", r.Name)
}

var _ webhook.Validator = &OVNDBRestore{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *OVNDBRestore) ValidateCreate() (admission.Warnings, error) {
	ovndbrestorelog.Info("validate create", "name", r.Name)

	errors := field.ErrorList{}
	basePath := field.NewPath("spec")

	if r.Spec.BackupSource == "" {
		errors = append(errors, field.Required(basePath.Child("backupSource"), "backupSource is required"))
	}

	if len(errors) != 0 {
		return nil, apierrors.NewInvalid(
			schema.GroupKind{Group: "ovn.openstack.org", Kind: "OVNDBRestore"},
			r.Name, errors)
	}
	return nil, nil
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *OVNDBRestore) ValidateUpdate(old runtime.Object) (admission.Warnings, error) {
	ovndbrestorelog.Info("validate update", "name", r.Name)

	errors := field.ErrorList{}
	basePath := field.NewPath("spec")

	oldRestore, ok := old.(*OVNDBRestore)
	if ok && oldRestore.Spec.BackupSource != r.Spec.BackupSource {
		errors = append(errors, field.Forbidden(basePath.Child("backupSource"), "backupSource is immutable"))
	}

	if len(errors) != 0 {
		return nil, apierrors.NewInvalid(
			schema.GroupKind{Group: "ovn.openstack.org", Kind: "OVNDBRestore"},
			r.Name, errors)
	}
	return nil, nil
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *OVNDBRestore) ValidateDelete() (admission.Warnings, error) {
	ovndbrestorelog.Info("validate delete", "name", r.Name)
	return nil, nil
}
