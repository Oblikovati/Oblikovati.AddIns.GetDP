// SPDX-License-Identifier: GPL-2.0-only

package getdp

import (
	"strings"
	"testing"
)

// shellGoldenInput builds a deterministic infinite-shell electrostatics deck input by hand: a
// dielectric part (εr=4), a near-air ball (εr=1), an infinite shell volume (εr=1) carrying the
// VolSphShell transform, an electrode at 1 V and the far-field (the Rext sphere) at 0 V. The
// sphere is at the origin with Rint=0.03 m, Rext=0.06 m.
func shellGoldenInput() DeckInput {
	regions := &RegionTable{
		Volumes: []VolumeRegion{
			{Tag: 1, Name: "Dielectric", Body: 0},
			{Tag: 2, Name: "Air", Body: airBodyIndex},
			{Tag: 3, Name: "Shell", Body: shellBodyIndex},
		},
		Surfaces: []SurfaceRegion{
			{Tag: 4, Name: "V+"},
			{Tag: 5, Name: "FarField"},
		},
	}
	model := &SolveModel{BoundPotentials: []BoundPotential{
		{Kind: KindVoltage, RegionTag: 4, Name: "V+", Value: 1},
		{Kind: KindVoltage, RegionTag: 5, Name: "FarField", Value: 0},
	}}
	return DeckInput{
		Regions:   regions,
		Model:     model,
		Materials: map[int]Material{1: {Epsilon: 4}, 2: {Epsilon: 1}, 3: {Epsilon: 1}},
		Order:     1,
		Shell:     &ShellTransform{VolumeTag: 3, Rint: 0.03, Rext: 0.06},
	}
}

// TestElectrostaticsDeckEmitsShellJacobian pins the Jacobian fragment for an infinite-shell
// study (acceptance criterion #25): the shell volume maps to VolSphShell{Rint,Rext,Cx,Cy,Cz},
// everything else to Vol. The shell case must precede the All catch-all.
func TestElectrostaticsDeckEmitsShellJacobian(t *testing.T) {
	deck, _, err := ElectrostaticsWriter{}.BuildDeck(shellGoldenInput())
	if err != nil {
		t.Fatalf("BuildDeck: %v", err)
	}
	const wantJac = `Jacobian {
  { Name JVol; Case {
      { Region Vol3; Jacobian VolSphShell{0.03, 0.06, 0, 0, 0}; }
      { Region All; Jacobian Vol; }
  } }
  { Name JSur; Case {
      { Region All; Jacobian Sur; }
  } }
}
`
	got := deck.Render()
	if !strings.Contains(got, wantJac) {
		t.Errorf("infinite-shell deck missing shell Jacobian fragment:\n--- got ---\n%s\n--- want fragment ---\n%s", got, wantJac)
	}
	// The energy/capacitance integrals must still run over VolAll (the shell included), so the
	// mapped-to-infinity energy is counted. VolAll must list the shell group.
	if !strings.Contains(got, "VolAll = Region[{Vol1, Vol2, Vol3}];") {
		t.Errorf("VolAll must include the shell volume so its energy is integrated:\n%s", got)
	}
}

// TestElectrostaticsDeckNoShellByDefault: without a ShellTransform the deck keeps the plain
// standard Jacobians (no regression to the padded-box path).
func TestElectrostaticsDeckNoShellByDefault(t *testing.T) {
	deck, _, err := ElectrostaticsWriter{}.BuildDeck(electrostaticGoldenInput())
	if err != nil {
		t.Fatalf("BuildDeck: %v", err)
	}
	if got := deck.Render(); strings.Contains(got, "VolSphShell") {
		t.Errorf("padded-box study must not emit a shell transform:\n%s", got)
	}
}
