#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
RUNTIME_DIR="${ROOT_DIR}/runtime"

cd "${RUNTIME_DIR}/runtime_go"

# Default release build: no diagnostic strings / GCLOG paths (see readme "三条独立轨道").
# For local debugging: SSA2LLVM_RUNTIME_DEBUG=1 ./build_runtime_go.sh
EXTRA_TAGS=""
if [[ "${SSA2LLVM_RUNTIME_DEBUG:-}" == "1" || "${SSA2LLVM_RUNTIME_DEBUG:-}" == "true" ]]; then
	EXTRA_TAGS="-tags=ssa2llvm_runtime_debug"
fi

go build ${EXTRA_TAGS} -buildmode=c-archive -o "${RUNTIME_DIR}/libyak.a" .
rm -f "${RUNTIME_DIR}/libyak.h"

echo "Built ${RUNTIME_DIR}/libyak.a (Go Runtime)"
