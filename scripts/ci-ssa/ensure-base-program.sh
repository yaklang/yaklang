#!/usr/bin/env bash
# Fail fast if the effective base program is missing from the CI SSA database,
# and verify manifest / pointer / DB agree on which program is the effective
# base. Pointer drift is the most common CI failure: weekly deletes a
# promote-* overlay but manifest.main_sha still points at the deleted program,
# so the next promote compiles against a stale/missing base and corrupts every
# subsequent PR scan.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
# shellcheck source=export-ssa-db-env.sh
source "$SCRIPT_DIR/export-ssa-db-env.sh"

BASE_PROGRAM="${CI_SSA_BASE_PROGRAM:-ci-yaklang-base}"

if [ ! -f "$SSA_DATABASE_RAW" ]; then
  echo "::error::SSA database not found: $SSA_DATABASE_RAW"
  echo "::error::Run workflow 'CI SSA Base Weekly' on self-hosted runner first."
  exit 1
fi

export SSA_DATABASE_RAW

# 1. The effective base program must exist in the database.
if ! yak ssa-program "$BASE_PROGRAM" --database "$SSA_DATABASE_RAW" 2>/dev/null | grep -qF "$BASE_PROGRAM"; then
  echo "::error::Base program '$BASE_PROGRAM' not in database $SSA_DATABASE_RAW"
  echo "::error::Run workflow 'CI SSA Base Weekly' (schedule Friday or workflow_dispatch)."
  exit 1
fi

# 2. Cross-check manifest.base_program_name, the pointer file, and the env.
#    Allow missing manifest/pointer (first run after data dir wipe) but fail
#    hard on *conflicting* values, since that means a prior write was partial.
MANIFEST="$SSA_CI_DATA_DIR/manifest.json"
POINTER="$SSA_CI_DATA_DIR/base-program-name"
DRIFT=0

MANIFEST_BASE=""
if [ -f "$MANIFEST" ]; then
  MANIFEST_BASE=$(jq -r '.base_program_name // empty' "$MANIFEST" 2>/dev/null || true)
  if [ -n "$MANIFEST_BASE" ] && [ "$MANIFEST_BASE" != "null" ] && [ "$MANIFEST_BASE" != "$BASE_PROGRAM" ]; then
    echo "::error::manifest.base_program_name='$MANIFEST_BASE' != effective base '$BASE_PROGRAM'"
    DRIFT=1
  fi
fi

POINTER_BASE=""
if [ -f "$POINTER" ]; then
  POINTER_BASE="$(tr -d '[:space:]' < "$POINTER")"
  if [ -n "$POINTER_BASE" ] && [ "$POINTER_BASE" != "$BASE_PROGRAM" ]; then
    echo "::error::pointer file '$POINTER'='$POINTER_BASE' != effective base '$BASE_PROGRAM'"
    DRIFT=1
  fi
fi

if [ -n "$MANIFEST_BASE" ] && [ "$MANIFEST_BASE" != "null" ] && [ -n "$POINTER_BASE" ] && [ "$MANIFEST_BASE" != "$POINTER_BASE" ]; then
  echo "::error::manifest.base_program_name='$MANIFEST_BASE' != pointer='$POINTER_BASE'"
  DRIFT=1
fi

if [ "$DRIFT" -ne 0 ]; then
  echo "::error::Base pointer drift detected (manifest='$MANIFEST_BASE' pointer='$POINTER_BASE' env='$BASE_PROGRAM')."
  echo "::error::A prior weekly/promote write was likely interrupted. Run 'CI SSA Base Weekly' to re-flatten."
  exit 1
fi

echo "Base program OK: $BASE_PROGRAM (manifest='$MANIFEST_BASE' pointer='$POINTER_BASE')"
