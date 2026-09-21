#!/usr/bin/env bash
# Build separately so cold toolchain compilation is not counted as mock runtime.
set -euo pipefail
cd "$(dirname "$0")/../.."
mode=${1:-test}
bin_dir=${DOCKERHTTP_TEST_BIN_DIR:-${TMPDIR:-/tmp}/yak-dockerhttp-tests}
export GOWORK=off GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off
if [[ "$mode" == build || "$mode" == test ]]; then
  mkdir -p "$bin_dir"
  go test -mod=readonly -c -o "$bin_dir/client.test" ./common/dockerhttp
  go test -mod=readonly -c -o "$bin_dir/runtime.test" scannode/runtime_host_docker.go scannode/runtime_host_docker_test.go
  go test -mod=readonly -c -o "$bin_dir/services.test" common/thirdpartyservices/docker_images.go common/thirdpartyservices/docker_test.go
fi
if [[ "$mode" == run || "$mode" == test ]]; then
  start=$SECONDS
  for suite in client runtime services; do
    "$bin_dir/$suite.test" -test.count=1 -test.timeout=10s
  done
  elapsed=$((SECONDS-start))
  echo "Docker offline mocks and core contracts: ${elapsed}s (budget: 10s; build excluded)"
  (( elapsed < 10 ))
elif [[ "$mode" != build ]]; then
  echo "Usage: $0 [build|run|test]" >&2
  exit 2
fi
