// SPDX-License-Identifier: GPL-2.0-only

package getdp

import "testing"

func TestMergeTetMeshesOffsetsAllNumbering(t *testing.T) {
	a := &TetMesh{
		Nodes:    []Node{{ID: 1}, {ID: 2}, {ID: 3}, {ID: 4}},
		Elements: []TetElement{{ID: 1, Nodes: []int{1, 2, 3, 4}}},
		Surface:  []BoundaryFacet{{Nodes: []int{1, 2, 3}, Corners: [3]int{1, 2, 3}, Face: 1}},
	}
	b := &TetMesh{
		Nodes:    []Node{{ID: 1}, {ID: 2}, {ID: 3}, {ID: 4}},
		Elements: []TetElement{{ID: 1, Nodes: []int{1, 2, 3, 4}}},
		Surface:  []BoundaryFacet{{Nodes: []int{1, 2, 3}, Corners: [3]int{1, 2, 3}, Face: 1}},
	}
	m := mergeTetMeshes([]*TetMesh{a, b})
	if len(m.Nodes) != 8 || len(m.Elements) != 2 || len(m.Surface) != 2 {
		t.Fatalf("merged %d nodes / %d tets / %d facets, want 8/2/2",
			len(m.Nodes), len(m.Elements), len(m.Surface))
	}
	if m.Elements[1].Nodes[0] != 5 {
		t.Errorf("second body's tet starts at node %d, want offset to 5", m.Elements[1].Nodes[0])
	}
	if m.Elements[0].Body != 0 || m.Elements[1].Body != 1 {
		t.Errorf("body tags = %d/%d, want 0/1", m.Elements[0].Body, m.Elements[1].Body)
	}
	if m.Surface[0].Face == m.Surface[1].Face {
		t.Error("surface tags collide across bodies; face binding could match the wrong body")
	}
}
