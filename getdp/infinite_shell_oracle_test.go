// SPDX-License-Identifier: GPL-2.0-only

package getdp

import (
	"context"
	"encoding/json"
	"math"
	"sync"
	"testing"

	"oblikovati.org/api/wire"
	"oblikovati.org/getdp/getdp/femmodel"
)

const sphereFaceKey = "conductor"

// sphereHost is a fake host serving one solid body shaped as a sphere of the given radius
// (model units) centred at the origin, its whole surface selectable as a single conductor face.
// It is the isolated-conductor oracle fixture (#25): the capacitance of a sphere in free space
// is C = 4πε₀R, reachable ONLY when the far boundary is truly at infinity (the shell), not a box.
type sphereHost struct {
	mu    sync.Mutex
	calls map[string]int
	sph   *SurfaceMesh
}

func newSphereHost(radius float64, subdiv int) *sphereHost {
	return &sphereHost{calls: map[string]int{}, sph: icosphere([3]float64{}, radius, subdiv)}
}

func (h *sphereHost) Call(method string, req []byte) ([]byte, error) {
	h.mu.Lock()
	h.calls[method]++
	h.mu.Unlock()
	switch method {
	case wire.MethodBodyList:
		return json.Marshal(wire.BodyListResult{Bodies: []wire.BodyInfo{
			{Index: 0, Name: "Conductor", Solid: true, Key: "body0"},
		}})
	case wire.MethodBodyCalculateFacets, wire.MethodFaceCalculateFacets:
		return json.Marshal(h.soup())
	default:
		return []byte("{}"), nil
	}
}

// soup returns the whole sphere surface as a triangle soup (both the body surface and the
// single conductor face resolve to it).
func (h *sphereHost) soup() wire.FacetSetResult {
	var coords []float64
	for _, v := range h.sph.Verts {
		coords = append(coords, v[0], v[1], v[2])
	}
	var idx []int
	for _, t := range h.sph.Tris {
		idx = append(idx, t[0], t[1], t[2])
	}
	return wire.FacetSetResult{VertexCoordinates: coords, VertexIndices: idx}
}

// solveSphereCapacitance runs an electrostatics study on the sphere conductor with the given
// air truncation, returning the reported capacitance (F).
func solveSphereCapacitance(t *testing.T, bins solverBinaries, radius float64, trunc femmodel.Truncation, padding float64) float64 {
	t.Helper()
	t.Setenv("OBK_GETDP_BIN", bins.getdp)
	t.Setenv("OBK_GMSH_BIN", bins.gmsh)
	e := NewEngine(newSphereHost(radius, 3))
	e.withAnalysis(func(a *femmodel.Analysis) {
		s := a.AddStudy(femmodel.PhysicsElectrostatics)
		mustAddBC(t, s, femmodel.ConstraintObject{Kind: femmodel.KindVoltage, Name: "V+",
			Faces: []string{sphereFaceKey}, Value: 1})
		s.Mesh.SizeModelUnits = 0.25
		s.Solver.Air.Truncation = trunc
		s.Solver.Air.PaddingFactor = padding
	})
	res, err := e.RunStudyOnHost(context.Background())
	if err != nil {
		t.Fatalf("RunStudyOnHost (truncation %v): %v", trunc, err)
	}
	return capacitanceScalar(t, res)
}

// TestIsolatedSphereCapacitanceShellOracle is the #25 acceptance oracle: the capacitance of an
// isolated conducting sphere is C = 4πε₀R. The infinite shell (far boundary mapped to infinity)
// must hit it within 2%; a small padded box CANNOT — its grounded walls sit at finite distance,
// so it over-reads C by C_box = 4πε₀·Rb/(b−R) > 4πε₀R. The test asserts both: the shell is
// within 2%, and the box is measurably worse.
func TestIsolatedSphereCapacitanceShellOracle(t *testing.T) {
	bins := requireSolver(t)
	const radius = 1.0 // model units → 0.01 m
	idealC := 4 * math.Pi * vacuumPermittivity * (radius * modelUnitM)

	shellC := solveSphereCapacitance(t, bins, radius, femmodel.TruncationInfiniteShell, 0)
	shellErr := math.Abs(shellC-idealC) / idealC
	t.Logf("shell  C = %.6g F (ideal %.6g F), rel err %.3f%%", shellC, idealC, 100*shellErr)
	if shellErr > 0.02 {
		t.Errorf("infinite-shell C = %.6g F, want 4πε₀R = %.6g F within 2%% (got %.3f%%)", shellC, idealC, 100*shellErr)
	}

	boxC := solveSphereCapacitance(t, bins, radius, femmodel.TruncationPaddedBox, 2.0)
	boxErr := math.Abs(boxC-idealC) / idealC
	t.Logf("box    C = %.6g F, rel err %.3f%%", boxC, 100*boxErr)
	if boxErr <= shellErr {
		t.Errorf("padded box (err %.3f%%) should be measurably WORSE than the shell (err %.3f%%) for an isolated sphere",
			100*boxErr, 100*shellErr)
	}
}
