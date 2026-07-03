// SPDX-License-Identifier: GPL-2.0-only

package getdp

import (
	"encoding/json"
	"strings"
	"testing"

	"oblikovati.org/api/wire"
	"oblikovati.org/getdp/getdp/femmodel"
)

// panelValueEvent / panelClosedEvent build the host events the panel loop consumes.
func panelValueEvent(windowID, controlID, value string) []byte {
	ev, _ := json.Marshal(map[string]string{"type": wire.EventPanelValueChanged,
		"windowId": windowID, "controlId": controlID, "value": value})
	return ev
}

func panelClosedEvent(windowID string, accepted bool) []byte {
	ev, _ := json.Marshal(map[string]any{"type": wire.EventTaskPanelClosed,
		"id": windowID, "accepted": accepted})
	return ev
}

// TestMeshPanelEditCommitRoundTrip opens TP-11, edits, accepts, and asserts the
// aggregate took the values.
func TestMeshPanelEditCommitRoundTrip(t *testing.T) {
	e := NewEngine(&recordingHost{})
	e.openMeshPanel(activeID(e))
	e.Notify(panelValueEvent(tpMeshID, "size", "0.25"))
	e.Notify(panelValueEvent(tpMeshID, "secondorder", "true"))
	e.Notify(panelClosedEvent(tpMeshID, true))
	waitFor(t, func() bool { return openPanelIsNil(e) })
	var mesh femmodel.MeshObject
	e.withAnalysis(func(a *femmodel.Analysis) { mesh = a.Active().Mesh })
	if mesh.SizeModelUnits != 0.25 || !mesh.SecondOrder {
		t.Errorf("mesh after commit = %+v, want size 0.25 second-order", mesh)
	}
}

// TestPanelCancelDiscardsDraft edits then cancels: the aggregate must be untouched.
func TestPanelCancelDiscardsDraft(t *testing.T) {
	e := NewEngine(&recordingHost{})
	e.openMeshPanel(activeID(e))
	e.Notify(panelValueEvent(tpMeshID, "size", "9"))
	e.Notify(panelClosedEvent(tpMeshID, false))
	waitFor(t, func() bool { return openPanelIsNil(e) })
	var mesh femmodel.MeshObject
	e.withAnalysis(func(a *femmodel.Analysis) { mesh = a.Active().Mesh })
	if mesh.SizeModelUnits != 0 {
		t.Errorf("cancel leaked draft into the aggregate: %+v", mesh)
	}
}

// TestConstraintPanelReferencesAndCommit drives the BC editor end to end: face picks
// arrive as reference events, values as edits, and OK appends the constraint.
func TestConstraintPanelReferencesAndCommit(t *testing.T) {
	e := NewEngine(&recordingHost{})
	e.showConstraintPanel(activeID(e), femmodel.ConstraintObject{
		Kind: femmodel.KindVoltage, Name: "voltage"}, true)
	refs, _ := json.Marshal(map[string]any{"type": wire.EventPanelReferencesChanged,
		"windowId": tpConstraintID, "controlId": "faces",
		"refs": []string{encodeFaceRef("faceA"), encodeFaceRef("faceB")}})
	e.Notify(refs)
	e.Notify(panelValueEvent(tpConstraintID, "value", "24"))
	e.Notify(panelClosedEvent(tpConstraintID, true))
	waitFor(t, func() bool { return openPanelIsNil(e) })
	var cs []femmodel.ConstraintObject
	e.withAnalysis(func(a *femmodel.Analysis) { cs = a.Active().Constraints() })
	if len(cs) != 1 || cs[0].Value != 24 || len(cs[0].Faces) != 2 || cs[0].Faces[0] != "faceA" {
		t.Errorf("committed constraint = %+v, want 24 V on faceA/faceB", cs)
	}
}

// TestStudyPanelPhysicsSwitchDropsIncompatibleBCs switches an electrokinetic study to
// thermal through TP-1 and asserts the incompatible BC is dropped WITH a report.
func TestStudyPanelPhysicsSwitchDropsIncompatibleBCs(t *testing.T) {
	h := &recordingHost{}
	e := NewEngine(h)
	e.withAnalysis(func(a *femmodel.Analysis) {
		if _, err := a.Active().AddConstraint(femmodel.ConstraintObject{Kind: femmodel.KindVoltage, Value: 5}); err != nil {
			t.Fatal(err)
		}
	})
	e.openStudyPanel(activeID(e))
	e.Notify(panelValueEvent(tpStudyID, "physics", string(femmodel.PhysicsThermalSteady)))
	e.Notify(panelClosedEvent(tpStudyID, true))
	waitFor(t, func() bool {
		return strings.Contains(h.lastStatus(), "incompatible boundary condition")
	})
	var physics femmodel.PhysicsKind
	var nBCs int
	e.withAnalysis(func(a *femmodel.Analysis) {
		physics, nBCs = a.Active().Solver.Physics, len(a.Active().Constraints())
	})
	if physics != femmodel.PhysicsThermalSteady || nBCs != 0 {
		t.Errorf("after switch: physics=%s bcs=%d, want thermal with 0 BCs", physics, nBCs)
	}
}

// TestAddBCCommandRejectsIncompatibleKind fires the Temperature flyout on an
// electrokinetic study: the panel must not open, the status must explain.
func TestAddBCCommandRejectsIncompatibleKind(t *testing.T) {
	h := &recordingHost{}
	e := NewEngine(h)
	e.Notify(commandStartedEvent(AddTemperatureCommandID))
	waitFor(t, func() bool { return strings.Contains(h.lastStatus(), "does not apply") })
	if !openPanelIsNil(e) {
		t.Error("incompatible BC kind still opened its editor")
	}
}

// TestNewStudyCommandAddsAndOpensPanel drives the flyout variant end to end.
func TestNewStudyCommandAddsAndOpensPanel(t *testing.T) {
	h := &recordingHost{}
	e := NewEngine(h)
	e.Notify(commandStartedEvent(NewThermalCommandID))
	waitFor(t, func() bool { return countStudies(e) == 2 })
	waitFor(t, func() bool { return h.saw(wire.MethodTaskPanelShow) })
	var physics femmodel.PhysicsKind
	e.withAnalysis(func(a *femmodel.Analysis) { physics = a.Active().Solver.Physics })
	if physics != femmodel.PhysicsThermalSteady {
		t.Errorf("new study physics = %s, want thermal", physics)
	}
}

func openPanelIsNil(e *Engine) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.panel == nil
}
