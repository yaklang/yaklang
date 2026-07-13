#!/usr/bin/env bash
set -euo pipefail

# Populate runtime/embed/assets/ with the self-contained runtime artifacts:
#   - libyak.a              (Go c-archive runtime, built registering a configured
#                            set of yaklib modules so their exports resolve)
#   - libgc.a               (Boehm GC)
#   - crt1.o crti.o crtn.o crtbegin.o crtend.o  (crt objects)
#   - libc.a libgcc.a libgcc_eh.a  (static glibc + gcc runtime)
#   - extdeps/              (extra cgo C static libs the registered modules pull
#                            in, e.g. libpcap.a for poc, libm.a, libresolv.a)
# and generate manifest_generated.go with their SHA256 (used as a cache key).
#
# Which yaklib modules the embedded libyak.a registers is controlled by
# SSA2LLVM_EMBED_MODULES (comma-separated module names; "all" = every registered
# module — large, ~270M libyak.a). Default is a curated common-stdlib subset.
# Run this before `go build -tags=selfcontained ./common/yak/ssa2llvm/cmd`.
# Building ssa2llvm itself may use any environment; these artifacts are embedded
# so the resulting ssa2llvm binary needs none of them at runtime.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SSA2LLVM_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
RUNTIME_DIR="${SSA2LLVM_DIR}/runtime"
ASSETS_DIR="${SSA2LLVM_DIR}/runtime/embed/assets"
REPO_ROOT="$(cd "${SSA2LLVM_DIR}/../../.." && pwd)"
RUNTIME_GO_DIR="${RUNTIME_DIR}/runtime_go"
IMPORTS_FILE="${RUNTIME_GO_DIR}/runtime_imports_generated.go"
EXTDEPS_DIR="${ASSETS_DIR}/extdeps"

# Default curated common-stdlib module set (covers typical scripting + the
# os/poc/http workflow). Override with SSA2LLVM_EMBED_MODULES="m1,m2,..." or
# SSA2LLVM_EMBED_MODULES="all" for the full set (large).
DEFAULT_MODULES="os,poc,http,httpool,httptpl,codec,str,json,re,re2,time,math,io,sync,cli,file,filesys,env,log,gzip,zip,context,bufio,container,dictutil,judge,x,xhtml,xml,xpath,yaml,exec,dns,fuzz,yakit,ssa"
MODULES="${SSA2LLVM_EMBED_MODULES:-${DEFAULT_MODULES}}"

mkdir -p "${ASSETS_DIR}"
rm -rf "${EXTDEPS_DIR}"
mkdir -p "${EXTDEPS_DIR}"

# 1. Build libyak.a with the configured yaklib modules registered.
#    genfull generates runtime_imports_generated.go for the module set; overlay
#    it onto the checked-in stub, build the c-archive, then restore the stub so
#    the source tree stays clean.
echo "[ssa2llvm] generating runtime imports for modules: ${MODULES}"
GENFULL_OUT="$(mktemp -t ssa2llvm-imports-XXXXXX.go)"
trap 'rm -f "${GENFULL_OUT}"; [ -n "${STUB_BACKUP:-}" ] && cp "${STUB_BACKUP}" "${IMPORTS_FILE}" 2>/dev/null || true; rm -f "${STUB_BACKUP:-/dev/null}" 2>/dev/null || true' EXIT
STUB_BACKUP="$(mktemp -t ssa2llvm-stub-XXXXXX.go)"
cp "${IMPORTS_FILE}" "${STUB_BACKUP}"

# shellcheck disable=SC2086
( cd "${REPO_ROOT}" && go run ./common/yak/ssa2llvm/runtime/embed/genfull "${GENFULL_OUT}" ${MODULES//,/ } )
cp "${GENFULL_OUT}" "${IMPORTS_FILE}"

echo "[ssa2llvm] building libyak.a (Go runtime, modules: ${MODULES})..."
"${SSA2LLVM_DIR}/scripts/build_runtime_go.sh"

# 1b. Collect the cgo C static libraries the registered modules pull in (while
#     the imports overlay is still in place, so go list sees the real deps).
#     This is what makes heavy modules like poc link with zero host dependency:
#     their cgo deps (libpcap.a, ...) are embedded and linked in-process by lld.
echo "[ssa2llvm] collecting cgo C static deps..."
collect_extdeps() {
  # go list emits one token per CgoLDFLAGS entry: -Ldir and -lfoo.
  local ldflags
  ldflags="$(cd "${RUNTIME_GO_DIR}" && go list -deps -f '{{if .CgoLDFLAGS}}{{range .CgoLDFLAGS}}{{printf "%s\n" .}}{{end}}{{end}}' . 2>/dev/null || true)"

  local -a ldirs=()
  local -a libs=()
  while IFS= read -r tok; do
    [ -z "${tok}" ] && continue
    case "${tok}" in
      -L*) ldirs+=("${tok#-L}") ;;
      -l*) libs+=("${tok#-l}") ;;
    esac
  done <<< "${ldflags}"

  # Skip libs already embedded separately or folded into libc on glibc >= 2.34.
  local skip_re='^(gc|c|gcc|gcc_eh|dl|pthread|rt)$'
  local seen_libs=""
  local seen_files=""
  local resolved=0

  # add_extdep <real-ar-path>: embed one real ar archive under its basename
  # (dedup by basename). Rejects non-archives (e.g. linker scripts) so we never
  # embed a script that would make lld follow host paths at runtime.
  add_extdep() {
    local arc="$1"
    [ -s "${arc}" ] || return 0
    local magic; magic="$(head -c 8 "${arc}" 2>/dev/null || true)"
    if [ "${magic}" != "!<arch>" ]; then
      echo "[ssa2llvm] WARN: $(basename "${arc}") (${arc}) is not an ar archive; skipping" >&2
      return 0
    fi
    local base; base="$(basename "${arc}")"
    case " ${seen_files} " in *" ${base} "*) return 0 ;; esac
    seen_files+=" ${base}"
    cp "${arc}" "${EXTDEPS_DIR}/${base}"
    echo "[ssa2llvm]   extdep ${base} <- ${arc}"
    resolved=$((resolved+1))
  }

  for lib in "${libs[@]}"; do
    [[ "${lib}" =~ ${skip_re} ]] && continue
    case " ${seen_libs} " in *" ${lib} "*) continue ;; esac
    seen_libs+=" ${lib}"
    local arc=""
    # Prefer cgo -L dirs (e.g. the version-pinned bundled libpcap.a in the
    # yaklang/pcap module) over host libs, for reproducibility.
    for d in "${ldirs[@]}"; do
      if [ -s "${d}/lib${lib}.a" ]; then arc="${d}/lib${lib}.a"; break; fi
    done
    if [ -z "${arc}" ]; then
      local p; p="$(gcc -print-file-name="lib${lib}.a" 2>/dev/null || true)"
      [ -n "${p}" ] && [ -s "${p}" ] && [ "${p}" != "lib${lib}.a" ] && arc="${p}"
    fi
    if [ -z "${arc}" ]; then
      echo "[ssa2llvm] WARN: no static lib${lib}.a found for cgo dep '-l${lib}'; scripts using it will fail to link" >&2
      continue
    fi
    # If the resolved file is a real ar archive, embed it directly. glibc ships
    # some libs (libm.a) as GNU ld linker scripts of the form
    #   GROUP ( /path/libm-2.39.a /path/libmvec.a )
    # In that case follow the GROUP and embed the referenced real archives
    # (embedding the script itself would make lld follow host paths at runtime,
    # breaking the zero-host-dependency guarantee).
    local magic; magic="$(head -c 8 "${arc}" 2>/dev/null || true)"
    if [ "${magic}" = "!<arch>" ]; then
      add_extdep "${arc}"
    else
      local grp
      grp="$(grep -Eo 'GROUP[[:space:]]*\([^)]*\)' "${arc}" 2>/dev/null | head -1)"
      if [ -n "${grp}" ]; then
        # extract whitespace-separated paths inside the parentheses
        local inner; inner="${grp#* (}"; inner="${inner%) *}"; inner="${inner%)}"
        for m in ${inner}; do
          case "${m}" in /*) ;; *) m="$(dirname "${arc}")/${m}" ;; esac
          add_extdep "${m}"
        done
      else
        echo "[ssa2llvm] WARN: lib${lib}.a (${arc}) is a non-archive linker script with no GROUP(); skipping" >&2
      fi
    fi
  done
  echo "[ssa2llvm] collected ${resolved} cgo C static dep(s) into ${EXTDEPS_DIR}"
}
collect_extdeps

# Restore the stub so the source tree is clean (libyak.a already built from the overlay).
cp "${STUB_BACKUP}" "${IMPORTS_FILE}"

# 2. Locate libgc.a — prefer runtime/runtime_go/libs/libgc.a, else via cc/gcc/clang.
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
[[ -n "${LIBGC_SRC}" && -s "${LIBGC_SRC}" ]] || {
  echo "libgc.a not found (install libgc-dev or place runtime/runtime_go/libs/libgc.a)" >&2
  exit 1
}

# 3. Locate crt objects + static libs via gcc -print-file-name (returns full path).
need_file() {
  local name="$1" p
  p="$(gcc -print-file-name="${name}")"
  [[ -s "${p}" ]] || { echo "missing system file: ${name} (gcc -print-file-name returned '${p}')" >&2; exit 1; }
  printf '%s' "${p}"
}
CRT1="$(need_file crt1.o)"; CRTI="$(need_file crti.o)"; CRTN="$(need_file crtn.o)"
CRTBEGIN="$(need_file crtbegin.o)"; CRTEND="$(need_file crtend.o)"
LIBC_A="$(need_file libc.a)"; LIBGCC_A="$(need_file libgcc.a)"; LIBGCC_EH_A="$(need_file libgcc_eh.a)"

# 4. Copy assets into the embed dir.
cp "${RUNTIME_DIR}/libyak.a" "${ASSETS_DIR}/libyak.a"
cp "${LIBGC_SRC}"            "${ASSETS_DIR}/libgc.a"
cp "${CRT1}"  "${ASSETS_DIR}/crt1.o";  cp "${CRTI}"  "${ASSETS_DIR}/crti.o";  cp "${CRTN}"  "${ASSETS_DIR}/crtn.o"
cp "${CRTBEGIN}" "${ASSETS_DIR}/crtbegin.o"; cp "${CRTEND}" "${ASSETS_DIR}/crtend.o"
cp "${LIBC_A}"     "${ASSETS_DIR}/libc.a"
cp "${LIBGCC_A}"   "${ASSETS_DIR}/libgcc.a"
cp "${LIBGCC_EH_A}" "${ASSETS_DIR}/libgcc_eh.a"

# 5. Generate manifest_generated.go with SHA256 of each asset (including extdeps).
sha_of() { sha256sum "$1" | awk '{print $1}'; }
gen_field() { printf '\t%-10s "%s",\n' "$1:" "$(sha_of "$2")"; }

EXTDEP_NAMES=()
if compgen -G "${EXTDEPS_DIR}/*.a" >/dev/null 2>&1; then
  for f in "${EXTDEPS_DIR}"/*.a; do
    EXTDEP_NAMES+=("$(basename "$f")")
  done
  # stable order
  IFS=$'\n' EXTDEP_NAMES=($(printf '%s\n' "${EXTDEP_NAMES[@]}" | sort)); unset IFS
fi

{
  echo "// Code generated by scripts/build_runtime_embed.sh — DO NOT EDIT manually."
  echo "// Regenerate by running scripts/build_runtime_embed.sh before building ssa2llvm."
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

echo "[ssa2llvm] embedded runtime assets prepared in ${ASSETS_DIR}"
echo "[ssa2llvm]   libyak.a $(du -h "${ASSETS_DIR}/libyak.a" | cut -f1) (modules: ${MODULES}), libgc.a $(du -h "${ASSETS_DIR}/libgc.a" | cut -f1)"
echo "[ssa2llvm]   crt1/crti/crtn + crtbegin/crtend"
echo "[ssa2llvm]   libc.a $(du -h "${ASSETS_DIR}/libc.a" | cut -f1), libgcc.a $(du -h "${ASSETS_DIR}/libgcc.a" | cut -f1), libgcc_eh.a $(du -h "${ASSETS_DIR}/libgcc_eh.a" | cut -f1)"
if [ "${#EXTDEP_NAMES[@]}" -gt 0 ]; then
  echo "[ssa2llvm]   extdeps: ${EXTDEP_NAMES[*]}"
fi
