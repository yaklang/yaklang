#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SSA2LLVM_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
REPO_ROOT="$(cd "${SSA2LLVM_DIR}/../../.." && pwd)"

RUNTIME_DIR="${SSA2LLVM_DIR}/runtime"
RUNTIMEEMBED_DIR="${SSA2LLVM_DIR}/runtimeembed"
STAGE_DIR="${RUNTIMEEMBED_DIR}/ssa2llvm-runtime"

echo "[ssa2llvm] building runtime archive..."
"${SSA2LLVM_DIR}/scripts/build_runtime_go.sh"

if [[ ! -f "${RUNTIME_DIR}/libyak.a" ]]; then
  echo "[ssa2llvm] runtime archive not found: ${RUNTIME_DIR}/libyak.a" >&2
  exit 1
fi

rm -rf "${STAGE_DIR}"
mkdir -p "${STAGE_DIR}"
cp "${RUNTIME_DIR}/libyak.a" "${STAGE_DIR}/libyak.a"

pushd "${RUNTIMEEMBED_DIR}" >/dev/null
echo "[ssa2llvm] generating ssa2llvm-runtime.tar.gz..."
go run "${REPO_ROOT}/common/utils/gzip_embed/gzip-embed/gzip-embed.go" \
  --no-embed \
  --root-path \
  --source ./ssa2llvm-runtime \
  --gz ssa2llvm-runtime.tar.gz
popd >/dev/null

rm -rf "${STAGE_DIR}"

echo "[ssa2llvm] done: ${RUNTIMEEMBED_DIR}/ssa2llvm-runtime.tar.gz"

