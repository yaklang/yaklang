#!/bin/sh
# Shared native Linux Node recipe. Both Actions packaging and the Session
# Runtime Dockerfile use this file so every CGO translation unit uses musl.
set -eu

builder_digest=1699c10032ca2582ec89a24a1312d986a3f094aed3d5c1147b19880afe40e052
default_builder="docker.io/library/golang:1.22.12-alpine3.21@sha256:$builder_digest"
musl_version=1.2.5
build_packages='musl=1.2.5-r12 musl-dev=1.2.5-r12 gcc=14.2.0-r4 g++=14.2.0-r4 libgcc=14.2.0-r4 libstdc++=14.2.0-r4 binutils=2.43.1-r3 build-base=0.5-r3 make=4.4.1-r2 file=5.46-r2 git=2.47.3-r0 linux-headers=6.6-r1 pkgconf=2.3.0-r0 jq=1.7.1-r0'

die() { printf '[legion-node-musl][error] %s\n' "$*" >&2; exit 1; }
log() { printf '[legion-node-musl] %s\n' "$*"; }

select_builder_manifest() {
  case "$target_arch" in
    amd64) platform_digest=6d405dfc5fdf3a45df1529cf060b920041f52ce523487e0f36f02765af294a51 ;;
    arm64) platform_digest=7030196c59858ca8942e85aa2f68599240d58282ee35eb795609738a1e242c1f ;;
    *) die "unsupported Node target architecture: $target_arch" ;;
  esac
  case "$builder_image" in
    *@sha256:"$platform_digest"|*@sha256:"$builder_digest") ;;
    *) die 'NODE_MUSL_BUILDER_IMAGE must retain the pinned Go 1.22.12 Alpine 3.21 platform digest' ;;
  esac
  # Select the child manifest before pulling: classic Docker stores cannot keep
  # two different architecture images under the same multiarch index reference.
  builder_index="${builder_image%@*}@sha256:$builder_digest"
  builder_image="${builder_image%@*}@sha256:$platform_digest"
}

install_toolchain() {
  [ -f /etc/alpine-release ] || die 'musl builder must run on Alpine 3.21'
  case "$(cat /etc/alpine-release)" in 3.21.*) ;; *) die 'musl builder must run on Alpine 3.21' ;; esac
  [ "$(go env GOVERSION)" = go1.22.12 ] || die 'musl builder requires Go 1.22.12'
  # Pin the C toolchain as well as the base image. Record the complete resolved
  # package set in the build metadata; dependency updates require a recipe edit.
  # shellcheck disable=SC2086 # Intentionally split the fixed, repository-owned package list.
  apk add --no-cache $build_packages
}

tags_for_profile() {
  case "$1" in
    product) printf 'hids,musl,netgo,osusergo,sqlite_omit_load_extension,dameng' ;;
    portable|runtime) printf 'musl,netgo,osusergo,sqlite_omit_load_extension,dameng' ;;
    *) die "unsupported Node build profile: $1" ;;
  esac
}

verify_binary() {
  verify_path="$1"; verify_arch="$2"; verify_tags="$3"
  case "$verify_arch" in
    amd64) expected_machine='x86-64' ;;
    arm64) expected_machine='ARM aarch64' ;;
    *) die "unsupported Node target architecture: $verify_arch" ;;
  esac
  description="$(file "$verify_path")"
  printf '%s\n' "$description"
  printf '%s\n' "$description" | grep -q 'ELF 64-bit' || die 'node binary is not a 64-bit ELF'
  printf '%s\n' "$description" | grep -q "$expected_machine" || die 'node binary architecture does not match its target'
  printf '%s\n' "$description" | grep -q 'statically linked' || die 'node binary is not fully statically linked'
  program_headers="$(readelf -lW "$verify_path")" || die 'cannot read node ELF program headers'
  if printf '%s\n' "$program_headers" | grep -q '[[:space:]]INTERP[[:space:]]'; then
    die 'node binary has a dynamic ELF interpreter'
  fi
  dynamic_section="$(readelf -dW "$verify_path")" || die 'cannot read node ELF dynamic section'
  if printf '%s\n' "$dynamic_section" | grep -q '(NEEDED)'; then
    die 'node binary declares dynamic library dependencies'
  fi
  symbol_versions="$(readelf --version-info "$verify_path")" || die 'cannot read node ELF symbol versions'
  if printf '%s\n' "$symbol_versions" | grep -q 'GLIBC_'; then
    die 'node binary contains glibc symbol-version dependencies'
  fi
  # Static glibc can hide these references from the ELF version table. Inspect
  # the artifact bytes too, including GLIBC_PRIVATE used by the #560 failure.
  if strings "$verify_path" | grep -E 'GLIBC_(PRIVATE|[0-9])' >/dev/null; then
    die 'node binary contains glibc version references'
  fi
  build_info="$(go version -m "$verify_path")" || die 'cannot read node Go build information'
  printf '%s\n' "$build_info" | grep -Fq -- "-tags=$verify_tags" || die 'node binary does not contain the required musl build tags'
  printf '%s\n' "$build_info" | grep -Fq 'CGO_ENABLED=1' || die 'node binary must preserve CGO'
  printf '%s\n' "$build_info" | grep -Fq "GOARCH=$verify_arch" || die 'node Go build architecture does not match its target'
}

container_build() {
  source_dir="${NODE_SOURCE_DIR:?NODE_SOURCE_DIR is required}"
  output_binary="${NODE_OUTPUT_BINARY:?NODE_OUTPUT_BINARY is required}"
  metadata_file="${NODE_MUSL_METADATA_FILE:?NODE_MUSL_METADATA_FILE is required}"
  target_arch="${NODE_GOARCH:?NODE_GOARCH is required}"
  profile="${NODE_BUILD_PROFILE:-product}"
  buildvcs="${NODE_BUILDVCS:-true}"
  builder_image="${NODE_MUSL_BUILDER_IMAGE:?NODE_MUSL_BUILDER_IMAGE is required}"
  select_builder_manifest
  build_tags="$(tags_for_profile "$profile")"
  case "$(uname -m)/$target_arch" in
    x86_64/amd64) musl_interpreter=ld-musl-x86_64.so.1 ;;
    aarch64/arm64) musl_interpreter=ld-musl-aarch64.so.1 ;;
    *) die "musl builds require a native Linux $target_arch builder" ;;
  esac
  [ "$(go env GOHOSTOS)/$(go env GOHOSTARCH)" = "linux/$target_arch" ] || die 'Go SDK does not match the native builder'
  module_go_version="$(awk '$1 == "go" { gsub(/\r/, "", $2); print $2; exit }' "$source_dir/go.mod")"
  [ "$module_go_version" = 1.22.12 ] || die 'Node musl recipe requires go.mod go 1.22.12'
  [ "$(go env GOVERSION)" = "go$module_go_version" ] || die 'Go SDK does not match go.mod'
  case "$buildvcs" in true|false) ;; *) die 'NODE_BUILDVCS must be true or false' ;; esac

  # Check the actual default C linker, not just a user-supplied build tag. The
  # static probe strongly references a musl-only symbol and executes its version.
  probe_dir="$(mktemp -d /tmp/legion-musl-probe.XXXXXX)"
  binary_stage_dir=
  metadata_stage_dir=
  # shellcheck disable=SC2317 # Called indirectly by the EXIT trap below.
  cleanup_build() {
    rm -rf "$probe_dir"
    if [ -n "$binary_stage_dir" ]; then rm -rf "$binary_stage_dir"; fi
    if [ -n "$metadata_stage_dir" ] && [ "$metadata_stage_dir" != "$binary_stage_dir" ]; then
      rm -rf "$metadata_stage_dir"
    fi
  }
  trap 'cleanup_build' EXIT
  trap 'exit 129' HUP
  trap 'exit 130' INT
  trap 'exit 143' TERM
  cat >"$probe_dir/probe.c" <<'C'
#include <features.h>
#include <stdio.h>
#if defined(__GLIBC__) || defined(__UCLIBC__) || defined(__BIONIC__)
#error the Legion Node C compiler must target musl
#endif
extern const char __libc_version[];
int main(void) { puts(__libc_version); return 0; }
C
  # __libc_version is available in musl's static archive but is hidden from
  # its shared-library ABI. The dynamic probe only verifies the interpreter.
  printf '#include <stdio.h>\nint main(void) { puts("musl-linker-probe"); return 0; }\n' >"$probe_dir/dynamic.c"
  gcc "$probe_dir/dynamic.c" -o "$probe_dir/dynamic-probe"
  readelf -lW "$probe_dir/dynamic-probe" >"$probe_dir/interpreter.txt"
  grep -Fq "/lib/$musl_interpreter" "$probe_dir/interpreter.txt" || die 'C compiler default libc is not native musl'
  gcc -static "$probe_dir/probe.c" -o "$probe_dir/static-probe"
  actual_musl_version="$("$probe_dir/static-probe")"
  [ "$actual_musl_version" = "$musl_version" ] || die 'C compiler musl version does not match the pinned recipe'
  compiler_version="$(gcc -dumpfullversion)"
  [ "$compiler_version" = 14.2.0 ] || die 'C compiler version does not match the pinned recipe'

  unset GOFLAGS GOEXPERIMENT CGO_CFLAGS CGO_CPPFLAGS CGO_CXXFLAGS CGO_FFLAGS CGO_LDFLAGS
  export GOENV=off GOTOOLCHAIN=local CGO_ENABLED=1 GOOS=linux GOARCH="$target_arch" CC=gcc CXX=g++
  build_source_dir="$source_dir"
  if [ "$buildvcs" = true ]; then
    git config --global --add safe.directory "$source_dir"
    source_revision="$(git -C "$source_dir" rev-parse HEAD)"
    source_modified=false
    if [ -n "$(git -C "$source_dir" status --porcelain --untracked-files=all)" ]; then
      source_modified=true
    fi
    if [ -f "$source_dir/.git" ]; then
      # Go 1.22's VCS discovery only recognizes .git directories. A linked
      # worktree's .git file silently omits stamping even with -buildvcs=true.
      # Copy its actual contents (including local changes) and expose the real
      # Git directory through a directory symlink; the original stays read-only.
      build_source_dir="$probe_dir/source"
      cp -a "$source_dir" "$build_source_dir"
      git_dir="$(git -C "$source_dir" rev-parse --absolute-git-dir)"
      rm "$build_source_dir/.git"
      ln -s "$git_dir" "$build_source_dir/.git"
      git config --global --add safe.directory "$build_source_dir"
    fi
  fi
  mkdir -p "$(dirname "$output_binary")" "$(dirname "$metadata_file")"
  [ "$output_binary" != "$metadata_file" ] || die 'Node binary and metadata must use different paths'
  [ ! -d "$output_binary" ] || die 'Node output path must not be a directory'
  [ ! -d "$metadata_file" ] || die 'Node metadata path must not be a directory'
  # Existing /tmp outputs belong to NODE_OUTPUT_UID after the first build.
  # protected_regular prevents container root from reopening them with O_CREAT.
  # Stage in private directories on each destination filesystem, then rename;
  # /tmp inside the container may be a different mount from either destination.
  binary_stage_dir="$(mktemp -d "$(dirname "$output_binary")/.legion-musl-build.XXXXXX")"
  metadata_stage_dir="$binary_stage_dir"
  if [ "$(dirname "$metadata_file")" != "$(dirname "$output_binary")" ]; then
    metadata_stage_dir="$(mktemp -d "$(dirname "$metadata_file")/.legion-musl-build.XXXXXX")"
  fi
  staged_binary="$binary_stage_dir/node"
  staged_metadata="$metadata_stage_dir/musl-build.json"
  log "building Linux $target_arch $profile node with musl $actual_musl_version, GCC $compiler_version"
  # Go 1.22 selects gold for ARM64 programs importing plugin, including the DM
  # driver's optional cipher loader. Alpine ships the pinned modern BFD linker;
  # select it explicitly for this static build instead of requiring ld.gold.
  (
    cd "$build_source_dir"
    go build -mod=readonly -trimpath -buildvcs="$buildvcs" -tags "$build_tags" \
      -ldflags '-s -w -buildid= -linkmode external -extld gcc -extldflags "-static -fuse-ld=bfd"' \
      -o "$staged_binary" ./cmd/legion-smoke-node
  )
  verify_binary "$staged_binary" "$target_arch" "$build_tags"
  if [ "$buildvcs" = true ]; then
    printf '%s\n' "$build_info" | grep -Fq 'vcs=git' || die 'node binary is missing its Git source identity'
    printf '%s\n' "$build_info" | grep -Fq "vcs.revision=$source_revision" || die 'node VCS revision does not match the source checkout'
    printf '%s\n' "$build_info" | grep -Fq "vcs.modified=$source_modified" || die 'node VCS modified state does not match the source checkout'
  fi

  # This bounded, no-credential help invocation validates the linked libc
  # marker. The NSS/glibc regression workflow separately exercises real startup.
  mkdir -p "$probe_dir/home" "$probe_dir/yakit"
  startup_status=0
  HOME="$probe_dir/home" YAKIT_HOME="$probe_dir/yakit" timeout 30 "$staged_binary" -h >"$probe_dir/startup.log" 2>&1 || startup_status=$?
  # The existing Node CLI returns flag.ErrHelp through log.Fatal, hence exit 1.
  # An arbitrary exit 1, a crash or timeout must never count as successful help.
  if [ "$startup_status" -ne 1 ] ||
    ! grep -Fq 'component=legion-node event=startup stage=main status=entered' "$probe_dir/startup.log" ||
    ! tail -n 1 "$probe_dir/startup.log" | grep -q 'flag: help requested$'; then
    cat "$probe_dir/startup.log" >&2
    die 'musl node help startup failed'
  fi
  grep -Fq "component=legion-node event=libc c_library=musl version=$actual_musl_version" "$probe_dir/startup.log" || die 'node does not report its linked musl libc version'

  apk info -v | LC_ALL=C sort >"$probe_dir/packages.txt"
  jq -n --arg builder_image "$builder_image" --arg builder_index "$builder_index" \
    --arg builder_platform_manifest "sha256:$platform_digest" --arg go_version "$(go version)" \
    --arg build_tags "$build_tags" --arg c_compiler "gcc $compiler_version" \
    --arg c_library_version "$actual_musl_version" --arg alpine_version "$(cat /etc/alpine-release)" \
    --arg pinned_packages "$build_packages" --arg goarch "$target_arch" \
    --arg builder_execution "${NODE_MUSL_BUILDER_EXECUTION:-native}" --argjson buildvcs "$buildvcs" \
    --rawfile installed_packages "$probe_dir/packages.txt" \
    '{builder_image: $builder_image, builder_index: $builder_index, builder_platform_manifest: $builder_platform_manifest,
      go_version: $go_version, build_tags: $build_tags, goos: "linux", goarch: $goarch,
      c_compiler: $c_compiler, c_library: "musl", c_library_version: $c_library_version,
      alpine_version: $alpine_version, link_mode: "external-static", cgo_enabled: true,
      builder_execution: $builder_execution, buildvcs: $buildvcs,
      pinned_build_packages: ($pinned_packages | split(" ")),
      installed_packages: ($installed_packages | split("\n") | map(select(length > 0)))}' >"$staged_metadata"
  chmod 755 "$staged_binary"
  if [ -n "${NODE_OUTPUT_UID:-}" ] && [ -n "${NODE_OUTPUT_GID:-}" ]; then
    chown "$NODE_OUTPUT_UID:$NODE_OUTPUT_GID" "$staged_binary" "$staged_metadata"
  fi
  # Both outputs have passed validation. Each rename replaces the destination
  # atomically; failures before publication preserve the previous pair.
  mv -f "$staged_binary" "$output_binary"
  mv -f "$staged_metadata" "$metadata_file"
}

case "${1:-}" in
  --install-toolchain) install_toolchain; exit 0 ;;
  --in-container) container_build; exit 0 ;;
  --verify-binary)
    [ "$#" -eq 4 ] || die 'usage: --verify-binary BINARY ARCH PROFILE'
    verify_binary "$2" "$3" "$(tags_for_profile "$4")"; exit 0 ;;
esac

repo_root="$(cd "$(dirname "$0")/../.." && pwd)"
target_arch="${NODE_GOARCH:-amd64}"
profile="${NODE_BUILD_PROFILE:-product}"
output_binary="${NODE_OUTPUT_BINARY:-}"
metadata_file="${NODE_MUSL_METADATA_FILE:-}"
builder_image="${NODE_MUSL_BUILDER_IMAGE:-$default_builder}"
while [ "$#" -gt 0 ]; do
  case "$1" in
    --goarch|--profile|--output|--metadata)
      [ "$#" -ge 2 ] || die "$1 requires a value"
      case "$1" in
        --goarch) target_arch="$2" ;;
        --profile) profile="$2" ;;
        --output) output_binary="$2" ;;
        --metadata) metadata_file="$2" ;;
      esac
      shift 2 ;;
    *) die "unknown argument: $1" ;;
  esac
done
tags_for_profile "$profile" >/dev/null
case "$target_arch" in amd64|arm64) ;; *) die "unsupported Node target architecture: $target_arch" ;; esac
select_builder_manifest
[ -n "$output_binary" ] || die '--output or NODE_OUTPUT_BINARY is required'
[ -n "$metadata_file" ] || metadata_file="$output_binary.musl-build.json"
for command in docker git realpath; do
  command -v "$command" >/dev/null 2>&1 || die "missing required command: $command"
done
builder_execution=native
case "${NODE_MUSL_ALLOW_EMULATION:-false}" in
  false) ;;
  true) [ "${CI:-}" != true ] || die 'emulated musl builds are not allowed in CI' ;;
  *) die 'NODE_MUSL_ALLOW_EMULATION must be true or false' ;;
esac
case "$(uname -s)/$(uname -m)/$target_arch" in
  Linux/x86_64/amd64|Linux/aarch64/arm64) ;;
  Linux/x86_64/arm64|Linux/aarch64/amd64)
    [ "${NODE_MUSL_ALLOW_EMULATION:-false}" = true ] || die "musl builds require a native Linux $target_arch host; local QEMU builds require NODE_MUSL_ALLOW_EMULATION=true"
    builder_execution=qemu-emulated ;;
  *) die "musl builds require a Linux $target_arch host" ;;
esac
source_sha="${SOURCE_SHA:-$(git -C "$repo_root" rev-parse HEAD)}"
[ "${#source_sha}" -eq 40 ] || die 'SOURCE_SHA must be a full Git commit'
case "$source_sha" in *[!0-9a-f]*) die 'SOURCE_SHA must be a hexadecimal Git commit' ;; esac
[ "$(git -C "$repo_root" rev-parse HEAD)" = "$source_sha" ] || die 'checkout HEAD does not match SOURCE_SHA'
if [ "${CI:-}" = true ] && [ -n "$(git -C "$repo_root" status --porcelain --untracked-files=all)" ]; then
  die 'CI checkout must be clean before building a musl Node'
fi
output_binary="$(realpath -m "$output_binary")"
metadata_file="$(realpath -m "$metadata_file")"
mkdir -p "$(dirname "$output_binary")" "$(dirname "$metadata_file")"
cache_root="${NODE_MUSL_CACHE_DIR:-${XDG_CACHE_HOME:-${HOME:?HOME or NODE_MUSL_CACHE_DIR is required}/.cache}/yaklang/node-musl/go1.22.12-alpine3.21-$target_arch}"
cache_root="$(realpath -m "$cache_root")"
mkdir -p "$cache_root/modules" "$cache_root/build"
git_common_dir="$(git -C "$repo_root" rev-parse --path-format=absolute --git-common-dir)"

# Preserve worktree .git pointers and Git source identity by mounting both the
# checkout and its common Git directory at their original absolute host paths.
set -- docker run --rm --platform "linux/$target_arch" \
  -v "$repo_root:$repo_root:ro" -v "$git_common_dir:$git_common_dir:ro" \
  -v "$(dirname "$output_binary"):$(dirname "$output_binary")" \
  -v "$cache_root/modules:/gomodcache" -v "$cache_root/build:/gocache" \
  -w "$repo_root" -e GOTOOLCHAIN=local -e GOMODCACHE=/gomodcache -e GOCACHE=/gocache \
  -e "NODE_SOURCE_DIR=$repo_root" -e "NODE_OUTPUT_BINARY=$output_binary" \
  -e "NODE_MUSL_METADATA_FILE=$metadata_file" -e "NODE_GOARCH=$target_arch" \
  -e "NODE_BUILD_PROFILE=$profile" -e NODE_BUILDVCS=true \
  -e "NODE_MUSL_BUILDER_EXECUTION=$builder_execution" \
  -e "NODE_MUSL_BUILDER_IMAGE=$builder_image" -e "NODE_OUTPUT_UID=$(id -u)" -e "NODE_OUTPUT_GID=$(id -g)"
if [ "$(dirname "$metadata_file")" != "$(dirname "$output_binary")" ]; then
  set -- "$@" -v "$(dirname "$metadata_file"):$(dirname "$metadata_file")"
fi
# shellcheck disable=SC2016 # Expand Node paths inside the builder container.
set -- "$@" "$builder_image" /bin/sh -ec \
  '"$NODE_SOURCE_DIR/.github/scripts/build-legion-linux-musl.sh" --install-toolchain; exec "$NODE_SOURCE_DIR/.github/scripts/build-legion-linux-musl.sh" --in-container'
"$@"
if [ ! -s "$output_binary" ] || [ ! -s "$metadata_file" ]; then
  die 'builder did not produce a Node binary and musl recipe'
fi
log "verified musl Node: $output_binary"
log "build recipe: $metadata_file"
