// SPDX-License-Identifier: GPL-2.0-only

package getdp

import "math"

// icosphere returns a geodesic sphere (a subdivided icosahedron) of the given radius centred
// at c, with every vertex lying exactly on the sphere. subdivisions=0 is the bare 20-face
// icosahedron; each further level splits every triangle into four (20·4ⁿ faces). It is the
// structured inner/outer boundary of the infinite shell (#25): vertices exactly on the sphere
// make the VolSphShell transform's f=1 identity hold on the shell's inner face.
//
//	inner := icosphere(center, Rint, 3) // a watertight Rint sphere to reclassify as an STL
func icosphere(c [3]float64, radius float64, subdivisions int) *SurfaceMesh {
	m := icosahedron()
	for i := 0; i < subdivisions; i++ {
		m = subdivideTriangles(m)
	}
	// The seed icosahedron is centred at the origin: project each vertex onto the sphere of
	// the given radius about the origin (uniform distribution), THEN translate to c. Projecting
	// directly about c would skew the distribution when c is far from the origin.
	for i, v := range m.Verts {
		p := projectRadius(v, [3]float64{}, radius)
		m.Verts[i] = [3]float64{p[0] + c[0], p[1] + c[1], p[2] + c[2]}
	}
	return m
}

// icosahedron is the 12-vertex, 20-face regular icosahedron (vertices on the golden-ratio
// rectangles), wound so every face normal points outward — the seed for subdivision.
func icosahedron() *SurfaceMesh {
	t := (1 + math.Sqrt(5)) / 2
	return &SurfaceMesh{
		Verts: [][3]float64{
			{-1, t, 0}, {1, t, 0}, {-1, -t, 0}, {1, -t, 0},
			{0, -1, t}, {0, 1, t}, {0, -1, -t}, {0, 1, -t},
			{t, 0, -1}, {t, 0, 1}, {-t, 0, -1}, {-t, 0, 1},
		},
		Tris: [][3]int{
			{0, 11, 5}, {0, 5, 1}, {0, 1, 7}, {0, 7, 10}, {0, 10, 11},
			{1, 5, 9}, {5, 11, 4}, {11, 10, 2}, {10, 7, 6}, {7, 1, 8},
			{3, 9, 4}, {3, 4, 2}, {3, 2, 6}, {3, 6, 8}, {3, 8, 9},
			{4, 9, 5}, {2, 4, 11}, {6, 2, 10}, {8, 6, 7}, {9, 8, 1},
		},
	}
}

// subdivideTriangles splits every triangle into four by its edge midpoints, sharing each
// midpoint between the two triangles on that edge (so the result stays a closed manifold).
// Winding is preserved: the three corner triangles and the central one all keep the parent's
// orientation.
func subdivideTriangles(m *SurfaceMesh) *SurfaceMesh {
	out := &SurfaceMesh{Verts: append([][3]float64(nil), m.Verts...)}
	mid := map[[2]int]int{}
	for _, t := range m.Tris {
		a := edgeMidpoint(out, mid, t[0], t[1])
		b := edgeMidpoint(out, mid, t[1], t[2])
		c := edgeMidpoint(out, mid, t[2], t[0])
		out.Tris = append(out.Tris,
			[3]int{t[0], a, c}, [3]int{t[1], b, a}, [3]int{t[2], c, b}, [3]int{a, b, c})
	}
	return out
}

// edgeMidpoint returns the index of the midpoint of edge (i,j), appending it on first sight
// and caching it under the order-independent edge key so both adjacent triangles share it.
func edgeMidpoint(m *SurfaceMesh, cache map[[2]int]int, i, j int) int {
	key := edgeKey(i, j)
	if idx, ok := cache[key]; ok {
		return idx
	}
	p, q := m.Verts[i], m.Verts[j]
	idx := len(m.Verts)
	m.Verts = append(m.Verts, [3]float64{(p[0] + q[0]) / 2, (p[1] + q[1]) / 2, (p[2] + q[2]) / 2})
	cache[key] = idx
	return idx
}

// projectRadius returns the point on the sphere of the given radius about c that lies along
// the ray from c through p. A point exactly at the centre maps to c+radius·x̂ (never NaN).
// Recomputing the projection (rather than trusting a scale factor) is why shell inner/outer
// nodes land at exactly Rint/Rext, satisfying the VolSphShell radius guard.
func projectRadius(p, c [3]float64, radius float64) [3]float64 {
	d := [3]float64{p[0] - c[0], p[1] - c[1], p[2] - c[2]}
	mag := math.Sqrt(d[0]*d[0] + d[1]*d[1] + d[2]*d[2])
	if mag == 0 {
		return [3]float64{c[0] + radius, c[1], c[2]}
	}
	s := radius / mag
	return [3]float64{c[0] + d[0]*s, c[1] + d[1]*s, c[2] + d[2]*s}
}
