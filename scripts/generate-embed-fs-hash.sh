#!/usr/bin/env bash
# Refresh metadata and every embed FS hash from this checkout in one pass.
# Kept as the existing CI/local entry point; no engine startup or database needed.
set -euo pipefail
exec bash "$(dirname "$0")/generate-builtin-metadata.sh" "$@"
