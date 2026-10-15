#!/usr/bin/env bash
# Test rejection paths in the verifier. Real Node runtime evidence is produced
# separately by verify-linux-nss-startup.sh on both architecture artifacts.
set -euo pipefail
script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
verifier="$script_dir/verify-linux-nss-startup.sh"
test_root="$(mktemp -d "${TMPDIR:-/tmp}/test-linux-nss.XXXXXX")"
trap 'rm -rf -- "$test_root"' EXIT
mkdir -p "$test_root/tools"
printf 'fixture binary\n' >"$test_root/binary"
chmod 755 "$test_root/binary"

cat >"$test_root/tools/file" <<'EOF'
#!/usr/bin/env bash
set -eu
if [[ "$MOCK_FAILURE" == dynamic-file ]]; then
  echo 'ELF 64-bit LSB executable, dynamically linked'
else
  echo 'ELF 64-bit LSB executable, statically linked'
fi
EOF
cat >"$test_root/tools/readelf" <<'EOF'
#!/usr/bin/env bash
set -eu
case "$1" in
  -hW)
    printf '  Class:                             ELF64\n'
    printf "  Data:                              2's complement, little endian\n"
    if [[ "$MOCK_ARCH" == arm64 && "$MOCK_FAILURE" != wrong-arch ]]; then
      echo '  Machine:                           AArch64'
    elif [[ "$MOCK_ARCH" == amd64 && "$MOCK_FAILURE" == wrong-arch ]]; then
      echo '  Machine:                           AArch64'
    else
      echo '  Machine:                           Advanced Micro Devices X86-64'
    fi
    ;;
  -lW)
    if [[ "$MOCK_FAILURE" == interp ]]; then
      echo '  INTERP 0x00 0x00 0x00 0x00 0x00 R 0x1'
    else
      echo '  LOAD 0x00 0x00 0x00 0x00 0x00 R 0x1'
    fi
    ;;
  -dW)
    if [[ "$MOCK_FAILURE" == needed ]]; then
      echo ' 0x0000000000000001 (NEEDED) Shared library: [libc.so]'
    else
      echo 'There is no dynamic section in this file.'
    fi
    ;;
  -VW)
    if [[ "$MOCK_FAILURE" == version-table ]]; then
      echo 'Name: GLIBC_2.28'
    else
      echo 'No version information found in this file.'
    fi
    ;;
  *) exit 99 ;;
esac
EOF
cat >"$test_root/tools/strings" <<'EOF'
#!/usr/bin/env bash
set -eu
if [[ "$MOCK_FAILURE" == glibc-private ]]; then echo GLIBC_PRIVATE; fi
if [[ "$MOCK_FAILURE" != missing-libc-marker ]]; then
  echo 'component=legion-node event=libc c_library=musl version=%s'
fi
EOF
cat >"$test_root/tools/go" <<'EOF'
#!/usr/bin/env bash
set -eu
[[ "$1" == version && "$2" == -m ]]
tags=musl,netgo,osusergo,sqlite_omit_load_extension
if [[ "$MOCK_PROFILE" == product ]]; then tags="hids,$tags"; fi
if [[ "$MOCK_FAILURE" == missing-tag ]]; then tags="${tags/osusergo,/}"; fi
printf '\tbuild\t-tags=%s\n' "$tags"
printf '\tbuild\tGOOS=linux\n'
printf '\tbuild\tGOARCH=%s\n' "$MOCK_ARCH"
printf '\tbuild\tCGO_ENABLED=1\n'
EOF
cat >"$test_root/tools/docker" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
if [[ "$1" == rm ]]; then exit 0; fi
[[ "$1" == run ]]
args=("$@")
scenario="${args[${#args[@]}-2]}"
case "$scenario" in files-nohome|compat-home|compat-nohome) ;; *) exit 99 ;; esac
declare -A found
for ((i=1; i<${#args[@]}; i++)); do
  case "${args[i]}" in
    --platform) [[ "${args[i+1]}" == "linux/$MOCK_ARCH" ]]; found[platform]=1 ;;
    --network) [[ "${args[i+1]}" == none ]]; found[network]=1 ;;
    --read-only) found[readonly]=1 ;;
    --cap-drop) [[ "${args[i+1]}" == ALL ]]; found[caps]=1 ;;
    --security-opt) [[ "${args[i+1]}" == no-new-privileges ]]; found[privileges]=1 ;;
    --ulimit) [[ "${args[i+1]}" == core=0 ]]; found[core]=1 ;;
    -v)
      if [[ "${args[i+1]}" == *:/evidence:ro ]]; then
        dir="${args[i+1]%:/evidence:ro}"
        [[ "$(stat -c %a "$dir")" == 755 ]]
        [[ -x "$dir/binary" ]]
        [[ "$(stat -c %a "$dir/nsswitch.conf")" == 644 ]]
        backend="${scenario%%-*}"
        grep -Fxq "passwd: $backend" "$dir/nsswitch.conf"
        grep -Fxq "group: $backend" "$dir/nsswitch.conf"
        found[fixture]=1
      fi
      ;;
    *@sha256:*)
      if [[ "$MOCK_ARCH" == amd64 ]]; then
        digest=32220ea48be72979c9f0810d69f3de9145c9584c2ee966c6ec26edbcf4640c0b
      else
        digest=7c4d2a7d6cf0863888c4dbe1207211c5a7c2eaa6374a51b5d4cc02dd0dd95099
      fi
      [[ "${args[i]}" == *@sha256:"$digest" ]]
      found[image]=1
      ;;
  esac
done
for setting in platform network readonly caps privileges core fixture image; do
  [[ "${found[$setting]:-}" == 1 ]]
done
printf '%s linux/%s\n' "$scenario" "$MOCK_ARCH" >>"$MOCK_RUN_LOG"
if [[ "$MOCK_FAILURE" == wrong-glibc ]]; then echo 'glibc 2.31'; else echo 'glibc 2.28'; fi
if [[ "$MOCK_FAILURE" == wrong-backend ]]; then
  echo 'nss_backend=unknown'
elif [[ "$MOCK_FAILURE" != missing-backend ]]; then
  printf 'nss_backend=%s\n' "${scenario%%-*}"
fi
if [[ "$scenario" == compat-nohome && "$MOCK_FAILURE" == abort ]]; then
  echo "Inconsistency detected by ld.so: Assertion 'sym != NULL' failed"
  echo SIGABRT
  exit 2
fi
if [[ "$MOCK_FAILURE" != missing-main ]]; then
  echo '2026/10/13 00:00:00 component=legion-node event=startup stage=main status=entered'
fi
if [[ "$MOCK_FAILURE" == runtime-version ]]; then
  echo '2026/10/13 00:00:00 component=legion-node event=libc c_library=musl version=%s'
elif [[ "$MOCK_FAILURE" != missing-libc-marker ]]; then
  echo '2026/10/13 00:00:00 component=legion-node event=libc c_library=musl version=1.2.5'
fi
if [[ "$MOCK_FAILURE" == wrong-error ]]; then
  echo '2026/10/13 00:00:00 connect: connection refused'
else
  echo '2026/10/13 00:00:00 initialize AI Runtime Host: runtime host installed release identity is required'
fi
if [[ "$MOCK_FAILURE" == exit-zero ]]; then exit 0; fi
if [[ "$MOCK_FAILURE" == timed-out ]]; then exit 124; fi
exit 1
EOF
chmod 755 "$test_root/tools/"*

run_verifier() {
  local name="$1" arch="$2" failure="$3"
  shift 3
  : >"$test_root/$name.runs"
  MOCK_ARCH="$arch" MOCK_FAILURE="$failure" MOCK_PROFILE="${MOCK_TAG_PROFILE:-product}" \
    MOCK_RUN_LOG="$test_root/$name.runs" \
    PATH="$test_root/tools:$PATH" \
    "$verifier" --binary "$test_root/binary" --goarch "$arch" \
      --evidence "$test_root/$name" "$@" >"$test_root/$name.log" 2>&1
}

for arch in amd64 arm64; do
  if ! run_verifier "valid-$arch" "$arch" none; then
    cat "$test_root/valid-$arch.log" >&2
    echo "valid $arch fixture rejected" >&2
    exit 1
  fi
  [[ "$(wc -l <"$test_root/valid-$arch.runs")" == 3 ]]
  grep -Fq $'index_digest\tsha256:bb3dc79fddbca7e8903248ab916bb775c96ec61014b3d02b4f06043b604726dc' \
    "$test_root/valid-$arch/image-identity.tsv"
  grep -Fq "linux/$arch" "$test_root/valid-$arch/image-identity.tsv"
  grep -Fxq product "$test_root/valid-$arch/profile.txt"
  for profile in portable runtime; do
    name="valid-$arch-$profile"
    if ! MOCK_TAG_PROFILE="$profile" run_verifier "$name" "$arch" none --profile "$profile"; then
      cat "$test_root/$name.log" >&2
      echo "valid $arch $profile fixture rejected" >&2
      exit 1
    fi
    [[ "$(wc -l <"$test_root/$name.runs")" == 3 ]]
    grep -Fxq "$profile" "$test_root/$name/profile.txt"
  done
done
(
  umask 077
  run_verifier restrictive-umask amd64 none
)
for failure in wrong-arch dynamic-file interp needed version-table glibc-private \
  missing-tag missing-libc-marker abort wrong-glibc missing-main runtime-version \
  wrong-error exit-zero timed-out missing-backend wrong-backend; do
  if run_verifier "$failure" amd64 "$failure"; then
    echo "invalid $failure fixture unexpectedly accepted" >&2
    exit 1
  fi
  grep -Fq '[linux-nss-startup][error]' "$test_root/$failure.log"
done

# The shared musl tags are required for every profile, and HIDS must match the
# actual release recipe rather than being accepted as an optional capability.
for profile in portable runtime; do
  if run_verifier "$profile-with-hids" amd64 none --profile "$profile"; then
    echo "$profile artifact with HIDS unexpectedly accepted" >&2
    exit 1
  fi
  [[ ! -s "$test_root/$profile-with-hids.runs" ]]
  if MOCK_TAG_PROFILE="$profile" run_verifier "$profile-missing-tag" amd64 missing-tag --profile "$profile"; then
    echo "$profile artifact without osusergo unexpectedly accepted" >&2
    exit 1
  fi
  [[ ! -s "$test_root/$profile-missing-tag.runs" ]]
done
if MOCK_TAG_PROFILE=portable run_verifier product-without-hids amd64 none --profile product; then
  echo 'Product artifact without HIDS unexpectedly accepted' >&2
  exit 1
fi
[[ ! -s "$test_root/product-without-hids.runs" ]]
if run_verifier invalid-profile amd64 none --profile unknown; then
  echo 'unsupported profile unexpectedly accepted' >&2
  exit 1
fi
grep -Fq 'unsupported node profile: unknown' "$test_root/invalid-profile.log"
[[ ! -s "$test_root/invalid-profile.runs" ]]

# A digest from the other architecture or from an arbitrary image cannot
# silently change the regression host; using the parent index is ambiguous on
# Docker's classic image store and is rejected too.
for digest in 7c4d2a7d6cf0863888c4dbe1207211c5a7c2eaa6374a51b5d4cc02dd0dd95099 \
  bb3dc79fddbca7e8903248ab916bb775c96ec61014b3d02b4f06043b604726dc \
  1111111111111111111111111111111111111111111111111111111111111111; do
  if NSS_TEST_IMAGE="mirror.invalid/debian@sha256:$digest" run_verifier wrong-image amd64 none; then
    echo 'unpinned or wrong-architecture image unexpectedly accepted' >&2
    exit 1
  fi
  grep -Fq 'NSS_TEST_IMAGE must use the pinned Buster linux/amd64 platform manifest digest' \
    "$test_root/wrong-image.log"
  [[ ! -s "$test_root/wrong-image.runs" ]]
done
NSS_TEST_IMAGE=mirror.invalid/debian@sha256:32220ea48be72979c9f0810d69f3de9145c9584c2ee966c6ec26edbcf4640c0b \
  run_verifier mirrored-image amd64 none

# The old glibc artifact may lack all musl markers. Its first two startup cases
# must still be evaluated, and its third-case crash must make the gate fail.
run_verifier legacy-compatible amd64 missing-libc-marker --startup-only
if run_verifier legacy-abort amd64 abort --startup-only; then
  echo 'legacy #560 crash unexpectedly accepted' >&2
  exit 1
fi
[[ "$(wc -l <"$test_root/legacy-abort.runs")" == 3 ]]
grep -Fq $'files-nohome\t1\tpassed' "$test_root/legacy-abort/results.tsv"
grep -Fq $'compat-home\t1\tpassed' "$test_root/legacy-abort/results.tsv"
grep -Fq $'compat-nohome\t2\tlibc or startup crash' "$test_root/legacy-abort/results.tsv"
printf 'Linux NSS startup verifier fixture tests passed\n'
