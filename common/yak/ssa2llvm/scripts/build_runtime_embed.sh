#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SSA2LLVM_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
REPO_ROOT="$(cd "${SSA2LLVM_DIR}/../../.." && pwd)"

RUNTIME_DIR="${SSA2LLVM_DIR}/runtime"
RUNTIMEEMBED_DIR="${SSA2LLVM_DIR}/runtime/embed"
STAGE_DIR="${RUNTIMEEMBED_DIR}/ssa2llvm-runtime"

echo "[ssa2llvm] building runtime archive..."
"${SSA2LLVM_DIR}/scripts/build_runtime_go.sh"

if [[ ! -f "${RUNTIME_DIR}/libyak.a" ]]; then
  echo "[ssa2llvm] runtime archive not found: ${RUNTIME_DIR}/libyak.a" >&2
  exit 1
fi

resolve_libgc() {
  local tools=("cc" "gcc" "clang")
  local tool=""
  local out=""
  for tool in "${tools[@]}"; do
    if ! command -v "${tool}" >/dev/null 2>&1; then
      continue
    fi
    out="$("${tool}" -print-file-name=libgc.a 2>/dev/null || true)"
    if [[ -n "${out}" && "${out}" != "libgc.a" && -f "${out}" ]]; then
      echo "${out}"
      return 0
    fi
  done
  return 1
}

LIBGC_PATH="$(resolve_libgc 2>/dev/null || true)"
if [[ -z "${LIBGC_PATH}" || "${LIBGC_PATH}" == "libgc.a" || ! -f "${LIBGC_PATH}" ]]; then
  echo "[ssa2llvm] libgc.a not found (need libgc-dev / bdwgc static library). Tried: cc/gcc/clang -print-file-name=libgc.a" >&2
  exit 1
fi

rm -rf "${STAGE_DIR}"
mkdir -p "${STAGE_DIR}"
cp "${RUNTIME_DIR}/libyak.a" "${STAGE_DIR}/libyak.a"
cp "${LIBGC_PATH}" "${STAGE_DIR}/libgc.a"

pushd "${RUNTIMEEMBED_DIR}" >/dev/null
echo "[ssa2llvm] generating ssa2llvm-runtime.tar.gz..."
go run "${REPO_ROOT}/common/utils/gzip_embed/gzip-embed/gzip-embed.go" \
  --no-embed \
  --root-path \
  --source ./ssa2llvm-runtime \
  --gz ssa2llvm-runtime.tar.gz
popd >/dev/null

rm -rf "${STAGE_DIR}"

TMP_DIR="$(mktemp -d)"
SRC_STAGE_DIR="${TMP_DIR}/ssa2llvm-runtime-src"
cleanup() {
  rm -rf "${TMP_DIR}"
}
trap cleanup EXIT

echo "[ssa2llvm] generating pruned runtime source tree..."
go run "${REPO_ROOT}/common/utils/gomodsrc/cmd" \
  --pkg ./common/yak/ssa2llvm/runtime/runtime_go \
  --dst "${SRC_STAGE_DIR}"

# Bundle libgc.a into the extracted source tree so runtime_go can build without system libgc.
mkdir -p "${SRC_STAGE_DIR}/common/yak/ssa2llvm/runtime/runtime_go/libs"
cp "${LIBGC_PATH}" "${SRC_STAGE_DIR}/common/yak/ssa2llvm/runtime/runtime_go/libs/libgc.a"

pushd "${RUNTIMEEMBED_DIR}" >/dev/null
echo "[ssa2llvm] generating ssa2llvm-runtime-src.tar.gz..."
go run "${REPO_ROOT}/common/utils/gzip_embed/gzip-embed/gzip-embed.go" \
  --no-embed \
  --root-path \
  --include-targz \
  --source "${SRC_STAGE_DIR}" \
  --gz ssa2llvm-runtime-src.tar.gz
popd >/dev/null

echo "[ssa2llvm] done: ${RUNTIMEEMBED_DIR}/ssa2llvm-runtime.tar.gz"
echo "[ssa2llvm] done: ${RUNTIMEEMBED_DIR}/ssa2llvm-runtime-src.tar.gz"
