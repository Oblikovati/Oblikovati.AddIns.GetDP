// SPDX-License-Identifier: GPL-2.0-only

package femmodel

import (
	"strings"
	"testing"
)

func TestNewAnalysisSeedsOneActiveStudy(t *testing.T) {
	a := NewAnalysis()
	if len(a.Studies()) != 1 {
		t.Fatalf("studies = %d, want 1", len(a.Studies()))
	}
	s := a.Active()
	if s.Solver.Physics != PhysicsElectrokinetics {
		t.Errorf("default physics = %s, want electrokinetics", s.Solver.Physics)
	}
	if len(s.Regions()) != 1 || s.Regions()[0].Material.Name != "Copper" {
		t.Errorf("default region = %+v, want one all-bodies copper region", s.Regions())
	}
}

func TestAddStudySwitchesActive(t *testing.T) {
	a := NewAnalysis()
	s2 := a.AddStudy(PhysicsThermalSteady)
	if a.Active() != s2 {
		t.Error("new study did not become active")
	}
	if s2.Regions()[0].Material.Name != "Aluminium" {
		t.Errorf("thermal default material = %q, want Aluminium", s2.Regions()[0].Material.Name)
	}
	if err := a.SetActive(a.Studies()[0].ID()); err != nil || a.Active() == s2 {
		t.Errorf("SetActive back failed: %v", err)
	}
}

func TestRemoveStudyKeepsInvariants(t *testing.T) {
	a := NewAnalysis()
	if err := a.RemoveStudy(a.Active().ID()); err == nil {
		t.Error("removing the last study succeeded")
	}
	s2 := a.AddStudy(PhysicsThermalSteady)
	if err := a.RemoveStudy(s2.ID()); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if len(a.Studies()) != 1 || a.Active() == s2 {
		t.Error("active study not repaired after removing the active one")
	}
}

func TestDuplicateStudyDeepCopies(t *testing.T) {
	a := NewAnalysis()
	src := a.Active()
	if _, err := src.AddConstraint(ConstraintObject{Kind: KindVoltage, Value: 5}); err != nil {
		t.Fatal(err)
	}
	cp, err := a.DuplicateStudy(src.ID())
	if err != nil {
		t.Fatalf("duplicate: %v", err)
	}
	if cp.ID() == src.ID() || !strings.Contains(cp.Name(), "(copy)") {
		t.Errorf("copy identity: id=%q name=%q", cp.ID(), cp.Name())
	}
	if len(cp.Constraints()) != 1 {
		t.Fatalf("copy has %d constraints, want 1", len(cp.Constraints()))
	}
	// Mutating the copy must not touch the source.
	c := cp.Constraints()[0]
	c.Value = 99
	if err := cp.UpdateConstraint(c); err != nil {
		t.Fatal(err)
	}
	if src.Constraints()[0].Value != 5 {
		t.Error("mutating the duplicate changed the source study")
	}
}

func TestConstraintKindPhysicsCompatibility(t *testing.T) {
	a := NewAnalysis() // electrokinetics
	s := a.Active()
	if _, err := s.AddConstraint(ConstraintObject{Kind: KindTemperature, Value: 300}); err == nil {
		t.Error("temperature constraint accepted on an electrokinetic study")
	}
	if _, err := s.AddConstraint(ConstraintObject{Kind: KindVoltage, Value: 12}); err != nil {
		t.Errorf("voltage constraint rejected: %v", err)
	}
}

func TestSetPhysicsDropsIncompatibleConstraints(t *testing.T) {
	a := NewAnalysis()
	s := a.Active()
	mustAdd(t, s, ConstraintObject{Kind: KindVoltage, Value: 12})
	mustAdd(t, s, ConstraintObject{Kind: KindCurrent, Value: 5})
	dropped := s.SetPhysics(PhysicsThermalSteady)
	if len(dropped) != 2 || len(s.Constraints()) != 0 {
		t.Errorf("dropped %d kept %d, want 2 dropped 0 kept", len(dropped), len(s.Constraints()))
	}
	if s.Regions()[0].Material.Name != "Aluminium" {
		t.Errorf("material after physics switch = %q, want thermal default", s.Regions()[0].Material.Name)
	}
}

func TestRegionBodyExclusivity(t *testing.T) {
	a := NewAnalysis()
	s := a.Active()
	first := s.Regions()[0]
	first.Bodies = []int{0, 1}
	if err := s.UpdateRegion(first); err != nil {
		t.Fatal(err)
	}
	id2 := s.AddRegion("Coil", nil)
	second, _ := findRegion(s, id2)
	second.Bodies = []int{1}
	if err := s.UpdateRegion(second); err == nil || !strings.Contains(err.Error(), "exactly one region") {
		t.Errorf("err = %v, want duplicate-body rejection", err)
	}
	second.Bodies = []int{2}
	if err := s.UpdateRegion(second); err != nil {
		t.Errorf("disjoint body assignment rejected: %v", err)
	}
}

func TestRemoveRegionKeepsAtLeastOne(t *testing.T) {
	a := NewAnalysis()
	s := a.Active()
	if err := s.RemoveRegion(s.Regions()[0].ID); err == nil {
		t.Error("removing the last region succeeded")
	}
	id2 := s.AddRegion("Extra", nil)
	if err := s.RemoveRegion(id2); err != nil {
		t.Errorf("remove: %v", err)
	}
}

func TestTransientDefaultsCarryTimeGrid(t *testing.T) {
	a := NewAnalysis()
	s := a.AddStudy(PhysicsThermalTransient)
	if s.Solver.TMax <= 0 || s.Solver.DT <= 0 || s.Solver.Theta != 1 {
		t.Errorf("transient defaults = %+v, want a positive implicit-Euler grid", s.Solver)
	}
	_ = a
}

func mustAdd(t *testing.T, s *Study, c ConstraintObject) string {
	t.Helper()
	id, err := s.AddConstraint(c)
	if err != nil {
		t.Fatalf("add constraint: %v", err)
	}
	return id
}

func findRegion(s *Study, id string) (RegionObject, bool) {
	for _, r := range s.Regions() {
		if r.ID == id {
			return r, true
		}
	}
	return RegionObject{}, false
}
