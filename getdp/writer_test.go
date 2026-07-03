// SPDX-License-Identifier: GPL-2.0-only

package getdp

import (
	"context"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestWriterForUnknownPhysics(t *testing.T) {
	if _, err := WriterFor(PhysicsKind("warp drive")); err == nil {
		t.Error("unknown physics returned a writer")
	}
	for _, k := range []PhysicsKind{PhysicsElectrokinetics, PhysicsThermalSteady, PhysicsThermalTransient} {
		w, err := WriterFor(k)
		if err != nil || w.Physics() != k {
			t.Errorf("WriterFor(%s) = %v, %v", k, w, err)
		}
	}
}

func TestElectrokineticsRequiresVoltage(t *testing.T) {
	_, _, err := ElectrokineticsWriter{}.BuildDeck(DeckInput{
		Regions: newRegionTable([]string{"B"}), Model: &SolveModel{},
	})
	if err == nil || !strings.Contains(err.Error(), "voltage") {
		t.Errorf("err = %v, want missing-voltage failure", err)
	}
}

func TestThermalRequiresAnchor(t *testing.T) {
	_, _, err := ThermalWriter{}.BuildDeck(DeckInput{
		Regions: newRegionTable([]string{"B"}), Model: &SolveModel{},
	})
	if err == nil || !strings.Contains(err.Error(), "anchor") {
		t.Errorf("err = %v, want unanchored-field failure", err)
	}
}

// solveBoxStudy runs the shared oracle harness: boxHost mesh pipeline → given specs →
// writer deck → real GetDP; returns the study directory for result parsing. The box is
// 20×1×1 cm ⇒ L = 0.2 m, A = 1e-4 m² in the SI deck.
func solveBoxStudy(t *testing.T, kind PhysicsKind, mat Material, specs []ConstraintSpec, tr *TransientSpec) string {
	t.Helper()
	return solveConfinedStudy(t, NewEngine(newBoxHost()), []string{inletFaceKey, outletFaceKey}, 0.8, kind, mat, specs, tr)
}

// solveConfinedStudy is the generalized confined-field oracle harness: mesh the engine's
// single body at the given size, bind the named electrode faces, run the physics writer, and
// solve on real GetDP. "Confined" = no air region; unbound faces get the natural zero-flux
// Neumann BC. Returns the study directory for result parsing.
func solveConfinedStudy(t *testing.T, e *Engine, faceKeys []string, size float64,
	kind PhysicsKind, mat Material, specs []ConstraintSpec, tr *TransientSpec) string {
	t.Helper()
	bins := requireSolver(t)
	dir := t.TempDir()
	mesh, regions, rc := meshAndBind(t, e, bins, dir, faceKeys, size)
	if err := resolveSpecs(specs, rc); err != nil {
		t.Fatalf("resolve: %v", err)
	}
	writer, err := WriterFor(kind)
	if err != nil {
		t.Fatal(err)
	}
	volTag := mustVolumeTag(t, regions)
	deck, outs, err := writer.BuildDeck(DeckInput{
		Regions: regions, Model: rc.Model,
		Materials: map[int]Material{volTag: mat}, Order: 1, Transient: tr,
	})
	if err != nil {
		t.Fatalf("build deck: %v", err)
	}
	writeStudyFiles(t, dir, deck.Render(), mesh, regions)
	log, err := runGetDP(context.Background(), bins.getdp, getdpRun{
		ProPath: "study.pro", MshPath: "study.msh",
		Resolution: outs.Resolution, PostOps: outs.PostOps, Dir: dir,
	})
	if err != nil {
		t.Fatalf("getdp: %v\n%s", err, log)
	}
	return dir
}

// writeStudyFiles stages the generated deck + MSH into the study dir.
func writeStudyFiles(t *testing.T, dir, deck string, mesh *TetMesh, regions *RegionTable) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "study.pro"), []byte(deck), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := writeFile(filepath.Join(dir, "study.msh"), func(f *os.File) error { return writeMSH(f, mesh, regions) }); err != nil {
		t.Fatal(err)
	}
}

// TestElectrokineticsBusbarResistanceOracle solves the copper busbar end to end.
// Oracle (exact): R = L/(σA) = 0.2/(5.96e7·1e-4) Ω; the linear potential is exactly
// representable, so I = V/R to solver precision. |I| because the electrode facet
// orientation fixes the sign.
func TestElectrokineticsBusbarResistanceOracle(t *testing.T) {
	const sigma, vApplied = 5.96e7, 1.0
	dir := solveBoxStudy(t, PhysicsElectrokinetics, Material{Sigma: sigma}, []ConstraintSpec{
		DirichletSpec{SpecKind: KindVoltage, SpecName: "V+", FaceKeys: []string{inletFaceKey}, Value: vApplied},
		DirichletSpec{SpecKind: KindVoltage, SpecName: "GND", FaceKeys: []string{outletFaceKey}, Value: 0},
	}, nil)
	wantR := 0.2 / (sigma * 1e-4)
	gotI := math.Abs(readTableValue(t, filepath.Join(dir, "current_Sur2.txt")))
	gotR := vApplied / gotI
	if rel := math.Abs(gotR-wantR) / wantR; rel > 0.01 {
		t.Errorf("R = %.6g Ω, want %.6g Ω (rel err %.2g > 1%%)", gotR, wantR, rel)
	}
}

// TestElectrokineticsCurrentDrivenOracle drives the busbar by injected current instead:
// ground on the outlet, I₀ through the inlet terminal. Oracle (exact): the inlet
// potential must read |U| = I₀·R = I₀·L/(σA).
func TestElectrokineticsCurrentDrivenOracle(t *testing.T) {
	const sigma, injected = 5.96e7, 1000.0
	dir := solveBoxStudy(t, PhysicsElectrokinetics, Material{Sigma: sigma}, []ConstraintSpec{
		DirichletSpec{SpecKind: KindVoltage, SpecName: "GND", FaceKeys: []string{outletFaceKey}, Value: 0},
		FluxSpec{SpecKind: KindCurrent, SpecName: "feed", FaceKeys: []string{inletFaceKey}, Value: injected},
	}, nil)
	wantU := injected * 0.2 / (sigma * 1e-4)
	gotU := math.Abs(readTableValue(t, filepath.Join(dir, "voltage_Sur3.txt")))
	if rel := math.Abs(gotU-wantU) / wantU; rel > 0.01 {
		t.Errorf("U = %.6g V, want %.6g V (rel err %.2g > 1%%)", gotU, wantU, rel)
	}
}

// TestThermalSteadyWallOracle solves the 1-D wall. Oracle (exact): Q = kAΔT/L =
// 200·1e-4·100/0.2 = 10 W through the hot face.
func TestThermalSteadyWallOracle(t *testing.T) {
	dir := solveBoxStudy(t, PhysicsThermalSteady, Material{K: 200}, []ConstraintSpec{
		DirichletSpec{SpecKind: KindTemperature, SpecName: "hot", FaceKeys: []string{inletFaceKey}, Value: 400},
		DirichletSpec{SpecKind: KindTemperature, SpecName: "cold", FaceKeys: []string{outletFaceKey}, Value: 300},
	}, nil)
	gotQ := math.Abs(readTableValue(t, filepath.Join(dir, "heatrate_Sur2.txt")))
	if rel := math.Abs(gotQ-10) / 10; rel > 0.01 {
		t.Errorf("Q = %.6g W, want 10 W (rel err %.2g > 1%%)", gotQ, rel)
	}
}

// TestThermalRobinConvectionOracle replaces the cold Dirichlet with a convection film.
// Oracle (exact, series resistance): q = ΔT/(L/k + 1/h) = 100/(0.001 + 0.02) W/m²,
// Q = q·A.
func TestThermalRobinConvectionOracle(t *testing.T) {
	dir := solveBoxStudy(t, PhysicsThermalSteady, Material{K: 200}, []ConstraintSpec{
		DirichletSpec{SpecKind: KindTemperature, SpecName: "hot", FaceKeys: []string{inletFaceKey}, Value: 400},
		FluxSpec{SpecKind: KindConvection, SpecName: "film", FaceKeys: []string{outletFaceKey}, H: 50, TInf: 300},
	}, nil)
	wantQ := 100.0 / (0.2/200 + 1.0/50) * 1e-4
	gotQ := math.Abs(readTableValue(t, filepath.Join(dir, "heatrate_Sur2.txt")))
	if rel := math.Abs(gotQ-wantQ) / wantQ; rel > 0.01 {
		t.Errorf("Q = %.6g W, want %.6g W (rel err %.2g > 1%%)", gotQ, wantQ, rel)
	}
}

// TestThermalTransientSlabFourierOracle steps the suddenly-heated slab (hot end jumps
// to 400 K, far end adiabatic, uniform 300 K start) and checks the heat rate into the
// slab at t = 50 s against the analytic Fourier series
//
//	Q(t) = (kAΔT/L) · 2·Σ exp(-α((2n+1)π/2L)²·t),
//
// α = k/(ρc). Implicit Euler (θ = 1) with dt = 0.5 s holds it within 3% — Crank-
// Nicolson is avoided here because the jump initial condition excites stiff modes it
// never damps, leaving the terminal heat rate oscillating.
func TestThermalTransientSlabFourierOracle(t *testing.T) {
	const k, rho, cp = 200.0, 8000.0, 250.0 // α = 1e-4 m²/s
	dir := solveBoxStudy(t, PhysicsThermalTransient, Material{K: k, Rho: rho, Cp: cp}, []ConstraintSpec{
		DirichletSpec{SpecKind: KindTemperature, SpecName: "hot", FaceKeys: []string{inletFaceKey}, Value: 400},
	}, &TransientSpec{TMax: 50, DT: 0.5, Theta: 1, Initial: 300})
	gotQ := math.Abs(lastTableValue(t, filepath.Join(dir, "heatrate_Sur2.txt")))
	wantQ := slabHeatRate(k, k/(rho*cp), 0.2, 1e-4, 100, 50)
	if rel := math.Abs(gotQ-wantQ) / wantQ; rel > 0.03 {
		t.Errorf("Q(50s) = %.6g W, want %.6g W from the Fourier series (rel err %.2g > 3%%)", gotQ, wantQ, rel)
	}
}

// slabHeatRate evaluates the Fourier-series heat rate into a suddenly-heated slab with
// an adiabatic far end (20 terms; the series converges fast at t = 50 s).
func slabHeatRate(k, alpha, l, area, deltaT, t float64) float64 {
	sum := 0.0
	for n := 0; n < 20; n++ {
		lambda := (2*float64(n) + 1) * math.Pi / (2 * l)
		sum += math.Exp(-alpha * lambda * lambda * t)
	}
	return k * area * deltaT / l * 2 * sum
}

// lastTableValue parses the value column of the LAST row of a per-timestep Table file.
func lastTableValue(t *testing.T, path string) float64 {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	fields := strings.Fields(lines[len(lines)-1])
	if len(fields) < 2 {
		t.Fatalf("table row %q, want `<step> ... <value>`", lines[len(lines)-1])
	}
	v, err := strconv.ParseFloat(fields[len(fields)-1], 64)
	if err != nil {
		t.Fatalf("parse %q: %v", fields[len(fields)-1], err)
	}
	return v
}
