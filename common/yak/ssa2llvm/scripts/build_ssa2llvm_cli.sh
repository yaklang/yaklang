#!/usr/bin/env bash
set -euo pipefail

# Build the ssa2llvm CLI. By default it uses the legacy clang/llc link path.
#
# Pass --selfcontained (or set SSA2LLVM_SELF_CONTAINED=1) to build the
# self-contained CLI: it embeds the runtime archives + crt + static libc/libgcc
# (via scripts/build_runtime_embed.sh) and links lld in-process, so the resulting
# ssa2llvm binary compiles yak scripts with zero external toolchain dependency
# and produces portable, fully-static executables. This requires the go-llvm
# dependency to provide the in-process lld bindings (see the replace directive in
# go.mod during development) and libstdc++ at build time.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SSA2LLVM_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
REPO_ROOT="$(cd "${SSA2LLVM_DIR}/../../.." && pwd)"

OUT="${REPO_ROOT}/ssa2llvm"
GO_LDFLAGS=${GO_LDFLAGS:-"-s -w"}
SELF_CONTAINED=0

if [[ $# -ge 2 && "$1" == "-o" ]]; then
  OUT="$2"
  shift 2
fi
for arg in "$@"; do
  case "$arg" in
    --selfcontained) SELF_CONTAINED=1 ;;
  esac
done
if [[ "${SSA2LLVM_SELF_CONTAINED:-0}" == "1" ]]; then
  SELF_CONTAINED=1
fi

cd "${REPO_ROOT}"

GO_BUILD_TAGS=()
if [[ "${SELF_CONTAINED}" == "1" ]]; then
  echo "[ssa2llvm] preparing embedded runtime assets..."
  "${SSA2LLVM_DIR}/scripts/build_runtime_embed.sh"
  GO_BUILD_TAGS+=( -tags=selfcontained )
  echo "[ssa2llvm] building self-contained CLI..."
else
  echo "[ssa2llvm] building CLI..."
fi

go build "${GO_BUILD_TAGS[@]}" -ldflags "${GO_LDFLAGS}" -o "${OUT}" ./common/yak/ssa2llvm/cmd

echo "[ssa2llvm] done: ${OUT}"