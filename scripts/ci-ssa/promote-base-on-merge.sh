#!/usr/bin/env bash
# Promote CI SSA base after a PR merges into main.
# Relative to the last manifest main_sha, incremental-compile tip into a new
# overlay program and switch the base pointer (scheme A).
#
# Usage: promote-base-on-merge.sh <new_main_sha> [pr_number]
set -euo pipefail

NEW_SHA="${1:?new main sha required}"
PR_NUMBER="${2:-}"

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
# shellcheck source=export-ssa-db-env.sh
source "$SCRIPT_DIR/export-ssa-db-env.sh"
export SSA_DATABASE_RAW

MANIFEST="$SSA_CI_DATA_DIR/manifest.json"
TEMPLATE="$SCRIPT_DIR/ci-yaklang-promote-compile.json"
PROMOTE_CONFIG="./promote-config.json"
FS_ZIP="./fs.zip"

if [ ! -f "$MANIFEST" ]; then
  echo "::error::Local manifest not found: $MANIFEST"
  echo "::error::Run workflow 'CI SSA Base Weekly' once so promote can track main_sha."
  exit 1
fi

# Acquire DB write lock before any compile/cleanup. GitHub concurrency group is
# a soft, single-runner assumption; flock guards the SQLite file when multiple
# self-hosted runners share the `ssa-ci` label. Must be sourced (not executed)
# so fd 9 stays open for this script's lifetime and the lock is not released
# when the helper subshell exits.
# shellcheck source=acquire-db-lock.sh
source "$SCRIPT_DIR/acquire-db-lock.sh"

"$SCRIPT_DIR/ensure-base-program.sh"

OLD_SHA=$(jq -r '.main_sha // empty' "$MANIFEST")
OLD_BASE=$(jq -r '.base_program_name // empty' "$MANIFEST")
OLD_DEPTH=$(jq -r '.overlay_depth // 0' "$MANIFEST" 2>/dev/null || echo 0)
case "$OLD_DEPTH" in ''|*[!0-9]*|null) OLD_DEPTH=0 ;; esac
if [ -z "$OLD_SHA" ] || [ "$OLD_SHA" = "null" ]; then
  echo "::error::manifest.main_sha is empty; run CI SSA Base Weekly first"
  exit 1
fi
if [ -z "$OLD_BASE" ] || [ "$OLD_BASE" = "null" ]; then
  OLD_BASE="${CI_SSA_BASE_PROGRAM:-ci-yaklang-base}"
fi

# Overlay chain depth guard. Each successful promote adds one layer on top of
# the effective base. When the chain exceeds the limit, flatten it into a
# single self-contained program via flatten-overlay.yak (recompiles the
# aggregated filesystem, so this is heavy — only triggers past the threshold).
OVERLAY_DEPTH_LIMIT="${CI_SSA_OVERLAY_DEPTH_LIMIT:-5}"
NEW_DEPTH=$((OLD_DEPTH + 1))
if [ "$NEW_DEPTH" -gt "$OVERLAY_DEPTH_LIMIT" ]; then
  echo "::notice::Overlay chain depth $NEW_DEPTH exceeds limit $OVERLAY_DEPTH_LIMIT; will flatten after promote."
  echo "::notice::flatten-overlay.yak recompiles the aggregated filesystem into a single program."
fi

echo "Promote: $OLD_SHA ($OLD_BASE) -> $NEW_SHA"

if [ "$OLD_SHA" = "$NEW_SHA" ]; then
  echo "Already at tip; no promote compile needed"
  if [ -n "$PR_NUMBER" ]; then
    "$SCRIPT_DIR/cleanup-pr-diff-programs.sh" "$PR_NUMBER" || true
  fi
  exit 0
fi

if ! git cat-file -e "${OLD_SHA}^{commit}" 2>/dev/null; then
  echo "::error::Old base sha $OLD_SHA not in this clone; fetch depth too shallow or history rewritten"
  exit 1
fi
if ! git merge-base --is-ancestor "$OLD_SHA" "$NEW_SHA"; then
  echo "::error::Base sha $OLD_SHA is not an ancestor of $NEW_SHA"
  echo "::error::History likely rewritten; run 'CI SSA Base Weekly' full recompile"
  exit 1
fi

echo "::group::Generating filesystem diff ($OLD_SHA..$NEW_SHA)"
rm -f "$FS_ZIP"
if ! yak gitefs --start "$OLD_SHA" --end "$NEW_SHA" --output "$FS_ZIP"; then
  echo "::error::yak gitefs failed"
  exit 1
fi
if [ ! -f "$FS_ZIP" ]; then
  echo "::error::fs.zip was not created"
  exit 1
fi

# Count file entries in zip (directories end with /)
FILE_COUNT=0
if command -v unzip >/dev/null 2>&1; then
  FILE_COUNT=$(unzip -Z1 "$FS_ZIP" 2>/dev/null | grep -cvE '/$' || true)
fi
FILE_COUNT=${FILE_COUNT:-0}
echo "Diff archive file entries: $FILE_COUNT"
echo "::endgroup::"

if [ "$FILE_COUNT" -eq 0 ]; then
  ZIP_SIZE=$(stat -c%s "$FS_ZIP" 2>/dev/null || stat -f%z "$FS_ZIP" || echo 0)
  if [ "${ZIP_SIZE:-0}" -lt 64 ]; then
    echo "Empty diff; advancing manifest.main_sha without new overlay"
    "$SCRIPT_DIR/write-local-manifest.sh" "$NEW_SHA" "$CI_SSA_BASE_PROGRAM" "" "$OLD_DEPTH"
    if [ -n "$PR_NUMBER" ]; then
      "$SCRIPT_DIR/cleanup-pr-diff-programs.sh" "$PR_NUMBER" || true
    fi
    exit 0
  fi
  echo "::warning::Could not enumerate zip entries but archive size=${ZIP_SIZE}; attempting promote compile"
fi

SHORT_SHA="${NEW_SHA:0:8}"
NEW_PROG="ci-yaklang-promote-${SHORT_SHA}"

jq \
  --arg name "$NEW_PROG" \
  --arg base "$CI_SSA_BASE_PROGRAM" \
  '.BaseInfo.program_names = [$name]
   | .SSACompile.base_program_name = $base
   | .SSACompile.enable_incremental_compile = true' \
  "$TEMPLATE" > "$PROMOTE_CONFIG"

echo "::group::Incremental promote compile -> $NEW_PROG (base=$CI_SSA_BASE_PROGRAM)"
cat "$PROMOTE_CONFIG"
if ! yak ssa-compile \
  --config "$PROMOTE_CONFIG" \
  --database "$SSA_DATABASE_RAW" \
  --file-perf-log; then
  echo "::error::Promote incremental compile failed"
  echo "::error::If base drifted too far, run 'CI SSA Base Weekly'"
  exit 1
fi
echo "::endgroup::"

if ! yak ssa-program "$NEW_PROG" --database "$SSA_DATABASE_RAW" 2>/dev/null | grep -qF "$NEW_PROG"; then
  echo "::error::Promote program '$NEW_PROG' not found in database after compile"
  exit 1
fi

export CI_SSA_BASE_PROGRAM="$NEW_PROG"
"$SCRIPT_DIR/write-local-manifest.sh" "$NEW_SHA" "$NEW_PROG" "" "$NEW_DEPTH"

if [ -n "$PR_NUMBER" ]; then
  "$SCRIPT_DIR/cleanup-pr-diff-programs.sh" "$PR_NUMBER" || true
fi

echo "Promote complete: effective base is now $NEW_PROG @ $NEW_SHA"

# If the overlay chain exceeds the depth limit, flatten it into a single
# self-contained program. This is a heavy full recompile (same order as
# weekly), so it only runs past the threshold. After flatten the effective
# base is a single-layer program and overlay_depth resets to 0.
if [ "$NEW_DEPTH" -gt "$OVERLAY_DEPTH_LIMIT" ]; then
  echo "::group::Flattening overlay chain (depth=$NEW_DEPTH > limit=$OVERLAY_DEPTH_LIMIT)"
  FLAT_NAME="ci-yaklang-flat-${SHORT_SHA}"
  FLATTEN_SCRIPT="$SCRIPT_DIR/flatten-overlay.yak"
  if [ -f "$FLATTEN_SCRIPT" ]; then
    if yak "$FLATTEN_SCRIPT" \
      --program "$NEW_PROG" \
      --output "$FLAT_NAME" \
      --database "$SSA_DATABASE_RAW" \
      --config "$SCRIPT_DIR/ci-yaklang-base-compile.json"; then
      # Flatten succeeded: update pointer to the single-layer program, reset depth.
      export CI_SSA_BASE_PROGRAM="$FLAT_NAME"
      "$SCRIPT_DIR/write-local-manifest.sh" "$NEW_SHA" "$FLAT_NAME" "" "0"
      echo "::endgroup::"
      echo "Flatten complete: effective base is now $FLAT_NAME @ $NEW_SHA (single-layer, depth=0)"
    else
      echo "::endgroup::"
      echo "::warning::Flatten failed; keeping overlay chain at depth $NEW_DEPTH."
      echo "::warning::PR scans will still work but will load $NEW_DEPTH layers. Next weekly will re-flatten."
    fi
  else
    echo "::endgroup::"
    echo "::warning::flatten-overlay.yak not found at $FLATTEN_SCRIPT; skipping flatten."
  fi
fi
