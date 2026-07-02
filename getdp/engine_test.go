// SPDX-License-Identifier: GPL-2.0-only

package getdp

import (
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"oblikovati.org/api/wire"
)

// recordingHost is a named fake HostCaller (no live host): it records the wire methods
// it is asked to call and returns an empty OK body, enough to drive the M0 scaffold
// (command registration, the Notify → study → status path).
type recordingHost struct {
	mu          sync.Mutex
	calls       []string
	statusTexts []string                 // every status.setText message, for outcome assertions
	createdCmds []wire.CreateCommandArgs // every commands.create request, decoded for placement assertions
}

func (h *recordingHost) Call(method string, payload []byte) ([]byte, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.calls = append(h.calls, method)
	switch method {
	case wire.MethodCommandsCreate:
		var a wire.CreateCommandArgs
		if json.Unmarshal(payload, &a) == nil {
			h.createdCmds = append(h.createdCmds, a)
		}
	case wire.MethodStatusSetText:
		var s struct {
			Text string `json:"text"`
		}
		if json.Unmarshal(payload, &s) == nil {
			h.statusTexts = append(h.statusTexts, s.Text)
		}
	}
	return []byte("{}"), nil
}

// createdCommandTabs decodes every recorded MethodCommandsCreate payload and returns
// a map of command ID → Tab so tests can assert placement without knowing order.
func (h *recordingHost) createdCommandTabs() map[string]string {
	h.mu.Lock()
	defer h.mu.Unlock()
	tabs := make(map[string]string, len(h.createdCmds))
	for _, a := range h.createdCmds {
		tabs[a.ID] = a.Tab
	}
	return tabs
}

func (h *recordingHost) saw(method string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, m := range h.calls {
		if m == method {
			return true
		}
	}
	return false
}

func (h *recordingHost) callCount() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.calls)
}

// lastStatus returns the most recent status.setText message, or "".
func (h *recordingHost) lastStatus() string {
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.statusTexts) == 0 {
		return ""
	}
	return h.statusTexts[len(h.statusTexts)-1]
}

func TestSetupRegistersCommands(t *testing.T) {
	h := &recordingHost{}
	if err := NewEngine(h).Setup(); err != nil {
		t.Fatalf("Setup: %v", err)
	}
	if !h.saw(wire.MethodCommandsCreate) {
		t.Errorf("Setup never called %q (calls: %v)", wire.MethodCommandsCreate, h.calls)
	}
}

func TestRegisteredCommandsLandOnGetDPTab(t *testing.T) {
	h := &recordingHost{}
	if err := NewEngine(h).RegisterCommands(); err != nil {
		t.Fatalf("RegisterCommands: %v", err)
	}
	got := h.createdCommandTabs()
	for _, id := range []string{RunStudyCommandID} {
		if got[id] != RibbonTab {
			t.Errorf("command %q on tab %q, want %q", id, got[id], RibbonTab)
		}
	}
}

func TestRunStudyOnHostErrorsWhilePipelineUnimplemented(t *testing.T) {
	// With no pipeline yet, the study must fail loudly rather than silently doing nothing.
	if _, err := NewEngine(&recordingHost{}).RunStudyOnHost(); err == nil {
		t.Fatal("RunStudyOnHost should error while the pipeline is unimplemented")
	}
}

// TestNotifyCommandStartedRunsStudy verifies a command.started event for our command
// drives the study path and reports a status message (the M0 not-implemented failure).
func TestNotifyCommandStartedRunsStudy(t *testing.T) {
	h := &recordingHost{}
	e := NewEngine(h)
	e.Notify(commandStartedEvent(RunStudyCommandID))
	waitIdle(e)
	if !h.saw(wire.MethodStatusSetText) {
		t.Errorf("study run never reported status (calls: %v)", h.calls)
	}
	if got := h.lastStatus(); !strings.Contains(got, "GetDP study failed") {
		t.Errorf("status = %q, want the not-implemented failure surfaced", got)
	}
}

// TestNotifyIgnoresForeignCommand verifies the command dispatch drops command.started
// events for commands we did not register (no goroutine, no host call).
func TestNotifyIgnoresForeignCommand(t *testing.T) {
	h := &recordingHost{}
	e := NewEngine(h)
	e.Notify(commandStartedEvent("SomeOtherAddIn.DoThing"))
	waitIdle(e)
	if n := h.callCount(); n != 0 {
		t.Errorf("foreign command triggered %d host calls, want 0 (calls: %v)", n, h.calls)
	}
}

// TestNotifyPanelAndBrowserEventsAreSafe verifies the panel.valueChanged and
// browser.node dispatch arms parse and drop events while no panel/pane is registered —
// no panic, no host call.
func TestNotifyPanelAndBrowserEventsAreSafe(t *testing.T) {
	h := &recordingHost{}
	e := NewEngine(h)
	e.Notify([]byte(`{"type":"` + wire.EventPanelValueChanged + `","windowId":"other.panel","controlId":"x","value":"1"}`))
	e.Notify([]byte(`{"type":"` + wire.EventBrowserNode + `","pane":"other.tree","node":"n","gesture":"click"}`))
	e.Notify([]byte(`not json`))
	if n := h.callCount(); n != 0 {
		t.Errorf("panel/browser events triggered %d host calls, want 0", n)
	}
}

// TestRunAndReportRecoversPanic injects a panicking pipeline and asserts the panic is
// recovered and surfaced on the status bar instead of crashing the host process.
func TestRunAndReportRecoversPanic(t *testing.T) {
	h := &recordingHost{}
	e := NewEngine(h)
	e.runStudy = func() (*StudyResult, error) { panic("mesh exploded") }
	e.Notify(commandStartedEvent(RunStudyCommandID))
	waitIdle(e)
	if got := h.lastStatus(); !strings.Contains(got, "GetDP study crashed") || !strings.Contains(got, "mesh exploded") {
		t.Errorf("status = %q, want the recovered panic surfaced", got)
	}
}

// TestLaunchStudyCoalescesOverlappingTriggers holds one study in flight and asserts a
// second trigger is dropped rather than queued.
func TestLaunchStudyCoalescesOverlappingTriggers(t *testing.T) {
	h := &recordingHost{}
	e := NewEngine(h)
	release := make(chan struct{})
	started := make(chan struct{})
	e.runStudy = func() (*StudyResult, error) {
		close(started)
		<-release
		return &StudyResult{SummaryText: "done"}, nil
	}
	e.launchStudy()
	<-started
	e.launchStudy() // coalesced: the first run is still in flight
	close(release)
	waitIdle(e)
	h.mu.Lock()
	runs := len(h.statusTexts)
	h.mu.Unlock()
	if runs != 1 {
		t.Errorf("got %d study reports, want 1 (second trigger must coalesce)", runs)
	}
}

// waitIdle blocks until the study goroutine launched by Notify has finished.
func waitIdle(e *Engine) {
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		e.mu.Lock()
		running := e.running
		e.mu.Unlock()
		if !running {
			return
		}
		time.Sleep(time.Millisecond)
	}
}

// commandStartedEvent builds the bytes for a command.started event carrying the given command id.
func commandStartedEvent(id string) []byte {
	ev, _ := json.Marshal(map[string]string{"type": wire.EventCommandStarted, "command": id})
	return ev
}
