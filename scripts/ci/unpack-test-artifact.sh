#!/usr/bin/env bash
set -euo pipefail

ARTIFACT_PATH="${ARTIFACT_PATH:-}"
DEST_DIR="${DEST_DIR:-}"

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

if [[ ! -f "$DEST_DIR/test_binaries/compiled_tests.txt" ]]; then
  echo "ERROR: unpacked test binaries are incomplete"
  exit 1
fi

if [[ ! -f "$DEST_DIR/yak" ]]; then
  echo "ERROR: unpacked yak binary is missing"
  exit 1
fi

chmod +x "$DEST_DIR/yak"

echo "Unpacked prepared suite into $DEST_DIR"
du -sh "$DEST_DIR" 2>/dev/null || true
