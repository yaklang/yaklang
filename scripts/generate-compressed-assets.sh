#!/usr/bin/env bash
# Extend the existing gzip-embed release preparation; ordinary local builds use
# the original files through !gzip_embed and do not require this step.
set -euo pipefail
cd "$(dirname "$0")/.."

gzip_embed_tool="${GZIP_EMBED:-gzip-embed}"
"$gzip_embed_tool" --source embed/data --source embed/dataex --base embed --gz embed/resources.tar.gz --include-targz --no-embed
"$gzip_embed_tool" --source common/crep/static --root-path --gz common/crep/static.tar.gz --no-embed
"$gzip_embed_tool" --source common/thirdparty_bin/bin_cfg.yml --base common/thirdparty_bin --gz common/thirdparty_bin/config.tar.gz --no-embed
"$gzip_embed_tool" --source common/syntaxflow/sfbuildin/standards/mappings.yaml --base common/syntaxflow/sfbuildin/standards --gz common/syntaxflow/sfbuildin/standards/mappings.tar.gz --no-embed
"$gzip_embed_tool" --source common/syntaxflow/sfdb/rule_versions.json --base common/syntaxflow/sfdb --gz common/syntaxflow/sfdb/rule_versions.tar.gz --no-embed
