#!/usr/bin/env bash
# Source in CI: export SSA_DATABASE_RAW and ensure data directory exists.
set -euo pipefail

SSA_CI_DATA_DIR="${SSA_CI_DATA_DIR:-/data/ci-ssa}"
export SSA_CI_DATA_DIR
export SSA_DATABASE_RAW="${SSA_DATABASE_RAW:-$SSA_CI_DATA_DIR/default-yakssa.db}"

mkdir -p "$(dirname "$SSA_DATABASE_RAW")"
echo "SSA_CI_DATA_DIR=$SSA_CI_DATA_DIR"
echo "SSA_DATABASE_RAW=$SSA_DATABASE_RAW"
