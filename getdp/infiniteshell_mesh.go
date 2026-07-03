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

// ShellSpec configures one infinite-shell air run (#25). InnerFactor sets the inner sphere
// radius as a multiple of the part's enclosing radius; Rint/Rext override it with explicit
// radii (model units). Zero fields take defaults (InnerFactor 1.5, Rext = 2·Rint).
type ShellSpec struct {
	InnerFactor float64
	Rint, Rext  float64
}

// shellGeometry is the sphere the VolSphShell transform is built on (model units): the centre
// the transform measures radius from, plus the shell's inner and outer radii.
type shellGeometry struct {
	Center     [3]float64
	RInt, RExt float64
}

// MeshWithInfiniteShell meshes the part plus a near-air ball reaching an inner sphere (Rint),
// then extrudes that sphere into a single STRUCTURED radial shell to Rext whose tets carry the
// VolSphShell transform — so a finite mesh represents open space (#25). Unlike the padded box,
// the far boundary is mapped to infinity, which is what open-boundary problems (isolated
// conductors) need. Single part body only, like the box path. Returns the sphere geometry so
// the deck can build the matching Jacobian.
func (g GmshMesher) MeshWithInfiniteShell(ctx context.Context, surface *SurfaceMesh, spec ShellSpec, opts MeshOptions, workdir string) (*TetMesh, shellGeometry, error) {
	if len(surface.Tris) == 0 {
		return nil, shellGeometry{}, fmt.Errorf("infinite-shell mesh: empty part surface")
	}
	geom, err := shellRadii(surface, spec)
	if err != nil {
		return nil, shellGeometry{}, err
	}
	stlPath := filepath.Join(workdir, "part.stl")
	if err := writeFile(stlPath, func(f *os.File) error { return surface.writeSTL(f) }); err != nil {
		return nil, shellGeometry{}, err
	}
	size := meshSize(opts.Size, surface)
	inner := icosphere(geom.Center, geom.RInt, subdivisionForSize(geom.RInt, size))
	geoPath := filepath.Join(workdir, "shell.geo")
	if err := writeFile(geoPath, func(f *os.File) error { return writeShellAirGeo(f, "part.stl", inner, size, opts.Order) }); err != nil {
		return nil, shellGeometry{}, err
	}
	mesh, err := g.runShellMesh(ctx, geoPath, workdir, geom)
	return mesh, geom, err
}

// runShellMesh runs gmsh on the near-air geo, reads the mesh, assigns the near-air/part bodies,
// and appends the structured infinite shell.
func (g GmshMesher) runShellMesh(ctx context.Context, geoPath, workdir string, geom shellGeometry) (*TetMesh, error) {
	mshPath := filepath.Join(workdir, "shell.msh")
	if err := runGmsh(ctx, g.bin, geoPath, mshPath); err != nil {
		return nil, fmt.Errorf("infinite-shell near-air mesh failed — the part shell could not be embedded "+
			"in the inner sphere (a dirty or open shell cannot form the ball hole): %w", err)
	}
	mesh, err := readMSHFile(mshPath)
	if err != nil {
		return nil, err
	}
	assignAirBodies(mesh) // near-air tets → airBodyIndex, part tets → body 0
	if err := appendInfiniteShell(mesh, geom.Center, geom.RInt, geom.RExt); err != nil {
		return nil, err
	}
	return mesh, nil
}

// shellRadii resolves the shell sphere from the spec: explicit radii if given (the inner one
// must enclose the part), else 1.5× the part enclosing radius with Rext = 2·Rint.
func shellRadii(surface *SurfaceMesh, spec ShellSpec) (shellGeometry, error) {
	center, rBound := enclosingSphere(surface)
	rInt := spec.Rint
	if rInt <= 0 {
		f := spec.InnerFactor
		if f <= 0 {
			f = 1.5
		}
		rInt = f * rBound
	} else if rInt <= rBound {
		return shellGeometry{}, fmt.Errorf("infinite-shell inner radius %g must exceed the part enclosing radius %g", rInt, rBound)
	}
	rExt := spec.Rext
	if rExt <= rInt {
		rExt = 2 * rInt
	}
	return shellGeometry{Center: center, RInt: rInt, RExt: rExt}, nil
}

// enclosingSphere returns the part's bbox centre and the AABB half-diagonal — a sphere that
// always contains every vertex (the far corner sits at exactly the half-diagonal). The centre
// is stable and deterministic, which the VolSphShell transform relies on (mesh and Jacobian
// must share one centre).
func enclosingSphere(surface *SurfaceMesh) ([3]float64, float64) {
	lo, hi := surfaceBBox(surface)
	c := [3]float64{(lo[0] + hi[0]) / 2, (lo[1] + hi[1]) / 2, (lo[2] + hi[2]) / 2}
	return c, 0.5 * boxDiagonal(lo, hi)
}

// subdivisionForSize picks the inner icosphere subdivision level so its facet edge tracks the
// mesh size (base icosahedron edge ≈ 1.0515·radius, halving per level), clamped to [1,4].
func subdivisionForSize(rInt, size float64) int {
	if size <= 0 {
		return 2
	}
	n := int(math.Round(math.Log2(1.0515 * rInt / size)))
	if n < 1 {
		return 1
	}
	if n > 4 {
		return 4
	}
	return n
}

// writeShellAirGeo emits the gmsh script that meshes the part AND the near-air ball between the
// part shell and an inline-triangulated inner sphere in one conformal run. The part STL is
// reclassified (Physical "Shell" 4, for electrode binding), the sphere is built explicitly
// (like writeBox) as Physical "InnerSphere" (shellInnerTag), and the air is the ball MINUS the
// part hole — Volume(2) = {2, 1}. The outer far-field boundary is added later by
// appendInfiniteShell, not here.
func writeShellAirGeo(w io.Writer, stlName string, sph *SurfaceMesh, size float64, order ElementOrder) error {
	if order == 0 {
		order = FirstOrderTet
	}
	if _, err := fmt.Fprintf(w, `Merge "%s";
ClassifySurfaces{40*Pi/180, 1, 1, Pi};
CreateGeometry;
Surface Loop(1) = Surface{:};
Volume(1) = {1};
Physical Surface("Shell", 4) = Surface{:};
`, stlName); err != nil {
		return err
	}
	surfList, err := writeSphereGeo(w, sph, 2, size)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, `Volume(2) = {2, 1};
Physical Volume("Part", %d) = {1};
Physical Volume("Air", %d) = {2};
Physical Surface("InnerSphere", %d) = {%s};
Mesh.MeshSizeMax = %g;
Mesh.MeshSizeMin = 0;
Mesh.ElementOrder = %d;
Mesh.Algorithm3D = 1;
Mesh.Optimize = 1;
`, partVolumeTag, airVolumeTag, shellInnerTag, surfList, size, int(order))
	return err
}

// sphereGeoBase offsets the inline sphere's gmsh entity ids clear of the reclassified part
// surfaces (which take low ids from CreateGeometry).
const sphereGeoBase = 100000

// writeSphereGeo emits the inline triangulated sphere (points, edges, plane-surface triangles,
// one Surface Loop with id loopTag) into the .geo, mirroring writeBox. It returns the
// comma-separated surface-id list for the inner-sphere physical group. cl is the vertex
// characteristic length (the sphere meshes at the part's size).
func writeSphereGeo(w io.Writer, sph *SurfaceMesh, loopTag int, cl float64) (string, error) {
	for i, v := range sph.Verts {
		if _, err := fmt.Fprintf(w, "Point(%d) = {%.12g, %.12g, %.12g, %g};\n", sphereGeoBase+1+i, v[0], v[1], v[2], cl); err != nil {
			return "", err
		}
	}
	edgeID := writeSphereEdges(w, sph)
	sBase := sphereGeoBase + len(sph.Verts) + 3*len(sph.Tris)
	for t, tri := range sph.Tris {
		sid := sBase + 1 + t
		if _, err := fmt.Fprintf(w, "Line Loop(%d) = {%d, %d, %d};\nPlane Surface(%d) = {%d};\n",
			sid, edgeID(tri[0], tri[1]), edgeID(tri[1], tri[2]), edgeID(tri[2], tri[0]), sid, sid); err != nil {
			return "", err
		}
	}
	surfList := idRange(sBase+1, len(sph.Tris))
	_, err := fmt.Fprintf(w, "Surface Loop(%d) = {%s};\n", loopTag, surfList)
	return surfList, err
}

// writeSphereEdges emits the sphere's unique edges (ids after the points) and returns a signed
// lookup: +id when the query matches the stored direction, −id when reversed, so every triangle
// loop is a correctly-oriented closed cycle.
func writeSphereEdges(w io.Writer, sph *SurfaceMesh) func(a, b int) int {
	index := map[[2]int]int{}
	next := sphereGeoBase + len(sph.Verts) + 1
	for _, tri := range sph.Tris {
		for _, e := range [3][2]int{{tri[0], tri[1]}, {tri[1], tri[2]}, {tri[2], tri[0]}} {
			if _, ok := index[e]; ok {
				continue
			}
			if _, ok := index[[2]int{e[1], e[0]}]; ok {
				continue
			}
			fmt.Fprintf(w, "Line(%d) = {%d, %d};\n", next, sphereGeoBase+1+e[0], sphereGeoBase+1+e[1])
			index[e] = next
			next++
		}
	}
	return func(a, b int) int {
		if id, ok := index[[2]int{a, b}]; ok {
			return id
		}
		return -index[[2]int{b, a}]
	}
}
