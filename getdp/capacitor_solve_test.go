// SPDX-License-Identifier: GPL-2.0-only

package getdp

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"

	"oblikovati.org/api/wire"
	"oblikovati.org/getdp/getdp/femmodel"
)

// slabHost is a fake host serving the capacitor demo's geometry: a square dielectric slab
// centred on the axis (X,Y ∈ [-2,2], Z ∈ [0,0.6] model units = 40×40×6 mm) whose z=0 and
// z=gap caps are the two plates. It is the fixed-geometry twin of the demo's authored slab,
// so the electrostatics + air solve can be exercised end to end on the real solvers.
type slabHost struct {
	mu    sync.Mutex
	calls map[string]int
	v     [8][3]float64
}

func newSlabHost() *slabHost {
	const w, g = 4.0, 0.6 // 40 mm plate, 6 mm gap (model units)
	return &slabHost{calls: map[string]int{}, v: [8][3]float64{
		{-w / 2, -w / 2, 0}, {w / 2, -w / 2, 0}, {w / 2, w / 2, 0}, {-w / 2, w / 2, 0},
		{-w / 2, -w / 2, g}, {w / 2, -w / 2, g}, {w / 2, w / 2, g}, {-w / 2, w / 2, g},
	}}
}

const (
	bottomPlateKey = "bottom"
	topPlateKey    = "top"
)

func (h *slabHost) Call(method string, req []byte) ([]byte, error) {
	h.mu.Lock()
	h.calls[method]++
	h.mu.Unlock()
	switch method {
	case wire.MethodBodyList:
		return json.Marshal(wire.BodyListResult{Bodies: []wire.BodyInfo{
			{Index: 0, Name: "Dielectric", Solid: true, Key: "body0"},
		}})
	case wire.MethodBodyCalculateFacets:
		return json.Marshal(h.slabSurface())
	case wire.MethodFaceCalculateFacets:
		return h.faceFacets(req)
	default:
		return []byte("{}"), nil
	}
}

// slabSurface is the whole six-face slab as a triangle soup.
func (h *slabHost) slabSurface() wire.FacetSetResult {
	quads := [6][4]int{{0, 3, 2, 1}, {4, 5, 6, 7}, {0, 1, 5, 4}, {1, 2, 6, 5}, {2, 3, 7, 6}, {3, 0, 4, 7}}
	var coords []float64
	var idx []int
	for _, q := range quads {
		coords, idx = appendQuad(coords, idx, h.v, q)
	}
	return wire.FacetSetResult{VertexCoordinates: coords, VertexIndices: idx}
}

// faceFacets serves the z=0 cap for the bottom plate and the z=gap cap for the top plate.
func (h *slabHost) faceFacets(req []byte) ([]byte, error) {
	var args wire.FaceFacetsArgs
	if err := json.Unmarshal(req, &args); err != nil {
		return nil, err
	}
	quad := [4]int{0, 3, 2, 1} // z=0 bottom
	if args.FaceKey == topPlateKey {
		quad = [4]int{4, 5, 6, 7} // z=gap top
	}
	var coords []float64
	var idx []int
	coords, idx = appendQuad(coords, idx, h.v, quad)
	return json.Marshal(wire.FacetSetResult{VertexCoordinates: coords, VertexIndices: idx})
}

// TestCapacitorDemoSlabSolvesWithAir drives the capacitor demo's physics setup end to end on
// the real solvers: the dielectric slab meshed with its automatic air box, V+ / ground on the
// two plates. It must solve, produce an electric-potential field, and report a positive
// capacitance in a physically sane band around the ideal C = ε₀·εr·A/d ≈ 9.4 pF (higher, from
// the open-air fringing). This proves the demo's two-plate air-region setup — the live path
// the MCPBridge/head walkthrough drives — is green, not just that it configures.
func TestCapacitorDemoSlabSolvesWithAir(t *testing.T) {
	bins := requireSolver(t)
	t.Setenv("OBK_GETDP_BIN", bins.getdp)
	t.Setenv("OBK_GMSH_BIN", bins.gmsh)
	e := NewEngine(newSlabHost())
	e.withAnalysis(func(a *femmodel.Analysis) {
		s := a.AddStudy(femmodel.PhysicsElectrostatics)
		mustAddBC(t, s, femmodel.ConstraintObject{Kind: femmodel.KindVoltage, Name: "V+",
			Faces: []string{topPlateKey}, Value: 1})
		mustAddBC(t, s, femmodel.ConstraintObject{Kind: femmodel.KindVoltage, Name: "Ground",
			Faces: []string{bottomPlateKey}, Value: 0})
		s.Mesh.SizeModelUnits = 0.3
		s.Solver.Air.PaddingFactor = 1.5
		if regs := s.Regions(); len(regs) > 0 {
			regs[0].Material.Epsilon = 4
			if err := s.UpdateRegion(regs[0]); err != nil {
				t.Fatal(err)
			}
		}
	})

	res, err := e.RunStudyOnHost(context.Background())
	if err != nil {
		t.Fatalf("RunStudyOnHost: %v", err)
	}
	if res.FieldLabel != "electric potential" || res.FieldMax <= res.FieldMin {
		t.Errorf("field = %q %g…%g, want a potential range", res.FieldLabel, res.FieldMin, res.FieldMax)
	}
	c := capacitanceScalar(t, res)
	const ideal = 9.44e-12 // ε₀·4·(0.04²)/0.006
	if c < 0.5*ideal || c > 5*ideal {
		t.Errorf("capacitance = %.3g F, want a sane value near/above ideal %.3g F (open-air fringing)", c, ideal)
	}
}

// capacitanceScalar pulls the reported capacitance from a study result.
func capacitanceScalar(t *testing.T, res *StudyResult) float64 {
	t.Helper()
	for _, s := range res.Scalars {
		if strings.Contains(s.Label, "capacitance") {
			return s.Value
		}
	}
	t.Fatalf("no capacitance scalar in %+v", res.Scalars)
	return 0
}
