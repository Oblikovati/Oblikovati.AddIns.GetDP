// SPDX-License-Identifier: GPL-2.0-only

package getdp

import (
	"strings"
	"testing"
)

func TestResolveSpecsBindsPotentialsAndFluxes(t *testing.T) {
	mesh := oneTetMesh()
	rc := &ResolveContext{
		Model:   &SolveModel{},
		Mesh:    mesh,
		Groups:  fakeGroups(mesh, "fA", "fB"),
		Regions: newRegionTable([]string{"Body"}),
	}
	specs := []ConstraintSpec{
		DirichletSpec{SpecKind: KindVoltage, SpecName: "V+", FaceKeys: []string{"fA"}, Value: 12},
		FluxSpec{SpecKind: KindConvection, SpecName: "cooling", FaceKeys: []string{"fB"}, H: 25, TInf: 293.15},
	}
	if err := resolveSpecs(specs, rc); err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if len(rc.Model.BoundPotentials) != 1 || rc.Model.BoundPotentials[0].Value != 12 {
		t.Errorf("potentials = %+v, want one 12 V entry", rc.Model.BoundPotentials)
	}
	if p := rc.Model.BoundPotentials[0]; p.RegionTag != 2 || p.Name != "V+" {
		t.Errorf("potential bound to tag %d name %q, want 2 / V+", p.RegionTag, p.Name)
	}
	if len(rc.Model.BoundFluxes) != 1 || rc.Model.BoundFluxes[0].H != 25 {
		t.Errorf("fluxes = %+v, want one h=25 film", rc.Model.BoundFluxes)
	}
	if got := len(rc.Regions.Surfaces); got != 2 {
		t.Errorf("physical surfaces = %d, want 2 (one per spec)", got)
	}
}

func TestResolveSpecsSurfacesFailureNamesSpec(t *testing.T) {
	mesh := oneTetMesh()
	rc := &ResolveContext{
		Model:   &SolveModel{},
		Mesh:    mesh,
		Groups:  fakeGroups(mesh), // no faces bound
		Regions: newRegionTable([]string{"Body"}),
	}
	err := resolveSpecs([]ConstraintSpec{
		DirichletSpec{SpecKind: KindTemperature, SpecName: "hot end", FaceKeys: []string{"gone"}},
	}, rc)
	if err == nil || !strings.Contains(err.Error(), "hot end") || !strings.Contains(err.Error(), "temperature") {
		t.Errorf("err = %v, want failure naming the spec kind and name", err)
	}
}
