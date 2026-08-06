#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
builder="$script_dir/build-legion-component-oss-index.sh"
test_root="$(mktemp -d)"
trap 'rm -rf "$test_root"' EXIT

source_sha="1111111111111111111111111111111111111111"

make_common() {
  local dir="$1" package="$2" manifest="$3"
  mkdir -p "$dir"
  printf 'package\n' >"$dir/$package"
  printf 'checksums\n' >"$dir/SHA256SUMS"
  (cd "$dir" && sha256sum "$package" >"$package.sha256")
  [[ -f "$dir/$manifest" ]]
}

node_dir="$test_root/node"
mkdir -p "$node_dir"
printf 'node\n' >"$node_dir/legion-smoke-node"
node_sha="$(sha256sum "$node_dir/legion-smoke-node" | awk '{print $1}')"
jq -n --arg sha "$source_sha" --arg binary_sha "$node_sha" '{
  schema_version:"1",artifact_type:"legion-product-node",version:"legion-node-v1.2.3-alpha.1",
  source:{repository:"yaklang/yaklang",commit:$sha,dirty:false},
  ci:{provider:"github-actions",run_id:"123"},recipe:{goos:"linux",goarch:"amd64"},
  binary:{sha256:$binary_sha}
}' >"$node_dir/PRODUCT_NODE_MANIFEST.json"
make_common "$node_dir" legion-product-node_linux_amd64.tar.gz PRODUCT_NODE_MANIFEST.json
"$builder" \
  --component product-node \
  --version legion-node-v1.2.3-alpha.1 \
  --source-sha "$source_sha" \
  --artifact-dir "$node_dir" \
  --public-base-url "https://yaklang.oss-accelerate.aliyuncs.com/legion/components/product-node/legion-node-v1.2.3-alpha.1/$source_sha" \
  --output "$node_dir/release-index.json"
jq -e --arg sha "$source_sha" '
  .component == "product-node" and .source.commit == $sha and
  .artifacts.binary.path == "legion-smoke-node" and
  (.artifacts.binary.sha256 | test("^[0-9a-f]{64}$"))
' "$node_dir/release-index.json" >/dev/null
(cd "$node_dir" && sha256sum -c release-index.json.sha256 >/dev/null)

node_arm_dir="$test_root/node-arm64"
mkdir -p "$node_arm_dir"
printf 'node-arm64\n' >"$node_arm_dir/legion-smoke-node_linux_arm64"
node_arm_sha="$(sha256sum "$node_arm_dir/legion-smoke-node_linux_arm64" | awk '{print $1}')"
jq -n --arg sha "$source_sha" --arg binary_sha "$node_arm_sha" '{
  schema_version:"1",artifact_type:"legion-product-node",version:"legion-node-v1.2.3-alpha.1",
  source:{repository:"yaklang/yaklang",commit:$sha,dirty:false},
  ci:{provider:"github-actions",run_id:"124"},recipe:{goos:"linux",goarch:"arm64"},
  binary:{sha256:$binary_sha}
}' >"$node_arm_dir/PRODUCT_NODE_MANIFEST_linux_arm64.json"
make_common "$node_arm_dir" legion-product-node_linux_arm64.tar.gz PRODUCT_NODE_MANIFEST_linux_arm64.json
mv "$node_arm_dir/SHA256SUMS" "$node_arm_dir/SHA256SUMS_linux_arm64"
"$builder" \
  --component product-node \
  --version legion-node-v1.2.3-alpha.1 \
  --source-sha "$source_sha" \
  --artifact-dir "$node_arm_dir" \
  --public-base-url "https://yaklang.oss-accelerate.aliyuncs.com/legion/components/product-node/legion-node-v1.2.3-alpha.1/$source_sha" \
  --goarch arm64 \
  --output "$node_arm_dir/release-index-linux-arm64.json"
jq -e '
  .component == "product-node" and .platform == {goos:"linux",goarch:"arm64"} and
  .artifacts.binary.path == "legion-smoke-node_linux_arm64" and
  .artifacts.manifest.path == "PRODUCT_NODE_MANIFEST_linux_arm64.json"
' "$node_arm_dir/release-index-linux-arm64.json" >/dev/null
(cd "$node_arm_dir" && sha256sum -c release-index-linux-arm64.json.sha256 >/dev/null)

runtime_dir="$test_root/runtime"
mkdir -p "$runtime_dir"
image_id="sha256:3333333333333333333333333333333333333333333333333333333333333333"
image_ref="yaklang-oss.invalid/legion-ai-session-runtime@$image_id"
printf 'image\n' >"$runtime_dir/legion-ai-session-runtime_linux_amd64.docker.tar.gz"
jq -n --arg sha "$source_sha" --arg image_ref "$image_ref" '{
  schema_version:"1",artifact_type:"legion-ai-session-runtime",version:"legion-runtime-v1.2.3",
  source:{repository:"yaklang/yaklang",commit:$sha,dirty:false},
  ci:{provider:"github-actions",run_id:"456"},recipe:{goos:"linux",goarch:"amd64"},
  image:{ref:$image_ref}
}' >"$runtime_dir/SESSION_RUNTIME_MANIFEST.json"
make_common "$runtime_dir" legion-ai-session-runtime_linux_amd64.tar.gz SESSION_RUNTIME_MANIFEST.json
"$builder" \
  --component session-runtime \
  --version legion-runtime-v1.2.3 \
  --source-sha "$source_sha" \
  --artifact-dir "$runtime_dir" \
  --public-base-url "https://yaklang.oss-accelerate.aliyuncs.com/legion/components/session-runtime/legion-runtime-v1.2.3/$source_sha" \
  --runtime-image-ref "$image_ref" \
  --runtime-image-id "$image_id" \
  --output "$runtime_dir/release-index.json"
jq -e --arg ref "$image_ref" --arg id "$image_id" '
  .component == "session-runtime" and .runtime.image_ref == $ref and .runtime.image_id == $id and
  .artifacts.image_archive.path == "legion-ai-session-runtime_linux_amd64.docker.tar.gz"
' "$runtime_dir/release-index.json" >/dev/null
(cd "$runtime_dir" && sha256sum -c release-index.json.sha256 >/dev/null)

runtime_arm_dir="$test_root/runtime-arm64"
mkdir -p "$runtime_arm_dir"
arm_image_id="sha256:5555555555555555555555555555555555555555555555555555555555555555"
arm_image_ref="yaklang-oss.invalid/legion-ai-session-runtime@$arm_image_id"
printf 'image-arm64\n' >"$runtime_arm_dir/legion-ai-session-runtime_linux_arm64.docker.tar.gz"
jq -n --arg sha "$source_sha" --arg image_ref "$arm_image_ref" '{
  schema_version:"1",artifact_type:"legion-ai-session-runtime",version:"legion-runtime-v1.2.3",
  source:{repository:"yaklang/yaklang",commit:$sha,dirty:false},
  ci:{provider:"github-actions",run_id:"457"},recipe:{goos:"linux",goarch:"arm64"},
  image:{ref:$image_ref}
}' >"$runtime_arm_dir/SESSION_RUNTIME_MANIFEST_linux_arm64.json"
make_common "$runtime_arm_dir" legion-ai-session-runtime_linux_arm64.tar.gz SESSION_RUNTIME_MANIFEST_linux_arm64.json
mv "$runtime_arm_dir/SHA256SUMS" "$runtime_arm_dir/SHA256SUMS_linux_arm64"
"$builder" \
  --component session-runtime \
  --version legion-runtime-v1.2.3 \
  --source-sha "$source_sha" \
  --artifact-dir "$runtime_arm_dir" \
  --public-base-url "https://yaklang.oss-accelerate.aliyuncs.com/legion/components/session-runtime/legion-runtime-v1.2.3/$source_sha" \
  --runtime-image-ref "$arm_image_ref" \
  --runtime-image-id "$arm_image_id" \
  --goarch arm64 \
  --output "$runtime_arm_dir/release-index-linux-arm64.json"
jq -e '
  .component == "session-runtime" and .platform == {goos:"linux",goarch:"arm64"} and
  .artifacts.image_archive.path == "legion-ai-session-runtime_linux_arm64.docker.tar.gz" and
  .artifacts.manifest.path == "SESSION_RUNTIME_MANIFEST_linux_arm64.json"
' "$runtime_arm_dir/release-index-linux-arm64.json" >/dev/null
(cd "$runtime_arm_dir" && sha256sum -c release-index-linux-arm64.json.sha256 >/dev/null)

if "$builder" \
  --component session-runtime \
  --version legion-runtime-v1.2.3 \
  --source-sha "$source_sha" \
  --artifact-dir "$runtime_dir" \
  --public-base-url "https://yaklang.oss-accelerate.aliyuncs.com/legion/components/session-runtime/legion-runtime-v1.2.3/$source_sha" \
  --runtime-image-ref "$image_ref" \
  --runtime-image-id sha256:4444444444444444444444444444444444444444444444444444444444444444 \
  --output "$test_root/mismatched-runtime.json"; then
  echo 'mismatched Runtime logical ref and image ID unexpectedly accepted' >&2
  exit 1
fi

if "$builder" \
  --component product-node \
  --version legion-node-v1.2.3 \
  --source-sha "$source_sha" \
  --artifact-dir "$node_dir" \
  --public-base-url "https://yaklang.oss-accelerate.aliyuncs.com/legion/components/product-node/legion-node-v1.2.3/$source_sha" \
  --goarch riscv64 \
  --output "$test_root/unsupported-platform.json"; then
  echo 'unsupported component architecture unexpectedly accepted' >&2
  exit 1
fi

if "$builder" \
  --component product-node \
  --version bad-tag \
  --source-sha "$source_sha" \
  --artifact-dir "$node_dir" \
  --public-base-url "https://yaklang.oss-accelerate.aliyuncs.com/legion/components/product-node/bad-tag/$source_sha" \
  --output "$test_root/invalid.json"; then
  echo 'invalid release tag unexpectedly accepted' >&2
  exit 1
fi

echo 'Legion component OSS index tests passed'
