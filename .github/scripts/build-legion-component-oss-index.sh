#!/usr/bin/env bash
set -euo pipefail

component=""
version=""
source_sha=""
artifact_dir=""
public_base_url=""
output=""
target_os="linux"
target_arch="amd64"

usage() {
  cat <<'EOF'
Usage: build-legion-component-oss-index.sh [options]

Required:
  --component product-node
  --version TAG
  --source-sha SHA
  --artifact-dir DIR
  --public-base-url URL
  --output FILE

Optional:
  --goos linux                    Default: linux
  --goarch amd64|arm64            Default: amd64

EOF
}

die() { printf '[legion-component-oss-index][error] %s\n' "$*" >&2; exit 1; }

while (($#)); do
  case "$1" in
    --component) component="$2"; shift 2 ;;
    --version) version="$2"; shift 2 ;;
    --source-sha) source_sha="$2"; shift 2 ;;
    --artifact-dir) artifact_dir="$2"; shift 2 ;;
    --public-base-url) public_base_url="$2"; shift 2 ;;
    --output) output="$2"; shift 2 ;;
    --goos) target_os="$2"; shift 2 ;;
    --goarch) target_arch="$2"; shift 2 ;;
    -h|--help) usage; exit 0 ;;
    *) die "unknown argument: $1" ;;
  esac
done

[[ "$target_os" == "linux" ]] || die "unsupported component target OS: $target_os"
case "$target_arch" in amd64|arm64) ;; *) die "unsupported component target architecture: $target_arch" ;; esac
platform_suffix=""
if [[ "$target_arch" != "amd64" ]]; then
  platform_suffix="_${target_os}_${target_arch}"
fi

for command in gzip jq sha256sum stat; do
  command -v "$command" >/dev/null 2>&1 || die "missing required command: $command"
done

[[ "$component" == "product-node" ]] || die "invalid --component"
[[ "$version" =~ ^legion-node-v[0-9]+(\.[0-9]+){2}(-[0-9A-Za-z.-]+)?$ ]] || die "invalid Product Node release version"
package_name="legion-product-node_${target_os}_${target_arch}.tar.gz"
manifest_name="PRODUCT_NODE_MANIFEST${platform_suffix}.json"
checksums_name="SHA256SUMS${platform_suffix}"
binary_name="legion-smoke-node${platform_suffix}"
manifest_type="legion-product-node"
runtime_manifest_name="SESSION_RUNTIME_MANIFEST${platform_suffix}.json"
runtime_archive_name="legion-ai-session-runtime_${target_os}_${target_arch}.docker.tar.gz"

[[ "$source_sha" =~ ^[0-9a-f]{40}$ ]] || die "invalid --source-sha"
[[ -d "$artifact_dir" ]] || die "artifact directory does not exist"
artifact_dir="$(cd "$artifact_dir" && pwd)"
expected_public_base_url="https://yaklang.oss-accelerate.aliyuncs.com/legion/components/$component/$version/$source_sha"
[[ "$public_base_url" == "$expected_public_base_url" ]] || \
  die "public base URL does not match the selected component, version, and source"
[[ -n "$output" ]] || die "--output is required"

manifest="$artifact_dir/$manifest_name"
package="$artifact_dir/$package_name"
checksums="$artifact_dir/$checksums_name"
package_checksum="$artifact_dir/$package_name.sha256"
runtime_manifest="$artifact_dir/$runtime_manifest_name"
runtime_archive="$artifact_dir/$runtime_archive_name"
for required in "$manifest" "$package" "$checksums" "$package_checksum" "$runtime_manifest" "$runtime_archive"; do
  [[ -f "$required" ]] || die "missing release artifact: $required"
done
gzip -t "$runtime_archive" || die "Runtime image archive is not gzip-compressed"

(
  cd "$artifact_dir"
  sha256sum -c "$package_name.sha256" >/dev/null
)
jq -e \
  --arg type "$manifest_type" \
  --arg version "$version" \
  --arg source_sha "$source_sha" \
  --arg target_os "$target_os" \
  --arg target_arch "$target_arch" '
    .schema_version == "1" and
    .artifact_type == $type and
    .version == $version and
    .source.repository == "yaklang/yaklang" and
    .source.commit == $source_sha and
    .source.dirty == false and
    .recipe.goos == $target_os and
    .recipe.goarch == $target_arch and
    .ci.provider == "github-actions" and
    (.ci.run_id | test("^[0-9]+$"))
  ' "$manifest" >/dev/null || die "producer manifest does not match the requested release"

file_record() {
  local path="$1"
  jq -cn \
    --arg path "$(basename "$path")" \
    --arg sha256 "$(sha256sum "$path" | awk '{print $1}')" \
    --argjson size "$(stat -c %s "$path")" \
    '{path:$path,sha256:$sha256,size:$size}'
}

package_record="$(file_record "$package")"
manifest_record="$(file_record "$manifest")"
checksums_record="$(file_record "$checksums")"
package_checksum_record="$(file_record "$package_checksum")"
runtime_manifest_record="$(file_record "$runtime_manifest")"
runtime_archive_record="$(file_record "$runtime_archive")"
mkdir -p "$(dirname "$output")"

binary="$artifact_dir/$binary_name"
[[ -f "$binary" ]] || die "missing release artifact: $binary"
binary_record="$(file_record "$binary")"
binary_sha="$(jq -er .binary.sha256 "$manifest")"
[[ "$binary_sha" == "$(sha256sum "$binary" | awk '{print $1}')" ]] || die "Product Node binary digest differs from its manifest"
runtime_image_ref="$(jq -er .image.ref "$runtime_manifest")"
runtime_image_id="${runtime_image_ref##*@}"
runtime_archive_sha="$(sha256sum "$runtime_archive" | awk '{print $1}')"
runtime_archive_size="$(stat -c %s "$runtime_archive")"
jq -e \
  --arg version "$version" \
  --arg source_sha "$source_sha" \
  --arg target_os "$target_os" \
  --arg target_arch "$target_arch" \
  --arg image_ref "$runtime_image_ref" \
  --arg archive_sha "$runtime_archive_sha" \
  --argjson archive_size "$runtime_archive_size" '
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
    .image.ref == $image_ref and
    .image.revision_label == $source_sha and
    .image.archive_sha256 == $archive_sha and
    .image.archive_size == $archive_size and
    (.capabilities | index("ai.session.runtime") != null) and
    (.capabilities | index("yak.execute") != null)
  ' "$runtime_manifest" >/dev/null || die "Runtime manifest does not match the Product Node release"
[[ "$runtime_image_ref" =~ ^[^[:space:]@]+@sha256:[0-9a-f]{64}$ ]] || die "Runtime image ref is not immutable"
jq -n \
  --arg version "$version" \
  --arg source_sha "$source_sha" \
  --arg base_url "$public_base_url" \
  --arg target_os "$target_os" \
  --arg target_arch "$target_arch" \
  --arg runtime_image_ref "$runtime_image_ref" \
  --arg runtime_image_id "$runtime_image_id" \
  --argjson package "$package_record" \
  --argjson package_checksum "$package_checksum_record" \
  --argjson manifest "$manifest_record" \
  --argjson checksums "$checksums_record" \
  --argjson binary "$binary_record" \
  --argjson runtime_manifest "$runtime_manifest_record" \
  --argjson runtime_image_archive "$runtime_archive_record" \
  '{
    schema_version:"1",
    artifact_type:"legion-component-oss-release",
    component:"product-node",
    version:$version,
    source:{repository:"yaklang/yaklang",commit:$source_sha},
    platform:{goos:$target_os,goarch:$target_arch},
    distribution:{provider:"aliyun-oss",base_url:$base_url},
    runtime:{image_ref:$runtime_image_ref,image_id:$runtime_image_id},
    artifacts:{
      package:$package,
      package_checksum:$package_checksum,
      manifest:$manifest,
      checksums:$checksums,
      binary:$binary,
      runtime_manifest:$runtime_manifest,
      runtime_image_archive:$runtime_image_archive
    }
  }' >"$output"

jq -e . "$output" >/dev/null
(
  cd "$(dirname "$output")"
  sha256sum "$(basename "$output")" >"$(basename "$output").sha256"
)
printf '[legion-component-oss-index] created %s\n' "$output"
