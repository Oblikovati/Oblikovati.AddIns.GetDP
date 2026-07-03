// SPDX-License-Identifier: GPL-2.0-only

package pro

import (
	"strings"
	"testing"
)

// TestSolverParamsRendersSolverPar: the SPARSKIT solver.par carries every key GetDP's
// parser expects, with the five magnetostatics knobs (algorithm, preconditioner, Krylov
// restart, max-iter, stopping test) overriding the SPARSKIT defaults. The format is
// whitespace-separated `KeyName value`, one per line.
func TestSolverParamsRendersSolverPar(t *testing.T) {
	p := SolverParams{Algorithm: 8, Preconditioner: 8, KrylovSize: 100, MaxIter: 5000, Tolerance: 1e-8}
	got := p.Render()
	for _, want := range []string{
		"Algorithm 8",
		"Preconditioner 8",
		"Krylov_Size 100",
		"Nb_Iter_Max 5000",
		"Stopping_Test 1e-08",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("solver.par missing %q:\n%s", want, got)
		}
	}
}

// TestSolverParamsDiagonalNotILU: the DEFAULT ungauged magnetostatics preconditioner is
// DIAGONAL (8), never an ILU factorization (2/6) — ILU risks a near-zero pivot on the
// consistent singular curl-curl operator (geometry-math-advisor, #27).
func TestSolverParamsDiagonalNotILU(t *testing.T) {
	got := DefaultMagnetostaticsSolver().Render()
	if !strings.Contains(got, "Preconditioner 8") {
		t.Errorf("default magnetostatics preconditioner must be DIAGONAL (8):\n%s", got)
	}
	if strings.Contains(got, "Algorithm 8") == false {
		t.Errorf("default magnetostatics solver must be GMRES (Algorithm 8):\n%s", got)
	}
}
