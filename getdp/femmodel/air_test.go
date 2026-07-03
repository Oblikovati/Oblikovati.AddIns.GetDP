// SPDX-License-Identifier: GPL-2.0-only

package femmodel

import "testing"

// TestNeedsAirByPhysics pins which physics solve fields in the space around the part and so
// need a meshed air region (spec §3.3): electrostatics does; conduction/thermal do not.
func TestNeedsAirByPhysics(t *testing.T) {
	cases := map[PhysicsKind]bool{
		PhysicsElectrostatics:   true,
		PhysicsElectrokinetics:  false,
		PhysicsThermalSteady:    false,
		PhysicsThermalTransient: false,
	}
	for kind, want := range cases {
		if got := NeedsAir(kind); got != want {
			t.Errorf("NeedsAir(%q) = %v, want %v", kind, got, want)
		}
	}
}

// TestElectrostaticsStudyDefaults asserts a new electrostatics study carries the air
// defaults (an automatic box) and a dielectric default material with a relative permittivity.
func TestElectrostaticsStudyDefaults(t *testing.T) {
	s := NewAnalysis().AddStudy(PhysicsElectrostatics)
	if s.Solver.Physics != PhysicsElectrostatics {
		t.Fatalf("physics = %q, want electrostatics", s.Solver.Physics)
	}
	if s.Solver.Air.Mode != AirAutomaticBox {
		t.Errorf("air mode = %v, want automatic box default", s.Solver.Air.Mode)
	}
	if s.Solver.Air.PaddingFactor != electrostaticsPadding {
		t.Errorf("padding = %g, want %g (electrostatics)", s.Solver.Air.PaddingFactor, electrostaticsPadding)
	}
	if eps := s.Regions()[0].Material.Epsilon; eps <= 0 {
		t.Errorf("default material Epsilon = %g, want a positive relative permittivity", eps)
	}
}

// TestElectrostaticsAllowsVoltageElectrodes asserts an electrode (voltage) BC applies to an
// electrostatics study.
func TestElectrostaticsAllowsVoltageElectrodes(t *testing.T) {
	s := NewAnalysis().AddStudy(PhysicsElectrostatics)
	if _, err := s.AddConstraint(ConstraintObject{Kind: KindVoltage, Name: "V+", Value: 1}); err != nil {
		t.Fatalf("electrode BC rejected: %v", err)
	}
}
