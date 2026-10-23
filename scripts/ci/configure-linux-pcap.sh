#!/usr/bin/env bash
# The DM SDK imports Go's plugin package on Linux. Go then forbids copy
# relocations, which the legacy bundled glibc libpcap archive requires.
# Prefer the runner's shared PIC library for glibc CI, retaining race/plugin
# checks without configuring the separate static musl build.
set -euo pipefail

: "${GITHUB_ENV:?GITHUB_ENV is required}"
pcap_libdir=$(pkg-config --variable=libdir libpcap)
if [[ "$pcap_libdir" != /* || ! -f "$pcap_libdir/libpcap.so" ]]; then
    echo "System libpcap development library is unavailable" >&2
    exit 1
fi
# Cgo prepends environment LDFLAGS to package directives. This search directory
# must precede the dependency's bundled -L path to select the shared library.
printf 'CGO_LDFLAGS=-L%s%s\n' "$pcap_libdir" "${CGO_LDFLAGS:+ $CGO_LDFLAGS}" >> "$GITHUB_ENV"
printf 'Linux CI libpcap: %s (%s)\n' "$(pkg-config --modversion libpcap)" "$pcap_libdir"
