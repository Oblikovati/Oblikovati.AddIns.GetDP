// SPDX-License-Identifier: GPL-2.0-only

package femmodel

import "testing"

// TestMagnetostaticsNeedsAirWithShell: magnetostatics solves the field in the space around
// the part, so it needs a meshed air region, and it defaults to the infinite shell (open
// boundary) with the wider magnetics padding.
func TestMagnetostaticsNeedsAirWithShell(t *testing.T) {
	if !NeedsAir(PhysicsMagnetostatics) {
		t.Fatal("magnetostatics must need an air region")
	}
	air := defaultAir(PhysicsMagnetostatics)
	if air.Mode != AirAutomaticBox {
		t.Errorf("magnetostatics air mode = %v, want AirAutomaticBox", air.Mode)
	}
	if air.Truncation != TruncationInfiniteShell {
		t.Errorf("magnetostatics truncation = %v, want infinite shell (open boundary)", air.Truncation)
	}
	if air.PaddingFactor != magneticsPadding {
		t.Errorf("magnetostatics padding = %v, want %v", air.PaddingFactor, magneticsPadding)
	}
}

// TestMagnetostaticsDefaultMaterialIsUnitPermeability: the default magnetostatics material is
// non-magnetic (μr = 1, copper/air), the demo raises μr for iron cores.
func TestMagnetostaticsDefaultMaterialIsUnitPermeability(t *testing.T) {
	m := defaultMaterial(PhysicsMagnetostatics)
	if m.Mu != 1 {
		t.Errorf("default magnetostatics μr = %v, want 1", m.Mu)
	}
}

// TestStudyCarriesCoils: a magnetostatics study holds coil (current-source) objects; Add
// returns an id and Coils reports them in creation order.
func TestStudyCarriesCoils(t *testing.T) {
	a := NewAnalysis()
	s := a.AddStudy(PhysicsMagnetostatics)
	id := s.AddCoil(CoilObject{Bodies: []int{0}, Axis: [3]float64{0, 0, 1}, CurrentDensity: 1e6})
	if id == "" {
		t.Fatal("AddCoil returned an empty id")
	}
	coils := s.Coils()
	if len(coils) != 1 || coils[0].CurrentDensity != 1e6 || coils[0].Axis != [3]float64{0, 0, 1} {
		t.Errorf("coils = %+v, want one z-axis coil at J0=1e6", coils)
	}
}
