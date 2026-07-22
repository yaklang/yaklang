#!/usr/bin/env bash
# Build per-PR code-scan config from diff-code-scan.json template.
# Usage: generate-diff-scan-config.sh <pr_number> <short_sha> [out] [template]
set -euo pipefail

PR_NUMBER="${1:?PR number required}"
SHORT_SHA="${2:?short sha required}"
OUT="${3:-./scan-config.json}"
TEMPLATE="${4:-$(cd "$(dirname "$0")" && pwd)/diff-code-scan.json}"

# --- inline: export-ssa-db-env.sh ---
SSA_CI_DATA_DIR="${SSA_CI_DATA_DIR:-/data/ci-ssa}"
export SSA_CI_DATA_DIR
export SSA_DATABASE_RAW="${SSA_DATABASE_RAW:-$SSA_CI_DATA_DIR/default-yakssa.db}"
mkdir -p "$(dirname "$SSA_DATABASE_RAW")" "$SSA_CI_DATA_DIR"
if [ -z "${CI_SSA_BASE_PROGRAM:-}" ]; then
  POINTER="$SSA_CI_DATA_DIR/base-program-name"
  if [ -f "$POINTER" ]; then
    CI_SSA_BASE_PROGRAM="$(tr -d '[:space:]' < "$POINTER")"
  fi
  if [ -z "${CI_SSA_BASE_PROGRAM:-}" ]; then
    CI_SSA_BASE_PROGRAM="ci-yaklang-base"
  fi
fi
export CI_SSA_BASE_PROGRAM
# --- end inline ---

DIFF_NAME="ci-yaklang-diff-pr-${PR_NUMBER}-${SHORT_SHA}"
BASE_PROGRAM="${CI_SSA_BASE_PROGRAM:-ci-yaklang-base}"

jq \
  --arg name "$DIFF_NAME" \
  --arg base "$BASE_PROGRAM" \
  '.BaseInfo.program_names = [$name]
   | .SSACompile.base_program_name = $base
   | .SSACompile.enable_incremental_compile = true' \
  "$TEMPLATE" > "$OUT"

echo "Wrote $OUT (program_name=$DIFF_NAME base=$BASE_PROGRAM)"