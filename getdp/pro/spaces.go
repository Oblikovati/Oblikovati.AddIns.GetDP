// SPDX-License-Identifier: GPL-2.0-only

package pro

import (
	"fmt"
	"io"
)

// Jacobian is one named Jacobian method with per-region cases.
type Jacobian struct {
	Name  string
	Cases []JacobianCase
}

// JacobianCase applies one Jacobian expression to a region ("All" is customary).
type JacobianCase struct {
	Region   string
	Jacobian string // "Vol", "Sur", "VolSphShell{...}" — written verbatim
}

// writeJacobians emits the Jacobian block.
func writeJacobians(w io.Writer, js []Jacobian) {
	if len(js) == 0 {
		return
	}
	fmt.Fprint(w, "Jacobian {\n")
	for _, j := range js {
		fmt.Fprintf(w, "  { Name %s; Case {\n", j.Name)
		for _, c := range j.Cases {
			fmt.Fprintf(w, "      { Region %s; Jacobian %s; }\n", c.Region, c.Jacobian)
		}
		fmt.Fprint(w, "  } }\n")
	}
	fmt.Fprint(w, "}\n")
}

// Integration is one named Gauss integration rule: points per geometric element type.
type Integration struct {
	Name  string
	Gauss []GaussCase
}

// GaussCase sets the point count for one element geometry.
type GaussCase struct {
	GeoElement string // "Tetrahedron", "Triangle", "Line", "Point"
	Points     int
}

// writeIntegrations emits the Integration block.
func writeIntegrations(w io.Writer, is []Integration) {
	if len(is) == 0 {
		return
	}
	fmt.Fprint(w, "Integration {\n")
	for _, in := range is {
		fmt.Fprintf(w, "  { Name %s; Case { { Type Gauss; Case {\n", in.Name)
		for _, g := range in.Gauss {
			fmt.Fprintf(w, "      { GeoElement %s; NumberOfPoints %d; }\n", g.GeoElement, g.Points)
		}
		fmt.Fprint(w, "  } } } }\n")
	}
	fmt.Fprint(w, "}\n")
}

// FunctionSpace is one named FE space: basis functions, global (terminal) quantities,
// and the constraints pinning coefficients.
type FunctionSpace struct {
	Name             string
	Type             string // "Form0" (nodal), "Form1" (edge), ...
	BasisFunctions   []BasisFunction
	GlobalQuantities []GlobalQuantity
	SpaceConstraints []SpaceConstraint
}

// GlobalQuantity exposes a terminal pair on a space: AliasOf a coefficient family (the
// terminal value, e.g. electrode potential) or AssociatedWith it (the dual flux the
// assembled system carries exactly, e.g. electrode current — how GetDP extracts
// terminal quantities without surface-gradient integrals).
type GlobalQuantity struct {
	Name string
	Type string // "AliasOf" or "AssociatedWith"
	Coef string
}

// BasisFunction is one basis-function family of a space.
type BasisFunction struct {
	Name    string
	Coef    string
	Func    string // "BF_Node", "BF_Edge", ...
	Support string
	Entity  string // "NodesOf[All]", "EdgesOf[All]", ...
}

// SpaceConstraint ties a coefficient family to a named Constraint object.
type SpaceConstraint struct {
	Coef       string
	EntityType string // "NodesOf", "EdgesOf", ...
	Constraint string
}

// writeFunctionSpaces emits the FunctionSpace block.
func writeFunctionSpaces(w io.Writer, fs []FunctionSpace) {
	if len(fs) == 0 {
		return
	}
	fmt.Fprint(w, "FunctionSpace {\n")
	for _, s := range fs {
		writeOneSpace(w, s)
	}
	fmt.Fprint(w, "}\n")
}

// writeOneSpace emits one FunctionSpace object.
func writeOneSpace(w io.Writer, s FunctionSpace) {
	fmt.Fprintf(w, "  { Name %s; Type %s;\n    BasisFunction {\n", s.Name, s.Type)
	for _, b := range s.BasisFunctions {
		fmt.Fprintf(w, "      { Name %s; NameOfCoef %s; Function %s;\n        Support %s; Entity %s; }\n",
			b.Name, b.Coef, b.Func, b.Support, b.Entity)
	}
	fmt.Fprint(w, "    }\n")
	writeGlobalQuantities(w, s.GlobalQuantities)
	writeSpaceConstraints(w, s.SpaceConstraints)
	fmt.Fprint(w, "  }\n")
}

// writeGlobalQuantities emits a space's GlobalQuantity sub-block when present.
func writeGlobalQuantities(w io.Writer, qs []GlobalQuantity) {
	if len(qs) == 0 {
		return
	}
	fmt.Fprint(w, "    GlobalQuantity {\n")
	for _, q := range qs {
		fmt.Fprintf(w, "      { Name %s; Type %s; NameOfCoef %s; }\n", q.Name, q.Type, q.Coef)
	}
	fmt.Fprint(w, "    }\n")
}

// writeSpaceConstraints emits a space's Constraint sub-block when present.
func writeSpaceConstraints(w io.Writer, cs []SpaceConstraint) {
	if len(cs) == 0 {
		return
	}
	fmt.Fprint(w, "    Constraint {\n")
	for _, c := range cs {
		fmt.Fprintf(w, "      { NameOfCoef %s; EntityType %s; NameOfConstraint %s; }\n",
			c.Coef, c.EntityType, c.Constraint)
	}
	fmt.Fprint(w, "    }\n")
}
