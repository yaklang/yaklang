#!/usr/bin/env bash
# Verify a built Linux Node, including the pre-main #560 regression.
set -euo pipefail
export LC_ALL=C

die() {
  printf '[linux-nss-startup][error] %s\n' "$*" >&2
  exit 1
}

usage() {
  printf 'Usage: %s --binary PATH --goarch amd64|arm64 --evidence DIR [--profile product|portable|runtime] [--startup-only]\n' "$0" >&2
  exit 1
}

binary=""
goarch=""
evidence=""
profile=product
startup_only=false
while [[ $# -gt 0 ]]; do
  case "$1" in
    --binary|--goarch|--evidence|--profile)
      [[ $# -ge 2 && -n "$2" ]] || usage
      case "$1" in
        --binary) binary="$2" ;;
        --goarch) goarch="$2" ;;
        --evidence) evidence="$2" ;;
        --profile) profile="$2" ;;
      esac
      shift 2
      ;;
    # Negative controls use the same runtime cases without demanding musl tags
    # from the old glibc artifact. Release/PR gates must use the default mode.
    --startup-only) startup_only=true; shift ;;
    *) usage ;;
  esac
done
[[ -n "$binary" && -n "$goarch" && -n "$evidence" ]] || usage
case "$profile" in
  product|portable|runtime) ;;
  *) die "unsupported node profile: $profile" ;;
esac
case "$goarch" in
  amd64)
    expected_machine='Advanced Micro Devices X86-64'; expected_uname=x86_64
    image_manifest_digest=32220ea48be72979c9f0810d69f3de9145c9584c2ee966c6ec26edbcf4640c0b
    ;;
  arm64)
    expected_machine=AArch64; expected_uname=aarch64
    image_manifest_digest=7c4d2a7d6cf0863888c4dbe1207211c5a7c2eaa6374a51b5d4cc02dd0dd95099
    ;;
  *) die "unsupported node architecture: $goarch" ;;
esac
for command in docker file readelf realpath sha256sum timeout; do
  command -v "$command" >/dev/null 2>&1 || die "missing required command: $command"
done
if [[ "$startup_only" == false ]]; then
  for command in go strings; do
    command -v "$command" >/dev/null 2>&1 || die "missing required command: $command"
  done
fi
[[ -f "$binary" && -x "$binary" ]] || die "binary must be an executable file: $binary"
binary="$(realpath "$binary")"
mkdir -p "$evidence"
evidence="$(realpath "$evidence")"
printf '%s\n' "$profile" >"$evidence/profile.txt"
sha256sum "$binary" >"$evidence/binary.sha256"
file "$binary" >"$evidence/file.txt"
readelf -hW "$binary" >"$evidence/elf-header.txt"
grep -Eq 'Class:[[:space:]]+ELF64$' "$evidence/elf-header.txt" || die 'node must be an ELF64 binary'
grep -Eq 'Data:.*little endian$' "$evidence/elf-header.txt" || die 'node must be a little-endian binary'
grep -Eq "Machine:[[:space:]]+$expected_machine\$" "$evidence/elf-header.txt" || \
  die "node ELF does not match linux/$goarch"

if [[ "$startup_only" == false ]]; then
  grep -Fq 'statically linked' "$evidence/file.txt" || die 'node is not statically linked'
  readelf -lW "$binary" >"$evidence/elf-program-headers.txt"
  readelf -dW "$binary" >"$evidence/elf-dynamic.txt"
  readelf -VW "$binary" >"$evidence/elf-versions.txt"
  if grep -Eq '(^|[[:space:]])INTERP([[:space:]]|$)' "$evidence/elf-program-headers.txt"; then
    die 'node declares a PT_INTERP dynamic loader'
  fi
  if grep -Fq '(NEEDED)' "$evidence/elf-dynamic.txt"; then
    die 'node declares DT_NEEDED dynamic libraries'
  fi
  # Static glibc has no ELF version table, but still embeds GLIBC_PRIVATE for
  # dlopen/NSS. Inspect strings as well; absence of DT_NEEDED alone is insufficient.
  strings -a "$binary" | awk '
    /GLIBC_(PRIVATE|[0-9]+(\.[0-9]+)+)/ {
      match($0, /GLIBC_(PRIVATE|[0-9]+(\.[0-9]+)+)/)
      print substr($0, RSTART, RLENGTH)
    }
    index($0, "component=legion-node event=libc c_library=musl version=") {
      print "component=legion-node event=libc c_library=musl version="
    }
  ' >"$evidence/libc-markers.txt"
  if grep -Eq 'GLIBC_(PRIVATE|[0-9])' "$evidence/elf-versions.txt" "$evidence/libc-markers.txt"; then
    die 'node contains glibc version references'
  fi
  grep -Fq 'component=legion-node event=libc c_library=musl version=' "$evidence/libc-markers.txt" || \
    die 'node is missing the musl runtime identity marker'

  go version -m "$binary" >"$evidence/build-info.txt"
  awk -v arch="$goarch" -v profile="$profile" '
    $1 == "build" {
      if ($2 == "GOOS=linux") linux = 1
      if ($2 == "GOARCH=" arch) correct_arch = 1
      if ($2 == "CGO_ENABLED=1") cgo = 1
      if ($2 ~ /^-tags=/) {
        sub(/^-tags=/, "", $2)
        count = split($2, tags, ",")
        for (i = 1; i <= count; i++) found[tags[i]] = 1
      }
    }
    END {
      hids_matches = profile == "product" ? found["hids"] : !found["hids"]
      exit !(linux && correct_arch && cgo && hids_matches && found["musl"] &&
        found["osusergo"] && found["netgo"] && found["sqlite_omit_load_extension"])
    }
  ' "$evidence/build-info.txt" || die "node build metadata does not describe the target musl $profile recipe"
fi

# These platform manifests belong to the pinned Debian Buster multi-architecture
# index; both use glibc 2.28. Select the child directly so Docker's classic image
# store can retain both architectures on the same host. Registry mirrors may
# change, but the selected architecture's exact content must remain pinned.
image_index_digest=bb3dc79fddbca7e8903248ab916bb775c96ec61014b3d02b4f06043b604726dc
image="${NSS_TEST_IMAGE:-debian@sha256:$image_manifest_digest}"
[[ "$image" == *@sha256:"$image_manifest_digest" ]] || \
  die "NSS_TEST_IMAGE must use the pinned Buster linux/$goarch platform manifest digest"
printf '%s\n' "$image" >"$evidence/image.txt"
printf 'index_digest\tsha256:%s\nplatform\tlinux/%s\nplatform_manifest_digest\tsha256:%s\nimage\t%s\n' \
  "$image_index_digest" "$goarch" "$image_manifest_digest" "$image" >"$evidence/image-identity.tsv"
printf 'scenario\texit\tresult\n' >"$evidence/results.tsv"

fixture="$(mktemp -d "${TMPDIR:-/tmp}/yaklang-nss.XXXXXX")"
container="yaklang-nss-${fixture##*.}"
cleanup() {
  docker rm -f "$container" >/dev/null 2>&1 || true
  rm -rf -- "$fixture"
}
trap cleanup EXIT
# CAP_DROP=ALL means container root cannot traverse a mode-700 host fixture.
chmod 755 "$fixture"
cp "$binary" "$fixture/binary"
chmod 755 "$fixture/binary"
failed=false
for scenario in files-nohome compat-home compat-nohome; do
  backend="${scenario%%-*}"
  printf 'passwd: %s\ngroup: %s\nshadow: files\nhosts: files dns\n' \
    "$backend" "$backend" >"$fixture/nsswitch.conf"
  status=0
  # shellcheck disable=SC2016 # Expand environment and arguments inside the container shell.
  timeout --signal=TERM --kill-after=5s 35s \
    docker run --rm --name "$container" --platform "linux/$goarch" \
      --network none --read-only --cap-drop ALL --security-opt no-new-privileges \
      --pids-limit 128 --memory 2g --ulimit core=0 \
      --tmpfs /tmp:rw,size=256m --tmpfs /root:rw,size=256m \
      --tmpfs /yakit-projects:rw,size=256m \
      -v "$fixture:/evidence:ro" \
      -v "$fixture/nsswitch.conf:/etc/nsswitch.conf:ro" \
      "$image" sh -c '
        set -eu
        libc_version="$(getconf GNU_LIBC_VERSION)"
        printf "%s\n" "$libc_version"
        test "$libc_version" = "glibc 2.28"
        test "$(uname -m)" = "$2"
        unset HOME YAKIT_HOME
        if [ "$1" = compat-home ]; then export HOME=/root; fi
        exec timeout 20 /evidence/binary -api-url http://127.0.0.1:1 \
          -base-dir /tmp/node -runtime-host=true -enrollment-token invalid-nss-regression
      ' sh "$scenario" "$expected_uname" >"$evidence/$scenario.log" 2>&1 || status=$?

  result=passed
  log_file="$evidence/$scenario.log"
  if grep -Eq 'SIGABRT|SIGSEGV|Assertion .*failed|symbol lookup error|Segmentation fault|Aborted' "$log_file"; then
    result='libc or startup crash'
  elif [[ "$status" != 1 ]]; then
    result='unexpected exit status'
  elif ! grep -Fxq 'glibc 2.28' "$log_file"; then
    result='missing glibc 2.28 precondition'
  elif ! grep -Fq 'component=legion-node event=startup stage=main status=entered' "$log_file"; then
    result='main was not entered'
  elif ! grep -Eq 'initialize AI Runtime Host: runtime host installed release identity is required$' "$log_file"; then
    result='missing expected release identity rejection'
  elif [[ "$startup_only" == false ]] && \
    ! grep -Eq '(^|[[:space:]])component=legion-node event=libc c_library=musl version=[0-9]+\.[0-9]+(\.[0-9]+)?([[:space:]]|$)' "$log_file"; then
    result='missing real musl runtime version'
  fi
  printf '%s\t%s\t%s\n' "$scenario" "$status" "$result" | tee -a "$evidence/results.tsv"
  if [[ "$result" != passed ]]; then
    failed=true
    tail -n 30 "$log_file" >&2
  fi
done
[[ "$failed" == false ]] || die "NSS startup regression failed; evidence: $evidence"
printf 'glibc 2.28 NSS startup regression passed for linux/%s profile=%s (initialization only)\n' "$goarch" "$profile"
