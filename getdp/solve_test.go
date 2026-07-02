// SPDX-License-Identifier: GPL-2.0-only

package getdp

import (
	"context"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

// requireSolver skips a solver-gated test when the vendored binaries are not built
// (local: `make build-solvers`; CI: the Linux `solvers` job builds them, the plain
// test matrix skips). Returns absolute paths so tests can chdir freely.
func requireSolver(t *testing.T) solverBinaries {
	t.Helper()
	repo, err := filepath.Abs("..")
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}
	bins := solverBinaries{
		getdp: filepath.Join(repo, "vendor-src/getdp/build/getdp"),
		gmsh:  filepath.Join(repo, "vendor-src/gmsh/build/gmsh"),
	}
	for tool, p := range map[string]string{"getdp": bins.getdp, "gmsh": bins.gmsh} {
		if _, err := os.Stat(p); err != nil {
			t.Skipf("%s not built: run `make build-solvers` (%s)", tool, p)
		}
	}
	return bins
}

func TestResolveBinaryEnvFile(t *testing.T) {
	f := filepath.Join(t.TempDir(), "getdp-binary")
	if err := os.WriteFile(f, []byte("#!"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("OBK_GETDP_BIN", f)
	if got := resolveBinary("OBK_GETDP_BIN", "vendor-src/getdp/build", "getdp"); got != f {
		t.Errorf("env file: resolved %q, want %q", got, f)
	}
}

func TestResolveBinaryEnvDir(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("OBK_GETDP_BIN", dir)
	want := filepath.Join(dir, "getdp")
	if got := resolveBinary("OBK_GETDP_BIN", "vendor-src/getdp/build", "getdp"); got != want {
		t.Errorf("env dir: resolved %q, want %q", got, want)
	}
}

func TestResolveBinaryDefault(t *testing.T) {
	t.Setenv("OBK_GETDP_BIN", "")
	want := filepath.Join("vendor-src/getdp/build", "getdp")
	if got := resolveBinary("OBK_GETDP_BIN", "vendor-src/getdp/build", "getdp"); got != want {
		t.Errorf("default: resolved %q, want %q", got, want)
	}
}

func TestFindSolverBinariesMissingNamesMakeTarget(t *testing.T) {
	t.Setenv("OBK_GETDP_BIN", filepath.Join(t.TempDir(), "nowhere"))
	t.Setenv("OBK_GMSH_BIN", filepath.Join(t.TempDir(), "nowhere"))
	_, err := findSolverBinaries()
	if err == nil || !strings.Contains(err.Error(), "make build-solvers") {
		t.Errorf("error = %v, want a message naming `make build-solvers`", err)
	}
}

func TestGetdpRunArgsDeterministic(t *testing.T) {
	r := getdpRun{
		ProPath: "p.pro", MshPath: "m.msh", Resolution: "Res",
		PostOps:    []string{"Op1", "Op2"},
		SetNumbers: map[string]float64{"zeta": 3, "alpha": 1.5},
	}
	want := []string{"p.pro", "-msh", "m.msh", "-solve", "Res", "-pos", "Op1", "Op2",
		"-setnumber", "alpha", "1.5", "-setnumber", "zeta", "3", "-v", "3"}
	if got := r.args(); !reflect.DeepEqual(got, want) {
		t.Errorf("args = %v, want %v", got, want)
	}
}

func TestScrapeGetDPErrors(t *testing.T) {
	log := "Info    : Running\nError   : Unknown Resolution 'X'\nInfo    : done\n" +
		"  Error : nested report\n"
	got := scrapeGetDPErrors(log)
	if !strings.Contains(got, "Unknown Resolution") || !strings.Contains(got, "nested report") {
		t.Errorf("scraped %q, want both Error lines", got)
	}
	if scrapeGetDPErrors("Info : all good") != "" {
		t.Error("scraped an error from a clean log")
	}
}

// TestRunGetDPCancelledContext verifies the runner honors context cancellation (the
// optimizer's stop button): a pre-cancelled context must fail fast without solving.
func TestRunGetDPCancelledContext(t *testing.T) {
	bins := requireSolver(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	dir := cubeFixtureDir(t)
	_, err := runGetDP(ctx, bins.getdp, cubeRun(dir, nil))
	if err == nil {
		t.Fatal("cancelled run reported success")
	}
	if _, statErr := os.Stat(filepath.Join(dir, "vint.txt")); statErr == nil {
		t.Error("cancelled run still produced results")
	}
}

// TestRunGmshMeshesBox drives the mesher runner on the committed box fixture and
// asserts the output is an MSH 2.2 volume mesh containing tetrahedra.
func TestRunGmshMeshesBox(t *testing.T) {
	bins := requireSolver(t)
	dir := t.TempDir()
	for _, name := range []string{"box.stl", "box.geo"} {
		src, err := os.ReadFile(filepath.Join("..", "vendor-src", "gmsh", "test", name))
		if err != nil {
			t.Fatalf("read fixture %s: %v", name, err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), src, 0o644); err != nil {
			t.Fatalf("stage fixture %s: %v", name, err)
		}
	}
	out := filepath.Join(dir, "box.msh")
	if err := runGmsh(context.Background(), bins.gmsh, filepath.Join(dir, "box.geo"), out); err != nil {
		t.Fatalf("runGmsh: %v", err)
	}
	msh, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read mesh: %v", err)
	}
	if !strings.Contains(string(msh), "$MeshFormat\n2.2") {
		t.Error("output is not MSH 2.2")
	}
	if !meshHasTets(string(msh)) {
		t.Error("volume mesh contains no tetrahedra")
	}
}

// meshHasTets scans an MSH 2.2 $Elements block for a tet element (type 4 first-order
// or 11 second-order).
func meshHasTets(msh string) bool {
	inElements := false
	for _, line := range strings.Split(msh, "\n") {
		switch strings.TrimSpace(line) {
		case "$Elements":
			inElements = true
			continue
		case "$EndElements":
			return false
		}
		f := strings.Fields(line)
		if inElements && len(f) > 2 && (f[1] == "4" || f[1] == "11") {
			return true
		}
	}
	return false
}

// TestVendoredGetDPSolvesCubeSmoke drives the vendored binary end-to-end on the
// committed cube fixture: the exact solution V(z) = V0·z gives ∫v dV = V0/2 over the
// unit cube, reproduced to machine precision by first-order tets. The second run
// exercises the -setnumber fast path (V0 = 2 doubles the integral without a new deck).
func TestVendoredGetDPSolvesCubeSmoke(t *testing.T) {
	bins := requireSolver(t)
	for _, tc := range []struct {
		setNumbers map[string]float64
		want       float64
	}{
		{nil, 0.5},
		{map[string]float64{"V0": 2}, 1.0},
	} {
		dir := cubeFixtureDir(t)
		log, err := runGetDP(context.Background(), bins.getdp, cubeRun(dir, tc.setNumbers))
		if err != nil {
			t.Fatalf("solve (set=%v): %v\n%s", tc.setNumbers, err, log)
		}
		got := readTableValue(t, filepath.Join(dir, "vint.txt"))
		if math.Abs(got-tc.want) > 1e-9 {
			t.Errorf("∫v = %v (set=%v), want %v exactly", got, tc.setNumbers, tc.want)
		}
	}
}

// cubeRun builds the getdpRun for the committed cube fixture, working in dir.
func cubeRun(dir string, setNumbers map[string]float64) getdpRun {
	return getdpRun{
		ProPath: "cube.pro", MshPath: "cube.msh", Resolution: "EleKin",
		PostOps: []string{"Smoke"}, SetNumbers: setNumbers, Dir: dir,
	}
}

// cubeFixtureDir copies the committed cube fixture into a fresh temp dir so parallel
// runs and result files never touch the repo tree.
func cubeFixtureDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, name := range []string{"cube.pro", "cube.msh"} {
		src, err := os.ReadFile(filepath.Join("..", "vendor-src", "getdp", "test", name))
		if err != nil {
			t.Fatalf("read fixture %s: %v", name, err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), src, 0o644); err != nil {
			t.Fatalf("stage fixture %s: %v", name, err)
		}
	}
	return dir
}

// readTableValue parses a one-row GetDP `Format Table` file ("<step> <value>").
func readTableValue(t *testing.T, path string) float64 {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	fields := strings.Fields(string(raw))
	if len(fields) < 2 {
		t.Fatalf("table %s = %q, want `<step> <value>`", path, raw)
	}
	v, err := strconv.ParseFloat(fields[len(fields)-1], 64)
	if err != nil {
		t.Fatalf("parse table value %q: %v", fields[len(fields)-1], err)
	}
	return v
}
