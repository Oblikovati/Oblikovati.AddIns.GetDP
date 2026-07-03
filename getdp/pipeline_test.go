// SPDX-License-Identifier: GPL-2.0-only

package getdp

import (
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"testing"
)

// volumeOraclePro is the hand-written deck the M2 pipeline test feeds GetDP: it solves
// a Laplace potential between the two bound physical surfaces and integrates 1 over the
// physical volume. Region tags are injected by the test from the RegionTable — the same
// numbers the MSH writer emitted, which is exactly the contract under test.
const volumeOraclePro = `Group {
  Vol = Region[%d];
  Inlet = Region[%d];
  Outlet = Region[%d];
}
Function { sigma[] = 1.0; }
Constraint {
  { Name SetV; Case {
      { Region Inlet; Value 1.0; }
      { Region Outlet; Value 0.0; }
  } }
}
Jacobian { { Name JVol; Case { { Region All; Jacobian Vol; } } } }
Integration {
  { Name I1; Case { { Type Gauss; Case {
      { GeoElement Tetrahedron; NumberOfPoints 4; }
      { GeoElement Triangle; NumberOfPoints 3; }
  } } } }
}
FunctionSpace {
  { Name Hgrad_v; Type Form0;
    BasisFunction {
      { Name sn; NameOfCoef vn; Function BF_Node;
        Support Region[{Vol, Inlet, Outlet}]; Entity NodesOf[All]; }
    }
    Constraint { { NameOfCoef vn; EntityType NodesOf; NameOfConstraint SetV; } }
  }
}
Formulation {
  { Name EleKin; Type FemEquation;
    Quantity { { Name v; Type Local; NameOfSpace Hgrad_v; } }
    Equation {
      Galerkin { [ sigma[] * Dof{d v}, {d v} ]; In Vol; Jacobian JVol; Integration I1; }
    }
  }
}
Resolution {
  { Name R; System { { Name A; NameOfFormulation EleKin; } }
    Operation { Generate[A]; Solve[A]; SaveSolution[A]; } }
}
PostProcessing {
  { Name PP; NameOfFormulation EleKin;
    Quantity {
      { Name vol; Value { Integral { [ 1 ]; In Vol; Jacobian JVol; Integration I1; } } }
    }
  }
}
PostOperation {
  { Name Oracle; NameOfPostProcessing PP;
    Operation { Print[ vol[Vol], OnGlobal, Format Table, File "vol.txt" ]; } }
}
`

// TestPipelineBoxHostToGetDPVolumeOracle drives the whole M2 pipeline against the real
// vendored toolchain: boxHost surface pull → weld → gmsh volume mesh → FaceKey binding
// of the inlet/outlet faces → RegionTable + constraint specs → Go-written MSH 2.2 (the
// cm→m seam) → GetDP solve. Oracle (exact): the boxHost part is 20×1×1 model units
// (cm), so the physical volume GetDP integrates must be 0.2 × 0.01 × 0.01 = 2e-5 m³ —
// tets fill the planar box exactly, so only roundoff separates them.
func TestPipelineBoxHostToGetDPVolumeOracle(t *testing.T) {
	bins := requireSolver(t)
	e := NewEngine(newBoxHost())
	dir := t.TempDir()

	mesh, regions, rc := meshAndBindBox(t, e, bins, dir)
	if err := resolveSpecs(boxSpecs(), rc); err != nil {
		t.Fatalf("resolve specs: %v", err)
	}
	mshPath := filepath.Join(dir, "study.msh")
	if err := writeFile(mshPath, func(f *os.File) error { return writeMSH(f, mesh, regions) }); err != nil {
		t.Fatalf("write msh: %v", err)
	}
	proPath := filepath.Join(dir, "study.pro")
	deck := fmt.Sprintf(volumeOraclePro, mustVolumeTag(t, regions), rc.Model.BoundPotentials[0].RegionTag,
		rc.Model.BoundPotentials[1].RegionTag)
	if err := os.WriteFile(proPath, []byte(deck), 0o644); err != nil {
		t.Fatalf("write pro: %v", err)
	}

	log, err := runGetDP(context.Background(), bins.getdp, getdpRun{
		ProPath: "study.pro", MshPath: "study.msh", Resolution: "R",
		PostOps: []string{"Oracle"}, Dir: dir,
	})
	if err != nil {
		t.Fatalf("getdp: %v\n%s", err, log)
	}
	const wantM3 = 0.20 * 0.01 * 0.01
	got := readTableValue(t, filepath.Join(dir, "vol.txt"))
	if math.Abs(got-wantM3)/wantM3 > 1e-9 {
		t.Errorf("GetDP volume integral = %.12g m³, want %.12g m³ exactly (units seam cm→m)", got, wantM3)
	}
}

// meshAndBindBox runs the host-facing half of the pipeline on the boxHost fixture:
// solid-body discovery, per-body meshing (real gmsh), inlet/outlet face binding, and
// the seeded region table.
func meshAndBindBox(t *testing.T, e *Engine, bins solverBinaries, dir string) (*TetMesh, *RegionTable, *ResolveContext) {
	t.Helper()
	return meshAndBind(t, e, bins, dir, []string{inletFaceKey, outletFaceKey}, 0.8)
}

// meshAndBind runs the host-facing half of the pipeline on any single-body fixture: solid
// mesh at the given size, then binds the named faces. It is the shared harness the box and
// coaxial oracles reuse (the coaxial one exercises curved-electrode binding, #61).
func meshAndBind(t *testing.T, e *Engine, bins solverBinaries, dir string, faceKeys []string, size float64) (*TetMesh, *RegionTable, *ResolveContext) {
	t.Helper()
	solids, err := e.solidBodies()
	if err != nil {
		t.Fatalf("solid bodies: %v", err)
	}
	mesh, err := e.meshSolidBodies(context.Background(), bins, MeshOptions{Size: size, Order: FirstOrderTet}, solids, dir)
	if err != nil {
		t.Fatalf("mesh bodies: %v", err)
	}
	groups, err := e.buildFaceGroups(faceKeys, mesh, solids)
	if err != nil {
		t.Fatalf("bind faces: %v", err)
	}
	names := make([]string, len(solids))
	for i, b := range solids {
		names[i] = b.Name
	}
	regions := newRegionTable(names)
	return mesh, regions, &ResolveContext{Model: &SolveModel{}, Mesh: mesh, Groups: groups, Regions: regions}
}

// boxSpecs is the M2 stand-in study: 1 V on the inlet face, ground on the outlet.
func boxSpecs() []ConstraintSpec {
	return []ConstraintSpec{
		DirichletSpec{SpecKind: KindVoltage, SpecName: "V+", FaceKeys: []string{inletFaceKey}, Value: 1},
		DirichletSpec{SpecKind: KindVoltage, SpecName: "GND", FaceKeys: []string{outletFaceKey}, Value: 0},
	}
}

func mustVolumeTag(t *testing.T, regions *RegionTable) int {
	t.Helper()
	tag, err := regions.VolumeTag(0)
	if err != nil {
		t.Fatalf("volume tag: %v", err)
	}
	return tag
}

// TestDecodeSelectedFaces pins the selection decoding the pipeline runs on: face refs
// decode to raw keys, non-face refs are dropped.
func TestDecodeSelectedFaces(t *testing.T) {
	refs := []string{encodeFaceRef("k1"), "edge/abc", encodeFaceRef("k2")}
	got := decodeSelectedFaces(refs)
	if len(got) != 2 || got[0] != "k1" || got[1] != "k2" {
		t.Errorf("decoded %v, want [k1 k2]", got)
	}
}
