// SPDX-License-Identifier: GPL-2.0-only

package femmodel

// defaultSolver returns the TP-1 defaults of a physics (spec §4.3): static studies
// carry no time grid; thermal transient defaults to 60 s of implicit Euler at 1 s
// steps from ambient.
func defaultSolver(kind PhysicsKind) SolverObject {
	s := SolverObject{Physics: kind, Air: defaultAir(kind)}
	if kind == PhysicsThermalTransient {
		s.TMax, s.DT, s.Theta, s.Initial = 60, 1, 1, 293.15
	}
	return s
}

// defaultMesh returns the TP-11 defaults: auto element size, first-order tets.
func defaultMesh() MeshObject { return MeshObject{SizeModelUnits: 0, SecondOrder: false} }

// defaultMaterial returns the TP-2 default material of a physics: copper for
// current-conduction studies, aluminium for thermal ones.
func defaultMaterial(kind PhysicsKind) MaterialProps {
	switch kind {
	case PhysicsElectrokinetics:
		return MaterialProps{Name: "Copper", Sigma: 5.96e7, K: 401, Rho: 8960, Cp: 385}
	case PhysicsElectrostatics:
		// A unit-permittivity dielectric by default (the demo raises εr); the generated
		// air region is vacuum (εr = 1) too.
		return MaterialProps{Name: "Dielectric", Epsilon: 1}
	case PhysicsMagnetostatics:
		// Non-magnetic by default (copper coil / air, μr = 1); the demo raises μr for an
		// iron core.
		return MaterialProps{Name: "Copper", Sigma: 5.96e7, Mu: 1}
	default:
		return MaterialProps{Name: "Aluminium", Sigma: 3.5e7, K: 205, Rho: 2700, Cp: 900}
	}
}
