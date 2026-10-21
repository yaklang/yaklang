#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
packager="$repo_root/.github/scripts/package-legion-ai-session-runtime.sh"
test_root="$(mktemp -d)"
trap 'rm -rf -- "$test_root"' EXIT

source_sha="$(git -C "$repo_root" rev-parse HEAD)"
case "$(uname -m)" in
  x86_64) target_arch="amd64"; builder_digest=6d405dfc5fdf3a45df1529cf060b920041f52ce523487e0f36f02765af294a51 ;;
  aarch64|arm64) target_arch="arm64"; builder_digest=7030196c59858ca8942e85aa2f68599240d58282ee35eb795609738a1e242c1f ;;
  *) echo "unsupported test host architecture: $(uname -m)" >&2; exit 1 ;;
esac
base_digest="$(printf 'b%.0s' {1..64})"
builder_image="docker.io/library/golang:1.22.12-alpine3.21@sha256:$builder_digest"
runtime_binary="$test_root/legion-session-runtime"
runtime_ldd="$test_root/runtime.ldd"
runtime_recipe="$test_root/musl-build.json"
runtime_archive="$test_root/runtime.docker.tar.gz"
runtime_image_tag="registry.example/legion-ai-session-runtime:fixture"
runtime_archive_root="$test_root/runtime-archive"
# Exercise real ELF and Go build-information checks without compiling the full
# Node. This fixture is local metadata evidence, not an official Node artifact.
command -v musl-gcc >/dev/null || { echo 'musl-tools is required for the static Runtime fixture' >&2; exit 1; }
cat >"$test_root/main.go" <<'GO'
package main
/*
#include <features.h>
#ifdef __GLIBC__
#error the fixture requires musl
#endif
extern const char __libc_version[];
static const char *version(void) { return __libc_version; }
*/
import "C"
import "fmt"
func main() { fmt.Println(C.GoString(C.version())) }
GO
CGO_ENABLED=1 GOOS=linux GOARCH="$target_arch" CC=musl-gcc GOTOOLCHAIN=local \
  go build -trimpath -buildvcs=false -tags musl,netgo,osusergo,sqlite_omit_load_extension,dameng \
    -ldflags '-s -w -buildid= -linkmode external -extldflags "-static"' \
    -o "$runtime_binary" "$test_root/main.go"
fixture_libc_version="$("$runtime_binary")"
printf 'statically linked (musl); no dynamic dependencies\n' >"$runtime_ldd"
jq -n --arg arch "$target_arch" --arg builder "$builder_image" \
  --arg version "$fixture_libc_version" --arg go_version "$(GOTOOLCHAIN=local go version)" \
  '{goos:"linux",goarch:$arch,builder_image:$builder,go_version:$go_version,
    c_library:"musl",c_library_version:$version,c_compiler:"fixture musl-gcc",
    cgo_enabled:true,link_mode:"external-static",builder_execution:"native",
    build_tags:"musl,netgo,osusergo,sqlite_omit_load_extension,dameng"}' >"$runtime_recipe"
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
  RUNTIME_MUSL_BUILD_METADATA="$runtime_recipe" \
  RUNTIME_IMAGE_REF="registry.example/legion-ai-session-runtime@sha256:$fake_digest" \
  RUNTIME_IMAGE_TAG="$runtime_image_tag" \
  RUNTIME_IMAGE_ARCHIVE="$archive" \
  RUNTIME_BASE_IMAGE="debian:bookworm-slim@sha256:$base_digest" \
  RUNTIME_BUILDER_IMAGE="$builder_image" \
  RUNTIME_GOARCH="$target_arch" \
  RUNTIME_GO_VERSION="$(GOTOOLCHAIN=local go version)" \
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
    .recipe.link_mode == "external-static" and
    .recipe.c_library == "musl" and
    .recipe.build_tags == "musl,netgo,osusergo,sqlite_omit_load_extension,dameng" and
    .recipe.module_go_version == "1.22.12" and
    (.capabilities | index("ai.input.managed_attachment.v1")) != null and
    (.capabilities | index("ai.session.bind_epoch.v1")) != null and
    (.capabilities | index("ai.session.turn_lifecycle.v1")) != null and
    (.capabilities | index("ai.skill_bundle.v1")) != null and
    (.capabilities | index("ai.session.runtime")) != null and
    (.capabilities | index("yak.execute")) != null and
    (.capabilities | index("ai.tools.builtin.v1")) != null and
    (.capabilities | map(select(startswith("ai.forge.discovery.") or startswith("ai.forge.evidence.") or startswith("ai.forge.http_assessment."))) | length == 0) and
    (.capabilities | index("ai.forge_release.v1")) != null and
    (.capabilities | index("ai.forge.custom_tools.v1")) != null and
    (.capabilities | index("hids")) == null and
    .image.ref == $image_ref and
    .image.revision_label == $source_sha and
    .image.archive_sha256 == $archive_sha and
    .image.archive_size > 0 and
    (.binary.sha256 | test("^[0-9a-f]{64}$")) and
    (.recipe.dockerfile_sha256 | test("^[0-9a-f]{64}$")) and
    (.recipe.dockerignore_sha256 | test("^[0-9a-f]{64}$"))
  ' "$manifest" >/dev/null

mv "$runtime_binary" "$runtime_binary.static"
cp /bin/true "$runtime_binary"
if run_packager "$runtime_archive" "$test_root/dynamic-dist" >/dev/null 2>&1; then
  echo 'Runtime packager accepted a dynamically linked binary labeled as musl static' >&2
  exit 1
fi
mv "$runtime_binary.static" "$runtime_binary"

cp "$runtime_recipe" "$runtime_recipe.original"
jq '.c_library = "glibc"' "$runtime_recipe.original" >"$runtime_recipe"
if run_packager "$runtime_archive" "$test_root/invalid-libc-dist" >/dev/null 2>&1; then
  echo 'Runtime packager accepted a libc recipe that is not musl' >&2
  exit 1
fi
mv "$runtime_recipe.original" "$runtime_recipe"

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
