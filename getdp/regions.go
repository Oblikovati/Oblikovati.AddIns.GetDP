// SPDX-License-Identifier: GPL-2.0-only

package getdp

import "fmt"

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
// (via FaceGroups) under one tag/name.
type SurfaceRegion struct {
	Tag    int
	Name   string
	Facets []BoundaryFacet
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

// BindSurface registers the union of the given face keys' bound facets as one physical
// surface and returns its tag. Surface tags continue after the volume tags, in claim
// order — deterministic because specs resolve in model order.
func (t *RegionTable) BindSurface(name string, faceKeys []string, groups *FaceGroups) (int, error) {
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
	t.Surfaces = append(t.Surfaces, SurfaceRegion{Tag: tag, Name: name, Facets: facets})
	return tag, nil
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
