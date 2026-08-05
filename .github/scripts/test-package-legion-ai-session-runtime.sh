#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
packager="$repo_root/.github/scripts/package-legion-ai-session-runtime.sh"
test_root="$(mktemp -d)"
trap 'rm -rf -- "$test_root"' EXIT

source_sha="$(git -C "$repo_root" rev-parse HEAD)"
fake_digest="$(printf 'a%.0s' {1..64})"
base_digest="$(printf 'b%.0s' {1..64})"
builder_digest="$(printf 'c%.0s' {1..64})"
runtime_binary="$test_root/legion-session-runtime"
runtime_ldd="$test_root/runtime.ldd"
cp /bin/true "$runtime_binary"
ldd "$runtime_binary" >"$runtime_ldd"

SOURCE_SHA="$source_sha" \
RUNTIME_PACKAGE_VERSION="fixture-${source_sha:0:12}" \
RUNTIME_DIST_DIR="$test_root/dist" \
RUNTIME_BINARY="$runtime_binary" \
RUNTIME_LDD_FILE="$runtime_ldd" \
RUNTIME_IMAGE_REF="registry.example/legion-ai-session-runtime@sha256:$fake_digest" \
RUNTIME_IMAGE_TAG="registry.example/legion-ai-session-runtime:fixture" \
RUNTIME_BASE_IMAGE="debian:bookworm-slim@sha256:$base_digest" \
RUNTIME_BUILDER_IMAGE="golang:1.22.12-bookworm@sha256:$builder_digest" \
RUNTIME_GO_VERSION="go version go1.22.12 linux/amd64" \
RUNTIME_CI_PROVIDER="fixture" \
SESSION_RUNTIME_WORKFLOW_NAME="fixture" \
"$packager"

package_name="legion-ai-session-runtime_linux_amd64"
package_dir="$test_root/dist/$package_name"
manifest="$package_dir/SESSION_RUNTIME_MANIFEST.json"

(
  cd "$package_dir"
  sha256sum -c SHA256SUMS
)
(
  cd "$test_root/dist"
  sha256sum -c "$package_name.tar.gz.sha256"
)

jq -e \
  --arg source_sha "$source_sha" \
  --arg image_ref "registry.example/legion-ai-session-runtime@sha256:$fake_digest" '
    .schema_version == "1" and
    .artifact_type == "legion-ai-session-runtime" and
    .source.commit == $source_sha and
    .recipe.packaging_source_sha == $source_sha and
    .recipe.goos == "linux" and
    .recipe.goarch == "amd64" and
    .recipe.cgo_enabled == true and
    .recipe.link_mode == "dynamic-container" and
    .recipe.module_go_version == "1.22.12" and
    (.capabilities | index("ai.session.runtime")) != null and
    (.capabilities | index("yak.execute")) != null and
    .image.ref == $image_ref and
    .image.revision_label == $source_sha and
    (.binary.sha256 | test("^[0-9a-f]{64}$")) and
    (.recipe.dockerfile_sha256 | test("^[0-9a-f]{64}$")) and
    (.recipe.dockerignore_sha256 | test("^[0-9a-f]{64}$"))
  ' "$manifest" >/dev/null

printf '[legion-ai-session-runtime-test] PASS\n'
