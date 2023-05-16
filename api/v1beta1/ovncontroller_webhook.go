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
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

// OvnControllerDefaults -
type OvnControllerDefaults struct {
	OvsContainerImageURL string
	OvnControllerContainerImageURL string
}

var ovnDefaults OvnControllerDefaults

// log is for logging in this package.
var ovncontrollerlog = logf.Log.WithName("ovncontroller-resource")

// SetupOvnControllerDefaults - initialize OVNController spec defaults for use with either internal or external webhooks
func SetupOvnControllerDefaults(defaults OvnControllerDefaults) {
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
		spec.OvsContainerImage = ovnDefaults.OvsContainerImageURL
	}
	if spec.OvnContainerImage == "" {
		spec.OvnContainerImage = ovnDefaults.OvnControllerContainerImageURL
	}
}

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
//+kubebuilder:webhook:path=/validate-ovn-openstack-org-v1beta1-ovncontroller,mutating=false,failurePolicy=fail,sideEffects=None,groups=ovn.openstack.org,resources=ovncontrollers,verbs=create;update,versions=v1beta1,name=vovncontroller.kb.io,admissionReviewVersions=v1

var _ webhook.Validator = &OVNController{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *OVNController) ValidateCreate() error {
	ovncontrollerlog.Info("validate create", "name", r.Name)

	// TODO(user): fill in your validation logic upon object creation.
	return nil
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *OVNController) ValidateUpdate(old runtime.Object) error {
	ovncontrollerlog.Info("validate update", "name", r.Name)

	// TODO(user): fill in your validation logic upon object update.
	return nil
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *OVNController) ValidateDelete() error {
	ovncontrollerlog.Info("validate delete", "name", r.Name)

	// TODO(user): fill in your validation logic upon object deletion.
	return nil
}
