#!/usr/bin/env bash
set -euo pipefail

version=""
source_sha=""
artifact_dir=""
output=""
target_os="linux"
target_arch="amd64"
signing_private_key_file=""
expected_public_key=""

usage() {
  cat <<'EOF'
Usage: build-legion-node-engine-import-bundle.sh [options]

Required:
  --version TAG
  --source-sha SHA
  --artifact-dir DIR
  --output FILE

Optional:
  --goos linux
  --goarch amd64|arm64
  --signing-private-key-file FILE
  --expected-public-key BASE64

The signing options must be provided together. If neither is provided, the
bundle is checksum-verified but does not contain release-index.json.sig.
EOF
}

die() { printf '[legion-node-engine-bundle][error] %s\n' "$*" >&2; exit 1; }

while (($#)); do
  case "$1" in
    --version) version="$2"; shift 2 ;;
    --source-sha) source_sha="$2"; shift 2 ;;
    --artifact-dir) artifact_dir="$2"; shift 2 ;;
    --output) output="$2"; shift 2 ;;
    --goos) target_os="$2"; shift 2 ;;
    --goarch) target_arch="$2"; shift 2 ;;
    --signing-private-key-file) signing_private_key_file="$2"; shift 2 ;;
    --expected-public-key) expected_public_key="$2"; shift 2 ;;
    -h|--help) usage; exit 0 ;;
    *) die "unknown argument: $1" ;;
  esac
done

[[ "$version" =~ ^legion-node-v[0-9]+(\.[0-9]+){2}(-[0-9A-Za-z.-]+)?$ ]] || die "invalid Product Node release version"
[[ "$source_sha" =~ ^[0-9a-f]{40}$ ]] || die "invalid --source-sha"
[[ "$target_os" == "linux" ]] || die "unsupported target OS: $target_os"
case "$target_arch" in amd64|arm64) ;; *) die "unsupported target architecture: $target_arch" ;; esac
[[ -d "$artifact_dir" ]] || die "artifact directory does not exist"
[[ -n "$output" ]] || die "--output is required"
if [[ -n "$signing_private_key_file" || -n "$expected_public_key" ]]; then
  [[ -f "$signing_private_key_file" && -n "$expected_public_key" ]] || \
    die "--signing-private-key-file and --expected-public-key must be configured together"
fi

for command in go gzip jq sha256sum stat tar; do
  command -v "$command" >/dev/null 2>&1 || die "missing required command: $command"
done

platform_suffix=""
if [[ "$target_arch" != "amd64" ]]; then
  platform_suffix="_${target_os}_${target_arch}"
fi
producer_manifest="$artifact_dir/PRODUCT_NODE_MANIFEST${platform_suffix}.json"
producer_binary="$artifact_dir/legion-smoke-node${platform_suffix}"
runtime_manifest="$artifact_dir/SESSION_RUNTIME_MANIFEST${platform_suffix}.json"
runtime_archive="$artifact_dir/legion-ai-session-runtime_${target_os}_${target_arch}.docker.tar.gz"
for required in "$producer_manifest" "$producer_binary" "$runtime_manifest" "$runtime_archive"; do
  [[ -f "$required" ]] || die "missing release artifact: $required"
done
gzip -t "$runtime_archive" || die "Runtime image archive is not a gzip-compressed Docker archive"

binary_sha="$(sha256sum "$producer_binary" | awk '{print $1}')"
binary_size="$(stat -c %s "$producer_binary")"
jq -e \
  --arg version "$version" \
  --arg source_sha "$source_sha" \
  --arg target_os "$target_os" \
  --arg target_arch "$target_arch" \
  --arg binary_sha "$binary_sha" \
  --argjson binary_size "$binary_size" '
    .schema_version == "1" and
    .artifact_type == "legion-product-node" and
    .version == $version and
    .source.repository == "yaklang/yaklang" and
    .source.commit == $source_sha and
    .source.dirty == false and
    .ci.provider == "github-actions" and
    .ci.workflow == "Build Trusted Legion Product Node" and
    .recipe.goos == $target_os and
    .recipe.goarch == $target_arch and
    .binary.sha256 == $binary_sha and
    .binary.size == $binary_size
  ' "$producer_manifest" >/dev/null || die "producer manifest does not describe this trusted release"

runtime_archive_sha="$(sha256sum "$runtime_archive" | awk '{print $1}')"
runtime_archive_size="$(stat -c %s "$runtime_archive")"
runtime_manifest_sha="$(sha256sum "$runtime_manifest" | awk '{print $1}')"
release_digest="$(printf 'yaklang-unified-release-v1\nnode:%s\nruntime:%s\n' "$binary_sha" "$runtime_archive_sha" | sha256sum | awk '{print $1}')"
release_id="sha256-$release_digest"
runtime_binary_sha="$(jq -er .binary.sha256 "$runtime_manifest")"
runtime_image_ref="$(jq -er .image.ref "$runtime_manifest")"
[[ "$runtime_image_ref" =~ ^[^[:space:]@]+@sha256:[0-9a-f]{64}$ ]] || die "Runtime manifest does not declare an immutable image ref"
runtime_image_id="${runtime_image_ref##*@}"
runtime_capabilities="$(jq -c '.capabilities | sort | unique' "$runtime_manifest")"
jq -e \
  --arg version "$version" \
  --arg source_sha "$source_sha" \
  --arg target_os "$target_os" \
  --arg target_arch "$target_arch" \
  --arg image_ref "$runtime_image_ref" \
  --arg runtime_archive_sha "$runtime_archive_sha" \
  --argjson runtime_archive_size "$runtime_archive_size" '
    .schema_version == "1" and
    .artifact_type == "legion-ai-session-runtime" and
    .version == $version and
    .source.repository == "yaklang/yaklang" and
    .source.commit == $source_sha and
    .source.dirty == false and
    .ci.provider == "github-actions" and
    .ci.workflow == "Build Trusted Legion Product Node" and
    .recipe.goos == $target_os and
    .recipe.goarch == $target_arch and
    .recipe.packaging_source_sha == $source_sha and
    .recipe.packaging_dirty == false and
    (.capabilities | index("ai.session.runtime") != null) and
    (.capabilities | index("yak.execute") != null) and
    (.binary.sha256 | test("^[0-9a-f]{64}$")) and
    (.binary.size > 0) and
    .image.ref == $image_ref and
    .image.revision_label == $source_sha and
    .image.archive_sha256 == $runtime_archive_sha and
    .image.archive_size == $runtime_archive_size
  ' "$runtime_manifest" >/dev/null || die "Runtime manifest does not describe the matching trusted release"

bundle_dir="$(mktemp -d)"
trap 'rm -rf "$bundle_dir"' EXIT
capabilities="$(jq -c '.capabilities | sort | unique' "$producer_manifest")"
jq -n \
  --arg release_id "$release_id" \
  --arg version "$version" \
  --arg source_sha "$source_sha" \
  --arg target_os "$target_os" \
  --arg target_arch "$target_arch" \
  --arg binary_sha "$binary_sha" \
  --argjson binary_size "$binary_size" \
  --argjson capabilities "$capabilities" \
  --arg runtime_manifest_sha "$runtime_manifest_sha" \
  --arg runtime_archive_sha "$runtime_archive_sha" \
  --argjson runtime_archive_size "$runtime_archive_size" \
  --arg runtime_image_ref "$runtime_image_ref" \
  --arg runtime_image_id "$runtime_image_id" \
  --arg runtime_binary_sha "$runtime_binary_sha" \
  --argjson runtime_capabilities "$runtime_capabilities" '
    {
      release_id:$release_id,
      version:$version,
      source_sha:$source_sha,
      producer:"yaklang-ci",
      operating_system:$target_os,
      architecture:$target_arch,
      object_key:"",
      sha256:$binary_sha,
      size:$binary_size,
      capability_keys:$capabilities,
      protocol_version:1,
      input_schema_versions:["job-input-v1"],
      result_schema_versions:["job-result-v1"],
      rollback_safe:true,
      runtime:{
        producer_manifest_sha256:$runtime_manifest_sha,
        archive_sha256:$runtime_archive_sha,
        archive_size:$runtime_archive_size,
        image_ref:$runtime_image_ref,
        image_id:$runtime_image_id,
        binary_sha256:$runtime_binary_sha,
        capability_keys:$runtime_capabilities
      }
    }
  ' >"$bundle_dir/manifest.json"

manifest_sha="$(sha256sum "$bundle_dir/manifest.json" | awk '{print $1}')"
jq -n \
  --arg version "$version" \
  --arg target_os "$target_os" \
    --arg target_arch "$target_arch" \
    --arg manifest_sha "$manifest_sha" \
    --arg binary_sha "$binary_sha" \
    --arg runtime_manifest_sha "$runtime_manifest_sha" \
    --arg runtime_archive_sha "$runtime_archive_sha" \
    --arg runtime_image_id "$runtime_image_id" '
    {
      schema_version:1,
      producer:"yaklang-ci",
      releases:[{
        version:$version,
        operating_system:$target_os,
        architecture:$target_arch,
        producer_manifest_sha256:$manifest_sha,
        binary_sha256:$binary_sha,
        runtime_manifest_sha256:$runtime_manifest_sha,
        runtime_archive_sha256:$runtime_archive_sha,
        runtime_image_id:$runtime_image_id
      }]
    }
  ' >"$bundle_dir/release-index.json"
(cd "$bundle_dir" && sha256sum release-index.json >release-index.json.sha256)
if [[ -n "$signing_private_key_file" ]]; then
  repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
  go run "$repo_root/.github/scripts/sign-legion-node-release-index.go" \
    --private-key-file "$signing_private_key_file" \
    --expected-public-key "$expected_public_key" \
    --input "$bundle_dir/release-index.json" \
    --output "$bundle_dir/release-index.json.sig"
fi
cp "$producer_binary" "$bundle_dir/yaklang-node"
cp "$runtime_manifest" "$bundle_dir/runtime-manifest.json"
cp "$runtime_archive" "$bundle_dir/ai-session-runtime.docker.tar.gz"

mkdir -p "$(dirname "$output")"
[[ ! -e "$output" ]] || die "output already exists: $output"
source_epoch="$(git -C "$(dirname "${BASH_SOURCE[0]}")/../.." show -s --format=%ct "$source_sha")"
archive_files=(
  ai-session-runtime.docker.tar.gz
  manifest.json
  release-index.json
  release-index.json.sha256
)
if [[ -n "$signing_private_key_file" ]]; then
  archive_files+=(release-index.json.sig)
fi
archive_files+=(runtime-manifest.json yaklang-node)
tar \
  --sort=name \
  --mtime="@$source_epoch" \
  --owner=0 \
  --group=0 \
  --numeric-owner \
  -C "$bundle_dir" \
  -cf - "${archive_files[@]}" | gzip -n -1 >"$output"
(cd "$(dirname "$output")" && sha256sum "$(basename "$output")" >"$(basename "$output").sha256")

printf '[legion-node-engine-bundle] created %s\n' "$output"
