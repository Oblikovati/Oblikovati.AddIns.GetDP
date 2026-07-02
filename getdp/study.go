// SPDX-License-Identifier: GPL-2.0-only

package getdp

import "errors"

// StudyResult is what one solved study reports back to the UI surfaces (status bar,
// results window) — grown field by field as the pipeline milestones land.
type StudyResult struct {
	// SummaryText is the one-line human outcome shown on the host status bar.
	SummaryText string
}

// Summary returns the one-line outcome for the host status bar.
//
//	res.Summary() // "GetDP study: 12 431 elements, peak |j| 3.2 A/mm²"
func (r *StudyResult) Summary() string { return r.SummaryText }

// RunStudyOnHost runs the active study end-to-end against the live host: selection →
// surface pull → gmsh volume mesh with physical groups → .pro deck → GetDP solve →
// .pos/table parse → client-graphics render. The pipeline lands milestone by milestone
// (M1 vendoring, M2 mesh, M3 first physics); until then it fails loudly so a ribbon
// click never silently does nothing.
func (e *Engine) RunStudyOnHost() (*StudyResult, error) {
	return nil, errors.New(
		"getdp: study pipeline not implemented yet — lands with M1 (vendored solvers) through M3 (electrokinetics)")
}
