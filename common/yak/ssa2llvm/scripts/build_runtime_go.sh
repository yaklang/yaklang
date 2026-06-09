#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
RUNTIME_DIR="${ROOT_DIR}/runtime"

cd "${RUNTIME_DIR}/runtime_go"

# Default release build: no diagnostic strings / GCLOG paths (see readme "三条独立轨道").
# For local debugging: SSA2LLVM_RUNTIME_DEBUG=1 ./build_runtime_go.sh
EXTRA_TAGS=""
EXTRA_LDFLAGS="-s -w"
if [[ "${SSA2LLVM_RUNTIME_DEBUG:-}" == "1" || "${SSA2LLVM_RUNTIME_DEBUG:-}" == "true" ]]; then
	EXTRA_TAGS="-tags=ssa2llvm_runtime_debug"
	EXTRA_LDFLAGS=""
fi

GO_BUILD_ARGS=(-trimpath)
if [[ -n "${EXTRA_TAGS}" ]]; then
	GO_BUILD_ARGS+=("${EXTRA_TAGS}")
fi
if [[ -n "${EXTRA_LDFLAGS}" ]]; then
	GO_BUILD_ARGS+=("-ldflags=${EXTRA_LDFLAGS}")
fi
GO_BUILD_ARGS+=(-buildmode=c-archive -o "${RUNTIME_DIR}/libyak.a" .)

go build "${GO_BUILD_ARGS[@]}"
rm -f "${RUNTIME_DIR}/libyak.h"

echo "Built ${RUNTIME_DIR}/libyak.a (Go Runtime)"
