#!/usr/bin/env bash
set -euo pipefail

ARTIFACT_SOURCE_DIR="${ARTIFACT_SOURCE_DIR:-}"
ARTIFACT_PATH="${ARTIFACT_PATH:-}"

if [[ -z "$ARTIFACT_SOURCE_DIR" || -z "$ARTIFACT_PATH" ]]; then
  echo "ERROR: ARTIFACT_SOURCE_DIR and ARTIFACT_PATH must be set"
  exit 1
fi

if [[ ! -d "$ARTIFACT_SOURCE_DIR" ]]; then
  echo "ERROR: ARTIFACT_SOURCE_DIR does not exist: $ARTIFACT_SOURCE_DIR"
  exit 1
fi

mkdir -p "$(dirname "$ARTIFACT_PATH")"
tar -C "$ARTIFACT_SOURCE_DIR" -cf - . | gzip -1 > "$ARTIFACT_PATH"

echo "Packaged artifact from $ARTIFACT_SOURCE_DIR to $ARTIFACT_PATH"
ls -lh "$ARTIFACT_PATH"
