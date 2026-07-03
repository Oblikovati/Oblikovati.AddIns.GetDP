// SPDX-License-Identifier: GPL-2.0-only

package demos

import (
	"testing"

	"oblikovati.org/getdp/getdp/femmodel"
)

// TestBuildHeatSinkAuthorsFinnedBlock asserts the heat-sink demo authors a parametric
// finned block: a base slab (new body) plus one fin (joined) replicated into a linear
// array driven by the fin-count parameter, all on parametric sketches.
func TestBuildHeatSinkAuthorsFinnedBlock(t *testing.T) {
	a := &recordingAuthor{}
	if _, err := BuildHeatSink(a); err != nil {
		t.Fatalf("BuildHeatSink: %v", err)
	}
	if len(a.sketches) != 2 || a.sketches[0] != "XY" || a.sketches[1] != "XY" {
		t.Fatalf("sketches = %v, want a base and a fin XY sketch", a.sketches)
	}
	if len(a.rectangles) != 2 {
		t.Fatalf("rectangles = %+v, want base + fin", a.rectangles)
	}
	base, fin := a.rectangles[0], a.rectangles[1]
	if base.x != "0" || base.y != "0" || base.w != "base_width" || base.h != "base_thickness" {
		t.Errorf("base rectangle = %+v, want origin-anchored base_width×base_thickness", base)
	}
	if fin.x != "fin_x0" || fin.y != "base_thickness" || fin.w != "fin_width" || fin.h != "fin_height" {
		t.Errorf("fin rectangle = %+v, want anchored on the base top, fin_width×fin_height", fin)
	}
	if len(a.extrudes) != 2 || a.extrudes[0].operation != "new" || a.extrudes[1].operation != "join" {
		t.Errorf("extrudes = %+v, want a new base and a joined fin", a.extrudes)
	}
	if len(a.patterns) != 1 {
		t.Fatalf("patterns = %+v, want one linear fin pattern", a.patterns)
	}
	p := a.patterns[0]
	if p.feature != "Extrude2" || p.countExpr != "fin_count" || p.count != heatSinkFinCount {
		t.Errorf("pattern = %+v, want the fin extrude replicated fin_count times", p)
	}
	for _, name := range []string{
		"base_width", "base_thickness", "sink_depth", "fin_width", "fin_height",
		"fin_count", "fin_pitch", "fin_x0",
	} {
		if _, ok := a.paramExpr(name); !ok {
			t.Errorf("parameter %q not published (have %v)", name, a.params)
		}
	}
}

// TestBuildHeatSinkStudyAppliesConductionAndConvection asserts the returned study is a
// steady-thermal problem with a hot base temperature and a convection film on every fin
// top face, probed at the fin-top centroids.
func TestBuildHeatSinkStudyAppliesConductionAndConvection(t *testing.T) {
	a := &recordingAuthor{}
	study, err := BuildHeatSink(a)
	if err != nil {
		t.Fatalf("BuildHeatSink: %v", err)
	}
	if study.Physics != femmodel.PhysicsThermalSteady {
		t.Fatalf("physics = %q, want steady thermal", study.Physics)
	}
	// One base-bottom probe + one probe per fin top.
	if len(a.probes) != 1+heatSinkFinCount {
		t.Fatalf("probes = %v, want base bottom + %d fin tops", a.probes, heatSinkFinCount)
	}
	bottom := a.probes[0]
	if bottom[1] != 0 {
		t.Errorf("base probe y = %g, want 0 (bottom face)", bottom[1])
	}
	// Fin-top probes all sit at the fin top height (base_thickness + fin_height in cm).
	topY := cm(heatSinkBaseThicknessMM + heatSinkFinHeightMM)
	for i, pt := range a.probes[1:] {
		if pt[1] != topY {
			t.Errorf("fin-top probe %d y = %g, want %g", i, pt[1], topY)
		}
	}
	if len(study.Constraints) != 1+heatSinkFinCount {
		t.Fatalf("constraints = %+v, want temperature + %d convection", study.Constraints, heatSinkFinCount)
	}
	hot := study.Constraints[0]
	if hot.Kind != femmodel.KindTemperature || hot.Value != heatSinkHotK {
		t.Errorf("base BC = %+v, want %g K temperature", hot, heatSinkHotK)
	}
	for _, c := range study.Constraints[1:] {
		if c.Kind != femmodel.KindConvection || c.H != heatSinkFilmH || c.TInf != heatSinkAmbientK {
			t.Errorf("fin BC = %+v, want convection h=%g T∞=%g", c, heatSinkFilmH, heatSinkAmbientK)
		}
	}
}
