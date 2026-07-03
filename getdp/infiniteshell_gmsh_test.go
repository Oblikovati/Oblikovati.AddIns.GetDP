// SPDX-License-Identifier: GPL-2.0-only

package getdp

import (
	"context"
	"math"
	"testing"
)

// TestMeshWithInfiniteShellBuildsStructuredShell drives the real gmsh mesher: a spherical part
// is wrapped in a near-air ball, and the inner sphere is extruded into the structured infinite
// shell. It must produce near-air tets, shell tets, and a WATERTIGHT far-field boundary whose
// every node sits on the Rext sphere — the mesh the VolSphShell deck solves on.
func TestMeshWithInfiniteShellBuildsStructuredShell(t *testing.T) {
	bins := requireSolver(t)
	part := icosphere([3]float64{0, 0, 0}, 1, 2) // conductor sphere, radius 1
	mesh, geom, err := NewGmshMesher(bins.gmsh).MeshWithInfiniteShell(
		context.Background(), part, ShellSpec{}, MeshOptions{Size: 0.5, Order: FirstOrderTet}, t.TempDir())
	if err != nil {
		t.Fatalf("MeshWithInfiniteShell: %v", err)
	}

	air, shell := countBodies(mesh)
	if air == 0 {
		t.Errorf("no near-air tets (body airBodyIndex)")
	}
	if shell == 0 {
		t.Errorf("no shell tets (body shellBodyIndex)")
	}

	byID := mesh.nodeByID()
	outerFacets := 0
	outerEdges := map[[2]int]int{}
	for _, f := range mesh.Surface {
		if f.Physical != outerBoundaryTag {
			continue
		}
		outerFacets++
		for _, id := range f.Corners {
			if r := radiusFrom(geom.Center, byID[id]); math.Abs(r-geom.RExt) > 1e-6*geom.RExt {
				t.Fatalf("outer facet node %d radius = %.9g, want Rext %.9g", id, r, geom.RExt)
			}
		}
		outerEdges[edgeKey(f.Corners[0], f.Corners[1])]++
		outerEdges[edgeKey(f.Corners[1], f.Corners[2])]++
		outerEdges[edgeKey(f.Corners[2], f.Corners[0])]++
	}
	if outerFacets == 0 {
		t.Fatal("no far-field (outer) facets produced")
	}
	for e, n := range outerEdges {
		if n != 2 {
			t.Fatalf("outer boundary edge %v shared by %d facets, want 2 (not watertight)", e, n)
		}
	}
}

// countBodies counts near-air and shell tets in a merged mesh.
func countBodies(mesh *TetMesh) (air, shell int) {
	for _, e := range mesh.Elements {
		switch e.Body {
		case airBodyIndex:
			air++
		case shellBodyIndex:
			shell++
		}
	}
	return air, shell
}
