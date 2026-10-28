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

package ovncontroller

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/openstack-k8s-operators/lib-common/modules/common/probes"
	ovnv1 "github.com/openstack-k8s-operators/ovn-operator/api/v1beta1"
)

func TestDaemonSetRejectsInvalidProbes(t *testing.T) {
	instance := &ovnv1.OVNController{}
	instance.Spec.Override.Probes.LivenessProbes = &probes.ProbeConf{Type: probes.ProbeHandlerHTTP}
	ds, err := CreateOVNDaemonSet(instance, "hash", nil, nil)
	if err == nil || ds != nil {
		t.Fatalf("expected construction error, got %v, %v", ds, err)
	}
}

func TestControllerProbeScripts(t *testing.T) {
	dir := t.TempDir()
	for name, script := range map[string]string{
		"pidof":      "#!/bin/bash\nexit \"${PIDOF_STATUS:-0}\"\n",
		"ovn-appctl": "#!/bin/bash\nprintf '%s\\n' \"$APPCTL_OUTPUT\"\nexit \"${APPCTL_STATUS:-0}\"\n",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(script), 0o700); err != nil { //nolint:gosec // Executable test stubs in a private temporary directory.
			t.Fatal(err)
		}
	}
	for _, tc := range []struct {
		name, script, pidStatus, appStatus, output, want string
	}{
		{"live", "liveness", "0", "0", "running", ""},
		{"ready", "readiness", "0", "0", "connected", ""},
		{"missing process", "liveness", "1", "0", "running", "not running"},
		{"paused", "liveness", "0", "0", "paused", "expecting 'running'"},
		{"disconnected", "readiness", "0", "0", "not connected", "expecting 'connected'"},
		{"liveness RPC failure", "liveness", "0", "7", "socket unavailable", "exit status: 7, output: socket unavailable"},
		{"readiness RPC failure", "readiness", "0", "9", "socket unavailable", "exit status: 9, output: socket unavailable"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, "bash", "../../templates/ovncontroller/bin/ovn_controller_"+tc.script+".sh") //nolint:gosec // Only fixed test case script names are used.
			cmd.Env = append(os.Environ(), "PATH="+dir+":"+os.Getenv("PATH"), "PIDOF_STATUS="+tc.pidStatus, "APPCTL_STATUS="+tc.appStatus, "APPCTL_OUTPUT="+tc.output)
			output, err := cmd.CombinedOutput()
			if tc.want == "" {
				if err != nil {
					t.Fatalf("healthy probe failed: %v: %s", err, output)
				}
			} else if err == nil || !strings.Contains(string(output), tc.want) {
				t.Fatalf("expected failure containing %q, got %v: %s", tc.want, err, output)
			}
		})
	}
}
