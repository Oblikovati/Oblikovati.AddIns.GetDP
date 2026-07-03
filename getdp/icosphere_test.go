// SPDX-License-Identifier: GPL-2.0-only

package getdp

import (
	"math"
	"testing"
)

// TestIcosphereVerticesLieExactlyOnSphere: the whole point of the infinite-shell inner
// boundary is that its vertices sit on the true sphere (radius within 1e-9·r of Rint), so the
// VolSphShell transform's f=1 identity holds on the inner face. Check every subdivision level.
func TestIcosphereVerticesLieExactlyOnSphere(t *testing.T) {
	c := [3]float64{2, -1, 3}
	const r = 1.7
	for n := 0; n <= 3; n++ {
		s := icosphere(c, r, n)
		for i, v := range s.Verts {
			d := math.Sqrt((v[0]-c[0])*(v[0]-c[0]) + (v[1]-c[1])*(v[1]-c[1]) + (v[2]-c[2])*(v[2]-c[2]))
			if math.Abs(d-r) > 1e-9*r {
				t.Fatalf("subdiv %d vert %d radius = %.15g, want %.15g", n, i, d, r)
			}
		}
	}
}

// TestIcosphereTopologyIsClosedManifold: an icosphere has 20·4^n triangles, 10·4^n+2 vertices
// (Euler V−E+F=2), and zero open edges — a watertight shell gmsh can reclassify.
func TestIcosphereTopologyIsClosedManifold(t *testing.T) {
	for n := 0; n <= 3; n++ {
		s := icosphere([3]float64{0, 0, 0}, 1, n)
		wantTris := 20 * intPow(4, n)
		wantVerts := 10*intPow(4, n) + 2
		if len(s.Tris) != wantTris {
			t.Errorf("subdiv %d tris = %d, want %d", n, len(s.Tris), wantTris)
		}
		if len(s.Verts) != wantVerts {
			t.Errorf("subdiv %d verts = %d, want %d", n, len(s.Verts), wantVerts)
		}
		if open := s.openEdges(); open != 0 {
			t.Errorf("subdiv %d open edges = %d, want 0 (watertight)", n, open)
		}
		e := 30 * intPow(4, n)
		if chi := len(s.Verts) - e + len(s.Tris); chi != 2 {
			t.Errorf("subdiv %d Euler characteristic = %d, want 2", n, chi)
		}
	}
}

// TestIcosphereTrianglesConsistentlyWound: every face normal points outward (dot with the
// centroid-to-face vector is positive), so the STL fed to gmsh has a coherent orientation.
func TestIcosphereTrianglesConsistentlyWound(t *testing.T) {
	c := [3]float64{0, 0, 0}
	s := icosphere(c, 1, 2)
	for i, tri := range s.Tris {
		a, b, v := s.Verts[tri[0]], s.Verts[tri[1]], s.Verts[tri[2]]
		n := triNormal(a, b, v)
		mid := [3]float64{(a[0] + b[0] + v[0]) / 3, (a[1] + b[1] + v[1]) / 3, (a[2] + b[2] + v[2]) / 3}
		if n[0]*mid[0]+n[1]*mid[1]+n[2]*mid[2] <= 0 {
			t.Fatalf("triangle %d winds inward (normal·radius ≤ 0)", i)
		}
	}
}

// intPow is a small integer power for the topology counts (no float rounding).
func intPow(base, exp int) int {
	out := 1
	for i := 0; i < exp; i++ {
		out *= base
	}
	return out
}
