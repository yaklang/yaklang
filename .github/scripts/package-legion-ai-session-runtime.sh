#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
target_os="${RUNTIME_GOOS:-linux}"
target_arch="${RUNTIME_GOARCH:-amd64}"
package_name="${RUNTIME_PACKAGE_NAME:-legion-ai-session-runtime_${target_os}_${target_arch}}"
dist_dir="${RUNTIME_DIST_DIR:-$repo_root/dist}"
package_dir="$dist_dir/$package_name"
package_path="$dist_dir/$package_name.tar.gz"
dockerfile="${RUNTIME_DOCKERFILE:-$repo_root/docker/session-runtime/Dockerfile}"
dockerignore="${RUNTIME_DOCKERIGNORE:-${dockerfile}.dockerignore}"

die() {
  printf '[legion-ai-session-runtime][error] %s\n' "$*" >&2
  exit 1
}

log() {
  printf '[legion-ai-session-runtime] %s\n' "$*"
}

for command in file git gzip jq sha256sum stat tar; do
  command -v "$command" >/dev/null 2>&1 || die "missing required command: $command"
done

[[ "$target_os" == "linux" ]] || die "unsupported Runtime target OS: $target_os"
case "$target_arch" in
  amd64) expected_machine="x86-64" ;;
  arm64) expected_machine="ARM aarch64" ;;
  *) die "unsupported Runtime target architecture: $target_arch" ;;
esac
[[ "$package_name" == "legion-ai-session-runtime_${target_os}_${target_arch}" ]] || \
  die "RUNTIME_PACKAGE_NAME does not match target platform: $package_name"

source_sha="${SOURCE_SHA:-$(git -C "$repo_root" rev-parse HEAD)}"
[[ "$source_sha" =~ ^[0-9a-f]{40}$ ]] || die "invalid SOURCE_SHA: $source_sha"
[[ "$(git -C "$repo_root" rev-parse HEAD)" == "$source_sha" ]] || \
  die "checkout HEAD does not match SOURCE_SHA"

source_dirty=false
if [[ -n "$(git -C "$repo_root" status --porcelain --untracked-files=all)" ]]; then
  source_dirty=true
fi
if [[ "${CI:-}" == "true" && "$source_dirty" == "true" ]]; then
  die "CI checkout must be clean before provenance is generated"
fi

runtime_binary="${RUNTIME_BINARY:-}"
runtime_ldd_file="${RUNTIME_LDD_FILE:-}"
runtime_image_ref="${RUNTIME_IMAGE_REF:-}"
runtime_image_tag="${RUNTIME_IMAGE_TAG:-}"
runtime_base_image="${RUNTIME_BASE_IMAGE:-}"
runtime_builder_image="${RUNTIME_BUILDER_IMAGE:-}"
runtime_go_version="${RUNTIME_GO_VERSION:-}"

[[ -f "$runtime_binary" ]] || die "RUNTIME_BINARY must reference the binary extracted from the built image"
[[ -f "$runtime_ldd_file" ]] || die "RUNTIME_LDD_FILE must reference ldd output captured inside the built image"
[[ -f "$dockerfile" ]] || die "runtime Dockerfile does not exist: $dockerfile"
[[ -f "$dockerignore" ]] || die "runtime Dockerfile ignore file does not exist: $dockerignore"
[[ "$runtime_image_ref" =~ ^[^[:space:]@]+@sha256:[0-9a-f]{64}$ ]] || \
  die "RUNTIME_IMAGE_REF must be an immutable image@sha256 digest"
[[ "$runtime_image_tag" =~ ^[^[:space:]@]+:[^[:space:]@]+$ ]] || \
  die "RUNTIME_IMAGE_TAG must be the human-readable build tag"
[[ "$runtime_base_image" =~ ^[^[:space:]@]+@sha256:[0-9a-f]{64}$ ]] || \
  die "RUNTIME_BASE_IMAGE must be pinned by sha256 digest"
[[ "$runtime_builder_image" =~ ^[^[:space:]@]+@sha256:[0-9a-f]{64}$ ]] || \
  die "RUNTIME_BUILDER_IMAGE must be pinned by sha256 digest"
[[ -n "$runtime_go_version" ]] || die "RUNTIME_GO_VERSION is required"
grep -qv '^[[:space:]]*$' "$runtime_ldd_file" || die "runtime ldd output is empty"
if grep -q 'not found' "$runtime_ldd_file"; then
  die "runtime binary has unresolved shared libraries in the final image"
fi

file_description="$(file "$runtime_binary")"
grep -q 'ELF 64-bit' <<<"$file_description" || die "runtime binary is not a 64-bit ELF file"
grep -q "$expected_machine" <<<"$file_description" || \
  die "runtime binary does not match ${target_os}/${target_arch}"

module_go_version="$(awk '$1 == "go" { gsub(/\r/, "", $2); print $2; exit }' "$repo_root/go.mod")"
[[ -n "$module_go_version" ]] || die "go.mod does not declare a Go version"
[[ "$runtime_go_version" == *"go${module_go_version}"* ]] || \
  die "Runtime Go version does not match go.mod go${module_go_version}: $runtime_go_version"

version="${RUNTIME_PACKAGE_VERSION:-sha-${source_sha:0:12}}"
[[ -n "$version" ]] || die "runtime package version must not be empty"
[[ ! -e "$package_dir" ]] || die "package directory already exists: $package_dir"
[[ ! -e "$package_path" ]] || die "package archive already exists: $package_path"

mkdir -p "$package_dir"
binary_sha="$(sha256sum "$runtime_binary" | awk '{print $1}')"
binary_size="$(stat -c %s "$runtime_binary")"
dockerfile_sha="$(sha256sum "$dockerfile" | awk '{print $1}')"
dockerignore_sha="$(sha256sum "$dockerignore" | awk '{print $1}')"
built_at="$(date -u +%Y-%m-%dT%H:%M:%SZ)"

jq -n \
  --arg version "$version" \
  --arg repository "${GITHUB_REPOSITORY:-yaklang/yaklang}" \
  --arg source_sha "$source_sha" \
  --argjson source_dirty "$source_dirty" \
  --arg ci_provider "${RUNTIME_CI_PROVIDER:-${GITHUB_ACTIONS:+github-actions}}" \
  --arg ci_run_id "${GITHUB_RUN_ID:-local}" \
  --arg ci_run_attempt "${GITHUB_RUN_ATTEMPT:-1}" \
  --arg ci_workflow "${SESSION_RUNTIME_WORKFLOW_NAME:-${GITHUB_WORKFLOW:-local}}" \
  --arg ci_actor "${GITHUB_ACTOR:-local}" \
  --arg runtime_base_image "$runtime_base_image" \
  --arg runtime_builder_image "$runtime_builder_image" \
  --arg dockerfile_sha "$dockerfile_sha" \
  --arg dockerignore_sha "$dockerignore_sha" \
  --arg module_go_version "$module_go_version" \
  --arg runtime_go_version "$runtime_go_version" \
  --arg target_os "$target_os" \
  --arg target_arch "$target_arch" \
  --arg binary_sha "$binary_sha" \
  --argjson binary_size "$binary_size" \
  --rawfile binary_ldd "$runtime_ldd_file" \
  --arg runtime_image_ref "$runtime_image_ref" \
  --arg runtime_image_tag "$runtime_image_tag" \
  --arg built_at "$built_at" \
  '{
    schema_version: "1",
    artifact_type: "legion-ai-session-runtime",
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
      version: "session-runtime-v1",
      base_image: $runtime_base_image,
      builder_image: $runtime_builder_image,
      packaging_repository: $repository,
      packaging_source_sha: $source_sha,
      packaging_dirty: $source_dirty,
      dockerfile_sha256: $dockerfile_sha,
      dockerignore_sha256: $dockerignore_sha,
      goos: $target_os,
      goarch: $target_arch,
      cgo_enabled: true,
      link_mode: "dynamic-container",
      build_tags: "",
      module_go_version: $module_go_version,
      go_version: $runtime_go_version
    },
    capabilities: ["ai.session.bind_epoch.v1", "ai.session.runtime", "ai.session.turn_lifecycle.v1", "yak.execute"],
    binary: {
      path: "/usr/local/bin/legion-session-runtime",
      sha256: $binary_sha,
      size: $binary_size,
      ldd: ($binary_ldd | rtrimstr("\n"))
    },
    image: {
      ref: $runtime_image_ref,
      tag: $runtime_image_tag,
      revision_label: $source_sha
    },
    built_at: $built_at
  }' >"$package_dir/SESSION_RUNTIME_MANIFEST.json"

(
  cd "$package_dir"
  sha256sum SESSION_RUNTIME_MANIFEST.json >SHA256SUMS
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

log "provenance package created: $package_path"
log "runtime image: $runtime_image_ref"
log "runtime binary sha256: $binary_sha"
