// SPDX-License-Identifier: GPL-2.0-only

package getdp

import (
	"strings"
	"testing"
)

const sampleMSH = `$MeshFormat
2.2 0 8
$EndMeshFormat
$Nodes
5
1 0 0 0
2 1 0 0
3 0 1 0
4 0 0 1
5 1 1 1
$EndNodes
$Elements
3
1 2 2 7 42 1 2 3
2 4 2 1 300 1 2 3 4
3 4 2 1 300 2 3 4 5
$EndElements
`

func TestParseMSHKeepsTetsAndBoundary(t *testing.T) {
	mesh, err := parseMSH(strings.NewReader(sampleMSH))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(mesh.Nodes) != 5 || len(mesh.Elements) != 2 || len(mesh.Surface) != 1 {
		t.Fatalf("parsed %d nodes / %d tets / %d facets, want 5/2/1",
			len(mesh.Nodes), len(mesh.Elements), len(mesh.Surface))
	}
	if got := mesh.Surface[0].Face; got != 42 {
		t.Errorf("boundary facet elementary tag = %d, want 42", got)
	}
	if got := mesh.Elements[0].Nodes; len(got) != 4 || got[0] != 1 || got[3] != 4 {
		t.Errorf("tet nodes = %v, want [1 2 3 4] in gmsh order", got)
	}
}

func TestParseMSHRejectsEmptyMesh(t *testing.T) {
	if _, err := parseMSH(strings.NewReader("$MeshFormat\n2.2 0 8\n$EndMeshFormat\n")); err == nil {
		t.Error("empty mesh accepted")
	}
}
