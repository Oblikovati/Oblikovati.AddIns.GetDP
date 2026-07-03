// SPDX-License-Identifier: GPL-2.0-only

package getdp

import (
	"math"

	"oblikovati.org/api/types"
	"oblikovati.org/api/wire"
	"oblikovati.org/getdp/getdp/femmodel"
)

// glyphClientPrefix namespaces the per-constraint glyph graphics groups.
const glyphClientPrefix = "getdp.glyph."

// Glyph colors per constraint kind (RGBA) — the language of spec §4.4: voltage red,
// ground cyan, temperature orange, flux magenta arrows, convection teal.
var glyphColors = map[femmodel.ConstraintKind][4]float32{
	femmodel.KindVoltage:     {0.9, 0.15, 0.15, 1},
	femmodel.KindCurrent:     {0.95, 0.55, 0.1, 1},
	femmodel.KindTemperature: {0.95, 0.45, 0.1, 1},
	femmodel.KindHeatFlux:    {0.85, 0.15, 0.75, 1},
	femmodel.KindConvection:  {0.1, 0.7, 0.7, 1},
}

// groundColor overrides the voltage color for a 0 V electrode (the ⏚ cyan of the
// glyph language).
var groundColor = [4]float32{0.1, 0.75, 0.85, 1}

// refreshGlyphs redraws the ACTIVE study's boundary-condition markers: one solid glyph
// group per constraint, anchored on each bound face's centroid. Face geometry comes
// from the host tessellation (no mesh required, so glyphs work before the first
// solve). Best-effort: glyphs never block editing.
func (e *Engine) refreshGlyphs() {
	e.mu.Lock()
	constraints := append([]femmodel.ConstraintObject(nil), e.analysis.Active().Constraints()...)
	e.mu.Unlock()
	solids, err := e.solidBodies()
	if err != nil {
		return // nothing meshable open; stale glyphs are cleared below regardless
	}
	e.clearGlyphs()
	for _, c := range constraints {
		e.drawConstraintGlyphs(c, solids)
	}
	e.drawAirBoxGlyph(solids)
}

// airGlyphClientID names the single air-box wireframe group of the active study.
const airGlyphClientID = glyphClientPrefix + "airbox"

// drawAirBoxGlyph frames the padded air domain the field solves in with a translucent grey
// dashed wireframe box (spec §4.4). It shows only for an EM study whose air is an automatic
// box, and tracks the padding because it is rebuilt from the current AirRegion every refresh
// (clearGlyphs already removed the previous box). solids is the active study's bodies.
func (e *Engine) drawAirBoxGlyph(solids []wire.BodyInfo) {
	e.mu.Lock()
	active := e.analysis.Active()
	physics, air := active.Solver.Physics, active.Solver.Air
	e.mu.Unlock()
	if !wantsAirBoxGlyph(physics, air) || len(solids) != 1 {
		return // auto air is single-body; None/Manual and confined physics frame nothing
	}
	surface, err := e.pullSurface(solids[0].Index)
	if err != nil {
		return
	}
	lo, hi := surfaceBBox(surface)
	e.pushAirBoxWireframe(airBox(lo, hi, air.PaddingFactor))
}

// wantsAirBoxGlyph reports whether the active study should show the air-box wireframe: an EM
// physics whose air region is an automatic padded box.
func wantsAirBoxGlyph(physics femmodel.PhysicsKind, air femmodel.AirRegion) bool {
	return femmodel.NeedsAir(physics) && air.Mode == femmodel.AirAutomaticBox
}

// airBoxWireframe returns the eight corner coordinates and the twelve edges (index pairs) of
// box b for a GraphicsLines primitive.
func airBoxWireframe(b box) (coords []float64, edges []int) {
	for _, c := range boxCorners(b) {
		coords = append(coords, c[0], c[1], c[2])
	}
	edges = []int{0, 1, 1, 2, 2, 3, 3, 0, 4, 5, 5, 6, 6, 7, 7, 4, 0, 4, 1, 5, 2, 6, 3, 7}
	return coords, edges
}

// pushAirBoxWireframe sends the dashed translucent grey air-box lines to the viewport.
func (e *Engine) pushAirBoxWireframe(b box) {
	coords, idx := airBoxWireframe(b)
	prim := wire.GraphicsPrimitive{
		Kind:        string(types.GraphicsLines),
		Coordinates: coords,
		Indices:     idx,
		Color:       []float32{0.6, 0.6, 0.6, 1},
		LineType:    string(types.GraphicsLineDashed),
		LineWeight:  1.5,
		Opacity:     0.5,
	}
	_, _ = e.api.Graphics().Set(wire.SetClientGraphicsArgs{
		ClientId: airGlyphClientID,
		Lane:     string(types.GraphicsLanePersistent),
		Nodes:    []wire.GraphicsNode{{Primitives: []wire.GraphicsPrimitive{prim}}},
	})
}

// clearGlyphs removes every glyph group we previously pushed.
func (e *Engine) clearGlyphs() {
	list, err := e.api.Graphics().List()
	if err != nil {
		return
	}
	for _, g := range list.Groups {
		if len(g.ClientId) > len(glyphClientPrefix) && g.ClientId[:len(glyphClientPrefix)] == glyphClientPrefix {
			_ = e.api.Graphics().Delete(g.ClientId)
		}
	}
}

// drawConstraintGlyphs pushes one constraint's markers.
func (e *Engine) drawConstraintGlyphs(c femmodel.ConstraintObject, solids []wire.BodyInfo) {
	mesh := &glyphMesh{}
	for _, key := range c.Faces {
		face, err := e.pullFaceOnAnyBody(key, solids)
		if err != nil {
			continue // an unbound face shows as [!] in the tree; no glyph
		}
		centroid, normal := surfaceCentroidNormal(face)
		addKindGlyph(mesh, c.Kind, centroid, normal, glyphSize(face))
	}
	if len(mesh.idx) == 0 {
		return
	}
	color := glyphColors[c.Kind]
	if c.Kind == femmodel.KindVoltage && c.Value == 0 {
		color = groundColor
	}
	e.pushGlyphMesh(glyphClientPrefix+c.ID, mesh, color)
}

// pushGlyphMesh sends one lit on-top mesh group to the viewport.
func (e *Engine) pushGlyphMesh(clientID string, m *glyphMesh, color [4]float32) {
	prim := wire.GraphicsPrimitive{
		Kind:          string(types.GraphicsTriangles),
		Coordinates:   m.coords,
		Normals:       m.normals,
		Indices:       m.idx,
		Color:         color[:],
		OnTop:         true,
		DepthPriority: 10,
	}
	_, _ = e.api.Graphics().Set(wire.SetClientGraphicsArgs{
		ClientId: clientID,
		Lane:     string(types.GraphicsLanePersistent),
		Nodes:    []wire.GraphicsNode{{Primitives: []wire.GraphicsPrimitive{prim}}},
	})
}

// addKindGlyph appends the kind's marker: Dirichlet kinds get an anchored cube,
// flux-type kinds a solid arrow INTO the face (the energy/current flows inward).
func addKindGlyph(m *glyphMesh, kind femmodel.ConstraintKind, centroid, normal [3]float64, size float64) {
	switch kind {
	case femmodel.KindVoltage, femmodel.KindTemperature:
		m.cube(centroid, size*0.5)
	case femmodel.KindConvection:
		// Waves read as three short parallel arrows leaving the face.
		off := scale(anyPerpendicular(normal), size*0.6)
		for _, k := range []float64{-1, 0, 1} {
			tip := add(add(centroid, scale(normal, size)), scale(off, k))
			m.arrow(tip, normal, size*0.8)
		}
	default: // current, heat flux: one arrow into the face
		m.arrow(centroid, scale(normal, -1), size*1.4)
	}
}

// glyphSize scales markers from the face extent (~14% of its bbox diagonal).
func glyphSize(face *SurfaceMesh) float64 {
	if len(face.Verts) == 0 {
		return 0.5
	}
	lo, hi := face.Verts[0], face.Verts[0]
	for _, v := range face.Verts {
		for k := 0; k < 3; k++ {
			lo[k], hi[k] = math.Min(lo[k], v[k]), math.Max(hi[k], v[k])
		}
	}
	diag := distance(lo, hi)
	if diag == 0 {
		return 0.5
	}
	return diag * 0.14
}

// glyphSegments is the radial tessellation of the round glyph parts (shafts, heads).
const glyphSegments = 12

// glyphMesh accumulates a lit triangle mesh (coordinates + per-vertex normals +
// indices) for the 3D constraint glyphs — solid arrows and cubes that read as real
// geometry rather than flat lines (same construction as the reference add-in).
type glyphMesh struct {
	coords  []float64
	normals []float64
	idx     []int
}

// tri appends one triangle with a flat face normal.
func (g *glyphMesh) tri(a, b, c [3]float64) {
	n := triNormal(a, b, c)
	base := len(g.coords) / 3
	for _, p := range [3][3]float64{a, b, c} {
		g.coords = append(g.coords, p[0], p[1], p[2])
		g.normals = append(g.normals, n[0], n[1], n[2])
	}
	g.idx = append(g.idx, base, base+1, base+2)
}

// arrow appends a solid 3D arrow whose head sits at tip and which points along dir.
func (g *glyphMesh) arrow(tip, dir [3]float64, length float64) {
	d := normalize(dir)
	headLen := length * 0.4
	shaftR, headR := length*0.05, length*0.13
	neck := add(tip, scale(d, -headLen))
	tail := add(tip, scale(d, -length))
	g.cylinder(tail, neck, shaftR)
	g.cone(neck, tip, headR)
}

// cube appends an axis-aligned solid cube centred at c with half-extent h.
func (g *glyphMesh) cube(c [3]float64, h float64) {
	v := func(sx, sy, sz float64) [3]float64 { return [3]float64{c[0] + sx*h, c[1] + sy*h, c[2] + sz*h} }
	quads := [6][4][3]float64{
		{v(-1, -1, -1), v(-1, 1, -1), v(1, 1, -1), v(1, -1, -1)},
		{v(-1, -1, 1), v(1, -1, 1), v(1, 1, 1), v(-1, 1, 1)},
		{v(-1, -1, -1), v(1, -1, -1), v(1, -1, 1), v(-1, -1, 1)},
		{v(1, -1, -1), v(1, 1, -1), v(1, 1, 1), v(1, -1, 1)},
		{v(1, 1, -1), v(-1, 1, -1), v(-1, 1, 1), v(1, 1, 1)},
		{v(-1, 1, -1), v(-1, -1, -1), v(-1, -1, 1), v(-1, 1, 1)},
	}
	for _, q := range quads {
		g.tri(q[0], q[1], q[2])
		g.tri(q[0], q[2], q[3])
	}
}

// cylinder appends a closed cylinder between p0 and p1 with the given radius.
func (g *glyphMesh) cylinder(p0, p1 [3]float64, r float64) {
	axis := normalize(sub(p1, p0))
	u, v := basis(axis)
	for s := 0; s < glyphSegments; s++ {
		a0, a1 := ringPoint(p0, u, v, r, s), ringPoint(p0, u, v, r, s+1)
		b0, b1 := ringPoint(p1, u, v, r, s), ringPoint(p1, u, v, r, s+1)
		g.tri(a0, b0, b1)
		g.tri(a0, b1, a1)
	}
}

// cone appends a cone with its circular base at base and its apex at apex.
func (g *glyphMesh) cone(base, apex [3]float64, r float64) {
	axis := normalize(sub(apex, base))
	u, v := basis(axis)
	for s := 0; s < glyphSegments; s++ {
		p0, p1 := ringPoint(base, u, v, r, s), ringPoint(base, u, v, r, s+1)
		g.tri(p0, p1, apex)
		g.tri(p1, p0, base)
	}
}

// ringPoint returns the s-th point of a glyphSegments-gon around centre in the (u,v)
// plane.
func ringPoint(centre, u, v [3]float64, r float64, s int) [3]float64 {
	ang := 2 * math.Pi * float64(s) / glyphSegments
	return add(centre, add(scale(u, r*math.Cos(ang)), scale(v, r*math.Sin(ang))))
}

// basis returns two unit vectors spanning the plane orthogonal to axis.
func basis(axis [3]float64) ([3]float64, [3]float64) {
	u := normalize(cross(axis, anyPerpendicular(axis)))
	return u, normalize(cross(axis, u))
}

// anyPerpendicular returns a unit vector orthogonal to d.
func anyPerpendicular(d [3]float64) [3]float64 {
	axis := [3]float64{1, 0, 0}
	if math.Abs(d[0]) > 0.9 {
		axis = [3]float64{0, 1, 0}
	}
	return normalize(cross(d, axis))
}

func add(a, b [3]float64) [3]float64           { return [3]float64{a[0] + b[0], a[1] + b[1], a[2] + b[2]} }
func sub(a, b [3]float64) [3]float64           { return [3]float64{a[0] - b[0], a[1] - b[1], a[2] - b[2]} }
func scale(a [3]float64, s float64) [3]float64 { return [3]float64{a[0] * s, a[1] * s, a[2] * s} }

func cross(a, b [3]float64) [3]float64 {
	return [3]float64{a[1]*b[2] - a[2]*b[1], a[2]*b[0] - a[0]*b[2], a[0]*b[1] - a[1]*b[0]}
}

// glyphKindLabel is used by tests to assert the kind→shape mapping stays exhaustive.
func glyphKindLabel(kind femmodel.ConstraintKind) string {
	switch kind {
	case femmodel.KindVoltage, femmodel.KindTemperature:
		return "cube"
	case femmodel.KindConvection:
		return "waves"
	default:
		return "arrow"
	}
}
