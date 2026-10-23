#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
helper="$script_dir/legion-product-node-capabilities.sh"
for build_tags in '' hids hids,musl,dameng musl,dameng,netgo dameng dameng_debug hids,nodameng; do
  actual="$(bash "$helper" "$build_tags")"
  expected=false
  case ",$build_tags," in *,dameng,*) expected=true ;; esac
  jq -e --argjson expected "$expected" '
    . == (sort | unique) and
    ((index("ssa.ir.dm.v1") != null) == $expected) and
    (index("yak.execute") != null)
  ' <<<"$actual" >/dev/null
done

grep -Fq 'legion-product-node-capabilities.sh" "$build_tags"' "$script_dir/build-legion-product-node.sh"
echo 'Product Node optional DM capability contract tests passed'
