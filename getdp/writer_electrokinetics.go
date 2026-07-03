// SPDX-License-Identifier: GPL-2.0-only

package getdp

import (
	"fmt"

	"oblikovati.org/getdp/getdp/pro"
)

// ElectrokineticsWriter generates the steady current-conduction deck: ∇·(σ∇v) = 0 with
// electrode terminals. Every voltage (KindVoltage) and current (KindCurrent) surface is
// a TERMINAL: its nodes collapse into one global coefficient whose AliasOf is the
// electrode potential U and whose AssociatedWith is the electrode current I, read
// exactly from the assembled system (the upstream template pattern — surface-gradient
// integrals cannot represent the normal flux). Voltage electrodes constrain U; current
// electrodes constrain I. Outputs: v.pos, j.pos, and per-terminal current_Sur<tag>.txt
// / voltage_Sur<tag>.txt tables (R = ΔU/I is derived from these; busbar oracle
// R = L/(σA) exactly).
type ElectrokineticsWriter struct{}

// Physics implements PhysicsWriter.
func (ElectrokineticsWriter) Physics() PhysicsKind { return PhysicsElectrokinetics }

// BuildDeck implements PhysicsWriter.
func (w ElectrokineticsWriter) BuildDeck(in DeckInput) (*pro.Deck, DeckOutputs, error) {
	volts := constraintsOf(in.Model, KindVoltage)
	currents := constraintsOf2(in.Model, KindCurrent)
	if len(volts) == 0 {
		return nil, DeckOutputs{}, fmt.Errorf("electrokinetics needs at least one voltage electrode to anchor the potential (got %d current electrodes)", len(currents))
	}
	terminals := terminalTags(volts, currents)
	d := &pro.Deck{
		Groups:         append(regionGroups(in.Regions), terminalsGroup(terminals)),
		Functions:      sigmaFunctions(in),
		Constraints:    w.constraints(volts, currents),
		Jacobians:      pro.StandardJacobians(),
		Integrations:   pro.StandardIntegration(in.Order),
		FunctionSpaces: []pro.FunctionSpace{w.space(currents)},
		Formulations:   []pro.Formulation{w.formulation()},
		Resolutions:    []pro.Resolution{pro.StaticResolution("EleKin", "A", "EleKin")},
	}
	outs := w.postProcessing(d, terminals)
	return d, outs, nil
}

// sigmaFunctions emits one conductivity function per volume group.
func sigmaFunctions(in DeckInput) []pro.Function {
	var fs []pro.Function
	for _, v := range in.Regions.Volumes {
		fs = append(fs, pro.Function{
			Name: "sigma", Region: volGroupName(v.Tag),
			Expr: fmt.Sprintf("%.17g", in.Materials[v.Tag].Sigma),
		})
	}
	return fs
}

// constraints pins voltage electrodes by potential and current electrodes by injected
// current (the associated global quantity).
func (ElectrokineticsWriter) constraints(volts []BoundPotential, currents []BoundFlux) []pro.Constraint {
	cs := []pro.Constraint{dirichletConstraint("SetV", volts)}
	if len(currents) > 0 {
		c := pro.Constraint{Name: "SetI"}
		for _, e := range currents {
			c.Cases = append(c.Cases, pro.ConstraintCase{
				Region: surGroupName(e.RegionTag), Value: fmt.Sprintf("%.17g", e.Value),
			})
		}
		cs = append(cs, c)
	}
	return cs
}

// space is the terminal nodal space with U/I global quantities.
func (ElectrokineticsWriter) space(currents []BoundFlux) pro.FunctionSpace {
	s := pro.TerminalNodalSpace("Hgrad_v", "DomAll", "Terminals", "U", "I")
	s.SpaceConstraints = []pro.SpaceConstraint{
		{Coef: "U", EntityType: "GroupsOfNodesOf", Constraint: "SetV"},
	}
	if len(currents) > 0 {
		s.SpaceConstraints = append(s.SpaceConstraints,
			pro.SpaceConstraint{Coef: "I", EntityType: "GroupsOfNodesOf", Constraint: "SetI"})
	}
	return s
}

// formulation is the conduction weak form plus the terminal global term.
func (ElectrokineticsWriter) formulation() pro.Formulation {
	return pro.Formulation{
		Name: "EleKin", Type: "FemEquation",
		Quantities: []pro.Quantity{
			{Name: "v", Type: "Local", Space: "Hgrad_v"},
			{Name: "U", Type: "Global", Space: "Hgrad_v", SpaceQuantity: "U"},
			{Name: "I", Type: "Global", Space: "Hgrad_v", SpaceQuantity: "I"},
		},
		Equations: []pro.EquationTerm{
			{Kind: "Galerkin", Expr: "[ sigma[] * Dof{d v}, {d v} ]",
				In: "VolAll", Jacobian: pro.JacVolName, Integration: pro.IntName},
			{Kind: "GlobalTerm", Expr: "[ Dof{I}, {U} ]", In: "Terminals"},
		},
	}
}

// postProcessing attaches the field maps and the per-terminal U/I tables.
func (ElectrokineticsWriter) postProcessing(d *pro.Deck, terminals []int) DeckOutputs {
	d.PostProcessings = []pro.PostProcessing{{
		Name: "EleKinPP", Formulation: "EleKin",
		Quantities: []pro.PostQuantity{
			{Name: "v", Kind: "Term", Expr: "[ {v} ]", In: "VolAll", Jacobian: pro.JacVolName},
			{Name: "j", Kind: "Term", Expr: "[ -sigma[] * {d v} ]", In: "VolAll", Jacobian: pro.JacVolName},
			{Name: "U", Kind: "Term", Expr: "[ {U} ]", In: "Terminals"},
			{Name: "I", Kind: "Term", Expr: "[ {I} ]", In: "Terminals"},
		},
	}}
	po := pro.PostOperation{
		Name: "EleKinOut", PostProcessing: "EleKinPP",
		Prints: []pro.Print{
			{Quantity: "v", On: "OnElementsOf VolAll", File: "v.pos"},
			{Quantity: "j", On: "OnElementsOf VolAll", File: "j.pos"},
		},
	}
	outs := DeckOutputs{
		Resolution: "EleKin", PostOps: []string{"EleKinOut"},
		Fields: []FieldOutput{
			{Path: "v.pos", Label: "electric potential", Unit: "V"},
			{Path: "j.pos", Label: "current density", Unit: "A/m²"},
		},
	}
	for _, tag := range terminals {
		po.Prints, outs.Tables = appendTerminalPrints(po.Prints, outs.Tables, tag,
			"I", "current", "electrode current", "A",
			"U", "voltage", "electrode potential", "V")
	}
	d.PostOperations = []pro.PostOperation{po}
	return outs
}

// appendTerminalPrints adds the flux + value table prints of one terminal.
func appendTerminalPrints(prints []pro.Print, tables []TableOutput, tag int,
	fluxQty, fluxFile, fluxLabel, fluxUnit, valQty, valFile, valLabel, valUnit string) ([]pro.Print, []TableOutput) {
	sur := surGroupName(tag)
	fluxPath := fmt.Sprintf("%s_%s.txt", fluxFile, sur)
	valPath := fmt.Sprintf("%s_%s.txt", valFile, sur)
	prints = append(prints,
		pro.Print{Quantity: fluxQty, On: "OnRegion " + sur, Format: "Table", File: fluxPath},
		pro.Print{Quantity: valQty, On: "OnRegion " + sur, Format: "Table", File: valPath})
	tables = append(tables,
		TableOutput{Path: fluxPath, Label: fmt.Sprintf("%s (%s)", fluxLabel, sur), Unit: fluxUnit},
		TableOutput{Path: valPath, Label: fmt.Sprintf("%s (%s)", valLabel, sur), Unit: valUnit})
	return prints, tables
}

// terminalTags collects the surface tags of every electrode, voltage first.
func terminalTags(volts []BoundPotential, currents []BoundFlux) []int {
	var tags []int
	for _, v := range volts {
		tags = append(tags, v.RegionTag)
	}
	for _, c := range currents {
		tags = append(tags, c.RegionTag)
	}
	return tags
}

// terminalsGroup unions the electrode surface groups under the Terminals name.
func terminalsGroup(tags []int) pro.Group {
	g := pro.Group{Name: "Terminals"}
	for _, t := range tags {
		g.SubGroups = append(g.SubGroups, surGroupName(t))
	}
	return g
}

// constraintsOf filters the model's Dirichlet entries by kind.
func constraintsOf(m *SolveModel, kind ConstraintKind) []BoundPotential {
	var out []BoundPotential
	for _, p := range m.BoundPotentials {
		if p.Kind == kind {
			out = append(out, p)
		}
	}
	return out
}

// constraintsOf2 filters the model's flux entries by kind.
func constraintsOf2(m *SolveModel, kind ConstraintKind) []BoundFlux {
	var out []BoundFlux
	for _, f := range m.BoundFluxes {
		if f.Kind == kind {
			out = append(out, f)
		}
	}
	return out
}

// dirichletConstraint folds Dirichlet entries into one named .pro constraint.
func dirichletConstraint(name string, entries []BoundPotential) pro.Constraint {
	c := pro.Constraint{Name: name}
	for _, e := range entries {
		c.Cases = append(c.Cases, pro.ConstraintCase{
			Region: surGroupName(e.RegionTag),
			Value:  fmt.Sprintf("%.17g", e.Value),
		})
	}
	return c
}

// surfaceAreaM2 returns a bound surface region's area in m² (facet coordinates are
// model units; the same modelUnitM seam the MSH writer applies). Flux-density BCs
// (heat flux) divide totals by it.
func surfaceAreaM2(regions *RegionTable, tag int) float64 {
	for _, s := range regions.Surfaces {
		if s.Tag == tag {
			return s.AreaModelUnits * modelUnitM * modelUnitM
		}
	}
	return 1 // unreachable when specs resolved; a wrong density is loudly visible in results
}
