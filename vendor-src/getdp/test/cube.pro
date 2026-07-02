// Electrokinetic smoke problem: unit cube, sigma = 1, V = V0 (default 1) on the top face (z = 1),
// V = 0 on the bottom face (z = 0). Exact solution V(z) = z is linear, so first-order
// tets reproduce it exactly; the volume integral of v over the cube is exactly 1/2.
Group {
  Vol = Region[1];
  Top = Region[2];
  Bot = Region[3];
}

Function {
  sigma[] = 1.0;
}

// V0 is a ONELAB constant so the subprocess runner's -setnumber path is testable:
// getdp cube.pro ... -setnumber V0 2 doubles the applied voltage (and vint).
DefineConstant[ V0 = 1.0 ];

Constraint {
  { Name SetV; Case {
      { Region Top; Value V0; }
      { Region Bot; Value 0.0; }
  } }
}

Jacobian {
  { Name JVol; Case { { Region All; Jacobian Vol; } } }
}

Integration {
  { Name I1; Case { { Type Gauss; Case {
      { GeoElement Tetrahedron; NumberOfPoints 4; }
      { GeoElement Triangle; NumberOfPoints 3; }
  } } } }
}

FunctionSpace {
  { Name Hgrad_v; Type Form0;
    BasisFunction {
      { Name sn; NameOfCoef vn; Function BF_Node;
        Support Region[{Vol, Top, Bot}]; Entity NodesOf[All]; }
    }
    Constraint {
      { NameOfCoef vn; EntityType NodesOf; NameOfConstraint SetV; }
    }
  }
}

Formulation {
  { Name Electrokinetics_v; Type FemEquation;
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
  { Name EleKin; System {
      { Name A; NameOfFormulation Electrokinetics_v; }
    }
    Operation {
      Generate[A]; Solve[A]; SaveSolution[A];
    }
  }
}

PostProcessing {
  { Name EleKin_PP; NameOfFormulation Electrokinetics_v;
    Quantity {
      { Name v; Value { Term { [ {v} ]; In Vol; Jacobian JVol; } } }
      { Name vInt; Value { Integral { [ {v} ];
          In Vol; Jacobian JVol; Integration I1; } } }
    }
  }
}

PostOperation {
  { Name Smoke; NameOfPostProcessing EleKin_PP;
    Operation {
      Print[ v, OnElementsOf Vol, File "v.pos" ];
      Print[ vInt[Vol], OnGlobal, Format Table, File "vint.txt" ];
    }
  }
}
