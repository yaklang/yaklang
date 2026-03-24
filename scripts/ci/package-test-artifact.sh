#!/usr/bin/env bash
set -euo pipefail

TEST_BIN_DIR="${TEST_BIN_DIR:-}"
YAK_BINARY_PATH="${YAK_BINARY_PATH:-}"
ARTIFACT_PATH="${ARTIFACT_PATH:-}"
INCLUDE_YAK="${INCLUDE_YAK:-1}"

if [[ -z "$TEST_BIN_DIR" || -z "$ARTIFACT_PATH" ]]; then
  echo "ERROR: TEST_BIN_DIR and ARTIFACT_PATH must be set"
  exit 1
fi

if [[ ! -d "$TEST_BIN_DIR" ]]; then
  echo "ERROR: TEST_BIN_DIR does not exist: $TEST_BIN_DIR"
  exit 1
fi

if [[ ! -f "$TEST_BIN_DIR/compiled_tests.txt" ]]; then
  echo "ERROR: compiled_tests.txt not found in $TEST_BIN_DIR"
  exit 1
fi

if [[ "$INCLUDE_YAK" == "1" ]]; then
  if [[ -z "$YAK_BINARY_PATH" ]]; then
    echo "ERROR: YAK_BINARY_PATH must be set when INCLUDE_YAK=1"
    exit 1
  fi

  if [[ ! -x "$YAK_BINARY_PATH" ]]; then
    echo "ERROR: YAK binary is missing or not executable: $YAK_BINARY_PATH"
    exit 1
  fi
fi

mkdir -p "$(dirname "$ARTIFACT_PATH")"

stage_dir=$(mktemp -d)
cleanup() {
  rm -rf "$stage_dir"
}
trap cleanup EXIT

mkdir -p "$stage_dir/payload"
cp -a "$TEST_BIN_DIR" "$stage_dir/payload/test_binaries"

if [[ "$INCLUDE_YAK" == "1" ]]; then
  cp -a "$YAK_BINARY_PATH" "$stage_dir/payload/yak"
fi

tar -C "$stage_dir/payload" -cf - . | gzip -1 > "$ARTIFACT_PATH"

echo "Packaged test artifact to $ARTIFACT_PATH"
ls -lh "$ARTIFACT_PATH"
