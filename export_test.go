// SPDX-License-Identifier: GPL-2.0-only

package main

import (
	"encoding/json"
	"testing"
	"time"

	"oblikovati.org/api"
)

// TestActivateDeactivateRoundTrip drives the C ABI entry points against a fake host
// call table (shellharness.go): Activate must start the engine (whose async Setup
// reaches the host), re-Activate must be idempotent, and Deactivate must drop the
// engine so Notify becomes a no-op — the load contract the host relies on (issue #1).
func TestActivateDeactivateRoundTrip(t *testing.T) {
	if rc := activateWithFakeHost(); rc != 0 {
		t.Fatalf("Activate = %d, want OBK_OK", rc)
	}
	if rc := activateWithFakeHost(); rc != 0 {
		t.Fatalf("second Activate = %d, want idempotent OBK_OK", rc)
	}
	waitForSetupCall(t)
	if rc := deactivate(); rc != 0 {
		t.Fatalf("Deactivate = %d, want OBK_OK", rc)
	}
	// After Deactivate the engine is gone: Notify must be a safe no-op.
	ev := []byte(`{"type":"command.started","command":"GetDP.RunStudy"}`)
	if rc := notifyBytes(ev); rc != 0 {
		t.Fatalf("Notify after Deactivate = %d, want OBK_OK", rc)
	}
}

// waitForSetupCall blocks until the Activate-spawned Setup goroutine has made at least
// one host call through the fake table (proving Setup runs OFF the caller's goroutine).
func waitForSetupCall(t *testing.T) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if fakeHostCallsSeen() > 0 {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("Setup never reached the fake host call table within 5 s")
}

// TestExportedIdentityAndVersions pins the load-gate exports: the stable add-in id,
// a manifest that parses and declares the parameters capability (the optimization
// milestone needs the Parameters API), and API major/minor matching the compiled-
// against oblikovati.org/api module.
func TestExportedIdentityAndVersions(t *testing.T) {
	if got := exportedID(); got != addInID {
		t.Errorf("ObkAddInId = %q, want %q", got, addInID)
	}
	var man struct {
		ID           string   `json:"id"`
		Capabilities []string `json:"capabilities"`
	}
	if err := json.Unmarshal([]byte(exportedManifest()), &man); err != nil {
		t.Fatalf("manifest is not valid JSON: %v", err)
	}
	if man.ID != addInID {
		t.Errorf("manifest id = %q, want %q", man.ID, addInID)
	}
	if !containsString(man.Capabilities, "parameters") {
		t.Errorf("manifest capabilities %v missing %q", man.Capabilities, "parameters")
	}
	if got := apiMajorExport(); got != api.Major() {
		t.Errorf("ObkAddInApiMajor = %d, want %d", got, api.Major())
	}
	if got := apiMinorExport(); got != api.Minor() {
		t.Errorf("ObkAddInApiMinor = %d, want %d", got, api.Minor())
	}
}

func containsString(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}
