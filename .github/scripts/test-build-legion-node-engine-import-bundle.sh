#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
builder="$script_dir/build-legion-node-engine-import-bundle.sh"
test_root="$(mktemp -d)"
trap 'rm -rf "$test_root"' EXIT

source_sha="$(git -C "$script_dir/../.." rev-parse HEAD)"
artifact_dir="$test_root/artifacts"
mkdir -p "$artifact_dir"
signing_key="$test_root/signing-key.b64"
printf '0123456789abcdef0123456789abcdef' | base64 | tr -d '\n' >"$signing_key"
printf '\n' >>"$signing_key"
signing_public_key="$(go run "$script_dir/sign-legion-node-release-index.go" \
  --private-key-file "$signing_key" --print-public-key)"
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

runtime_image_id="sha256:$(printf '3%.0s' {1..64})"
runtime_image_ref="yaklang-oss.invalid/legion-ai-session-runtime@$runtime_image_id"
runtime_binary_sha="$(printf '4%.0s' {1..64})"
printf 'trusted-runtime-image\n' | gzip -n >"$artifact_dir/legion-ai-session-runtime_linux_amd64.docker.tar.gz"
runtime_archive_sha="$(sha256sum "$artifact_dir/legion-ai-session-runtime_linux_amd64.docker.tar.gz" | awk '{print $1}')"
runtime_archive_size="$(stat -c %s "$artifact_dir/legion-ai-session-runtime_linux_amd64.docker.tar.gz")"
jq -n \
  --arg source_sha "$source_sha" \
  --arg image_ref "$runtime_image_ref" \
  --arg binary_sha "$runtime_binary_sha" \
  --arg archive_sha "$runtime_archive_sha" \
  --argjson archive_size "$runtime_archive_size" '
  {
    schema_version:"1",
    artifact_type:"legion-ai-session-runtime",
    version:"legion-node-v1.2.3",
    source:{repository:"yaklang/yaklang",commit:$source_sha,dirty:false},
    ci:{provider:"github-actions",run_id:"123",workflow:"Build Trusted Legion Product Node"},
    recipe:{goos:"linux",goarch:"amd64",packaging_source_sha:$source_sha,packaging_dirty:false},
    capabilities:["ai.session.bind_epoch.v1","ai.session.runtime","yak.execute"],
    binary:{sha256:$binary_sha,size:123},
    image:{ref:$image_ref,revision_label:$source_sha,archive_sha256:$archive_sha,archive_size:$archive_size}
  }
' >"$artifact_dir/SESSION_RUNTIME_MANIFEST.json"
runtime_manifest_sha="$(sha256sum "$artifact_dir/SESSION_RUNTIME_MANIFEST.json" | awk '{print $1}')"
release_digest="$(printf 'yaklang-unified-release-v1\nnode:%s\nruntime:%s\n' "$binary_sha" "$runtime_archive_sha" | sha256sum | awk '{print $1}')"
release_id="sha256-$release_digest"

output="$test_root/yaklang-node-engine_legion-node-v1.2.3_linux_amd64.tar.gz"
"$builder" \
  --version legion-node-v1.2.3 \
  --source-sha "$source_sha" \
  --artifact-dir "$artifact_dir" \
  --signing-private-key-file "$signing_key" \
  --expected-public-key "$signing_public_key" \
  --output "$output"

gzip -t "$output"
(cd "$test_root" && sha256sum -c "$(basename "$output").sha256" >/dev/null)
extract_dir="$test_root/extracted"
mkdir -p "$extract_dir"
tar -xzf "$output" -C "$extract_dir"
[[ "$(find "$extract_dir" -maxdepth 1 -type f | wc -l)" == 7 ]]
(cd "$extract_dir" && sha256sum -c release-index.json.sha256 >/dev/null)
cat >"$test_root/verify-signature.go" <<'EOF'
package main
import("crypto/ed25519";"encoding/base64";"os";"strings")
func main(){p,_:=base64.StdEncoding.DecodeString(os.Args[1]);m,_:=os.ReadFile(os.Args[2]);s0,_:=os.ReadFile(os.Args[3]);s,_:=base64.StdEncoding.DecodeString(strings.TrimSpace(string(s0)));if !ed25519.Verify(p,m,s){os.Exit(1)}}
EOF
go run "$test_root/verify-signature.go" "$signing_public_key" \
  "$extract_dir/release-index.json" "$extract_dir/release-index.json.sig"
jq -e \
  --arg source_sha "$source_sha" \
  --arg release_id "$release_id" \
  --arg binary_sha "$binary_sha" \
  --arg runtime_archive_sha "$runtime_archive_sha" \
  --arg runtime_manifest_sha "$runtime_manifest_sha" \
  --arg runtime_image_ref "$runtime_image_ref" '
  .release_id == $release_id and
  .version == "legion-node-v1.2.3" and
  .source_sha == $source_sha and
  .producer == "yaklang-ci" and
  .sha256 == $binary_sha and
  .protocol_version == 1 and
  .rollback_safe == true and
  (.capability_keys | index("yak.execute") != null) and
  .runtime.archive_sha256 == $runtime_archive_sha and
  .runtime.producer_manifest_sha256 == $runtime_manifest_sha and
  .runtime.image_ref == $runtime_image_ref and
  (.runtime.capability_keys | index("ai.session.runtime") != null)
' "$extract_dir/manifest.json" >/dev/null
jq -e \
  --arg binary_sha "$binary_sha" \
  --arg runtime_archive_sha "$runtime_archive_sha" \
  --arg runtime_manifest_sha "$runtime_manifest_sha" \
  --arg runtime_image_id "$runtime_image_id" '
  .schema_version == 1 and .producer == "yaklang-ci" and
  .releases[0].version == "legion-node-v1.2.3" and
  .releases[0].binary_sha256 == $binary_sha and
  .releases[0].runtime_archive_sha256 == $runtime_archive_sha and
  .releases[0].runtime_manifest_sha256 == $runtime_manifest_sha and
  .releases[0].runtime_image_id == $runtime_image_id and
  (.releases[0].producer_manifest_sha256 | test("^[0-9a-f]{64}$"))
' "$extract_dir/release-index.json" >/dev/null
cmp "$artifact_dir/legion-smoke-node" "$extract_dir/yaklang-node"
cmp "$artifact_dir/SESSION_RUNTIME_MANIFEST.json" "$extract_dir/runtime-manifest.json"
cmp "$artifact_dir/legion-ai-session-runtime_linux_amd64.docker.tar.gz" "$extract_dir/ai-session-runtime.docker.tar.gz"

unsigned_output="$test_root/yaklang-node-engine_legion-node-v1.2.3_linux_amd64-unsigned.tar.gz"
"$builder" \
  --version legion-node-v1.2.3 \
  --source-sha "$source_sha" \
  --artifact-dir "$artifact_dir" \
  --output "$unsigned_output"
gzip -t "$unsigned_output"
(cd "$test_root" && sha256sum -c "$(basename "$unsigned_output").sha256" >/dev/null)
unsigned_extract_dir="$test_root/unsigned-extracted"
mkdir -p "$unsigned_extract_dir"
tar -xzf "$unsigned_output" -C "$unsigned_extract_dir"
[[ "$(find "$unsigned_extract_dir" -maxdepth 1 -type f | wc -l)" == 6 ]]
[[ ! -e "$unsigned_extract_dir/release-index.json.sig" ]]
(cd "$unsigned_extract_dir" && sha256sum -c release-index.json.sha256 >/dev/null)
cmp "$artifact_dir/legion-smoke-node" "$unsigned_extract_dir/yaklang-node"
cmp "$artifact_dir/SESSION_RUNTIME_MANIFEST.json" "$unsigned_extract_dir/runtime-manifest.json"
cmp "$artifact_dir/legion-ai-session-runtime_linux_amd64.docker.tar.gz" "$unsigned_extract_dir/ai-session-runtime.docker.tar.gz"

if "$builder" \
  --version legion-node-v1.2.3 \
  --source-sha "$source_sha" \
  --artifact-dir "$artifact_dir" \
  --signing-private-key-file "$signing_key" \
  --output "$test_root/private-key-only.tar.gz"; then
  echo 'incomplete signing configuration unexpectedly accepted' >&2
  exit 1
fi

if "$builder" \
  --version legion-node-v1.2.3 \
  --source-sha "$source_sha" \
  --artifact-dir "$artifact_dir" \
  --expected-public-key "$signing_public_key" \
  --output "$test_root/public-key-only.tar.gz"; then
  echo 'incomplete signing configuration unexpectedly accepted' >&2
  exit 1
fi

tampered_runtime="$test_root/tampered-runtime"
cp -a "$artifact_dir" "$tampered_runtime"
printf 'tampered\n' >>"$tampered_runtime/legion-ai-session-runtime_linux_amd64.docker.tar.gz"
if "$builder" \
  --version legion-node-v1.2.3 \
  --source-sha "$source_sha" \
  --artifact-dir "$tampered_runtime" \
  --signing-private-key-file "$signing_key" \
  --expected-public-key "$signing_public_key" \
  --output "$test_root/tampered.tar.gz"; then
  echo 'tampered Runtime archive unexpectedly accepted' >&2
  exit 1
fi

if "$builder" \
  --version bad-version \
  --source-sha "$source_sha" \
  --artifact-dir "$artifact_dir" \
  --signing-private-key-file "$signing_key" \
  --expected-public-key "$signing_public_key" \
  --output "$test_root/invalid.tar.gz"; then
  echo 'invalid engine version unexpectedly accepted' >&2
  exit 1
fi

wrong_signing_key="$test_root/wrong-signing-key.b64"
printf 'fedcba9876543210fedcba9876543210' | base64 | tr -d '\n' >"$wrong_signing_key"
printf '\n' >>"$wrong_signing_key"
if "$builder" \
  --version legion-node-v1.2.3 \
  --source-sha "$source_sha" \
  --artifact-dir "$artifact_dir" \
  --signing-private-key-file "$wrong_signing_key" \
  --expected-public-key "$signing_public_key" \
  --output "$test_root/wrong-key.tar.gz"; then
  echo 'private key that does not match the approved public key unexpectedly accepted' >&2
  exit 1
fi

echo 'Legion Node engine import bundle tests passed'
