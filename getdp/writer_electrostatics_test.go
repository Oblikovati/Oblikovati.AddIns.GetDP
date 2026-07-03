// SPDX-License-Identifier: GPL-2.0-only

package getdp

import (
	"context"
	"math"
	"path/filepath"
	"strings"
	"testing"

	"oblikovati.org/getdp/getdp/femmodel"
)

// TestElectrostaticsAirRegionSolvesWithFarField drives the WHOLE air pipeline end to end:
// an electrostatics study on the boxHost bar with its automatic air box. The runner meshes
// part+air in one conformal run, registers the air volume, pins the generated outer boundary
// to the far-field zero, and solves. The bar is a charged conductor (V+) radiating into the
// grounded box, so the field must exist and a capacitance scalar must be reported — proving
// meshForStudy's air branch, the air volume region, and bindFarField all wire up.
func TestElectrostaticsAirRegionSolvesWithFarField(t *testing.T) {
	bins := requireSolver(t)
	t.Setenv("OBK_GETDP_BIN", bins.getdp)
	t.Setenv("OBK_GMSH_BIN", bins.gmsh)
	b := newBoxHost()
	e := NewEngine(b)
	e.withAnalysis(func(a *femmodel.Analysis) {
		s := a.AddStudy(femmodel.PhysicsElectrostatics) // active; automatic air box by default
		mustAddBC(t, s, femmodel.ConstraintObject{Kind: femmodel.KindVoltage, Name: "V+",
			Faces: []string{inletFaceKey}, Value: 1})
		s.Mesh.SizeModelUnits = 1.0
		s.Solver.Air.PaddingFactor = 1.5 // a tight box keeps this wiring test fast (accuracy is the oracle's job)
	})

	res, err := e.RunStudyOnHost(context.Background())
	if err != nil {
		t.Fatalf("RunStudyOnHost: %v", err)
	}
	if res.FieldLabel != "electric potential" || res.FieldMax <= res.FieldMin {
		t.Errorf("field = %q %g…%g, want a potential range", res.FieldLabel, res.FieldMin, res.FieldMax)
	}
	var hasC bool
	for _, s := range res.Scalars {
		if strings.Contains(s.Label, "capacitance") {
			hasC = true
			if s.Value <= 0 {
				t.Errorf("capacitance = %g F, want positive", s.Value)
			}
		}
	}
	if !hasC {
		t.Errorf("no capacitance scalar reported in %+v", res.Scalars)
	}
}

// TestElectrostaticsParallelPlateCapacitanceOracle solves a dielectric-filled parallel-plate
// capacitor end to end (the boxHost 20×1×1 cm bar: electrodes on the two end faces, insulating
// sides). Oracle (exact for the confined uniform field): C = ε₀·εr·A/d, with A = 1 cm² and
// d = 20 cm. The energy-method capacitance the writer prints must match, and be self-consistent
// with the printed energy W = ½CV².
func TestElectrostaticsParallelPlateCapacitanceOracle(t *testing.T) {
	const er, vApplied = 4.0, 1.0
	dir := solveBoxStudy(t, PhysicsElectrostatics, Material{Epsilon: er}, []ConstraintSpec{
		DirichletSpec{SpecKind: KindVoltage, SpecName: "V+", FaceKeys: []string{inletFaceKey}, Value: vApplied},
		DirichletSpec{SpecKind: KindVoltage, SpecName: "GND", FaceKeys: []string{outletFaceKey}, Value: 0},
	}, nil)

	const areaM2, gapM = 1e-4, 0.2 // 1 cm² plates, 20 cm gap (SI)
	wantC := vacuumPermittivity * er * areaM2 / gapM
	gotC := readTableValue(t, filepath.Join(dir, "capacitance.txt"))
	if rel := math.Abs(gotC-wantC) / wantC; rel > 0.02 {
		t.Errorf("C = %.6g F, want ε₀·εr·A/d = %.6g F (rel err %.2g > 2%%)", gotC, wantC, rel)
	}

	gotW := readTableValue(t, filepath.Join(dir, "energy.txt"))
	wantW := 0.5 * gotC * vApplied * vApplied
	if rel := math.Abs(gotW-wantW) / wantW; rel > 1e-6 {
		t.Errorf("energy %.6g F inconsistent with ½CV² = %.6g", gotW, wantW)
	}
}
