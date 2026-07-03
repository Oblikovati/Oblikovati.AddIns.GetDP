// SPDX-License-Identifier: GPL-2.0-only

package getdp

import (
	"bufio"
	"fmt"
	"io"
)

// writeMSH emits the mesh GetDP consumes: MSH 2.2 ASCII with $PhysicalNames, one
// Physical Volume per registered body region and one Physical Surface per bound
// constraint face group — and NOTHING else (referenced-groups-only, design spec §3.2:
// emitting every gmsh entity is the classic Mesh.SaveAll pitfall that breaks GetDP
// Group blocks). This writer is the ONE place host model units (cm) become SI metres
// (units.go modelUnitM); no -msh_scaling is ever passed to GetDP.
//
//	err := writeMSH(f, mesh, regions) // then: getdp deck.pro -msh f.Name() ...
func writeMSH(w io.Writer, mesh *TetMesh, regions *RegionTable) error {
	if err := validateBodyTags(mesh, regions); err != nil {
		return err
	}
	bw := bufio.NewWriter(w)
	writeMSHHeader(bw, regions)
	writeMSHNodes(bw, mesh.Nodes)
	writeMSHElements(bw, mesh, regions)
	return bw.Flush()
}

// validateBodyTags rejects a mesh containing elements of a body no volume region was
// registered for. GetDP would silently skip elements with an unknown physical tag —
// exactly the bug class this writer exists to prevent — so the writer fails loudly
// instead of emitting them.
func validateBodyTags(mesh *TetMesh, regions *RegionTable) error {
	tags := map[int]bool{}
	for _, v := range regions.Volumes {
		tags[v.Body] = true
	}
	for _, el := range mesh.Elements {
		if !tags[el.Body] {
			return fmt.Errorf("element %d belongs to body %d, which has no registered physical volume (registered bodies: %d)",
				el.ID, el.Body, len(regions.Volumes))
		}
	}
	return nil
}

// writeMSHHeader emits the format block and the $PhysicalNames of every registered
// region (dim 2 = surfaces, dim 3 = volumes) — named groups make GetDP diagnostics and
// mesh debugging in gmsh legible.
func writeMSHHeader(bw *bufio.Writer, regions *RegionTable) {
	fmt.Fprint(bw, "$MeshFormat\n2.2 0 8\n$EndMeshFormat\n")
	fmt.Fprintf(bw, "$PhysicalNames\n%d\n", len(regions.Volumes)+len(regions.Surfaces))
	for _, s := range regions.Surfaces {
		fmt.Fprintf(bw, "2 %d \"%s\"\n", s.Tag, s.Name)
	}
	for _, v := range regions.Volumes {
		fmt.Fprintf(bw, "3 %d \"%s\"\n", v.Tag, v.Name)
	}
	fmt.Fprint(bw, "$EndPhysicalNames\n")
}

// writeMSHNodes emits every mesh node, scaling model-unit coordinates (cm) to metres —
// the single unit conversion of the whole pipeline.
func writeMSHNodes(bw *bufio.Writer, nodes []Node) {
	fmt.Fprintf(bw, "$Nodes\n%d\n", len(nodes))
	for _, n := range nodes {
		fmt.Fprintf(bw, "%d %.16g %.16g %.16g\n", n.ID, n.X*modelUnitM, n.Y*modelUnitM, n.Z*modelUnitM)
	}
	fmt.Fprint(bw, "$EndNodes\n")
}

// writeMSHElements emits the referenced surface facets then the volume tets, with
// sequential element ids and MSH 2.2 tag pairs (physical, elementary — the physical tag
// doubles as elementary, which is valid and keeps the file minimal).
func writeMSHElements(bw *bufio.Writer, mesh *TetMesh, regions *RegionTable) {
	count := len(mesh.Elements)
	for _, s := range regions.Surfaces {
		count += len(s.Facets)
	}
	fmt.Fprintf(bw, "$Elements\n%d\n", count)
	id := 1
	for _, s := range regions.Surfaces {
		for _, f := range s.Facets {
			writeElementRow(bw, id, triType(f.Nodes), s.Tag, f.Nodes)
			id++
		}
	}
	writeVolumeElements(bw, mesh, regions, &id)
	fmt.Fprint(bw, "$EndElements\n")
}

// writeVolumeElements emits each body's tets under that body's physical-volume tag
// (validateBodyTags has already guaranteed every body is registered).
func writeVolumeElements(bw *bufio.Writer, mesh *TetMesh, regions *RegionTable, id *int) {
	tags := map[int]int{}
	for _, v := range regions.Volumes {
		tags[v.Body] = v.Tag
	}
	for _, el := range mesh.Elements {
		writeElementRow(bw, *id, tetType(el.Nodes), tags[el.Body], el.Nodes)
		*id++
	}
}

// writeElementRow emits one MSH 2.2 element row: id, type, 2 tags (physical,
// elementary), node ids.
func writeElementRow(bw *bufio.Writer, id, etype, tag int, nodes []int) {
	fmt.Fprintf(bw, "%d %d 2 %d %d", id, etype, tag, tag)
	for _, n := range nodes {
		fmt.Fprintf(bw, " %d", n)
	}
	fmt.Fprint(bw, "\n")
}

// triType returns the MSH element type for a boundary facet (3 or 6 nodes).
func triType(nodes []int) int {
	if len(nodes) == 6 {
		return gmshTri6
	}
	return gmshTri3
}

// tetType returns the MSH element type for a volume element (4 or 10 nodes).
func tetType(nodes []int) int {
	if len(nodes) == 10 {
		return gmshTet10
	}
	return gmshTet4
}
