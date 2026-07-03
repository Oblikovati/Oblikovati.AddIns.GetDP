// SPDX-License-Identifier: GPL-2.0-only

package getdp

import (
	"fmt"
	"math"
)

// shellInnerTag is the gmsh physical-surface tag the near-air ball's inner-sphere boundary
// carries, so appendInfiniteShell can find the facets to extrude radially (#25).
const shellInnerTag = 5

// shellBodyIndex is the merged-mesh body index of the infinite-shell tets — above the air
// body so it registers as its own physical volume, mapped to the VolSphShell Jacobian in the
// deck while every other body keeps the plain Vol Jacobian.
const shellBodyIndex = 1 << 21

// appendInfiniteShell turns the near-air ball's inner-sphere boundary (facets tagged
// shellInnerTag, at radius ≈Rint about center) into a single STRUCTURED layer of radial prisms
// reaching Rext, so GetDP's VolSphShell{Rint,Rext} transform maps the shell to infinity. It
// snaps the inner nodes onto the exact Rint sphere (making the transform's f=1 identity hold on
// the inner face), creates one outer node per inner node on the exact Rext sphere (so every
// radial edge is exactly radial), splits each inner facet's prism into three conformally-shared
// tets, and tags the new outer triangles as the far-field boundary. It errors — never emits a
// bad element — on a degenerate inner facet or an inverted tet (#25 design; see infshell notes).
func appendInfiniteShell(mesh *TetMesh, center [3]float64, rInt, rExt float64) error {
	inner := innerShellFacets(mesh)
	if len(inner) == 0 {
		return fmt.Errorf("infinite shell: no inner-sphere facets (physical tag %d) to extrude; "+
			"the near-air ball produced no inner boundary", shellInnerTag)
	}
	outerID := snapAndExtrude(mesh, inner, center, rInt, rExt)
	return buildShellVolume(mesh, inner, outerID, rInt)
}

// innerShellFacets collects the boundary facets on the near-air ball's inner sphere.
func innerShellFacets(mesh *TetMesh) []BoundaryFacet {
	var out []BoundaryFacet
	for _, f := range mesh.Surface {
		if f.Physical == shellInnerTag {
			out = append(out, f)
		}
	}
	return out
}

// snapAndExtrude snaps every unique inner-facet node onto the exact Rint sphere and appends
// one outer node per inner node on the exact Rext sphere, returning the inner→outer id map.
func snapAndExtrude(mesh *TetMesh, inner []BoundaryFacet, center [3]float64, rInt, rExt float64) map[int]int {
	idx := nodeIndexByID(mesh)
	outer := make(map[int]int)
	nextID := maxNodeID(mesh) + 1
	for _, f := range inner {
		for _, id := range f.Corners {
			if _, done := outer[id]; done {
				continue
			}
			n := &mesh.Nodes[idx[id]]
			snapped := projectRadius(nodeXYZ(*n), center, rInt)
			n.X, n.Y, n.Z = snapped[0], snapped[1], snapped[2]
			p := projectRadius(snapped, center, rExt)
			mesh.Nodes = append(mesh.Nodes, Node{ID: nextID, X: p[0], Y: p[1], Z: p[2]})
			outer[id] = nextID
			nextID++
		}
	}
	return outer
}

// buildShellVolume emits three tets per inner facet (its radial prism) and the outer far-field
// triangle. Degenerate facets and zero-volume tets are reported, not silently meshed.
func buildShellVolume(mesh *TetMesh, inner []BoundaryFacet, outerID map[int]int, rInt float64) error {
	byID := mesh.nodeByID()
	areaTol := 1e-14 * rInt * rInt
	nextEID := maxElemID(mesh) + 1
	for _, f := range inner {
		b := f.Corners
		if facetArea(byID, b) < areaTol {
			return fmt.Errorf("infinite shell: inner facet %v is degenerate (area < %.3g); cannot form a radial prism", b, areaTol)
		}
		t := [3]int{outerID[b[0]], outerID[b[1]], outerID[b[2]]}
		for _, tet := range splitPrismToTets([6]int{b[0], b[1], b[2], t[0], t[1], t[2]}) {
			nodes, err := orientTet(byID, tet)
			if err != nil {
				return fmt.Errorf("infinite shell: facet %v: %w", b, err)
			}
			mesh.Elements = append(mesh.Elements, TetElement{ID: nextEID, Nodes: nodes, Body: shellBodyIndex})
			nextEID++
		}
		mesh.Surface = append(mesh.Surface, BoundaryFacet{
			Nodes: []int{t[0], t[1], t[2]}, Corners: t, Physical: outerBoundaryTag,
		})
	}
	return nil
}

// splitPrismToTets splits a triangular prism (bottom p0,p1,p2; top p3,p4,p5 with p[i+3] the
// radial image of p[i]) into three tets, choosing each quad-face diagonal from the lower-id
// bottom vertex so neighbouring prisms sharing a quad pick the SAME diagonal — a conformal,
// watertight single layer (Dompierre et al. 1999). Top ids always exceed bottom ids (the outer
// nodes are created last), so the global-minimum vertex is always on the bottom.
func splitPrismToTets(p [6]int) [3][4]int {
	p = rotatePrismToMinColumn(p)
	b0, b1, b2, t0, t1, t2 := p[0], p[1], p[2], p[3], p[4], p[5]
	if b1 < b2 {
		return [3][4]int{{b0, b1, b2, t2}, {b0, b1, t2, t1}, {b0, t1, t2, t0}}
	}
	return [3][4]int{{b0, b2, b1, t1}, {b0, b2, t1, t2}, {b0, t2, t1, t0}}
}

// rotatePrismToMinColumn cyclically rotates the prism's three columns so column 0 holds the
// smallest bottom-vertex id (a symmetry of the prism that fixes the split template's origin).
func rotatePrismToMinColumn(p [6]int) [6]int {
	min, r := p[0], 0
	if p[1] < min {
		min, r = p[1], 1
	}
	if p[2] < min {
		r = 2
	}
	switch r {
	case 1:
		return [6]int{p[1], p[2], p[0], p[4], p[5], p[3]}
	case 2:
		return [6]int{p[2], p[0], p[1], p[5], p[3], p[4]}
	}
	return p
}

// orientTet returns the tet's nodes reordered to positive signed volume, erroring on a
// zero-volume (degenerate) tet.
func orientTet(byID map[int]Node, tet [4]int) ([]int, error) {
	if v := signedTetVolume(byID, tet[:]); v < 0 {
		tet[2], tet[3] = tet[3], tet[2]
	} else if v == 0 {
		return nil, fmt.Errorf("shell tet %v has zero volume", tet)
	}
	return []int{tet[0], tet[1], tet[2], tet[3]}, nil
}

// signedTetVolume is the signed volume of the tet a,b,c,d (positive when d is on the positive
// side of the a,b,c plane wound CCW).
func signedTetVolume(byID map[int]Node, nodes []int) float64 {
	a := nodeXYZ(byID[nodes[0]])
	b := nodeXYZ(byID[nodes[1]])
	c := nodeXYZ(byID[nodes[2]])
	d := nodeXYZ(byID[nodes[3]])
	return dot(sub(d, a), cross(sub(b, a), sub(c, a))) / 6
}

// facetArea is the area of triangle tri (model units²).
func facetArea(byID map[int]Node, tri [3]int) float64 {
	a := nodeXYZ(byID[tri[0]])
	b := nodeXYZ(byID[tri[1]])
	c := nodeXYZ(byID[tri[2]])
	n := cross(sub(b, a), sub(c, a))
	return 0.5 * math.Sqrt(dot(n, n))
}

// tetFaces returns the four triangular faces (corner-node triples) of a tet.
func tetFaces(n []int) [4][3]int {
	return [4][3]int{
		{n[0], n[1], n[2]}, {n[0], n[1], n[3]}, {n[0], n[2], n[3]}, {n[1], n[2], n[3]},
	}
}

// sortedTriKey is the order-independent key of a triangle (ascending corner ids), used to
// count how many tets share a face.
func sortedTriKey(f [3]int) [3]int {
	a, b, c := f[0], f[1], f[2]
	if a > b {
		a, b = b, a
	}
	if b > c {
		b, c = c, b
	}
	if a > b {
		a, b = b, a
	}
	return [3]int{a, b, c}
}

// nodeIndexByID maps a node id to its slice index in mesh.Nodes (for in-place mutation).
func nodeIndexByID(mesh *TetMesh) map[int]int {
	idx := make(map[int]int, len(mesh.Nodes))
	for i, n := range mesh.Nodes {
		idx[n.ID] = i
	}
	return idx
}

// maxNodeID returns the largest node id in the mesh (0 for an empty mesh).
func maxNodeID(mesh *TetMesh) int {
	m := 0
	for _, n := range mesh.Nodes {
		if n.ID > m {
			m = n.ID
		}
	}
	return m
}

// maxElemID returns the largest element id in the mesh (0 for an empty mesh).
func maxElemID(mesh *TetMesh) int {
	m := 0
	for _, e := range mesh.Elements {
		if e.ID > m {
			m = e.ID
		}
	}
	return m
}
