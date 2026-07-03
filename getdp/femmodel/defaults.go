// SPDX-License-Identifier: GPL-2.0-only

package femmodel

// defaultSolver returns the TP-1 defaults of a physics (spec §4.3): static studies
// carry no time grid; thermal transient defaults to 60 s of implicit Euler at 1 s
// steps from ambient.
func defaultSolver(kind PhysicsKind) SolverObject {
	s := SolverObject{Physics: kind}
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
	if kind == PhysicsElectrokinetics {
		return MaterialProps{Name: "Copper", Sigma: 5.96e7, K: 401, Rho: 8960, Cp: 385}
	}
	return MaterialProps{Name: "Aluminium", Sigma: 3.5e7, K: 205, Rho: 2700, Cp: 900}
}
