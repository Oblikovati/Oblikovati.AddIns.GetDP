# Vendored GetDP — provenance & license

This directory vendors **GetDP** in source form so the add-in builds a self-contained
finite-element solver with no build-time or runtime external dependencies. `build.sh`
compiles it into `build/getdp`; the add-in runs that binary as a subprocess
(`getdp problem.pro -msh mesh.msh -solve <Resolution> -pos <PostOperation>`).

| Component | Version | Upstream | SHA-256 of source archive | License |
|---|---|---|---|---|
| GetDP | 3.5.0 | https://getdp.info/src/getdp-3.5.0-source.tgz | `d6814dc3f81431f1db30b3d5318553efab616d7ea53b352a2c2d0640d130a328` | GPL-2.0-or-later (see `LICENSE.txt`, `CREDITS.txt`) |
| Reference LAPACK (incl. BLAS) | 3.8.0 | https://github.com/Reference-LAPACK/lapack (tag v3.8.0) | `deb22cc4a6120bff72621155a9917f485f96ef8319ac074a7afbc68aab88bcf6` | modified BSD-3-Clause (see `lapack-3.8.0/LICENSE`) |

GetDP is **GPL-2.0-or-later**, compatible with this GPL-2.0-only repository. It bundles
**SPARSKIT** (iterative sparse solvers) and **ARPACK** (eigensolver) under `contrib/`,
each under its own license documented in `CREDITS.txt` / the respective `contrib/*/`
subdirectories — these are **not** governed by this repo's GPL. The reference LAPACK
tree is the same copy the CalculiX add-in vendors (each add-in is self-contained by
the workspace standing rule; no cross-repo build coupling).

## Vendored subtree (what was kept / dropped)

The complete upstream 3.5.0 release tree is kept (`src/`, `contrib/`, `templates/`,
`examples/`, `doc/`, `utils/`, CMake + license/credits/readme files) — it is only
~12 MB and `templates/` + `examples/` serve as **test oracles** for the add-in's
generated `.pro` decks (design spec §3.1: the add-in generates decks with its own Go
AST; upstream templates are never shipped or Included at runtime). `lapack-3.8.0/` is
added beside it for the hermetic BLAS/LAPACK build.

## Local patches

- `src/kernel/LinAlg_SPARSKIT.cpp` — `LinAlg_CreateMatrix` gained a `bool silent`
  parameter in the 3.5 API (`src/kernel/LinAlg.h` declares it, every kernel call site
  passes it), but the 3.5.0 release tarball still ships the four-argument definition in
  the SPARSKIT backend, so a `-DENABLE_SPARSKIT=1` build fails to link out of the box
  (the released binaries are PETSc builds, which is why upstream did not catch it).
  The vendored copy adds the parameter and ignores it — it only mutes an informational
  print in the PETSc backend, and the SPARSKIT backend prints nothing there. The patch
  site carries an `// Oblikovati patch` comment pointing back at this file.

## Build configuration

`build.sh` configures with `-DDEFAULT=0` (hermetic: no feature auto-detection picks up
host GSL/Python/Gmsh/PETSc), then enables exactly: the kernel, Fortran, **SPARSKIT**
(linear solvers), **ARPACK** (eigensolver, needed for the full-wave/acoustic mode
studies), and the in-tree reference BLAS/LAPACK. `getdp --info` on the result reports
`Build options: 64Bit Arpack[contrib] Blas[custom] Kernel Lapack[custom] Sparskit`.

## Smoke fixture (`test/`)

`test/cube.pro` + `test/cube.msh` are a hand-written unit-cube electrokinetic problem
(σ = 1, V = 1 on top, V = 0 on bottom). The exact solution V(z) = z is linear, so
first-order tets reproduce it exactly: the solved `vint.txt` volume integral must be
0.5 to machine precision, and every nodal value in `v.pos` equals the node's z. CI's
`solvers` job runs it after every vendored build.
