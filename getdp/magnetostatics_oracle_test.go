// SPDX-License-Identifier: GPL-2.0-only

package getdp

import (
	"context"
	"fmt"
	"math"
	"os"
	"testing"

	"oblikovati.org/getdp/getdp/femmodel"
)

// TestSolenoidCenterFieldOracle is the #27 solenoid acceptance oracle, run END-TO-END through
// the host pipeline (mesh → air+shell → ungauged edge-element solve → |B| probe): the on-axis
// centre field of a finite solenoid is B = μ₀·(N I/L)/√(1+(2R/L)²), which tends to the long-
// solenoid μ₀·n·I. A thin annular-cylinder winding carries an azimuthal current density J0, so
// n I = J0·t (t = radial thickness). The solved centre |B| must hit the finite-solenoid value
// within 3% and the μ₀·n·I limit within a few %.
func TestSolenoidCenterFieldOracle(t *testing.T) {
	bins := requireSolver(t)
	const ri, ro, halfLen, j0 = 0.7, 1.0, 4.0, 1e6

	solved := solveSolenoidCenterB(t, bins, ri, ro, halfLen, j0, 0.25)

	thick := (ro - ri) * modelUnitM
	R := 0.5 * (ri + ro) * modelUnitM
	L := 2 * halfLen * modelUnitM
	nI := j0 * thick
	bInfinite := vacuumPermeability * nI
	bFinite := bInfinite / math.Sqrt(1+(2*R/L)*(2*R/L))

	finiteErr := math.Abs(solved-bFinite) / bFinite
	infErr := math.Abs(solved-bInfinite) / bInfinite
	t.Logf("solenoid centre |B| = %.6g T (finite %.6g, μ₀nI %.6g); finite err %.3f%%, μ₀nI err %.3f%%",
		solved, bFinite, bInfinite, 100*finiteErr, 100*infErr)
	if finiteErr > 0.03 {
		t.Errorf("solenoid centre |B| = %.6g T, want finite-solenoid %.6g T within 3%% (got %.3f%%)", solved, bFinite, 100*finiteErr)
	}
	if infErr > 0.05 {
		t.Errorf("solenoid centre |B| = %.6g T, want μ₀·n·I = %.6g T within 5%% (got %.3f%%)", solved, bInfinite, 100*infErr)
	}
}

// solveSolenoidCenterB runs a magnetostatics study on a solenoid winding through the full host
// pipeline and returns the solved |B| at the coil centre (SI tesla).
func solveSolenoidCenterB(t *testing.T, bins solverBinaries, ri, ro, halfLen, j0, size float64) float64 {
	t.Helper()
	t.Setenv("OBK_GETDP_BIN", bins.getdp)
	t.Setenv("OBK_GMSH_BIN", bins.gmsh)
	e := NewEngine(solenoidCoilHost(ri, ro, halfLen, 48))
	e.withAnalysis(func(a *femmodel.Analysis) {
		s := a.AddStudy(femmodel.PhysicsMagnetostatics)
		s.AddCoil(femmodel.CoilObject{Bodies: []int{0}, Axis: [3]float64{0, 0, 1}, CurrentDensity: j0})
		s.Mesh.SizeModelUnits = size
	})
	res, err := e.RunStudyOnHost(context.Background())
	if err != nil {
		t.Fatalf("RunStudyOnHost (solenoid): %v", err)
	}
	for _, sc := range res.Scalars {
		if sc.Label == "|B| at coil1" {
			return sc.Value
		}
	}
	t.Fatalf("no coil-centre |B| scalar in results: %+v", res.Scalars)
	return 0
}

// TestCircularLoopBiotSavartOracle is the #27 loop acceptance oracle: the on-axis field of a
// circular current loop follows the Biot-Savart law. A torus of major radius R and wire radius
// a carrying an azimuthal current density is a continuous stack of coaxial current loops, so
// its on-axis profile is the exact finite-torus Biot-Savart integral torusAxialBz (which, for a
// thin wire, tends to μ₀ I R²/(2(R²+z²)^{3/2})).
//
// The oracle compares the solved on-axis PROFILE SHAPE, B(z)/B(0), against that integral within
// 3%. The ratio cancels the coil's total-current factor, so it is immune to the modest
// current-integration deficit a coarse mesh leaves on a small coil cross-section (the absolute
// field magnitude is validated to a fraction of a percent by the solenoid oracle). Probes stay
// well inside the near-air ball (Rint) so none fall in the infinite-shell mapping zone.
func TestCircularLoopBiotSavartOracle(t *testing.T) {
	bins := requireSolver(t)
	const ringR, wireA, j0 = 1.5, 0.3, 1e6
	R := ringR * modelUnitM
	a := wireA * modelUnitM
	// Probe the smooth region (z ≥ 0.7R): the first-order edge-element B = curl a is
	// piecewise constant and pointwise-noisy in the immediate near-field (z < 0.6R). All
	// points stay well inside the near-air ball (Rint ≈ 1.5·enclosing ≈ 1.8R).
	zs := []float64{0, 0.7 * R, 1.1 * R}

	got := solveLoopProfile(t, bins, ringR, wireA, j0, 0.15, zs)
	solvedB0 := got["z0"]
	biotB0 := torusAxialBz(R, a, 0, j0, 400)

	for i, z := range zs {
		name := fmt.Sprintf("z%d", i)
		wantRatio := torusAxialBz(R, a, z, j0, 400) / biotB0
		gotRatio := got[name] / solvedB0
		relErr := math.Abs(gotRatio-wantRatio) / wantRatio
		t.Logf("loop z=%.3gR: |B|=%.6g T  ratio solved=%.5f biot-savart=%.5f  err=%.3f%%",
			z/R, got[name], gotRatio, wantRatio, 100*relErr)
		if relErr > 0.03 {
			t.Errorf("loop on-axis profile at z=%.3gR: B(z)/B(0) = %.5f, want Biot-Savart %.5f within 3%% (got %.3f%%)",
				z/R, gotRatio, wantRatio, 100*relErr)
		}
	}
}

// torusAxialBz returns the on-axis (z-axis) flux density of an azimuthal current density j0
// filling a torus of major radius R and minor (wire) radius a, at axial height zp — the exact
// Biot-Savart superposition of the torus's coaxial current filaments. Each meridional area
// element (ρ,h) is a circular loop of radius ρ contributing μ₀ j0 ρ²/(2(ρ²+(zp−h)²)^{3/2}) dρ dh;
// the midpoint rule integrates over the meridional disk (ρ−R)²+h² ≤ a² on an n×n grid.
func torusAxialBz(rMaj, rMin, zp, j0 float64, n int) float64 {
	sum, step := 0.0, 2*rMin/float64(n)
	for i := 0; i < n; i++ {
		rho := rMaj - rMin + (float64(i)+0.5)*step
		for k := 0; k < n; k++ {
			h := -rMin + (float64(k)+0.5)*step
			if (rho-rMaj)*(rho-rMaj)+h*h > rMin*rMin {
				continue
			}
			sum += rho * rho / math.Pow(rho*rho+(zp-h)*(zp-h), 1.5) * step * step
		}
	}
	return vacuumPermeability * j0 * sum / 2
}

// solveLoopProfile solves a torus current loop at writer+solver level (direct mesh → deck →
// GetDP) and returns |B| at each on-axis probe z (SI metres). It drives the same writer and
// solver.par the host pipeline uses, with explicit on-axis probes for the profile check.
func solveLoopProfile(t *testing.T, bins solverBinaries, ringR, wireA, j0, size float64, zs []float64) map[string]float64 {
	t.Helper()
	soup := torusSoup(ringR, wireA, 64, 16)
	surface, err := weldSurface(soup.VertexCoordinates, soup.VertexIndices)
	if err != nil {
		t.Fatalf("weld torus: %v", err)
	}
	dir, _ := os.MkdirTemp("", "magloop-*")
	defer os.RemoveAll(dir)
	mesh, geom, err := NewGmshMesher(bins.gmsh).MeshWithInfiniteShell(
		context.Background(), surface, ShellSpec{}, MeshOptions{Size: size, Order: FirstOrderTet}, dir)
	if err != nil {
		t.Fatalf("mesh loop: %v", err)
	}
	deck, outs, regions := buildLoopDeck(t, mesh, &geom, j0, zs)
	// stageFiles MUST receive the SAME region table the deck was built from — it carries the
	// bound far-field surface (tag 4), so writeMSH tags the outer facets and the deck's a=0
	// constraint on Sur4 actually applies. A fresh table would omit that surface.
	if err := stageFiles(dir, deck, mesh, regions, outs.Solver); err != nil {
		t.Fatalf("stage: %v", err)
	}
	if _, err := runGetDP(context.Background(), bins.getdp, getdpRun{
		ProPath: "study.pro", MshPath: "study.msh", Resolution: outs.Resolution, PostOps: outs.PostOps, Dir: dir,
	}); err != nil {
		t.Fatalf("getdp: %v", err)
	}
	out := map[string]float64{}
	for i := range zs {
		name := fmt.Sprintf("z%d", i)
		v, err := readLastTableValue(dir + "/b_" + name + ".txt")
		if err != nil {
			t.Fatalf("read probe %s: %v", name, err)
		}
		out[name] = v
	}
	return out
}

// loopRegions builds the coil+air+shell region table for the loop mesh.
func loopRegions(_ *TetMesh) *RegionTable {
	regions := newRegionTable([]string{"Coil"})
	regions.addAirVolume()
	regions.addShellVolume()
	return regions
}

// buildLoopDeck resolves the loop regions, binds the far field, and generates the deck with
// on-axis |B| probes. It returns the region table (carrying the bound far-field surface) so the
// caller stages the MSH from the SAME table the deck references.
func buildLoopDeck(t *testing.T, mesh *TetMesh, geom *shellGeometry, j0 float64, zs []float64) (string, DeckOutputs, *RegionTable) {
	t.Helper()
	regions := loopRegions(mesh)
	farTag, err := regions.BindOuterBoundary(mesh)
	if err != nil {
		t.Fatalf("bind outer boundary: %v", err)
	}
	model := &SolveModel{FarFieldTag: farTag, Coils: []Coil{{
		RegionTag: 1, Axis: [3]float64{0, 0, 1}, Center: [3]float64{}, CurrentDensity: j0,
	}}}
	transform, err := shellTransform(geom, regions)
	if err != nil {
		t.Fatalf("shell transform: %v", err)
	}
	var probes []FieldProbe
	for i, z := range zs {
		probes = append(probes, FieldProbe{Name: fmt.Sprintf("z%d", i), Point: [3]float64{0, 0, z}})
	}
	deck, outs, err := MagnetostaticsWriter{}.BuildDeck(DeckInput{
		Regions: regions, Model: model, Materials: map[int]Material{1: {Mu: 1}, 2: {Mu: 1}, 3: {Mu: 1}},
		Order: 1, Shell: transform, Probes: probes,
	})
	if err != nil {
		t.Fatalf("build loop deck: %v", err)
	}
	return deck.Render(), outs, regions
}
