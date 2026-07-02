// SPDX-License-Identifier: GPL-2.0-only

package pro

import (
	"strings"
	"testing"
)

// minimalLaplaceDeck assembles the smallest complete problem the AST can express — the
// deck-level golden of issue #11. Field names mirror the hand-written cube fixture the
// vendored solver validated in M1, so the shapes are known-parseable.
func minimalLaplaceDeck() *Deck {
	return &Deck{
		Constants: []Constant{{Name: "V0", Value: 1}},
		Groups: []Group{
			{Name: "Vol", Regions: []int{1}},
			{Name: "Inlet", Regions: []int{2}},
			{Name: "Outlet", Regions: []int{3}},
			{Name: "Dom", SubGroups: []string{"Vol", "Inlet", "Outlet"}},
		},
		Functions: []Function{{Name: "sigma", Region: "Vol", Expr: "1.0"}},
		Constraints: []Constraint{{Name: "SetV", Cases: []ConstraintCase{
			{Region: "Inlet", Value: "V0"},
			{Region: "Outlet", Value: "0"},
		}}},
		Jacobians:      StandardJacobians(),
		Integrations:   StandardIntegration(1),
		FunctionSpaces: []FunctionSpace{NodalSpace("Hgrad_v", "Dom", "SetV")},
		Formulations: []Formulation{{
			Name: "EleKin", Type: "FemEquation",
			Quantities: []Quantity{{Name: "v", Type: "Local", Space: "Hgrad_v"}},
			Equations: []EquationTerm{{
				Kind: "Galerkin", Expr: "[ sigma[] * Dof{d v}, {d v} ]",
				In: "Vol", Jacobian: JacVolName, Integration: IntName,
			}},
		}},
		Resolutions: []Resolution{StaticResolution("R", "A", "EleKin")},
		PostProcessings: []PostProcessing{{
			Name: "PP", Formulation: "EleKin",
			Quantities: []PostQuantity{
				{Name: "v", Kind: "Term", Expr: "[ {v} ]", In: "Vol", Jacobian: JacVolName},
				{Name: "vol", Kind: "Integral", Expr: "[ 1 ]", In: "Vol", Jacobian: JacVolName, Integration: IntName},
			},
		}},
		PostOperations: []PostOperation{{
			Name: "Out", PostProcessing: "PP",
			Prints: []Print{
				{Quantity: "v", On: "OnElementsOf Vol", File: "v.pos"},
				{Quantity: "vol", Of: "[Vol]", On: "OnGlobal", Format: "Table", File: "vol.txt"},
			},
		}},
	}
}

const goldenLaplace = `DefineConstant[
  V0 = 1,
];
Group {
  Vol = Region[{1}];
  Inlet = Region[{2}];
  Outlet = Region[{3}];
  Dom = Region[{Vol, Inlet, Outlet}];
}
Function {
  sigma[Vol] = 1.0;
}
Constraint {
  { Name SetV; Case {
      { Region Inlet; Value V0; }
      { Region Outlet; Value 0; }
  } }
}
Jacobian {
  { Name JVol; Case {
      { Region All; Jacobian Vol; }
  } }
  { Name JSur; Case {
      { Region All; Jacobian Sur; }
  } }
}
Integration {
  { Name I1; Case { { Type Gauss; Case {
      { GeoElement Point; NumberOfPoints 1; }
      { GeoElement Line; NumberOfPoints 3; }
      { GeoElement Triangle; NumberOfPoints 3; }
      { GeoElement Tetrahedron; NumberOfPoints 4; }
  } } } }
}
FunctionSpace {
  { Name Hgrad_v; Type Form0;
    BasisFunction {
      { Name sn; NameOfCoef vn; Function BF_Node;
        Support Dom; Entity NodesOf[All]; }
    }
    Constraint {
      { NameOfCoef vn; EntityType NodesOf; NameOfConstraint SetV; }
    }
  }
}
Formulation {
  { Name EleKin; Type FemEquation;
    Quantity {
      { Name v; Type Local; NameOfSpace Hgrad_v; }
    }
    Equation {
      Galerkin { [ sigma[] * Dof{d v}, {d v} ];
        In Vol; Jacobian JVol; Integration I1; }
    }
  }
}
Resolution {
  { Name R;
    System {
      { Name A; NameOfFormulation EleKin; }
    }
    Operation {
      Generate[A];
      Solve[A];
      SaveSolution[A];
    }
  }
}
PostProcessing {
  { Name PP; NameOfFormulation EleKin;
    Quantity {
      { Name v; Value { Term { [ {v} ];
          In Vol; Jacobian JVol; } } }
      { Name vol; Value { Integral { [ 1 ];
          In Vol; Jacobian JVol; Integration I1; } } }
    }
  }
}
PostOperation {
  { Name Out; NameOfPostProcessing PP;
    Operation {
      Print[ v, OnElementsOf Vol, File "v.pos" ];
      Print[ vol[Vol], OnGlobal, Format Table, File "vol.txt" ];
    }
  }
}
`

func TestDeckGoldenMinimalLaplace(t *testing.T) {
	got := minimalLaplaceDeck().Render()
	if got != goldenLaplace {
		t.Errorf("deck drifted from golden:\n--- got ---\n%s--- want ---\n%s", got, goldenLaplace)
	}
}

func TestEmptyBlocksAreOmitted(t *testing.T) {
	d := &Deck{Groups: []Group{{Name: "Vol", Regions: []int{1}}}}
	got := d.Render()
	for _, block := range []string{"Function", "Constraint", "Jacobian", "Integration",
		"FunctionSpace", "Formulation", "Resolution", "PostProcessing", "PostOperation", "DefineConstant"} {
		if strings.Contains(got, block) {
			t.Errorf("empty block %q was emitted:\n%s", block, got)
		}
	}
}

func TestThetaResolutionGolden(t *testing.T) {
	r := ThetaResolution("T", "A", "Heat", "0", "tMax", "dt", "theta")
	d := &Deck{Resolutions: []Resolution{r}}
	want := `Resolution {
  { Name T;
    System {
      { Name A; NameOfFormulation Heat; }
    }
    Operation {
      InitSolution[A];
      TimeLoopTheta[0, tMax, dt, theta] {
        Generate[A];
        Solve[A];
        SaveSolution[A];
      }
    }
  }
}
`
	if got := d.Render(); got != want {
		t.Errorf("theta resolution drifted:\n--- got ---\n%s--- want ---\n%s", got, want)
	}
}

func TestHarmonicSystemCarriesFrequency(t *testing.T) {
	d := &Deck{Resolutions: []Resolution{{
		Name:       "R",
		Systems:    []System{{Name: "A", Formulation: "MagDyn", Frequency: "freq"}},
		Operations: []Operation{RawOp("Generate[A]")},
	}}}
	if got := d.Render(); !strings.Contains(got, "Type ComplexValue; Frequency freq;") {
		t.Errorf("harmonic system missing frequency:\n%s", got)
	}
}
