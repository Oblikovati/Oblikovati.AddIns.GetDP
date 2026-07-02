// SPDX-License-Identifier: GPL-2.0-only

package pro

import (
	"fmt"
	"io"
)

// Formulation is one named weak form: quantities over spaces plus equation terms.
type Formulation struct {
	Name       string
	Type       string // "FemEquation"
	Quantities []Quantity
	Equations  []EquationTerm
}

// Quantity declares one unknown/test field of a formulation. Global quantities name
// the space quantity they alias in brackets: `NameOfSpace Hgrad_v [I]`.
type Quantity struct {
	Name          string
	Type          string // "Local", "Global", "Integral"
	Space         string
	SpaceQuantity string // Global only: the space's GlobalQuantity name
}

// EquationTerm is one weak-form term. Kind is the GetDP term keyword; Expr is the
// bracketed density (leaf math owned by the physics writer). GlobalTerm rows leave
// Jacobian/Integration empty.
type EquationTerm struct {
	Kind        string // "Galerkin", "GlobalTerm"
	Expr        string // e.g. "[ sigma[] * Dof{d v}, {d v} ]"
	In          string
	Jacobian    string
	Integration string
}

// writeFormulations emits the Formulation block.
func writeFormulations(w io.Writer, fs []Formulation) {
	if len(fs) == 0 {
		return
	}
	fmt.Fprint(w, "Formulation {\n")
	for _, f := range fs {
		writeOneFormulation(w, f)
	}
	fmt.Fprint(w, "}\n")
}

// writeOneFormulation emits one formulation object.
func writeOneFormulation(w io.Writer, f Formulation) {
	fmt.Fprintf(w, "  { Name %s; Type %s;\n    Quantity {\n", f.Name, f.Type)
	for _, q := range f.Quantities {
		space := q.Space
		if q.SpaceQuantity != "" {
			space += " [" + q.SpaceQuantity + "]"
		}
		fmt.Fprintf(w, "      { Name %s; Type %s; NameOfSpace %s; }\n", q.Name, q.Type, space)
	}
	fmt.Fprint(w, "    }\n    Equation {\n")
	for _, e := range f.Equations {
		writeEquationTerm(w, e)
	}
	fmt.Fprint(w, "    }\n  }\n")
}

// writeEquationTerm emits one term; GlobalTerm rows carry no Jacobian/Integration.
func writeEquationTerm(w io.Writer, e EquationTerm) {
	if e.Jacobian == "" {
		fmt.Fprintf(w, "      %s { %s; In %s; }\n", e.Kind, e.Expr, e.In)
		return
	}
	fmt.Fprintf(w, "      %s { %s;\n        In %s; Jacobian %s; Integration %s; }\n",
		e.Kind, e.Expr, e.In, e.Jacobian, e.Integration)
}

// Resolution is one named solve recipe: systems plus an operation sequence.
type Resolution struct {
	Name       string
	Systems    []System
	Operations []Operation
}

// System binds one linear system to a formulation. Frequency (when set) makes the
// system complex-harmonic.
type System struct {
	Name        string
	Formulation string
	Frequency   string // empty = real static/transient system
}

// Operation is one resolution step. Raw ops cover the simple verbs; structured ops
// (theta time loop, nonlinear iteration) compose nested steps.
type Operation interface {
	writeOp(w io.Writer, indent string)
}

// RawOp is a verbatim operation statement, e.g. `Generate[A]`, `Solve[A]`.
type RawOp string

func (r RawOp) writeOp(w io.Writer, indent string) { fmt.Fprintf(w, "%s%s;\n", indent, string(r)) }

// TimeLoopTheta advances the system with the theta scheme from T0 to Tmax.
type TimeLoopTheta struct {
	T0, TMax, DT string // expressions so constants can drive them
	Theta        string
	Body         []Operation
}

func (t TimeLoopTheta) writeOp(w io.Writer, indent string) {
	fmt.Fprintf(w, "%sTimeLoopTheta[%s, %s, %s, %s] {\n", indent, t.T0, t.TMax, t.DT, t.Theta)
	for _, op := range t.Body {
		op.writeOp(w, indent+"  ")
	}
	fmt.Fprintf(w, "%s}\n", indent)
}

// IterativeLoop repeats its body until the relaxed solution moves less than Criterion
// (the nonlinear Picard/Newton driver; the writer supplies GenerateJac/SolveJac ops).
type IterativeLoop struct {
	MaxIter    int
	Criterion  string
	Relaxation string
	Body       []Operation
}

func (l IterativeLoop) writeOp(w io.Writer, indent string) {
	fmt.Fprintf(w, "%sIterativeLoop[%d, %s, %s] {\n", indent, l.MaxIter, l.Criterion, l.Relaxation)
	for _, op := range l.Body {
		op.writeOp(w, indent+"  ")
	}
	fmt.Fprintf(w, "%s}\n", indent)
}

// writeResolutions emits the Resolution block.
func writeResolutions(w io.Writer, rs []Resolution) {
	if len(rs) == 0 {
		return
	}
	fmt.Fprint(w, "Resolution {\n")
	for _, r := range rs {
		writeOneResolution(w, r)
	}
	fmt.Fprint(w, "}\n")
}

// writeOneResolution emits one resolution object.
func writeOneResolution(w io.Writer, r Resolution) {
	fmt.Fprintf(w, "  { Name %s;\n    System {\n", r.Name)
	for _, s := range r.Systems {
		fmt.Fprintf(w, "      { Name %s; NameOfFormulation %s;", s.Name, s.Formulation)
		if s.Frequency != "" {
			fmt.Fprintf(w, " Type ComplexValue; Frequency %s;", s.Frequency)
		}
		fmt.Fprint(w, " }\n")
	}
	fmt.Fprint(w, "    }\n    Operation {\n")
	for _, op := range r.Operations {
		op.writeOp(w, "      ")
	}
	fmt.Fprint(w, "    }\n  }\n")
}
