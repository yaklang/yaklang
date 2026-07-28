#!/usr/bin/env bash
set -euo pipefail

# Build the ssa2llvm CLI (self-contained, fully static). It embeds the runtime
# archives + crt + static libc/libgcc + the extra cgo C static deps (via
# scripts/build_runtime_embed.sh) and links lld in-process, so the resulting
# ssa2llvm binary compiles yak scripts with zero external toolchain dependency
# and produces portable, fully-static executables.
#
# The output binary is written to the directory from which this script is
# invoked (the caller's CWD), not the repo root. Pass -o <path> to override.
#
# The binary itself is statically linked (-extldflags '-static') so it has no
# runtime shared-library dependencies. This requires the go-llvm dependency to
# provide the in-process lld bindings (see the replace directive in go.mod
# during development) and the static C libraries (z/zstd/tinfo/ffi/xml2/dbus/
# libnl/pcap) to be present on the build host.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SSA2LLVM_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
REPO_ROOT="$(cd "${SSA2LLVM_DIR}/../../.." && pwd)"

# Default output: caller's CWD.
OUT="$(pwd)/ssa2llvm"
GO_LDFLAGS=${GO_LDFLAGS:-"-s -w"}

if [[ $# -ge 2 && "$1" == "-o" ]]; then
  OUT="$2"
  shift 2
fi

cd "${REPO_ROOT}"

echo "[ssa2llvm] preparing embedded runtime assets..."
"${SSA2LLVM_DIR}/scripts/build_runtime_embed.sh"

echo "[ssa2llvm] building static self-contained CLI -> ${OUT}"
CGO_ENABLED=1 go build \
  -ldflags "${GO_LDFLAGS} -extldflags '-static'" \
  -o "${OUT}" \
  ./common/yak/ssa2llvm/cmd

echo "[ssa2llvm] done: ${OUT}"
echo "[ssa2llvm] verifying static link..."
if command -v ldd >/dev/null 2>&1; then
  ldd_out="$(ldd "${OUT}" 2>&1 || true)"
  if echo "${ldd_out}" | grep -q "not a dynamic executable"; then
    echo "[ssa2llvm] OK: statically linked"
  else
    echo "[ssa2llvm] WARNING: binary is NOT statically linked:" >&2
    echo "${ldd_out}" >&2
  fi
fi
