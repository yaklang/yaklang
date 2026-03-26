#!/usr/bin/env bash
set -euo pipefail

ARTIFACT_PATH="${ARTIFACT_PATH:-}"
DEST_DIR="${DEST_DIR:-}"
ARTIFACT_EXPECT_PATHS="${ARTIFACT_EXPECT_PATHS:-}"
ARTIFACT_EXECUTABLE_PATHS="${ARTIFACT_EXECUTABLE_PATHS:-}"

if [[ -z "$ARTIFACT_PATH" || -z "$DEST_DIR" ]]; then
  echo "ERROR: ARTIFACT_PATH and DEST_DIR must be set"
  exit 1
fi

if [[ ! -f "$ARTIFACT_PATH" ]]; then
  echo "ERROR: Artifact file not found: $ARTIFACT_PATH"
  exit 1
fi

rm -rf "$DEST_DIR"
mkdir -p "$DEST_DIR"
tar -C "$DEST_DIR" -xzf "$ARTIFACT_PATH"

while IFS= read -r expected_path; do
  [[ -z "$expected_path" ]] && continue
  if [[ ! -e "$DEST_DIR/$expected_path" ]]; then
    echo "ERROR: expected unpacked path is missing: $DEST_DIR/$expected_path"
    exit 1
  fi
done <<< "$ARTIFACT_EXPECT_PATHS"

while IFS= read -r executable_path; do
  [[ -z "$executable_path" ]] && continue
  if [[ ! -e "$DEST_DIR/$executable_path" ]]; then
    echo "ERROR: executable path is missing after unpack: $DEST_DIR/$executable_path"
    exit 1
  fi
  chmod +x "$DEST_DIR/$executable_path"
done <<< "$ARTIFACT_EXECUTABLE_PATHS"

echo "Unpacked artifact $ARTIFACT_PATH into $DEST_DIR"
du -sh "$DEST_DIR" 2>/dev/null || true
