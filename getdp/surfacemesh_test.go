// SPDX-License-Identifier: GPL-2.0-only

package getdp

import (
	"strings"
	"testing"
)

// boxSurface returns a raw triangle soup for an sx×sy×sz box with each face's vertices
// listed independently (24 vertices, 12 triangles) — exercising the weld, which must
// collapse the 24 shared-corner vertices down to 8.
func boxSurface(sx, sy, sz float64) ([]float64, []int) {
	v := [8][3]float64{
		{0, 0, 0}, {sx, 0, 0}, {sx, sy, 0}, {0, sy, 0},
		{0, 0, sz}, {sx, 0, sz}, {sx, sy, sz}, {0, sy, sz},
	}
	quads := [6][4]int{{0, 3, 2, 1}, {4, 5, 6, 7}, {0, 1, 5, 4}, {1, 2, 6, 5}, {2, 3, 7, 6}, {3, 0, 4, 7}}
	var coords []float64
	var idx []int
	for _, q := range quads {
		base := len(coords) / 3
		for _, c := range q {
			coords = append(coords, v[c][0], v[c][1], v[c][2])
		}
		idx = append(idx, base, base+1, base+2, base, base+2, base+3)
	}
	return coords, idx
}

func TestWeldSurfaceCollapsesSharedCorners(t *testing.T) {
	coords, idx := boxSurface(2, 1, 1)
	s, err := weldSurface(coords, idx)
	if err != nil {
		t.Fatalf("weld: %v", err)
	}
	if len(s.Verts) != 8 {
		t.Errorf("welded verts = %d, want 8", len(s.Verts))
	}
	if len(s.Tris) != 12 {
		t.Errorf("welded tris = %d, want 12", len(s.Tris))
	}
	if open := s.openEdges(); open != 0 {
		t.Errorf("box surface has %d open edges, want watertight", open)
	}
}

func TestOpenEdgesDetectsHole(t *testing.T) {
	coords, idx := boxSurface(1, 1, 1)
	s, err := weldSurface(coords, idx[:len(idx)-3]) // drop one triangle
	if err != nil {
		t.Fatalf("weld: %v", err)
	}
	if s.openEdges() == 0 {
		t.Error("surface with a missing triangle reported watertight")
	}
}

func TestWeldSurfaceRejectsMalformedInput(t *testing.T) {
	if _, err := weldSurface([]float64{1, 2}, nil); err == nil {
		t.Error("coords not divisible by 3 accepted")
	}
	if _, err := weldSurface(nil, []int{0, 1}); err == nil {
		t.Error("indices not divisible by 3 accepted")
	}
}

func TestWriteSTLRoundTrip(t *testing.T) {
	coords, idx := boxSurface(1, 1, 1)
	s, _ := weldSurface(coords, idx)
	var sb strings.Builder
	if err := s.writeSTL(&sb); err != nil {
		t.Fatalf("writeSTL: %v", err)
	}
	stl := sb.String()
	if !strings.HasPrefix(stl, "solid part") || !strings.Contains(stl, "endsolid part") {
		t.Error("STL missing solid wrapper")
	}
	if got := strings.Count(stl, "facet normal"); got != 12 {
		t.Errorf("STL has %d facets, want 12", got)
	}
}
