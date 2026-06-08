#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../../.." && pwd)"

CONFIG_PATH="${1:-${SCRIPT_DIR}/generic-event-runner.example.json}"
if [[ $# -gt 0 ]]; then
  shift
fi

cd "${REPO_ROOT}"

go run common/yak/cmd/yak.go \
  "${SCRIPT_DIR}/generic-event-runner.yak" \
  --config "${CONFIG_PATH}" \
  --api-key you-key
  "$@"
