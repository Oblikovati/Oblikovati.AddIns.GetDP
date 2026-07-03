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

// TestAirBoxWireframeGeometry pins the wireframe: eight corners matching the padded box and
// the twelve edges, and that it tracks padding (doubling the padding doubles the box).
func TestAirBoxWireframeGeometry(t *testing.T) {
	lo, hi := [3]float64{0, 0, 0}, [3]float64{2, 0, 0} // diagonal 2, centre (1,0,0)
	coords, edges := airBoxWireframe(airBox(lo, hi, 3))
	if len(coords) != 24 || len(edges) != 24 {
		t.Fatalf("wireframe = %d coords / %d edge-indices, want 24 / 24 (8 corners, 12 edges)", len(coords), len(edges))
	}
	want := boxCorners(airBox(lo, hi, 3))
	for i, c := range want {
		if coords[i*3] != c[0] || coords[i*3+1] != c[1] || coords[i*3+2] != c[2] {
			t.Errorf("corner %d = %v, want %v", i, coords[i*3:i*3+3], c)
		}
	}
	small := airBox(lo, hi, 3)
	big := airBox(lo, hi, 6)
	if (big.max[0] - big.min[0]) != 2*(small.max[0]-small.min[0]) {
		t.Errorf("padding 6 box side %g not double padding 3 side %g", big.max[0]-big.min[0], small.max[0]-small.min[0])
	}
}

// TestWantsAirBoxGlyph pins the visibility rule: only an EM study with an automatic box.
func TestWantsAirBoxGlyph(t *testing.T) {
	auto := femmodel.AirRegion{Mode: femmodel.AirAutomaticBox}
	if !wantsAirBoxGlyph(femmodel.PhysicsElectrostatics, auto) {
		t.Error("electrostatics automatic box should show the air-box glyph")
	}
	if wantsAirBoxGlyph(femmodel.PhysicsElectrokinetics, auto) {
		t.Error("confined electrokinetics must not show an air box")
	}
	if wantsAirBoxGlyph(femmodel.PhysicsElectrostatics, femmodel.AirRegion{Mode: femmodel.AirNone}) {
		t.Error("mode None must not show an air box")
	}
}

// TestAirBoxGlyphDrawnForEMStudy: an electrostatics study with no BCs still pushes one
// graphics group — the air-box wireframe — while a confined study with no BCs pushes none.
func TestAirBoxGlyphDrawnForEMStudy(t *testing.T) {
	esHost := newBoxHost()
	es := NewEngine(esHost)
	es.withAnalysis(func(a *femmodel.Analysis) { a.AddStudy(femmodel.PhysicsElectrostatics) })
	es.refreshGlyphs()
	if !esHost.saw("clientGraphics.set") {
		t.Error("electrostatics automatic-box study drew no air-box wireframe")
	}

	ekHost := newBoxHost()
	ek := NewEngine(ekHost)
	ek.withAnalysis(func(a *femmodel.Analysis) { a.AddStudy(femmodel.PhysicsElectrokinetics) })
	ek.refreshGlyphs()
	if ekHost.saw("clientGraphics.set") {
		t.Error("confined study with no BCs pushed a glyph group")
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
