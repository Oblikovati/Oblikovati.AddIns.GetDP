// SPDX-License-Identifier: GPL-2.0-only

package getdp

import (
	"math"
	"testing"
)

// innerSphereMesh builds a mesh whose only surface is an icosphere at radius rInt about
// center (tagged shellInnerTag) — the near-air ball's outer boundary the shell attaches to.
// Node coordinates are deliberately perturbed off the exact sphere (as gmsh's remesher would
// leave them) so the snap step in appendInfiniteShell is exercised.
func innerSphereMesh(center [3]float64, rInt float64, subdiv int) *TetMesh {
	sph := icosphere(center, rInt, subdiv)
	mesh := &TetMesh{}
	for i, v := range sph.Verts {
		p := projectRadius(v, center, rInt*(1+0.003*float64(i%3-1))) // ±0.3% radial jitter
		mesh.Nodes = append(mesh.Nodes, Node{ID: i + 1, X: p[0], Y: p[1], Z: p[2]})
	}
	for _, t := range sph.Tris {
		c := [3]int{t[0] + 1, t[1] + 1, t[2] + 1}
		mesh.Surface = append(mesh.Surface, BoundaryFacet{
			Nodes: []int{c[0], c[1], c[2]}, Corners: c, Physical: shellInnerTag,
		})
	}
	return mesh
}

func radiusFrom(center [3]float64, n Node) float64 {
	return math.Sqrt((n.X-center[0])*(n.X-center[0]) + (n.Y-center[1])*(n.Y-center[1]) + (n.Z-center[2])*(n.Z-center[2]))
}

// TestAppendInfiniteShellRadialStructure: the shell must be a single structured layer — one
// outer node per inner node at exactly Rext, inner nodes snapped to exactly Rint, three tets
// per inner facet, one outer facet per inner facet. These are the invariants the VolSphShell
// transform relies on (radial edges, f=1 inner / f→∞ outer).
func TestAppendInfiniteShellRadialStructure(t *testing.T) {
	c := [3]float64{1, 2, -1}
	const rInt, rExt = 2.0, 4.0
	mesh := innerSphereMesh(c, rInt, 0) // icosphere n=0: 12 verts, 20 facets
	nodesBefore := len(mesh.Nodes)

	if err := appendInfiniteShell(mesh, c, rInt, rExt); err != nil {
		t.Fatalf("appendInfiniteShell: %v", err)
	}

	if got := len(mesh.Nodes) - nodesBefore; got != 12 {
		t.Errorf("new outer nodes = %d, want 12 (one per inner vertex)", got)
	}
	if got := len(mesh.Elements); got != 60 {
		t.Errorf("shell tets = %d, want 60 (3 per inner facet)", got)
	}
	byID := mesh.nodeByID()
	for _, n := range mesh.Nodes {
		r := radiusFrom(c, n)
		onInner := math.Abs(r-rInt) < 1e-9*rInt
		onOuter := math.Abs(r-rExt) < 1e-9*rExt
		if !onInner && !onOuter {
			t.Fatalf("node %d radius = %.12g, want exactly %g or %g", n.ID, r, rInt, rExt)
		}
	}
	for _, e := range mesh.Elements {
		if e.Body != shellBodyIndex {
			t.Errorf("shell tet %d body = %d, want shellBodyIndex", e.ID, e.Body)
		}
		if v := signedTetVolume(byID, e.Nodes); v <= 0 {
			t.Errorf("shell tet %d signed volume = %g, want > 0", e.ID, v)
		}
	}
	assertShellConformal(t, mesh, byID, c, rInt, rExt)
}

// assertShellConformal verifies watertightness: every triangular face of the shell tets that
// is NOT wholly on the inner or outer sphere is shared by exactly two tets (a conformal single
// layer); faces on a sphere appear once (they are the layer's two boundaries).
func assertShellConformal(t *testing.T, mesh *TetMesh, byID map[int]Node, c [3]float64, rInt, rExt float64) {
	t.Helper()
	faceUse := map[[3]int]int{}
	for _, e := range mesh.Elements {
		for _, f := range tetFaces(e.Nodes) {
			faceUse[sortedTriKey(f)]++
		}
	}
	onSphere := func(id int, r float64) bool { return math.Abs(radiusFrom(c, byID[id])-r) < 1e-9*r }
	for key, uses := range faceUse {
		allInner := onSphere(key[0], rInt) && onSphere(key[1], rInt) && onSphere(key[2], rInt)
		allOuter := onSphere(key[0], rExt) && onSphere(key[1], rExt) && onSphere(key[2], rExt)
		want := 2
		if allInner || allOuter {
			want = 1
		}
		if uses != want {
			t.Fatalf("face %v used %d times, want %d (non-conformal shell)", key, uses, want)
		}
	}
}

// TestAppendInfiniteShellOuterFacets: the shell's outer triangles become the far-field
// boundary — one per inner facet, tagged outerBoundaryTag, all corners at Rext.
func TestAppendInfiniteShellOuterFacets(t *testing.T) {
	c := [3]float64{0, 0, 0}
	const rInt, rExt = 3.0, 6.0
	mesh := innerSphereMesh(c, rInt, 1) // 80 facets
	if err := appendInfiniteShell(mesh, c, rInt, rExt); err != nil {
		t.Fatalf("appendInfiniteShell: %v", err)
	}
	byID := mesh.nodeByID()
	outer := 0
	for _, f := range mesh.Surface {
		if f.Physical != outerBoundaryTag {
			continue
		}
		outer++
		for _, id := range f.Corners {
			if r := radiusFrom(c, byID[id]); math.Abs(r-rExt) > 1e-9*rExt {
				t.Errorf("outer facet corner %d radius = %.12g, want %g", id, r, rExt)
			}
		}
	}
	if outer != 80 {
		t.Errorf("outer facets = %d, want 80", outer)
	}
}

// TestAppendInfiniteShellRejectsDegenerateFacet: a collinear inner facet cannot form a
// valid shell prism; the builder must report it (with the facet), never emit a zero-volume tet.
func TestAppendInfiniteShellRejectsDegenerateFacet(t *testing.T) {
	c := [3]float64{0, 0, 0}
	mesh := &TetMesh{
		Nodes: []Node{{ID: 1, X: 2}, {ID: 2, X: 2, Y: 1e-13}, {ID: 3, X: 2, Y: 2e-13}},
		Surface: []BoundaryFacet{{
			Nodes: []int{1, 2, 3}, Corners: [3]int{1, 2, 3}, Physical: shellInnerTag,
		}},
	}
	if err := appendInfiniteShell(mesh, c, 2, 4); err == nil {
		t.Fatal("expected error for degenerate inner facet, got nil")
	}
}

// TestAppendInfiniteShellNoInnerFacets: with no shellInnerTag facets there is nothing to
// extrude — a clear error, not a silent empty shell.
func TestAppendInfiniteShellNoInnerFacets(t *testing.T) {
	mesh := &TetMesh{Nodes: []Node{{ID: 1, X: 2}}}
	if err := appendInfiniteShell(mesh, [3]float64{}, 2, 4); err == nil {
		t.Fatal("expected error when no inner-sphere facets are present, got nil")
	}
}
