// SPDX-License-Identifier: GPL-2.0-only

// Package demos builds the add-in's bundled tutorial documents: fully-parametric
// (DOF=0) parts wired to a configured GetDP study. Each demo is a host-agnostic
// GEOMETRY PROGRAM expressed against the Author seam plus a returned Study intent, so
// the geometry can be unit-tested against a fake Author with no live host and the engine
// replays the same program over api/client (spec §6 M3, issue #21).
//
// All lengths passed to Author are host MODEL UNITS (1 unit = 1 cm = 10 mm); parameter
// expressions carry their own units ("200 mm"). Face probe points are model units too.
package demos

import "oblikovati.org/getdp/getdp/femmodel"

// Author is the parametric-modelling seam a demo builds against — a thin projection of
// the host's Documents/Parameters/Sketch/Features/Body API. The engine implements it over
// api/client; tests implement a fake. Keeping it primitive (strings + floats) keeps this
// package free of the client dependency and trivially fakeable.
type Author interface {
	// Parameter publishes a host user-parameter, literal ("200 mm") or a formula of
	// earlier ones ("2 * fin_pitch"), so the model stays parametric.
	Parameter(name, expr string) error
	// Sketch creates a sketch on a named base plane ("XY"/"XZ"/"YZ") and returns its index.
	Sketch(plane string) (int, error)
	// CornerRectangle lays a fully-constrained (DOF=0) axis-aligned rectangle whose
	// lower-left corner is anchored at (xExpr,yExpr) and whose size is driven by
	// (widthExpr,heightExpr). Anchoring at the origin ("0","0") keeps it parameter-driven.
	CornerRectangle(sketch int, xExpr, yExpr, widthExpr, heightExpr string) error
	// Extrude extrudes the sketch's single profile by distanceExpr; operation is "new"
	// (fresh body) or "join" (fuse with the active body). It returns the feature name so
	// it can seed a pattern.
	Extrude(sketch int, distanceExpr, operation string) (string, error)
	// PatternX linearly replicates sourceFeature countExpr times along +X at stepXcm
	// spacing (model units). count seeds the host schema; countExpr is the driving parameter.
	PatternX(sourceFeature string, count int, countExpr string, stepXcm float64) error
	// FaceKeyAt returns the persistent reference key of the body face at a model-unit
	// point (used to bind a boundary condition to a specific face without interactive
	// selection). The point must lie on the target face.
	FaceKeyAt(point [3]float64) (string, error)
}

// Study is a demo's configured GetDP study intent: the physics regime, the constraints
// (already bound to resolved face keys), and the global mesh size. The engine loads it
// onto a fresh femmodel study after replaying the geometry program.
type Study struct {
	Physics        femmodel.PhysicsKind
	Constraints    []femmodel.ConstraintObject
	MeshModelUnits float64 // characteristic element length (model units); 0 = auto
	Epsilon        float64 // dielectric relative permittivity εr for the part region; 0 = physics default
	AirPadding     float64 // automatic air-box padding (× part diagonal); 0 = physics default
}

// Param is one published host parameter (name + unit-bearing expression). Demos expose
// their parameter program so the engine can publish it and the README can table it.
type Param struct {
	Name string
	Expr string
	Note string // human description for the demos/README table
}
