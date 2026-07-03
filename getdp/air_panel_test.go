// SPDX-License-Identifier: GPL-2.0-only

package getdp

import (
	"testing"

	"oblikovati.org/api/wire"
	"oblikovati.org/getdp/getdp/femmodel"
)

// controlIDs lists a panel's control ids in order — the shape assertions ride on this.
func controlIDs(cs []wire.PanelControlSpec) []string {
	ids := make([]string, len(cs))
	for i, c := range cs {
		ids[i] = c.ID
	}
	return ids
}

func sameIDs(got []string, want ...string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

// TestAirControlsRenderPerMode pins which controls each air mode exposes: the automatic box
// shows padding + the shell toggle (and the radii only once the shell is on); Manual and None
// collapse to the mode dropdown plus a hint. This is the "mode switching re-renders controls"
// requirement at the control-builder level.
func TestAirControlsRenderPerMode(t *testing.T) {
	auto := &openPanel{air: femmodel.AirRegion{Mode: femmodel.AirAutomaticBox, PaddingFactor: 3}}
	if got := controlIDs(airControls(auto)); !sameIDs(got, "mode", "padding", "infshell", "hint") {
		t.Errorf("automatic-box controls = %v, want mode/padding/infshell/hint", got)
	}
	shell := &openPanel{air: femmodel.AirRegion{Mode: femmodel.AirAutomaticBox, Truncation: femmodel.TruncationInfiniteShell}}
	if got := controlIDs(airControls(shell)); !sameIDs(got, "mode", "padding", "infshell", "rint", "rext", "hint") {
		t.Errorf("infinite-shell controls = %v, want the two radii to appear", got)
	}
	none := &openPanel{air: femmodel.AirRegion{Mode: femmodel.AirNone}}
	if got := controlIDs(airControls(none)); !sameIDs(got, "mode", "hint") {
		t.Errorf("None controls = %v, want mode/hint only", got)
	}
	manual := &openPanel{air: femmodel.AirRegion{Mode: femmodel.AirManualBodies}}
	if got := controlIDs(airControls(manual)); !sameIDs(got, "mode", "hint") {
		t.Errorf("Manual controls = %v, want mode/hint only", got)
	}
}

// TestAirModeRoundTrip checks the enum⇄label mapping the dropdown depends on.
func TestAirModeRoundTrip(t *testing.T) {
	for _, m := range []femmodel.AirMode{femmodel.AirNone, femmodel.AirAutomaticBox, femmodel.AirManualBodies} {
		if got := parseAirMode(airModeLabel(m)); got != m {
			t.Errorf("round trip of mode %d = %d", m, got)
		}
	}
}

// TestAirPanelPaddingCommitRoundTrip opens TP-3 on an electrostatics study (default automatic
// box), edits the padding, accepts, and asserts the study's air region took the value.
func TestAirPanelPaddingCommitRoundTrip(t *testing.T) {
	e := NewEngine(&recordingHost{})
	e.withAnalysis(func(a *femmodel.Analysis) { a.AddStudy(femmodel.PhysicsElectrostatics) })
	id := activeID(e)
	e.openAirPanel(id)
	e.Notify(panelValueEvent(tpAirID, "padding", "4.5"))
	e.Notify(panelClosedEvent(tpAirID, true))
	waitFor(t, func() bool { return openPanelIsNil(e) })
	var air femmodel.AirRegion
	e.withAnalysis(func(a *femmodel.Analysis) { air = a.Active().Solver.Air })
	if air.Mode != femmodel.AirAutomaticBox || air.PaddingFactor != 4.5 {
		t.Errorf("air after commit = %+v, want automatic box padding 4.5", air)
	}
}

// TestAirPanelModeSwitchReRenders drives a mode edit and asserts the host was asked to
// re-show TP-3 (so the controls change with the mode), and that Cancel leaves the model's
// air region untouched.
func TestAirPanelModeSwitchReRenders(t *testing.T) {
	h := &recordingHost{}
	e := NewEngine(h)
	e.withAnalysis(func(a *femmodel.Analysis) { a.AddStudy(femmodel.PhysicsElectrostatics) })
	e.openAirPanel(activeID(e))
	shows := h.count(wire.MethodTaskPanelShow)
	e.Notify(panelValueEvent(tpAirID, "mode", "None"))
	waitFor(t, func() bool { return h.count(wire.MethodTaskPanelShow) > shows })
	e.Notify(panelClosedEvent(tpAirID, false))
	waitFor(t, func() bool { return openPanelIsNil(e) })
	var air femmodel.AirRegion
	e.withAnalysis(func(a *femmodel.Analysis) { air = a.Active().Solver.Air })
	if air.Mode != femmodel.AirAutomaticBox {
		t.Errorf("cancel leaked a mode switch into the model: %+v", air)
	}
}

// TestAirRegionCommandOpensPanel fires the ribbon command and asserts TP-3 opens.
func TestAirRegionCommandOpensPanel(t *testing.T) {
	h := &recordingHost{}
	e := NewEngine(h)
	e.withAnalysis(func(a *femmodel.Analysis) { a.AddStudy(femmodel.PhysicsElectrostatics) })
	e.Notify(commandStartedEvent(AirRegionCommandID))
	waitFor(t, func() bool { return h.saw(wire.MethodTaskPanelShow) })
	if iconSVG("airregion") == "" {
		t.Error("air-region ribbon icon not bundled")
	}
}
