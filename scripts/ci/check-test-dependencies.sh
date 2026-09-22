#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/../.."

# Check source as well as the selected build's graph so build-tagged tests
# cannot reintroduce instruction patching unnoticed.
forbidden='github.com/bytedance/mockey'
if git grep -n -F "$forbidden" -- '*.go' go.mod go.sum; then
  echo 'ERROR: runtime instruction patching is not allowed; use instance dependencies and real test databases.' >&2
  exit 1
fi
dependencies=$(go list -deps -test -f '{{.ImportPath}}' ./common/yakgrpc ./common/coreplugin)
if printf '%s\n' "$dependencies" | grep -F "$forbidden"; then
  echo 'ERROR: forbidden runtime patching library in the test dependency graph.' >&2
  exit 1
fi
echo 'Test dependency guard passed.'
