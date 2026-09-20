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
# setup-go has already installed the native Linux SDK on both CI runners.
# A non-Linux caller can explicitly provide a matching Linux SDK directory.
GOROOT_HOST="${MANYLINUX_GOROOT:-$(go env GOROOT)}"
GIT_HASH="$(git -C "$WORKSPACE" show -s --format=%H)"
BUILD_TIME="$(git -C "$WORKSPACE" show -s --format=%cd)"

if [[ ! -x "$GOROOT_HOST/bin/go" ]]; then
  echo "Go SDK missing: $GOROOT_HOST/bin/go" >&2
  exit 1
fi

echo "manylinux image: ${MANYLINUX_IMAGE}"
echo "output: ${OUTPUT_BINARY}, tags: ${BUILD_TAGS}, SDK: ${GOROOT_HOST}"

docker run --rm -i \
  -v "${WORKSPACE}:/work" \
  -v "${GOMODCACHE_HOST}:/gomodcache:ro" \
  -v "${GOROOT_HOST}:/usr/local/go:ro" \
  -w /work \
  -e GOROOT=/usr/local/go \
  -e GOTOOLCHAIN=local \
  -e GOMODCACHE=/gomodcache \
  -e GOPATH=/tmp/gopath \
  -e CGO_ENABLED=1 \
  -e BUILD_TAGS="${BUILD_TAGS}" \
  -e OUTPUT_BINARY="${OUTPUT_BINARY}" \
  -e YAK_TAG="${YAK_TAG}" \
  -e GIT_HASH="${GIT_HASH}" \
  -e BUILD_TIME="${BUILD_TIME}" \
  "${MANYLINUX_IMAGE}" \
  bash -leo pipefail -s <<'EOS'
source /opt/rh/devtoolset-10/enable
case "$(uname -m)" in
  x86_64) EXPECTED_GOARCH=amd64 ;;
  aarch64) EXPECTED_GOARCH=arm64 ;;
  *) echo "unsupported container arch: $(uname -m)" >&2; exit 1 ;;
esac
export PATH=/usr/local/go/bin:$PATH
if [[ "$(go env GOHOSTOS)/$(go env GOHOSTARCH)" != "linux/${EXPECTED_GOARCH}" ]]; then
  echo "The mounted Go SDK must target the container host linux/${EXPECTED_GOARCH}" >&2
  exit 1
fi
export CGO_ENABLED=1
export GOMODCACHE=/gomodcache
export GOPATH=/tmp/gopath
GO_VER_STR="$(go version)"
args=(go build -trimpath)
if [ -n "${BUILD_TAGS}" ]; then
  args+=(-tags "${BUILD_TAGS}")
fi
# go build keeps only the last occurrence of -ldflags. Pass every linker option
# together, quoting values with spaces inside the linker argument string.
args+=(-ldflags "-s -w -X 'main.goVersion=${GO_VER_STR}' -X 'main.gitHash=${GIT_HASH}' -X 'main.buildTime=${BUILD_TIME}' -X 'main.yakVersion=${YAK_TAG}'")
args+=(-o "./${OUTPUT_BINARY}" -v common/yak/cmd/yak.go)
echo "Executing: ${args[*]}"
"${args[@]}"
sections="$(readelf --sections --wide "./${OUTPUT_BINARY}")"
if grep -Eq '[[:space:]]\.(symtab|debug_info)[[:space:]]' <<< "$sections"; then
  echo "Unstripped binary: linker -s -w flags did not take effect" >&2
  exit 1
fi
file "./${OUTPUT_BINARY}"
YAKIT_HOME="$(mktemp -d)" "./${OUTPUT_BINARY}" --help >/tmp/yak-build-help.txt
ls -lh "./${OUTPUT_BINARY}"
EOS
