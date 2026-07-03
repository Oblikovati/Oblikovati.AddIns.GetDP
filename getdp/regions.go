// SPDX-License-Identifier: GPL-2.0-only

package getdp

import (
	"fmt"
	"math"
)

// RegionTable allocates the physical groups that the written MSH and the generated .pro
// deck SHARE — it is the single source of the tag numbering on both sides (design spec
// §3.2). Volumes are registered first (one per body, plus the air region later); bound
// surfaces are appended as constraint specs claim them. Only regions in this table are
// emitted into the MSH ($PhysicalNames + element physical tags): the
// referenced-groups-only rule that keeps GetDP from tripping over unnamed entities.
type RegionTable struct {
	Volumes  []VolumeRegion
	Surfaces []SurfaceRegion
}

// VolumeRegion is one physical volume: a body (by merged-mesh body index) with a tag
// and a human-readable name ($PhysicalNames is what makes solver logs debuggable).
type VolumeRegion struct {
	Tag  int
	Name string
	Body int // TetElement.Body index in the merged mesh
}

// SurfaceRegion is one physical surface: the boundary facets a constraint spec bound
// (via FaceGroups) under one tag/name. AreaModelUnits is the summed facet area in
// model units² — flux-type writers divide totals (current, heat) by it to get the
// uniform surface density the deck applies.
type SurfaceRegion struct {
	Tag            int
	Name           string
	Facets         []BoundaryFacet
	AreaModelUnits float64
}

// newRegionTable seeds the table with one physical volume per body, tagged 1..n in body
// order (deterministic: the .pro Group block references the same numbers).
func newRegionTable(bodyNames []string) *RegionTable {
	t := &RegionTable{}
	for i, name := range bodyNames {
		if name == "" {
			name = fmt.Sprintf("Body%d", i+1)
		}
		t.Volumes = append(t.Volumes, VolumeRegion{Tag: i + 1, Name: name, Body: i})
	}
	return t
}

// addAirVolume registers the generated air region as one more physical volume (the last
// tag, body airBodyIndex) so the deck and MSH tag its tets alongside the part bodies.
func (t *RegionTable) addAirVolume() {
	t.Volumes = append(t.Volumes, VolumeRegion{Tag: t.nextTag(), Name: "Air", Body: airBodyIndex})
}

// BindOuterBoundary registers the air box's outer facets (mesh physical tag outerBoundaryTag)
// as one physical surface and returns its tag — the far-field boundary the electrostatic
// solve pins to zero. Unlike BindSurface it binds directly from the mesh, not a host face:
// the generated box has no B-rep face to pick.
func (t *RegionTable) BindOuterBoundary(mesh *TetMesh) (int, error) {
	var facets []BoundaryFacet
	for _, f := range mesh.Surface {
		if f.Physical == outerBoundaryTag {
			facets = append(facets, f)
		}
	}
	if len(facets) == 0 {
		return 0, fmt.Errorf("no outer-boundary facets (physical tag %d) in the mesh to bind the far-field potential", outerBoundaryTag)
	}
	tag := t.nextTag()
	t.Surfaces = append(t.Surfaces, SurfaceRegion{
		Tag: tag, Name: "FarField", Facets: facets, AreaModelUnits: facetsArea(mesh, facets),
	})
	return tag, nil
}

// BindSurface registers the union of the given face keys' bound facets as one physical
// surface and returns its tag. Surface tags continue after the volume tags, in claim
// order — deterministic because specs resolve in model order.
func (t *RegionTable) BindSurface(name string, faceKeys []string, groups *FaceGroups, mesh *TetMesh) (int, error) {
	var facets []BoundaryFacet
	for _, key := range faceKeys {
		fs, ok := groups.Facets[key]
		if !ok {
			return 0, fmt.Errorf("surface region %q: face %s is not bound to the mesh", name, key)
		}
		facets = append(facets, fs...)
	}
	if len(facets) == 0 {
		return 0, fmt.Errorf("surface region %q bound no facets (faces: %v)", name, faceKeys)
	}
	tag := t.nextTag()
	t.Surfaces = append(t.Surfaces, SurfaceRegion{
		Tag: tag, Name: name, Facets: facets, AreaModelUnits: facetsArea(mesh, facets),
	})
	return tag, nil
}

// facetsArea sums the corner-triangle areas of a facet list (model units²). Second-
// order facets use their corner triangle — exact for the planar faces flux BCs bind.
func facetsArea(mesh *TetMesh, facets []BoundaryFacet) float64 {
	index := mesh.nodeByID()
	total := 0.0
	for _, f := range facets {
		a := nodeXYZ(index[f.Corners[0]])
		b := nodeXYZ(index[f.Corners[1]])
		c := nodeXYZ(index[f.Corners[2]])
		u := [3]float64{b[0] - a[0], b[1] - a[1], b[2] - a[2]}
		v := [3]float64{c[0] - a[0], c[1] - a[1], c[2] - a[2]}
		n := [3]float64{u[1]*v[2] - u[2]*v[1], u[2]*v[0] - u[0]*v[2], u[0]*v[1] - u[1]*v[0]}
		total += 0.5 * math.Sqrt(dot(n, n))
	}
	return total
}

// nextTag returns the first unused physical tag (volumes and surfaces share one space).
func (t *RegionTable) nextTag() int {
	next := 1
	for _, v := range t.Volumes {
		if v.Tag >= next {
			next = v.Tag + 1
		}
	}
	for _, s := range t.Surfaces {
		if s.Tag >= next {
			next = s.Tag + 1
		}
	}
	return next
}

// VolumeTag returns the physical tag of the given merged-mesh body index.
func (t *RegionTable) VolumeTag(body int) (int, error) {
	for _, v := range t.Volumes {
		if v.Body == body {
			return v.Tag, nil
		}
	}
	return 0, fmt.Errorf("no physical volume registered for body index %d (have %d volumes)", body, len(t.Volumes))
}
