// The oblikovati-getdp add-in: a c-shared library (.so/.dll/.dylib) loaded by the
// host at runtime, integrating GetDP as a general finite-element multiphysics
// provider (electrokinetics, thermal, electrostatics, magnetostatics,
// magnetodynamics, full-wave, elasticity, acoustics, circuit coupling) plus
// parametric sweeps and design optimization. It pulls a body's surface mesh +
// materials + selected faces from the host over the Apache-2.0 API, volume-meshes
// with a vendored mesher (gmsh), writes a GetDP .pro problem definition + MSH mesh,
// solves with a vendored headless GetDP, parses the .pos/table results, and renders
// the field maps back as client graphics. Its own module so the solver-bridge deps
// stay independent of the host — the runtime boundary is the C ABI, not Go (see
// ./include/oblikovati_addin.h).
//
// The SHIPPED library links only the Apache-2.0 contract (oblikovati.org/api). The
// require on the GPL application module (oblikovati) is TEST-SCOPE ONLY — the
// bridge↔real-host integration tests drive the live router/model. Both modules are
// sibling repos resolved by the go.work workspace at this repo's root (no committed
// replace); CI injects the equivalent replaces via .github/actions/siblings.
module oblikovati.org/getdp

go 1.24.0

require oblikovati.org/api v0.105.0
