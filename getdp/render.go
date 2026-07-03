// SPDX-License-Identifier: GPL-2.0-only

package getdp

import "oblikovati.org/api/wire"

// resultClientID is the client-graphics group the field result is pushed under.
const resultClientID = "getdp.result"

// fieldMapperName is the registered color mapper the scalar-field flood plot uses.
const fieldMapperName = "getdp.scalarfield"

// fieldColorStops is the blue→cyan→green→yellow→red ramp (RGBA) every scalar field
// spans — the customary FEA rainbow, matching the reference add-in's plots.
var fieldColorStops = [][4]float32{
	{0.05, 0.05, 0.85, 1}, {0.05, 0.75, 0.95, 1}, {0.1, 0.85, 0.25, 1},
	{0.95, 0.9, 0.1, 1}, {0.9, 0.1, 0.1, 1},
}

// rampMapper builds a color mapper spanning [lo, hi] across the ramp. A degenerate
// range is widened to a unit span so the mapper stays valid.
func rampMapper(lo, hi float64) wire.GraphicsColorMapper {
	if hi <= lo {
		hi = lo + 1
	}
	n := len(fieldColorStops)
	values := make([]float64, n)
	colors := make([]float32, 0, n*4)
	for i, stop := range fieldColorStops {
		values[i] = lo + (hi-lo)*float64(i)/float64(n-1)
		colors = append(colors, stop[0], stop[1], stop[2], stop[3])
	}
	return wire.GraphicsColorMapper{Values: values, Colors: colors}
}

// renderScalarField paints a nodal scalar field (potential, temperature) over the mesh
// surface as a flood plot spanning [lo, hi]. Mesh coordinates are already host model
// units (the pipeline never converts them), so the viewport gets them verbatim.
func (e *Engine) renderScalarField(mesh *TetMesh, values map[int]float64, lo, hi float64) error {
	coords, indices, scalars := surfaceRenderData(mesh, values)
	mapper := rampMapper(lo, hi)
	if err := e.api.Graphics().RegisterColorMapper(fieldMapperName, mapper); err != nil {
		return err
	}
	_, err := e.api.Graphics().AddFloodPlot(resultClientID, coords, indices, scalars, mapper, 1.0)
	return err
}

// surfaceRenderData flattens the mesh surface into the (coords, triangle-indices,
// per-vertex scalar) arrays the flood plot expects. Only the corner nodes of the
// boundary facets are emitted (a linear triangle skin), each carrying its nodal value.
func surfaceRenderData(mesh *TetMesh, field map[int]float64) ([]float64, []int, []float64) {
	index := mesh.nodeByID()
	slot := make(map[int]int) // node id -> 0-based render vertex
	var coords, scalars []float64
	var indices []int
	for _, bf := range mesh.Surface {
		for _, nid := range bf.Corners {
			if _, ok := slot[nid]; !ok {
				slot[nid] = len(coords) / 3
				n := index[nid]
				coords = append(coords, n.X, n.Y, n.Z)
				scalars = append(scalars, field[nid])
			}
			indices = append(indices, slot[nid])
		}
	}
	return coords, indices, scalars
}
