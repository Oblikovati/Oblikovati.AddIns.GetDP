#!/usr/bin/env bash
# Build the vendored GetDP solver entirely from the in-repo sources — no network, no
# system BLAS/LAPACK/ARPACK/PETSc. Produces a single self-contained `getdp` executable
# in ./build that the add-in runs as a subprocess (OBK_GETDP_BIN / vendor-src/getdp/build).
#
# Stack (all vendored under this directory, see NOTICE.md for provenance + licenses):
#   LAPACK 3.8.0   reference BLAS + LAPACK           -> build/liblapack.a (fixed-form Fortran)
#   SPARSKIT       iterative linear solvers           -> bundled (contrib/Sparskit)
#   ARPACK         eigensolver (full-wave modes)      -> bundled (contrib/Arpack)
#   GetDP 3.5.0    the FE solver                      -> getdp
#
# Configuration: -DDEFAULT=0 keeps the build hermetic (no feature auto-detection picks
# up host libraries); SPARSKIT replaces PETSc for the linear systems and Arpack serves
# the eigenproblems. Requires a C/C++ compiler + gfortran + cmake (build-time only; the
# shipped binary links nothing beyond libc/libstdc++/libgfortran). Tested with gcc 13.
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
JOBS="${JOBS:-$( (nproc 2>/dev/null || sysctl -n hw.ncpu 2>/dev/null) || echo 4)}"
# Resolve a Fortran compiler. Homebrew's gcc installs `gfortran-14` (no bare `gfortran`
# symlink), so fall back to the versioned names before giving up.
FC="${FC:-$(command -v gfortran gfortran-14 gfortran-13 gfortran-12 gfortran-11 2>/dev/null | head -1)}"
FC="${FC:-gfortran}"
OUT="$HERE/build"
mkdir -p "$OUT"

echo "### [1/2] LAPACK 3.8.0 (reference BLAS+LAPACK) -> liblapack.a"
( cd "$HERE/lapack-3.8.0" && rm -f ./*.o
  $FC -O2 -fcommon -fallow-argument-mismatch -c \
     BLAS/SRC/*.f SRC/*.f \
     INSTALL/dlamch.f INSTALL/slamch.f \
     INSTALL/second_INT_ETIME.f INSTALL/dsecnd_INT_ETIME.f
  ar rcs "$OUT/liblapack.a" ./*.o && rm -f ./*.o )

echo "### [2/2] GetDP 3.5.0 (SPARSKIT + Arpack, no PETSc) -> getdp"
cmake -S "$HERE" -B "$OUT" \
  -DCMAKE_BUILD_TYPE=Release \
  -DCMAKE_Fortran_COMPILER="$FC" \
  -DGETDP_RELEASE=1 \
  -DDEFAULT=0 \
  -DENABLE_KERNEL=1 -DENABLE_FORTRAN=1 \
  -DENABLE_SPARSKIT=1 -DENABLE_ARPACK=1 \
  -DENABLE_BLAS_LAPACK=1 \
  -DBLAS_LAPACK_LIBRARIES="$OUT/liblapack.a;gfortran" \
  -DENABLE_PETSC=0 -DENABLE_SLEPC=0

cmake --build "$OUT" --target getdp -j"$JOBS"
strip -s "$OUT/getdp" 2>/dev/null || true
echo "### built $OUT/getdp"
