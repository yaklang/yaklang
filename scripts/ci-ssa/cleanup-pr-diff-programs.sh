#!/usr/bin/env bash
# Remove per-PR incremental scan programs: ci-yaklang-diff-pr-{N}-*
# Usage: cleanup-pr-diff-programs.sh <pr_number>
set -euo pipefail

PR_NUMBER="${1:?PR number required}"
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
# shellcheck source=export-ssa-db-env.sh
source "$SCRIPT_DIR/export-ssa-db-env.sh"
export SSA_DATABASE_RAW

PATTERN="^ci-yaklang-diff-pr-${PR_NUMBER}-"
echo "Cleaning PR #$PR_NUMBER diff programs matching: $PATTERN"

mapfile -t NAMES < <(
  ./yak ssa-program --database "$SSA_DATABASE_RAW" 2>/dev/null \
    | sed -n 's/^[[:space:]]*\[[^]]*\]:[[:space:]]*//p' \
    | sed 's/[[:space:]]*$//' \
    | grep -E "$PATTERN" || true
)

if [ "${#NAMES[@]}" -eq 0 ]; then
  echo "No diff programs to remove for PR #$PR_NUMBER"
  exit 0
fi

for name in "${NAMES[@]}"; do
  echo "Removing: $name"
  ./yak "$SCRIPT_DIR/remove-program.yak" \
    --database "sqlite://$SSA_DATABASE_RAW" \
    --program "$name" || echo "::warning::Failed to remove $name"
done

echo "Cleanup done (${#NAMES[@]} program(s))"
