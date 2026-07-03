// SPDX-License-Identifier: GPL-2.0-only

package demos

import "oblikovati.org/getdp/getdp/femmodel"

// Parallel-plate capacitor demo dimensions (mm) and material. A square dielectric slab whose
// two large faces are the plates: the field solves in the slab AND the surrounding air, so the
// reported capacitance sits above the ideal C = ε₀·εr·A/d by the fringing the air captures.
// This is the geometry family the optimization walkthrough reuses because C(gap) is analytic.
const (
	capPlateMM   = 40.0 // plate side (X and Y)
	capGapMM     = 6.0  // plate separation = dielectric thickness (Z)
	capEr        = 4.0  // dielectric relative permittivity (a glass-epoxy-ish εr)
	capVoltageV  = 1.0  // applied plate potential
	capAirPad    = 1.5  // tight air box: a thin capacitor in the 3× default box is needlessly costly
	capMeshUnits = 0.25 // 2.5 mm elements — a few across the 6 mm gap, enough for the flood plot
)

// CapacitorParams is the demo's published GEOMETRIC parameter program: the plate side and the
// gap, as unit-bearing literals. Editing either re-drives the slab (C tracks gap⁻¹). The
// dielectric εr and the applied voltage are study/material values, not host parameters — the
// host parameter engine carries only geometric units (length/angle/count).
func CapacitorParams() []Param {
	return []Param{
		{Name: "plate_size", Expr: mm(capPlateMM), Note: "Plate side length (X and Y)"},
		{Name: "gap", Expr: mm(capGapMM), Note: "Plate separation / dielectric thickness (Z)"},
	}
}

// BuildCapacitor authors a single parametric dielectric slab centred on the origin and returns
// its electrostatics study: the two plate faces held at V+ and ground, the slab given a
// dielectric permittivity, and an automatic air box (the field radiates into the surrounding
// air). The square is centred by the −plate_size/2 corner formulas and driven by plate_size;
// the +Z extrude is driven by gap, so the part is DOF=0 and recomputes on any parameter edit.
func BuildCapacitor(a Author) (Study, error) {
	if err := publish(a, CapacitorParams()); err != nil {
		return Study{}, err
	}
	slab := rect{x: "-plate_size / 2", y: "-plate_size / 2", w: "plate_size", h: "plate_size"}
	if err := extrudeRectangle(a, slab, "gap", "new"); err != nil {
		return Study{}, err
	}
	return capacitorStudy(a)
}

// capacitorStudy binds ground / V+ to the slab's two plate_size² caps, probed at their centres
// (the slab is centred on the axis, caps at z=0 and z=gap in model units), and carries the
// dielectric and the tight air box.
func capacitorStudy(a Author) (Study, error) {
	ground, err := a.FaceKeyAt([3]float64{0, 0, 0})
	if err != nil {
		return Study{}, err
	}
	vplus, err := a.FaceKeyAt([3]float64{0, 0, cm(capGapMM)})
	if err != nil {
		return Study{}, err
	}
	return Study{
		Physics: femmodel.PhysicsElectrostatics,
		Constraints: []femmodel.ConstraintObject{
			{Name: "V+", Kind: femmodel.KindVoltage, Faces: []string{vplus}, Value: capVoltageV},
			{Name: "Ground", Kind: femmodel.KindVoltage, Faces: []string{ground}, Value: 0},
		},
		Epsilon:        capEr,
		AirPadding:     capAirPad,
		MeshModelUnits: capMeshUnits,
	}, nil
}
