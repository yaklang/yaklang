#!/usr/bin/env bash
set -euo pipefail

# Build the ssa2llvm CLI (self-contained only). It embeds the runtime archives +
# crt + static libc/libgcc + the extra cgo C static deps (via
# scripts/build_runtime_embed.sh) and links lld in-process, so the resulting
# ssa2llvm binary compiles yak scripts with zero external toolchain dependency
# and produces portable, fully-static executables. This requires the go-llvm
# dependency to provide the in-process lld bindings (see the replace directive in
# go.mod during development) and libstdc++ at build time.
#
# Run scripts/build_runtime_embed.sh first (this script does it for you) so the
# embedded asset files exist.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SSA2LLVM_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
REPO_ROOT="$(cd "${SSA2LLVM_DIR}/../../.." && pwd)"

OUT="${REPO_ROOT}/ssa2llvm"
GO_LDFLAGS=${GO_LDFLAGS:-"-s -w"}

if [[ $# -ge 2 && "$1" == "-o" ]]; then
  OUT="$2"
  shift 2
fi

cd "${REPO_ROOT}"

echo "[ssa2llvm] preparing embedded runtime assets..."
"${SSA2LLVM_DIR}/scripts/build_runtime_embed.sh"

echo "[ssa2llvm] building self-contained CLI..."
go build -ldflags "${GO_LDFLAGS}" -o "${OUT}" ./common/yak/ssa2llvm/cmd

echo "[ssa2llvm] done: ${OUT}"
