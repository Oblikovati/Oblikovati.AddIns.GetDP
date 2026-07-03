// SPDX-License-Identifier: GPL-2.0-only

package getdp

import (
	"encoding/json"
	"math"
	"sync"

	"oblikovati.org/api/wire"
)

// coilHost is a fake host serving one solid body shaped as a coil (a body of revolution about
// the z-axis) whose whole volume is the current source. It is the magnetostatics oracle
// fixture (#27): an azimuthal current in a body of revolution has js·n = 0 on every face, so
// the discrete source stays consistent with the ungauged edge-element system.
type coilHost struct {
	mu     sync.Mutex
	calls  map[string]int
	soupFn func() wire.FacetSetResult
}

// solenoidCoilHost serves an annular cylinder (a solenoid winding): inner radius ri, outer
// radius ro, from z=-halfLen to +halfLen, about the z-axis.
func solenoidCoilHost(ri, ro, halfLen float64, nSeg int) *coilHost {
	return &coilHost{calls: map[string]int{}, soupFn: func() wire.FacetSetResult {
		return annularCylinderSoup(ri, ro, halfLen, nSeg)
	}}
}

func (h *coilHost) Call(method string, _ []byte) ([]byte, error) {
	h.mu.Lock()
	h.calls[method]++
	h.mu.Unlock()
	switch method {
	case wire.MethodBodyList:
		return json.Marshal(wire.BodyListResult{Bodies: []wire.BodyInfo{
			{Index: 0, Name: "Coil", Solid: true, Key: "body0"},
		}})
	case wire.MethodBodyCalculateFacets, wire.MethodFaceCalculateFacets:
		return json.Marshal(h.soupFn())
	default:
		return []byte("{}"), nil
	}
}

// annularCylinderSoup triangulates a closed annular cylinder (tube) about the z-axis: outer
// wall, inner wall, and the top and bottom annular caps. nSeg segments around.
func annularCylinderSoup(ri, ro, halfLen float64, nSeg int) wire.FacetSetResult {
	b := &soupBuilder{}
	ob := b.ring(ro, -halfLen, nSeg)
	ot := b.ring(ro, +halfLen, nSeg)
	ib := b.ring(ri, -halfLen, nSeg)
	it := b.ring(ri, +halfLen, nSeg)
	for k := 0; k < nSeg; k++ {
		j := (k + 1) % nSeg
		b.quad(ob[k], ob[j], ot[j], ot[k]) // outer wall (outward +r)
		b.quad(it[k], it[j], ib[j], ib[k]) // inner wall (outward -r)
		b.quad(ot[k], ot[j], it[j], it[k]) // top cap (+z)
		b.quad(ib[k], ib[j], ob[j], ob[k]) // bottom cap (-z)
	}
	return b.result()
}

// torusSoup triangulates a torus about the z-axis: major (ring) radius R, minor (wire) radius
// a, with nMajor segments around the ring and nMinor around the wire cross-section.
func torusSoup(ringR, wireA float64, nMajor, nMinor int) wire.FacetSetResult {
	b := &soupBuilder{}
	grid := make([][]int, nMajor)
	for i := 0; i < nMajor; i++ {
		phi := 2 * math.Pi * float64(i) / float64(nMajor)
		grid[i] = make([]int, nMinor)
		for j := 0; j < nMinor; j++ {
			theta := 2 * math.Pi * float64(j) / float64(nMinor)
			r := ringR + wireA*math.Cos(theta)
			grid[i][j] = b.vertex(r*math.Cos(phi), r*math.Sin(phi), wireA*math.Sin(theta))
		}
	}
	for i := 0; i < nMajor; i++ {
		in := (i + 1) % nMajor
		for j := 0; j < nMinor; j++ {
			jn := (j + 1) % nMinor
			b.quad(grid[i][j], grid[in][j], grid[in][jn], grid[i][jn])
		}
	}
	return b.result()
}

// soupBuilder accumulates a triangle soup (deduplicated by index) for the coil fixtures.
type soupBuilder struct {
	coords []float64
	idx    []int
}

func (b *soupBuilder) vertex(x, y, z float64) int {
	id := len(b.coords) / 3
	b.coords = append(b.coords, x, y, z)
	return id
}

// ring appends nSeg vertices on a circle of the given radius at height z, returning their ids.
func (b *soupBuilder) ring(radius, z float64, nSeg int) []int {
	ids := make([]int, nSeg)
	for k := 0; k < nSeg; k++ {
		a := 2 * math.Pi * float64(k) / float64(nSeg)
		ids[k] = b.vertex(radius*math.Cos(a), radius*math.Sin(a), z)
	}
	return ids
}

// quad emits two triangles for the quad (a,b,c,d), wound consistently.
func (b *soupBuilder) quad(a, bb, c, d int) {
	b.idx = append(b.idx, a, bb, c, a, c, d)
}

func (b *soupBuilder) result() wire.FacetSetResult {
	return wire.FacetSetResult{VertexCoordinates: b.coords, VertexIndices: b.idx}
}
