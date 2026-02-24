#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

go run common/ai/aid/aireact/reactloops/loop_default/testprompt/testprompt.go "$@"
