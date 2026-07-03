// SPDX-License-Identifier: GPL-2.0-only

package demos

import "oblikovati.org/getdp/getdp/femmodel"

// Busbar demo dimensions (mm). A 200×10×10 mm copper bar: a length long enough that the
// field is a clean axial gradient and a 1 cm² cross-section, so the electrode current
// matches the exact Ohm oracle I = V·σ·A/L (see the engine's busbar oracle test).
const (
	busbarLengthMM  = 200.0
	busbarWidthMM   = 10.0
	busbarHeightMM  = 10.0
	busbarVoltageV  = 1.0
	busbarMeshUnits = 0.5 // 5 mm elements — a few across the section, plenty for the planar solve
)

// BusbarParams is the demo's published GEOMETRIC parameter program: independent driver
// dimensions as unit-bearing literals. Editing any of them re-drives the bar. The applied
// voltage is a study value (busbarVoltageV), not a host parameter — the host parameter
// engine only carries geometric units (length/angle/count).
func BusbarParams() []Param {
	return []Param{
		{Name: "bar_length", Expr: mm(busbarLengthMM), Note: "Bar length (current-flow axis, +Z)"},
		{Name: "bar_width", Expr: mm(busbarWidthMM), Note: "Cross-section width (X)"},
		{Name: "bar_height", Expr: mm(busbarHeightMM), Note: "Cross-section height (Y)"},
	}
}

// BuildBusbar authors a single parametric copper bar and returns its electrokinetics
// study: a fixed potential on one end cap and ground on the other, so current conducts
// axially. The bar's XY rectangle is anchored at the origin and driven by the width/height
// parameters; the +Z extrude is driven by the length parameter, so the whole part is DOF=0
// and recomputes when a parameter changes.
func BuildBusbar(a Author) (Study, error) {
	if err := publish(a, BusbarParams()); err != nil {
		return Study{}, err
	}
	if err := extrudeRectangle(a, rect{"0", "0", "bar_width", "bar_height"}, "bar_length", "new"); err != nil {
		return Study{}, err
	}
	return busbarStudy(a)
}

// busbarStudy binds V+ / ground to the two extrude-cap faces, probed at the cap centroids
// in model units (mm→cm): the caps sit at z=0 and z=length, centred on the section.
func busbarStudy(a Author) (Study, error) {
	cx, cy := cm(busbarWidthMM)/2, cm(busbarHeightMM)/2
	inlet, err := a.FaceKeyAt([3]float64{cx, cy, 0})
	if err != nil {
		return Study{}, err
	}
	outlet, err := a.FaceKeyAt([3]float64{cx, cy, cm(busbarLengthMM)})
	if err != nil {
		return Study{}, err
	}
	return Study{
		Physics: femmodel.PhysicsElectrokinetics,
		Constraints: []femmodel.ConstraintObject{
			{Name: "V+", Kind: femmodel.KindVoltage, Faces: []string{inlet}, Value: busbarVoltageV},
			{Name: "Ground", Kind: femmodel.KindVoltage, Faces: []string{outlet}, Value: 0},
		},
		MeshModelUnits: busbarMeshUnits,
	}, nil
}
