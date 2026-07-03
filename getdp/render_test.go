// SPDX-License-Identifier: GPL-2.0-only

package getdp

import "testing"

// twoFacetMesh is a mesh with one part-interface facet (shell tag) and one outer-air-box
// facet, each on its own three nodes, so the render filter can be checked node-for-node.
func twoFacetMesh() (*TetMesh, map[int]float64) {
	mesh := &TetMesh{
		Nodes: []Node{
			{ID: 1}, {ID: 2}, {ID: 3}, // shell (part) triangle
			{ID: 4}, {ID: 5}, {ID: 6}, // outer-box triangle
		},
		Surface: []BoundaryFacet{
			{Corners: [3]int{1, 2, 3}, Physical: 4},                // part/interface
			{Corners: [3]int{4, 5, 6}, Physical: outerBoundaryTag}, // air box outer wall
		},
	}
	return mesh, map[int]float64{1: 1, 2: 0.5, 3: 0, 4: 0, 5: 0, 6: 0}
}

// TestSurfaceRenderDataHidesOuterBoxForAirStudies: with hideOuter the flood plot skips the
// outer air-box facets (so the part surface is what shows through, not the surrounding box),
// and without it every boundary facet renders (the confined-study path, unchanged).
func TestSurfaceRenderDataHidesOuterBoxForAirStudies(t *testing.T) {
	mesh, field := twoFacetMesh()

	coords, indices, _ := surfaceRenderData(mesh, field, true)
	if len(indices) != 3 || len(coords) != 9 {
		t.Errorf("hideOuter render = %d indices / %d coords, want just the 3-node part facet", len(indices), len(coords))
	}

	allCoords, allIndices, _ := surfaceRenderData(mesh, field, false)
	if len(allIndices) != 6 || len(allCoords) != 18 {
		t.Errorf("confined render = %d indices / %d coords, want both facets (6 nodes)", len(allIndices), len(allCoords))
	}
}
