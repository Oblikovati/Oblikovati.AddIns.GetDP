// SPDX-License-Identifier: GPL-2.0-only

package pro

import (
	"strings"
	"testing"
)

// TestEdgeSpaceRendersHcurlForm1: the vector-potential magnetostatics space is a Form1
// H(curl) space with a single BF_Edge family over the support, whose coefficients are
// pinned by the far-field constraint on EdgesOf. UNGAUGED — there must be NO tree-cotree
// (EdgesOfTreeIn) entry (the consistent singular system is left to the iterative solver).
func TestEdgeSpaceRendersHcurlForm1(t *testing.T) {
	d := &Deck{FunctionSpaces: []FunctionSpace{EdgeSpace("Hcurl_a", "VolAll", "SetA")}}
	got := d.Render()
	const want = `FunctionSpace {
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
`
	if got != want {
		t.Errorf("edge space drifted:\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
	if strings.Contains(got, "TreeIn") || strings.Contains(got, "GaugeCondition") {
		t.Errorf("edge space must be UNGAUGED (no tree-cotree gauge), got:\n%s", got)
	}
}
