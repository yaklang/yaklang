#!/usr/bin/env bash
# Remove SSA programs from the CI database.
#
# Usage:
#   cleanup-programs.sh pr <pr_number>        # remove ci-yaklang-diff-pr-{N}-*
#   cleanup-programs.sh stale                 # remove all promote/diff layers (keep base)
#   cleanup-programs.sh name <program_name>   # remove a single program
set -euo pipefail

MODE="${1:?mode required (pr|stale|name)}"
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

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
# --- end inline --#

remove_one() {
  local name="$1"
  ./yak "$SCRIPT_DIR/remove-program.yak" \
    --database "sqlite://$SSA_DATABASE_RAW" \
    --program "$name" || echo "::warning::Failed to remove $name"
}

list_programs() {
  ./yak ssa-program --database "$SSA_DATABASE_RAW" 2>/dev/null \
    | sed -n 's/^[[:space:]]*\[[^]]*\]:[[:space:]]*//p' \
    | sed 's/[[:space:]]*$//'
}

case "$MODE" in
  pr)
    PR_NUMBER="${2:?PR number required}"
    PATTERN="^ci-yaklang-diff-pr-${PR_NUMBER}-"
    echo "Cleaning PR #$PR_NUMBER diff programs matching: $PATTERN"
    mapfile -t NAMES < <(list_programs | grep -E "$PATTERN" || true)
    ;;
  stale)
    KEEP="${CI_SSA_BASE_PROGRAM:-ci-yaklang-base}"
    echo "Keeping base: $KEEP; removing stale promote/diff layers"
    mapfile -t NAMES < <(list_programs | grep -E '^(ci-yaklang-promote-|ci-yaklang-diff-pr-)' | grep -vxF "$KEEP" || true)
    ;;
  name)
    TARGET="${2:?program name required}"
    echo "Removing: $TARGET"
    remove_one "$TARGET"
    exit 0
    ;;
  *)
    echo "::error::Unknown mode '$MODE' (use: pr|stale|name)"
    exit 1
    ;;
esac

if [ "${#NAMES[@]}" -eq 0 ]; then
  echo "No programs to remove"
  exit 0
fi

for name in "${NAMES[@]}"; do
  echo "Removing: $name"
  remove_one "$name"
done
echo "Cleanup done (${#NAMES[@]} program(s))"