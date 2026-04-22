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
var ovndbbackuplog = logf.Log.WithName("ovndbbackup-resource")

// SetupOVNDBBackupWebhookWithManager registers the webhook for OVNDBBackup in the manager.
func SetupOVNDBBackupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).For(&ovnv1beta1.OVNDBBackup{}).
		WithValidator(&OVNDBBackupCustomValidator{}).
		WithDefaulter(&OVNDBBackupCustomDefaulter{}).
		Complete()
}

// +kubebuilder:webhook:path=/mutate-ovn-openstack-org-v1beta1-ovndbbackup,mutating=true,failurePolicy=fail,sideEffects=None,groups=ovn.openstack.org,resources=ovndbbackups,verbs=create;update,versions=v1beta1,name=movndbbackup-v1beta1.kb.io,admissionReviewVersions=v1

// OVNDBBackupCustomDefaulter struct is responsible for setting default values on the custom resource of the
// Kind OVNDBBackup when those are created or updated.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as it is used only for temporary operations and does not need to be deeply copied.
type OVNDBBackupCustomDefaulter struct{}

var _ webhook.CustomDefaulter = &OVNDBBackupCustomDefaulter{}

// Default implements webhook.CustomDefaulter so a webhook will be registered for the Kind OVNDBBackup.
func (d *OVNDBBackupCustomDefaulter) Default(_ context.Context, obj runtime.Object) error {
	ovndbbackup, ok := obj.(*ovnv1beta1.OVNDBBackup)
	if !ok {
		return fmt.Errorf("expected an OVNDBBackup object but got %T: %w", obj, ErrInvalidObjectType)
	}
	ovndbbackuplog.Info("Defaulting for OVNDBBackup", "name", ovndbbackup.GetName())

	ovndbbackup.Default()

	return nil
}

// +kubebuilder:webhook:path=/validate-ovn-openstack-org-v1beta1-ovndbbackup,mutating=false,failurePolicy=fail,sideEffects=None,groups=ovn.openstack.org,resources=ovndbbackups,verbs=create;update,versions=v1beta1,name=vovndbbackup-v1beta1.kb.io,admissionReviewVersions=v1

// OVNDBBackupCustomValidator struct is responsible for validating the OVNDBBackup resource
// when it is created, updated, or deleted.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as this struct is used only for temporary operations and does not need to be deeply copied.
type OVNDBBackupCustomValidator struct{}

var _ webhook.CustomValidator = &OVNDBBackupCustomValidator{}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type OVNDBBackup.
func (v *OVNDBBackupCustomValidator) ValidateCreate(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	ovndbbackup, ok := obj.(*ovnv1beta1.OVNDBBackup)
	if !ok {
		return nil, fmt.Errorf("expected a OVNDBBackup object but got %T: %w", obj, ErrInvalidObjectType)
	}
	ovndbbackuplog.Info("Validation for OVNDBBackup upon creation", "name", ovndbbackup.GetName())

	return ovndbbackup.ValidateCreate()
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type OVNDBBackup.
func (v *OVNDBBackupCustomValidator) ValidateUpdate(_ context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	ovndbbackup, ok := newObj.(*ovnv1beta1.OVNDBBackup)
	if !ok {
		return nil, fmt.Errorf("expected a OVNDBBackup object for the newObj but got %T: %w", newObj, ErrInvalidObjectType)
	}
	ovndbbackuplog.Info("Validation for OVNDBBackup upon update", "name", ovndbbackup.GetName())

	return ovndbbackup.ValidateUpdate(oldObj)
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type OVNDBBackup.
func (v *OVNDBBackupCustomValidator) ValidateDelete(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	ovndbbackup, ok := obj.(*ovnv1beta1.OVNDBBackup)
	if !ok {
		return nil, fmt.Errorf("expected a OVNDBBackup object but got %T: %w", obj, ErrInvalidObjectType)
	}
	ovndbbackuplog.Info("Validation for OVNDBBackup upon deletion", "name", ovndbbackup.GetName())

	return ovndbbackup.ValidateDelete()
}
