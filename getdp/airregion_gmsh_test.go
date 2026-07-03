// SPDX-License-Identifier: GPL-2.0-only

package getdp

import (
	"context"
	"math"
	"strings"
	"testing"
)

// unitCubeSurface returns a watertight, outward-wound surface mesh of the [0,side]³ cube.
func unitCubeSurface(side float64) *SurfaceMesh {
	s := side
	verts := [][3]float64{
		{0, 0, 0}, {s, 0, 0}, {s, s, 0}, {0, s, 0},
		{0, 0, s}, {s, 0, s}, {s, s, s}, {0, s, s},
	}
	var tris [][3]int
	for _, q := range boxFaceCycles {
		tris = append(tris, [3]int{q[0], q[1], q[2]}, [3]int{q[0], q[2], q[3]})
	}
	return &SurfaceMesh{Verts: verts, Tris: tris}
}

// sumBodyVolume sums the signed tetra volumes of one body (model units³), a robust
// conformality oracle: the part body must fill exactly the cube and the air body exactly the
// box minus the cube, which only holds if the shell is a true conformal hole.
func sumBodyVolume(mesh *TetMesh, body int) float64 {
	index := mesh.nodeByID()
	total := 0.0
	for _, el := range mesh.Elements {
		if el.Body != body || len(el.Nodes) < 4 {
			continue
		}
		a := nodeXYZ(index[el.Nodes[0]])
		b := nodeXYZ(index[el.Nodes[1]])
		c := nodeXYZ(index[el.Nodes[2]])
		d := nodeXYZ(index[el.Nodes[3]])
		total += math.Abs(tetVolume(a, b, c, d))
	}
	return total
}

func tetVolume(a, b, c, d [3]float64) float64 {
	u := [3]float64{b[0] - a[0], b[1] - a[1], b[2] - a[2]}
	v := [3]float64{c[0] - a[0], c[1] - a[1], c[2] - a[2]}
	w := [3]float64{d[0] - a[0], d[1] - a[1], d[2] - a[2]}
	det := u[0]*(v[1]*w[2]-v[2]*w[1]) - u[1]*(v[0]*w[2]-v[2]*w[0]) + u[2]*(v[0]*w[1]-v[1]*w[0])
	return det / 6
}

// TestAirMeshCubeInBoxIsConformalTwoVolume drives the auto air region against the real gmsh
// binary: a cube meshed as a hole in a padded box. The oracle is exact — the part region
// must fill the cube's analytic volume and the air region the box minus the cube — which can
// only hold if the two regions share the cube's shell conformally (no overlap, no gap).
func TestAirMeshCubeInBoxIsConformalTwoVolume(t *testing.T) {
	bins := requireSolver(t)
	const side, pad = 2.0, 3.0
	surface := unitCubeSurface(side)
	dir := t.TempDir()
	mesh, err := NewGmshMesher(bins.gmsh).MeshWithAir(context.Background(), surface,
		AirSpec{PaddingFactor: pad}, MeshOptions{Size: 0.5, Order: FirstOrderTet}, dir)
	if err != nil {
		t.Fatalf("MeshWithAir: %v", err)
	}

	partVol := sumBodyVolume(mesh, 0)
	airVol := sumBodyVolume(mesh, airBodyIndex)
	if partVol == 0 || airVol == 0 {
		t.Fatalf("expected both regions meshed: part=%g air=%g", partVol, airVol)
	}
	wantPart := side * side * side // 8
	boxSide := pad * math.Sqrt(3*side*side)
	wantAir := boxSide*boxSide*boxSide - wantPart
	if rel := math.Abs(partVol-wantPart) / wantPart; rel > 0.01 {
		t.Errorf("part volume = %g, want cube %g (rel %g)", partVol, wantPart, rel)
	}
	if rel := math.Abs(airVol-wantAir) / wantAir; rel > 0.01 {
		t.Errorf("air volume = %g, want box−cube %g (rel %g)", airVol, wantAir, rel)
	}

	// Physical tags must split part (1) from air (2), and the outer boundary must be tagged.
	for _, el := range mesh.Elements {
		if el.Body == airBodyIndex && el.Physical != airVolumeTag {
			t.Fatalf("air tet %d physical = %d, want %d", el.ID, el.Physical, airVolumeTag)
		}
	}
	if !hasPhysical(mesh.Surface, outerBoundaryTag) {
		t.Error("no outer-boundary facets tagged — the far-field BC would have nothing to bind")
	}
}

// openCubeSurface returns a NON-watertight cube surface (its top face removed) — a dirty
// shell that cannot form the air-box hole, so the mesher must fail with the actionable
// explicit-air-body fallback message.
func openCubeSurface(side float64) *SurfaceMesh {
	full := unitCubeSurface(side)
	full.Tris = full.Tris[2:] // drop the two triangles of the first (bottom) face
	return full
}

// TestAirMeshOpenShellPointsToExplicitFallback asserts a dirty/open part shell fails the
// auto air mesh with a message that points the user at the explicit Air-role body path.
func TestAirMeshOpenShellPointsToExplicitFallback(t *testing.T) {
	bins := requireSolver(t)
	_, err := NewGmshMesher(bins.gmsh).MeshWithAir(context.Background(), openCubeSurface(2.0),
		AirSpec{PaddingFactor: 3}, MeshOptions{Size: 0.6, Order: FirstOrderTet}, t.TempDir())
	if err == nil {
		t.Fatal("open shell meshed without error, want an air-box failure")
	}
	if !strings.Contains(err.Error(), "Air role") {
		t.Errorf("error %q does not point to the explicit Air-role fallback", err)
	}
}

func hasPhysical(facets []BoundaryFacet, tag int) bool {
	for _, f := range facets {
		if f.Physical == tag {
			return true
		}
	}
	return false
}
