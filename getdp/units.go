// SPDX-License-Identifier: GPL-2.0-only

package getdp

// modelUnitM is the host length unit expressed in metres: the kernel length unit is the
// centimetre (1 model unit = 0.01 m, ADR-0042 / units of measure #146). GetDP decks are
// written in pure SI, and this factor is applied in EXACTLY ONE place — the MSH writer
// scales node coordinates on the way out (design spec §3.5). Everything else (surface
// pull, gmsh meshing, face binding, render coordinates) stays in model units, so the
// viewport pipeline never converts back.
const modelUnitM = 0.01
