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
	"slices"
	"strings"
	"testing"

	"github.com/openstack-k8s-operators/lib-common/modules/common/probes"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/validation/field"
)

func TestOVNControllerProbes(t *testing.T) {
	spec := OVNControllerSpecCore{}
	set, err := spec.GetProbeSet()
	if err != nil {
		t.Fatal(err)
	}
	if set.Startup != nil {
		t.Fatal("startup must remain disabled without an override")
	}
	for _, p := range []*corev1.Probe{set.Liveness, set.Readiness} {
		if p.InitialDelaySeconds != 30 || p.TimeoutSeconds != 5 || p.PeriodSeconds != 5 || p.FailureThreshold != 3 || p.Exec == nil {
			t.Fatalf("unexpected default probe: %+v", p)
		}
	}
	spec.Override.Probes.LivenessProbes = &probes.ProbeConf{Type: probes.ProbeHandlerExec, TimeoutSeconds: 30}
	spec.Override.Probes.StartupProbes = &probes.ProbeConf{FailureThreshold: 60}
	set, err = spec.GetProbeSet()
	if err != nil {
		t.Fatal(err)
	}
	if set.Liveness.TimeoutSeconds != 30 || set.Readiness.TimeoutSeconds != 5 || set.Startup.FailureThreshold != 60 || set.Startup.TimeoutSeconds != 5 {
		t.Fatalf("independent partial overrides were not applied: %+v", set)
	}
	if set.Liveness.Exec.Command[0] != set.Startup.Exec.Command[0] {
		t.Fatal("startup must inherit the default liveness check")
	}
	// Returned probes must not share mutable defaults across reconciliations.
	set.Liveness.Exec.Command[0] = "changed"
	again, err := spec.GetProbeSet()
	if err != nil || again.Liveness.Exec.Command[0] == "changed" {
		t.Fatalf("probe defaults were mutated: %+v, %v", again, err)
	}
}

func TestOVNControllerProbeValidation(t *testing.T) {
	ftp := corev1.URIScheme("FTP")
	for _, tc := range []struct {
		name string
		conf probes.ProbeConf
		want string
	}{
		{"negative timeout", probes.ProbeConf{TimeoutSeconds: -1}, "timeoutSeconds"},
		{"negative period", probes.ProbeConf{PeriodSeconds: -1}, "periodSeconds"},
		{"negative failures", probes.ProbeConf{FailureThreshold: -1}, "failureThreshold"},
		{"negative delay", probes.ProbeConf{InitialDelaySeconds: -1}, "initialDelaySeconds"},
		{"unknown handler", probes.ProbeConf{Type: "TCP"}, "type"},
		{"HTTP without port", probes.ProbeConf{Type: probes.ProbeHandlerHTTP}, "invalid port"},
		{"HTTP port too large", probes.ProbeConf{Type: probes.ProbeHandlerHTTP, Port: 65536}, "invalid port"},
		{"unsupported HTTP scheme", probes.ProbeConf{Type: probes.ProbeHandlerHTTP, Port: 8080, Scheme: &ftp}, "scheme"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			spec := OVNControllerSpecCore{Override: OVNControllerOverrideSpec{Probes: probes.OverrideSpec{LivenessProbes: &tc.conf}}}
			base := field.NewPath("spec", "ovn", "template", "ovnController")
			create := spec.ValidateCreate(base, "openstack")
			update := spec.ValidateUpdate(OVNControllerSpec{}, base, "openstack")
			for _, errs := range []field.ErrorList{create, update} {
				if len(errs) == 0 || !strings.Contains(errs.ToAggregate().Error(), tc.want) || !strings.Contains(errs[0].Field, base.String()+".override.probes.livenessProbes") {
					t.Fatalf("expected validation at the parent's field path: %v", errs)
				}
			}
			if _, err := spec.GetProbeSet(); err == nil {
				t.Fatal("reconciliation must also reject invalid configurations")
			}
		})
	}
}

func TestOVNControllerCustomProbeHandlers(t *testing.T) {
	spec := OVNControllerSpecCore{Override: OVNControllerOverrideSpec{Probes: probes.OverrideSpec{
		LivenessProbes:  &probes.ProbeConf{Command: []string{"/custom/check"}},
		ReadinessProbes: &probes.ProbeConf{Type: probes.ProbeHandlerHTTP, Port: 8080, Path: "/ready"},
		StartupProbes:   &probes.ProbeConf{},
	}}}
	set, err := spec.GetProbeSet()
	if err != nil {
		t.Fatal(err)
	}
	if set.Liveness.Exec.Command[0] != "/custom/check" || set.Readiness.Exec != nil || set.Readiness.HTTPGet.Port.IntVal != 8080 || set.Startup == nil {
		t.Fatalf("custom handlers or explicit empty startup override not applied: %+v", set)
	}
}

func TestOVNControllerCustomProbeCommandIsolation(t *testing.T) {
	spec := OVNControllerSpecCore{Override: OVNControllerOverrideSpec{Probes: probes.OverrideSpec{
		LivenessProbes:  &probes.ProbeConf{Command: []string{"/custom/liveness", "--check"}},
		ReadinessProbes: &probes.ProbeConf{Type: probes.ProbeHandlerExec, Command: []string{"/custom/readiness", "--check"}},
		StartupProbes:   &probes.ProbeConf{Command: []string{"/custom/startup", "--check"}},
	}}}
	first, err := spec.GetProbeSet()
	if err != nil {
		t.Fatal(err)
	}
	second, err := spec.GetProbeSet()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name     string
		override *probes.ProbeConf
		first    *corev1.Probe
		second   *corev1.Probe
	}{
		{"liveness", spec.Override.Probes.LivenessProbes, first.Liveness, second.Liveness},
		{"readiness", spec.Override.Probes.ReadinessProbes, first.Readiness, second.Readiness},
		{"startup", spec.Override.Probes.StartupProbes, first.Startup, second.Startup},
	} {
		t.Run(tc.name, func(t *testing.T) {
			want := []string{"/custom/" + tc.name, "--check"}
			for _, probe := range []*corev1.Probe{tc.first, tc.second} {
				if probe == nil || probe.Exec == nil || !slices.Equal(probe.Exec.Command, want) {
					t.Fatalf("custom command was not preserved: %+v", probe)
				}
			}

			tc.first.Exec.Command[0] = "/changed/probe"
			if !slices.Equal(tc.override.Command, want) {
				t.Fatalf("modifying a built probe changed the CR command: %v", tc.override.Command)
			}
			if !slices.Equal(tc.second.Exec.Command, want) {
				t.Fatalf("separate probe sets share a command slice: %v", tc.second.Exec.Command)
			}

			tc.override.Command[1] = "--changed"
			if tc.first.Exec.Command[1] != want[1] || !slices.Equal(tc.second.Exec.Command, want) {
				t.Fatalf("modifying the CR command changed a built probe: %v, %v", tc.first.Exec.Command, tc.second.Exec.Command)
			}
		})
	}
}
