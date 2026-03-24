#!/usr/bin/env bash
set -euo pipefail

TEST_BIN_DIR="${TEST_BIN_DIR:-}"
YAK_BINARY_PATH="${YAK_BINARY_PATH:-}"
ARTIFACT_PATH="${ARTIFACT_PATH:-}"
PACKAGE_PATTERNS_FILE="${PACKAGE_PATTERNS_FILE:-}"
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

if [[ -n "$PACKAGE_PATTERNS_FILE" && ! -f "$PACKAGE_PATTERNS_FILE" ]]; then
  echo "ERROR: PACKAGE_PATTERNS_FILE not found: $PACKAGE_PATTERNS_FILE"
  exit 1
fi

mkdir -p "$(dirname "$ARTIFACT_PATH")"

stage_dir=$(mktemp -d)
cleanup() {
  rm -rf "$stage_dir"
}
trap cleanup EXIT

mkdir -p "$stage_dir/payload"

pkg_matches_pattern() {
  local pkg="$1"
  local pattern="$2"

  if [[ "$pkg" == "$pattern" ]]; then
    return 0
  fi

  if [[ "$pattern" == */... ]]; then
    local prefix="${pattern%/...}"
    if [[ "$pkg" == "$prefix" ]] || [[ "$pkg" == "$prefix"/* ]]; then
      return 0
    fi
  fi

  if [[ "$pattern" == */. ]]; then
    local exact="${pattern%/.}"
    if [[ "$pkg" == "$exact" ]]; then
      return 0
    fi
  fi

  return 1
}

should_include_package() {
  local pkg="$1"

  if [[ -z "$PACKAGE_PATTERNS_FILE" ]]; then
    return 0
  fi

  while IFS= read -r pattern; do
    [[ -z "$pattern" ]] && continue
    if pkg_matches_pattern "$pkg" "$pattern"; then
      return 0
    fi
  done < "$PACKAGE_PATTERNS_FILE"

  return 1
}

copy_filtered_test_binaries() {
  local dest_dir="$1"
  local manifest="$TEST_BIN_DIR/compiled_tests.txt"
  local copied=0
  local dest_manifest="$dest_dir/compiled_tests.txt"
  : > "$dest_manifest"

  while IFS= read -r bin; do
    [[ -z "$bin" ]] && continue
    local pkg_file="${bin}.package"
    [[ -f "$pkg_file" ]] || continue

    local pkg
    pkg="$(cat "$pkg_file")"
    if ! should_include_package "$pkg"; then
      continue
    fi

    local base
    base="$(basename "$bin")"
    cp -a "$bin" "$dest_dir/$base"
    cp -a "$pkg_file" "$dest_dir/${base}.package"
    echo "$base" >> "$dest_manifest"
    copied=1
  done < "$manifest"

  if [[ "$copied" == "0" ]]; then
    echo "ERROR: no test binaries matched PACKAGE_PATTERNS_FILE=$PACKAGE_PATTERNS_FILE"
    exit 1
  fi
}

dest_test_bin_dir="$stage_dir/payload/test_binaries"
mkdir -p "$dest_test_bin_dir"

if [[ -n "$PACKAGE_PATTERNS_FILE" ]]; then
  copy_filtered_test_binaries "$dest_test_bin_dir"
else
  cp -a "$TEST_BIN_DIR/." "$dest_test_bin_dir/"
fi

if [[ "$INCLUDE_YAK" == "1" ]]; then
  cp -a "$YAK_BINARY_PATH" "$stage_dir/payload/yak"
fi

tar -C "$stage_dir/payload" -cf - . | gzip -1 > "$ARTIFACT_PATH"

echo "Packaged test artifact to $ARTIFACT_PATH"
ls -lh "$ARTIFACT_PATH"
