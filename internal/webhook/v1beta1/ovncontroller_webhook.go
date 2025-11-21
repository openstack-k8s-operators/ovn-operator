/*
Copyright 2025.

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

// Package v1beta1 implements webhook handlers for OVN v1beta1 API resources.
package v1beta1

import (
	"context"
	"errors"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	ovnv1beta1 "github.com/openstack-k8s-operators/ovn-operator/api/v1beta1"
)

var (
	// ErrInvalidObjectType is returned when an unexpected object type is provided
	ErrInvalidObjectType = errors.New("invalid object type")
)

// nolint:unused
// log is for logging in this package.
var ovncontrollerlog = logf.Log.WithName("ovncontroller-resource")

// SetupOVNControllerWebhookWithManager registers the webhook for OVNController in the manager.
func SetupOVNControllerWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).For(&ovnv1beta1.OVNController{}).
		WithValidator(&OVNControllerCustomValidator{}).
		WithDefaulter(&OVNControllerCustomDefaulter{}).
		Complete()
}

// +kubebuilder:webhook:path=/mutate-ovn-openstack-org-v1beta1-ovncontroller,mutating=true,failurePolicy=fail,sideEffects=None,groups=ovn.openstack.org,resources=ovncontrollers,verbs=create;update,versions=v1beta1,name=movncontroller-v1beta1.kb.io,admissionReviewVersions=v1

// OVNControllerCustomDefaulter struct is responsible for setting default values on the custom resource of the
// Kind OVNController when those are created or updated.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as it is used only for temporary operations and does not need to be deeply copied.
type OVNControllerCustomDefaulter struct {
	// TODO(user): Add more fields as needed for defaulting
}

var _ webhook.CustomDefaulter = &OVNControllerCustomDefaulter{}

// Default implements webhook.CustomDefaulter so a webhook will be registered for the Kind OVNController.
func (d *OVNControllerCustomDefaulter) Default(_ context.Context, obj runtime.Object) error {
	ovncontroller, ok := obj.(*ovnv1beta1.OVNController)

	if !ok {
		return fmt.Errorf("expected an OVNController object but got %T: %w", obj, ErrInvalidObjectType)
	}
	ovncontrollerlog.Info("Defaulting for OVNController", "name", ovncontroller.GetName())

	// Call the Default method on the OVNController type
	ovncontroller.Default()

	return nil
}

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
// NOTE: The 'path' attribute must follow a specific pattern and should not be modified directly here.
// Modifying the path for an invalid path can cause API server errors; failing to locate the webhook.
// +kubebuilder:webhook:path=/validate-ovn-openstack-org-v1beta1-ovncontroller,mutating=false,failurePolicy=fail,sideEffects=None,groups=ovn.openstack.org,resources=ovncontrollers,verbs=create;update,versions=v1beta1,name=vovncontroller-v1beta1.kb.io,admissionReviewVersions=v1

// OVNControllerCustomValidator struct is responsible for validating the OVNController resource
// when it is created, updated, or deleted.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as this struct is used only for temporary operations and does not need to be deeply copied.
type OVNControllerCustomValidator struct {
	// TODO(user): Add more fields as needed for validation
}

var _ webhook.CustomValidator = &OVNControllerCustomValidator{}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type OVNController.
func (v *OVNControllerCustomValidator) ValidateCreate(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	ovncontroller, ok := obj.(*ovnv1beta1.OVNController)
	if !ok {
		return nil, fmt.Errorf("expected a OVNController object but got %T: %w", obj, ErrInvalidObjectType)
	}
	ovncontrollerlog.Info("Validation for OVNController upon creation", "name", ovncontroller.GetName())

	// Call the ValidateCreate method on the OVNController type
	return ovncontroller.ValidateCreate()
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type OVNController.
func (v *OVNControllerCustomValidator) ValidateUpdate(_ context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	ovncontroller, ok := newObj.(*ovnv1beta1.OVNController)
	if !ok {
		return nil, fmt.Errorf("expected a OVNController object for the newObj but got %T: %w", newObj, ErrInvalidObjectType)
	}
	ovncontrollerlog.Info("Validation for OVNController upon update", "name", ovncontroller.GetName())

	// Call the ValidateUpdate method on the OVNController type
	return ovncontroller.ValidateUpdate(oldObj)
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type OVNController.
func (v *OVNControllerCustomValidator) ValidateDelete(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	ovncontroller, ok := obj.(*ovnv1beta1.OVNController)
	if !ok {
		return nil, fmt.Errorf("expected a OVNController object but got %T: %w", obj, ErrInvalidObjectType)
	}
	ovncontrollerlog.Info("Validation for OVNController upon deletion", "name", ovncontroller.GetName())

	// Call the ValidateDelete method on the OVNController type
	return ovncontroller.ValidateDelete()
}
