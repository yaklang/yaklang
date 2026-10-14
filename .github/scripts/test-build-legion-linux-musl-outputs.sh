#!/usr/bin/env bash
# Exercise the real builder's output handling on a protected sticky filesystem.
# Compiler/ELF tools are fixtures; filesystem permissions, chown and rename are
# real. Full Node compilation is covered separately by the native musl jobs.
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
test_root="$(mktemp -d)"
trap 'rm -rf "$test_root"' EXIT
mkdir -p "$test_root/bin" "$test_root/source"
printf '3.21.3\n' >"$test_root/alpine-release"
printf 'module fixture\n\ngo 1.22.12\n' >"$test_root/source/go.mod"

cat >"$test_root/bin/tool" <<'SH'
#!/bin/sh
set -eu
case "$(basename "$0")" in
  go)
    case "$1" in
      env)
        case "$2" in
          GOVERSION) echo go1.22.12 ;;
          GOHOSTOS) echo linux ;;
          GOHOSTARCH) echo "$NODE_GOARCH" ;;
          *) exit 1 ;;
        esac ;;
      version)
        if [ "${2:-}" = -m ]; then
          case "$NODE_BUILD_PROFILE" in
            product) tags=hids,musl,netgo,osusergo,sqlite_omit_load_extension ;;
            *) tags=musl,netgo,osusergo,sqlite_omit_load_extension ;;
          esac
          printf '\tbuild\t-tags=%s\n\tbuild\tCGO_ENABLED=1\n\tbuild\tGOARCH=%s\n' "$tags" "$NODE_GOARCH"
        else
          printf 'go version go1.22.12 linux/%s\n' "$NODE_GOARCH"
        fi ;;
      build)
        [ "${MUSL_OUTPUT_TEST_FAILURE:-}" != build ] || exit 42
        output=
        while [ "$#" -gt 0 ]; do
          if [ "$1" = -o ]; then output="$2"; break; fi
          shift
        done
        test -n "$output"
        generation=0
        if [ -f /tmp/fixture-generation ]; then generation="$(cat /tmp/fixture-generation)"; fi
        generation=$((generation + 1))
        printf '%s\n' "$generation" >/tmp/fixture-generation
        # Go links to a temporary file and renames it over -o. Preserve this
        # behavior so the old helper reaches its failing metadata redirection.
        linked="$(mktemp "$(dirname "$output")/.fixture-link.XXXXXX")"
        {
          printf '#!/bin/sh\n# generation %s\n' "$generation"
          printf '%s\n' '[ "${MUSL_OUTPUT_TEST_FAILURE:-}" != startup ] || exit 42'
          printf '%s\n' 'echo "component=legion-node event=startup stage=main status=entered"'
          printf '%s\n' 'echo "component=legion-node event=libc c_library=musl version=1.2.5"'
          printf '%s\n' 'echo "flag: help requested"' 'exit 1'
        } >"$linked"
        chmod 755 "$linked"
        mv -f "$linked" "$output" ;;
      *) exit 1 ;;
    esac ;;
  gcc)
    if [ "$1" = -dumpfullversion ]; then echo 14.2.0; exit 0; fi
    while [ "$1" != -o ]; do shift; done
    printf '#!/bin/sh\necho 1.2.5\n' >"$2"
    chmod 755 "$2" ;;
  file)
    case "$NODE_GOARCH" in amd64) machine=x86-64 ;; *) machine='ARM aarch64' ;; esac
    printf '%s: ELF 64-bit LSB executable, %s, statically linked\n' "$1" "$machine" ;;
  readelf)
    case "$2" in
      */dynamic-probe) printf 'INTERP /lib/ld-musl-%s.so.1\n' "$(uname -m)" ;;
      *) [ "${MUSL_OUTPUT_TEST_FAILURE:-}" != verify ] || exit 42 ;;
    esac ;;
  strings) : ;;
  apk) echo musl-1.2.5-r11 ;;
  jq)
    if [ "${MUSL_OUTPUT_TEST_FAILURE:-}" = metadata ]; then
      printf '{incomplete'
      exit 42
    fi
    printf '{"fixture":true,"generation":%s}\n' "$(cat /tmp/fixture-generation)" ;;
  *) exit 1 ;;
esac
SH
chmod 755 "$test_root/bin/tool"
for command in go gcc file readelf strings apk jq; do
  ln -s tool "$test_root/bin/$command"
done

cat >"$test_root/run.sh" <<'SH'
#!/bin/sh
set -eu
export PATH="/fixture/bin:$PATH"
export NODE_SOURCE_DIR=/fixture/source NODE_GOARCH NODE_BUILD_PROFILE=product NODE_BUILDVCS=false
export NODE_MUSL_BUILDER_IMAGE='docker.io/library/golang:1.22.12-alpine3.21@sha256:1699c10032ca2582ec89a24a1312d986a3f094aed3d5c1147b19880afe40e052'
export NODE_OUTPUT_UID=1000 NODE_OUTPUT_GID=1000
helper=/recipe/build-legion-linux-musl.sh
protection="$(cat /proc/sys/fs/protected_regular)"
case "$protection" in 1|2) ;; *) echo 'sticky-file protection must be enabled' >&2; exit 1 ;; esac
test "$(stat -c %a /tmp)" = 1777
printf 'fs.protected_regular=%s output_uid=%s\n' "$protection" "$NODE_OUTPUT_UID"

assert_no_staging() {
  test -z "$(find /tmp /metadata -maxdepth 1 -type d \( -name '.legion-musl-build.*' -o -name 'legion-musl-probe.*' \) -print)"
}

for placement in default separate; do
  export NODE_OUTPUT_BINARY="/tmp/node-$placement"
  if [ "$placement" = default ]; then
    export NODE_MUSL_METADATA_FILE="$NODE_OUTPUT_BINARY.musl-build.json"
  else
    export NODE_MUSL_METADATA_FILE='/metadata/recipe with spaces.json'
  fi
  for build in first second; do
    if ! "$helper" --in-container >/tmp/build.log 2>&1; then
      cat /tmp/build.log >&2
      echo "$placement $build build failed" >&2
      exit 1
    fi
    test "$(stat -c %u "$NODE_OUTPUT_BINARY")" = 1000
    test "$(stat -c %u "$NODE_MUSL_METADATA_FILE")" = 1000
    test "$(stat -c %a "$NODE_OUTPUT_BINARY")" = 755
    assert_no_staging
    printf '%s %s build passed with non-root-owned outputs\n' "$placement" "$build"
  done
  sha256sum "$NODE_OUTPUT_BINARY" "$NODE_MUSL_METADATA_FILE" >/tmp/previous.sha256
  for failure in build verify startup metadata; do
    if MUSL_OUTPUT_TEST_FAILURE="$failure" "$helper" --in-container >/tmp/build.log 2>&1; then
      echo "accepted injected $failure failure" >&2
      exit 1
    fi
    sha256sum -c /tmp/previous.sha256
    assert_no_staging
    printf '%s %s failure preserved both previous outputs\n' "$placement" "$failure"
  done
done
printf '[legion-node-musl-outputs-test] PASS\n'
SH
chmod 755 "$test_root/run.sh"
case "$(uname -m)" in
  x86_64) arch=amd64 ;;
  aarch64) arch=arm64 ;;
  *) echo 'output regression requires a native Linux amd64/arm64 host' >&2; exit 1 ;;
esac
image="${MUSL_OUTPUT_TEST_IMAGE:-docker.io/library/debian@sha256:88200866dfff7ea7f5cbcb6ec7c8a701889efe6fe859fe64d6990e4b07ea4171}"
docker run --rm --platform "linux/$arch" --network none --read-only \
  --cap-drop ALL --cap-add CHOWN --cap-add FOWNER --cap-add DAC_OVERRIDE \
  --tmpfs /tmp:rw,exec,mode=1777 --tmpfs /metadata:rw,mode=1777 \
  -v "$test_root:/fixture:ro" \
  -v "$repo_root/.github/scripts/build-legion-linux-musl.sh:/recipe/build-legion-linux-musl.sh:ro" \
  -v "$test_root/alpine-release:/etc/alpine-release:ro" \
  -e "NODE_GOARCH=$arch" --entrypoint /bin/sh "$image" /fixture/run.sh
