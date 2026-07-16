#!/usr/bin/env bash
# Write CI SSA manifest to the self-hosted data dir (and optional extra path).
# Usage: write-local-manifest.sh <main_sha> <base_program_name> [extra_out_path]
set -euo pipefail

MAIN_SHA="${1:?main_sha required}"
BASE_PROGRAM="${2:?base_program_name required}"
EXTRA_OUT="${3:-}"

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
# shellcheck source=export-ssa-db-env.sh
source "$SCRIPT_DIR/export-ssa-db-env.sh"

YAK_VER=$(yak version 2>/dev/null | head -1 || echo "")
NOW=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
DB_SIZE=0
if [ -f "$SSA_DATABASE_RAW" ]; then
  DB_SIZE=$(stat -c%s "$SSA_DATABASE_RAW" 2>/dev/null || stat -f%z "$SSA_DATABASE_RAW")
fi

OUT_DIR="$SSA_CI_DATA_DIR"
mkdir -p "$OUT_DIR"
OUT="$OUT_DIR/manifest.json"

jq -n \
  --arg version "1" \
  --arg base "$BASE_PROGRAM" \
  --arg sha "$MAIN_SHA" \
  --arg yak "$YAK_VER" \
  --arg path "$SSA_DATABASE_RAW" \
  --argjson size "$DB_SIZE" \
  --arg now "$NOW" \
  '{
    version: $version,
    base_program_name: $base,
    main_sha: $sha,
    yak_version: $yak,
    database: {
      url: ("local://" + $path),
      sha256: "",
      size_bytes: $size,
      compression: "none"
    },
    updated_at: $now
  }' > "$OUT"

# Pointer used by export-ssa-db-env.sh / PR scans
printf '%s\n' "$BASE_PROGRAM" > "$OUT_DIR/base-program-name"

if [ -n "$EXTRA_OUT" ]; then
  cp "$OUT" "$EXTRA_OUT"
fi

echo "Wrote $OUT (base=$BASE_PROGRAM main_sha=$MAIN_SHA)"
cat "$OUT"
