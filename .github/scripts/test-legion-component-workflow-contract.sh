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

runtime_release_workflow="$repo_root/.github/workflows/build-legion-ai-session-runtime.yml"
runtime_workflow="$repo_root/.github/workflows/build-legion-ai-session-runtime-common.yml"
runtime_alpha_workflow="$repo_root/.github/workflows/build-legion-ai-session-runtime-alpha.yml"
[[ ! -e "$runtime_release_workflow" ]] || {
  echo "AI Session Runtime must not retain a separate formal release workflow" >&2
  exit 1
}
check_release_only_workflow "$runtime_alpha_workflow" 'legion-runtime-alpha-*'
grep -Fq 'workflow_call:' "$runtime_workflow"
grep -Fq 'source_mode: alpha' "$runtime_alpha_workflow"
grep -Fq 'retention_days: 7' "$runtime_alpha_workflow"
grep -Fq 'uses: ./.github/workflows/build-legion-ai-session-runtime-common.yml' "$runtime_alpha_workflow"
grep -Fq 'Source validation mode: alpha or unified' "$runtime_workflow"
grep -Fq 'refs/tags/legion-node-v' "$runtime_workflow"
grep -Fq '.docker.tar.gz' "$runtime_workflow"
# The Runtime packager verifies a clean Git checkout before generating
# provenance. Keep the image archive outside the checkout until that gate has
# passed, then copy it into the Actions handoff directory.
if ! grep -Fq 'id: runtime-archive' "$runtime_workflow"; then
  echo "$runtime_workflow must expose the runner-temp Runtime archive path" >&2
  exit 1
fi
# shellcheck disable=SC2016 # Match literal workflow shell syntax.
if ! grep -Fq 'archive_dir="${RUNNER_TEMP}/runtime-archive-${RUNTIME_GOARCH}"' "$runtime_workflow"; then
  echo "$runtime_workflow must stage the Runtime archive outside the checkout" >&2
  exit 1
fi
# shellcheck disable=SC2016 # Match literal GitHub Actions expression syntax.
if ! grep -Fq 'RUNTIME_IMAGE_TAG: ${{ steps.inspect.outputs.image_tag }}' "$runtime_workflow" ||
   ! grep -Fq 'docker save "$RUNTIME_IMAGE_TAG"' "$runtime_workflow"; then
  echo "$runtime_workflow must export a tagged Runtime image for containerd-backed Docker hosts" >&2
  exit 1
fi
# shellcheck disable=SC2016 # Match literal workflow shell syntax.
if grep -Fq 'docker save "$RUNTIME_IMAGE_ID"' "$runtime_workflow"; then
  echo "$runtime_workflow must not export the Runtime by bare image ID" >&2
  exit 1
fi
# shellcheck disable=SC2016 # Match literal GitHub Actions expression syntax.
if ! grep -Fq 'RUNTIME_IMAGE_ARCHIVE: ${{ steps.runtime-archive.outputs.path }}' "$runtime_workflow"; then
  echo "$runtime_workflow must pass the runner-temp archive into provenance generation" >&2
  exit 1
fi
# shellcheck disable=SC2016 # Match literal workflow shell syntax.
if ! grep -Fq 'cp "$RUNTIME_IMAGE_ARCHIVE" dist/artifact/' "$runtime_workflow"; then
  echo "$runtime_workflow must stage the verified Runtime archive for handoff" >&2
  exit 1
fi
# shellcheck disable=SC2016 # Reject the literal checkout-local archive path.
if grep -Fq '>"dist/artifact/${RUNTIME_PACKAGE_NAME}.docker.tar.gz"' "$runtime_workflow"; then
  echo "$runtime_workflow must not dirty the checkout before Runtime provenance is generated" >&2
  exit 1
fi
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
if grep -Eq 'legion-runtime-v|PUBLISH_OSS|OSS_KEY_(ID|SECRET)|upload-oss|components/session-runtime' "$runtime_workflow"; then
  echo "$runtime_workflow must produce handoff artifacts, not a separate Runtime release" >&2
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
grep -Fq 'build-session-runtimes:' "$product_workflow"
grep -Fq 'source_mode: unified' "$product_workflow"
grep -Fq 'producer_workflow_name: Build Trusted Legion Product Node' "$product_workflow"
grep -Fq 'Download matching AI Session Runtime artifacts' "$product_workflow"
grep -Fq 'ai-session-runtime.docker.tar.gz' "$repo_root/.github/scripts/build-legion-node-engine-import-bundle.sh"
grep -Fq 'ubuntu-24.04-arm' "$product_workflow"
grep -Fq 'ubuntu-24.04-arm' "$runtime_workflow"
grep -Fq 'release-index-linux-arm64.json' "$product_workflow"
grep -Fq 'build-legion-node-engine-import-bundle.sh' "$product_workflow"
grep -Fq 'environment: production-release' "$product_workflow"
grep -Fq 'LEGION_NODE_RELEASE_SIGNING_PRIVATE_KEY_B64' "$product_workflow"
grep -Fq 'LEGION_NODE_RELEASE_SIGNING_PUBLIC_KEY_B64' "$product_workflow"
grep -Fq 'release-index.json.sig' "$repo_root/.github/scripts/build-legion-node-engine-import-bundle.sh"
grep -Fq 'both Yaklang Node release signing values or neither' "$product_workflow"
if grep -Fq 'must configure the Yaklang Node release signing key pair' "$product_workflow"; then
  echo 'Yaklang Node signing is unexpectedly mandatory' >&2
  exit 1
fi
# shellcheck disable=SC2016 # Match literal workflow environment syntax.
grep -Fq 'yaklang-node-engine_${NODE_PACKAGE_VERSION}_${NODE_GOOS}_${NODE_GOARCH}.tar.gz' "$product_workflow"

echo 'Legion component workflow contract tests passed'
