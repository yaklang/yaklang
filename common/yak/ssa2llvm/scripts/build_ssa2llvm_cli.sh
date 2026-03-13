#!/usr/bin/env bash
set -euo pipefail

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

echo "[ssa2llvm] generating embedded runtime archive..."
"${SSA2LLVM_DIR}/scripts/build_runtime_embed.sh"

echo "[ssa2llvm] building CLI with embedded runtime..."
go build -tags gzip_embed -ldflags "${GO_LDFLAGS}" -o "${OUT}" ./common/yak/ssa2llvm/cmd

echo "[ssa2llvm] done: ${OUT}"

