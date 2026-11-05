#!/usr/bin/env bash
# Lift one binary with every Reify mode and archive the text output.
set -euo pipefail

usage() {
	cat <<'EOF'
Usage: lift-all.sh [-o OUTPUT_DIR] [-a ADDRESS -s SIZE] <file>

Runs `reify lift` for asm, lowir, ssair and source, and writes one text file
per mode. The input file name prefixes every output.

Options:
  -o OUTPUT_DIR   Output directory (default: ./reify-out)
  -a ADDRESS      Lift one region starting at an image address
  -s SIZE         Region size in bytes; required with -a
  -h              Show this help

Files written:
  <output>/version               version, commit hash and input metadata
  <output>/<name>.asm.txt        disassembly text
  <output>/<name>.lowir.txt      LowIR text
  <output>/<name>.ssair.txt      YakSSA text
  <output>/<name>.source.txt     source-like text

Requires go and git. Runs from the repository root, so relative Go package
paths resolve. Exits 1 if any mode fails; failed modes keep diagnostics in
their text file.
EOF
}

fail() {
	printf 'lift-all: %s\n' "$*" >&2
	exit 1
}

output_dir="./reify-out"
address=""
size=""
while getopts ":o:a:s:h" option; do
	case "$option" in
	o) output_dir=$OPTARG ;;
	a) address=$OPTARG ;;
	s) size=$OPTARG ;;
	h)
		usage
		exit 0
		;;
	:)
		printf 'lift-all: option -%s requires a value\n' "$OPTARG" >&2
		usage >&2
		exit 2
		;;
	\?)
		printf 'lift-all: unknown option -%s\n' "$OPTARG" >&2
		usage >&2
		exit 2
		;;
	esac
done
shift $((OPTIND - 1))

if [ "$#" -ne 1 ]; then
	usage >&2
	exit 2
fi

if { [ -n "$address" ] && [ -z "$size" ]; } || { [ -z "$address" ] && [ -n "$size" ]; }; then
	fail "options -a and -s must be used together"
fi

input=$1
[ -e "$input" ] || fail "input does not exist: $input"
[ -f "$input" ] || fail "input is not a regular file: $input"
[ -r "$input" ] || fail "input is not readable: $input"

command -v go >/dev/null 2>&1 || fail "go is not installed"
command -v git >/dev/null 2>&1 || fail "git is not installed"

script_dir=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
repo_root=$(git -C "$script_dir" rev-parse --show-toplevel 2>/dev/null || true)
if [ -z "$repo_root" ]; then
	repo_root=$(cd "$script_dir/../../../.." && pwd)
fi

input_abs=$(cd "$(dirname "$input")" && pwd)/$(basename "$input")
base=$(basename "$input")
mkdir -p "$output_dir"
output_dir=$(cd "$output_dir" && pwd)

version=$(git -C "$repo_root" describe --tags --always --dirty 2>/dev/null || echo unknown)
commit=$(git -C "$repo_root" rev-parse HEAD 2>/dev/null || echo unknown)
branch=$(git -C "$repo_root" rev-parse --abbrev-ref HEAD 2>/dev/null || echo unknown)
digest=$(sha256sum "$input_abs" | awk '{print $1}')

{
	printf 'version: %s\n' "$version"
	printf 'commit: %s\n' "$commit"
	printf 'branch: %s\n' "$branch"
	printf 'generated_at: %s\n' "$(date -Iseconds)"
	printf 'input: %s\n' "$input_abs"
	printf 'input_sha256: %s\n' "$digest"
	printf 'go: %s\n' "$(go version)"
	if [ -n "$address" ]; then
		printf 'region: address=%s size=%s\n' "$address" "$size"
	else
		printf 'region: .text\n'
	fi
	printf 'modes: asm lowir ssair source\n'
} >"$output_dir/version"

failed=0
for mode in asm lowir ssair source; do
	target="$output_dir/$base.$mode.txt"
	stderr_file="$output_dir/.$base.$mode.stderr"
	printf 'lift-all: %s -> %s\n' "$mode" "$target" >&2
	args=(lift --mode "$mode")
	if [ -n "$address" ]; then
		args+=(--address "$address" --size "$size")
	fi
	args+=("$input_abs")
	if (cd "$repo_root" && go run ./common/yak/reify/cmd/reify "${args[@]}") >"$target" 2>"$stderr_file"; then
		rm "$stderr_file"
		continue
	else
		status=$?
	fi
	failed=1
	{
		printf 'reify lift --mode %s failed with exit %d\n' "$mode" "$status"
		cat "$stderr_file"
	} >>"$target"
	rm "$stderr_file"
done

if [ "$failed" -ne 0 ]; then
	printf 'lift-all: one or more modes failed; see %s\n' "$output_dir" >&2
	exit 1
fi

printf 'lift-all: wrote %s\n' "$output_dir" >&2
printf '%s\n' "$output_dir"
