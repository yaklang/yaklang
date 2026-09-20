#!/usr/bin/env bash
# Build the helper from this checkout once, including a native .exe on Windows.
# stdout is the executable path so callers can reuse it for every resource group.
set -euo pipefail
cd "$(dirname "$0")/.."
tool_temp_root="${RUNNER_TEMP:-${TMPDIR:-/tmp}}"
if command -v cygpath >/dev/null 2>&1; then
  tool_temp_root="$(cygpath -u "$tool_temp_root")"
fi
tool_dir="$(mktemp -d "$tool_temp_root/yak-gzip-embed.XXXXXX")"
# The packer must run on the host even when a caller selected a target GOOS.
GOOS="$(go env GOHOSTOS)"
GOARCH="$(go env GOHOSTARCH)"
export GOOS GOARCH
tool_path="$tool_dir/gzip-embed$(go env GOEXE)"
go build -o "$tool_path" ./common/utils/gzip_embed/gzip-embed
printf '%s\n' "$tool_path"
