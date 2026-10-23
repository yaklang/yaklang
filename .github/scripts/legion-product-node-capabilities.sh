#!/usr/bin/env bash
# Emit host Node capabilities from the baseline and verified binary build tags.
set -euo pipefail

[[ $# == 1 ]] || { echo 'usage: legion-product-node-capabilities.sh BUILD_TAGS' >&2; exit 1; }
script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
jq -ce --arg build_tags "$1" '
  if type == "array" and length > 0 and
     all(.[]; type == "string" and length > 0) and
     . == (sort | unique) and index("ssa.ir.dm.v1") == null
  then .
  else error("product-node baseline must be sorted, unique and exclude optional DM capability")
  end |
  if ($build_tags | split(",") | index("dameng")) != null
  then . + ["ssa.ir.dm.v1"] | sort | unique
  else .
  end
' "$script_dir/legion-product-node-capabilities.json"
