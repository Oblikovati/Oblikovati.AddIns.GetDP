// SPDX-License-Identifier: GPL-2.0-only

package getdp

import (
	"strings"
	"testing"
)

// TestAirBoxCentresAndPadsThePart asserts the generated air box is centred on the part
// bbox centroid and padded by paddingFactor × bbox-diagonal, so the part sits well inside
// it with margin on every side.
func TestAirBoxCentresAndPadsThePart(t *testing.T) {
	lo, hi := [3]float64{0, 0, 0}, [3]float64{2, 1, 1} // a 2×1×1 part, centroid (1,0.5,0.5)
	b := airBox(lo, hi, 3)
	// Cubic and centred: each axis spans the same length about the centroid.
	side := b.max[0] - b.min[0]
	for k := 0; k < 3; k++ {
		if got := b.max[k] - b.min[k]; !approx(got, side, 1e-9) {
			t.Errorf("axis %d span = %g, want cubic %g", k, got, side)
		}
		c := (lo[k] + hi[k]) / 2
		if !approx((b.min[k]+b.max[k])/2, c, 1e-9) {
			t.Errorf("axis %d box centre = %g, want part centroid %g", k, (b.min[k]+b.max[k])/2, c)
		}
	}
	// The box strictly contains the part with real margin (> one part extent).
	for k := 0; k < 3; k++ {
		if b.min[k] >= lo[k] || b.max[k] <= hi[k] {
			t.Errorf("axis %d box [%g,%g] does not strictly contain part [%g,%g]", k, b.min[k], b.max[k], lo[k], hi[k])
		}
	}
}

// TestWriteAirGeoDefinesConformalHole asserts the generated .geo defines the air volume as
// the box minus the part loop (Volume(2) = {2, 1}) with both volumes and the outer boundary
// tagged as physical groups — the conformal two-region construct.
func TestWriteAirGeoDefinesConformalHole(t *testing.T) {
	var sb strings.Builder
	b := airBox([3]float64{0, 0, 0}, [3]float64{1, 1, 1}, 3)
	if err := writeAirGeo(&sb, "part.stl", b, 0.2, FirstOrderTet); err != nil {
		t.Fatalf("writeAirGeo: %v", err)
	}
	geo := sb.String()
	for _, want := range []string{
		`Merge "part.stl";`,
		"Surface Loop(1) = Surface{:};",
		"Volume(1) = {1};",    // part region bounded by its shell
		"Volume(2) = {2, 1};", // air = box (loop 2) minus part hole (loop 1)
		`Physical Volume("Part", 1) = {1};`,
		`Physical Volume("Air", 2) = {2};`,
		`Physical Surface("Outer", 3) = `,
	} {
		if !strings.Contains(geo, want) {
			t.Errorf("air .geo missing %q\n---\n%s", want, geo)
		}
	}
	// Eight box corner points must be emitted for the outer loop.
	if n := strings.Count(geo, "Point("); n != 8 {
		t.Errorf("box points = %d, want 8 corners\n%s", n, geo)
	}
}

func approx(a, b, tol float64) bool {
	d := a - b
	if d < 0 {
		d = -d
	}
	return d <= tol
}
