// SPDX-License-Identifier: GPL-2.0-only

package getdp

import "testing"

// TestMagnetostaticsDeckGolden pins the exact generated magnetostatics .pro. A drift here is
// a deliberate change to the deck the solver runs — update the golden only alongside it.
func TestMagnetostaticsDeckGolden(t *testing.T) {
	deck, _, err := MagnetostaticsWriter{}.BuildDeck(magnetostaticGoldenInput())
	if err != nil {
		t.Fatalf("BuildDeck: %v", err)
	}
	got := deck.Render()
	if got != magnetostaticsDeckGolden {
		t.Errorf("magnetostatics deck drifted from golden:\n--- got ---\n%s\n--- want ---\n%s", got, magnetostaticsDeckGolden)
	}
}

const magnetostaticsDeckGolden = `Group {
  Vol1 = Region[{1}];
  Vol2 = Region[{2}];
  Vol3 = Region[{3}];
  Sur4 = Region[{4}];
  VolAll = Region[{Vol1, Vol2, Vol3}];
  DomAll = Region[{Vol1, Vol2, Vol3, Sur4}];
  VolS = Region[{Vol1}];
}
Function {
  nu[Vol1] = 795774.7150262763;
  nu[Vol2] = 795774.7150262763;
  nu[Vol3] = 795774.7150262763;
  js[Vol1] = 1000 * Unit[ Vector[0, 0, 1] /\ (XYZ[] - Vector[0, 0, 0]) ];
}
Constraint {
  { Name SetA; Case {
      { Region Sur4; Value 0; }
  } }
}
Jacobian {
  { Name JVol; Case {
      { Region Vol3; Jacobian VolSphShell{0.03, 0.06, 0, 0, 0}; }
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
  { Name Hcurl_a; Type Form1;
    BasisFunction {
      { Name se; NameOfCoef ae; Function BF_Edge;
        Support VolAll; Entity EdgesOf[All]; }
    }
    Constraint {
      { NameOfCoef ae; EntityType EdgesOf; NameOfConstraint SetA; }
    }
  }
}
Formulation {
  { Name Magnetostatics; Type FemEquation;
    Quantity {
      { Name a; Type Local; NameOfSpace Hcurl_a; }
    }
    Equation {
      Galerkin { [ nu[] * Dof{d a}, {d a} ];
        In VolAll; Jacobian JVol; Integration I1; }
      Galerkin { [ -js[], {a} ];
        In VolS; Jacobian JVol; Integration I1; }
    }
  }
}
Resolution {
  { Name Magnetostatics;
    System {
      { Name A; NameOfFormulation Magnetostatics; }
    }
    Operation {
      Generate[A];
      Solve[A];
      SaveSolution[A];
    }
  }
}
PostProcessing {
  { Name MagStaPP; NameOfFormulation Magnetostatics;
    Quantity {
      { Name b; Value { Term { [ {d a} ];
          In VolAll; Jacobian JVol; } } }
      { Name bnorm; Value { Term { [ Norm[{d a}] ];
          In VolAll; Jacobian JVol; } } }
      { Name h; Value { Term { [ nu[] * {d a} ];
          In VolAll; Jacobian JVol; } } }
      { Name a; Value { Term { [ {a} ];
          In VolAll; Jacobian JVol; } } }
      { Name energy; Value { Integral { [ 0.5 * nu[] * SquNorm[{d a}] ];
          In VolAll; Jacobian JVol; Integration I1; } } }
    }
  }
}
PostOperation {
  { Name MagStaOut; NameOfPostProcessing MagStaPP;
    Operation {
      Print[ bnorm, OnElementsOf VolAll, File "b.pos" ];
      Print[ energy[VolAll], OnGlobal, Format Table, File "energy.txt" ];
      Print[ bnorm, OnPoint {0, 0, 0}, Format Table, File "b_center.txt" ];
    }
  }
}
`
