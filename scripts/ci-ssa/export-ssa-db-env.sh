#!/usr/bin/env bash
# Source in CI: export SSA_DATABASE_RAW, CI_SSA_BASE_PROGRAM, ensure data directory exists.
set -euo pipefail

SSA_CI_DATA_DIR="${SSA_CI_DATA_DIR:-/data/ci-ssa}"
export SSA_CI_DATA_DIR
export SSA_DATABASE_RAW="${SSA_DATABASE_RAW:-$SSA_CI_DATA_DIR/default-yakssa.db}"

mkdir -p "$(dirname "$SSA_DATABASE_RAW")"
mkdir -p "$SSA_CI_DATA_DIR"

# Effective base program: explicit env > pointer file > default weekly name
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

echo "SSA_CI_DATA_DIR=$SSA_CI_DATA_DIR"
echo "SSA_DATABASE_RAW=$SSA_DATABASE_RAW"
echo "CI_SSA_BASE_PROGRAM=$CI_SSA_BASE_PROGRAM"
