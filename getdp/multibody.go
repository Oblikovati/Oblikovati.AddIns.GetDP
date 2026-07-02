// SPDX-License-Identifier: GPL-2.0-only

package getdp

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"oblikovati.org/api/wire"
)

// solidBodies returns the active part's solid bodies — the ones a study meshes.
// Non-solid bodies (surfaces, wires) are skipped.
func (e *Engine) solidBodies() ([]wire.BodyInfo, error) {
	list, err := e.api.Body().List()
	if err != nil {
		return nil, fmt.Errorf("list bodies: %w", err)
	}
	var solids []wire.BodyInfo
	for _, b := range list.Bodies {
		if b.Solid {
			solids = append(solids, b)
		}
	}
	if len(solids) == 0 {
		return nil, fmt.Errorf("the active part has no solid bodies to analyse")
	}
	return solids, nil
}

// meshSolidBodies meshes each solid body separately (its own gmsh run in its own workdir)
// and merges the results into one tet mesh whose elements are tagged with their source
// body, so a multi-body part is analysed as one model with per-body physical volumes.
// Bodies are meshed independently, so coincident interfaces between bonded bodies are NOT
// node-conformal (a documented limitation; the auto air region of M4 meshes part + air in
// ONE conformal gmsh run instead, precisely to avoid this).
func (e *Engine) meshSolidBodies(ctx context.Context, bins solverBinaries, opts MeshOptions, solids []wire.BodyInfo, dir string) (*TetMesh, error) {
	meshes := make([]*TetMesh, 0, len(solids))
	for i, b := range solids {
		surface, err := e.pullSurface(b.Index)
		if err != nil {
			return nil, err
		}
		bodyDir := filepath.Join(dir, fmt.Sprintf("body%d", i))
		if err := os.MkdirAll(bodyDir, 0o755); err != nil {
			return nil, fmt.Errorf("create body workdir: %w", err)
		}
		m, err := NewGmshMesher(bins.gmsh).Mesh(ctx, surface, opts, bodyDir)
		if err != nil {
			return nil, fmt.Errorf("mesh body %d (%s): %w", b.Index, b.Name, err)
		}
		meshes = append(meshes, m)
	}
	return mergeTetMeshes(meshes), nil
}

// mergeTetMeshes offsets each body mesh's node ids, element ids, and gmsh surface tags so
// the merged mesh has one global numbering, tagging every element with its source body
// index. Offsetting the surface tags per body keeps each body's face groups distinct, so the
// face→FaceKey binding never matches a facet on the wrong body.
func mergeTetMeshes(meshes []*TetMesh) *TetMesh {
	merged := &TetMesh{}
	var nodeOff, elemOff, faceOff int
	for body, m := range meshes {
		maxNode, maxElem, maxFace := 0, 0, 0
		for _, n := range m.Nodes {
			merged.Nodes = append(merged.Nodes, Node{ID: n.ID + nodeOff, X: n.X, Y: n.Y, Z: n.Z})
			maxNode = max(maxNode, n.ID)
		}
		for _, el := range m.Elements {
			merged.Elements = append(merged.Elements, TetElement{ID: el.ID + elemOff, Nodes: offsetIDs(el.Nodes, nodeOff), Body: body})
			maxElem = max(maxElem, el.ID)
		}
		for _, bf := range m.Surface {
			merged.Surface = append(merged.Surface, BoundaryFacet{
				Nodes:   offsetIDs(bf.Nodes, nodeOff),
				Corners: [3]int{bf.Corners[0] + nodeOff, bf.Corners[1] + nodeOff, bf.Corners[2] + nodeOff},
				Face:    bf.Face + faceOff,
			})
			maxFace = max(maxFace, bf.Face)
		}
		nodeOff += maxNode
		elemOff += maxElem
		faceOff += maxFace + 1
	}
	return merged
}

// offsetIDs returns a copy of ids with off added to each (re-basing a body's local node
// numbering into the merged mesh's global numbering).
func offsetIDs(ids []int, off int) []int {
	out := make([]int, len(ids))
	for i, id := range ids {
		out[i] = id + off
	}
	return out
}
