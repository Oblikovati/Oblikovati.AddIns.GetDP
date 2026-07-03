// SPDX-License-Identifier: GPL-2.0-only

package getdp

import (
	"strings"
	"testing"

	"oblikovati.org/api/wire"
	"oblikovati.org/getdp/getdp/femmodel"
)

func TestStudyNodesProjection(t *testing.T) {
	a := femmodel.NewAnalysis()
	s2 := a.AddStudy(femmodel.PhysicsThermalSteady) // becomes active
	if _, err := s2.AddConstraint(femmodel.ConstraintObject{Kind: femmodel.KindTemperature, Value: 300}); err != nil {
		t.Fatal(err)
	}
	nodes := studyNodes(a)
	if len(nodes) != 1 || nodes[0].ID != "studies" {
		t.Fatalf("root = %+v, want one studies node", nodes)
	}
	kids := nodes[0].Children
	if len(kids) != 2 {
		t.Fatalf("studies children = %d, want 2", len(kids))
	}
	if strings.Contains(kids[0].Label, "[active]") || !strings.Contains(kids[1].Label, "[active]") {
		t.Errorf("active suffix on wrong study: %q / %q", kids[0].Label, kids[1].Label)
	}
	assertPrereqFlags(t, kids)
}

// assertPrereqFlags checks the [!] markers: study 1 has no BCs (flagged), study 2's
// only BC has no faces (flagged on the BC row).
func assertPrereqFlags(t *testing.T, kids []wire.BrowserNodeSpec) {
	t.Helper()
	bcs1 := childByPrefix(t, kids[0], "bcs:")
	if !strings.Contains(bcs1.Label, "[!]") {
		t.Errorf("empty BC section not flagged: %q", bcs1.Label)
	}
	bcs2 := childByPrefix(t, kids[1], "bcs:")
	if len(bcs2.Children) != 1 || !strings.Contains(bcs2.Children[0].Label, "[!] no faces") {
		t.Errorf("faceless BC not flagged: %+v", bcs2.Children)
	}
}

func childByPrefix(t *testing.T, node wire.BrowserNodeSpec, prefix string) wire.BrowserNodeSpec {
	t.Helper()
	for _, c := range node.Children {
		if strings.HasPrefix(c.ID, prefix) {
			return c
		}
	}
	t.Fatalf("node %q has no %q child (children: %+v)", node.ID, prefix, node.Children)
	return wire.BrowserNodeSpec{}
}

// TestRegionsNodeShowsAirForEMStudies: an electrostatics study's Regions node carries an
// air-region child, a confined (electrokinetics) study's does not.
func TestRegionsNodeShowsAirForEMStudies(t *testing.T) {
	a := femmodel.NewAnalysis() // default study is electrokinetics (confined)
	a.AddStudy(femmodel.PhysicsElectrostatics)
	kids := studyNodes(a)[0].Children
	esRegions := childByPrefix(t, kids[len(kids)-1], "regions:")
	air := childByPrefix(t, esRegions, "air:")
	if !strings.Contains(air.Label, "automatic box") {
		t.Errorf("air node label = %q, want an automatic-box summary", air.Label)
	}
	ekRegions := childByPrefix(t, kids[0], "regions:")
	for _, c := range ekRegions.Children {
		if strings.HasPrefix(c.ID, "air:") {
			t.Errorf("confined study must not show an air node: %+v", c)
		}
	}
}

// TestTreeMenuMutationsRoundTrip drives set-active / duplicate / delete through the
// browser-node event path against the recording host.
func TestTreeMenuMutationsRoundTrip(t *testing.T) {
	h := &recordingHost{}
	e := NewEngine(h)
	var firstID string
	e.withAnalysis(func(a *femmodel.Analysis) { firstID = a.Active().ID() })

	e.Notify(browserEvent("study:"+firstID, "menu", "duplicate"))
	waitFor(t, func() bool { return countStudies(e) == 2 })
	e.Notify(browserEvent("study:"+firstID, "menu", "setactive"))
	waitFor(t, func() bool { return activeID(e) == firstID })
	e.Notify(browserEvent("study:"+firstID, "menu", "delete"))
	waitFor(t, func() bool { return countStudies(e) == 1 })
	if !h.saw(wire.MethodBrowserSetPane) {
		t.Error("tree mutations never redrew the pane")
	}
}

func browserEvent(node, gesture, menuItem string) []byte {
	return []byte(`{"type":"browser.node","pane":"` + StudyBrowserPaneID + `","node":"` + node +
		`","gesture":"` + gesture + `","menuItem":"` + menuItem + `"}`)
}

func countStudies(e *Engine) int {
	n := 0
	e.withAnalysis(func(a *femmodel.Analysis) { n = len(a.Studies()) })
	return n
}

func activeID(e *Engine) string {
	var id string
	e.withAnalysis(func(a *femmodel.Analysis) { id = a.Active().ID() })
	return id
}
