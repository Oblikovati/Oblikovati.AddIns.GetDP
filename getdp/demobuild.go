// SPDX-License-Identifier: GPL-2.0-only

package getdp

import (
	"encoding/json"
	"fmt"

	"oblikovati.org/api/client"
	"oblikovati.org/api/wire"
)

// clientAuthor implements demos.Author over the host api/client: it authors the demo's
// parametric geometry (parameters, sketches, extrudes, patterns) and resolves face keys by
// point, so a demo builder written against the seam replays as real host geometry. It is
// the single place the abstract demo program meets the host modelling API (issue #21).
type clientAuthor struct {
	api      *client.Client
	extrudes int // names the fin/base features deterministically for pattern sourcing
}

// extrudeArgs is the "extrude" feature-kind payload: profile 0 of a sketch, a unit-bearing
// (parameter-expression) distance, and the boolean against existing bodies.
type extrudeArgs struct {
	SketchIndex  int    `json:"sketchIndex"`
	ProfileIndex int    `json:"profileIndex"`
	Distance     string `json:"distance"`
	Operation    string `json:"operation"`
}

// extrudeReply is the host's extrude result — the produced body count and health flag.
type extrudeReply struct {
	Bodies  int  `json:"bodies"`
	Healthy bool `json:"healthy"`
}

// Parameter publishes a host user-parameter (literal or formula).
func (c *clientAuthor) Parameter(name, expr string) error {
	_, err := c.api.Parameters().Add(wire.ParameterSetArgs{Name: name, Expression: expr})
	return err
}

// Sketch creates a sketch on a base plane and returns its index.
func (c *clientAuthor) Sketch(plane string) (int, error) {
	r, err := c.api.Sketch().Create(wire.CreateSketchArgs{Plane: plane})
	return r.SketchIndex, err
}

// CornerRectangle lays a fully-constrained (DOF=0) axis-aligned rectangle: four lines welded
// at the corners, horizontal/vertical constraints, the lower-left corner anchored, and
// driving width/height dimensions referencing the width/height parameter expressions — so
// the profile recomputes when those parameters change. Corners are placed at the evaluated
// expression coordinates so the solver starts on the solution.
func (c *clientAuthor) CornerRectangle(sk int, x, y, w, h string) error {
	s := c.api.Sketch()
	xr, yt := sumExpr(x, w), sumExpr(y, h)
	bottom, err := s.AddLineExpr(sk, []string{x, y}, []string{xr, y}, false)
	if err != nil {
		return err
	}
	right, err := s.AddLineExpr(sk, []string{xr, y}, []string{xr, yt}, false)
	if err != nil {
		return err
	}
	top, err := s.AddLineExpr(sk, []string{xr, yt}, []string{x, yt}, false)
	if err != nil {
		return err
	}
	left, err := s.AddLineExpr(sk, []string{x, yt}, []string{x, y}, false)
	if err != nil {
		return err
	}
	return c.constrainRectangle(sk, rectLines{bottom, right, top, left}, w, h)
}

// rectLines holds the four edges of a laid-out rectangle loop (bottom→right→top→left), each
// carrying its two ordered endpoint ids.
type rectLines struct{ bottom, right, top, left wire.AddSketchEntityResult }

// constrainRectangle welds the loop, axis-aligns it, anchors the lower-left corner, and adds
// the driving width/height dimensions, leaving DOF=0.
func (c *clientAuthor) constrainRectangle(sk int, r rectLines, wExpr, hExpr string) error {
	g, d := c.api.Sketch().Constrain(sk), c.api.Sketch().Dimension(sk)
	bl, br := r.bottom.PointIDs[0], r.bottom.PointIDs[1]
	tr, tl := r.right.PointIDs[1], r.top.PointIDs[1]
	welds := [][2]uint64{{br, r.right.PointIDs[0]}, {tr, r.top.PointIDs[0]}, {tl, r.left.PointIDs[0]}, {r.left.PointIDs[1], bl}}
	for _, wd := range welds {
		if _, err := g.Coincident(wd[0], wd[1]); err != nil {
			return err
		}
	}
	if err := axisAlign(g, r); err != nil {
		return err
	}
	if _, err := g.Fix(bl); err != nil {
		return err
	}
	if _, err := d.Distance(bl, br, wExpr); err != nil {
		return fmt.Errorf("width dimension %q: %w", wExpr, err)
	}
	_, err := d.Distance(bl, tl, hExpr)
	return err
}

// axisAlign makes the bottom/top edges horizontal and the left/right edges vertical.
func axisAlign(g client.Constrain, r rectLines) error {
	pairs := []struct {
		horizontal bool
		a, b       uint64
	}{
		{true, r.bottom.PointIDs[0], r.bottom.PointIDs[1]},
		{true, r.top.PointIDs[0], r.top.PointIDs[1]},
		{false, r.left.PointIDs[0], r.left.PointIDs[1]},
		{false, r.right.PointIDs[0], r.right.PointIDs[1]},
	}
	for _, p := range pairs {
		var err error
		if p.horizontal {
			_, err = g.Horizontal(p.a, p.b)
		} else {
			_, err = g.Vertical(p.a, p.b)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// Extrude extrudes profile 0 of a sketch and renames the new feature to a deterministic name
// (so a following pattern can source it by name), returning that name.
func (c *clientAuthor) Extrude(sk int, distanceExpr, operation string) (string, error) {
	args, err := json.Marshal(extrudeArgs{SketchIndex: sk, ProfileIndex: 0, Distance: distanceExpr, Operation: operation})
	if err != nil {
		return "", err
	}
	raw, err := c.api.Features().Add(wire.AddFeatureArgs{Kind: "extrude", Args: args})
	if err != nil {
		return "", err
	}
	var reply extrudeReply
	if err := json.Unmarshal(raw, &reply); err != nil {
		return "", fmt.Errorf("decode extrude reply %q: %w", string(raw), err)
	}
	if !reply.Healthy || reply.Bodies == 0 {
		return "", fmt.Errorf("extrude produced no healthy body (reply %q)", string(raw))
	}
	c.extrudes++
	name := fmt.Sprintf("GetDPExtrude%d", c.extrudes)
	c.renameLastFeature(name)
	return name, nil
}

// renameLastFeature renames the most recently added feature (best-effort — a rename failure
// must not abort the demo build).
func (c *clientAuthor) renameLastFeature(name string) {
	tree, err := c.api.Model().Tree()
	if err != nil || len(tree.Features) == 0 {
		return
	}
	last := tree.Features[len(tree.Features)-1]
	_, _ = c.api.Features().Rename(last.ID, name)
}

// PatternX linearly replicates a feature along +X. count seeds the host schema; countExpr is
// the driving parameter so the instance count tracks it.
func (c *clientAuthor) PatternX(feature string, count int, countExpr string, stepXcm float64) error {
	_, err := c.api.Features().PatternRectangular(wire.RectangularPatternFeatureArgs{
		SourceFeatures: []string{feature}, CountX: count, CountXExpr: countExpr, StepX: []float64{stepXcm, 0, 0},
	})
	return err
}

// FaceKeyAt returns the persistent reference key of the solid-body face at a model-unit
// point, scanning every solid body so a demo need not know which body carries the face.
func (c *clientAuthor) FaceKeyAt(point [3]float64) (string, error) {
	list, err := c.api.Body().List()
	if err != nil {
		return "", err
	}
	pt := []float64{point[0], point[1], point[2]}
	for _, b := range list.Bodies {
		if !b.Solid {
			continue
		}
		res, err := c.api.Body().LocateUsingPoint(b.Index, pt, "face", faceProbeToleranceCm)
		if err != nil {
			return "", err
		}
		if res.Found && res.Entity.Key != "" {
			return res.Entity.Key, nil
		}
	}
	return "", fmt.Errorf("no body face found at model-unit point %v", point)
}

// faceProbeToleranceCm bounds how far a probe point may sit from the face it names (1 mm) —
// generous against float noise, tight enough not to snap to a neighbouring face.
const faceProbeToleranceCm = 0.1

// sumExpr composes two host expressions additively, collapsing a "0" anchor so the common
// origin-anchored case stays clean ("bar_width" rather than "(0) + (bar_width)").
func sumExpr(a, b string) string {
	if a == "0" || a == "" {
		return b
	}
	return "(" + a + ") + (" + b + ")"
}
