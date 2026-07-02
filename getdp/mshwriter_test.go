// SPDX-License-Identifier: GPL-2.0-only

package getdp

import (
	"strings"
	"testing"
)

// oneTetMesh is a deterministic single-tet, single-boundary-facet mesh for golden tests.
// Coordinates are model units (cm): node 2 sits at x = 1 cm.
func oneTetMesh() *TetMesh {
	return &TetMesh{
		Nodes: []Node{
			{ID: 1, X: 0, Y: 0, Z: 0}, {ID: 2, X: 1, Y: 0, Z: 0},
			{ID: 3, X: 0, Y: 1, Z: 0}, {ID: 4, X: 0, Y: 0, Z: 1},
		},
		Elements: []TetElement{{ID: 1, Nodes: []int{1, 2, 3, 4}, Body: 0}},
		Surface:  []BoundaryFacet{{Nodes: []int{1, 2, 3}, Corners: [3]int{1, 2, 3}, Face: 7}},
	}
}

// goldenMSH pins the exact writer output: $PhysicalNames for the volume and the one
// referenced surface, node coordinates in METRES (1 cm → 0.01 m), boundary triangles
// before tets, sequential element ids, physical=elementary tag pairs.
const goldenMSH = `$MeshFormat
2.2 0 8
$EndMeshFormat
$PhysicalNames
2
2 2 "inlet"
3 1 "Body1"
$EndPhysicalNames
$Nodes
4
1 0 0 0
2 0.01 0 0
3 0 0.01 0
4 0 0 0.01
$EndNodes
$Elements
2
1 2 2 2 2 1 2 3
2 4 2 1 1 1 2 3 4
$EndElements
`

func TestWriteMSHGolden(t *testing.T) {
	mesh := oneTetMesh()
	regions := newRegionTable([]string{""}) // unnamed body -> "Body1", tag 1
	if _, err := regions.BindSurface("inlet", []string{"k"}, fakeGroups(mesh, "k")); err != nil {
		t.Fatalf("bind: %v", err)
	}
	var sb strings.Builder
	if err := writeMSH(&sb, mesh, regions); err != nil {
		t.Fatalf("writeMSH: %v", err)
	}
	if sb.String() != goldenMSH {
		t.Errorf("writer output drifted from golden:\n--- got ---\n%s--- want ---\n%s", sb.String(), goldenMSH)
	}
}

// fakeGroups binds face key(s) straight to the mesh's boundary facets, skipping the
// geometric match — the writer under test only cares about the resulting facet lists.
func fakeGroups(mesh *TetMesh, keys ...string) *FaceGroups {
	g := &FaceGroups{
		Facets:  map[string][]BoundaryFacet{},
		Nodes:   map[string][]int{},
		Normals: map[string][3]float64{},
	}
	for _, k := range keys {
		g.Facets[k] = mesh.Surface
	}
	return g
}

func TestWriteMSHOmitsUnreferencedGroups(t *testing.T) {
	mesh := oneTetMesh()
	regions := newRegionTable([]string{"Bar"})
	var sb strings.Builder
	if err := writeMSH(&sb, mesh, regions); err != nil {
		t.Fatalf("writeMSH: %v", err)
	}
	out := sb.String()
	// The unbound boundary facet (gmsh face 7) must NOT appear: only referenced groups
	// are emitted (the Mesh.SaveAll pitfall this writer exists to avoid).
	if strings.Contains(out, " 2 2 ") {
		t.Errorf("unreferenced boundary facets were emitted:\n%s", out)
	}
	if !strings.Contains(out, `3 1 "Bar"`) {
		t.Errorf("named physical volume missing:\n%s", out)
	}
}

func TestWriteMSHRejectsUnregisteredBody(t *testing.T) {
	mesh := oneTetMesh()
	mesh.Elements[0].Body = 3 // no volume region registered for body 3
	var sb strings.Builder
	err := writeMSH(&sb, mesh, newRegionTable([]string{"A"}))
	if err == nil || !strings.Contains(err.Error(), "body 3") {
		t.Errorf("err = %v, want a loud unregistered-body failure naming body 3", err)
	}
}
