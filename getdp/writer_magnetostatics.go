// SPDX-License-Identifier: GPL-2.0-only

package getdp

import (
	"fmt"

	"oblikovati.org/getdp/getdp/pro"
)

// vacuumPermeability is μ₀ (H/m); the deck is pure SI (the MSH writer put geometry in metres).
const vacuumPermeability = 1.25663706212e-06

// MagnetostaticsWriter generates the 3D magnetostatics deck: the magnetic vector potential a
// on edge (Whitney-1) elements, weak form ∫ ν (∇×a)·(∇×a') = ∫ js·a' over the part, coil and
// surrounding air/shell (spec §3.4). The system is UNGAUGED — no tree-cotree gauge — so it is
// consistent-singular; SPARSKIT GMRES with a diagonal preconditioner recovers the
// gauge-invariant field B = ∇×a (the non-unique a is immaterial; gauging is #40). Coils carry
// an azimuthal current density; the far-field shell boundary pins a×n = 0. Outputs: |B| field
// map (b.pos), magnetic energy (energy.txt) and an on-axis |B| probe per FieldProbe — the
// solenoid/Biot-Savart oracles read the probe.
type MagnetostaticsWriter struct{}

// Physics implements PhysicsWriter.
func (MagnetostaticsWriter) Physics() PhysicsKind { return PhysicsMagnetostatics }

// BuildDeck implements PhysicsWriter.
func (w MagnetostaticsWriter) BuildDeck(in DeckInput) (*pro.Deck, DeckOutputs, error) {
	if len(in.Model.Coils) == 0 {
		return nil, DeckOutputs{}, fmt.Errorf("magnetostatics needs at least one coil (current source) to drive a field; got none")
	}
	if in.Model.FarFieldTag == 0 {
		return nil, DeckOutputs{}, fmt.Errorf("magnetostatics needs a far-field boundary (the auto air/shell outer wall) to anchor a×n=0; none bound")
	}
	d := &pro.Deck{
		Groups:         append(regionGroups(in.Regions), coilSourceGroup(in.Model.Coils)),
		Functions:      append(nuFunctions(in), jsFunctions(in.Model.Coils)...),
		Constraints:    []pro.Constraint{farFieldConstraint(in.Model.FarFieldTag)},
		Jacobians:      jacobiansFor(in),
		Integrations:   pro.StandardIntegration(in.Order),
		FunctionSpaces: []pro.FunctionSpace{pro.EdgeSpace("Hcurl_a", "VolAll", "SetA")},
		Formulations:   []pro.Formulation{w.formulation()},
		Resolutions:    []pro.Resolution{pro.StaticResolution("Magnetostatics", "A", "Magnetostatics")},
	}
	return d, w.postProcessing(d, in.Probes), nil
}

// nuFunctions emits the magnetic reluctivity ν = 1/(μ₀·μr) per volume group (air, coil and
// shell all resolve to μr = 1).
func nuFunctions(in DeckInput) []pro.Function {
	var fs []pro.Function
	for _, v := range in.Regions.Volumes {
		mur := in.Materials[v.Tag].Mu
		if mur == 0 {
			mur = 1
		}
		fs = append(fs, pro.Function{
			Name: "nu", Region: volGroupName(v.Tag), Expr: fmt.Sprintf("%.17g", 1/(vacuumPermeability*mur)),
		})
	}
	return fs
}

// jsFunctions emits the azimuthal source current density per coil: js = J0 · unit(axis ×
// (position − center)). GetDP's `/\` is the cross product and Unit[] normalizes (and returns
// zero on the axis, so the singular line is safe). See geometry-math-advisor / #27.
func jsFunctions(coils []Coil) []pro.Function {
	var fs []pro.Function
	for _, c := range coils {
		expr := fmt.Sprintf("%.17g * Unit[ Vector[%v, %v, %v] /\\ (XYZ[] - Vector[%v, %v, %v]) ]",
			c.CurrentDensity, c.Axis[0], c.Axis[1], c.Axis[2], c.Center[0], c.Center[1], c.Center[2])
		fs = append(fs, pro.Function{Name: "js", Region: volGroupName(c.RegionTag), Expr: expr})
	}
	return fs
}

// coilSourceGroup unions the coil volume groups under VolS — the support of the source term.
func coilSourceGroup(coils []Coil) pro.Group {
	g := pro.Group{Name: "VolS"}
	for _, c := range coils {
		g.SubGroups = append(g.SubGroups, volGroupName(c.RegionTag))
	}
	return g
}

// farFieldConstraint pins a = 0 on the far-field boundary edges (the edge space applies it
// via EntityType EdgesOf).
func farFieldConstraint(tag int) pro.Constraint {
	return pro.Constraint{Name: "SetA", Cases: []pro.ConstraintCase{
		{Region: surGroupName(tag), Value: "0"},
	}}
}

// formulation is the ungauged curl-curl weak form plus the coil source term. The curl-curl
// term runs over the whole domain (VolAll); the source only over the coil volumes (VolS).
func (MagnetostaticsWriter) formulation() pro.Formulation {
	return pro.Formulation{
		Name: "Magnetostatics", Type: "FemEquation",
		Quantities: []pro.Quantity{{Name: "a", Type: "Local", Space: "Hcurl_a"}},
		Equations: []pro.EquationTerm{
			{Kind: "Galerkin", Expr: "[ nu[] * Dof{d a}, {d a} ]",
				In: "VolAll", Jacobian: pro.JacVolName, Integration: pro.IntName},
			{Kind: "Galerkin", Expr: "[ -js[], {a} ]",
				In: "VolS", Jacobian: pro.JacVolName, Integration: pro.IntName},
		},
	}
}

// postProcessing prints the |B| field map, the magnetic energy scalar, and one |B| point
// probe per FieldProbe, and attaches the SPARSKIT solver.par for the ungauged system.
func (MagnetostaticsWriter) postProcessing(d *pro.Deck, probes []FieldProbe) DeckOutputs {
	d.PostProcessings = []pro.PostProcessing{{
		Name: "MagStaPP", Formulation: "Magnetostatics", Quantities: magPostQuantities(),
	}}
	prints := []pro.Print{
		{Quantity: "bnorm", On: "OnElementsOf VolAll", File: "b.pos"},
		{Quantity: "energy", Of: "[VolAll]", On: "OnGlobal", Format: "Table", File: "energy.txt"},
	}
	outs := DeckOutputs{
		Resolution: "Magnetostatics", PostOps: []string{"MagStaOut"},
		Fields: []FieldOutput{{Path: "b.pos", Label: "magnetic flux density", Unit: "T"}},
		Tables: []TableOutput{{Path: "energy.txt", Label: "magnetic energy", Unit: "J"}},
		Solver: ptrSolver(pro.DefaultMagnetostaticsSolver()),
	}
	prints, outs.Tables = appendProbePrints(prints, outs.Tables, probes)
	d.PostOperations = []pro.PostOperation{{Name: "MagStaOut", PostProcessing: "MagStaPP", Prints: prints}}
	return outs
}

// magPostQuantities are the magnetostatic post-quantities: B and H field maps, |B| (the
// scalar the flood plot and probes read), and the magnetic energy ∫ ½ν|B|² = ∫ ½ B·H.
func magPostQuantities() []pro.PostQuantity {
	return []pro.PostQuantity{
		{Name: "b", Kind: "Term", Expr: "[ {d a} ]", In: "VolAll", Jacobian: pro.JacVolName},
		{Name: "bnorm", Kind: "Term", Expr: "[ Norm[{d a}] ]", In: "VolAll", Jacobian: pro.JacVolName},
		{Name: "h", Kind: "Term", Expr: "[ nu[] * {d a} ]", In: "VolAll", Jacobian: pro.JacVolName},
		{Name: "a", Kind: "Term", Expr: "[ {a} ]", In: "VolAll", Jacobian: pro.JacVolName},
		{Name: "energy", Kind: "Integral", Expr: "[ 0.5 * nu[] * SquNorm[{d a}] ]",
			In: "VolAll", Jacobian: pro.JacVolName, Integration: pro.IntName},
	}
}

// appendProbePrints adds one |B| point print + table per probe (SI metres; the oracle reads
// these). Each file is b_<name>.txt.
func appendProbePrints(prints []pro.Print, tables []TableOutput, probes []FieldProbe) ([]pro.Print, []TableOutput) {
	for _, p := range probes {
		file := fmt.Sprintf("b_%s.txt", p.Name)
		on := fmt.Sprintf("OnPoint {%v, %v, %v}", p.Point[0], p.Point[1], p.Point[2])
		prints = append(prints, pro.Print{Quantity: "bnorm", On: on, Format: "Table", File: file})
		tables = append(tables, TableOutput{Path: file, Label: "|B| at " + p.Name, Unit: "T"})
	}
	return prints, tables
}

// ptrSolver returns a pointer to a solver-params value (DeckOutputs.Solver is optional).
func ptrSolver(p pro.SolverParams) *pro.SolverParams { return &p }
