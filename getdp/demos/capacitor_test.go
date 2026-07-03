// SPDX-License-Identifier: GPL-2.0-only

package demos

import (
	"testing"

	"oblikovati.org/getdp/getdp/femmodel"
)

// TestBuildCapacitorAuthorsCenteredDielectricSlab asserts the capacitor demo authors one
// parametric dielectric slab: named gap/plate parameters, a single XY sketch whose square is
// centred on the origin by the −plate_size/2 corner formulas and driven by plate_size, and one
// new-body extrude driven by the gap parameter (so the whole part is DOF=0).
func TestBuildCapacitorAuthorsCenteredDielectricSlab(t *testing.T) {
	a := &recordingAuthor{}
	if _, err := BuildCapacitor(a); err != nil {
		t.Fatalf("BuildCapacitor: %v", err)
	}
	if len(a.sketches) != 1 || a.sketches[0] != "XY" {
		t.Fatalf("sketches = %v, want one XY sketch", a.sketches)
	}
	if len(a.rectangles) != 1 {
		t.Fatalf("rectangles = %v, want one", a.rectangles)
	}
	r := a.rectangles[0]
	if r.x != "-plate_size / 2" || r.y != "-plate_size / 2" || r.w != "plate_size" || r.h != "plate_size" {
		t.Errorf("rectangle = %+v, want a plate_size square centred by -plate_size/2 corners", r)
	}
	if len(a.extrudes) != 1 || a.extrudes[0].distance != "gap" || a.extrudes[0].operation != "new" {
		t.Errorf("extrudes = %+v, want one new-body gap extrude", a.extrudes)
	}
	for _, name := range []string{"plate_size", "gap"} {
		if _, ok := a.paramExpr(name); !ok {
			t.Errorf("parameter %q not published (have %v)", name, a.params)
		}
	}
}

// TestBuildCapacitorStudyBindsPlatesAndDielectric asserts the returned study is an
// electrostatics problem with V+ / ground bound to the two plate_size² faces (the slab's
// z=0 and z=gap caps, probed at their centres), a dielectric permittivity override, an
// automatic air box, and a tighter-than-default padding so the thin capacitor stays cheap.
func TestBuildCapacitorStudyBindsPlatesAndDielectric(t *testing.T) {
	a := &recordingAuthor{}
	study, err := BuildCapacitor(a)
	if err != nil {
		t.Fatalf("BuildCapacitor: %v", err)
	}
	if study.Physics != femmodel.PhysicsElectrostatics {
		t.Fatalf("physics = %q, want electrostatics", study.Physics)
	}
	if study.Epsilon <= 1 {
		t.Errorf("epsilon = %g, want a dielectric εr > 1", study.Epsilon)
	}
	if study.AirPadding <= 0 || study.AirPadding >= 3 {
		t.Errorf("air padding = %g, want a tight positive padding under the 3× default", study.AirPadding)
	}
	if len(a.probes) != 2 {
		t.Fatalf("face probes = %v, want two plate probes", a.probes)
	}
	// Both plates are centred on the axis (x=y=0); the caps sit at z=0 and z=gap (cm).
	gnd, vplus := a.probes[0], a.probes[1]
	if gnd[0] != 0 || gnd[1] != 0 || vplus[0] != 0 || vplus[1] != 0 {
		t.Errorf("plate probes off-axis: %v %v, want centred on (0,0)", gnd, vplus)
	}
	if gnd[2] != 0 || vplus[2] <= 0 {
		t.Errorf("plate z = %g,%g, want 0 (ground) and gap>0 (V+)", gnd[2], vplus[2])
	}
	if len(study.Constraints) != 2 {
		t.Fatalf("constraints = %+v, want V+ and ground", study.Constraints)
	}
	vp, g := study.Constraints[0], study.Constraints[1]
	if vp.Kind != femmodel.KindVoltage || vp.Value != 1 || g.Kind != femmodel.KindVoltage || g.Value != 0 {
		t.Errorf("electrodes = %+v / %+v, want 1 V and 0 V voltage BCs", vp, g)
	}
}
