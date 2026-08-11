#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
builder="$script_dir/build-legion-node-engine-import-bundle.sh"
test_root="$(mktemp -d)"
trap 'rm -rf "$test_root"' EXIT

source_sha="$(git -C "$script_dir/../.." rev-parse HEAD)"
artifact_dir="$test_root/artifacts"
mkdir -p "$artifact_dir"
printf 'trusted-node\n' >"$artifact_dir/legion-smoke-node"
binary_sha="$(sha256sum "$artifact_dir/legion-smoke-node" | awk '{print $1}')"
binary_size="$(stat -c %s "$artifact_dir/legion-smoke-node")"
jq -n \
  --arg source_sha "$source_sha" \
  --arg binary_sha "$binary_sha" \
  --argjson binary_size "$binary_size" '
  {
    schema_version:"1",
    artifact_type:"legion-product-node",
    version:"legion-node-v1.2.3",
    source:{repository:"yaklang/yaklang",commit:$source_sha,dirty:false},
    ci:{provider:"github-actions",run_id:"123",workflow:"Build Trusted Legion Product Node"},
    recipe:{goos:"linux",goarch:"amd64"},
    capabilities:["yak.execute","hids"],
    binary:{sha256:$binary_sha,size:$binary_size}
  }
' >"$artifact_dir/PRODUCT_NODE_MANIFEST.json"

output="$test_root/yaklang-node-engine_legion-node-v1.2.3_linux_amd64.tar.gz"
"$builder" \
  --version legion-node-v1.2.3 \
  --source-sha "$source_sha" \
  --artifact-dir "$artifact_dir" \
  --output "$output"

gzip -t "$output"
(cd "$test_root" && sha256sum -c "$(basename "$output").sha256" >/dev/null)
extract_dir="$test_root/extracted"
mkdir -p "$extract_dir"
tar -xzf "$output" -C "$extract_dir"
[[ "$(find "$extract_dir" -maxdepth 1 -type f | wc -l)" == 4 ]]
(cd "$extract_dir" && sha256sum -c release-index.json.sha256 >/dev/null)
jq -e --arg source_sha "$source_sha" --arg binary_sha "$binary_sha" '
  .version == "legion-node-v1.2.3" and
  .source_sha == $source_sha and
  .producer == "yaklang-ci" and
  .sha256 == $binary_sha and
  .protocol_version == 1 and
  .rollback_safe == true and
  (.capability_keys | index("yak.execute") != null)
' "$extract_dir/manifest.json" >/dev/null
jq -e --arg binary_sha "$binary_sha" '
  .schema_version == 1 and .producer == "yaklang-ci" and
  .releases[0].version == "legion-node-v1.2.3" and
  .releases[0].binary_sha256 == $binary_sha and
  (.releases[0].producer_manifest_sha256 | test("^[0-9a-f]{64}$"))
' "$extract_dir/release-index.json" >/dev/null
cmp "$artifact_dir/legion-smoke-node" "$extract_dir/yaklang-node"

if "$builder" \
  --version bad-version \
  --source-sha "$source_sha" \
  --artifact-dir "$artifact_dir" \
  --output "$test_root/invalid.tar.gz"; then
  echo 'invalid engine version unexpectedly accepted' >&2
  exit 1
fi

echo 'Legion Node engine import bundle tests passed'
