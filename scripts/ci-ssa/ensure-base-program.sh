#!/usr/bin/env bash
# Fail fast if weekly base program is missing from the CI SSA database.
set -euo pipefail

BASE_PROGRAM="${CI_SSA_BASE_PROGRAM:-ci-yaklang-base}"
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
# shellcheck source=export-ssa-db-env.sh
source "$SCRIPT_DIR/export-ssa-db-env.sh"

if [ ! -f "$SSA_DATABASE_RAW" ]; then
  echo "::error::SSA database not found: $SSA_DATABASE_RAW"
  echo "::error::Run workflow 'CI SSA Base Weekly' on self-hosted runner first."
  exit 1
fi

export SSA_DATABASE_RAW

if ! yak ssa-program "$BASE_PROGRAM" --database "$SSA_DATABASE_RAW" 2>/dev/null | grep -qF "$BASE_PROGRAM"; then
  echo "::error::Base program '$BASE_PROGRAM' not in database $SSA_DATABASE_RAW"
  echo "::error::Run workflow 'CI SSA Base Weekly' (schedule Friday or workflow_dispatch)."
  exit 1
fi

echo "Base program OK: $BASE_PROGRAM"
