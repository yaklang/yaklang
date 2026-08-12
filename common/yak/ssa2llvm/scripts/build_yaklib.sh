#!/usr/bin/env bash
set -euo pipefail

# build_yaklib.sh — Build the yaklib c-archive (libyak.a) and prepare it for
# per-module dead-code elimination at ssa2llvm compile time.
#
# Steps:
#   1. Generate runtime_imports_generated.go (permodule mode: empty init(),
#      one //export yak_register_module_<m> per module)
#   2. Build libyak.a via `go build -buildmode=c-archive`
#   3. Post-process go.o: split .text into per-module sections (.text.mod_<m>)
#      using the elfsplit tool (cmd/elfsplit)
#   4. Collect cgo C static deps (libpcap.a, libm.a, ...)
#   5. Copy all artifacts to runtime/embed/assets/
#   6. Generate manifest_generated.go (SHA256 for cache key)
#
# Which yaklib modules are registered is controlled by SSA2LLVM_EMBED_MODULES
# (comma-separated; "all" = every registered module). Default is a curated set.
#
# Run this before building ssa2llvm (scripts/build_cli.sh calls this automatically).

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SSA2LLVM_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
RUNTIME_DIR="${SSA2LLVM_DIR}/runtime"
ASSETS_DIR="${RUNTIME_DIR}/embed/assets"
REPO_ROOT="$(cd "${SSA2LLVM_DIR}/../../.." && pwd)"
RUNTIME_GO_DIR="${RUNTIME_DIR}/runtime_go"
IMPORTS_FILE="${RUNTIME_GO_DIR}/runtime_imports_generated.go"
EXTDEPS_DIR="${ASSETS_DIR}/extdeps"

# Default AOT module set. The AOT runtime is built with the ssa2llvm_aot build
# tag: globals come from runtime_globals_aot.go and monolith-backed modules use
# the lightweight aotlib export tables (os/codec), so the monolithic
# common/yak/yaklib and its yaklang frontend dependencies stay out of libyak.a.
# libyak.a stays complete (all modules below); per-script pruning happens at
# link time via --gc-sections, so a plain print script drops the poc/cli/http
# code and their dependency closures while a poc script keeps them.
DEFAULT_MODULES="os,poc,cli,http,codec"
MODULES="${SSA2LLVM_EMBED_MODULES:-${DEFAULT_MODULES}}"
# The shared poc/ssa dependency closure and the ssa language frontends live in
# their own split groups. genfull ignores the unknown names; elfsplit splits
# them. The compiler keeps "shared" whenever any module is used, and keeps
# "ssafront" when the ssa module is used.
AOT_SPLIT_MODULES="${MODULES}"
AOT_SPLIT_MODULES="${AOT_SPLIT_MODULES},shared"
case ",${MODULES}," in
  *,ssa,*) AOT_SPLIT_MODULES="${AOT_SPLIT_MODULES},ssafront" ;;
esac

mkdir -p "${ASSETS_DIR}"
rm -rf "${EXTDEPS_DIR}"
mkdir -p "${EXTDEPS_DIR}"

# ── 1. Generate runtime_imports_generated.go (permodule mode) ──────────────
echo "[yaklib] generating per-module runtime imports for: ${MODULES}"
GENFULL_OUT="$(mktemp -t ssa2llvm-imports-XXXXXX.go)"
trap 'rm -f "${GENFULL_OUT}"; [ -n "${STUB_BACKUP:-}" ] && cp "${STUB_BACKUP}" "${IMPORTS_FILE}" 2>/dev/null || true; rm -f "${STUB_BACKUP:-/dev/null}" 2>/dev/null || true' EXIT
STUB_BACKUP="$(mktemp -t ssa2llvm-stub-XXXXXX.go)"
cp "${IMPORTS_FILE}" "${STUB_BACKUP}"

# Use --permodule --aot to generate per-module //export registration functions
# with lightweight AOT export tables.
( cd "${REPO_ROOT}" && go run ./common/yak/ssa2llvm/runtime/embed/genfull --permodule --aot "${GENFULL_OUT}" ${MODULES//,/ } )
cp "${GENFULL_OUT}" "${IMPORTS_FILE}"

# ── 2. Build libyak.a (Go c-archive runtime) ───────────────────────────────
echo "[yaklib] building libyak.a (Go runtime, modules: ${MODULES})..."
cd "${RUNTIME_GO_DIR}"
EXTRA_LDFLAGS="-s -w"
if [[ "${SSA2LLVM_RUNTIME_DEBUG:-}" == "1" || "${SSA2LLVM_RUNTIME_DEBUG:-}" == "true" ]]; then
    EXTRA_LDFLAGS=""
fi
CGO_CFLAGS="-ffunction-sections" CGO_ENABLED=1 GOWORK=off go build -tags ssa2llvm_aot -trimpath -ldflags="${EXTRA_LDFLAGS}" -buildmode=c-archive -o "${RUNTIME_DIR}/libyak.a" .
rm -f "${RUNTIME_DIR}/libyak.h" "${RUNTIME_GO_DIR}/libyak.h"
echo "[yaklib] built libyak.a"

# ── 2b. Collect cgo C static deps (while overlay is in place) ──────────────
echo "[yaklib] collecting cgo C static deps..."
collect_extdeps() {
  local ldflags
  ldflags="$(go list -deps -f '{{if .CgoLDFLAGS}}{{range .CgoLDFLAGS}}{{printf "%s\n" .}}{{end}}{{end}}' . 2>/dev/null || true)"

  local -a ldirs=() libs=()
  while IFS= read -r tok; do
    [ -z "${tok}" ] && continue
    case "${tok}" in
      -L*) ldirs+=("${tok#-L}") ;;
      -l*) libs+=("${tok#-l}") ;;
    esac
  done <<< "${ldflags}"

  local skip_re='^(gc|c|gcc|gcc_eh|dl|pthread|rt)$'
  local seen_libs="" seen_files="" resolved=0

  add_extdep() {
    local arc="$1"
    [ -s "${arc}" ] || return 0
    local magic; magic="$(head -c 8 "${arc}" 2>/dev/null || true)"
    if [ "${magic}" != "!<arch>" ]; then return 0; fi
    local base; base="$(basename "${arc}")"
    case " ${seen_files} " in *" ${base} "*) return 0 ;; esac
    seen_files+=" ${base}"
    cp "${arc}" "${EXTDEPS_DIR}/${base}"
    echo "[yaklib]   extdep ${base} <- ${arc}"
    resolved=$((resolved+1))
  }

  for lib in "${libs[@]}"; do
    [[ "${lib}" =~ ${skip_re} ]] && continue
    case " ${seen_libs} " in *" ${lib} "*) continue ;; esac
    seen_libs+=" ${lib}"
    local arc=""
    for d in "${ldirs[@]}"; do
      if [ -s "${d}/lib${lib}.a" ]; then arc="${d}/lib${lib}.a"; break; fi
    done
    if [ -z "${arc}" ]; then
      local p; p="$(gcc -print-file-name="lib${lib}.a" 2>/dev/null || true)"
      [ -n "${p}" ] && [ -s "${p}" ] && [ "${p}" != "lib${lib}.a" ] && arc="${p}"
    fi
    if [ -z "${arc}" ]; then
      echo "[yaklib] WARN: no static lib${lib}.a found; scripts using it will fail to link" >&2
      continue
    fi
    local magic; magic="$(head -c 8 "${arc}" 2>/dev/null || true)"
    if [ "${magic}" = "!<arch>" ]; then
      add_extdep "${arc}"
    else
      local grp
      grp="$(grep -Eo 'GROUP[[:space:]]*\([^)]*\)' "${arc}" 2>/dev/null | head -1)"
      if [ -n "${grp}" ]; then
        local inner; inner="${grp#* (}"; inner="${inner%) *}"; inner="${inner%)}"
        for m in ${inner}; do
          case "${m}" in /*) ;; *) m="$(dirname "${arc}")/${m}" ;; esac
          add_extdep "${m}"
        done
      fi
      fi
  done
  echo "[yaklib] collected ${resolved} cgo C static dep(s) into ${EXTDEPS_DIR}"
}
collect_extdeps

# Restore the stub so the source tree stays clean.
cp "${STUB_BACKUP}" "${IMPORTS_FILE}"

# ── 3. Post-process go.o: split .text into per-module sections ─────────────
echo "[yaklib] post-processing go.o (per-module .text split)..."
# Build the elfsplit tool
( cd "${REPO_ROOT}" && go build -o "${SSA2LLVM_DIR}/cmd/elfsplit/elfsplit" ./common/yak/ssa2llvm/cmd/elfsplit )

# Extract go.o from libyak.a, split it, repack
SPLIT_DIR="$(mktemp -d)"
REPACK_DIR="$(mktemp -d)"
trap 'rm -rf "${SPLIT_DIR}" "${REPACK_DIR}"; rm -f "${GENFULL_OUT}"; [ -n "${STUB_BACKUP:-}" ] && cp "${STUB_BACKUP}" "${IMPORTS_FILE}" 2>/dev/null || true; rm -f "${STUB_BACKUP:-/dev/null}" 2>/dev/null || true' EXIT
# Extract only go.o, split it, then repack the archive from a fresh dir so the
# split go.o is NOT overwritten by the original member.
llvm-ar x "${RUNTIME_DIR}/libyak.a" --output="${SPLIT_DIR}" go.o 2>/dev/null || true
if [ -f "${SPLIT_DIR}/go.o" ]; then
    "${SSA2LLVM_DIR}/cmd/elfsplit/elfsplit" "${SPLIT_DIR}/go.o" "${SPLIT_DIR}/go_split.o" "${AOT_SPLIT_MODULES}"
    # Extract all original members into a fresh repack dir.
    llvm-ar x "${RUNTIME_DIR}/libyak.a" --output="${REPACK_DIR}"
    # Replace go.o with the split version.
    cp "${SPLIT_DIR}/go_split.o" "${REPACK_DIR}/go.o"
    # Repack all members (keep original order via listing).
    cd "${REPACK_DIR}"
    llvm-ar rcs "${RUNTIME_DIR}/libyak.a" $(ls *.o)
    cd - >/dev/null
    echo "[yaklib] go.o split into per-module sections"
else
    echo "[yaklib] WARNING: go.o not found in libyak.a, skipping split"
fi
rm -rf "${SPLIT_DIR}" "${REPACK_DIR}"

# ── 4. Locate system libraries (libgc, crt, libc, libgcc) ───────────────────
LIBGC_SRC="${RUNTIME_GO_DIR}/libs/libgc.a"
if [[ ! -s "${LIBGC_SRC}" ]]; then
  LIBGC_SRC=""
  for tool in cc gcc clang; do
    if command -v "$tool" >/dev/null 2>&1; then
      p="$("$tool" -print-file-name=libgc.a 2>/dev/null || true)"
      if [[ -n "${p}" && -f "${p}" ]]; then LIBGC_SRC="${p}"; break; fi
    fi
  done
fi
[[ -n "${LIBGC_SRC}" && -s "${LIBGC_SRC}" ]] || { echo "libgc.a not found" >&2; exit 1; }

need_file() {
  local name="$1" p
  p="$(gcc -print-file-name="${name}")"
  [[ -s "${p}" ]] || { echo "missing: ${name}" >&2; exit 1; }
  printf '%s' "${p}"
}
CRT1="$(need_file crt1.o)"; CRTI="$(need_file crti.o)"; CRTN="$(need_file crtn.o)"
CRTBEGIN="$(need_file crtbegin.o)"; CRTEND="$(need_file crtend.o)"
LIBC_A="$(need_file libc.a)"; LIBGCC_A="$(need_file libgcc.a)"; LIBGCC_EH_A="$(need_file libgcc_eh.a)"

# ── 5. Copy all assets to embed dir ─────────────────────────────────────────
cp "${RUNTIME_DIR}/libyak.a" "${ASSETS_DIR}/libyak.a"
cp "${LIBGC_SRC}"            "${ASSETS_DIR}/libgc.a"
cp "${CRT1}"  "${ASSETS_DIR}/crt1.o";  cp "${CRTI}"  "${ASSETS_DIR}/crti.o";  cp "${CRTN}"  "${ASSETS_DIR}/crtn.o"
cp "${CRTBEGIN}" "${ASSETS_DIR}/crtbegin.o"; cp "${CRTEND}" "${ASSETS_DIR}/crtend.o"
cp "${LIBC_A}"     "${ASSETS_DIR}/libc.a"
cp "${LIBGCC_A}"   "${ASSETS_DIR}/libgcc.a"
cp "${LIBGCC_EH_A}" "${ASSETS_DIR}/libgcc_eh.a"

# ── 6. Generate manifest_generated.go ──────────────────────────────────────
sha_of() { sha256sum "$1" | awk '{print $1}'; }
gen_field() { printf '\t%-10s "%s",\n' "$1:" "$(sha_of "$2")"; }

EXTDEP_NAMES=()
if compgen -G "${EXTDEPS_DIR}/*.a" >/dev/null 2>&1; then
  for f in "${EXTDEPS_DIR}"/*.a; do
    EXTDEP_NAMES+=("$(basename "$f")")
  done
  IFS=$'\n' EXTDEP_NAMES=($(printf '%s\n' "${EXTDEP_NAMES[@]}" | sort)); unset IFS
fi

{
  echo "// Code generated by scripts/build_yaklib.sh — DO NOT EDIT manually."
  echo "// Regenerate by running scripts/build_yaklib.sh before building ssa2llvm."
  echo ""
  echo "package assets"
  echo ""
  echo "var EmbeddedManifest = Manifest{"
  gen_field Libyak   "${ASSETS_DIR}/libyak.a"
  gen_field Libgc    "${ASSETS_DIR}/libgc.a"
  gen_field Crt1     "${ASSETS_DIR}/crt1.o"
  gen_field Crti     "${ASSETS_DIR}/crti.o"
  gen_field Crtn     "${ASSETS_DIR}/crtn.o"
  gen_field CrtBegin "${ASSETS_DIR}/crtbegin.o"
  gen_field CrtEnd   "${ASSETS_DIR}/crtend.o"
  gen_field Libc     "${ASSETS_DIR}/libc.a"
  gen_field Libgcc   "${ASSETS_DIR}/libgcc.a"
  gen_field LibgccEh "${ASSETS_DIR}/libgcc_eh.a"
  echo "}"
  echo ""
  echo "// extdepManifest lists the extra cgo C static libraries embedded under"
  echo "// extdeps/, in stable (name-sorted) order. Linked inside the lld group."
  echo "var extdepManifest = []ExtDep{"
  for name in "${EXTDEP_NAMES[@]}"; do
    printf '\t{Name: "%s", Sha: "%s"},\n' "${name}" "$(sha_of "${EXTDEPS_DIR}/${name}")"
  done
  echo "}"
} > "${ASSETS_DIR}/manifest_generated.go"

echo "[yaklib] embedded runtime assets prepared in ${ASSETS_DIR}"
echo "[yaklib]   libyak.a $(du -h "${ASSETS_DIR}/libyak.a" | cut -f1) (modules: ${MODULES}), libgc.a $(du -h "${ASSETS_DIR}/libgc.a" | cut -f1)"
echo "[yaklib]   crt1/crti/crtn + crtbegin/crtend"
echo "[yaklib]   libc.a $(du -h "${ASSETS_DIR}/libc.a" | cut -f1), libgcc.a $(du -h "${ASSETS_DIR}/libgcc.a" | cut -f1), libgcc_eh.a $(du -h "${ASSETS_DIR}/libgcc_eh.a" | cut -f1)"
if [ "${#EXTDEP_NAMES[@]}" -gt 0 ]; then
  echo "[yaklib]   extdeps: ${EXTDEP_NAMES[*]}"
fi
