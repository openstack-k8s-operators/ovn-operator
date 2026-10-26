/*
Copyright 2026 Red Hat Inc.

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
	"github.com/openstack-k8s-operators/lib-common/modules/common/probes"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/validation/field"
)

// GetProbeSet builds the effective ovn-controller probes. Startup is opt-in.
// Keeping defaults and validation in the API also lets parent operators
// validate partial overrides against the same defaults used by reconciliation.
func (spec *OVNControllerSpecCore) GetProbeSet() (*probes.ProbeSet, error) {
	set, errors := spec.buildProbes(field.NewPath("spec", "override", "probes"))
	return set, errors.ToAggregate()
}

// ValidateProbes validates overrides and their effective configurations.
// basePath points to the probes object in either the child or parent CR.
func (spec *OVNControllerSpecCore) ValidateProbes(basePath *field.Path) field.ErrorList {
	_, errors := spec.buildProbes(basePath)
	return errors
}

func (spec *OVNControllerSpecCore) buildProbes(basePath *field.Path) (*probes.ProbeSet, field.ErrorList) {
	set := &probes.ProbeSet{}
	errors := field.ErrorList{}
	for _, probe := range []struct {
		name     string
		override *probes.ProbeConf
		script   string
		target   **corev1.Probe
	}{
		{"livenessProbes", spec.Override.Probes.LivenessProbes, "ovn_controller_liveness.sh", &set.Liveness},
		{"readinessProbes", spec.Override.Probes.ReadinessProbes, "ovn_controller_readiness.sh", &set.Readiness},
		{"startupProbes", spec.Override.Probes.StartupProbes, "ovn_controller_liveness.sh", &set.Startup},
	} {
		if probe.name == "startupProbes" && probe.override == nil {
			continue
		}
		config := probes.ProbeConf{
			Type:                probes.ProbeHandlerExec,
			Command:             []string{"/usr/local/bin/container-scripts/" + probe.script},
			InitialDelaySeconds: 30,
			TimeoutSeconds:      5,
			PeriodSeconds:       5,
			FailureThreshold:    3,
		}
		path := basePath.Child(probe.name)
		if probe.override != nil {
			// Validate before merging: Merge ignores non-positive timing values.
			// An explicitly selected Exec handler may inherit the default command.
			supplied := *probe.override
			// lib-common retains the command slice when merging and building probes.
			supplied.Command = append([]string(nil), supplied.Command...)
			if supplied.Type == probes.ProbeHandlerExec && len(supplied.Command) == 0 {
				supplied.Command = config.Command
			}
			if errs := probes.ValidateProbeConf(path, &supplied); len(errs) > 0 {
				errors = append(errors, errs...)
				continue
			}
			config.Merge(supplied)
		}
		if errs := probes.ValidateProbeConf(path, &config); len(errs) > 0 {
			errors = append(errors, errs...)
			continue
		}
		if config.Type == probes.ProbeHandlerHTTP && config.Scheme != nil &&
			*config.Scheme != "" && *config.Scheme != corev1.URISchemeHTTP && *config.Scheme != corev1.URISchemeHTTPS {
			errors = append(errors, field.NotSupported(path.Child("scheme"), *config.Scheme, []corev1.URIScheme{corev1.URISchemeHTTP, corev1.URISchemeHTTPS}))
			continue
		}
		built, err := probes.SetProbeConfV2(config)
		if err != nil {
			errors = append(errors, field.Invalid(path, nil, err.Error()))
			continue
		}
		*probe.target = built
	}
	return set, errors
}
