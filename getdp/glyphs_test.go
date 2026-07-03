// SPDX-License-Identifier: GPL-2.0-only

package getdp

import (
	"testing"

	"oblikovati.org/getdp/getdp/femmodel"
)

// TestGlyphKindMappingIsExhaustive pins the kind→shape language (spec §4.4): Dirichlet
// kinds anchor cubes, flux kinds arrows, convection waves.
func TestGlyphKindMappingIsExhaustive(t *testing.T) {
	want := map[femmodel.ConstraintKind]string{
		femmodel.KindVoltage:     "cube",
		femmodel.KindTemperature: "cube",
		femmodel.KindCurrent:     "arrow",
		femmodel.KindHeatFlux:    "arrow",
		femmodel.KindConvection:  "waves",
	}
	for kind, shape := range want {
		if got := glyphKindLabel(kind); got != shape {
			t.Errorf("glyph for %s = %s, want %s", kind, got, shape)
		}
		if _, ok := glyphColors[kind]; !ok {
			t.Errorf("kind %s has no glyph color", kind)
		}
	}
}

// TestGlyphMeshBuildersProduceClosedShapes sanity-checks the solid builders: normals
// per vertex, index triples, non-empty output.
func TestGlyphMeshBuildersProduceClosedShapes(t *testing.T) {
	m := &glyphMesh{}
	m.cube([3]float64{0, 0, 0}, 1)
	m.arrow([3]float64{0, 0, 2}, [3]float64{0, 0, 1}, 1)
	if len(m.idx)%3 != 0 || len(m.coords) != len(m.normals) || len(m.idx) == 0 {
		t.Fatalf("glyph mesh malformed: %d coords, %d normals, %d indices",
			len(m.coords), len(m.normals), len(m.idx))
	}
}

// TestRefreshGlyphsPushesActiveStudyMarkers binds a voltage BC to the boxHost's inlet
// face and asserts a glyph group reaches the host.
func TestRefreshGlyphsPushesActiveStudyMarkers(t *testing.T) {
	b := newBoxHost()
	e := NewEngine(b)
	e.withAnalysis(func(a *femmodel.Analysis) {
		if _, err := a.Active().AddConstraint(femmodel.ConstraintObject{
			Kind: femmodel.KindVoltage, Name: "V+", Faces: []string{inletFaceKey}, Value: 5,
		}); err != nil {
			t.Fatal(err)
		}
	})
	e.refreshGlyphs()
	if !b.saw("clientGraphics.set") {
		t.Error("refreshGlyphs never pushed a graphics group")
	}
}
