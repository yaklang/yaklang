#!/usr/bin/env bash
# CI hash refresh: the metadata artifacts belong in the same commit as the
# existing Go hash files. Release preparation itself never stages source files.
set -euo pipefail
cd "$(dirname "$0")/.."
bash scripts/generate-embed-fs-hash.sh "$@"
if [[ "${CI:-}" == "true" ]]; then
  for metadata_arg in "$@"; do
    if [[ "$metadata_arg" == "-check" || "$metadata_arg" == "--check" || "$metadata_arg" == "-check=true" || "$metadata_arg" == "--check=true" ]]; then
      exit 0
    fi
  done
  # Existing CI callers stage the Go hash files; include their gzip companions.
  git add -- common/consts/aitool-metadata.json.gz common/consts/coreplugin-metadata.json.gz \
    common/consts/forge-metadata.json.gz common/consts/syntaxflow-metadata.json.gz
fi
