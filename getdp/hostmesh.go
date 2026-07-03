// SPDX-License-Identifier: GPL-2.0-only

package getdp

import (
	"fmt"

	"oblikovati.org/api/wire"
)

// facetTolerance is the chordal tessellation tolerance (host model units) requested for
// the surface pull. It also sets the scale at which the volume mesher approximates curved
// faces; a panel knob can override it later.
const facetTolerance = 0.05

// pullSurface fetches the body's triangulated surface from the host and welds it into a
// watertight indexed mesh. Coordinates STAY in host model units (cm) — the one cm→m
// conversion happens in the MSH writer (units.go). The welded surface is the input to
// the volume mesher.
func (e *Engine) pullSurface(bodyIndex int) (*SurfaceMesh, error) {
	facets, err := e.api.Body().CalculateFacets(wire.CalculateFacetsArgs{
		BodyIndex: bodyIndex,
		Tolerance: facetTolerance,
	})
	if err != nil {
		return nil, fmt.Errorf("calculate facets for body %d: %w", bodyIndex, err)
	}
	surface, err := weldSurface(facets.VertexCoordinates, facets.VertexIndices)
	if err != nil {
		return nil, err
	}
	if open := surface.openEdges(); open > 0 {
		return nil, fmt.Errorf("the body surface is not watertight (%d open/non-manifold edges); it cannot be meshed into a solid", open)
	}
	return surface, nil
}

// pullFaceFacets fetches the triangulation of a single B-rep face (by reference key) in
// model units, for matching against the volume mesh's boundary facets (face-group binding).
func (e *Engine) pullFaceFacets(bodyIndex int, faceKey string) (*SurfaceMesh, error) {
	facets, err := e.api.Body().FaceCalculateFacets(wire.FaceFacetsArgs{
		BodyIndex: bodyIndex,
		FaceKey:   faceKey,
		Tolerance: facetTolerance,
	})
	if err != nil {
		return nil, fmt.Errorf("calculate facets for face %s: %w", faceKey, err)
	}
	return weldSurface(facets.VertexCoordinates, facets.VertexIndices)
}
