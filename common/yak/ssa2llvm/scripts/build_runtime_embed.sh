#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SSA2LLVM_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"

echo "[ssa2llvm] building minimal runtime archive (local source only)..."
"${SSA2LLVM_DIR}/scripts/build_runtime_go.sh"

echo "[ssa2llvm] done: ${SSA2LLVM_DIR}/runtime/libyak.a"
