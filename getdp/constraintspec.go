// SPDX-License-Identifier: GPL-2.0-only

package getdp

import "fmt"

// ConstraintKind names one kind of study boundary condition or excitation. It is the
// tag the task-panel editors and the spec factory key on. Kinds grow physics by
// physics (design spec §4.3): the M3 electrokinetics/thermal set is below; magnetic,
// wave and structural kinds arrive with their milestones.
type ConstraintKind string

// M3 electric + thermal constraint kinds.
const (
	KindVoltage     ConstraintKind = "voltage"     // fixed electric potential on faces
	KindCurrent     ConstraintKind = "current"     // total current injected through faces
	KindTemperature ConstraintKind = "temperature" // fixed temperature on faces
	KindHeatFlux    ConstraintKind = "heat flux"   // prescribed flux through faces
	KindConvection  ConstraintKind = "convection"  // Robin h·(T−T∞) film on faces
)

// ConstraintSpec is one study constraint as user INTENT: a kind, a display name (also
// the physical-surface name in the MSH), parameters, and a selection of host face
// reference keys. It is NOT mesh-bound — Resolve runs once the mesh exists and the
// picked faces are bound (FaceGroups), claiming a Physical Surface in the RegionTable
// and appending its solver-side contribution to the SolveModel. This is the intent-side
// analog of the .pro physics writers (the deck-side seam): a new constraint kind is a
// new spec + a writer case, nothing else.
type ConstraintSpec interface {
	Kind() ConstraintKind
	Name() string
	Faces() []string
	Resolve(rc *ResolveContext) error
}

// ResolveContext is the run-time binding environment handed to each spec: the mesh
// exists and the picked faces are bound, so a spec can claim physical surfaces and
// append to the solve model.
type ResolveContext struct {
	Model   *SolveModel
	Mesh    *TetMesh
	Groups  *FaceGroups
	Regions *RegionTable
}

// SolveModel is the resolved, mesh-bound model the .pro generator consumes: every
// entry references regions by physical tag, never by face key. The per-physics deck
// writers (M3+) read exactly this. It grows a slice per constraint family.
type SolveModel struct {
	BoundPotentials []BoundPotential
	BoundFluxes     []BoundFlux
}

// BoundPotential is a resolved Dirichlet-type condition: a scalar value (electric
// potential in V or temperature in K, per Kind) fixed on a surface region.
type BoundPotential struct {
	Kind      ConstraintKind
	RegionTag int
	Name      string
	Value     float64
}

// BoundFlux is a resolved Neumann/Robin-type condition on a surface region: a total
// current (A), a heat flux (W/m²) or a convection film (h in W/m²K against TInf in K).
type BoundFlux struct {
	Kind      ConstraintKind
	RegionTag int
	Name      string
	Value     float64
	H, TInf   float64 // convection only
}

// resolveSpecs folds constraint specs into the model in order. Order matters: surface
// tags are allocated in claim order, so the MSH and .pro stay deterministic for a
// given study tree.
func resolveSpecs(specs []ConstraintSpec, rc *ResolveContext) error {
	for i, s := range specs {
		if err := s.Resolve(rc); err != nil {
			return fmt.Errorf("constraint %d (%s %q): %w", i+1, s.Kind(), s.Name(), err)
		}
	}
	return nil
}

// DirichletSpec fixes a scalar value on its faces — the shared intent shape behind
// KindVoltage and KindTemperature.
type DirichletSpec struct {
	SpecKind ConstraintKind
	SpecName string
	FaceKeys []string
	Value    float64
}

// Kind/Name/Faces implement ConstraintSpec.
func (s DirichletSpec) Kind() ConstraintKind { return s.SpecKind }
func (s DirichletSpec) Name() string         { return s.SpecName }
func (s DirichletSpec) Faces() []string      { return s.FaceKeys }

// Resolve claims the spec's physical surface and records the fixed value against it.
func (s DirichletSpec) Resolve(rc *ResolveContext) error {
	tag, err := rc.Regions.BindSurface(s.SpecName, s.FaceKeys, rc.Groups)
	if err != nil {
		return err
	}
	rc.Model.BoundPotentials = append(rc.Model.BoundPotentials, BoundPotential{
		Kind: s.SpecKind, RegionTag: tag, Name: s.SpecName, Value: s.Value,
	})
	return nil
}

// FluxSpec drives a flux-type condition through its faces — the shared intent shape
// behind KindCurrent, KindHeatFlux and KindConvection.
type FluxSpec struct {
	SpecKind ConstraintKind
	SpecName string
	FaceKeys []string
	Value    float64
	H, TInf  float64 // convection only
}

// Kind/Name/Faces implement ConstraintSpec.
func (s FluxSpec) Kind() ConstraintKind { return s.SpecKind }
func (s FluxSpec) Name() string         { return s.SpecName }
func (s FluxSpec) Faces() []string      { return s.FaceKeys }

// Resolve claims the spec's physical surface and records the flux against it.
func (s FluxSpec) Resolve(rc *ResolveContext) error {
	tag, err := rc.Regions.BindSurface(s.SpecName, s.FaceKeys, rc.Groups)
	if err != nil {
		return err
	}
	rc.Model.BoundFluxes = append(rc.Model.BoundFluxes, BoundFlux{
		Kind: s.SpecKind, RegionTag: tag, Name: s.SpecName, Value: s.Value, H: s.H, TInf: s.TInf,
	})
	return nil
}
