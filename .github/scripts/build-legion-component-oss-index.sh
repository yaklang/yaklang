#!/usr/bin/env bash
set -euo pipefail

component=""
version=""
source_sha=""
artifact_dir=""
public_base_url=""
output=""
runtime_image_ref=""
runtime_image_id=""

usage() {
  cat <<'EOF'
Usage: build-legion-component-oss-index.sh [options]

Required:
  --component product-node|session-runtime
  --version TAG
  --source-sha SHA
  --artifact-dir DIR
  --public-base-url URL
  --output FILE

Required for session-runtime:
  --runtime-image-ref IMAGE@sha256:...
  --runtime-image-id sha256:...
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
    --runtime-image-ref) runtime_image_ref="$2"; shift 2 ;;
    --runtime-image-id) runtime_image_id="$2"; shift 2 ;;
    -h|--help) usage; exit 0 ;;
    *) die "unknown argument: $1" ;;
  esac
done

for command in jq sha256sum stat; do
  command -v "$command" >/dev/null 2>&1 || die "missing required command: $command"
done

case "$component" in
  product-node)
    [[ "$version" =~ ^legion-node-v[0-9]+(\.[0-9]+){2}(-[0-9A-Za-z.-]+)?$ ]] || die "invalid Product Node release version"
    package_name="legion-product-node_linux_amd64.tar.gz"
    manifest_name="PRODUCT_NODE_MANIFEST.json"
    checksums_name="SHA256SUMS"
    binary_name="legion-smoke-node"
    manifest_type="legion-product-node"
    ;;
  session-runtime)
    [[ "$version" =~ ^legion-runtime-v[0-9]+(\.[0-9]+){2}(-[0-9A-Za-z.-]+)?$ ]] || die "invalid Session Runtime release version"
    package_name="legion-ai-session-runtime_linux_amd64.tar.gz"
    manifest_name="SESSION_RUNTIME_MANIFEST.json"
    checksums_name="SHA256SUMS"
    image_archive_name="legion-ai-session-runtime_linux_amd64.docker.tar.gz"
    manifest_type="legion-ai-session-runtime"
    [[ "$runtime_image_ref" =~ ^[^[:space:]@]+@sha256:[0-9a-f]{64}$ ]] || die "invalid Runtime image ref"
    [[ "$runtime_image_id" =~ ^sha256:[0-9a-f]{64}$ ]] || die "invalid Runtime image ID"
    [[ "${runtime_image_ref##*@}" == "$runtime_image_id" ]] || die "Runtime logical image ref and image ID differ"
    ;;
  *) die "invalid --component" ;;
esac

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
for required in "$manifest" "$package" "$checksums" "$package_checksum"; do
  [[ -f "$required" ]] || die "missing release artifact: $required"
done

(
  cd "$artifact_dir"
  sha256sum -c "$package_name.sha256" >/dev/null
)
jq -e \
  --arg type "$manifest_type" \
  --arg version "$version" \
  --arg source_sha "$source_sha" '
    .schema_version == "1" and
    .artifact_type == $type and
    .version == $version and
    .source.repository == "yaklang/yaklang" and
    .source.commit == $source_sha and
    .source.dirty == false and
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
mkdir -p "$(dirname "$output")"

if [[ "$component" == product-node ]]; then
  binary="$artifact_dir/$binary_name"
  [[ -f "$binary" ]] || die "missing release artifact: $binary"
  binary_record="$(file_record "$binary")"
  binary_sha="$(jq -er .binary.sha256 "$manifest")"
  [[ "$binary_sha" == "$(sha256sum "$binary" | awk '{print $1}')" ]] || die "Product Node binary digest differs from its manifest"
  jq -n \
    --arg version "$version" \
    --arg source_sha "$source_sha" \
    --arg base_url "$public_base_url" \
    --argjson package "$package_record" \
    --argjson package_checksum "$package_checksum_record" \
    --argjson manifest "$manifest_record" \
    --argjson checksums "$checksums_record" \
    --argjson binary "$binary_record" \
    '{
      schema_version:"1",
      artifact_type:"legion-component-oss-release",
      component:"product-node",
      version:$version,
      source:{repository:"yaklang/yaklang",commit:$source_sha},
      distribution:{provider:"aliyun-oss",base_url:$base_url},
      artifacts:{package:$package,package_checksum:$package_checksum,manifest:$manifest,checksums:$checksums,binary:$binary}
    }' >"$output"
else
  image_archive="$artifact_dir/$image_archive_name"
  [[ -f "$image_archive" ]] || die "missing Runtime image archive: $image_archive"
  image_archive_record="$(file_record "$image_archive")"
  [[ "$(jq -er .image.ref "$manifest")" == "$runtime_image_ref" ]] || die "Runtime image ref differs from its manifest"
  jq -n \
    --arg version "$version" \
    --arg source_sha "$source_sha" \
    --arg base_url "$public_base_url" \
    --arg runtime_image_ref "$runtime_image_ref" \
    --arg runtime_image_id "$runtime_image_id" \
    --argjson package "$package_record" \
    --argjson package_checksum "$package_checksum_record" \
    --argjson manifest "$manifest_record" \
    --argjson checksums "$checksums_record" \
    --argjson image_archive "$image_archive_record" \
    '{
      schema_version:"1",
      artifact_type:"legion-component-oss-release",
      component:"session-runtime",
      version:$version,
      source:{repository:"yaklang/yaklang",commit:$source_sha},
      distribution:{provider:"aliyun-oss",base_url:$base_url},
      runtime:{image_ref:$runtime_image_ref,image_id:$runtime_image_id},
      artifacts:{package:$package,package_checksum:$package_checksum,manifest:$manifest,checksums:$checksums,image_archive:$image_archive}
    }' >"$output"
fi

jq -e . "$output" >/dev/null
(
  cd "$(dirname "$output")"
  sha256sum "$(basename "$output")" >"$(basename "$output").sha256"
)
printf '[legion-component-oss-index] created %s\n' "$output"
