// SPDX-License-Identifier: GPL-2.0-only

package demos

import "fmt"

// mm formats a millimetre driver as a unit-bearing host parameter expression.
func mm(v float64) string { return fmt.Sprintf("%g mm", v) }

// cm converts a millimetre dimension to host model units (1 unit = 1 cm = 10 mm), used to
// place face-probe points in the same space the host reports geometry in.
func cm(mm float64) float64 { return mm / 10 }

// publish pushes a demo's whole parameter program to the host in order (later formulas may
// reference earlier names).
func publish(a Author, params []Param) error {
	for _, p := range params {
		if err := a.Parameter(p.Name, p.Expr); err != nil {
			return fmt.Errorf("publish parameter %q=%q: %w", p.Name, p.Expr, err)
		}
	}
	return nil
}
