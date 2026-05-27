#!/usr/bin/env bash
# Build yak inside a manylinux2014 container so the binary only needs glibc 2.17+.
# CI steps (checkout, setup-go, actions) must run on the host runner, not in the container.
set -euo pipefail

MANYLINUX_IMAGE="${1:?manylinux image required}"
OUTPUT_BINARY="${2:?output binary required}"
BUILD_TAGS="${3:-gzip_embed}"
YAK_TAG="${4:?yak tag required}"

WORKSPACE="${GITHUB_WORKSPACE:-$(pwd)}"
GOMODCACHE_HOST="${GOMODCACHE:-$(go env GOMODCACHE)}"
GIT_HASH="$(git -C "$WORKSPACE" show -s --format=%H)"
BUILD_TIME="$(git -C "$WORKSPACE" show -s --format=%cd)"
GO_VERSION="$(grep '^go ' "$WORKSPACE/go.mod" | awk '{print $2}')"

echo "manylinux image: ${MANYLINUX_IMAGE}"
echo "output: ${OUTPUT_BINARY}, tags: ${BUILD_TAGS}, go: ${GO_VERSION}"

docker run --rm -i \
  -v "${WORKSPACE}:/work" \
  -v "${GOMODCACHE_HOST}:/gomodcache:ro" \
  -w /work \
  -e GOMODCACHE=/gomodcache \
  -e GOPATH=/tmp/gopath \
  -e CGO_ENABLED=1 \
  -e BUILD_TAGS="${BUILD_TAGS}" \
  -e OUTPUT_BINARY="${OUTPUT_BINARY}" \
  -e YAK_TAG="${YAK_TAG}" \
  -e GIT_HASH="${GIT_HASH}" \
  -e BUILD_TIME="${BUILD_TIME}" \
  -e GO_VERSION="${GO_VERSION}" \
  "${MANYLINUX_IMAGE}" \
  bash -leo pipefail -s <<'EOS'
source /opt/rh/devtoolset-10/enable
case "$(uname -m)" in
  x86_64) GOARCH_DL=amd64 ;;
  aarch64) GOARCH_DL=arm64 ;;
  *) echo "unsupported container arch: $(uname -m)" >&2; exit 1 ;;
esac
curl -fsSL "https://go.dev/dl/go${GO_VERSION}.linux-${GOARCH_DL}.tar.gz" | tar -C /usr/local -xz
export PATH=/usr/local/go/bin:$PATH
export CGO_ENABLED=1
export GOMODCACHE=/gomodcache
export GOPATH=/tmp/gopath
GO_VER_STR="$(go version)"
args=(go build)
if [ -n "${BUILD_TAGS}" ]; then
  args+=(-tags "${BUILD_TAGS}")
fi
args+=(-ldflags "-s -w")
args+=(-ldflags "-X main.goVersion=${GO_VER_STR}")
args+=(-ldflags "-X main.gitHash=${GIT_HASH}")
args+=(-ldflags "-X main.buildTime=${BUILD_TIME}")
args+=(-ldflags "-X main.yakVersion=${YAK_TAG}")
args+=(-o "./${OUTPUT_BINARY}" -v common/yak/cmd/yak.go)
echo "Executing: ${args[*]}"
"${args[@]}"
ls -lh "./${OUTPUT_BINARY}"
EOS
