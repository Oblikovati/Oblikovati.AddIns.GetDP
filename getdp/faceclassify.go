// SPDX-License-Identifier: GPL-2.0-only

package getdp

import "math"

// hostTri is one triangle of a selected host face's tessellation, tagged with the face key
// it belongs to. The classifier scans these to decide which selected face a mesh boundary
// facet lies on.
type hostTri struct {
	key        string
	a, b, c    [3]float64
	unitNormal [3]float64
}

// nearestHostFace assigns a mesh boundary facet (its centroid and unit normal) to the
// selected host face it lies on, or reports unbound. It picks the nearest host triangle by
// true point-to-triangle distance — this separates concentric walls (an inner-wall facet is
// genuinely nearer the inner tessellation than the outer one), which a mean-normal match
// cannot do because both walls carry radial normals. Two guards keep it honest: the facet
// must sit within distTol of that triangle (so facets on unselected faces stay unbound), and
// its normal must agree with the triangle's to within cosGate (so an axial rim facet is not
// captured by a radial wall it happens to touch). |dot| is used because the facet and host
// windings may disagree; only the line direction matters.
func nearestHostFace(centroid, normal [3]float64, tris []hostTri, distTol, cosGate float64) (string, bool) {
	best := math.Inf(1)
	bestIdx := -1
	for i := range tris {
		if d := pointTriangleDistance(centroid, tris[i].a, tris[i].b, tris[i].c); d < best {
			best, bestIdx = d, i
		}
	}
	if bestIdx < 0 || best > distTol {
		return "", false
	}
	if math.Abs(dot(normal, tris[bestIdx].unitNormal)) < cosGate {
		return "", false
	}
	return tris[bestIdx].key, true
}

// pointTriangleDistance returns the Euclidean distance from p to triangle abc, exact for all
// three feature regions (face, edge, vertex). It is the Ericson closest-point construction
// (Real-Time Collision Detection §5.1.5) — robust where a nearest-vertex-centroid proxy would
// misrank a point near a long edge of a large host triangle.
func pointTriangleDistance(p, a, b, c [3]float64) float64 {
	q := closestPointOnTriangle(p, a, b, c)
	return distance(p, q)
}

// edgeDots holds the six edge-projection dot products (d1..d6) Ericson's region test shares
// between the vertex, edge, and face cases — computed once, passed by value.
type edgeDots struct {
	d1, d2, d3, d4, d5, d6 float64
}

// closestPointOnTriangle is the barycentric region test of Ericson §5.1.5, split by region so
// each branch group stays within the complexity budget.
func closestPointOnTriangle(p, a, b, c [3]float64) [3]float64 {
	ab, ac := sub(b, a), sub(c, a)
	d := edgeDots{
		d1: dot(ab, sub(p, a)), d2: dot(ac, sub(p, a)),
		d3: dot(ab, sub(p, b)), d4: dot(ac, sub(p, b)),
		d5: dot(ab, sub(p, c)), d6: dot(ac, sub(p, c)),
	}
	if v, ok := closestVertexRegion(a, b, c, d); ok {
		return v
	}
	if e, ok := closestEdgeRegion(a, b, c, ab, ac, d); ok {
		return e
	}
	return closestInterior(a, ab, ac, d)
}

// closestVertexRegion returns the nearest triangle vertex when p projects outside all edges
// into a vertex Voronoi region.
func closestVertexRegion(a, b, c [3]float64, d edgeDots) ([3]float64, bool) {
	if d.d1 <= 0 && d.d2 <= 0 {
		return a, true
	}
	if d.d3 >= 0 && d.d4 <= d.d3 {
		return b, true
	}
	if d.d6 >= 0 && d.d5 <= d.d6 {
		return c, true
	}
	return [3]float64{}, false
}

// closestEdgeRegion returns the nearest point on an edge when p projects into that edge's
// Voronoi region.
func closestEdgeRegion(a, b, c, ab, ac [3]float64, d edgeDots) ([3]float64, bool) {
	if vc := d.d1*d.d4 - d.d3*d.d2; vc <= 0 && d.d1 >= 0 && d.d3 <= 0 {
		return addScaled(a, ab, d.d1/(d.d1-d.d3)), true // edge AB
	}
	if vb := d.d5*d.d2 - d.d1*d.d6; vb <= 0 && d.d2 >= 0 && d.d6 <= 0 {
		return addScaled(a, ac, d.d2/(d.d2-d.d6)), true // edge AC
	}
	if va := d.d3*d.d6 - d.d5*d.d4; va <= 0 && (d.d4-d.d3) >= 0 && (d.d5-d.d6) >= 0 {
		return addScaled(b, sub(c, b), (d.d4-d.d3)/((d.d4-d.d3)+(d.d5-d.d6))), true // edge BC
	}
	return [3]float64{}, false
}

// closestInterior handles the face region: the barycentric projection onto the triangle plane.
func closestInterior(a, ab, ac [3]float64, d edgeDots) [3]float64 {
	vc := d.d1*d.d4 - d.d3*d.d2
	vb := d.d5*d.d2 - d.d1*d.d6
	va := d.d3*d.d6 - d.d5*d.d4
	denom := 1.0 / (va + vb + vc)
	v, w := vb*denom, vc*denom
	return [3]float64{
		a[0] + ab[0]*v + ac[0]*w,
		a[1] + ab[1]*v + ac[1]*w,
		a[2] + ab[2]*v + ac[2]*w,
	}
}

// addScaled returns base + t·dir — the point-on-segment/plane the closest-point test needs
// beyond the shared sub/dot/distance helpers.
func addScaled(base, dir [3]float64, t float64) [3]float64 {
	return [3]float64{base[0] + dir[0]*t, base[1] + dir[1]*t, base[2] + dir[2]*t}
}
