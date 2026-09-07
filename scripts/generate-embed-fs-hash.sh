#!/bin/bash -e
# Regenerate all embed FS hash files from the repository resource directories.
#
# Usage:
#   scripts/generate-embed-fs-hash.sh
#
# Set YAK to a prebuilt yak binary to skip the local build, e.g.:
#   YAK=/path/to/yak scripts/generate-embed-fs-hash.sh
#
# The script resolves its own location, so it can be run from any directory.

cd "$(dirname "$0")/.."

if [ -n "${YAK:-}" ]; then
  run_yak() { "$YAK" "$@"; }
else
  run_yak() { go run ./common/yak/cmd/yak.go "$@"; }
fi

run_yak embed-fs-hash \
  --type coreplugin --dir common/coreplugin/base-yak-plugin \
  --output-file common/consts/coreplugin-hash.go
run_yak embed-fs-hash \
  --type syntaxflow --dir common/syntaxflow/sfbuildin/buildin \
  --output-file common/consts/syntaxflow-hash.go
run_yak embed-fs-hash \
  --type forge --dir common/aiforge/buildinforge \
  --output-file common/consts/forge-hash.go
run_yak embed-fs-hash \
  --type aitool --dir common/ai/aid/aitool/buildinaitools/yakscripttools/yakscriptforai \
  --output-file common/consts/aitool-hash.go
