// SPDX-License-Identifier: GPL-2.0-only

// Package getdp is the host-facing core of the GetDP multiphysics add-in: it turns
// host bodies into finite-element studies (surface mesh → volume mesh with physical
// groups → GetDP .pro problem definition → solve → field/table render) using only
// the Apache-2.0 oblikovati.org/api client. The cgo c-shared shell (../export.go)
// owns the C ABI; this package owns the simulation pipeline and stays cgo-free so
// it unit-tests on every platform.
package getdp

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"oblikovati.org/api/client"
	"oblikovati.org/api/wire"
	"oblikovati.org/getdp/getdp/femmodel"
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

	mu           sync.Mutex         // guards everything below
	analysis     *femmodel.Analysis // tree-owned source of truth (studies/regions/constraints)
	panel        *openPanel         // the open task-panel draft, nil when closed
	selectedNode string             // last-clicked study-tree node (Set Active target)
	running      bool               // a study is in flight (coalesces overlapping triggers)
	cancelRun    context.CancelFunc // cancels the in-flight solve (Stop command)

	// runStudy is the study pipeline entry runAndReport drives — a field so tests can
	// inject failing/panicking pipelines without a live solver (see engine_test.go).
	runStudy func(ctx context.Context) (*StudyResult, error)
}

// NewEngine binds the engine to the host transport with a default study model.
func NewEngine(host HostCaller) *Engine {
	e := &Engine{host: host, api: client.New(host), analysis: femmodel.NewAnalysis()}
	e.runStudy = e.RunStudyOnHost
	return e
}

// Notify receives host event bytes. Handlers that make host calls run on SEPARATE
// goroutines — never inline, because Notify is invoked on the host's session goroutine
// and a host call from there blocks until the frame loop drains the dispatcher (which
// cannot happen while we're inside it), deadlocking every host call.
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
	case wire.EventPanelReferencesChanged:
		e.onPanelReferencesChanged(ev)
	case wire.EventTaskPanelClosed:
		e.onTaskPanelClosed(ev)
	case wire.EventBrowserNode:
		e.onBrowserNode(ev)
	}
}

// onPanelValueChanged applies a task-panel edit to the open draft. Editing the draft
// makes no host call — safe to run inline on the session goroutine.
func (e *Engine) onPanelValueChanged(ev []byte) {
	var p struct {
		WindowId  string `json:"windowId"`
		ControlId string `json:"controlId"`
		Value     string `json:"value"`
	}
	if json.Unmarshal(ev, &p) == nil {
		e.applyPanelEdit(p.WindowId, p.ControlId, p.Value)
	}
}

// onPanelReferencesChanged applies a referenceList change (the BC editor's face picks)
// to the open draft — inline, no host call.
func (e *Engine) onPanelReferencesChanged(ev []byte) {
	var p wire.PanelReferencesChangedEvent
	if json.Unmarshal(ev, &p) == nil {
		e.applyPanelReferences(p.WindowId, p.ControlId, p.Refs)
	}
}

// onTaskPanelClosed commits or discards the open draft. Committing refreshes the tree
// (a host call), so it runs on its own goroutine.
func (e *Engine) onTaskPanelClosed(ev []byte) {
	var p wire.TaskPanelClosedEvent
	if json.Unmarshal(ev, &p) != nil {
		return
	}
	go e.closePanel(p.ID, p.Accepted)
}

// onBrowserNode routes a gesture on our study tree pane (ignoring events for other
// panes). Tree gestures open panels / run studies — host calls — so they run off the
// session goroutine.
func (e *Engine) onBrowserNode(ev []byte) {
	var b struct {
		Pane     string `json:"pane"`
		Node     string `json:"node"`
		Gesture  string `json:"gesture"`
		MenuItem string `json:"menuItem"`
	}
	if json.Unmarshal(ev, &b) == nil && b.Pane == StudyBrowserPaneID {
		e.rememberSelection(b.Node)
		go e.handleStudyNode(b.Node, b.Gesture, b.MenuItem)
	}
}

// rememberSelection records the last-clicked tree node (the Set Active target).
func (e *Engine) rememberSelection(node string) {
	e.mu.Lock()
	e.selectedNode = node
	e.mu.Unlock()
}

// launchStudy starts one study goroutine, coalescing overlapping triggers, and reports the
// outcome to the host status bar so a failed solve is visible rather than silently empty.
func (e *Engine) launchStudy() {
	e.mu.Lock()
	if e.running {
		e.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	e.running, e.cancelRun = true, cancel
	e.mu.Unlock()

	go e.runAndReport(ctx)
}

// stopStudy cancels the in-flight solve, if any.
func (e *Engine) stopStudy() {
	e.mu.Lock()
	cancel := e.cancelRun
	e.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

// runAndReport runs one study and reports its outcome, recovering from any panic in the
// pipeline so a bug cannot take down the in-process host — the failure is surfaced on the
// status bar instead.
func (e *Engine) runAndReport(ctx context.Context) {
	defer func() {
		e.mu.Lock()
		e.running, e.cancelRun = false, nil
		e.mu.Unlock()
		if r := recover(); r != nil {
			e.reportStatus(fmt.Sprintf("GetDP study crashed: %v", r))
		}
	}()
	res, err := e.runStudy(ctx)
	if err != nil {
		e.reportStatus("GetDP study failed: " + err.Error())
		return
	}
	e.reportStatus(res.Summary())
}

// reportStatus surfaces a study's outcome on the host status bar (best-effort: a status
// failure must not mask the study result).
func (e *Engine) reportStatus(msg string) { _, _ = e.api.Status().SetText(msg) }

// withAnalysis runs fn under the model lock — the single mutation seam, so tree
// refreshes always see a consistent aggregate.
func (e *Engine) withAnalysis(fn func(a *femmodel.Analysis)) {
	e.mu.Lock()
	defer e.mu.Unlock()
	fn(e.analysis)
}
