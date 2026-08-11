#!/usr/bin/env bash
set -euo pipefail

version=""
source_sha=""
artifact_dir=""
output=""
target_os="linux"
target_arch="amd64"

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

for command in gzip jq sha256sum stat tar; do
  command -v "$command" >/dev/null 2>&1 || die "missing required command: $command"
done

platform_suffix=""
if [[ "$target_arch" != "amd64" ]]; then
  platform_suffix="_${target_os}_${target_arch}"
fi
producer_manifest="$artifact_dir/PRODUCT_NODE_MANIFEST${platform_suffix}.json"
producer_binary="$artifact_dir/legion-smoke-node${platform_suffix}"
for required in "$producer_manifest" "$producer_binary"; do
  [[ -f "$required" ]] || die "missing release artifact: $required"
done

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

bundle_dir="$(mktemp -d)"
trap 'rm -rf "$bundle_dir"' EXIT
capabilities="$(jq -c '.capabilities | sort | unique' "$producer_manifest")"
jq -n \
  --arg version "$version" \
  --arg source_sha "$source_sha" \
  --arg target_os "$target_os" \
  --arg target_arch "$target_arch" \
  --arg binary_sha "$binary_sha" \
  --argjson binary_size "$binary_size" \
  --argjson capabilities "$capabilities" '
    {
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
      rollback_safe:true
    }
  ' >"$bundle_dir/manifest.json"

manifest_sha="$(sha256sum "$bundle_dir/manifest.json" | awk '{print $1}')"
jq -n \
  --arg version "$version" \
  --arg target_os "$target_os" \
  --arg target_arch "$target_arch" \
  --arg manifest_sha "$manifest_sha" \
  --arg binary_sha "$binary_sha" '
    {
      schema_version:1,
      producer:"yaklang-ci",
      releases:[{
        version:$version,
        operating_system:$target_os,
        architecture:$target_arch,
        producer_manifest_sha256:$manifest_sha,
        binary_sha256:$binary_sha
      }]
    }
  ' >"$bundle_dir/release-index.json"
(cd "$bundle_dir" && sha256sum release-index.json >release-index.json.sha256)
cp "$producer_binary" "$bundle_dir/yaklang-node"

mkdir -p "$(dirname "$output")"
[[ ! -e "$output" ]] || die "output already exists: $output"
source_epoch="$(git -C "$(dirname "${BASH_SOURCE[0]}")/../.." show -s --format=%ct "$source_sha")"
tar \
  --sort=name \
  --mtime="@$source_epoch" \
  --owner=0 \
  --group=0 \
  --numeric-owner \
  -C "$bundle_dir" \
  -cf - manifest.json release-index.json release-index.json.sha256 yaklang-node | gzip -n -1 >"$output"
(cd "$(dirname "$output")" && sha256sum "$(basename "$output")" >"$(basename "$output").sha256")

printf '[legion-node-engine-bundle] created %s\n' "$output"
