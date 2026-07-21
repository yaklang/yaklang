#!/usr/bin/env bash
# After a full weekly recompile of ci-yaklang-base, drop promote/diff layers.
# Keeps the current CI_SSA_BASE_PROGRAM (normally ci-yaklang-base).
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
# shellcheck source=export-ssa-db-env.sh
source "$SCRIPT_DIR/export-ssa-db-env.sh"
export SSA_DATABASE_RAW

KEEP="${CI_SSA_BASE_PROGRAM:-ci-yaklang-base}"
echo "Keeping base program: $KEEP"
echo "Removing stale ci-yaklang-promote-* and ci-yaklang-diff-pr-* programs"

mapfile -t NAMES < <(
  ./yak ssa-program --database "$SSA_DATABASE_RAW" 2>/dev/null \
    | sed -n 's/^[[:space:]]*\[[^]]*\]:[[:space:]]*//p' \
    | sed 's/[[:space:]]*$//' \
    | grep -E '^(ci-yaklang-promote-|ci-yaklang-diff-pr-)' \
    | grep -vxF "$KEEP" || true
)

if [ "${#NAMES[@]}" -eq 0 ]; then
  echo "No stale overlay programs to remove"
  exit 0
fi

for name in "${NAMES[@]}"; do
  echo "Removing: $name"
  ./yak "$SCRIPT_DIR/remove-program.yak" \
    --database "sqlite://$SSA_DATABASE_RAW" \
    --program "$name" || echo "::warning::Failed to remove $name"
done

echo "Stale overlay cleanup done (${#NAMES[@]} program(s))"
