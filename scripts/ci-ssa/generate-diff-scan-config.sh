#!/usr/bin/env bash
# Build per-PR code-scan config from diff-code-scan.json template.
set -euo pipefail

PR_NUMBER="${1:?PR number required}"
SHORT_SHA="${2:?short sha required}"
OUT="${3:-./scan-config.json}"
TEMPLATE="${4:-$(cd "$(dirname "$0")" && pwd)/diff-code-scan.json}"

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
# shellcheck source=export-ssa-db-env.sh
source "$SCRIPT_DIR/export-ssa-db-env.sh"

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
