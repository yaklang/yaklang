#!/usr/bin/env bash
set -euo pipefail

ARTIFACT_SOURCE_DIR="${ARTIFACT_SOURCE_DIR:-}"
ARTIFACT_COPY_LIST="${ARTIFACT_COPY_LIST:-}"
ARTIFACT_REQUIRE_PATHS="${ARTIFACT_REQUIRE_PATHS:-}"
ARTIFACT_PATH="${ARTIFACT_PATH:-}"

if [[ -z "$ARTIFACT_PATH" ]]; then
  echo "ERROR: ARTIFACT_PATH must be set"
  exit 1
fi

if [[ -z "$ARTIFACT_SOURCE_DIR" && -z "$ARTIFACT_COPY_LIST" ]]; then
  echo "ERROR: ARTIFACT_SOURCE_DIR or ARTIFACT_COPY_LIST must be set"
  exit 1
fi

while IFS= read -r required_path; do
  [[ -z "$required_path" ]] && continue
  if [[ ! -e "$required_path" ]]; then
    echo "ERROR: required path does not exist: $required_path"
    exit 1
  fi
done <<< "$ARTIFACT_REQUIRE_PATHS"

mkdir -p "$(dirname "$ARTIFACT_PATH")"

if [[ -n "$ARTIFACT_SOURCE_DIR" ]]; then
  if [[ ! -d "$ARTIFACT_SOURCE_DIR" ]]; then
    echo "ERROR: ARTIFACT_SOURCE_DIR does not exist: $ARTIFACT_SOURCE_DIR"
    exit 1
  fi
  tar -C "$ARTIFACT_SOURCE_DIR" -cf - . | gzip -1 > "$ARTIFACT_PATH"
else
  stage_dir="$(mktemp -d)"
  cleanup() {
    rm -rf "$stage_dir"
  }
  trap cleanup EXIT

  mkdir -p "$stage_dir/payload"
  while IFS= read -r item; do
    [[ -z "$item" ]] && continue

    src="${item%%|*}"
    dest="${item#*|}"
    if [[ "$src" == "$dest" ]]; then
      echo "ERROR: invalid ARTIFACT_COPY_LIST entry: $item"
      exit 1
    fi
    if [[ ! -e "$src" ]]; then
      echo "ERROR: source path does not exist: $src"
      exit 1
    fi

    mkdir -p "$stage_dir/payload/$(dirname "$dest")"
    cp -a "$src" "$stage_dir/payload/$dest"
  done <<< "$ARTIFACT_COPY_LIST"

  tar -C "$stage_dir/payload" -cf - . | gzip -1 > "$ARTIFACT_PATH"
fi

echo "Packaged artifact to $ARTIFACT_PATH"
ls -lh "$ARTIFACT_PATH"
