// SPDX-License-Identifier: GPL-2.0-only

package getdp

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// gmsh MSH 2.2 element type codes we consume.
const (
	gmshTet4  = 4  // 4-node first-order tetrahedron
	gmshTet10 = 11 // 10-node second-order tetrahedron
	gmshTri3  = 2  // 3-node triangle (boundary of a first-order mesh)
	gmshTri6  = 9  // 6-node triangle (boundary of a second-order mesh)
)

// parseMSH reads a gmsh MSH version-2.2 ASCII mesh into a TetMesh. Node ordering is
// kept as gmsh emits it (GetDP shares gmsh's conventions, so no re-numbering is ever
// needed); boundary triangles are collected for face-group binding. Volume tets and
// surface triangles are the only element kinds kept.
func parseMSH(r io.Reader) (*TetMesh, error) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 1024*1024), 16*1024*1024)
	mesh := &TetMesh{}
	for sc.Scan() {
		switch strings.TrimSpace(sc.Text()) {
		case "$Nodes":
			if err := readNodes(sc, mesh); err != nil {
				return nil, err
			}
		case "$Elements":
			if err := readElements(sc, mesh); err != nil {
				return nil, err
			}
		}
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("read msh: %w", err)
	}
	if len(mesh.Nodes) == 0 || len(mesh.Elements) == 0 {
		return nil, fmt.Errorf("msh has no volume mesh (%d nodes, %d tets)", len(mesh.Nodes), len(mesh.Elements))
	}
	return mesh, nil
}

// readNodes consumes a $Nodes block: a count line then "id x y z" rows.
func readNodes(sc *bufio.Scanner, mesh *TetMesh) error {
	count, err := scanCount(sc, "$Nodes")
	if err != nil {
		return err
	}
	mesh.Nodes = make([]Node, 0, count)
	for i := 0; i < count && sc.Scan(); i++ {
		f := strings.Fields(sc.Text())
		if len(f) < 4 {
			return fmt.Errorf("msh node line %d malformed: %q", i, sc.Text())
		}
		id, _ := strconv.Atoi(f[0])
		x, _ := strconv.ParseFloat(f[1], 64)
		y, _ := strconv.ParseFloat(f[2], 64)
		z, _ := strconv.ParseFloat(f[3], 64)
		mesh.Nodes = append(mesh.Nodes, Node{ID: id, X: x, Y: y, Z: z})
	}
	return nil
}

// readElements consumes an $Elements block, keeping tetrahedra (as volume elements)
// and triangles (as boundary facets); all other element kinds are ignored.
func readElements(sc *bufio.Scanner, mesh *TetMesh) error {
	count, err := scanCount(sc, "$Elements")
	if err != nil {
		return err
	}
	for i := 0; i < count && sc.Scan(); i++ {
		nums, err := parseInts(sc.Text())
		if err != nil || len(nums) < 3 {
			return fmt.Errorf("msh element line %d malformed: %q", i, sc.Text())
		}
		addElement(mesh, nums)
	}
	return nil
}

// addElement appends one parsed element row to the mesh as a tet or boundary facet.
// Row layout (MSH 2.2): id, type, ntags, <ntags tags>, node-ids...
func addElement(mesh *TetMesh, nums []int) {
	id, etype, ntags := nums[0], nums[1], nums[2]
	nodes := nums[3+ntags:]
	switch etype {
	case gmshTet4, gmshTet10:
		mesh.Elements = append(mesh.Elements, TetElement{
			ID: id, Nodes: append([]int(nil), nodes...), Physical: physicalTag(nums, ntags),
		})
	case gmshTri6, gmshTri3:
		if len(nodes) >= 3 {
			mesh.Surface = append(mesh.Surface, BoundaryFacet{
				Nodes:    append([]int(nil), nodes...),
				Corners:  [3]int{nodes[0], nodes[1], nodes[2]},
				Face:     elementaryTag(nums, ntags),
				Physical: physicalTag(nums, ntags),
			})
		}
	}
}

// physicalTag returns the gmsh physical-group tag of an element row (the FIRST tag in MSH
// 2.2's [physical, elementary, …]); 0 when the element carries no tags.
func physicalTag(nums []int, ntags int) int {
	if ntags >= 1 {
		return nums[3]
	}
	return 0
}

// elementaryTag returns the gmsh elementary (geometric) entity tag of an element row.
// In MSH 2.2 the tags are [physical, elementary, ...]; the elementary tag (the second,
// when present) identifies the model surface a boundary triangle lies on.
func elementaryTag(nums []int, ntags int) int {
	switch {
	case ntags >= 2:
		return nums[4]
	case ntags == 1:
		return nums[3]
	default:
		return 0
	}
}

// scanCount reads the integer count line that follows a block header.
func scanCount(sc *bufio.Scanner, block string) (int, error) {
	if !sc.Scan() {
		return 0, fmt.Errorf("msh: %s block truncated before count", block)
	}
	n, err := strconv.Atoi(strings.TrimSpace(sc.Text()))
	if err != nil {
		return 0, fmt.Errorf("msh: %s count %q: %w", block, sc.Text(), err)
	}
	return n, nil
}

// parseInts splits a whitespace-separated line of integers.
func parseInts(line string) ([]int, error) {
	f := strings.Fields(line)
	out := make([]int, len(f))
	for i, s := range f {
		v, err := strconv.Atoi(s)
		if err != nil {
			return nil, err
		}
		out[i] = v
	}
	return out, nil
}
