#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

check_release_only_workflow() {
  local workflow="$1" expected_tag="$2" trigger_block
  trigger_block="$(tr -d '\r' <"$workflow" | awk '/^on:$/ { capture=1 } /^permissions:$/ { capture=0 } capture')"

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

runtime_release_workflow="$repo_root/.github/workflows/build-legion-ai-session-runtime.yml"
runtime_workflow="$repo_root/.github/workflows/build-legion-ai-session-runtime-common.yml"
runtime_alpha_workflow="$repo_root/.github/workflows/build-legion-ai-session-runtime-alpha.yml"
check_release_only_workflow "$runtime_alpha_workflow" 'legion-runtime-alpha-*'
grep -Fq 'workflow_call:' "$runtime_workflow"
grep -Fq 'source_mode: release' "$runtime_release_workflow"
grep -Fq 'retention_days: 14' "$runtime_release_workflow"
grep -Fq 'uses: ./.github/workflows/build-legion-ai-session-runtime-common.yml' "$runtime_release_workflow"
grep -Fq 'id-token: write' "$runtime_release_workflow"
grep -Fq 'attestations: write' "$runtime_release_workflow"
# shellcheck disable=SC2016 # Match literal GitHub expression syntax.
grep -Fq 'OSS_KEY_ID: ${{ secrets.OSS_KEY_ID }}' "$runtime_release_workflow"
# shellcheck disable=SC2016 # Match literal GitHub expression syntax.
grep -Fq 'OSS_KEY_SECRET: ${{ secrets.OSS_KEY_SECRET }}' "$runtime_release_workflow"
grep -Fq 'source_mode: alpha' "$runtime_alpha_workflow"
grep -Fq 'retention_days: 7' "$runtime_alpha_workflow"
grep -Fq 'uses: ./.github/workflows/build-legion-ai-session-runtime-common.yml' "$runtime_alpha_workflow"
grep -Fq 'PUBLISH_OSS' "$runtime_workflow"
grep -Fq 'if: env.PUBLISH_OSS == '\''true'\''' "$runtime_workflow"
[[ "$(grep -Fc "if: env.PUBLISH_OSS == 'true'" "$runtime_workflow")" -eq 4 ]]
grep -Fq '.docker.tar.gz' "$runtime_workflow"
grep -Fq 'contents: read' "$runtime_alpha_workflow"
if grep -Eq 'secrets: inherit|OSS_KEY_(ID|SECRET)|upload-oss' "$runtime_alpha_workflow"; then
  echo "$runtime_alpha_workflow must not receive Runtime release secrets or publish to OSS" >&2
  exit 1
fi
if grep -Eq 'id-token:|attestations:' "$runtime_alpha_workflow"; then
  echo "$runtime_alpha_workflow must remain a read-only Runtime candidate caller" >&2
  exit 1
fi
if grep -Eq '^[[:space:]]+(id-token|attestations):' "$runtime_workflow"; then
  echo "$runtime_workflow must inherit, not elevate, caller permissions" >&2
  exit 1
fi

runtime_alpha_tag='legion-runtime-alpha-0212'
[[ "$runtime_alpha_tag" == legion-runtime-alpha-* ]]
[[ "$runtime_alpha_tag" != legion-runtime-v* ]]

alpha_workflow="$repo_root/.github/workflows/build-legion-node-alpha.yml"
check_release_only_workflow "$alpha_workflow" 'legion-node-alpha-*'

grep -Fq 'workflow_call:' "$alpha_workflow"
grep -Fq 'macos-15-intel' "$alpha_workflow"
grep -Fq 'macos-15' "$alpha_workflow"
grep -Fq 'ubuntu-22.04' "$alpha_workflow"
grep -Fq 'ubuntu-24.04-arm' "$alpha_workflow"
grep -Fq './cmd/legion-smoke-node' "$alpha_workflow"
grep -Fq 'default: 7' "$alpha_workflow"
grep -Fq 'retention-days:' "$alpha_workflow"
grep -Fq 'inputs.retention_days || 7' "$alpha_workflow"
grep -Fq 'Actions artifact only; not published to OSS or a stable channel' "$alpha_workflow"
if grep -Eq 'pull-requests: read|/pulls/|OSS_KEY_(ID|SECRET)|upload-oss|build-legion-component-oss-index' "$alpha_workflow"; then
  echo "$alpha_workflow must not resolve PR metadata or publish alpha artifacts to OSS" >&2
  exit 1
fi
if grep -Fq -- '-tags hids' "$alpha_workflow"; then
  echo "$alpha_workflow must build the portable non-HIDS node" >&2
  exit 1
fi

alpha_tag='legion-node-alpha-0212'
[[ "$alpha_tag" == legion-node-alpha-* ]]
[[ "$alpha_tag" != legion-node-v* ]]

product_workflow="$repo_root/.github/workflows/build-legion-product-node.yml"
grep -Fq 'build-portable-nodes:' "$product_workflow"
grep -Fq 'uses: ./.github/workflows/build-legion-node-alpha.yml' "$product_workflow"
grep -Fq 'source_mode: release' "$product_workflow"
grep -Fq 'production_release: true' "$product_workflow"
grep -Fq 'ubuntu-24.04-arm' "$product_workflow"
grep -Fq 'ubuntu-24.04-arm' "$runtime_workflow"
grep -Fq 'release-index-linux-arm64.json' "$product_workflow"
grep -Fq 'release-index-linux-arm64.json' "$runtime_workflow"

echo 'Legion component workflow contract tests passed'
