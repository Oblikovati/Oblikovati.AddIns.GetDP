// SPDX-License-Identifier: GPL-2.0-only

package getdp

import (
	"strings"
	"testing"
)

// magnetostaticGoldenInput builds a deterministic magnetostatics deck input by hand: a
// copper coil volume (μr=1) carrying an azimuthal current density about the z-axis, wrapped
// in an air volume (μr=1) and an infinite shell (μr=1), with the far-field shell boundary
// pinned to a=0. It exercises every branch the writer has (per-volume ν, the azimuthal js,
// the ungauged edge space, the curl-curl + source formulation, the a=0 constraint, the
// b/h/energy post and the on-axis |B| probe).
func magnetostaticGoldenInput() DeckInput {
	regions := &RegionTable{
		Volumes: []VolumeRegion{
			{Tag: 1, Name: "Coil", Body: 0},
			{Tag: 2, Name: "Air", Body: airBodyIndex},
			{Tag: 3, Name: "Shell", Body: shellBodyIndex},
		},
		Surfaces: []SurfaceRegion{{Tag: 4, Name: "FarField"}},
	}
	model := &SolveModel{
		FarFieldTag: 4,
		Coils: []Coil{{
			RegionTag: 1, Axis: [3]float64{0, 0, 1}, Center: [3]float64{0, 0, 0},
			CurrentDensity: 1000,
		}},
	}
	return DeckInput{
		Regions:   regions,
		Model:     model,
		Materials: map[int]Material{1: {Mu: 1}, 2: {Mu: 1}, 3: {Mu: 1}},
		Order:     1,
		Shell:     &ShellTransform{VolumeTag: 3, Rint: 0.03, Rext: 0.06},
		Probes:    []FieldProbe{{Name: "center", Point: [3]float64{0, 0, 0}}},
	}
}

// TestMagnetostaticsDeckEdgeElementFormulation pins the load-bearing physics fragments: the
// ungauged H(curl) edge space, the curl-curl bilinear term, and the azimuthal source term.
func TestMagnetostaticsDeckEdgeElementFormulation(t *testing.T) {
	deck, _, err := MagnetostaticsWriter{}.BuildDeck(magnetostaticGoldenInput())
	if err != nil {
		t.Fatalf("BuildDeck: %v", err)
	}
	got := deck.Render()
	for _, want := range []string{
		"{ Name se; NameOfCoef ae; Function BF_Edge;",   // edge element
		"Galerkin { [ nu[] * Dof{d a}, {d a} ];",        // curl-curl (reluctivity)
		"Galerkin { [ -js[], {a} ];",                    // coil source
		"In VolS; Jacobian JVol; Integration I1; }",     // source integrated over coil volumes only
		"{ Name SetA; Case {",                           // far-field a=0 constraint
		"{ Region Sur4; Value 0; }",                     // pinned on the shell boundary
	} {
		if !strings.Contains(got, want) {
			t.Errorf("magnetostatics deck missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "TreeIn") || strings.Contains(got, "GaugeCondition") {
		t.Errorf("magnetostatics #27 must be UNGAUGED (no tree-cotree gauge):\n%s", got)
	}
}

// TestMagnetostaticsDeckAzimuthalSource pins the azimuthal current-density function: js is
// J0 times the unit azimuthal direction axis /\ (XYZ - center) — tangent to every face of a
// body-of-revolution coil (js·n=0) and divergence-free, so the discrete source stays
// consistent with the ungauged system (no RHS projection needed).
func TestMagnetostaticsDeckAzimuthalSource(t *testing.T) {
	deck, _, err := MagnetostaticsWriter{}.BuildDeck(magnetostaticGoldenInput())
	if err != nil {
		t.Fatalf("BuildDeck: %v", err)
	}
	got := deck.Render()
	const wantJs = "js[Vol1] = 1000 * Unit[ Vector[0, 0, 1] /\\ (XYZ[] - Vector[0, 0, 0]) ];"
	if !strings.Contains(got, wantJs) {
		t.Errorf("azimuthal source js drifted:\nwant: %s\n--- got ---\n%s", wantJs, got)
	}
	if !strings.Contains(got, "VolS = Region[{Vol1}];") {
		t.Errorf("source-volume group VolS must union the coil volumes:\n%s", got)
	}
}

// TestMagnetostaticsDeckReluctivity: ν = 1/(μ0·μr) per volume; every volume carries one.
func TestMagnetostaticsDeckReluctivity(t *testing.T) {
	deck, _, err := MagnetostaticsWriter{}.BuildDeck(magnetostaticGoldenInput())
	if err != nil {
		t.Fatalf("BuildDeck: %v", err)
	}
	got := deck.Render()
	// 1/(μ0·1) = 1/(4π·1e-7) ≈ 795774.7154594767
	for _, vol := range []string{"nu[Vol1] = 795774", "nu[Vol2] = 795774", "nu[Vol3] = 795774"} {
		if !strings.Contains(got, vol) {
			t.Errorf("magnetostatics deck missing reluctivity %q:\n%s", vol, got)
		}
	}
}

// TestMagnetostaticsDeckSolverAndProbe: the deck ships a SPARSKIT solver.par (GMRES +
// diagonal preconditioner for the singular ungauged system) and prints |B| at each probe.
func TestMagnetostaticsDeckSolverAndProbe(t *testing.T) {
	deck, outs, err := MagnetostaticsWriter{}.BuildDeck(magnetostaticGoldenInput())
	if err != nil {
		t.Fatalf("BuildDeck: %v", err)
	}
	if outs.Solver == nil {
		t.Fatal("magnetostatics DeckOutputs must carry a solver.par (SPARSKIT knobs)")
	}
	if outs.Solver.Preconditioner != 8 || outs.Solver.Algorithm != 8 {
		t.Errorf("want GMRES(8) + diagonal(8) defaults, got algo=%d precond=%d", outs.Solver.Algorithm, outs.Solver.Preconditioner)
	}
	got := deck.Render()
	if !strings.Contains(got, `Print[ bnorm, OnPoint {0, 0, 0}, Format Table, File "b_center.txt" ]`) {
		t.Errorf("magnetostatics deck missing the |B| point probe:\n%s", got)
	}
	if !hasTable(outs.Tables, "b_center.txt") {
		t.Errorf("DeckOutputs.Tables must list the probe file b_center.txt, got %+v", outs.Tables)
	}
}

// TestMagnetostaticsDeckRejectsNoCoil / NoFarField: an open-boundary magnetostatics study
// with no source or no far-field anchor is a user error, reported (not a silent zero field).
func TestMagnetostaticsDeckRejectsNoCoil(t *testing.T) {
	in := magnetostaticGoldenInput()
	in.Model.Coils = nil
	if _, _, err := (MagnetostaticsWriter{}).BuildDeck(in); err == nil {
		t.Fatal("expected an error when the study has no coil source, got nil")
	}
}

func TestMagnetostaticsDeckRejectsNoFarField(t *testing.T) {
	in := magnetostaticGoldenInput()
	in.Model.FarFieldTag = 0
	if _, _, err := (MagnetostaticsWriter{}).BuildDeck(in); err == nil {
		t.Fatal("expected an error when the study has no far-field anchor, got nil")
	}
}

// hasTable reports whether a table path is in the outputs.
func hasTable(tables []TableOutput, path string) bool {
	for _, tb := range tables {
		if tb.Path == path {
			return true
		}
	}
	return false
}
