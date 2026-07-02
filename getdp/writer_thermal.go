// SPDX-License-Identifier: GPL-2.0-only

package getdp

import (
	"fmt"

	"oblikovati.org/getdp/getdp/pro"
)

// ThermalWriter generates the heat-conduction deck: ∇·(k∇T) = ρc ∂T/∂t. Fixed-
// temperature surfaces (KindTemperature) are TERMINALS (same global-quantity pattern
// as the electrokinetic electrodes), so each reports its exact heat rate
// heatrate_Sur<tag>.txt from the assembled system. Prescribed fluxes (KindHeatFlux)
// and Robin films (KindConvection) enter as Galerkin surface terms — those need no
// normal gradient, so plain surface integration is exact for them. Steady state solves
// the static problem; Transient=true adds the ρc DtDof storage term under a theta time
// loop (issue #14). No air region: conduction lives in the solid (spec §3.3).
type ThermalWriter struct {
	Transient bool
}

// Physics implements PhysicsWriter.
func (w ThermalWriter) Physics() PhysicsKind {
	if w.Transient {
		return PhysicsThermalTransient
	}
	return PhysicsThermalSteady
}

// BuildDeck implements PhysicsWriter.
func (w ThermalWriter) BuildDeck(in DeckInput) (*pro.Deck, DeckOutputs, error) {
	temps := constraintsOf(in.Model, KindTemperature)
	if err := w.validate(in, temps); err != nil {
		return nil, DeckOutputs{}, err
	}
	terminals := terminalTags(temps, nil)
	d := &pro.Deck{
		Groups:         append(regionGroups(in.Regions), terminalsGroup(terminals)),
		Functions:      w.thermalFunctions(in),
		Constraints:    w.constraints(in, temps),
		Jacobians:      pro.StandardJacobians(),
		Integrations:   pro.StandardIntegration(in.Order),
		FunctionSpaces: []pro.FunctionSpace{w.space()},
		Formulations:   []pro.Formulation{w.formulation(in)},
		Resolutions:    []pro.Resolution{w.resolution(in)},
	}
	outs := w.postProcessing(d, terminals)
	return d, outs, nil
}

// validate rejects an unsolvable study up front: without any Dirichlet temperature the
// steady pure-flux problem is singular (convection alone anchors it, but through a
// terminal-less space — a follow-up; M3 requires a temperature anchor).
func (w ThermalWriter) validate(in DeckInput, temps []BoundPotential) error {
	if len(temps) == 0 {
		return fmt.Errorf("thermal study needs a temperature constraint to anchor the field (got %d fluxes/films only)",
			len(in.Model.BoundFluxes))
	}
	if w.Transient && in.Transient == nil {
		return fmt.Errorf("transient thermal study has no time grid (TransientSpec is nil)")
	}
	return nil
}

// thermalFunctions emits conductivity (and, transient, ρc) per volume group.
func (w ThermalWriter) thermalFunctions(in DeckInput) []pro.Function {
	var fs []pro.Function
	for _, v := range in.Regions.Volumes {
		m := in.Materials[v.Tag]
		fs = append(fs, pro.Function{Name: "k", Region: volGroupName(v.Tag), Expr: fmt.Sprintf("%.17g", m.K)})
		if w.Transient {
			fs = append(fs, pro.Function{Name: "rhoc", Region: volGroupName(v.Tag), Expr: fmt.Sprintf("%.17g", m.Rho*m.Cp)})
		}
	}
	return fs
}

// constraints emits the terminal temperatures and, transient, the initial condition.
func (w ThermalWriter) constraints(in DeckInput, temps []BoundPotential) []pro.Constraint {
	cs := []pro.Constraint{dirichletConstraint("SetT", temps)}
	if w.Transient {
		cs = append(cs, pro.Constraint{Name: "InitT", Type: "Init", Cases: []pro.ConstraintCase{
			{Region: "VolAll", Value: fmt.Sprintf("%.17g", in.Transient.Initial)},
		}})
	}
	return cs
}

// space is the terminal nodal temperature space; transient studies also seed the
// interior nodes' initial condition.
func (w ThermalWriter) space() pro.FunctionSpace {
	s := pro.TerminalNodalSpace("Hgrad_T", "DomAll", "Terminals", "Tterm", "Qterm")
	s.SpaceConstraints = []pro.SpaceConstraint{
		{Coef: "Tterm", EntityType: "GroupsOfNodesOf", Constraint: "SetT"},
	}
	if w.Transient {
		s.SpaceConstraints = append(s.SpaceConstraints,
			pro.SpaceConstraint{Coef: "vn", EntityType: "NodesOf", Constraint: "InitT"})
	}
	return s
}

// formulation is the conduction weak form plus the terminal global term, the surface
// flux/convection terms, and (transient) the storage term.
func (w ThermalWriter) formulation(in DeckInput) pro.Formulation {
	f := pro.Formulation{
		Name: "Thermal", Type: "FemEquation",
		Quantities: []pro.Quantity{
			{Name: "T", Type: "Local", Space: "Hgrad_T"},
			{Name: "Tterm", Type: "Global", Space: "Hgrad_T", SpaceQuantity: "Tterm"},
			{Name: "Qterm", Type: "Global", Space: "Hgrad_T", SpaceQuantity: "Qterm"},
		},
		Equations: []pro.EquationTerm{{
			Kind: "Galerkin", Expr: "[ k[] * Dof{d T}, {d T} ]",
			In: "VolAll", Jacobian: pro.JacVolName, Integration: pro.IntName,
		}},
	}
	if w.Transient {
		f.Equations = append(f.Equations, pro.EquationTerm{
			Kind: "Galerkin", Expr: "DtDof[ rhoc[] * Dof{T}, {T} ]",
			In: "VolAll", Jacobian: pro.JacVolName, Integration: pro.IntName,
		})
	}
	f.Equations = append(f.Equations, w.surfaceTerms(in)...)
	f.Equations = append(f.Equations, pro.EquationTerm{
		Kind: "GlobalTerm", Expr: "[ Dof{Qterm}, {Tterm} ]", In: "Terminals",
	})
	return f
}

// surfaceTerms renders the Neumann heat-flux and Robin convection contributions.
func (ThermalWriter) surfaceTerms(in DeckInput) []pro.EquationTerm {
	var terms []pro.EquationTerm
	for _, c := range constraintsOf2(in.Model, KindHeatFlux) {
		q := c.Value / surfaceAreaM2(in.Regions, c.RegionTag)
		terms = append(terms, pro.EquationTerm{
			Kind: "Galerkin", Expr: fmt.Sprintf("[ -%.17g, {T} ]", q),
			In: surGroupName(c.RegionTag), Jacobian: pro.JacSurName, Integration: pro.IntName,
		})
	}
	for _, c := range constraintsOf2(in.Model, KindConvection) {
		terms = append(terms,
			pro.EquationTerm{
				Kind: "Galerkin", Expr: fmt.Sprintf("[ %.17g * Dof{T}, {T} ]", c.H),
				In: surGroupName(c.RegionTag), Jacobian: pro.JacSurName, Integration: pro.IntName,
			},
			pro.EquationTerm{
				Kind: "Galerkin", Expr: fmt.Sprintf("[ %.17g, {T} ]", -c.H*c.TInf),
				In: surGroupName(c.RegionTag), Jacobian: pro.JacSurName, Integration: pro.IntName,
			})
	}
	return terms
}

// resolution picks static or theta-transient solve.
func (w ThermalWriter) resolution(in DeckInput) pro.Resolution {
	if !w.Transient {
		return pro.StaticResolution("Thermal", "A", "Thermal")
	}
	t := in.Transient
	return pro.ThetaResolution("Thermal", "A", "Thermal",
		"0", fmt.Sprintf("%.17g", t.TMax), fmt.Sprintf("%.17g", t.DT), fmt.Sprintf("%.17g", t.Theta))
}

// postProcessing attaches the field maps and the per-terminal heat-rate tables.
func (ThermalWriter) postProcessing(d *pro.Deck, terminals []int) DeckOutputs {
	d.PostProcessings = []pro.PostProcessing{{
		Name: "ThermalPP", Formulation: "Thermal",
		Quantities: []pro.PostQuantity{
			{Name: "T", Kind: "Term", Expr: "[ {T} ]", In: "VolAll", Jacobian: pro.JacVolName},
			{Name: "q", Kind: "Term", Expr: "[ -k[] * {d T} ]", In: "VolAll", Jacobian: pro.JacVolName},
			{Name: "Tterm", Kind: "Term", Expr: "[ {Tterm} ]", In: "Terminals"},
			{Name: "Qterm", Kind: "Term", Expr: "[ {Qterm} ]", In: "Terminals"},
		},
	}}
	po := pro.PostOperation{
		Name: "ThermalOut", PostProcessing: "ThermalPP",
		Prints: []pro.Print{
			{Quantity: "T", On: "OnElementsOf VolAll", File: "T.pos"},
			{Quantity: "q", On: "OnElementsOf VolAll", File: "q.pos"},
		},
	}
	outs := DeckOutputs{
		Resolution: "Thermal", PostOps: []string{"ThermalOut"},
		Fields: []FieldOutput{
			{Path: "T.pos", Label: "temperature", Unit: "K"},
			{Path: "q.pos", Label: "heat flux", Unit: "W/m²"},
		},
	}
	for _, tag := range terminals {
		po.Prints, outs.Tables = appendTerminalPrints(po.Prints, outs.Tables, tag,
			"Qterm", "heatrate", "terminal heat rate", "W",
			"Tterm", "temperature", "terminal temperature", "K")
	}
	d.PostOperations = []pro.PostOperation{po}
	return outs
}
