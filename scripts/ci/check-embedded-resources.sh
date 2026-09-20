#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/../.."
(
  cd embed
  go run ../common/utils/embedfs/generate -check -package embed -var FS -output resources_embed.go data dataex
)
(
  cd common/crep
  go run ../utils/embedfs/generate -check -package crep -var staticFS -output static_embed.go static
)
(
  cd common/thirdparty_bin
  go run ../utils/embedfs/generate -check -package thirdparty_bin -var configFS -output resources_embed.go bin_cfg.yml
)
(
  cd common/syntaxflow/sfbuildin/standards
  go run ../../../utils/embedfs/generate -check -package standards -var mappingsFS -output resources_embed.go mappings.yaml
)
(
  cd common/syntaxflow/sfdb
  go run ../../utils/embedfs/generate -check -package sfdb -var ruleVersionFS -output rule_version_resources_embed.go -build-tag '!irify_exclude' rule_versions.json
)
