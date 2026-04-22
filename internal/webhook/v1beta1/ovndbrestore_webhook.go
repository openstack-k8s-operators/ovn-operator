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
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	ovnv1beta1 "github.com/openstack-k8s-operators/ovn-operator/api/v1beta1"
)

// nolint:unused
var ovndbrestorelog = logf.Log.WithName("ovndbrestore-resource")

// SetupOVNDBRestoreWebhookWithManager registers the webhook for OVNDBRestore in the manager.
func SetupOVNDBRestoreWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).For(&ovnv1beta1.OVNDBRestore{}).
		WithValidator(&OVNDBRestoreCustomValidator{}).
		WithDefaulter(&OVNDBRestoreCustomDefaulter{}).
		Complete()
}

// +kubebuilder:webhook:path=/mutate-ovn-openstack-org-v1beta1-ovndbrestore,mutating=true,failurePolicy=fail,sideEffects=None,groups=ovn.openstack.org,resources=ovndbrestores,verbs=create;update,versions=v1beta1,name=movndbrestore-v1beta1.kb.io,admissionReviewVersions=v1

// OVNDBRestoreCustomDefaulter struct is responsible for setting default values on the custom resource of the
// Kind OVNDBRestore when those are created or updated.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as it is used only for temporary operations and does not need to be deeply copied.
type OVNDBRestoreCustomDefaulter struct{}

var _ webhook.CustomDefaulter = &OVNDBRestoreCustomDefaulter{}

// Default implements webhook.CustomDefaulter so a webhook will be registered for the Kind OVNDBRestore.
func (d *OVNDBRestoreCustomDefaulter) Default(_ context.Context, obj runtime.Object) error {
	ovndbrestore, ok := obj.(*ovnv1beta1.OVNDBRestore)
	if !ok {
		return fmt.Errorf("expected an OVNDBRestore object but got %T: %w", obj, ErrInvalidObjectType)
	}
	ovndbrestorelog.Info("Defaulting for OVNDBRestore", "name", ovndbrestore.GetName())

	ovndbrestore.Default()

	return nil
}

// +kubebuilder:webhook:path=/validate-ovn-openstack-org-v1beta1-ovndbrestore,mutating=false,failurePolicy=fail,sideEffects=None,groups=ovn.openstack.org,resources=ovndbrestores,verbs=create;update,versions=v1beta1,name=vovndbrestore-v1beta1.kb.io,admissionReviewVersions=v1

// OVNDBRestoreCustomValidator struct is responsible for validating the OVNDBRestore resource
// when it is created, updated, or deleted.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as this struct is used only for temporary operations and does not need to be deeply copied.
type OVNDBRestoreCustomValidator struct{}

var _ webhook.CustomValidator = &OVNDBRestoreCustomValidator{}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type OVNDBRestore.
func (v *OVNDBRestoreCustomValidator) ValidateCreate(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	ovndbrestore, ok := obj.(*ovnv1beta1.OVNDBRestore)
	if !ok {
		return nil, fmt.Errorf("expected a OVNDBRestore object but got %T: %w", obj, ErrInvalidObjectType)
	}
	ovndbrestorelog.Info("Validation for OVNDBRestore upon creation", "name", ovndbrestore.GetName())

	return ovndbrestore.ValidateCreate()
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type OVNDBRestore.
func (v *OVNDBRestoreCustomValidator) ValidateUpdate(_ context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	ovndbrestore, ok := newObj.(*ovnv1beta1.OVNDBRestore)
	if !ok {
		return nil, fmt.Errorf("expected a OVNDBRestore object for the newObj but got %T: %w", newObj, ErrInvalidObjectType)
	}
	ovndbrestorelog.Info("Validation for OVNDBRestore upon update", "name", ovndbrestore.GetName())

	return ovndbrestore.ValidateUpdate(oldObj)
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type OVNDBRestore.
func (v *OVNDBRestoreCustomValidator) ValidateDelete(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	ovndbrestore, ok := obj.(*ovnv1beta1.OVNDBRestore)
	if !ok {
		return nil, fmt.Errorf("expected a OVNDBRestore object but got %T: %w", obj, ErrInvalidObjectType)
	}
	ovndbrestorelog.Info("Validation for OVNDBRestore upon deletion", "name", ovndbrestore.GetName())

	return ovndbrestore.ValidateDelete()
}
