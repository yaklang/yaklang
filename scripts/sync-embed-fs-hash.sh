#!/usr/bin/env bash
# Generate from this checkout, including metadata; a published binary cannot
# analyze source with this checkout's current extractor implementation.
set -euo pipefail
exec bash "$(dirname "$0")/generate-embed-fs-hash.sh" "$@"
