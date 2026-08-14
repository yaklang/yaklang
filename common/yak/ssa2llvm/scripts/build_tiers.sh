#!/usr/bin/env bash
set -euo pipefail

# build_tiers.sh — Build every rung of the runtime tier ladder.
#
# A tier is a libyak.a built with a smaller yaklib module set, so the Go linker
# drops the unused modules' metadata (pclntab, type descriptors, itabs) as well
# as their code. Link-time pruning cannot do this: it edits an archive the Go
# linker has already finished with. See runtime/tiers for the ladder and
# docs/article-link-time-pruning.md for the measurements.
#
# Usage:
#   scripts/build_tiers.sh [output-dir]        # default: build/tiers
#
# Output layout, which is what SSA2LLVM_TIER_DIR expects:
#   <output-dir>/core/libyak.a
#   <output-dir>/net/libyak.a
#   <output-dir>/staticanalyze/libyak.a
#
# Each tier build overwrites runtime/embed/assets/, so the tier built last is
# the one a subsequent `go build` embeds. The ladder is built smallest-first and
# the largest tier must cover every module a script can ask for, so ending on it
# leaves the tree in the right state for a normal build.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SSA2LLVM_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
REPO_ROOT="$(cd "${SSA2LLVM_DIR}/../../.." && pwd)"

OUT_DIR="${1:-${REPO_ROOT}/build/tiers}"
mkdir -p "${OUT_DIR}"
# build_yaklib.sh chdirs into runtime_go, so a relative OUT_DIR would publish
# tiers under the wrong directory. Resolve it to an absolute path up front.
OUT_DIR="$(cd "${OUT_DIR}" && pwd)"

cd "${REPO_ROOT}"
TIER_LIST="$(go run ./common/yak/ssa2llvm/cmd/tiers list)"

for tier in ${TIER_LIST}; do
  echo "[tiers] building tier '${tier}'..."
  SSA2LLVM_TIER="${tier}" SSA2LLVM_TIER_OUT="${OUT_DIR}" \
    bash "${SCRIPT_DIR}/build_yaklib.sh"
done

echo "[tiers] ladder built in ${OUT_DIR}:"
for tier in ${TIER_LIST}; do
  printf '[tiers]   %-6s %s\n' "${tier}" "$(du -h "${OUT_DIR}/${tier}/libyak.a" | cut -f1)"
done
echo "[tiers] embedded assets now hold the last tier built ($(echo ${TIER_LIST} | awk '{print $NF}'))"
echo "[tiers] point the compiler at the ladder with: export SSA2LLVM_TIER_DIR=${OUT_DIR}"
