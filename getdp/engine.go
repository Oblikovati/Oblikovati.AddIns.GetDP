// SPDX-License-Identifier: GPL-2.0-only

// Package getdp is the host-facing core of the GetDP multiphysics add-in: it turns
// host bodies into finite-element studies (surface mesh → volume mesh with physical
// groups → GetDP .pro problem definition → solve → field/table render) using only
// the Apache-2.0 oblikovati.org/api client. The cgo c-shared shell (../export.go)
// owns the C ABI; this package owns the simulation pipeline and stays cgo-free so
// it unit-tests on every platform.
package getdp

import (
	"encoding/json"
	"fmt"
	"sync"

	"oblikovati.org/api/client"
	"oblikovati.org/api/wire"
)

// HostCaller is the transport the engine talks to the host through — exactly the
// api/client Caller contract, supplied by the cgo shell at Activate (or a fake in
// tests). Keeping it an interface here keeps this package cgo-free and testable.
type HostCaller interface {
	Call(method string, req []byte) ([]byte, error)
}

// Engine runs GetDP simulation studies against a live host.
type Engine struct {
	host HostCaller
	api  *client.Client

	mu      sync.Mutex // guards running
	running bool       // a study is in flight (coalesces overlapping command triggers)

	// runStudy is the study pipeline entry runAndReport drives — a field so tests can
	// inject failing/panicking pipelines without a live solver (see engine_test.go).
	runStudy func() (*StudyResult, error)
}

// NewEngine binds the engine to the host transport with the default study parameters.
func NewEngine(host HostCaller) *Engine {
	e := &Engine{host: host, api: client.New(host)}
	e.runStudy = e.RunStudyOnHost
	return e
}

// Notify receives host event bytes. A command.started carrying RunStudyCommandID runs the
// study on a SEPARATE goroutine — never inline, because Notify is invoked on the host's
// session goroutine and a host call from there blocks until the frame loop drains the
// dispatcher (which cannot happen while we're inside it), deadlocking every host call. A
// guard coalesces overlapping triggers so one study is in flight at a time.
func (e *Engine) Notify(ev []byte) {
	var hdr struct {
		Type string `json:"type"`
	}
	if json.Unmarshal(ev, &hdr) != nil {
		return
	}
	switch hdr.Type {
	case wire.EventCommandStarted:
		e.onCommandStarted(ev)
	case wire.EventPanelValueChanged:
		e.onPanelValueChanged(ev)
	case wire.EventBrowserNode:
		e.onBrowserNode(ev)
	}
}

// onCommandStarted dispatches our registered commands. The study runs through
// launchStudy's coalescing guard on its own goroutine (host calls from the session
// goroutine deadlock the dispatcher — same rule as Notify).
func (e *Engine) onCommandStarted(ev []byte) {
	var c struct {
		Command string `json:"command"`
	}
	if json.Unmarshal(ev, &c) != nil {
		return
	}
	if c.Command == RunStudyCommandID {
		e.launchStudy()
	}
}

// onPanelValueChanged applies a panel edit. Editing engine state makes no host call —
// safe to run inline on the session goroutine. No panels exist yet (the task-panel
// editors land with the M3 UI slice); events for other windows are ignored by ID.
func (e *Engine) onPanelValueChanged(ev []byte) {
	var p struct {
		WindowId  string `json:"windowId"`
		ControlId string `json:"controlId"`
		Value     string `json:"value"`
	}
	_ = json.Unmarshal(ev, &p) // unknown windows fall through: nothing to apply yet
}

// onBrowserNode dispatches a gesture on our study tree pane (ignoring events for other
// panes). The tree itself lands with the M3 UI slice.
func (e *Engine) onBrowserNode(ev []byte) {
	var b struct {
		Pane    string `json:"pane"`
		Node    string `json:"node"`
		Gesture string `json:"gesture"`
	}
	_ = json.Unmarshal(ev, &b) // no pane registered yet: nothing to route
}

// launchStudy starts one study goroutine, coalescing overlapping triggers, and reports the
// outcome to the host status bar so a failed solve is visible rather than silently empty.
func (e *Engine) launchStudy() {
	e.mu.Lock()
	if e.running {
		e.mu.Unlock()
		return
	}
	e.running = true
	e.mu.Unlock()

	go e.runAndReport()
}

// runAndReport runs one study and reports its outcome, recovering from any panic in the
// pipeline so a bug cannot take down the in-process host — the failure is surfaced on the
// status bar instead.
func (e *Engine) runAndReport() {
	defer func() {
		e.mu.Lock()
		e.running = false
		e.mu.Unlock()
		if r := recover(); r != nil {
			e.reportStatus(fmt.Sprintf("GetDP study crashed: %v", r))
		}
	}()
	res, err := e.runStudy()
	if err != nil {
		e.reportStatus("GetDP study failed: " + err.Error())
		return
	}
	e.reportStatus(res.Summary())
}

// reportStatus surfaces a study's outcome on the host status bar (best-effort: a status
// failure must not mask the study result).
func (e *Engine) reportStatus(msg string) { _, _ = e.api.Status().SetText(msg) }
