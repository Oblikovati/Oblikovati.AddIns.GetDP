// SPDX-License-Identifier: GPL-2.0-only

package demos

import (
	"fmt"
	"testing"

	"oblikovati.org/getdp/getdp/femmodel"
)

// recordingAuthor is a fake Author that records every authored call so a demo's geometry
// program can be asserted with no live host. FaceKeyAt returns a key encoding the probe
// point so binding assertions can check which face each BC landed on.
type recordingAuthor struct {
	params     []Param
	sketches   []string        // planes, in creation order
	rectangles []rectangleCall // corner rectangles laid
	extrudes   []extrudeCall   // extrudes performed
	patterns   []patternCall   // linear patterns
	probes     [][3]float64    // FaceKeyAt points, in call order
	nextSketch int
}

type rectangleCall struct {
	sketch     int
	x, y, w, h string
}
type extrudeCall struct {
	sketch    int
	distance  string
	operation string
}
type patternCall struct {
	feature   string
	count     int
	countExpr string
	stepXcm   float64
}

func (a *recordingAuthor) Parameter(name, expr string) error {
	a.params = append(a.params, Param{Name: name, Expr: expr})
	return nil
}
func (a *recordingAuthor) Sketch(plane string) (int, error) {
	a.sketches = append(a.sketches, plane)
	a.nextSketch++
	return a.nextSketch, nil
}
func (a *recordingAuthor) CornerRectangle(sketch int, x, y, w, h string) error {
	a.rectangles = append(a.rectangles, rectangleCall{sketch, x, y, w, h})
	return nil
}
func (a *recordingAuthor) Extrude(sketch int, distance, operation string) (string, error) {
	a.extrudes = append(a.extrudes, extrudeCall{sketch, distance, operation})
	return fmt.Sprintf("Extrude%d", len(a.extrudes)), nil
}
func (a *recordingAuthor) PatternX(feature string, count int, countExpr string, stepXcm float64) error {
	a.patterns = append(a.patterns, patternCall{feature, count, countExpr, stepXcm})
	return nil
}
func (a *recordingAuthor) FaceKeyAt(point [3]float64) (string, error) {
	a.probes = append(a.probes, point)
	return fmt.Sprintf("face@%.3f,%.3f,%.3f", point[0], point[1], point[2]), nil
}

func (a *recordingAuthor) paramExpr(name string) (string, bool) {
	for _, p := range a.params {
		if p.Name == name {
			return p.Expr, true
		}
	}
	return "", false
}

// TestBuildBusbarAuthorsParametricBar asserts the busbar demo authors one parametric
// bar: named parameters, a single XY sketch with an origin-anchored rectangle driven by
// the width/height parameters, and one "new"-body extrude driven by the length parameter.
func TestBuildBusbarAuthorsParametricBar(t *testing.T) {
	a := &recordingAuthor{}
	if _, err := BuildBusbar(a); err != nil {
		t.Fatalf("BuildBusbar: %v", err)
	}
	if len(a.sketches) != 1 || a.sketches[0] != "XY" {
		t.Fatalf("sketches = %v, want one XY sketch", a.sketches)
	}
	if len(a.rectangles) != 1 {
		t.Fatalf("rectangles = %v, want one", a.rectangles)
	}
	r := a.rectangles[0]
	if r.x != "0" || r.y != "0" || r.w != "bar_width" || r.h != "bar_height" {
		t.Errorf("rectangle = %+v, want origin-anchored width/height-driven", r)
	}
	if len(a.extrudes) != 1 || a.extrudes[0].distance != "bar_length" || a.extrudes[0].operation != "new" {
		t.Errorf("extrudes = %+v, want one new-body bar_length extrude", a.extrudes)
	}
	for _, name := range []string{"bar_length", "bar_width", "bar_height"} {
		if _, ok := a.paramExpr(name); !ok {
			t.Errorf("parameter %q not published (have %v)", name, a.params)
		}
	}
}

// TestBuildBusbarStudyBindsElectrodesToEndFaces asserts the returned study is an
// electrokinetics problem with V+ and ground bound to the two extrude-cap faces (probed
// at the cap centroids in model units), i.e. the exact busbar setup the oracle checks.
func TestBuildBusbarStudyBindsElectrodesToEndFaces(t *testing.T) {
	a := &recordingAuthor{}
	study, err := BuildBusbar(a)
	if err != nil {
		t.Fatalf("BuildBusbar: %v", err)
	}
	if study.Physics != femmodel.PhysicsElectrokinetics {
		t.Fatalf("physics = %q, want electrokinetics", study.Physics)
	}
	if len(a.probes) != 2 {
		t.Fatalf("face probes = %v, want two end-cap probes", a.probes)
	}
	// The two probes must be the cap centroids: same x,y (cross-section centre), z at 0
	// and at the bar length; the bar is 200×10×10 mm ⇒ 20×1×1 cm.
	in, out := a.probes[0], a.probes[1]
	wantXY := [2]float64{0.5, 0.5}
	if in[0] != wantXY[0] || in[1] != wantXY[1] || out[0] != wantXY[0] || out[1] != wantXY[1] {
		t.Errorf("probe cross-section centres = %v %v, want (0.5,0.5)", in, out)
	}
	if in[2] != 0 || out[2] != 20 {
		t.Errorf("probe z = %g,%g, want 0 and 20 (cm)", in[2], out[2])
	}
	if len(study.Constraints) != 2 {
		t.Fatalf("constraints = %+v, want V+ and ground", study.Constraints)
	}
	vplus, gnd := study.Constraints[0], study.Constraints[1]
	if vplus.Kind != femmodel.KindVoltage || gnd.Kind != femmodel.KindVoltage {
		t.Errorf("constraint kinds = %q,%q, want both voltage", vplus.Kind, gnd.Kind)
	}
	if vplus.Value != 1 || gnd.Value != 0 {
		t.Errorf("electrode values = %g,%g, want 1 and 0 V", vplus.Value, gnd.Value)
	}
	if len(vplus.Faces) != 1 || vplus.Faces[0] != "face@0.500,0.500,0.000" {
		t.Errorf("V+ faces = %v, want the inlet cap key", vplus.Faces)
	}
	if len(gnd.Faces) != 1 || gnd.Faces[0] != "face@0.500,0.500,20.000" {
		t.Errorf("ground faces = %v, want the outlet cap key", gnd.Faces)
	}
}
