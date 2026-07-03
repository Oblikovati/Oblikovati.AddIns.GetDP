// SPDX-License-Identifier: GPL-2.0-only

package getdp

import (
	"context"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
)

// Physical volume/surface tags writeAirGeo assigns (shared by the .geo and the parse).
const (
	partVolumeTag    = 1
	airVolumeTag     = 2
	outerBoundaryTag = 3
)

// AirSpec configures one conformal air-mesh run.
type AirSpec struct {
	PaddingFactor float64 // box half-extent as a multiple of the part bbox diagonal
}

// MeshWithAir meshes the part AND a surrounding padded-box air region in ONE conformal gmsh
// run (spec §3.3): part tets keep body 0, air tets are tagged airBodyIndex, and the outer
// box boundary facets carry the outer physical tag. The part surface must be watertight —
// a dirty/open shell cannot be embedded, and the error points to the explicit-air-body path.
// Single part body only (the M4 oracles/demos are all one body in air); multi-body auto air
// is deferred.
func (g GmshMesher) MeshWithAir(ctx context.Context, surface *SurfaceMesh, spec AirSpec, opts MeshOptions, workdir string) (*TetMesh, error) {
	if len(surface.Tris) == 0 {
		return nil, fmt.Errorf("air mesh: empty part surface")
	}
	stlPath := filepath.Join(workdir, "part.stl")
	if err := writeFile(stlPath, func(f *os.File) error { return surface.writeSTL(f) }); err != nil {
		return nil, err
	}
	lo, hi := surfaceBBox(surface)
	b := airBox(lo, hi, spec.PaddingFactor)
	size := meshSize(opts.Size, surface)
	geoPath := filepath.Join(workdir, "air.geo")
	if err := writeFile(geoPath, func(f *os.File) error { return writeAirGeo(f, "part.stl", b, size, opts.Order) }); err != nil {
		return nil, err
	}
	mshPath := filepath.Join(workdir, "air.msh")
	if err := runGmsh(ctx, g.bin, geoPath, mshPath); err != nil {
		return nil, fmt.Errorf("air-region mesh failed — the part shell could not be embedded in the air box "+
			"(a dirty or open shell cannot form the box hole); assign a body the Air role to mesh air explicitly: %w", err)
	}
	mesh, err := readMSHFile(mshPath)
	if err != nil {
		return nil, err
	}
	assignAirBodies(mesh)
	return mesh, nil
}

// assignAirBodies maps the conformal mesh's physical volume tags onto merged-mesh body
// indexes: air-volume tets become airBodyIndex, all other tets stay body 0 (the single part
// body). The .pro/MSH region table registers a Part volume (body 0) and an Air volume
// (airBodyIndex) to match.
func assignAirBodies(mesh *TetMesh) {
	for i := range mesh.Elements {
		if mesh.Elements[i].Physical == airVolumeTag {
			mesh.Elements[i].Body = airBodyIndex
		} else {
			mesh.Elements[i].Body = 0
		}
	}
}

// surfaceBBox returns the axis-aligned bounding box of a welded surface (model units).
func surfaceBBox(surface *SurfaceMesh) (lo, hi [3]float64) {
	lo, hi = surface.Verts[0], surface.Verts[0]
	for _, v := range surface.Verts {
		for k := 0; k < 3; k++ {
			lo[k] = math.Min(lo[k], v[k])
			hi[k] = math.Max(hi[k], v[k])
		}
	}
	return lo, hi
}

// airBodyIndex is the merged-mesh body index reserved for the air volume (generated box or
// an explicit Air-role body). It sits far above any real body index so it never collides and
// so the air region sorts last in deterministic body order.
const airBodyIndex = 1 << 20

// box is an axis-aligned bounding box in host model units (cm).
type box struct{ min, max [3]float64 }

// airBox returns the padded air box around a part bbox: a cube centred on the part centroid
// whose side is paddingFactor × the part bbox diagonal (spec §3.3). Centring on the centroid
// keeps equal margin on every side; the diagonal-based side keeps the margin proportional to
// the part regardless of aspect ratio.
func airBox(lo, hi [3]float64, paddingFactor float64) box {
	d := boxDiagonal(lo, hi)
	half := paddingFactor * d / 2
	var b box
	for k := 0; k < 3; k++ {
		c := (lo[k] + hi[k]) / 2
		b.min[k], b.max[k] = c-half, c+half
	}
	return b
}

// boxDiagonal returns the length of the diagonal of the bbox [lo,hi].
func boxDiagonal(lo, hi [3]float64) float64 {
	var s float64
	for k := 0; k < 3; k++ {
		d := hi[k] - lo[k]
		s += d * d
	}
	return math.Sqrt(s)
}

// airGeoBase offsets the generated box's gmsh entity ids clear of the reclassified part
// surfaces (which take low ids from CreateGeometry).
const airGeoBase = 1000

// boxFaceCycles are the six box faces as outward-CCW corner cycles (matching the codebase's
// box-facet convention), so each generated plane surface faces out of the air volume.
var boxFaceCycles = [6][4]int{
	{0, 3, 2, 1}, {4, 5, 6, 7}, {0, 1, 5, 4}, {1, 2, 6, 5}, {2, 3, 7, 6}, {3, 0, 4, 7},
}

// writeAirGeo emits the gmsh script that meshes the part AND the surrounding air in one
// conformal run: the part STL is reclassified into a shell (Surface Loop 1 → part Volume 1),
// a padded box is built from eight corner points (Surface Loop 2), and the air volume is the
// box MINUS the part hole — Volume(2) = {2, 1}. Because the air volume is bounded by the very
// same part-shell surfaces as the part volume, the interface triangles are shared, not
// duplicated (spec §3.3, ADR-0003). Both volumes and the outer box boundary are tagged.
func writeAirGeo(w io.Writer, stlName string, b box, size float64, order ElementOrder) error {
	if order == 0 {
		order = FirstOrderTet
	}
	// Group the part shell (Physical Surface "Shell") BEFORE the box exists so Surface{:}
	// captures only the part faces. Grouping is required: gmsh saves only physical-grouped
	// elements (SaveAll would drop every physical TAG, not just ungrouped elements), and the
	// interface facets must survive for electrode face-binding — their per-face elementary
	// tags are preserved under the shared physical tag, so binding is unaffected.
	if _, err := fmt.Fprintf(w, `Merge "%s";
ClassifySurfaces{40*Pi/180, 1, 1, Pi};
CreateGeometry;
Surface Loop(1) = Surface{:};
Volume(1) = {1};
Physical Surface("Shell", 4) = Surface{:};
`, stlName); err != nil {
		return err
	}
	if err := writeBox(w, b, boxCharLength(b, size)); err != nil {
		return err
	}
	_, err := fmt.Fprintf(w, `Volume(2) = {2, 1};
Physical Volume("Part", 1) = {1};
Physical Volume("Air", 2) = {2};
Physical Surface("Outer", 3) = {%s};
Mesh.MeshSizeMax = %g;
Mesh.MeshSizeMin = 0;
Mesh.ElementOrder = %d;
Mesh.Algorithm3D = 1;
Mesh.Optimize = 1;
`, idRange(airGeoBase+1, 6), size, int(order))
	return err
}

// writeBox emits the eight corner points, twelve edges, six plane faces and the outer
// surface loop of the padded air box (entity ids offset by airGeoBase). cl is the corner
// characteristic length (the air coarsens away from the fine part interface).
func writeBox(w io.Writer, b box, cl float64) error {
	for i, p := range boxCorners(b) {
		if _, err := fmt.Fprintf(w, "Point(%d) = {%.10g, %.10g, %.10g, %g};\n", airGeoBase+1+i, p[0], p[1], p[2], cl); err != nil {
			return err
		}
	}
	edgeID := writeBoxEdges(w)
	for f, cyc := range boxFaceCycles {
		loop := make([]int, 4)
		for i := 0; i < 4; i++ {
			loop[i] = edgeID(cyc[i], cyc[(i+1)%4])
		}
		if _, err := fmt.Fprintf(w, "Line Loop(%d) = {%d, %d, %d, %d};\nPlane Surface(%d) = {%d};\n",
			airGeoBase+1+f, loop[0], loop[1], loop[2], loop[3], airGeoBase+1+f, airGeoBase+1+f); err != nil {
			return err
		}
	}
	_, err := fmt.Fprintf(w, "Surface Loop(2) = {%s};\n", idRange(airGeoBase+1, 6))
	return err
}

// writeBoxEdges emits the twelve unique box edges and returns a lookup that maps a directed
// corner pair to the signed gmsh line id (negative when traversed against the stored edge),
// so each face loop is a correctly-oriented closed cycle.
func writeBoxEdges(w io.Writer) func(a, b int) int {
	edges := [12][2]int{
		{0, 1}, {1, 2}, {2, 3}, {3, 0}, {4, 5}, {5, 6}, {6, 7}, {7, 4}, {0, 4}, {1, 5}, {2, 6}, {3, 7},
	}
	index := map[[2]int]int{}
	for i, e := range edges {
		id := airGeoBase + 100 + i
		fmt.Fprintf(w, "Line(%d) = {%d, %d};\n", id, airGeoBase+1+e[0], airGeoBase+1+e[1])
		index[[2]int{e[0], e[1]}] = id
	}
	return func(a, b int) int {
		if id, ok := index[[2]int{a, b}]; ok {
			return id
		}
		return -index[[2]int{b, a}]
	}
}

// boxCorners returns the eight corners of b in the standard order the face cycles index.
func boxCorners(b box) [8][3]float64 {
	m, M := b.min, b.max
	return [8][3]float64{
		{m[0], m[1], m[2]}, {M[0], m[1], m[2]}, {M[0], M[1], m[2]}, {m[0], M[1], m[2]},
		{m[0], m[1], M[2]}, {M[0], m[1], M[2]}, {M[0], M[1], M[2]}, {m[0], M[1], M[2]},
	}
}

// boxCharLength returns the box-corner element length: coarse (a twelfth of the box side)
// but never finer than the part size, so the air mesh grows away from the interface without
// exploding the element count.
func boxCharLength(b box, size float64) float64 {
	side := b.max[0] - b.min[0]
	if c := side / 12; c > size {
		return c
	}
	return size
}

// idRange formats a comma-separated inclusive id range starting at start with n entries
// (e.g. "1001, 1002, ... 1006").
func idRange(start, n int) string {
	out := ""
	for i := 0; i < n; i++ {
		if i > 0 {
			out += ", "
		}
		out += fmt.Sprintf("%d", start+i)
	}
	return out
}
