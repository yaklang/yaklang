#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
helper="$repo_root/.github/scripts/build-legion-linux-musl.sh"
test_root="$(mktemp -d)"
trap 'rm -rf "$test_root"' EXIT
mkdir -p "$test_root/bin"
cat >"$test_root/bin/docker" <<'SH'
#!/bin/sh
printf '%s\n' "$@" >"$NODE_HELPER_DOCKER_ARGS"
SH
chmod 755 "$test_root/bin/docker"
export NODE_HELPER_DOCKER_ARGS="$test_root/docker-args"
export NODE_MUSL_CACHE_DIR="$test_root/cache"
export PATH="$test_root/bin:$PATH"

case "$(uname -m)" in
  x86_64) arch=amd64; child=6d405dfc5fdf3a45df1529cf060b920041f52ce523487e0f36f02765af294a51 ;;
  aarch64) arch=arm64; child=7030196c59858ca8942e85aa2f68599240d58282ee35eb795609738a1e242c1f ;;
  *) echo 'helper contract test requires a supported native Linux host' >&2; exit 1 ;;
esac

expect_rejected() {
  local error="$1"; shift
  rm -f "$NODE_HELPER_DOCKER_ARGS"
  if "$@" >"$test_root/result.log" 2>&1; then
    echo "musl helper accepted invalid input: $*" >&2
    exit 1
  fi
  grep -Fq "$error" "$test_root/result.log" || { cat "$test_root/result.log" >&2; exit 1; }
  [[ ! -e "$NODE_HELPER_DOCKER_ARGS" ]] || { echo 'invalid build input reached Docker' >&2; exit 1; }
}

expect_rejected 'requires a value' "$helper" --output
expect_rejected 'unsupported Node target architecture' "$helper" --goarch riscv64 --output "$test_root/node"
expect_rejected 'unsupported Node build profile' "$helper" --profile unknown --output "$test_root/node"
expect_rejected 'must retain the pinned' env NODE_MUSL_BUILDER_IMAGE=golang:1.22.12-bookworm \
  "$helper" --goarch "$arch" --output "$test_root/node"
expect_rejected 'emulated musl builds are not allowed in CI' env CI=true NODE_MUSL_ALLOW_EMULATION=true \
  "$helper" --goarch "$arch" --output "$test_root/node"
expect_rejected 'checkout HEAD does not match' env CI=false SOURCE_SHA=0000000000000000000000000000000000000000 \
  "$helper" --goarch "$arch" --output "$test_root/node"

# Observe routing without building Node: missing outputs must be rejected, while
# the Docker arguments preserve the worktree's real common Git path and choose
# an immutable platform child rather than a shared multiarch-store reference.
if CI=false "$helper" --goarch "$arch" --output "$test_root/node" >"$test_root/result.log" 2>&1; then
  echo 'musl helper accepted a successful builder that produced no artifact' >&2
  exit 1
fi
grep -Fq 'builder did not produce' "$test_root/result.log"
git_common_dir="$(git -C "$repo_root" rev-parse --path-format=absolute --git-common-dir)"
grep -Fxq "$repo_root:$repo_root:ro" "$NODE_HELPER_DOCKER_ARGS"
grep -Fxq "$git_common_dir:$git_common_dir:ro" "$NODE_HELPER_DOCKER_ARGS"
grep -Fxq "docker.io/library/golang:1.22.12-alpine3.21@sha256:$child" "$NODE_HELPER_DOCKER_ARGS"
[[ "$(grep -Fxc "$test_root:$test_root" "$NODE_HELPER_DOCKER_ARGS")" -eq 1 ]] || {
  echo 'output and metadata directories must not create duplicate Docker mounts' >&2
  exit 1
}

# A manifest field cannot turn a dynamic glibc ELF into a static musl artifact.
expect_rejected 'not fully statically linked' "$helper" --verify-binary /bin/true "$arch" runtime
printf '[legion-node-musl-test] PASS\n'
