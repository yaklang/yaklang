#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
packager="$repo_root/.github/scripts/package-legion-ai-session-runtime.sh"
test_root="$(mktemp -d)"
trap 'rm -rf -- "$test_root"' EXIT

source_sha="$(git -C "$repo_root" rev-parse HEAD)"
case "$(uname -m)" in
  x86_64) target_arch="amd64" ;;
  aarch64|arm64) target_arch="arm64" ;;
  *) echo "unsupported test host architecture: $(uname -m)" >&2; exit 1 ;;
esac
base_digest="$(printf 'b%.0s' {1..64})"
builder_digest="$(printf 'c%.0s' {1..64})"
runtime_binary="$test_root/legion-session-runtime"
runtime_ldd="$test_root/runtime.ldd"
runtime_archive="$test_root/runtime.docker.tar.gz"
runtime_image_tag="registry.example/legion-ai-session-runtime:fixture"
runtime_archive_root="$test_root/runtime-archive"
cp /bin/true "$runtime_binary"
ldd "$runtime_binary" >"$runtime_ldd"
mkdir -p "$runtime_archive_root/blobs/sha256"
printf '{"architecture":"%s","fixture":true}\n' "$target_arch" \
  >"$runtime_archive_root/runtime-config.json"
fake_digest="$(sha256sum "$runtime_archive_root/runtime-config.json" | awk '{print $1}')"
runtime_config_path="blobs/sha256/$fake_digest"
mv "$runtime_archive_root/runtime-config.json" "$runtime_archive_root/$runtime_config_path"

write_runtime_archive() {
  local output="$1" tag_mode="$2"
  case "$tag_mode" in
    tagged)
      jq -n --arg config "$runtime_config_path" --arg tag "$runtime_image_tag" \
        '[{Config:$config,RepoTags:[$tag],Layers:[]}]' \
        >"$runtime_archive_root/manifest.json"
      ;;
    untagged)
      jq -n --arg config "$runtime_config_path" \
        '[{Config:$config,RepoTags:null,Layers:[]}]' \
        >"$runtime_archive_root/manifest.json"
      ;;
    *)
      echo "unsupported Runtime archive fixture mode: $tag_mode" >&2
      exit 1
      ;;
  esac
  tar -C "$runtime_archive_root" -cf - manifest.json blobs | gzip -n >"$output"
}

run_packager() {
  local archive="$1" output_dir="$2"
  SOURCE_SHA="$source_sha" \
  RUNTIME_PACKAGE_VERSION="fixture-${source_sha:0:12}" \
  RUNTIME_DIST_DIR="$output_dir" \
  RUNTIME_BINARY="$runtime_binary" \
  RUNTIME_LDD_FILE="$runtime_ldd" \
  RUNTIME_IMAGE_REF="registry.example/legion-ai-session-runtime@sha256:$fake_digest" \
  RUNTIME_IMAGE_TAG="$runtime_image_tag" \
  RUNTIME_IMAGE_ARCHIVE="$archive" \
  RUNTIME_BASE_IMAGE="debian:bookworm-slim@sha256:$base_digest" \
  RUNTIME_BUILDER_IMAGE="golang:1.22.12-bookworm@sha256:$builder_digest" \
  RUNTIME_GOARCH="$target_arch" \
  RUNTIME_GO_VERSION="go version go1.22.12 linux/$target_arch" \
  RUNTIME_CI_PROVIDER="fixture" \
  SESSION_RUNTIME_WORKFLOW_NAME="fixture" \
  "$packager"
}

write_runtime_archive "$runtime_archive" tagged
runtime_archive_sha="$(sha256sum "$runtime_archive" | awk '{print $1}')"

run_packager "$runtime_archive" "$test_root/dist"

package_name="legion-ai-session-runtime_linux_${target_arch}"
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
  --arg image_ref "registry.example/legion-ai-session-runtime@sha256:$fake_digest" \
  --arg archive_sha "$runtime_archive_sha" \
  --arg target_arch "$target_arch" '
    .schema_version == "1" and
    .artifact_type == "legion-ai-session-runtime" and
    .source.commit == $source_sha and
    .recipe.packaging_source_sha == $source_sha and
    .recipe.goos == "linux" and
    .recipe.goarch == $target_arch and
    .recipe.cgo_enabled == true and
    .recipe.link_mode == "dynamic-container" and
    .recipe.module_go_version == "1.22.12" and
    (.capabilities | index("ai.input.managed_attachment.v1")) != null and
    (.capabilities | index("ai.session.bind_epoch.v1")) != null and
    (.capabilities | index("ai.session.turn_lifecycle.v1")) != null and
    (.capabilities | index("ai.session.runtime")) != null and
    (.capabilities | index("yak.execute")) != null and
    .image.ref == $image_ref and
    .image.revision_label == $source_sha and
    .image.archive_sha256 == $archive_sha and
    .image.archive_size > 0 and
    (.binary.sha256 | test("^[0-9a-f]{64}$")) and
    (.recipe.dockerfile_sha256 | test("^[0-9a-f]{64}$")) and
    (.recipe.dockerignore_sha256 | test("^[0-9a-f]{64}$"))
  ' "$manifest" >/dev/null

untagged_archive="$test_root/runtime-untagged.docker.tar.gz"
write_runtime_archive "$untagged_archive" untagged
if run_packager "$untagged_archive" "$test_root/untagged-dist" >/dev/null 2>&1; then
  echo 'Runtime packager accepted an archive without an importable image tag' >&2
  exit 1
fi

mismatched_archive="$test_root/runtime-config-mismatch.docker.tar.gz"
printf '{"tampered":true}\n' >"$runtime_archive_root/$runtime_config_path"
write_runtime_archive "$mismatched_archive" tagged
if run_packager "$mismatched_archive" "$test_root/mismatched-dist" >/dev/null 2>&1; then
  echo 'Runtime packager accepted a config blob that did not match the immutable image ID' >&2
  exit 1
fi

printf '[legion-ai-session-runtime-test] PASS\n'
