// SPDX-License-Identifier: GPL-2.0-only

package demos

import (
	"fmt"

	"oblikovati.org/getdp/getdp/femmodel"
)

// HeatSinkFinCount is the demo's fin count, exported so host-path tests and the README can
// reference it without duplicating the literal.
const HeatSinkFinCount = 5

// Heat-sink demo dimensions (mm) and thermal drivers. A plate-fin aluminium block: an
// 80×40 mm, 5 mm base carrying five 3×25 mm fins on a 14 mm pitch. The base bottom is
// held hot and each fin top rejects heat by convection — a steady conduction+convection
// field with a clear base→tip temperature drop. Fin SIDES are left adiabatic (a modelling
// simplification; the finned tops are the dominant rejection surface for a demo).
const (
	heatSinkBaseWidthMM     = 80.0
	heatSinkBaseThicknessMM = 5.0
	heatSinkDepthMM         = 40.0
	heatSinkFinWidthMM      = 3.0
	heatSinkFinHeightMM     = 25.0
	heatSinkFinPitchMM      = 14.0
	heatSinkFinCount        = HeatSinkFinCount

	heatSinkHotK      = 353.15 // 80 °C base
	heatSinkAmbientK  = 293.15 // 20 °C ambient
	heatSinkFilmH     = 25.0   // W/(m²·K) natural-convection film
	heatSinkMeshUnits = 0.3    // 3 mm elements — resolves the thin fins
)

// HeatSinkParams is the demo's published GEOMETRIC parameter program. Independent drivers
// are literals; fin_x0 is a FORMULA centring the fin array on the base, so editing the base
// width, fin count, pitch or width re-centres the fins (derived-dimension discipline). The
// thermal drivers (temperatures, film coefficient) are study values, not host parameters —
// the host parameter engine only carries geometric units.
func HeatSinkParams() []Param {
	return []Param{
		{Name: "base_width", Expr: mm(heatSinkBaseWidthMM), Note: "Base slab width (X)"},
		{Name: "base_thickness", Expr: mm(heatSinkBaseThicknessMM), Note: "Base slab thickness (Y)"},
		{Name: "sink_depth", Expr: mm(heatSinkDepthMM), Note: "Extrusion depth (Z)"},
		{Name: "fin_width", Expr: mm(heatSinkFinWidthMM), Note: "Single fin width (X)"},
		{Name: "fin_height", Expr: mm(heatSinkFinHeightMM), Note: "Single fin height (Y)"},
		{Name: "fin_count", Expr: fmt.Sprintf("%d", heatSinkFinCount), Note: "Number of fins (linear array)"},
		{Name: "fin_pitch", Expr: mm(heatSinkFinPitchMM), Note: "Fin-to-fin spacing (X)"},
		{Name: "fin_x0", Expr: "(base_width - ((fin_count - 1) * fin_pitch + fin_width)) / 2",
			Note: "First-fin offset — centres the array (derived)"},
	}
}

// BuildHeatSink authors the parametric plate-fin block and returns its steady-thermal
// study. The base slab is a new body; one fin is joined onto its top and replicated into a
// fin_count-long array along +X. The bottom face is held at hot_temperature and every fin
// top carries a convection film to ambient.
func BuildHeatSink(a Author) (Study, error) {
	if err := publish(a, HeatSinkParams()); err != nil {
		return Study{}, err
	}
	if err := extrudeRectangle(a, rect{"0", "0", "base_width", "base_thickness"}, "sink_depth", "new"); err != nil {
		return Study{}, fmt.Errorf("base slab: %w", err)
	}
	finFeature, err := extrudeRectangleFeature(a, rect{"fin_x0", "base_thickness", "fin_width", "fin_height"}, "sink_depth", "join")
	if err != nil {
		return Study{}, fmt.Errorf("fin: %w", err)
	}
	if err := a.PatternX(finFeature, heatSinkFinCount, "fin_count", cm(heatSinkFinPitchMM)); err != nil {
		return Study{}, fmt.Errorf("fin array: %w", err)
	}
	return heatSinkStudy(a)
}

// heatSinkStudy binds the hot base temperature to the bottom face and a convection film to
// every fin top, probed at the analytically-known centroids (model units).
func heatSinkStudy(a Author) (Study, error) {
	depthMid := cm(heatSinkDepthMM) / 2
	bottom, err := a.FaceKeyAt([3]float64{cm(heatSinkBaseWidthMM) / 2, 0, depthMid})
	if err != nil {
		return Study{}, err
	}
	constraints := []femmodel.ConstraintObject{
		{Name: "Hot base", Kind: femmodel.KindTemperature, Faces: []string{bottom}, Value: heatSinkHotK},
	}
	topY := cm(heatSinkBaseThicknessMM + heatSinkFinHeightMM)
	for i := 0; i < heatSinkFinCount; i++ {
		key, err := a.FaceKeyAt([3]float64{finCenterCm(i), topY, depthMid})
		if err != nil {
			return Study{}, err
		}
		constraints = append(constraints, femmodel.ConstraintObject{
			Name: fmt.Sprintf("Fin %d cooling", i+1), Kind: femmodel.KindConvection,
			Faces: []string{key}, H: heatSinkFilmH, TInf: heatSinkAmbientK,
		})
	}
	return Study{Physics: femmodel.PhysicsThermalSteady, Constraints: constraints, MeshModelUnits: heatSinkMeshUnits}, nil
}

// finCenterCm is the X centre (model units) of fin i: the centred first-fin offset plus i
// pitches plus half the fin width — the same geometry the pattern lays down.
func finCenterCm(i int) float64 {
	x0mm := (heatSinkBaseWidthMM - (float64(heatSinkFinCount-1)*heatSinkFinPitchMM + heatSinkFinWidthMM)) / 2
	return cm(x0mm + float64(i)*heatSinkFinPitchMM + heatSinkFinWidthMM/2)
}
