#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
package_name="${NODE_PACKAGE_NAME:-legion-product-node_linux_amd64}"
dist_dir="${NODE_DIST_DIR:-$repo_root/dist}"
package_dir="$dist_dir/$package_name"
package_path="$dist_dir/$package_name.tar.gz"

die() {
  printf '[legion-product-node][error] %s\n' "$*" >&2
  exit 1
}

log() {
  printf '[legion-product-node] %s\n' "$*"
}

for command in file git go gzip jq readelf sha256sum stat tar; do
  command -v "$command" >/dev/null 2>&1 || die "missing required command: $command"
done

source_sha="${SOURCE_SHA:-$(git -C "$repo_root" rev-parse HEAD)}"
[[ "$source_sha" =~ ^[0-9a-f]{40}$ ]] || die "invalid SOURCE_SHA: $source_sha"
[[ "$(git -C "$repo_root" rev-parse HEAD)" == "$source_sha" ]] || \
  die "checkout HEAD does not match SOURCE_SHA"

source_dirty=false
if [[ -n "$(git -C "$repo_root" status --porcelain --untracked-files=all)" ]]; then
  source_dirty=true
fi
if [[ "${CI:-}" == "true" && "$source_dirty" == "true" ]]; then
  die "CI checkout must be clean"
fi

version="${NODE_PACKAGE_VERSION:-sha-${source_sha:0:12}}"
[[ -n "$version" ]] || die "node package version must not be empty"
[[ ! -e "$package_dir" ]] || die "package directory already exists: $package_dir"
[[ ! -e "$package_path" ]] || die "package archive already exists: $package_path"

binary="$package_dir/legion-smoke-node"
build_tags="hids"
module_go_version="$(awk '$1 == "go" { gsub(/\r/, "", $2); print $2; exit }' "$repo_root/go.mod")"
[[ -n "$module_go_version" ]] || die "go.mod does not declare a Go version"
toolchain_version="$(go env GOVERSION)"
[[ "$toolchain_version" == "go$module_go_version" ]] || \
  die "Go toolchain $toolchain_version does not match go.mod version go$module_go_version"
mkdir -p "$package_dir"

log "building Linux amd64 HIDS node from $source_sha with $(go version)"
(
  cd "$repo_root"
  CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build \
    -trimpath \
    -buildvcs=true \
    -tags "$build_tags" \
    -ldflags '-s -w -buildid= -linkmode external -extldflags "-static"' \
    -o "$binary" \
    ./cmd/legion-smoke-node
)

file_description="$(file "$binary")"
printf '%s\n' "$file_description"
grep -q 'statically linked' <<<"$file_description" || die "node binary is not statically linked"
if readelf -d "$binary" 2>/dev/null | grep -q '(NEEDED)'; then
  die "node binary declares dynamic library dependencies"
fi

binary_sha="$(sha256sum "$binary" | awk '{print $1}')"
binary_size="$(stat -c %s "$binary")"
built_at="$(date -u +%Y-%m-%dT%H:%M:%SZ)"

jq -n \
  --arg version "$version" \
  --arg repository "${GITHUB_REPOSITORY:-yaklang/yaklang}" \
  --arg source_sha "$source_sha" \
  --argjson source_dirty "$source_dirty" \
  --arg module_go_version "$module_go_version" \
  --arg go_version "$(go version)" \
  --arg build_tags "$build_tags" \
  --arg binary_sha "$binary_sha" \
  --argjson binary_size "$binary_size" \
  --arg built_at "$built_at" \
  --arg ci_provider "${GITHUB_ACTIONS:+github-actions}" \
  --arg ci_run_id "${GITHUB_RUN_ID:-local}" \
  --arg ci_run_attempt "${GITHUB_RUN_ATTEMPT:-1}" \
  --arg ci_workflow "${GITHUB_WORKFLOW:-local}" \
  --arg ci_actor "${GITHUB_ACTOR:-local}" \
  '{
    schema_version: "1",
    artifact_type: "legion-product-node",
    version: $version,
    source: {
      repository: $repository,
      commit: $source_sha,
      dirty: $source_dirty
    },
    ci: {
      provider: (if ($ci_provider | length) > 0 then $ci_provider else "local" end),
      run_id: $ci_run_id,
      run_attempt: $ci_run_attempt,
      workflow: $ci_workflow,
      actor: $ci_actor
    },
    recipe: {
      goos: "linux",
      goarch: "amd64",
      cgo_enabled: true,
      link_mode: "external-static",
      build_tags: $build_tags,
      module_go_version: $module_go_version,
      go_version: $go_version
    },
    capabilities: ["hids", "ssa.rule_sync.export", "yak.execute"],
    binary: {
      path: "legion-smoke-node",
      sha256: $binary_sha,
      size: $binary_size
    },
    built_at: $built_at
  }' >"$package_dir/PRODUCT_NODE_MANIFEST.json"

(
  cd "$package_dir"
  sha256sum legion-smoke-node PRODUCT_NODE_MANIFEST.json >SHA256SUMS
  sha256sum -c SHA256SUMS
)

source_epoch="$(git -C "$repo_root" show -s --format=%ct "$source_sha")"
tar \
  --sort=name \
  --mtime="@$source_epoch" \
  --owner=0 \
  --group=0 \
  --numeric-owner \
  -C "$package_dir" \
  -cf - . | gzip -n -1 >"$package_path"
(
  cd "$dist_dir"
  sha256sum "$(basename "$package_path")" >"$(basename "$package_path").sha256"
)

log "package created: $package_path"
log "package checksum: $package_path.sha256"
