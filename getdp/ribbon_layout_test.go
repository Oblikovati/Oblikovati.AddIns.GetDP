// SPDX-License-Identifier: GPL-2.0-only

package getdp

import (
	"strings"
	"testing"
)

// TestEveryCommandHasARibbonSpot keeps the layout map exhaustive: a command missing a
// spot would land unglyphed on an unnamed panel.
func TestEveryCommandHasARibbonSpot(t *testing.T) {
	for _, c := range getdpCommands {
		spot, ok := getdpRibbonSpots[c.id]
		if !ok {
			t.Errorf("command %q has no ribbon spot", c.id)
			continue
		}
		if spot.icon == "" || iconSVG(spot.icon) == "" {
			t.Errorf("command %q icon %q missing or unbundled", c.id, spot.icon)
		}
	}
	for id := range getdpRibbonSpots {
		if !commandRegistered(id) {
			t.Errorf("ribbon spot %q has no registered command", id)
		}
	}
}

func commandRegistered(id string) bool {
	for _, c := range getdpCommands {
		if c.id == id {
			return true
		}
	}
	return false
}

// TestPanelsAreTheWorkflowSet pins the panel names to the spec's workflow order set.
func TestPanelsAreTheWorkflowSet(t *testing.T) {
	allowed := map[string]bool{}
	for _, p := range getdpPanels {
		allowed[p] = true
	}
	for id, spot := range getdpRibbonSpots {
		if spot.panel != "" && !allowed[spot.panel] {
			t.Errorf("command %q sits on unknown panel %q (panels: %v)", id, spot.panel, getdpPanels)
		}
	}
}

// TestFlyoutVariantsArePopupOnly keeps the physics/BC variants off the panels: they
// are reachable through their flyout heads only.
func TestFlyoutVariantsArePopupOnly(t *testing.T) {
	for _, id := range []string{
		NewElectrokineticsCommandID, NewThermalCommandID, NewThermalTransientCommandID,
		AddVoltageCommandID, AddCurrentCommandID, AddTemperatureCommandID,
		AddHeatFluxCommandID, AddConvectionCommandID,
	} {
		if spot := getdpRibbonSpots[id]; spot.panel != "" {
			t.Errorf("flyout variant %q placed directly on panel %q", id, spot.panel)
		}
		if !inAnyFlyout(id) {
			t.Errorf("flyout variant %q not listed in any popup's items", id)
		}
	}
}

func inAnyFlyout(id string) bool {
	for _, spot := range getdpRibbonSpots {
		for _, item := range spot.items {
			if item == id {
				return true
			}
		}
	}
	return false
}

// TestPaneledCommandsRegisterOnGetDPTab drives RegisterCommands through the recording
// host and asserts every paneled command lands on the GetDP tab (popup-only variants
// carry no tab).
func TestPaneledCommandsRegisterOnGetDPTab(t *testing.T) {
	h := &recordingHost{}
	if err := NewEngine(h).RegisterCommands(); err != nil {
		t.Fatalf("RegisterCommands: %v", err)
	}
	tabs := h.createdCommandTabs()
	if len(tabs) != len(getdpCommands) {
		t.Fatalf("registered %d commands, want %d", len(tabs), len(getdpCommands))
	}
	for id, spot := range getdpRibbonSpots {
		want := ""
		if spot.panel != "" {
			want = RibbonTab
		}
		if tabs[id] != want {
			t.Errorf("command %q tab = %q, want %q", id, tabs[id], want)
		}
	}
}

// TestBCFlyoutCoversEveryConstraintKind keeps the BC flyout in lockstep with the
// shipped constraint vocabulary.
func TestBCFlyoutCoversEveryConstraintKind(t *testing.T) {
	items := getdpRibbonSpots[AddBCCommandID].items
	if len(items) != 5 {
		t.Fatalf("BC flyout lists %d kinds, want the 5 shipped kinds", len(items))
	}
	for _, item := range items {
		if !strings.HasPrefix(item, "GetDP.AddBC.") {
			t.Errorf("BC flyout item %q is not an AddBC variant", item)
		}
	}
}
