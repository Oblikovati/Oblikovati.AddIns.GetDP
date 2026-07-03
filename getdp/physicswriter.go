// SPDX-License-Identifier: GPL-2.0-only

package getdp

import (
	"fmt"

	"oblikovati.org/getdp/getdp/pro"
)

// PhysicsKind names one study physics (design spec §4.3 TP-1). Kinds grow milestone by
// milestone; the writer registry below is the single dispatch point.
type PhysicsKind string

// M3 physics kinds; M4 added electrostatics, M5 magnetostatics.
const (
	PhysicsElectrokinetics  PhysicsKind = "electrokinetics"
	PhysicsThermalSteady    PhysicsKind = "thermal"
	PhysicsThermalTransient PhysicsKind = "thermal transient"
	PhysicsElectrostatics   PhysicsKind = "electrostatics"
	PhysicsMagnetostatics   PhysicsKind = "magnetostatics"
)

// Material carries the volumetric properties the shipped physics read. Values are SI (the
// deck is pure SI; the MSH writer already put the geometry in metres).
type Material struct {
	Sigma   float64 // electrical conductivity, S/m
	K       float64 // thermal conductivity, W/(m·K)
	Rho     float64 // density, kg/m³ (transient thermal)
	Cp      float64 // specific heat, J/(kg·K) (transient thermal)
	Epsilon float64 // relative permittivity εr (electrostatics), 1 = vacuum/air
	Mu      float64 // relative permeability μr (magnetostatics), 1 = vacuum/air/copper
}

// DeckInput is everything a physics writer needs to build a deck: the region table
// (tags shared with the written MSH), the resolved constraints, per-volume materials,
// and the study's numeric knobs.
type DeckInput struct {
	Regions   *RegionTable
	Model     *SolveModel
	Materials map[int]Material  // by volume tag
	Order     int               // element order (integration rule selection)
	Transient *TransientSpec    // nil for static studies
	Shell     *ShellTransform   // nil unless the study is truncated by an infinite shell (#25)
	Probes    []FieldProbe      // point field-value probes (magnetostatics oracle / coil-centre field)
	Solver    *pro.SolverParams // linear-solver knobs (TP-12); nil = the physics writer's defaults
}

// FieldProbe is one point at which a physics writer prints the field value to its own
// Table file (SI metres). Magnetostatics prints |B| there — the on-axis probe the
// solenoid/Biot-Savart oracles read, and the physically meaningful field at a coil centre.
type FieldProbe struct {
	Name  string
	Point [3]float64
}

// ShellTransform tells a physics writer this study is truncated by an infinite spherical shell:
// the shell's physical volume tag and the metres-space sphere the VolSphShell map is built on.
// When set, the volume Jacobian maps the shell region to VolSphShell (exterior→infinity) and
// every other region to plain Vol. Radii and center are SI metres (the deck is pure SI).
type ShellTransform struct {
	VolumeTag  int
	Rint, Rext float64
	Center     [3]float64
}

// TransientSpec is the theta-scheme time grid of a transient study.
type TransientSpec struct {
	TMax, DT float64
	Theta    float64 // 1 = implicit Euler, 0.5 = Crank-Nicolson
	Initial  float64 // initial field value (e.g. starting temperature)
}

// DeckOutputs tells the runner what to execute and what files the deck produces.
type DeckOutputs struct {
	Resolution string
	PostOps    []string
	Fields     []FieldOutput
	Tables     []TableOutput
	Solver     *pro.SolverParams // non-nil = write a solver.par (SPARSKIT knobs) into the run dir
}

// FieldOutput is one .pos field map the deck prints.
type FieldOutput struct {
	Path  string
	Label string
	Unit  string
}

// TableOutput is one Format Table scalar the deck prints — the objective registry of
// the optimization milestone reads exactly these.
type TableOutput struct {
	Path  string
	Label string
	Unit  string
}

// PhysicsWriter builds the .pro deck for one study physics. One writer per kind; the
// registry keeps dispatch declarative (a new physics registers, nothing else changes).
type PhysicsWriter interface {
	Physics() PhysicsKind
	BuildDeck(in DeckInput) (*pro.Deck, DeckOutputs, error)
}

// physicsWriters is the writer registry, keyed by kind.
var physicsWriters = map[PhysicsKind]PhysicsWriter{
	PhysicsElectrokinetics:  ElectrokineticsWriter{},
	PhysicsThermalSteady:    ThermalWriter{},
	PhysicsThermalTransient: ThermalWriter{Transient: true},
	PhysicsElectrostatics:   ElectrostaticsWriter{},
	PhysicsMagnetostatics:   MagnetostaticsWriter{},
}

// WriterFor returns the deck writer for a physics kind, failing loudly on a kind no
// milestone has shipped yet.
func WriterFor(kind PhysicsKind) (PhysicsWriter, error) {
	w, ok := physicsWriters[kind]
	if !ok {
		return nil, fmt.Errorf("physics %q has no deck writer yet (shipped kinds: electrokinetics, thermal, thermal transient)", kind)
	}
	return w, nil
}

// volGroupName / surGroupName are the deterministic .pro identifiers for regions —
// $PhysicalNames carry the human names, the deck uses tag-derived identifiers so any
// user-entered name is syntactically safe.
func volGroupName(tag int) string { return fmt.Sprintf("Vol%d", tag) }
func surGroupName(tag int) string { return fmt.Sprintf("Sur%d", tag) }

// jacobiansFor picks the deck's Jacobians: the plain volume+surface pair for a confined or
// padded-box study, or the shell-aware pair (shell region → VolSphShell) when the study is
// truncated by an infinite spherical shell (#25).
func jacobiansFor(in DeckInput) []pro.Jacobian {
	if in.Shell == nil {
		return pro.StandardJacobians()
	}
	return pro.ShellJacobians(volGroupName(in.Shell.VolumeTag), in.Shell.Rint, in.Shell.Rext, in.Shell.Center)
}

// regionGroups builds the shared Group block skeleton: one group per volume, one per
// bound surface, a VolAll union, and a DomAll union of everything (nodal spaces need
// the boundary in their support).
func regionGroups(regions *RegionTable) []pro.Group {
	var gs []pro.Group
	var volNames, allNames []string
	for _, v := range regions.Volumes {
		gs = append(gs, pro.Group{Name: volGroupName(v.Tag), Regions: []int{v.Tag}})
		volNames = append(volNames, volGroupName(v.Tag))
		allNames = append(allNames, volGroupName(v.Tag))
	}
	for _, s := range regions.Surfaces {
		gs = append(gs, pro.Group{Name: surGroupName(s.Tag), Regions: []int{s.Tag}})
		allNames = append(allNames, surGroupName(s.Tag))
	}
	gs = append(gs, pro.Group{Name: "VolAll", SubGroups: volNames})
	gs = append(gs, pro.Group{Name: "DomAll", SubGroups: allNames})
	return gs
}
