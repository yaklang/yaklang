#!/usr/bin/env bash
# Analyze source files on the build host, even during a cross compilation.
set -euo pipefail
cd "$(dirname "$0")/.."
metadata_host_os="$(go env GOHOSTOS)"
metadata_host_arch="$(go env GOHOSTARCH)"
if [[ "$(go env GOOS)/$(go env GOARCH)" != "$metadata_host_os/$metadata_host_arch" ]]; then
  # A target compiler and its flags cannot build an executable for the host.
  unset CGO_ENABLED CC CXX CGO_CFLAGS CGO_CPPFLAGS CGO_CXXFLAGS CGO_LDFLAGS
fi
export GOOS="$metadata_host_os" GOARCH="$metadata_host_arch"
# gzip_embed inputs may not have been packed yet. Keep native compiler settings
# while explicitly excluding target build tags from the generator itself.
GOFLAGS= go run -tags='' ./common/yak/cmd/generate-builtin-metadata "$@"
