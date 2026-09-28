#!/usr/bin/env bash
# Exercise package initialization without HOME on an older, pinned glibc host.
set -euo pipefail
binary="$(realpath "${1:?binary required}")"
kind="${2:?node or engine required}"
evidence="${3:?evidence directory required}"
[[ -x "$binary" ]] || { echo 'binary must be executable' >&2; exit 1; }
case "$kind" in node|engine) ;; *) echo 'expected node or engine' >&2; exit 1 ;; esac
# This digest is the amd64 Buster image. ARM builds retain compile/metadata gates;
# this regression must run on a native amd64 Docker host.
image="${NSS_TEST_IMAGE:-debian@sha256:bb3dc79fddbca7e8903248ab916bb775c96ec61014b3d02b4f06043b604726dc}"
mkdir -p "$evidence"
evidence="$(realpath "$evidence")"
: >"$evidence/results.txt"
fixture="$(mktemp -d)"
container="yaklang-nss-${fixture##*.}"
cleanup() {
  docker rm -f "$container" >/dev/null 2>&1 || true
  rm -rf -- "$fixture"
}
trap cleanup EXIT
chmod 755 "$fixture"
cp "$binary" "$fixture/binary"
chmod 755 "$fixture/binary"
printf 'println("NSS_STARTUP_OK")\n' >"$fixture/smoke.yak"
sha256sum "$binary" >"$evidence/binary.sha256"
go version -m "$binary" >"$evidence/build-info.txt"
printf '%s\n' "$image" >"$evidence/image.txt"
for scenario in files-nohome compat-home compat-nohome; do
  backend="${scenario%%-*}"
  printf 'passwd: %s\ngroup: %s\nshadow: files\nhosts: files dns\n' \
    "$backend" "$backend" >"$fixture/nsswitch.conf"
  status=0
  docker run --rm --name "$container" --platform linux/amd64 \
    --network none --read-only --cap-drop ALL --security-opt no-new-privileges \
    --pids-limit 128 --memory 2g --ulimit core=0 \
    --tmpfs /tmp:rw,size=256m --tmpfs /root:rw,size=256m \
    --tmpfs /yakit-projects:rw,size=256m \
    -v "$fixture:/evidence:ro" \
    -v "$fixture/nsswitch.conf:/etc/nsswitch.conf:ro" \
    "$image" sh -c '
      set -eu
      test "$(getconf GNU_LIBC_VERSION)" = "glibc 2.28"
      unset HOME YAKIT_HOME
      if [ "$1" = compat-home ]; then export HOME=/root; fi
      if [ "$2" = node ]; then
        exec timeout 20 /evidence/binary -api-url http://127.0.0.1:1 \
          -base-dir /tmp/node -runtime-host=true -enrollment-token invalid-nss-regression
      else
        exec timeout 20 /evidence/binary /evidence/smoke.yak
      fi
    ' sh "$scenario" "$kind" >"$evidence/$scenario.log" 2>&1 || status=$?
  printf '%s exit=%s\n' "$scenario" "$status" | tee -a "$evidence/results.txt"
  if grep -Eq 'SIGABRT|Assertion .*failed|symbol lookup error|Segmentation fault' "$evidence/$scenario.log"; then
    echo "libc/startup failure: $evidence/$scenario.log" >&2
    tail -n 30 "$evidence/$scenario.log" >&2
    exit 1
  fi
  if [[ "$kind" == node ]]; then
    # Deliberately stop after main, before any enrollment; exit 1 alone is not a pass.
    [[ "$status" == 1 ]]
    grep -Fq 'event=startup stage=main status=entered' "$evidence/$scenario.log"
    grep -Fq 'runtime host installed release identity is required' "$evidence/$scenario.log"
  else
    [[ "$status" == 0 ]]
    grep -Fxq 'NSS_STARTUP_OK' "$evidence/$scenario.log"
  fi
done
# A compatible old libc may pass startup even without the intended build tag.
# Check the artifact itself, not only the workflow source spelling.
awk '
  $1 == "build" && $2 ~ /^-tags=/ {
    sub(/^-tags=/, "", $2)
    count = split($2, tags, ",")
    for (i = 1; i <= count; i++) if (tags[i] == "osusergo") found = 1
  }
  END { exit !found }
' "$evidence/build-info.txt" || { echo 'artifact is missing osusergo' >&2; exit 1; }
echo 'glibc 2.28 NSS startup regression passed (not registration/HIDS acceptance)'
