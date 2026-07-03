// SPDX-License-Identifier: GPL-2.0-only

package getdp

import (
	"fmt"
	"math"

	"oblikovati.org/api/wire"
)

// FaceGroups binds each selected host face (by reference key) to the mesh entities on
// that face: the boundary facets (emitted as a Physical Surface in the written MSH),
// the node set, and the face's outward unit normal.
type FaceGroups struct {
	Facets  map[string][]BoundaryFacet
	Nodes   map[string][]int
	Normals map[string][3]float64
}

// faceNormalGate is the minimum |dot| of a facet normal and its nearest host-triangle normal
// to accept a binding (about 45°): generous enough to tolerate the tilt of a faceted curved
// wall, tight enough to reject a perpendicular rim/cap facet a wall merely touches.
const faceNormalGate = 0.7

// facetBindTolerance scales the mean facet size into the distance a bound facet may sit from
// its host face — a backstop against binding facets that lie on no selected face.
const facetBindTolerance = 1.5

// faceAgg accumulates the mesh boundary facets bound to one host face: a representative
// normal, the union of their node ids, and the facets themselves (emitted as the Physical
// Surface triangles).
type faceAgg struct {
	normalSum [3]float64
	count     int
	nodes     map[int]bool
	facets    []BoundaryFacet
}

// buildFaceGroups binds each selected host face to the mesh boundary facets that lie on it.
// Every facet is classified independently — assigned to the nearest selected host face by
// true point-to-triangle distance, gated on local normal agreement (#61). Unlike the old
// mean-normal group match this is exact for curved walls (a closed cylinder's mean normal
// cancels; per-facet distance still separates concentric electrodes) and does not depend on
// how gmsh partitioned the surface.
func (e *Engine) buildFaceGroups(faceKeys []string, mesh *TetMesh, solids []wire.BodyInfo) (*FaceGroups, error) {
	tris, err := e.hostTrisForFaces(faceKeys, solids)
	if err != nil {
		return nil, err
	}
	distTol := facetBindTolerance * characteristicFacetSize(mesh)
	aggs := classifyBoundary(mesh, tris, distTol)
	out := &FaceGroups{
		Facets:  make(map[string][]BoundaryFacet, len(faceKeys)),
		Nodes:   make(map[string][]int, len(faceKeys)),
		Normals: make(map[string][3]float64, len(faceKeys)),
	}
	for _, key := range faceKeys {
		agg := aggs[key]
		if agg == nil {
			return nil, fmt.Errorf("face %s bound no mesh boundary facets within %g model units of its "+
				"host tessellation (is it interior, or inside another body?)", key, distTol)
		}
		out.Facets[key] = agg.facets
		out.Nodes[key] = agg.nodeList()
		out.Normals[key] = agg.normal()
	}
	return out, nil
}

// hostTrisForFaces pulls each selected face's tessellation and flattens it into keyed
// triangles (with unit normals) the classifier scans.
func (e *Engine) hostTrisForFaces(faceKeys []string, solids []wire.BodyInfo) ([]hostTri, error) {
	var tris []hostTri
	for _, key := range faceKeys {
		host, err := e.pullFaceOnAnyBody(key, solids)
		if err != nil {
			return nil, err
		}
		for _, t := range host.Tris {
			a, b, c := host.Verts[t[0]], host.Verts[t[1]], host.Verts[t[2]]
			tris = append(tris, hostTri{key: key, a: a, b: b, c: c, unitNormal: triNormal(a, b, c)})
		}
	}
	return tris, nil
}

// classifyBoundary assigns every mesh boundary facet to the selected host face it lies on,
// grouping the bound facets (with their nodes and normals) per face key.
func classifyBoundary(mesh *TetMesh, tris []hostTri, distTol float64) map[string]*faceAgg {
	index := mesh.nodeByID()
	aggs := map[string]*faceAgg{}
	for _, bf := range mesh.Surface {
		c, n := facetCentroidNormal(bf, index)
		key, ok := nearestHostFace(c, n, tris, distTol, faceNormalGate)
		if !ok {
			continue
		}
		agg := aggs[key]
		if agg == nil {
			agg = &faceAgg{nodes: map[int]bool{}}
			aggs[key] = agg
		}
		agg.accumulate(n, bf)
	}
	return aggs
}

// characteristicFacetSize is the mean boundary-facet edge length (model units) — the natural
// scale the bind tolerance rides on, valid whether the mesh size was set or auto-picked.
func characteristicFacetSize(mesh *TetMesh) float64 {
	index := mesh.nodeByID()
	var sum float64
	var count int
	for _, bf := range mesh.Surface {
		a := nodeXYZ(index[bf.Corners[0]])
		b := nodeXYZ(index[bf.Corners[1]])
		c := nodeXYZ(index[bf.Corners[2]])
		sum += distance(a, b) + distance(b, c) + distance(c, a)
		count += 3
	}
	if count == 0 {
		return 0
	}
	return sum / float64(count)
}

// pullFaceOnAnyBody fetches a face's triangulation by trying each solid body until the key
// resolves — a selected face belongs to one body, but the selection ref carries no body
// index, so the engine probes (the FaceCalculateFacets handler is body-scoped). The match is
// cheap: a part has few bodies and a study few picked faces.
func (e *Engine) pullFaceOnAnyBody(key string, solids []wire.BodyInfo) (*SurfaceMesh, error) {
	var lastErr error
	for _, b := range solids {
		host, err := e.pullFaceFacets(b.Index, key)
		if err == nil {
			return host, nil
		}
		lastErr = err
	}
	return nil, fmt.Errorf("face %s not found on any of the %d solid bodies: %w", key, len(solids), lastErr)
}

// accumulate folds one facet's normal and nodes into the aggregate.
func (a *faceAgg) accumulate(normal [3]float64, bf BoundaryFacet) {
	for k := 0; k < 3; k++ {
		a.normalSum[k] += normal[k]
	}
	a.count++
	for _, n := range bf.Nodes {
		a.nodes[n] = true
	}
	a.facets = append(a.facets, bf)
}

// normal returns the (unit) mean facet normal of the group.
func (a *faceAgg) normal() [3]float64 { return normalize(a.normalSum) }

// nodeList returns the group's node ids.
func (a *faceAgg) nodeList() []int {
	ids := make([]int, 0, len(a.nodes))
	for n := range a.nodes {
		ids = append(ids, n)
	}
	return ids
}

// facetCentroidNormal returns a boundary facet's corner centroid and unit normal.
func facetCentroidNormal(bf BoundaryFacet, index map[int]Node) ([3]float64, [3]float64) {
	a := nodeXYZ(index[bf.Corners[0]])
	b := nodeXYZ(index[bf.Corners[1]])
	c := nodeXYZ(index[bf.Corners[2]])
	centroid := [3]float64{(a[0] + b[0] + c[0]) / 3, (a[1] + b[1] + c[1]) / 3, (a[2] + b[2] + c[2]) / 3}
	return centroid, triNormal(a, b, c)
}

// surfaceCentroidNormal returns the mean triangle centroid and unit mean normal of a
// host face's tessellation.
func surfaceCentroidNormal(s *SurfaceMesh) ([3]float64, [3]float64) {
	var cs, ns [3]float64
	for _, tri := range s.Tris {
		a, b, c := s.Verts[tri[0]], s.Verts[tri[1]], s.Verts[tri[2]]
		n := triNormal(a, b, c)
		for k := 0; k < 3; k++ {
			cs[k] += (a[k] + b[k] + c[k]) / 3
			ns[k] += n[k]
		}
	}
	if len(s.Tris) > 0 {
		inv := 1.0 / float64(len(s.Tris))
		cs = [3]float64{cs[0] * inv, cs[1] * inv, cs[2] * inv}
	}
	return cs, normalize(ns)
}

func nodeXYZ(n Node) [3]float64 { return [3]float64{n.X, n.Y, n.Z} }

func dot(a, b [3]float64) float64 { return a[0]*b[0] + a[1]*b[1] + a[2]*b[2] }

func distance(a, b [3]float64) float64 {
	return math.Sqrt((a[0]-b[0])*(a[0]-b[0]) + (a[1]-b[1])*(a[1]-b[1]) + (a[2]-b[2])*(a[2]-b[2]))
}

func normalize(v [3]float64) [3]float64 {
	mag := math.Sqrt(dot(v, v))
	if mag == 0 {
		return [3]float64{}
	}
	return [3]float64{v[0] / mag, v[1] / mag, v[2] / mag}
}
