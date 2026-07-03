// SPDX-License-Identifier: GPL-2.0-only

package getdp

import (
	"testing"
)

// electrostaticGoldenInput builds a deterministic two-region electrostatics deck input by
// hand (no host, no solver): a dielectric part volume (εr = 4) surrounded by an air volume
// (εr = 1), an electrode surface at 1 V and the far-field boundary at 0 V. It exercises
// every branch the writer has — per-volume ε, the Dirichlet block, the energy/C/Q post —
// so the golden below pins the exact .pro the runner ships.
func electrostaticGoldenInput() DeckInput {
	regions := &RegionTable{
		Volumes: []VolumeRegion{
			{Tag: 1, Name: "Dielectric", Body: 0},
			{Tag: 2, Name: "Air", Body: airBodyIndex},
		},
		Surfaces: []SurfaceRegion{
			{Tag: 3, Name: "V+"},
			{Tag: 4, Name: "FarField"},
		},
	}
	model := &SolveModel{BoundPotentials: []BoundPotential{
		{Kind: KindVoltage, RegionTag: 3, Name: "V+", Value: 1},
		{Kind: KindVoltage, RegionTag: 4, Name: "FarField", Value: 0},
	}}
	return DeckInput{
		Regions:   regions,
		Model:     model,
		Materials: map[int]Material{1: {Epsilon: 4}, 2: {Epsilon: 1}},
		Order:     1,
	}
}

// TestElectrostaticsDeckGolden pins the exact generated electrostatics .pro. A drift here
// is a deliberate change to the deck the solver runs — update the golden only alongside it.
func TestElectrostaticsDeckGolden(t *testing.T) {
	deck, _, err := ElectrostaticsWriter{}.BuildDeck(electrostaticGoldenInput())
	if err != nil {
		t.Fatalf("BuildDeck: %v", err)
	}
	got := deck.Render()
	if got != electrostaticsDeckGolden {
		t.Errorf("electrostatics deck drifted from golden:\n--- got ---\n%s\n--- want ---\n%s", got, electrostaticsDeckGolden)
	}
}

const electrostaticsDeckGolden = `Group {
  Vol1 = Region[{1}];
  Vol2 = Region[{2}];
  Sur3 = Region[{3}];
  Sur4 = Region[{4}];
  VolAll = Region[{Vol1, Vol2}];
  DomAll = Region[{Vol1, Vol2, Sur3, Sur4}];
}
Function {
  eps[Vol1] = 3.5416751251200002e-11;
  eps[Vol2] = 8.8541878128000006e-12;
}
Constraint {
  { Name SetV; Case {
      { Region Sur3; Value 1; }
      { Region Sur4; Value 0; }
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
        Support VolAll; Entity NodesOf[All]; }
    }
    Constraint {
      { NameOfCoef vn; EntityType NodesOf; NameOfConstraint SetV; }
    }
  }
}
Formulation {
  { Name Electrostatics; Type FemEquation;
    Quantity {
      { Name v; Type Local; NameOfSpace Hgrad_v; }
    }
    Equation {
      Galerkin { [ eps[] * Dof{d v}, {d v} ];
        In VolAll; Jacobian JVol; Integration I1; }
    }
  }
}
Resolution {
  { Name Electrostatics;
    System {
      { Name A; NameOfFormulation Electrostatics; }
    }
    Operation {
      Generate[A];
      Solve[A];
      SaveSolution[A];
    }
  }
}
PostProcessing {
  { Name EleStaPP; NameOfFormulation Electrostatics;
    Quantity {
      { Name v; Value { Term { [ {v} ];
          In VolAll; Jacobian JVol; } } }
      { Name e; Value { Term { [ -{d v} ];
          In VolAll; Jacobian JVol; } } }
      { Name energy; Value { Integral { [ 0.5 * eps[] * SquNorm[{d v}] ];
          In VolAll; Jacobian JVol; Integration I1; } } }
      { Name capacitance; Value { Integral { [ eps[] * SquNorm[{d v}] / 1 ];
          In VolAll; Jacobian JVol; Integration I1; } } }
      { Name charge; Value { Integral { [ eps[] * SquNorm[{d v}] / 1 ];
          In VolAll; Jacobian JVol; Integration I1; } } }
    }
  }
}
PostOperation {
  { Name EleStaOut; NameOfPostProcessing EleStaPP;
    Operation {
      Print[ v, OnElementsOf VolAll, File "v.pos" ];
      Print[ e, OnElementsOf VolAll, File "e.pos" ];
      Print[ energy[VolAll], OnGlobal, Format Table, File "energy.txt" ];
      Print[ capacitance[VolAll], OnGlobal, Format Table, File "capacitance.txt" ];
      Print[ charge[VolAll], OnGlobal, Format Table, File "charge.txt" ];
    }
  }
}
`
