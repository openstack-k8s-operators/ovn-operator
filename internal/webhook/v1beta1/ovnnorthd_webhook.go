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
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	ovnv1beta1 "github.com/openstack-k8s-operators/ovn-operator/api/v1beta1"
)

// nolint:unused
// log is for logging in this package.
var ovnnorthdlog = logf.Log.WithName("ovnnorthd-resource")

// SetupOVNNorthdWebhookWithManager registers the webhook for OVNNorthd in the manager.
func SetupOVNNorthdWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).For(&ovnv1beta1.OVNNorthd{}).
		WithValidator(&OVNNorthdCustomValidator{}).
		WithDefaulter(&OVNNorthdCustomDefaulter{}).
		Complete()
}

// +kubebuilder:webhook:path=/mutate-ovn-openstack-org-v1beta1-ovnnorthd,mutating=true,failurePolicy=fail,sideEffects=None,groups=ovn.openstack.org,resources=ovnnorthds,verbs=create;update,versions=v1beta1,name=movnnorthd-v1beta1.kb.io,admissionReviewVersions=v1

// OVNNorthdCustomDefaulter struct is responsible for setting default values on the custom resource of the
// Kind OVNNorthd when those are created or updated.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as it is used only for temporary operations and does not need to be deeply copied.
type OVNNorthdCustomDefaulter struct {
	// TODO(user): Add more fields as needed for defaulting
}

var _ webhook.CustomDefaulter = &OVNNorthdCustomDefaulter{}

// Default implements webhook.CustomDefaulter so a webhook will be registered for the Kind OVNNorthd.
func (d *OVNNorthdCustomDefaulter) Default(_ context.Context, obj runtime.Object) error {
	ovnnorthd, ok := obj.(*ovnv1beta1.OVNNorthd)

	if !ok {
		return fmt.Errorf("expected an OVNNorthd object but got %T: %w", obj, ErrInvalidObjectType)
	}
	ovnnorthdlog.Info("Defaulting for OVNNorthd", "name", ovnnorthd.GetName())

	// Call the Default method on the OVNNorthd type
	ovnnorthd.Default()

	return nil
}

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
// NOTE: The 'path' attribute must follow a specific pattern and should not be modified directly here.
// Modifying the path for an invalid path can cause API server errors; failing to locate the webhook.
// +kubebuilder:webhook:path=/validate-ovn-openstack-org-v1beta1-ovnnorthd,mutating=false,failurePolicy=fail,sideEffects=None,groups=ovn.openstack.org,resources=ovnnorthds,verbs=create;update,versions=v1beta1,name=vovnnorthd-v1beta1.kb.io,admissionReviewVersions=v1

// OVNNorthdCustomValidator struct is responsible for validating the OVNNorthd resource
// when it is created, updated, or deleted.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as this struct is used only for temporary operations and does not need to be deeply copied.
type OVNNorthdCustomValidator struct {
	// TODO(user): Add more fields as needed for validation
}

var _ webhook.CustomValidator = &OVNNorthdCustomValidator{}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type OVNNorthd.
func (v *OVNNorthdCustomValidator) ValidateCreate(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	ovnnorthd, ok := obj.(*ovnv1beta1.OVNNorthd)
	if !ok {
		return nil, fmt.Errorf("expected a OVNNorthd object but got %T: %w", obj, ErrInvalidObjectType)
	}
	ovnnorthdlog.Info("Validation for OVNNorthd upon creation", "name", ovnnorthd.GetName())

	// Call the ValidateCreate method on the OVNNorthd type
	return ovnnorthd.ValidateCreate()
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type OVNNorthd.
func (v *OVNNorthdCustomValidator) ValidateUpdate(_ context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	ovnnorthd, ok := newObj.(*ovnv1beta1.OVNNorthd)
	if !ok {
		return nil, fmt.Errorf("expected a OVNNorthd object for the newObj but got %T: %w", newObj, ErrInvalidObjectType)
	}
	ovnnorthdlog.Info("Validation for OVNNorthd upon update", "name", ovnnorthd.GetName())

	// Call the ValidateUpdate method on the OVNNorthd type
	return ovnnorthd.ValidateUpdate(oldObj)
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type OVNNorthd.
func (v *OVNNorthdCustomValidator) ValidateDelete(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	ovnnorthd, ok := obj.(*ovnv1beta1.OVNNorthd)
	if !ok {
		return nil, fmt.Errorf("expected a OVNNorthd object but got %T: %w", obj, ErrInvalidObjectType)
	}
	ovnnorthdlog.Info("Validation for OVNNorthd upon deletion", "name", ovnnorthd.GetName())

	// Call the ValidateDelete method on the OVNNorthd type
	return ovnnorthd.ValidateDelete()
}
