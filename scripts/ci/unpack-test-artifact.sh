#!/usr/bin/env bash
set -euo pipefail

ARTIFACT_PATH="${ARTIFACT_PATH:-}"
DEST_DIR="${DEST_DIR:-}"
REQUIRE_YAK="${REQUIRE_YAK:-1}"

if [[ -z "$ARTIFACT_PATH" || -z "$DEST_DIR" ]]; then
  echo "ERROR: ARTIFACT_PATH and DEST_DIR must be set"
  exit 1
fi

if [[ ! -f "$ARTIFACT_PATH" ]]; then
  echo "ERROR: Artifact file not found: $ARTIFACT_PATH"
  exit 1
fi

ARTIFACT_PATH="$ARTIFACT_PATH" DEST_DIR="$DEST_DIR" ./scripts/ci/unpack-artifact.sh

if [[ ! -f "$DEST_DIR/test_binaries/compiled_tests.txt" ]]; then
  echo "ERROR: unpacked test binaries are incomplete"
  exit 1
fi

manifest="$DEST_DIR/test_binaries/compiled_tests.txt"
find "$DEST_DIR/test_binaries" -maxdepth 1 -type f -name 'test_*' ! -name '*.log' ! -name '*.package' | sort > "$manifest"

if [[ "$REQUIRE_YAK" == "1" ]]; then
  if [[ ! -f "$DEST_DIR/yak" ]]; then
    echo "ERROR: unpacked yak binary is missing"
    exit 1
  fi

  chmod +x "$DEST_DIR/yak"
fi

echo "Unpacked prepared suite into $DEST_DIR"
