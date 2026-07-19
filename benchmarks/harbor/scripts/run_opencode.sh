#!/usr/bin/env bash
# Run the same yak-agent-v1 Harbor tasks with a pinned Linux OpenCode binary.
#
# Required:
#   OPENCODE_BINARY_PATH    Linux OpenCode executable
#   OPENCODE_MODEL          provider/model, e.g. deepseek/deepseek-v4-flash
#   OPENCODE_AI_CONFIG_FILE tiered YAML produced by gen_ai_config_yaml.py
#
# Optional:
#   OPENCODE_BENCHMARK_ATTEMPTS  full-dataset attempts (default: 1)
#   OPENCODE_BENCHMARK_OUTPUT    output root
#   OPENCODE_VARIANT             provider-specific reasoning variant
set -uo pipefail

err() { echo "[run_opencode] $*" >&2; }

for var in OPENCODE_BINARY_PATH OPENCODE_MODEL OPENCODE_AI_CONFIG_FILE; do
  if [[ -z "${!var:-}" ]]; then
    err "missing required env: $var"
    exit 2
  fi
done

if ! command -v harbor >/dev/null 2>&1; then
  err "harbor is not installed; run: uv tool install harbor==0.6.5"
  exit 1
fi
version="$(harbor --version 2>&1)"
if [[ "$version" != *0.6.5* ]]; then
  err "harbor 0.6.5 required, got: $version"
  exit 1
fi
if [[ ! -f "$OPENCODE_BINARY_PATH" ]]; then
  err "OpenCode binary not found: $OPENCODE_BINARY_PATH"
  exit 2
fi
binary_kind="$(file -b "$OPENCODE_BINARY_PATH" 2>/dev/null || true)"
if [[ "$binary_kind" != *ELF* ]]; then
  err "OpenCode binary must be a Linux ELF executable, got: $binary_kind"
  exit 2
fi
if [[ ! -f "$OPENCODE_AI_CONFIG_FILE" ]]; then
  err "AI config not found: $OPENCODE_AI_CONFIG_FILE"
  exit 2
fi

root="$(git rev-parse --show-toplevel)"
dataset="$root/benchmarks/harbor/datasets/yak-agent-v1"
agent="benchmarks.harbor.agents.opencode_agent:OpenCodeAgent"
attempts="${OPENCODE_BENCHMARK_ATTEMPTS:-1}"
stamp="$(date -u +%Y%m%dT%H%M%SZ)"
output="${OPENCODE_BENCHMARK_OUTPUT:-$root/benchmarks/harbor/results/opencode-$stamp}"
config="$(cd "$(dirname "$OPENCODE_AI_CONFIG_FILE")" && pwd)/$(basename "$OPENCODE_AI_CONFIG_FILE")"
binary="$(cd "$(dirname "$OPENCODE_BINARY_PATH")" && pwd)/$(basename "$OPENCODE_BINARY_PATH")"
mkdir -p "$output"

jobs=()
for attempt in $(seq 1 "$attempts"); do
  name="opencode-v1-${stamp}-${attempt}"
  echo "=== running OpenCode attempt $attempt (job=$name) ==="
  args=(
    run
    -p "$dataset"
    --agent-import-path "$agent"
    -m "$OPENCODE_MODEL"
    --agent-env "OPENCODE_BINARY_PATH=$binary"
    --agent-env "OPENCODE_AI_CONFIG_FILE=$config"
    --force-build
    --n-concurrent 1
    -k 1
    --job-name "$name"
    -o "$output"
    -y
  )
  if [[ -n "${OPENCODE_VARIANT:-}" ]]; then
    args+=(--agent-env "OPENCODE_VARIANT=$OPENCODE_VARIANT")
  fi
  if harbor "${args[@]}"; then
    jobs+=("$output/$name")
  else
    err "attempt $attempt failed; continuing"
  fi
done

if [[ ${#jobs[@]} -eq 0 ]]; then
  err "all OpenCode jobs failed"
  exit 1
fi

python3 "$root/benchmarks/harbor/scripts/harbor_results_to_jsonl.py" \
  "${jobs[@]}" --label opencode --output "$output/opencode.jsonl"
echo "done. results under $output"
