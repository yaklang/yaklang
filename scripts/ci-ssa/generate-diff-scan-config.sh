#!/usr/bin/env bash
# Build per-PR code-scan config from diff-code-scan.json template.
set -euo pipefail

PR_NUMBER="${1:?PR number required}"
SHORT_SHA="${2:?short sha required}"
OUT="${3:-./scan-config.json}"
TEMPLATE="${4:-$(cd "$(dirname "$0")" && pwd)/diff-code-scan.json}"

DIFF_NAME="ci-yaklang-diff-pr-${PR_NUMBER}-${SHORT_SHA}"
jq --arg name "$DIFF_NAME" '.BaseInfo.program_names = [$name]' "$TEMPLATE" > "$OUT"
echo "Wrote $OUT (program_name=$DIFF_NAME)"
