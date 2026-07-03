// SPDX-License-Identifier: GPL-2.0-only

package getdp

import (
	"math"
	"testing"
)

// TestEnclosingSphereContainsEveryVertex: the near-air ball's inner sphere must enclose the
// whole part, and its centre is the part's bbox centre (the centre the VolSphShell transform
// measures radius from). The AABB half-diagonal always reaches the corners, so every vertex
// lies inside.
func TestEnclosingSphereContainsEveryVertex(t *testing.T) {
	center := [3]float64{-2, 5, 1}
	part := icosphere(center, 3, 2) // a sphere the ball must wrap
	c, r := enclosingSphere(part)
	for _, v := range part.Verts {
		d := math.Sqrt((v[0]-c[0])*(v[0]-c[0]) + (v[1]-c[1])*(v[1]-c[1]) + (v[2]-c[2])*(v[2]-c[2]))
		if d > r*(1+1e-12) {
			t.Fatalf("vertex %v at distance %.6g lies outside enclosing sphere r=%.6g", v, d, r)
		}
	}
	if dc := math.Sqrt((c[0]-center[0])*(c[0]-center[0]) + (c[1]-center[1])*(c[1]-center[1]) + (c[2]-center[2])*(c[2]-center[2])); dc > 1e-9 {
		t.Errorf("centre = %v, want the sphere centre %v (bbox centre)", c, center)
	}
}

// TestSubdivisionForSizeTracksMeshSize: the inner icosphere's facet size should track the mesh
// size (finer mesh → more subdivisions), clamped to a sane [1,4] band so a coarse mesh still
// gets a round sphere and a fine mesh does not explode the facet count.
func TestSubdivisionForSizeTracksMeshSize(t *testing.T) {
	if n := subdivisionForSize(10, 100); n != 1 {
		t.Errorf("coarse mesh subdiv = %d, want 1 (min)", n)
	}
	if n := subdivisionForSize(10, 1e-4); n != 4 {
		t.Errorf("very fine mesh subdiv = %d, want 4 (max)", n)
	}
	if subdivisionForSize(10, 5) > subdivisionForSize(10, 1) {
		t.Errorf("subdivision must be monotone non-decreasing as mesh size shrinks")
	}
}
