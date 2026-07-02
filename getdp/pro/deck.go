// SPDX-License-Identifier: GPL-2.0-only

// Package pro generates GetDP .pro problem definitions from typed Go values — the full
// deck is built here (ADR: no runtime Include of upstream template files; upstream
// templates serve only as test oracles). The AST is structural: blocks and objects are
// typed and ordered for byte-stable golden decks, while leaf math expressions
// ("sigma[] * Dof{d v}") are strings owned by the per-physics writers.
package pro

import (
	"fmt"
	"io"
	"strings"
)

// Deck is one complete GetDP problem definition, rendered block by block in GetDP's
// customary order. Every block is optional; empty blocks are omitted entirely.
type Deck struct {
	// Constants are emitted first as DefineConstant[...] so every tunable in the deck
	// can be overridden per run with -setnumber (the optimizer's fast path).
	Constants       []Constant
	Groups          []Group
	Functions       []Function
	Constraints     []Constraint
	Jacobians       []Jacobian
	Integrations    []Integration
	FunctionSpaces  []FunctionSpace
	Formulations    []Formulation
	Resolutions     []Resolution
	PostProcessings []PostProcessing
	PostOperations  []PostOperation
}

// Render returns the deck as .pro text.
//
//	deck := pro.Deck{Groups: []pro.Group{{Name: "Vol", Regions: []int{1}}}}
//	text := deck.Render()
func (d *Deck) Render() string {
	var sb strings.Builder
	d.WriteTo(&sb)
	return sb.String()
}

// WriteTo renders the deck into w in deterministic block order.
func (d *Deck) WriteTo(w io.Writer) {
	writeConstants(w, d.Constants)
	writeGroups(w, d.Groups)
	writeFunctions(w, d.Functions)
	writeConstraints(w, d.Constraints)
	writeJacobians(w, d.Jacobians)
	writeIntegrations(w, d.Integrations)
	writeFunctionSpaces(w, d.FunctionSpaces)
	writeFormulations(w, d.Formulations)
	writeResolutions(w, d.Resolutions)
	writePostProcessings(w, d.PostProcessings)
	writePostOperations(w, d.PostOperations)
}

// Constant is one -setnumber-overridable named number.
type Constant struct {
	Name  string
	Value float64
}

// writeConstants emits one DefineConstant block for all tunables.
func writeConstants(w io.Writer, cs []Constant) {
	if len(cs) == 0 {
		return
	}
	fmt.Fprint(w, "DefineConstant[\n")
	for _, c := range cs {
		fmt.Fprintf(w, "  %s = %.17g,\n", c.Name, c.Value)
	}
	fmt.Fprint(w, "];\n")
}

// Group names a region set: `Name = Region[{tags}];`. Groups may also reference other
// groups by name (SubGroups), e.g. `Dom = Region[{Vol, Air}];`.
type Group struct {
	Name      string
	Regions   []int
	SubGroups []string
}

// writeGroups emits the Group block.
func writeGroups(w io.Writer, gs []Group) {
	if len(gs) == 0 {
		return
	}
	fmt.Fprint(w, "Group {\n")
	for _, g := range gs {
		fmt.Fprintf(w, "  %s = Region[{%s}];\n", g.Name, g.regionList())
	}
	fmt.Fprint(w, "}\n")
}

// regionList renders the group's numeric tags and sub-group names, tags first.
func (g Group) regionList() string {
	parts := make([]string, 0, len(g.Regions)+len(g.SubGroups))
	for _, r := range g.Regions {
		parts = append(parts, fmt.Sprintf("%d", r))
	}
	parts = append(parts, g.SubGroups...)
	return strings.Join(parts, ", ")
}

// Function defines one material property / source expression, optionally region-scoped:
// `Name[Region] = Expr;` (or `Name[] = Expr;` for global).
type Function struct {
	Name   string
	Region string // empty = global ("[]")
	Expr   string
}

// writeFunctions emits the Function block.
func writeFunctions(w io.Writer, fs []Function) {
	if len(fs) == 0 {
		return
	}
	fmt.Fprint(w, "Function {\n")
	for _, f := range fs {
		fmt.Fprintf(w, "  %s[%s] = %s;\n", f.Name, f.Region, f.Expr)
	}
	fmt.Fprint(w, "}\n")
}

// Constraint is one named constraint with per-region cases (Dirichlet values, initial
// conditions, …). Value is an expression so it can reference constants.
type Constraint struct {
	Name  string
	Type  string // empty for Assign (the default); "Init" for initial conditions
	Cases []ConstraintCase
}

// ConstraintCase fixes Value on one region group.
type ConstraintCase struct {
	Region string
	Value  string
}

// writeConstraints emits the Constraint block.
func writeConstraints(w io.Writer, cs []Constraint) {
	if len(cs) == 0 {
		return
	}
	fmt.Fprint(w, "Constraint {\n")
	for _, c := range cs {
		writeOneConstraint(w, c)
	}
	fmt.Fprint(w, "}\n")
}

// writeOneConstraint emits one constraint object with its cases.
func writeOneConstraint(w io.Writer, c Constraint) {
	fmt.Fprintf(w, "  { Name %s;", c.Name)
	if c.Type != "" {
		fmt.Fprintf(w, " Type %s;", c.Type)
	}
	fmt.Fprint(w, " Case {\n")
	for _, cc := range c.Cases {
		fmt.Fprintf(w, "      { Region %s; Value %s; }\n", cc.Region, cc.Value)
	}
	fmt.Fprint(w, "  } }\n")
}
