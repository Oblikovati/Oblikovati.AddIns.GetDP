// SPDX-License-Identifier: GPL-2.0-only

package pro

import (
	"strings"
	"testing"
)

// TestShellJacobiansRenderVolSphShell: an infinite-shell study maps its shell region to the
// VolSphShell transform (exterior→infinity) and every other region to the plain Vol Jacobian.
// The shell case MUST come first — GetDP applies the first matching region — and the surface
// Jacobian is unchanged.
func TestShellJacobiansRenderVolSphShell(t *testing.T) {
	d := &Deck{Jacobians: ShellJacobians("Vol3", 0.02, 0.04, [3]float64{0.01, -0.02, 0})}
	got := d.Render()
	want := `Jacobian {
  { Name JVol; Case {
      { Region Vol3; Jacobian VolSphShell{0.02, 0.04, 0.01, -0.02, 0}; }
      { Region All; Jacobian Vol; }
  } }
  { Name JSur; Case {
      { Region All; Jacobian Sur; }
  } }
}
`
	if !strings.Contains(got, want) {
		t.Errorf("shell Jacobian drifted:\n--- got ---\n%s--- want ---\n%s", got, want)
	}
}
