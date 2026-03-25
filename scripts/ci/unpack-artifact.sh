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

echo "Unpacked artifact $ARTIFACT_PATH into $DEST_DIR"
du -sh "$DEST_DIR" 2>/dev/null || true
