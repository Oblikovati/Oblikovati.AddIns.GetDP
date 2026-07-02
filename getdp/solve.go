// SPDX-License-Identifier: GPL-2.0-only

package getdp

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// solverBinaries locates the vendored GetDP solver and gmsh mesher the engine runs at
// arm's length (subprocess + file exchange; the engine never links them). Both are
// built from source under vendor-src/ (see vendor-src/*/NOTICE.md).
type solverBinaries struct {
	getdp string
	gmsh  string
}

// findSolverBinaries resolves the solver paths, erroring if either is missing so the
// failure names what to build rather than surfacing a cryptic exec error. Resolution:
// OBK_GETDP_BIN / OBK_GMSH_BIN (a directory or a direct path), else vendor-src/*/build.
func findSolverBinaries() (solverBinaries, error) {
	b := solverBinaries{
		getdp: resolveBinary("OBK_GETDP_BIN", "vendor-src/getdp/build", "getdp"),
		gmsh:  resolveBinary("OBK_GMSH_BIN", "vendor-src/gmsh/build", "gmsh"),
	}
	for tool, p := range map[string]string{"getdp": b.getdp, "gmsh": b.gmsh} {
		if _, err := os.Stat(p); err != nil {
			return b, fmt.Errorf("%s binary missing: %s (run `make build-solvers` or set OBK_%s_BIN): %w",
				tool, p, strings.ToUpper(tool), err)
		}
	}
	return b, nil
}

// resolveBinary returns the binary path from an env override (a file or a directory
// holding the named binary) or the in-repo build directory.
func resolveBinary(env, defaultDir, name string) string {
	dir := os.Getenv(env)
	if dir == "" {
		dir = defaultDir
	}
	if fi, err := os.Stat(dir); err == nil && !fi.IsDir() {
		return dir // env pointed straight at the binary
	}
	return filepath.Join(dir, name)
}

// runGmsh runs the mesher on geoPath, writing the MSH 2.2 mesh to outPath. -3 generates
// the 3D (volume) mesh; -format msh2 is what GetDP (and our parser) reads. The context
// kills the subprocess when cancelled (the optimizer cancels in-flight iterations).
func runGmsh(ctx context.Context, gmsh, geoPath, outPath string) error {
	cmd := exec.CommandContext(ctx, gmsh, geoPath, "-3", "-format", "msh2", "-o", outPath)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("gmsh mesh %s: %w: %s", geoPath, err, out)
	}
	return nil
}

// getdpRun is one GetDP invocation: solve a resolution on a .pro + .msh pair and run
// post-operations, with optional -setnumber constant overrides (the optimizer's fast
// path re-runs the same deck varying only these).
type getdpRun struct {
	ProPath    string             // the problem definition (absolute or Dir-relative)
	MshPath    string             // the MSH 2.2 mesh
	Resolution string             // Resolution name to -solve
	PostOps    []string           // PostOperation names to -pos (run in order)
	SetNumbers map[string]float64 // ONELAB constants overridden via -setnumber
	Dir        string             // working directory (each study gets its own)
}

// args assembles the CLI argument list. SetNumbers are emitted in sorted-key order so
// the command line is deterministic (golden tests + reproducible logs).
func (r getdpRun) args() []string {
	a := []string{r.ProPath, "-msh", r.MshPath, "-solve", r.Resolution}
	if len(r.PostOps) > 0 {
		a = append(a, "-pos")
		a = append(a, r.PostOps...)
	}
	keys := make([]string, 0, len(r.SetNumbers))
	for k := range r.SetNumbers {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		a = append(a, "-setnumber", k, strconv.FormatFloat(r.SetNumbers[k], 'g', -1, 64))
	}
	return append(a, "-v", "3")
}

// runGetDP executes one solve, returning the combined solver output (always — the
// caller may want the log even on success). GetDP reports problems as `Error : ...`
// lines and may still exit 0 on some of them, so those lines are scraped into the
// returned error either way. The context kills the subprocess when cancelled.
func runGetDP(ctx context.Context, bin string, r getdpRun) (string, error) {
	cmd := exec.CommandContext(ctx, bin, r.args()...)
	cmd.Dir = r.Dir
	out, err := cmd.CombinedOutput()
	log := string(out)
	if scraped := scrapeGetDPErrors(log); scraped != "" {
		return log, fmt.Errorf("getdp -solve %s: %s", r.Resolution, scraped)
	}
	if err != nil {
		return log, fmt.Errorf("getdp -solve %s: %w: %s", r.Resolution, err, tail(log, 400))
	}
	return log, nil
}

// scrapeGetDPErrors collects GetDP's `Error : ...` diagnostic lines from a run log.
func scrapeGetDPErrors(log string) string {
	var errs []string
	for _, line := range strings.Split(log, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "Error") {
			errs = append(errs, strings.TrimSpace(line))
		}
	}
	return strings.Join(errs, "; ")
}

// tail returns the last n bytes of s (whole string when shorter) for error context.
func tail(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return "…" + s[len(s)-n:]
}
