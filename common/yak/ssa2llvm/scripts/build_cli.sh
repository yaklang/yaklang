#!/usr/bin/env bash
set -euo pipefail

# build_cli.sh — Build the ssa2llvm CLI (self-contained, fully static).
#
# This script:
#   1. Calls build_yaklib.sh to prepare the embedded runtime (libyak.a with
#      per-module .text section split, crt objects, static libs, cgo deps)
#   2. Builds the ssa2llvm binary from cmd/ssa2llvm/main.go
#
# The resulting ssa2llvm binary is statically linked and embeds all runtime
# artifacts. At compile time it uses in-process lld (via go-llvm) with
# --gc-sections to produce per-script minimal executables.

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

# 1. Build yaklib (runtime + per-module .text split + embed assets)
echo "[ssa2llvm] building yaklib..."
"${SSA2LLVM_DIR}/scripts/build_yaklib.sh"

# 2. Build the ssa2llvm CLI
echo "[ssa2llvm] building static self-contained CLI -> ${OUT}"
CGO_ENABLED=1 go build \
  -ldflags "${GO_LDFLAGS} -extldflags '-static'" \
  -o "${OUT}" \
  ./common/yak/ssa2llvm/cmd/ssa2llvm

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
