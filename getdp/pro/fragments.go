// SPDX-License-Identifier: GPL-2.0-only

package pro

// Shared composable fragments: every physics writer starts from the same Jacobian and
// integration rules so decks stay uniform (and golden diffs stay small). Names are
// package-level constants so writers and post-processing agree by construction.

// Fragment names shared across all generated decks.
const (
	JacVolName = "JVol" // volume Jacobian
	JacSurName = "JSur" // surface Jacobian (flux integrals, Robin terms)
	IntName    = "I1"   // the standard Gauss rule
)

// StandardJacobians returns the volume + surface Jacobians every deck carries.
func StandardJacobians() []Jacobian {
	return []Jacobian{
		{Name: JacVolName, Cases: []JacobianCase{{Region: "All", Jacobian: "Vol"}}},
		{Name: JacSurName, Cases: []JacobianCase{{Region: "All", Jacobian: "Sur"}}},
	}
}

// StandardIntegration returns the Gauss rule matched to first/second-order tet meshes:
// 4-point tets and 3-point triangles integrate the v1 nodal formulations exactly;
// second-order meshes bump both.
func StandardIntegration(order int) []Integration {
	tet, tri := 4, 3
	if order >= 2 {
		tet, tri = 15, 7
	}
	return []Integration{{
		Name: IntName,
		Gauss: []GaussCase{
			{GeoElement: "Point", Points: 1},
			{GeoElement: "Line", Points: 3},
			{GeoElement: "Triangle", Points: tri},
			{GeoElement: "Tetrahedron", Points: tet},
		},
	}}
}

// NodalSpace returns the H(grad) Form0 nodal space every scalar-potential physics
// (electrokinetics, thermal, electrostatics) builds on, with coefficients pinned by
// the named Dirichlet constraint.
func NodalSpace(name, support, constraint string) FunctionSpace {
	return FunctionSpace{
		Name: name, Type: "Form0",
		BasisFunctions: []BasisFunction{{
			Name: "sn", Coef: "vn", Func: "BF_Node",
			Support: support, Entity: "NodesOf[All]",
		}},
		SpaceConstraints: []SpaceConstraint{{
			Coef: "vn", EntityType: "NodesOf", Constraint: constraint,
		}},
	}
}

// TerminalNodalSpace returns the H(grad) nodal space with TERMINAL global quantities —
// the pattern every scalar-potential physics uses to read exact terminal fluxes
// (electrode current, anchored heat rate) from the assembled system instead of
// integrating surface gradients (which in-plane triangle bases cannot represent).
// Interior nodes carry BF_Node; each terminal's nodes collapse into one
// BF_GroupOfNodes coefficient whose AliasOf is the terminal value and whose
// AssociatedWith is the terminal flux.
func TerminalNodalSpace(name, support, terminals, valueName, fluxName string) FunctionSpace {
	return FunctionSpace{
		Name: name, Type: "Form0",
		BasisFunctions: []BasisFunction{
			{Name: "sn", Coef: "vn", Func: "BF_Node",
				Support: support, Entity: "NodesOf[All, Not " + terminals + "]"},
			{Name: "sf", Coef: "vf", Func: "BF_GroupOfNodes",
				Support: support, Entity: "GroupsOfNodesOf[" + terminals + "]"},
		},
		GlobalQuantities: []GlobalQuantity{
			{Name: valueName, Type: "AliasOf", Coef: "vf"},
			{Name: fluxName, Type: "AssociatedWith", Coef: "vf"},
		},
	}
}

// StaticResolution returns the Generate/Solve/SaveSolution recipe for one linear
// static system.
func StaticResolution(name, system, formulation string) Resolution {
	return Resolution{
		Name:    name,
		Systems: []System{{Name: system, Formulation: formulation}},
		Operations: []Operation{
			RawOp("Generate[" + system + "]"),
			RawOp("Solve[" + system + "]"),
			RawOp("SaveSolution[" + system + "]"),
		},
	}
}

// ThetaResolution returns the theta-scheme transient recipe: initialize, then loop
// Generate/Solve/SaveSolution from t0 to tMax with step dt (all constant expressions,
// so -setnumber can drive them).
func ThetaResolution(name, system, formulation, t0, tMax, dt, theta string) Resolution {
	return Resolution{
		Name:    name,
		Systems: []System{{Name: system, Formulation: formulation}},
		Operations: []Operation{
			RawOp("InitSolution[" + system + "]"),
			TimeLoopTheta{T0: t0, TMax: tMax, DT: dt, Theta: theta, Body: []Operation{
				RawOp("Generate[" + system + "]"),
				RawOp("Solve[" + system + "]"),
				RawOp("SaveSolution[" + system + "]"),
			}},
		},
	}
}
