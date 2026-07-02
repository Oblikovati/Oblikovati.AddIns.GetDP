// SPDX-License-Identifier: GPL-2.0-only

package pro

import (
	"fmt"
	"io"
)

// PostProcessing is one named post-processing catalog over a formulation.
type PostProcessing struct {
	Name        string
	Formulation string
	Quantities  []PostQuantity
}

// PostQuantity is one derived quantity: a local Term (field map) or an Integral
// (global scalar). Expr is the bracketed density.
type PostQuantity struct {
	Name        string
	Kind        string // "Term" or "Integral"
	Expr        string
	In          string
	Jacobian    string
	Integration string // Integral only
}

// writePostProcessings emits the PostProcessing block.
func writePostProcessings(w io.Writer, ps []PostProcessing) {
	if len(ps) == 0 {
		return
	}
	fmt.Fprint(w, "PostProcessing {\n")
	for _, p := range ps {
		fmt.Fprintf(w, "  { Name %s; NameOfFormulation %s;\n    Quantity {\n", p.Name, p.Formulation)
		for _, q := range p.Quantities {
			writePostQuantity(w, q)
		}
		fmt.Fprint(w, "    }\n  }\n")
	}
	fmt.Fprint(w, "}\n")
}

// writePostQuantity emits one quantity: Term { ... } or Integral { ... }. Global-
// quantity terms carry no Jacobian (they read the assembled system directly).
func writePostQuantity(w io.Writer, q PostQuantity) {
	if q.Jacobian == "" {
		fmt.Fprintf(w, "      { Name %s; Value { %s { %s; In %s; } } }\n", q.Name, q.Kind, q.Expr, q.In)
		return
	}
	fmt.Fprintf(w, "      { Name %s; Value { %s { %s;\n          In %s; Jacobian %s;", q.Name, q.Kind, q.Expr, q.In, q.Jacobian)
	if q.Kind == "Integral" {
		fmt.Fprintf(w, " Integration %s;", q.Integration)
	}
	fmt.Fprint(w, " } } }\n")
}

// PostOperation is one named output pass over a PostProcessing.
type PostOperation struct {
	Name           string
	PostProcessing string
	Prints         []Print
}

// Print is one Print[...] statement. On selects the support ("OnElementsOf Vol",
// "OnRegion Inlet", "OnGlobal"); Format/File choose the output (.pos field map or
// Table scalar).
type Print struct {
	Quantity string
	Of       string // rendered after the quantity, e.g. "[Vol]" for integral quantities
	On       string
	Format   string // empty = default (.pos parsed field)
	File     string
}

// writePostOperations emits the PostOperation block.
func writePostOperations(w io.Writer, ps []PostOperation) {
	if len(ps) == 0 {
		return
	}
	fmt.Fprint(w, "PostOperation {\n")
	for _, p := range ps {
		fmt.Fprintf(w, "  { Name %s; NameOfPostProcessing %s;\n    Operation {\n", p.Name, p.PostProcessing)
		for _, pr := range p.Prints {
			writePrint(w, pr)
		}
		fmt.Fprint(w, "    }\n  }\n")
	}
	fmt.Fprint(w, "}\n")
}

// writePrint emits one Print statement.
func writePrint(w io.Writer, p Print) {
	fmt.Fprintf(w, "      Print[ %s%s, %s", p.Quantity, p.Of, p.On)
	if p.Format != "" {
		fmt.Fprintf(w, ", Format %s", p.Format)
	}
	fmt.Fprintf(w, ", File \"%s\" ];\n", p.File)
}
