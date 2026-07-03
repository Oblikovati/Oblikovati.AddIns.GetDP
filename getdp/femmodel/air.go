// SPDX-License-Identifier: GPL-2.0-only

package femmodel

// AirMode is a study's air-region strategy (spec §3.3): the physics that solve fields in
// the space around the part need a meshed, truncated air domain.
type AirMode int

// Air-region strategies.
const (
	AirNone         AirMode = iota // no surrounding domain (conduction/thermal)
	AirAutomaticBox                // a padded box generated around the part shell
	AirManualBodies                // bodies explicitly assigned the Air role
)

// Truncation is how an automatic air domain closes its far boundary.
type Truncation int

// Far-boundary truncations (ABC arrives with full-wave).
const (
	TruncationPaddedBox     Truncation = iota // a plain finite box (fields must be confined)
	TruncationInfiniteShell                   // a concentric shell mapping the exterior to infinity (#25)
)

// AirRegion configures the surrounding air domain of an EM study (TP-3).
type AirRegion struct {
	Mode          AirMode
	PaddingFactor float64    // box half-extent as a multiple of the part bbox diagonal
	Truncation    Truncation // how the auto box's far boundary is closed
	ShellRint     float64    // infinite-shell inner radius (model units), #25
	ShellRext     float64    // infinite-shell outer radius (model units), #25
	Bodies        []int      // manual air-role body indexes (AirManualBodies)
}

// Padding factors per physics family (spec §3.3): confined electrostatic fields need less
// margin than the slowly-decaying fields of magnetics.
const (
	electrostaticsPadding = 3.0
	magneticsPadding      = 5.0
)

// NeedsAir reports whether a physics solves fields in the space around the part and so
// requires a meshed air region (electrostatics; magnetics/full-wave join later). Conduction
// and thermal solve only inside the part.
func NeedsAir(k PhysicsKind) bool {
	switch k {
	case PhysicsElectrostatics, PhysicsMagnetostatics:
		return true
	default:
		return false
	}
}

// defaultAir returns the air defaults for a physics: electrostatics closes with a padded box
// (its field is confined), magnetostatics with an infinite shell (its slowly-decaying field
// wants a true open boundary) and the wider magnetics padding; none for confined physics.
func defaultAir(k PhysicsKind) AirRegion {
	switch k {
	case PhysicsElectrostatics:
		return AirRegion{Mode: AirAutomaticBox, PaddingFactor: electrostaticsPadding, Truncation: TruncationPaddedBox}
	case PhysicsMagnetostatics:
		return AirRegion{Mode: AirAutomaticBox, PaddingFactor: magneticsPadding, Truncation: TruncationInfiniteShell}
	default:
		return AirRegion{Mode: AirNone}
	}
}
