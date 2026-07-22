#!/usr/bin/env bash
# Promote CI SSA base after a PR merges into main.
#
# Relative to the last manifest main_sha, incremental-compile tip into a new
# overlay program and switch the base pointer. When overlay depth exceeds the
# limit, flatten the chain into a single program.
#
# Catch-up mode: if multiple PRs merged between runs, this loop advances
# manifest.main_sha one commit at a time until it reaches NEW_SHA. Each
# iteration compiles the diff for one commit range and stacks the overlay.
#
# Usage: promote-base-on-merge.sh <new_main_sha> [pr_number]
set -euo pipefail

NEW_SHA_TARGET="${1:?new main sha required}"
PR_NUMBER="${2:-}"

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

# ---------------------------------------------------------------------------
# Inline: export-ssa-db-env.sh — shared env bootstrap
# ---------------------------------------------------------------------------
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

MANIFEST="$SSA_CI_DATA_DIR/manifest.json"
TEMPLATE="$SCRIPT_DIR/ci-yaklang-promote-compile.json"
PROMOTE_CONFIG="./promote-config.json"
FS_ZIP="./fs.zip"

if [ ! -f "$MANIFEST" ]; then
  echo "::error::Local manifest not found: $MANIFEST"
  echo "::error::Run weekly full compile first so promote can track main_sha."
  exit 1
fi

# ---------------------------------------------------------------------------
# Inline: acquire-db-lock.sh — flock exclusive lock for DB writes
# ---------------------------------------------------------------------------
LOCK_FILE="${SSA_DATABASE_RAW}.lock"
if command -v flock >/dev/null 2>&1; then
  exec 9>"$LOCK_FILE"
  LOCK_TIMEOUT="${CI_SSA_DB_LOCK_TIMEOUT_SEC:-3600}"
  if ! flock --exclusive --timeout "$LOCK_TIMEOUT" 9; then
    echo "::error::Could not acquire DB lock $LOCK_FILE within ${LOCK_TIMEOUT}s."
    exit 1
  fi
  echo "Acquired DB write lock: $LOCK_FILE (timeout ${LOCK_TIMEOUT}s)"
else
  echo "::warning::flock not found; relying on single-process serialization only."
fi

# ---------------------------------------------------------------------------
# Inline: ensure-base-program.sh — verify base program exists + pointer consistency
# ---------------------------------------------------------------------------
BASE_PROGRAM="${CI_SSA_BASE_PROGRAM:-ci-yaklang-base}"
if [ ! -f "$SSA_DATABASE_RAW" ]; then
  echo "::error::SSA database not found: $SSA_DATABASE_RAW"
  exit 1
fi
if ! ./yak ssa-program "$BASE_PROGRAM" --database "$SSA_DATABASE_RAW" 2>/dev/null | grep -qF "$BASE_PROGRAM"; then
  echo "::error::Base program '$BASE_PROGRAM' not in database $SSA_DATABASE_RAW"
  echo "::error::Run weekly full compile first."
  exit 1
fi
# Cross-check manifest / pointer / env for drift.
MANIFEST_BASE=""
if [ -f "$MANIFEST" ]; then
  MANIFEST_BASE=$(jq -r '.base_program_name // empty' "$MANIFEST" 2>/dev/null || true)
fi
POINTER_BASE=""
if [ -f "$POINTER" ]; then
  POINTER_BASE="$(tr -d '[:space:]' < "$POINTER")"
fi
DRIFT=0
if [ -n "$MANIFEST_BASE" ] && [ "$MANIFEST_BASE" != "null" ] && [ "$MANIFEST_BASE" != "$BASE_PROGRAM" ]; then
  echo "::error::manifest.base_program_name='$MANIFEST_BASE' != effective base '$BASE_PROGRAM'"
  DRIFT=1
fi
if [ -n "$POINTER_BASE" ] && [ "$POINTER_BASE" != "$BASE_PROGRAM" ]; then
  echo "::error::pointer='$POINTER_BASE' != effective base '$BASE_PROGRAM'"
  DRIFT=1
fi
if [ -n "$MANIFEST_BASE" ] && [ "$MANIFEST_BASE" != "null" ] && [ -n "$POINTER_BASE" ] && [ "$MANIFEST_BASE" != "$POINTER_BASE" ]; then
  echo "::error::manifest='$MANIFEST_BASE' != pointer='$POINTER_BASE'"
  DRIFT=1
fi
if [ "$DRIFT" -ne 0 ]; then
  echo "::error::Base pointer drift. Run weekly full compile to re-flatten."
  exit 1
fi
echo "Base program OK: $BASE_PROGRAM"

# ---------------------------------------------------------------------------
# Inline: write-local-manifest.sh — write manifest + pointer file
# ---------------------------------------------------------------------------
write_manifest() {
  local M_SHA="$1"
  local M_BASE="$2"
  local M_DEPTH="${3:-0}"
  local YAK_VER
  YAK_VER=$(./yak version 2>/dev/null | head -1 || echo "")
  local NOW
  NOW=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
  local DB_SIZE=0
  if [ -f "$SSA_DATABASE_RAW" ]; then
    DB_SIZE=$(stat -c%s "$SSA_DATABASE_RAW" 2>/dev/null || stat -f%z "$SSA_DATABASE_RAW" 2>/dev/null || echo 0)
  fi
  local OUT="$SSA_CI_DATA_DIR/manifest.json"
  jq -n \
    --arg version "1" \
    --arg base "$M_BASE" \
    --arg sha "$M_SHA" \
    --arg yak "$YAK_VER" \
    --arg path "$SSA_DATABASE_RAW" \
    --argjson size "$DB_SIZE" \
    --argjson depth "$M_DEPTH" \
    --arg now "$NOW" \
    '{
      version: $version,
      base_program_name: $base,
      main_sha: $sha,
      overlay_depth: $depth,
      yak_version: $yak,
      database: { url: ("local://" + $path), sha256: "", size_bytes: $size, compression: "none" },
      updated_at: $now
    }' > "$OUT"
  printf '%s\n' "$M_BASE" > "$SSA_CI_DATA_DIR/base-program-name"
  echo "Wrote manifest: base=$M_BASE main_sha=$M_SHA depth=$M_DEPTH"
}

# ---------------------------------------------------------------------------
# promote_once: advance base by one range (OLD_SHA..NEW_SHA), stack a new
# overlay layer, update manifest, clean that PR's diff programs.
# ---------------------------------------------------------------------------
promote_once() {
  local NEW_SHA="$1"
  local PR_NUM="${2:-}"

  # Re-read manifest each iteration: the previous iteration may have advanced.
  local OLD_SHA OLD_BASE OLD_DEPTH
  OLD_SHA=$(jq -r '.main_sha // empty' "$MANIFEST")
  OLD_BASE=$(jq -r '.base_program_name // empty' "$MANIFEST")
  OLD_DEPTH=$(jq -r '.overlay_depth // 0' "$MANIFEST" 2>/dev/null || echo 0)
  case "$OLD_DEPTH" in ''|*[!0-9]*|null) OLD_DEPTH=0 ;; esac
  if [ -z "$OLD_SHA" ] || [ "$OLD_SHA" = "null" ]; then
    echo "::error::manifest.main_sha is empty; run weekly full compile first"
    return 1
  fi
  if [ -z "$OLD_BASE" ] || [ "$OLD_BASE" = "null" ]; then
    OLD_BASE="${CI_SSA_BASE_PROGRAM:-ci-yaklang-base}"
  fi

  local OVERLAY_DEPTH_LIMIT="${CI_SSA_OVERLAY_DEPTH_LIMIT:-5}"
  local NEW_DEPTH=$((OLD_DEPTH + 1))

  echo "Promote: $OLD_SHA ($OLD_BASE) -> $NEW_SHA"

  if [ "$OLD_SHA" = "$NEW_SHA" ]; then
    echo "Already at tip; no promote compile needed"
    if [ -n "$PR_NUM" ]; then
      "$SCRIPT_DIR/cleanup-programs.sh" pr "$PR_NUM" || true
    fi
    return 0
  fi

  if ! git cat-file -e "${OLD_SHA}^{commit}" 2>/dev/null; then
    echo "::error::Old base sha $OLD_SHA not in this clone"
    return 1
  fi
  if ! git merge-base --is-ancestor "$OLD_SHA" "$NEW_SHA"; then
    echo "::error::Base sha $OLD_SHA is not an ancestor of $NEW_SHA; history rewritten?"
    return 1
  fi

  echo "::group::Generating filesystem diff ($OLD_SHA..$NEW_SHA)"
  if [ "${FS_ZIP_PREBUILT:-0}" = "1" ]; then
    if [ ! -f "$FS_ZIP" ]; then
      echo "::error::FS_ZIP_PREBUILT=1 but $FS_ZIP not found"
      return 1
    fi
    echo "Using prebuilt $FS_ZIP"
  else
    rm -f "$FS_ZIP"
    if ! ./yak gitefs --start "$OLD_SHA" --end "$NEW_SHA" --output "$FS_ZIP"; then
      echo "::error::yak gitefs failed"
      return 1
    fi
  fi
  if [ ! -f "$FS_ZIP" ]; then
    echo "::error::fs.zip was not created"
    return 1
  fi

  local FILE_COUNT=0
  if command -v unzip >/dev/null 2>&1; then
    FILE_COUNT=$(unzip -Z1 "$FS_ZIP" 2>/dev/null | grep -cvE '/$' || true)
  fi
  FILE_COUNT=${FILE_COUNT:-0}
  echo "Diff archive file entries: $FILE_COUNT"
  echo "::endgroup::"

  if [ "$FILE_COUNT" -eq 0 ]; then
    local ZIP_SIZE
    ZIP_SIZE=$(stat -c%s "$FS_ZIP" 2>/dev/null || stat -f%z "$FS_ZIP" 2>/dev/null || echo 0)
    if [ "${ZIP_SIZE:-0}" -lt 64 ]; then
      echo "Empty diff; advancing manifest.main_sha without new overlay"
      write_manifest "$NEW_SHA" "$CI_SSA_BASE_PROGRAM" "$OLD_DEPTH"
      if [ -n "$PR_NUM" ]; then
        "$SCRIPT_DIR/cleanup-programs.sh" pr "$PR_NUM" || true
      fi
      return 0
    fi
  fi

  local SHORT_SHA="${NEW_SHA:0:8}"
  local NEW_PROG="ci-yaklang-promote-${SHORT_SHA}"

  # Remove stale program if it exists (retry safety).
  if ./yak ssa-program "$NEW_PROG" --database "$SSA_DATABASE_RAW" 2>/dev/null | grep -qF "$NEW_PROG"; then
    echo "Removing stale '$NEW_PROG' from DB before re-compile"
    "$SCRIPT_DIR/cleanup-programs.sh" name "$NEW_PROG" || true
  fi

  jq \
    --arg name "$NEW_PROG" \
    --arg base "$CI_SSA_BASE_PROGRAM" \
    '.BaseInfo.program_names = [$name]
     | .SSACompile.base_program_name = $base
     | .SSACompile.enable_incremental_compile = true' \
    "$TEMPLATE" > "$PROMOTE_CONFIG"

  echo "::group::Incremental promote compile -> $NEW_PROG (base=$CI_SSA_BASE_PROGRAM)"
  if ! ./yak ssa-compile \
    --config "$PROMOTE_CONFIG" \
    --database "$SSA_DATABASE_RAW" \
    --file-perf-log; then
    echo "::error::Promote incremental compile failed"
    return 1
  fi
  echo "::endgroup::"

  if ! ./yak ssa-program "$NEW_PROG" --database "$SSA_DATABASE_RAW" 2>/dev/null | grep -qF "$NEW_PROG"; then
    echo "::error::Promote program '$NEW_PROG' not found after compile"
    return 1
  fi

  export CI_SSA_BASE_PROGRAM="$NEW_PROG"
  write_manifest "$NEW_SHA" "$NEW_PROG" "$NEW_DEPTH"

  if [ -n "$PR_NUM" ]; then
    "$SCRIPT_DIR/cleanup-programs.sh" pr "$PR_NUM" || true
  fi

  echo "Promote complete: effective base is now $NEW_PROG @ $NEW_SHA"

  # Flatten if overlay chain exceeds depth limit.
  if [ "$NEW_DEPTH" -gt "$OVERLAY_DEPTH_LIMIT" ]; then
    echo "::group::Flattening overlay chain (depth=$NEW_DEPTH > limit=$OVERLAY_DEPTH_LIMIT)"
    local FLAT_NAME="ci-yaklang-flat-${SHORT_SHA}"
    local FLATTEN_SCRIPT="$SCRIPT_DIR/flatten-overlay.yak"
    if [ -f "$FLATTEN_SCRIPT" ]; then
      if ./yak "$FLATTEN_SCRIPT" \
        --program "$NEW_PROG" \
        --output "$FLAT_NAME" \
        --database "$SSA_DATABASE_RAW" \
        --config "$SCRIPT_DIR/ci-yaklang-base-compile.json"; then
        export CI_SSA_BASE_PROGRAM="$FLAT_NAME"
        write_manifest "$NEW_SHA" "$FLAT_NAME" "0"
        echo "::endgroup::"
        echo "Flatten complete: base is now $FLAT_NAME (single-layer, depth=0)"
      else
        echo "::endgroup::"
        echo "::warning::Flatten failed; keeping overlay chain at depth $NEW_DEPTH."
      fi
    else
      echo "::endgroup::"
      echo "::warning::flatten-overlay.yak not found; skipping flatten."
    fi
  fi
}

# ---------------------------------------------------------------------------
# Catch-up loop
# ---------------------------------------------------------------------------
CATCH_UP_MODE="${CI_SSA_PROMOTE_CATCH_UP:-1}"

if [ "$CATCH_UP_MODE" = "1" ]; then
  CURRENT_TARGET="$NEW_SHA_TARGET"
  ITERATION=0
  while true; do
    ITERATION=$((ITERATION + 1))
    MANIFEST_SHA_NOW=$(jq -r '.main_sha // empty' "$MANIFEST")
    if [ "$MANIFEST_SHA_NOW" = "$CURRENT_TARGET" ]; then
      echo "Catch-up complete: manifest.main_sha == target"
      break
    fi
    if [ "$ITERATION" -gt 50 ]; then
      echo "::error::Catch-up looped $ITERATION times; aborting"
      exit 1
    fi
    if [ "${FS_ZIP_PREBUILT:-0}" = "1" ] && [ "$ITERATION" -gt 1 ]; then
      echo "::error::FS_ZIP_PREBUILT=1 but catch-up needs iteration $ITERATION (multiple ranges)."
      echo "::error::Monitor must invoke promote once per range, or use yak gitefs."
      exit 1
    fi
    echo "=== Catch-up iteration $ITERATION ==="
    if ! promote_once "$CURRENT_TARGET" "$PR_NUMBER"; then
      echo "::error::promote_once failed on iteration $ITERATION"
      exit 1
    fi
  done
else
  promote_once "$NEW_SHA_TARGET" "$PR_NUMBER"
fi

echo "All promote work complete: base @ $NEW_SHA_TARGET"