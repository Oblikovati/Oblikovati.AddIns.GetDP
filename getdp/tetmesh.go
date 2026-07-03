// SPDX-License-Identifier: GPL-2.0-only

package getdp

// Node is a mesh node: a 1-based id (gmsh/GetDP use 1-based numbering) and its
// coordinates in host model units (cm) — the MSH writer converts to metres on output.
type Node struct {
	ID      int
	X, Y, Z float64
}

// TetElement is a tetrahedral finite element: a 1-based id and the node ids of its
// corners in gmsh's native ordering (GetDP reads gmsh meshes, so no re-ordering is ever
// needed). A 4-id element is a first-order tet; a 10-id element is second-order. Body is
// the source body's index in a merged multi-body mesh (0 for a single-body mesh), used
// to assign elements to per-body physical volumes.
type TetElement struct {
	ID    int
	Nodes []int
	Body  int
	// Physical is the gmsh physical-group tag the element was saved under (0 when
	// ungrouped). The conformal air mesh groups the part and air volumes under distinct
	// tags, which assignAirBodies reads to split them into part vs air bodies.
	Physical int
}

// IsSecondOrder reports whether this element is a 10-node (second-order) tetrahedron.
func (e TetElement) IsSecondOrder() bool { return len(e.Nodes) == 10 }

// BoundaryFacet is a triangular face on the mesh surface, carrying the node ids of the
// triangle and the gmsh elementary surface tag it belongs to. Second-order meshes give
// 6-node triangles (the 3 corners plus 3 edge midpoints); Corners holds the 3 corner ids
// used for face-group matching. Face is gmsh's reclassified surface id — facets sharing
// a Face lie on one smooth B-rep face, which the engine maps to a host FaceKey.
type BoundaryFacet struct {
	Nodes   []int // 3 (first-order) or 6 (second-order) node ids
	Corners [3]int
	Face    int
	// Physical is the gmsh physical-group tag the facet was saved under (0 when ungrouped).
	// The air mesh groups the outer box boundary under one tag, which the far-field BC binds.
	Physical int
}

// TetMesh is a solid tetrahedral mesh: nodes, volume elements, and the triangular
// facets on its outer surface (used to bind boundary conditions to picked B-rep faces).
type TetMesh struct {
	Nodes    []Node
	Elements []TetElement
	Surface  []BoundaryFacet
}

// nodeByID indexes nodes by their 1-based id for O(1) coordinate lookup.
func (m *TetMesh) nodeByID() map[int]Node {
	index := make(map[int]Node, len(m.Nodes))
	for _, n := range m.Nodes {
		index[n.ID] = n
	}
	return index
}
