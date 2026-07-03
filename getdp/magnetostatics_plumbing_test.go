// SPDX-License-Identifier: GPL-2.0-only

package getdp

import (
	"testing"

	"oblikovati.org/getdp/getdp/femmodel"
)

// TestResolveCoilsMapsBodiesToTags: each coil body resolves to its physical volume tag, the
// centre is converted to SI metres, and axis/current density pass through unchanged.
func TestResolveCoilsMapsBodiesToTags(t *testing.T) {
	regions := &RegionTable{Volumes: []VolumeRegion{{Tag: 1, Body: 0}, {Tag: 2, Body: airBodyIndex}}}
	rc := &ResolveContext{Model: &SolveModel{}}
	coils := []femmodel.CoilObject{{
		Name: "Winding", Bodies: []int{0}, Axis: [3]float64{0, 0, 1},
		Center: [3]float64{0, 0, 10}, CurrentDensity: 1e6,
	}}
	if err := resolveCoils(coils, rc, regions); err != nil {
		t.Fatalf("resolveCoils: %v", err)
	}
	if len(rc.Model.Coils) != 1 {
		t.Fatalf("resolved %d coils, want 1", len(rc.Model.Coils))
	}
	c := rc.Model.Coils[0]
	if c.RegionTag != 1 || c.CurrentDensity != 1e6 || c.Axis != [3]float64{0, 0, 1} {
		t.Errorf("coil = %+v, want tag 1, J0 1e6, z-axis", c)
	}
	if want := 10 * modelUnitM; c.Center[2] != want {
		t.Errorf("coil centre z = %v m, want %v (SI-converted)", c.Center[2], want)
	}
}

// TestResolveCoilsUnknownBody: a coil naming a body with no physical volume is a loud error.
func TestResolveCoilsUnknownBody(t *testing.T) {
	regions := &RegionTable{Volumes: []VolumeRegion{{Tag: 1, Body: 0}}}
	rc := &ResolveContext{Model: &SolveModel{}}
	coils := []femmodel.CoilObject{{Name: "Bad", Bodies: []int{99}}}
	if err := resolveCoils(coils, rc, regions); err == nil {
		t.Fatal("expected an error for a coil on an unregistered body, got nil")
	}
}

// TestCoilCenterProbes: one probe per coil at its centre, named coil1, coil2, …
func TestCoilCenterProbes(t *testing.T) {
	coils := []Coil{
		{RegionTag: 1, Center: [3]float64{0, 0, 0.5}},
		{RegionTag: 2, Center: [3]float64{1, 0, 0}},
	}
	got := coilCenterProbes(coils)
	if len(got) != 2 {
		t.Fatalf("probes = %d, want 2", len(got))
	}
	if got[0].Name != "coil1" || got[0].Point != [3]float64{0, 0, 0.5} {
		t.Errorf("probe 0 = %+v, want coil1 at coil centre", got[0])
	}
	if got[1].Name != "coil2" || got[1].Point != [3]float64{1, 0, 0} {
		t.Errorf("probe 1 = %+v, want coil2 at coil centre", got[1])
	}
}

// TestLinearSolverOfDefaultsAndOverride: magnetostatics gets the GMRES+diagonal defaults, and
// a study's TP-12 knobs override them field-by-field; non-magnetic physics get no solver.par.
func TestLinearSolverOfDefaultsAndOverride(t *testing.T) {
	def := linearSolverOf(runInputs{physics: femmodel.PhysicsMagnetostatics})
	if def == nil || def.Algorithm != 8 || def.Preconditioner != 8 || def.MaxIter != 5000 {
		t.Fatalf("magnetostatics default solver = %+v, want GMRES(8)+diagonal(8), 5000 iters", def)
	}
	over := linearSolverOf(runInputs{
		physics: femmodel.PhysicsMagnetostatics,
		solver:  femmodel.SolverObject{Linear: femmodel.LinearSolver{Tolerance: 1e-6, Preconditioner: 2}},
	})
	if over.Tolerance != 1e-6 || over.Preconditioner != 2 {
		t.Errorf("overridden solver = %+v, want tol 1e-6 + preconditioner 2", over)
	}
	if over.MaxIter != 5000 {
		t.Errorf("unset knob must keep the default: MaxIter = %d, want 5000", over.MaxIter)
	}
	if ls := linearSolverOf(runInputs{physics: femmodel.PhysicsElectrostatics}); ls != nil {
		t.Errorf("non-magnetic physics must not configure a linear solver, got %+v", ls)
	}
}

// outerBoundaryMesh is a one-facet mesh whose single triangle is the air-box outer wall.
func outerBoundaryMesh() *TetMesh {
	return &TetMesh{
		Nodes:   []Node{{ID: 1, X: 1}, {ID: 2, Y: 1}, {ID: 3, Z: 1}},
		Surface: []BoundaryFacet{{Corners: [3]int{1, 2, 3}, Physical: outerBoundaryTag}},
	}
}

// TestBindFarFieldMagnetostaticsTagOnly: magnetostatics records the far-field surface tag (for
// the a×n=0 edge constraint) but appends NO scalar potential (that is electrostatics-only).
func TestBindFarFieldMagnetostaticsTagOnly(t *testing.T) {
	rc := &ResolveContext{Model: &SolveModel{}, Regions: &RegionTable{}}
	if err := bindFarField(rc, outerBoundaryMesh(), femmodel.PhysicsMagnetostatics); err != nil {
		t.Fatalf("bindFarField: %v", err)
	}
	if rc.Model.FarFieldTag == 0 {
		t.Error("magnetostatics far-field tag not recorded")
	}
	if len(rc.Model.BoundPotentials) != 0 {
		t.Errorf("magnetostatics must not append a voltage far-field, got %+v", rc.Model.BoundPotentials)
	}
}

// TestBindFarFieldElectrostaticsPinsZero: electrostatics records the tag AND pins it to V=0.
func TestBindFarFieldElectrostaticsPinsZero(t *testing.T) {
	rc := &ResolveContext{Model: &SolveModel{}, Regions: &RegionTable{}}
	if err := bindFarField(rc, outerBoundaryMesh(), femmodel.PhysicsElectrostatics); err != nil {
		t.Fatalf("bindFarField: %v", err)
	}
	if rc.Model.FarFieldTag == 0 {
		t.Error("electrostatics far-field tag not recorded")
	}
	if len(rc.Model.BoundPotentials) != 1 || rc.Model.BoundPotentials[0].Kind != KindVoltage ||
		rc.Model.BoundPotentials[0].Value != 0 {
		t.Errorf("electrostatics far-field must pin one V=0 potential, got %+v", rc.Model.BoundPotentials)
	}
}
