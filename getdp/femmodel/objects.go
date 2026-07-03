// SPDX-License-Identifier: GPL-2.0-only

package femmodel

// SolverObject holds the study's physics and solve settings (TP-1 + TP-12).
type SolverObject struct {
	Physics PhysicsKind
	// Transient time grid (thermal transient; more regimes join per milestone).
	TMax, DT, Theta float64
	// Initial field value for transient studies (starting temperature).
	Initial float64
	// Air is the surrounding-domain configuration for physics that solve fields around the
	// part (electrostatics and the EM family); zero-valued (AirNone) otherwise.
	Air AirRegion
}

// MeshObject holds the study's global mesh settings (TP-11).
type MeshObject struct {
	SizeModelUnits float64 // characteristic element length; 0 = auto from bbox
	SecondOrder    bool
}

// RegionObject scopes bodies to a material (TP-2). Bodies lists merged-mesh body
// indexes; nil means "all solid bodies not claimed by another region".
type RegionObject struct {
	ID       string
	Name     string
	Bodies   []int
	Material MaterialProps
}

// MaterialProps carries the SI volumetric properties the shipped physics read.
type MaterialProps struct {
	Name    string
	Sigma   float64 // electrical conductivity, S/m
	K       float64 // thermal conductivity, W/(m·K)
	Rho     float64 // density, kg/m³
	Cp      float64 // specific heat, J/(kg·K)
	Epsilon float64 // relative permittivity εr (electrostatics), 1 = vacuum/air
}

// ConstraintKind mirrors the engine's constraint kinds (value-identical strings).
type ConstraintKind string

// M3 constraint kinds.
const (
	KindVoltage     ConstraintKind = "voltage"
	KindCurrent     ConstraintKind = "current"
	KindTemperature ConstraintKind = "temperature"
	KindHeatFlux    ConstraintKind = "heat flux"
	KindConvection  ConstraintKind = "convection"
)

// ConstraintObject is one boundary condition / excitation intent (TP-4/TP-6): a kind,
// picked faces (host reference keys), and the kind's parameters.
type ConstraintObject struct {
	ID    string
	Name  string
	Kind  ConstraintKind
	Faces []string
	Value float64 // potential (V), temperature (K), current (A), heat rate (W)
	H     float64 // convection film coefficient, W/(m²·K)
	TInf  float64 // convection ambient, K
}

// electricKinds / thermalKinds are the per-physics constraint vocabularies (spec §4.3).
var (
	electricKinds      = []ConstraintKind{KindVoltage, KindCurrent}
	thermalKinds       = []ConstraintKind{KindTemperature, KindHeatFlux, KindConvection}
	electrostaticKinds = []ConstraintKind{KindVoltage}
)

// allowedKinds returns the constraint vocabulary of a physics.
func allowedKinds(p PhysicsKind) []ConstraintKind {
	switch p {
	case PhysicsElectrokinetics:
		return electricKinds
	case PhysicsThermalSteady, PhysicsThermalTransient:
		return thermalKinds
	case PhysicsElectrostatics:
		return electrostaticKinds
	default:
		return nil
	}
}

// kindAllowed reports whether a constraint kind applies to a physics.
func kindAllowed(p PhysicsKind, k ConstraintKind) bool {
	for _, allowed := range allowedKinds(p) {
		if allowed == k {
			return true
		}
	}
	return false
}
