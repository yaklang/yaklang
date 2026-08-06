#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

check_release_only_workflow() {
  local workflow="$1" expected_tag="$2" trigger_block
  trigger_block="$(awk '/^on:$/ { capture=1 } /^permissions:$/ { capture=0 } capture' "$workflow")"

  grep -Fxq "      - \"$expected_tag\"" <<<"$trigger_block" || {
    echo "$workflow does not declare the exact $expected_tag tag trigger" >&2
    exit 1
  }
  if grep -Eq '^[[:space:]]+(pull_request|workflow_dispatch|branches):' <<<"$trigger_block"; then
    echo "$workflow must remain tag-only" >&2
    exit 1
  fi
}

check_release_only_workflow \
  "$repo_root/.github/workflows/build-legion-product-node.yml" \
  'legion-node-v*'
check_release_only_workflow \
  "$repo_root/.github/workflows/build-legion-ai-session-runtime.yml" \
  'legion-runtime-v*'

grep -Fq 'ubuntu-24.04-arm' "$repo_root/.github/workflows/build-legion-product-node.yml"
grep -Fq 'ubuntu-24.04-arm' "$repo_root/.github/workflows/build-legion-ai-session-runtime.yml"
grep -Fq 'release-index-linux-arm64.json' "$repo_root/.github/workflows/build-legion-product-node.yml"
grep -Fq 'release-index-linux-arm64.json' "$repo_root/.github/workflows/build-legion-ai-session-runtime.yml"

echo 'Legion component workflow contract tests passed'
