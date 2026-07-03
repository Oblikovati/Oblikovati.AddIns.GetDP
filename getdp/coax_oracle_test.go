// SPDX-License-Identifier: GPL-2.0-only

package getdp

import (
	"encoding/json"
	"math"
	"path/filepath"
	"sync"
	"testing"

	"oblikovati.org/api/wire"
)

const (
	innerFaceKey = "inner"
	outerFaceKey = "outer"
)

// coaxHost is a fake host serving one solid body shaped as a straight thick-walled tube
// (inner radius a, outer radius b, length L along z), with the inner and outer cylindrical
// walls selectable as separate faces. It exercises curved-electrode binding (#61): both walls
// carry radial normals, so only per-facet distance can tell the inner electrode from the
// outer one. Coordinates are host model units (1 unit = 10 mm).
type coaxHost struct {
	mu     sync.Mutex
	calls  map[string]int
	a, b   float64
	length float64
	seg    int
}

// newCoaxHost builds a 1→2 model-unit annulus, 3 units long, faceted into 48 segments —
// after the engine's cm→m scaling: a = 0.01 m, b = 0.02 m, L = 0.03 m.
func newCoaxHost() *coaxHost {
	return &coaxHost{calls: map[string]int{}, a: 1, b: 2, length: 3, seg: 48}
}

func (h *coaxHost) Call(method string, req []byte) ([]byte, error) {
	h.mu.Lock()
	h.calls[method]++
	h.mu.Unlock()
	switch method {
	case wire.MethodBodyList:
		return json.Marshal(wire.BodyListResult{Bodies: []wire.BodyInfo{
			{Index: 0, Name: "Dielectric", Solid: true, Key: "body0"},
		}})
	case wire.MethodBodyCalculateFacets:
		return json.Marshal(h.tubeSurface())
	case wire.MethodFaceCalculateFacets:
		return h.faceFacets(req)
	default:
		return []byte("{}"), nil
	}
}

func (h *coaxHost) saw(method string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.calls[method] > 0
}

// ringPoint is the i-th vertex of a radius-r ring at height z.
func (h *coaxHost) ringPoint(r, z float64, i int) [3]float64 {
	t := 2 * math.Pi * float64(i) / float64(h.seg)
	return [3]float64{r * math.Cos(t), r * math.Sin(t), z}
}

// wall returns the facets of the cylindrical wall at radius r, wound so the triangle normals
// point in the sign direction s of the radial (outward from the solid: +1 for the outer wall,
// −1 for the inner wall).
func (h *coaxHost) wall(r, s float64) wire.FacetSetResult {
	var coords []float64
	var idx []int
	for i := 0; i < h.seg; i++ {
		p00 := h.ringPoint(r, 0, i)
		p01 := h.ringPoint(r, 0, i+1)
		p11 := h.ringPoint(r, h.length, i+1)
		p10 := h.ringPoint(r, h.length, i)
		mid := 2 * math.Pi * (float64(i) + 0.5) / float64(h.seg)
		outward := [3]float64{s * math.Cos(mid), s * math.Sin(mid), 0}
		coords, idx = appendOrientedQuad(coords, idx, [4][3]float64{p00, p01, p11, p10}, outward)
	}
	return wire.FacetSetResult{VertexCoordinates: coords, VertexIndices: idx}
}

// tubeSurface is the whole closed tube: both walls plus the two annular end caps (outward
// normals ±z), so the welded soup is watertight for volume meshing.
func (h *coaxHost) tubeSurface() wire.FacetSetResult {
	var coords []float64
	var idx []int
	for _, w := range []wire.FacetSetResult{h.wall(h.a, -1), h.wall(h.b, +1)} {
		coords, idx = appendFacetSet(coords, idx, w)
	}
	for _, end := range []struct {
		z, out float64
	}{{0, -1}, {h.length, +1}} {
		for i := 0; i < h.seg; i++ {
			pa0 := h.ringPoint(h.a, end.z, i)
			pb0 := h.ringPoint(h.b, end.z, i)
			pb1 := h.ringPoint(h.b, end.z, i+1)
			pa1 := h.ringPoint(h.a, end.z, i+1)
			coords, idx = appendOrientedQuad(coords, idx, [4][3]float64{pa0, pb0, pb1, pa1}, [3]float64{0, 0, end.out})
		}
	}
	return wire.FacetSetResult{VertexCoordinates: coords, VertexIndices: idx}
}

// faceFacets serves the inner wall for the inner key and the outer wall for the outer key.
func (h *coaxHost) faceFacets(req []byte) ([]byte, error) {
	var args wire.FaceFacetsArgs
	if err := json.Unmarshal(req, &args); err != nil {
		return nil, err
	}
	switch args.FaceKey {
	case innerFaceKey:
		return json.Marshal(h.wall(h.a, -1))
	case outerFaceKey:
		return json.Marshal(h.wall(h.b, +1))
	default:
		return json.Marshal(wire.FacetSetResult{})
	}
}

// appendOrientedQuad appends a quad's two triangles, winding them so the triangle normal
// points along outward (flips the winding when the natural normal opposes it).
func appendOrientedQuad(coords []float64, idx []int, p [4][3]float64, outward [3]float64) ([]float64, []int) {
	order := []int{0, 1, 2, 0, 2, 3}
	if dot(triNormal(p[0], p[1], p[2]), outward) < 0 {
		order = []int{0, 2, 1, 0, 3, 2}
	}
	base := len(coords) / 3
	for _, pt := range p {
		coords = append(coords, pt[0], pt[1], pt[2])
	}
	for _, o := range order {
		idx = append(idx, base+o)
	}
	return coords, idx
}

// appendFacetSet concatenates a facet set into a running coordinate/index soup, rebasing the
// indices onto the accumulated vertex count.
func appendFacetSet(coords []float64, idx []int, s wire.FacetSetResult) ([]float64, []int) {
	base := len(coords) / 3
	coords = append(coords, s.VertexCoordinates...)
	for _, i := range s.VertexIndices {
		idx = append(idx, base+i)
	}
	return coords, idx
}

// TestElectrostaticsCoaxialCapacitanceOracle solves a coaxial capacitor end to end: the inner
// wall is the driven electrode (1 V), the outer wall is ground, the annular dielectric (εr)
// fills the gap, and the flat end caps take the natural zero-normal-D BC — which makes the
// field purely radial, i.e. an infinite coax. Oracle: C = 2π·ε₀·εr·L / ln(b/a). This can only
// pass once curved-electrode binding (#61) separates the two concentric walls.
func TestElectrostaticsCoaxialCapacitanceOracle(t *testing.T) {
	const er = 2.0
	h := newCoaxHost()
	dir := solveConfinedStudy(t, NewEngine(h), []string{innerFaceKey, outerFaceKey}, 0.35,
		PhysicsElectrostatics, Material{Epsilon: er}, []ConstraintSpec{
			DirichletSpec{SpecKind: KindVoltage, SpecName: "V+", FaceKeys: []string{innerFaceKey}, Value: 1},
			DirichletSpec{SpecKind: KindVoltage, SpecName: "GND", FaceKeys: []string{outerFaceKey}, Value: 0},
		}, nil)

	const lengthM = 0.03 // 3 model units × cm→m
	wantC := 2 * math.Pi * vacuumPermittivity * er * lengthM / math.Log(2.0)
	gotC := readTableValue(t, filepath.Join(dir, "capacitance.txt"))
	if rel := math.Abs(gotC-wantC) / wantC; rel > 0.02 {
		t.Errorf("C = %.6g F, want 2πεL/ln(b/a) = %.6g F (rel err %.2g > 2%%)", gotC, wantC, rel)
	}
	if !h.saw(wire.MethodFaceCalculateFacets) {
		t.Error("study never pulled the electrode face tessellations")
	}
}

// TestCoaxBindingSeparatesWalls is the faster, solver-free half of the curved-binding proof:
// the pipeline meshes the tube and binds the two walls; the inner electrode's bound area must
// match 2πaL and the outer's 2πbL (within remeshing tolerance), confirming no facet leaked to
// the wrong wall or to an end cap.
func TestCoaxBindingSeparatesWalls(t *testing.T) {
	bins := requireSolver(t)
	h := newCoaxHost()
	e := NewEngine(h)
	dir := t.TempDir()
	_, _, rc := meshAndBind(t, e, bins, dir, []string{innerFaceKey, outerFaceKey}, 0.35)

	inner := facetsArea(rc.Mesh, rc.Groups.Facets[innerFaceKey])
	outer := facetsArea(rc.Mesh, rc.Groups.Facets[outerFaceKey])
	wantInner := 2 * math.Pi * h.a * h.length
	wantOuter := 2 * math.Pi * h.b * h.length
	if rel := math.Abs(inner-wantInner) / wantInner; rel > 0.05 {
		t.Errorf("inner electrode area = %.4g, want 2πaL = %.4g (rel %.2g)", inner, wantInner, rel)
	}
	if rel := math.Abs(outer-wantOuter) / wantOuter; rel > 0.05 {
		t.Errorf("outer electrode area = %.4g, want 2πbL = %.4g (rel %.2g)", outer, wantOuter, rel)
	}
}
