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

var ovndbbackuplog = logf.Log.WithName("ovndbbackup-resource")

var _ webhook.Defaulter = &OVNDBBackup{}

// Default implements webhook.Defaulter so a webhook will be registered for the type
func (r *OVNDBBackup) Default() {
	ovndbbackuplog.Info("default", "name", r.Name)
}

var _ webhook.Validator = &OVNDBBackup{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *OVNDBBackup) ValidateCreate() (admission.Warnings, error) {
	ovndbbackuplog.Info("validate create", "name", r.Name)

	errors := field.ErrorList{}
	basePath := field.NewPath("spec")

	if r.Spec.DatabaseInstance == "" {
		errors = append(errors, field.Required(basePath.Child("databaseInstance"), "databaseInstance is required"))
	}

	if len(errors) != 0 {
		return nil, apierrors.NewInvalid(
			schema.GroupKind{Group: "ovn.openstack.org", Kind: "OVNDBBackup"},
			r.Name, errors)
	}
	return nil, nil
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *OVNDBBackup) ValidateUpdate(old runtime.Object) (admission.Warnings, error) {
	ovndbbackuplog.Info("validate update", "name", r.Name)

	errors := field.ErrorList{}
	basePath := field.NewPath("spec")

	oldBackup, ok := old.(*OVNDBBackup)
	if ok && oldBackup.Spec.DatabaseInstance != r.Spec.DatabaseInstance {
		errors = append(errors, field.Forbidden(basePath.Child("databaseInstance"), "databaseInstance is immutable"))
	}

	if len(errors) != 0 {
		return nil, apierrors.NewInvalid(
			schema.GroupKind{Group: "ovn.openstack.org", Kind: "OVNDBBackup"},
			r.Name, errors)
	}
	return nil, nil
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *OVNDBBackup) ValidateDelete() (admission.Warnings, error) {
	ovndbbackuplog.Info("validate delete", "name", r.Name)
	return nil, nil
}
