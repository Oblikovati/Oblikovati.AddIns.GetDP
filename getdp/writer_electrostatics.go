// SPDX-License-Identifier: GPL-2.0-only

package getdp

import (
	"fmt"

	"oblikovati.org/getdp/getdp/pro"
)

// vacuumPermittivity is ε₀ (F/m); the deck is pure SI (the MSH writer put geometry in metres).
const vacuumPermittivity = 8.8541878128e-12

// ElectrostaticsWriter generates the electrostatic deck: ∇·(ε∇v) = 0 over the part AND the
// surrounding air (spec §3.1). Electrodes and the far-field outer boundary are Dirichlet
// potentials; the field lives in the conformal part+air mesh. Capacitance is read by the
// ENERGY method — C = 2W/ΔV² with W = ∫½ε|∇v|² over the whole domain — which needs no
// surface-charge integration (unreliable on in-plane triangle bases) and is exact for the
// parallel-plate and coaxial oracles. Outputs: v.pos, e.pos, and energy/capacitance/charge
// global tables.
type ElectrostaticsWriter struct{}

// Physics implements PhysicsWriter.
func (ElectrostaticsWriter) Physics() PhysicsKind { return PhysicsElectrostatics }

// BuildDeck implements PhysicsWriter.
func (w ElectrostaticsWriter) BuildDeck(in DeckInput) (*pro.Deck, DeckOutputs, error) {
	volts := constraintsOf(in.Model, KindVoltage)
	if len(volts) < 2 {
		return nil, DeckOutputs{}, fmt.Errorf("electrostatics needs at least two fixed potentials "+
			"(the electrodes plus the far-field boundary) to set up a field; got %d", len(volts))
	}
	dv := voltageSpan(volts)
	if dv == 0 {
		return nil, DeckOutputs{}, fmt.Errorf("all electrostatic potentials are equal (%.17g V) — no field to solve", volts[0].Value)
	}
	d := &pro.Deck{
		Groups:         regionGroups(in.Regions),
		Functions:      epsFunctions(in),
		Constraints:    []pro.Constraint{dirichletConstraint("SetV", volts)},
		Jacobians:      jacobiansFor(in),
		Integrations:   pro.StandardIntegration(in.Order),
		FunctionSpaces: []pro.FunctionSpace{pro.NodalSpace("Hgrad_v", "VolAll", "SetV")},
		Formulations:   []pro.Formulation{w.formulation()},
		Resolutions:    []pro.Resolution{pro.StaticResolution("Electrostatics", "A", "Electrostatics")},
	}
	return d, w.postProcessing(d, dv), nil
}

// epsFunctions emits the absolute permittivity ε = ε₀·εr per volume group (the air region's
// material resolves to εr = 1).
func epsFunctions(in DeckInput) []pro.Function {
	var fs []pro.Function
	for _, v := range in.Regions.Volumes {
		er := in.Materials[v.Tag].Epsilon
		if er == 0 {
			er = 1
		}
		fs = append(fs, pro.Function{
			Name: "eps", Region: volGroupName(v.Tag), Expr: fmt.Sprintf("%.17g", vacuumPermittivity*er),
		})
	}
	return fs
}

// formulation is the electrostatic weak form ∫ ε ∇v·∇v' = 0 over the whole domain.
func (ElectrostaticsWriter) formulation() pro.Formulation {
	return pro.Formulation{
		Name: "Electrostatics", Type: "FemEquation",
		Quantities: []pro.Quantity{{Name: "v", Type: "Local", Space: "Hgrad_v"}},
		Equations: []pro.EquationTerm{{
			Kind: "Galerkin", Expr: "[ eps[] * Dof{d v}, {d v} ]",
			In: "VolAll", Jacobian: pro.JacVolName, Integration: pro.IntName,
		}},
	}
}

// postProcessing prints the potential and field maps and the three global scalars. The
// capacitance and charge integrands fold in the (constant) applied span so GetDP prints C
// and Q directly: ∫ε|∇v|² = 2W, so ∫ε|∇v|²/ΔV² = C and ∫ε|∇v|²/ΔV = Q = C·ΔV.
func (ElectrostaticsWriter) postProcessing(d *pro.Deck, dv float64) DeckOutputs {
	d.PostProcessings = []pro.PostProcessing{{
		Name: "EleStaPP", Formulation: "Electrostatics", Quantities: postQuantities(dv),
	}}
	d.PostOperations = []pro.PostOperation{{
		Name: "EleStaOut", PostProcessing: "EleStaPP", Prints: postPrints(),
	}}
	return DeckOutputs{
		Resolution: "Electrostatics", PostOps: []string{"EleStaOut"},
		Fields: []FieldOutput{
			{Path: "v.pos", Label: "electric potential", Unit: "V"},
			{Path: "e.pos", Label: "electric field", Unit: "V/m"},
		},
		Tables: []TableOutput{
			{Path: "energy.txt", Label: "electrostatic energy", Unit: "J"},
			{Path: "capacitance.txt", Label: "capacitance", Unit: "F"},
			{Path: "charge.txt", Label: "electrode charge", Unit: "C"},
		},
	}
}

// postQuantities are the electrostatic post-quantities: the potential and field maps plus
// the three energy-derived global integrals (energy, capacitance, charge). SquNorm takes
// its argument in SQUARE brackets — GetDP functions are [ ]-applied.
func postQuantities(dv float64) []pro.PostQuantity {
	return []pro.PostQuantity{
		{Name: "v", Kind: "Term", Expr: "[ {v} ]", In: "VolAll", Jacobian: pro.JacVolName},
		{Name: "e", Kind: "Term", Expr: "[ -{d v} ]", In: "VolAll", Jacobian: pro.JacVolName},
		{Name: "energy", Kind: "Integral", Expr: "[ 0.5 * eps[] * SquNorm[{d v}] ]",
			In: "VolAll", Jacobian: pro.JacVolName, Integration: pro.IntName},
		{Name: "capacitance", Kind: "Integral", Expr: fmt.Sprintf("[ eps[] * SquNorm[{d v}] / %.17g ]", dv*dv),
			In: "VolAll", Jacobian: pro.JacVolName, Integration: pro.IntName},
		{Name: "charge", Kind: "Integral", Expr: fmt.Sprintf("[ eps[] * SquNorm[{d v}] / %.17g ]", dv),
			In: "VolAll", Jacobian: pro.JacVolName, Integration: pro.IntName},
	}
}

// postPrints writes the two field maps (.pos) and the three global scalars (Table) the
// runner and objective registry read back.
func postPrints() []pro.Print {
	return []pro.Print{
		{Quantity: "v", On: "OnElementsOf VolAll", File: "v.pos"},
		{Quantity: "e", On: "OnElementsOf VolAll", File: "e.pos"},
		{Quantity: "energy", Of: "[VolAll]", On: "OnGlobal", Format: "Table", File: "energy.txt"},
		{Quantity: "capacitance", Of: "[VolAll]", On: "OnGlobal", Format: "Table", File: "capacitance.txt"},
		{Quantity: "charge", Of: "[VolAll]", On: "OnGlobal", Format: "Table", File: "charge.txt"},
	}
}

// voltageSpan is the max−min of the fixed potentials — the applied ΔV the energy-method
// capacitance divides by.
func voltageSpan(volts []BoundPotential) float64 {
	lo, hi := volts[0].Value, volts[0].Value
	for _, v := range volts[1:] {
		if v.Value < lo {
			lo = v.Value
		}
		if v.Value > hi {
			hi = v.Value
		}
	}
	return hi - lo
}
