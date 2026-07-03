// SPDX-License-Identifier: GPL-2.0-only

package getdp

import (
	"math"
	"testing"
)

// cylWallTris builds a faceted cylindrical wall (radius r, z ∈ [0,2], seg segments) as
// host triangles tagged with key — the tessellation a host would hand back for a selected
// cylindrical electrode.
func cylWallTris(key string, r float64, seg int) []hostTri {
	const z0, z1 = 0.0, 2.0
	var out []hostTri
	for i := 0; i < seg; i++ {
		t0 := 2 * math.Pi * float64(i) / float64(seg)
		t1 := 2 * math.Pi * float64(i+1) / float64(seg)
		p00 := [3]float64{r * math.Cos(t0), r * math.Sin(t0), z0}
		p01 := [3]float64{r * math.Cos(t1), r * math.Sin(t1), z0}
		p11 := [3]float64{r * math.Cos(t1), r * math.Sin(t1), z1}
		p10 := [3]float64{r * math.Cos(t0), r * math.Sin(t0), z1}
		out = append(out,
			hostTri{key: key, a: p00, b: p01, c: p11, unitNormal: triNormal(p00, p01, p11)},
			hostTri{key: key, a: p00, b: p11, c: p10, unitNormal: triNormal(p00, p11, p10)})
	}
	return out
}

// TestNearestHostFaceSeparatesConcentricWalls is the core of curved-electrode binding
// (#61): two concentric cylindrical walls (r=1 inner, r=2 outer) whose facets all carry
// radial normals, so the mean-normal binder cannot tell them apart. The per-facet rule must
// bind each facet to the wall it physically lies on (distance separates them), reject a
// facet whose normal is axial even if it sits on a wall (the wall/cap rim), and reject a
// facet that lies on no selected wall (an interior cap facet, and a far-away facet).
func TestNearestHostFaceSeparatesConcentricWalls(t *testing.T) {
	tris := append(cylWallTris("inner", 1, 16), cylWallTris("outer", 2, 16)...)
	const distTol, cosGate = 0.3, 0.7
	cases := []struct {
		name string
		c, n [3]float64
		want string
		ok   bool
	}{
		{"inner wall, radial normal", [3]float64{1, 0, 1}, [3]float64{-1, 0, 0}, "inner", true},
		{"outer wall, radial normal", [3]float64{2, 0, 1}, [3]float64{1, 0, 0}, "outer", true},
		{"on inner wall but axial normal (rim) → gate rejects", [3]float64{1, 0, 1}, [3]float64{0, 0, 1}, "", false},
		{"interior cap facet, no wall within tol", [3]float64{1.5, 0, 0}, [3]float64{0, 0, -1}, "", false},
		{"far from every wall", [3]float64{10, 0, 1}, [3]float64{-1, 0, 0}, "", false},
	}
	for _, tc := range cases {
		got, ok := nearestHostFace(tc.c, tc.n, tris, distTol, cosGate)
		if ok != tc.ok || got != tc.want {
			t.Errorf("%s: got (%q, %v), want (%q, %v)", tc.name, got, ok, tc.want, tc.ok)
		}
	}
}

// TestPointTriangleDistanceClosestFeature pins the closest-point distance against the three
// feature regions (interior, edge, vertex) — the exactness the concentric-wall separation
// leans on when host triangles are large relative to the facet spacing.
func TestPointTriangleDistanceClosestFeature(t *testing.T) {
	a, b, c := [3]float64{0, 0, 0}, [3]float64{2, 0, 0}, [3]float64{0, 2, 0}
	cases := []struct {
		name string
		p    [3]float64
		want float64
	}{
		{"above interior", [3]float64{0.5, 0.5, 3}, 3},
		{"beyond a vertex", [3]float64{-1, -1, 0}, math.Sqrt2},
		{"off an edge", [3]float64{1, -1, 0}, 1},
		{"inside the triangle plane", [3]float64{0.5, 0.5, 0}, 0},
	}
	for _, tc := range cases {
		if got := pointTriangleDistance(tc.p, a, b, c); math.Abs(got-tc.want) > 1e-9 {
			t.Errorf("%s: distance = %g, want %g", tc.name, got, tc.want)
		}
	}
}
