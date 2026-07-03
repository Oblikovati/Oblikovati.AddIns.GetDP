// SPDX-License-Identifier: GPL-2.0-only

package pro

import (
	"fmt"
	"strings"
)

// SPARSKIT algorithm and preconditioner codes (this GetDP build has SPARSKIT, no PETSc;
// the linear solver is configured through a solver.par file read from the run directory,
// see LinAlg_SPARSKIT.cpp). Only the codes the magnetostatics writer uses are named.
const (
	SolverGMRES         = 8 // Algorithm: restarted GMRES
	SolverCG            = 1 // Algorithm: conjugate gradients (symmetric systems)
	PreconditionerDiag  = 8 // Preconditioner: diagonal (Jacobi) — safe on singular operators
	PreconditionerILUTP = 2 // Preconditioner: ILUTP — faster but risks a zero pivot on a singular matrix
)

// SolverParams configures the SPARSKIT linear solver GetDP reads from a solver.par file
// in the run directory. The ungauged magnetostatics curl-curl system is symmetric
// positive SEMI-definite (consistent singular); a Krylov method (GMRES/CG) solves it and
// the field B = curl a is gauge-invariant, so the non-unique a is immaterial. A DIAGONAL
// preconditioner is the default — an ILU factorization can hit a near-zero pivot on the
// singular operator (geometry-math-advisor, #27; gauging is #40).
type SolverParams struct {
	Algorithm      int     // SPARSKIT algorithm code (SolverGMRES, SolverCG)
	Preconditioner int     // SPARSKIT preconditioner code (PreconditionerDiag, …)
	KrylovSize     int     // GMRES restart dimension (raise past 40 to clear the near-null cluster)
	MaxIter        int     // Nb_Iter_Max
	Tolerance      float64 // Stopping_Test: target relative residual
}

// DefaultMagnetostaticsSolver returns the bring-up-safe defaults for the ungauged
// vector-potential system: GMRES with a diagonal preconditioner, a large-enough Krylov
// restart to avoid stagnation on the near-null cluster, a generous iteration budget, and
// a 1e-8 relative residual (mesh error dominates the field well before that; chasing
// 1e-10 is wasteful and, if the discrete source is not exactly divergence-free,
// unreachable). See #27.
func DefaultMagnetostaticsSolver() SolverParams {
	return SolverParams{
		Algorithm:      SolverGMRES,
		Preconditioner: PreconditionerDiag,
		KrylovSize:     100,
		MaxIter:        5000,
		Tolerance:      1e-8,
	}
}

// Render returns the solver.par text: every key GetDP's SPARSKIT parser reads, with the
// five tunable knobs overriding the shipped defaults. Absent keys would fall back to
// compiled defaults, but writing the full block keeps the solver reproducible and the
// intent explicit. Format: whitespace-separated `KeyName value`, one per line.
func (p SolverParams) Render() string {
	rows := [][2]string{
		{"Matrix_Format", "1"},
		{"Matrix_Printing", "0"},
		{"Matrix_Storage", "0"},
		{"Scaling", "0"},
		{"Renumbering_Technique", "1"},
		{"Preconditioner", fmt.Sprintf("%d", p.Preconditioner)},
		{"Preconditioner_Position", "2"},
		{"Nb_Fill", "20"},
		{"Permutation_Tolerance", "0.05"},
		{"Dropping_Tolerance", "0"},
		{"Diagonal_Compensation", "0"},
		{"Re_Use_ILU", "0"},
		{"Algorithm", fmt.Sprintf("%d", p.Algorithm)},
		{"Krylov_Size", fmt.Sprintf("%d", p.KrylovSize)},
		{"IC_Acceleration", "1"},
		{"Re_Use_LU", "0"},
		{"Iterative_Improvement", "0"},
		{"Nb_Iter_Max", fmt.Sprintf("%d", p.MaxIter)},
		{"Stopping_Test", fmt.Sprintf("%g", p.Tolerance)},
	}
	var sb strings.Builder
	for _, r := range rows {
		fmt.Fprintf(&sb, "%s %s\n", r[0], r[1])
	}
	return sb.String()
}
