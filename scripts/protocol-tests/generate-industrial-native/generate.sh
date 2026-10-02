#!/bin/sh
# CC0-1.0. macOS: CMake, Xcode C compiler, pkg-config/OpenSSL 3, Docker,
# Python 3, tcpdump BPF access, and TShark must already be installed.
# Network is used to fetch/build pinned independent implementations. Test
# traffic stays on localhost (OPC UA) or isolated container veth (GOOSE).
set -eu
work=${1:?usage: generate.sh ABSOLUTE_WORK_DIRECTORY}
case "$work" in /*) ;; *) echo 'work directory must be absolute' >&2; exit 2;; esac
scripts=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
mkdir -p "$work"
source="$work/open62541"
if [ ! -d "$source/.git" ]; then
 git clone --depth 1 --branch v1.5.5 https://github.com/open62541/open62541.git "$source"
fi
[ "$(git -C "$source" rev-parse HEAD)" = 3bdeed5dfe8309cecabf36e04afe956b7f72ebd8 ]
[ -z "$(git -C "$source" status --porcelain)" ]
openssl_prefix=$(pkg-config --variable=prefix openssl)
cmake -S "$source" -B "$source/build" -DUA_ENABLE_ENCRYPTION=OPENSSL -DOPENSSL_ROOT_DIR="$openssl_prefix" -DUA_BUILD_EXAMPLES=OFF -DCMAKE_BUILD_TYPE=Release
cmake --build "$source/build" -j4
cc "$scripts/opcua.c" -I"$source/include" -I"$source/plugins/include" -I"$source/build/src_generated" -I"$source/examples" "$source/build/bin/libopen62541.a" $(pkg-config --libs openssl) -lpthread -lm -o "$work/opcua-native"
python3 "$scripts/capture-opcua.py" "$work/opcua-native" "$work/opcua-archive"
docker build -t pr5013-industrial-native:519b020 "$scripts"
mkdir -p "$work/goose-captures"
docker run --rm --network none --cap-add NET_ADMIN --cap-add NET_RAW --mount "type=bind,src=$work/goose-captures,dst=/output" pr5013-industrial-native:519b020
# Private keys remain under work/opcua-archive/keys and are never archived.
# Captures use fresh nonces, TCP sequence numbers and timestamps on each run.
