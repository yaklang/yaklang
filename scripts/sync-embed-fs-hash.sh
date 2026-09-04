#!/bin/bash -e
# CI entry point that refreshes the embed FS hash files from a yak binary.
#
#   YAK=/path/to/yak scripts/sync-embed-fs-hash.sh
#
# Two code paths, picked by probing the given binary:
#   * new yak (knows embed-fs-hash --output-file): regenerate straight from the
#     repository resource dirs via scripts/generate-embed-fs-hash.sh.
#   * published legacy yak (only knows --override --all against a hash.go
#     template): keep that old behaviour working after the split into per-resource
#     files -- assemble a throw-away template, let the old binary rewrite it from
#     its own embedded resources, then split the results back into the files.
#
# Unchanged hashes are never rewritten, so an idempotent run leaves no diff.

cd "$(dirname "$0")/.."

if [ -z "${YAK:-}" ]; then
  echo "sync-embed-fs-hash.sh: YAK must point to a yak binary" >&2
  exit 1
fi

if ! [ -f "$YAK" ]; then
  echo "sync-embed-fs-hash.sh: yak binary not found: $YAK" >&2
  exit 1
fi

if "$YAK" embed-fs-hash --help 2>&1 | grep -q -- "--output-file"; then
  exec bash scripts/generate-embed-fs-hash.sh
fi

echo "sync-embed-fs-hash.sh: yak predates --dir/--output-file, using legacy embed-fs-hash --override --all"

YAK_ABS=$(cd "$(dirname "$YAK")" && pwd)/$(basename "$YAK")
WORK=$(mktemp -d)
trap 'rm -rf "$WORK"' EXIT
mkdir -p "$WORK/consts"

FILES=(
  "common/consts/coreplugin-hash.go"
  "common/consts/syntaxflow-hash.go"
  "common/consts/forge-hash.go"
  "common/consts/aitool-hash.go"
)

{
  echo "package consts"
  echo ""
  for f in "${FILES[@]}"; do
    grep '^const ' "$f"
  done
} > "$WORK/consts/hash.go"

( cd "$WORK" && "$YAK_ABS" embed-fs-hash --override --all >/dev/null )

for f in "${FILES[@]}"; do
  const=$(grep '^const ' "$f" | sed -n 's/^const \([A-Za-z0-9_]*\) .*/\1/p')
  [ -n "$const" ] || continue
  value=$(sed -n "s/^const ${const} string = \"\([0-9a-fA-F]*\)\".*/\1/p" "$WORK/consts/hash.go")
  [ -n "$value" ] || continue
  sed -i.bak "s/^\(const ${const} string = \)\"[0-9a-fA-F]*\"/\1\"${value}\"/" "$f"
  rm -f "${f}.bak"
done

echo "sync-embed-fs-hash.sh: legacy sync done"
