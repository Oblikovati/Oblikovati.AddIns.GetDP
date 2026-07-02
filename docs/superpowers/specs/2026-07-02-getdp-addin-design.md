# Oblikovati.AddIns.GetDP — design specification

Status: APPROVED (2026-07-02) · Author: Claude + vmiguel

## 1. Goal

Ship GetDP 3.5.0 (GPL, ONELAB) as an Oblikovati solver add-in, module
`oblikovati.org/getdp`, cloned structurally from the CalculiX add-in (the reference
implementation and quality standard). GetDP's differentiated value versus the CalculiX
add-in is **electromagnetics** — electrostatics, magnetostatics with nonlinear B-H,
magnetodynamics/eddy currents in frequency and time domain, full-wave — plus circuit
coupling and coupled magneto-thermal, alongside thermal, elasticity and acoustics.

All GetDP physics templates become first-class UI studies; users never hand-write `.pro`
files. A dedicated **"GetDP" ribbon tab** and a coherent multi-window UI (browser study
tree + modal task-panel editors + focused dockables — never one mega-panel) expose every
simulation option. A **design-optimization mode** (parametric sweep + derivative-free
single-objective optimizer over document parameters) is a first-class feature. The GetDP
reference-manual tutorials drive live tests and fully-parametric (DOF=0) demo documents.

## 2. Confirmed decisions

- **D1** — Independent add-in; clones the CalculiX add-in architecture. No dependency on
  any other solver add-in effort.
- **D2** — ALL GetDP physics templates exposed as UI studies, rolled out over milestones.
- **D3** — Optimization = grid sweep + Nelder-Mead/golden-section over document
  parameters; objective = any post-processing scalar; bounds; results table; apply-best.
- **D4** — Vendor GetDP built from source (`-DENABLE_PETSC=0 -DENABLE_SLEPC=0
  -DENABLE_SPARSKIT=1`; bundled SPARSKIT + Arpack from `contrib/` — self-contained, no
  PETSc). Copy the gmsh vendoring recipe into this repo (add-ins are self-contained).

## 3. Architecture

GPL-2.0-only c-shared library over the host C ABI; links only the Apache-2.0
`oblikovati.org/api` module (v0.102.1 at design time). Solvers run **only as
subprocesses** (arm's-length file exchange); `gplpurity` guards the link graph.

```text
/                     cgo c-shared shell: export.go, hostcaller.go, manifest.json
                      (id com.oblikovati.getdp; capabilities incl. "parameters"), Makefile
getdp/                cgo-free engine: engine.go, commands.go, ribbon_layout.go, panel.go,
                      analysis_tree.go, study.go, solve.go, render.go, units.go,
                      hostmesh/surfacemesh/volumemesh/mshparse (ported mesh pipeline),
                      mshwriter.go (NEW), airregion.go (NEW), posparse.go + tableparse.go
                      (NEW), constraintspec.go + constraint_*.go, femmodel/ aggregate
getdp/pro/            .pro writer: typed Deck AST (Group/Function/Constraint/FunctionSpace/
                      Jacobian/Integration/Formulation/Resolution/PostProcessing/
                      PostOperation) + one PhysicsWriter per study kind
getdp/opt/            pure optimizer package (GridSweep, NelderMead, GoldenSection)
vendor-src/{getdp,gmsh}/  committed source + build.sh + NOTICE.md + fixtures
gplpurity/  demos/  architecture/decisions/  .github/{workflows,actions}
```

Per-study pipeline: host facets + materials + selections → weld → gmsh tet mesh (+ air
region for EM) → FaceKey↔physical-group bind → Go-owned `model.msh` (MSH 2.2, physical
groups) + generated `study.pro` → `getdp study.pro -msh model.msh -solve <Res>
-pos <PostOp> [-setnumber …]` → `.pos`/Table parse → flood plot + global scalars.

### 3.1 `.pro` generation — full Go typed-AST (ADR-0002)

Generate the complete `.pro` from Go builders over a typed AST; do **not** `Include` the
GetDP template library. Rationale: golden-deck testability, "all options exposed"
(templates hide formulation choices behind macro flags), hermetic two-file subprocess
input. Shared fragments (Jacobian `Vol`/`Sur`/`VolSphShell`, Gauss tables, gauge blocks,
theta time-loop resolutions) are composable helpers. Two registries mirror the reference
add-in: `PhysicsWriter` (one file per physics) and `ConstraintSpec`/`ConstraintWriter`
(intent seam resolving face refs → physical tags; deck seam emitting `Group`/`Constraint`
entries). Upstream tutorials are **test oracles run through the vendored binary**, never
runtime templates.

### 3.2 Mesh & physical groups (ADR-0005)

Port the proven pipeline (facets → weld → STL + `.geo` → gmsh `-3` → tet parse; multibody
surface loops), then write the GetDP-input `model.msh` from the in-memory mesh with a
**Go-owned MSH 2.2 writer**: `$PhysicalNames`, one Physical Volume per body (+ air), one
Physical Surface per bound constraint face group, **only referenced groups emitted**
(sidesteps the `Mesh.SaveAll` MSH2 pitfall). Face binding reuses the geometric
centroid + normal ≥ 0.9 match. `.pro` `Group { Region {N} }` blocks reference the tags.

### 3.3 Air region (ADR-0003)

Needed (meshed, truncated) for electrostatics, magnetostatics, magnetodynamics,
full-wave; not needed for electrokinetics, thermal (Robin convection on the surface),
elasticity, cavity acoustics. Studies declare `NeedsAir()`.

- **Auto (default)**: padded box around the part shell — the classified closed shell
  becomes a hole in a generated outer box volume, one conformal gmsh run, both volumes
  tagged. Padding factor per physics (5× bbox diagonal magnetics, 3× electrostatics).
- **Explicit fallback**: a body with the "Air" role in the study scope.
- **Infinite shell**: optional second concentric layer with `Jacobian VolSphShell`;
  `SolverObject.Truncation = padded box | infinite shell | ABC (full-wave)`.

This is the riskiest mesh step: dedicated cube-in-box fixture, and failure messages point
to the explicit-air-body path.

### 3.4 3D-first; axisymmetric later (ADR-0006)

All studies ship as 3D tet formulations first. 3D magnetics starts **ungauged edge
elements + SPARSKIT GMRES** (consistent singular systems solve fine iteratively);
tree-cotree/Coulomb gauging is a later option. Axisymmetric 2D studies from a selected
planar face (2D msh writer + axi Jacobians) are their own late milestone. Until then,
demos are 3D equivalents of the tutorials validated against analytic oracles.

### 3.5 Units (ADR-0004)

Kernel unit is cm. SI meters are applied **only** in `mshwriter.go`
(`modelUnitM = 0.01`); `.pro` files are pure SI; no `-msh_scaling`; the render path uses
original model-unit coordinates.

### 3.6 Results

- **Field maps**: `Print[…, OnElementsOf …, Format Gmsh]` → `posparse.go` (SP/VP/SS/VS/
  ST/VT records; values bind to mesh nodes by coordinate) → flood plot + color-mapper
  legend. Vector fields render as magnitude floods first; arrow glyphs are polish.
- **Global scalars**: every `PhysicsWriter` publishes `[]ScalarQuantity{Name, Unit,
  PostOpName}` printed `Format Table` → `tableparse.go` → status line, Results window,
  **and** the optimizer objective registry (one mechanism, three consumers).
- **Harmonic**: re/im views; display modes magnitude/phase (animate later); Joule loss
  and time-averaged quantities printed as separate real views.

### 3.7 Optimization (ADR-0007)

- `getdp/opt` is pure (no host imports): `Problem{Vars, Sense, Eval}`; `GridSweep`,
  `NelderMead` (bounded), `GoldenSection`; unit-tested on analytic optima.
- Runner mirrors the study-launch goroutine discipline. Two variable kinds:
  **DocumentParameter** (`Parameters().Set` → recompute → re-pull → remesh per
  evaluation) and **SolverConstant** (`-setnumber` only — no remesh; fast path for
  excitations, material values, frequency).
- Memoized evaluations on rounded vectors; `context.Context` cancellation with
  subprocess kill; parameter snapshot restored on finish/cancel; **Apply Best Design**
  is an explicit command.

## 4. UI design

Command namespace `GetDP.*`; window/pane IDs `com.oblikovati.getdp.*`. Principles:
ribbon order = workflow order; one surface per concern; family editors with a Kind
dropdown (never a dialog per sub-kind, never a mega-window); physics-scoped dropdowns;
geometry assignment always via `PanelReferenceList`; physical values always via
unit-bearing `PanelValueEditor`; every BC/excitation draws a canvas glyph.

### 4.1 Ribbon tab "GetDP" (Part + Assembly ribbons)

Implemented as an exhaustive `command → {panel, icon, style, kind, items}` map guarded by
tests. Panels left→right:

| Panel | Buttons (Large first) |
|---|---|
| Study | **New Study** (flyout: Electrostatics, Electrokinetics, Magnetostatics, Magnetodynamics, Full-Wave, Thermal, Elasticity, Acoustics, Magneto-Thermal) · Study Settings · Duplicate Study · Set Active Study (flyout) |
| Setup | **Regions** · **Boundary Condition** (flyout, physics-filtered kinds) · **Excitation** (flyout, physics-filtered) · Air Region · Circuit Coupling · Materials Library |
| Mesh | **Generate Mesh** · Mesh Settings · Local Refinement · Show Mesh (toggle) |
| Solve | **Run Study** · Solver Settings · Run All Studies · Stop |
| Results | **Field Plot** (flyout, physics fields) · Global Quantities · Phase/Animate · Export Results · Clear Plot |
| Optimization | **Parameter Sweep** · **Optimize** · Optimization Results |
| Windows | Study Tree · Run Monitor · Results · Optimization (all small toggles) |
| Help | **Demo Studies** (flyout of bundled demos) · GetDP Help |

Every command carries Tooltip/TooltipTitle/TooltipExpanded describing its workflow step.

### 4.2 Surfaces

- **Browser tree pane** (`…tree`) — the study model. N studies per document, one
  *active* (bold/✓). Per study: Physics & Regime, Regions (leaf per region), Boundary
  Conditions, Excitations, Mesh, Solver, Post-processing, Results (+ a document-level
  Optimization node). Node label suffixes carry state (`[!]` blocking, `[solved 14:02]`,
  `[out of date]`, `[error]`). Single-click highlights geometry + glyphs; double-click
  opens the matching editor; context menus mirror ribbon verbs.
- **Run Monitor** dockable (`…monitor`, bottom) — queue rows, progress, nonlinear
  residual readout, log tail, Stop. Read-only; auto-shows on Run; mirrors to status bar.
- **Results** dockable (`…results`, right) — study/field/component dropdowns, harmonic
  complex-display group (magnitude/real/imag/phase + phase slider), transient time
  slider, legend controls (log, min/max, colormap), global-quantities rows, export.
- **Optimization** dockable (`…optim`, right) — live header (n/N, best-so-far), results
  table (★ best row), Apply Best / Apply Selected / Export CSV, Show Plot (web dialog
  with convergence or parameter-vs-objective curves).
- **Modal TaskPanels** — 14 family editors (section 4.3).

New-user path: New Study → TP-1 → tree appears populated with defaults (auto air proposal
for EM, default mesh/solver); blocking nodes show `[!]`; Run with missing prerequisites
opens a task panel listing them.

### 4.3 TaskPanel catalog

Conventions: title `<Verb> <Object> — <Study>`; red error label + OK refusal while
invalid; one `PanelReferenceList` per geometry-bearing editor; two-column `PanelGrid`
rows inside `PanelGroup`s.

- **TP-1 `study`** — name; Physics group (study type, formulation — physics-contextual:
  magnetostatics `[vector potential a | scalar potential φ]`, magnetodynamics
  `[a-v | h-φ]`, full-wave `[E-field edge]`; dimension `[3D]`, axi later); Regime group
  (Static/Harmonic/Transient; frequency; end time + step + theta scheme); Coupling group
  (magneto-thermal: one-way | iterative two-way + tolerance).
- **TP-2 `region`** — bodies list; role (Conductor/Dielectric/Coil/Core/Magnet/Air/
  Solid, physics-filtered); material combo (library + Custom); property-override group,
  physics-filtered: εr, σ; μr, B-H model (linear | analytic Brauer | curve) + params,
  Br + magnetization direction (magnets); k, ρ, cp; E, ν; c, ρ0. Validation: every body
  in exactly one region; role×physics compatibility.
- **TP-3 `airregion`** — mode (Automatic box | Manual bodies | None); padding slider;
  infinite-shell checkbox + inner/outer radii; manual air-bodies list; hint label.
- **TP-4 `bc_electric`** — kind (Voltage, Ground, Floating Potential, Surface Charge,
  Zero-Normal-D symmetry, Normal Current Density) + faces + kind params (V + phase for
  harmonic; q; float sub-kind + Q; jn); auto label.
- **TP-5 `bc_magnetic`** — kind (Flux Tangent n·B=0, Flux Normal H×n=0, Fixed Vector
  Potential, Applied H) + faces + params.
- **TP-6 `bc_thermal`** — kind (Temperature, Heat Flux, Convection, Radiation, Periodic
  Pair, Adiabatic) + faces (+ paired faces for periodic) + params (T; q; h + T∞;
  emissivity + T∞).
- **TP-7 `bc_mech`** — kind (Fixed, Prescribed Displacement, Pressure, Force, Symmetry)
  + faces + params (per-axis u with free checkboxes; p; F + direction).
- **TP-8 `bc_wave`** — full-wave kinds (Port + mode + power, PEC, PMC, Absorbing ABC);
  acoustics kinds (Pressure, Normal Velocity, Impedance, Absorbing).
- **TP-9 `excitation`** — kind physics-filtered (Coil Current stranded [I, turns,
  polarity, phase], Solid Conductor Current, Current Density [J vector], Voltage-driven
  Conductor; Electrode Voltage, Injected Current; Body/Surface Heat Source [Q or "use EM
  losses" for coupled]; Gravity, Centrifugal, Body Force) + scope references.
- **TP-10 `circuit`** — dynamic netlist rows (element kind V/I/R/L/C, value, from/to
  node dropdowns incl. coil terminals, remove) + add-row; validation: connected graph,
  no dangling terminals.
- **TP-11 `mesh`** — tabs: Global (max size, growth rate, element order linear/
  quadratic, air size factor, curvature refinement) · Local refinement (dynamic rows:
  scope references + local size).
- **TP-12 `solver`** — tabs: Linear (Direct | GMRES | BiCGSTAB; tolerance, max iter,
  preconditioner ILU/ILUT/none, fill-in) · Nonlinear (Newton | Picard; residual tol, max
  iter, relaxation) · Regime (frequency list override; transient params + adaptive).
- **TP-13 `postpro`** — field-plot checkboxes (physics-valid: V, E, D, J, B, H, A,
  Joule-loss density, T, heat flux, u, von Mises, p, Poynting…) + global-quantity
  checkboxes (energy, capacitance [matrix], resistance, inductance, impedance, flux
  linkage, force, torque, total loss, charge, mean/max T, max von Mises, S-parameters).
- **TP-14 `optim_setup`** — name; study; design-parameters grid (checkbox per document
  parameter + min/max [+ steps for sweep]); Sweep: combination (full grid |
  one-at-a-time) + run-count estimate; Optimize: objective (quantity from TP-13, goal
  min/max/target + target value) + method (Nelder-Mead; max evals; convergence tol).
  OK = **Start**: queues immediately and opens the Optimization window.

### 4.4 Glyphs & icons

Glyph language (Graphics AddPoints/AddLines/AddLabel; active study only; hover
emphasis): voltage ⊕ red `#E5484D` · ground ⏚ cyan `#00B7C3` · floating dashed amber ·
surface charge + orange · flux-tangent double-lines blue · flux-normal/applied-H arrows
violet · coil copper loop + turns · current-density arrow array · magnet N/S red-blue ·
temperature thermometer dot (red/blue vs ambient) · heat-flux/convection wavy arrows
(orange/teal + "h, T∞") · fixed cyan cubes · force/pressure red arrows · port/ABC green
chevrons · air box grey dashed wireframe. Flood-plot legend titles carry field, unit and
complex mode ("|B| (T) @ 50 Hz").

Icon set (~40 SVGs, 16×16, 2-3 stroke geometric, consistent with the reference add-in):
ribbon `study-new/study-settings/study-duplicate/study-active/regions/bc/excitation/
air-region/circuit/materials/mesh-generate/mesh-settings/mesh-refine/mesh-view/run/
solver-settings/run-all/stop/field-plot/global-qty/phase/export/clear-plot/sweep/
optimize/optim-results/win-*/demo/help`; tree `physics-{electro,magneto,thermal,wave,
elastic,acoustic,coupled}/node-bc-*/node-region/node-excitation/node-postpro/
node-results-{ok,stale}/node-optim-run`.

## 5. Testing & process

- Golden `.pro` decks (byte-stable, deterministic ordering) + golden `model.msh`.
- fakeHost engine tests (recordingHost/boxHost ports); registry-exhaustiveness tests;
  ribbon-layout test enumerating every command.
- ≥1 analytic oracle per milestone through the REAL vendored binaries (skip-gated via
  `requireSolver(t)`): Ohm, Fourier (steady + series transient), parallel-plate/coax/
  sphere capacitance, solenoid/Biot-Savart B, skin depth, eddy loss, cavity modes,
  cantilever tip deflection (cross-solver constants), circuit DC/AC, axi equivalents.
- Upstream tutorial `.pro` files run through the vendored binary as reference oracles.
- Live MCPBridge in-app run + screenshot before every PR; coverage >80%, duplication
  <3%; golangci funlen 20/30; SPDX `GPL-2.0-only` headers on every exported `.go`.
- CI: lint/spdx/test (3 OS, CGO_ENABLED=0)/race/build (c-shared, 3 OS)/solvers
  (Linux-only build + exact-value smoke) + siblings action (API at go.mod-pinned
  release, host at develop); release.yml clones the reference add-in's catalogue flow.
- Process: PR per issue or small group, `Closes #N`, CI green then manual merge (no
  auto-merge), no `git add -A`, ADRs in Oblikovati terms only.

## 6. Milestones

Implementation is tracked as GitHub issues in this repository (milestones M0–M10):

- **M0 Scaffold & shell** — c-shared shell, engine skeleton + fakeHost, CI + gplpurity.
- **M1 Vendoring** — getdp + gmsh from source, binary discovery, solvers CI smoke.
- **M2 Mesh & physical groups** — ported pipeline, MSH 2.2 writer + units seam, FaceKey
  bind + registries.
- **M3 First physics slice** — electrokinetics + thermal (steady + transient); `pro`
  AST; femmodel v1; ribbon v1; tree v1; TaskPanels v1; run orchestration + flood plot;
  glyphs v1; parametric busbar + heat-sink demos. (Electrokinetics first: no air region,
  nodal elements, exact Ohm oracle.)
- **M4 Electrostatics + air region** — auto air box + explicit fallback, electrostatics
  writer, TP-3, infinite shell, capacitor demo.
- **M5 Magnetostatics** — vector potential (ungauged + GMRES), nonlinear B-H, energy/
  force, magnetic UI, electromagnet demo.
- **M6 Magnetodynamics** — frequency-domain a-v, complex results + Joule loss, time
  domain, AC electromagnet demo.
- **M7 Global quantities & circuits** — floating potentials/C-matrix, global V-I
  coupling + impedance, lumped-circuit netlist + TP-10, transformer demo.
- **M8 Coverage completion** — gauging, full-wave, elasticity, acoustics, coupled
  magneto-thermal, axisymmetric, local refinement UI.
- **M9 Optimization** — `opt` package, objective registry, runner (two variable kinds,
  cancel/restore), TP-14 + Optimization window, optimization demo.
- **M10 Demos, docs, release** — demo sweep (one per physics), Help/demo access, ADR
  set + README, release.yml.

## 7. Risks

1. STL hole-in-box air meshing robustness → M4 fixture + explicit-air-body fallback.
2. Ungauged edge-element GMRES convergence without PETSc → gauging option, exposed
   knobs, mesh-size guardrails.
3. Per-iteration remesh cost in optimization → SolverConstant fast path + memo cache.
4. `.pos`/Table format drift → parsers pinned to golden fixtures from the vendored
   3.5.0 binary.
5. Tutorials are mostly 2D/axi → 3D demos validate against analytic oracles until the
   axisymmetric milestone lands.

## 8. Out of scope (v1)

MPI/parallel solves; PETSc/SLEPc builds; hand-written `.pro` editing UI; moving-band /
rotating-machine kinematics; superconductor models; result persistence in `.obk`
(shared future item across solver add-ins); arrow-glyph vector rendering (polish);
multi-objective/gradient optimization.

## 9. ADRs to write

- ADR-0001 GetDP integration & capability surface (subprocess GPL arm's length).
- ADR-0002 `.pro` generation: full Go AST; upstream templates as test oracles only.
- ADR-0003 Air-region strategy: auto padded box, explicit body fallback, infinite shell.
- ADR-0004 Units seam: SI meters at the Go msh writer, no `-msh_scaling`.
- ADR-0005 Go-owned MSH 2.2 writer; referenced-groups-only emission.
- ADR-0006 3D-first; ungauged + GMRES first; axisymmetric via planar face later.
- ADR-0007 Optimization variable kinds + goroutine/cancel/restore discipline.

## 10. References

- GetDP: <https://getdp.info/> · reference manual
  <https://getdp.info/doc/texinfo/getdp.html> (10 tutorials) ·
  <https://gitlab.onelab.info/getdp/getdp> (v3.5.0).
- Host API `oblikovati.org/api` v0.102.1: `client/{commands,ribbon,docking,task_panels,
  panel_controls,grid_controls,parameters,graphics,model,materials,body}.go`.
- Reference add-in: `Oblikovati.AddIns.CalculiX` (shell, engine, mesh pipeline,
  registries, vendoring, CI, tests).
