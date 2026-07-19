#!/usr/bin/env bash
# Local benchmark convenience wrapper (no Harbor, no Docker).
#
# Three modes, selected by the first argument:
#
#   run_local.sh yak <task> [task...]       run tasks with the local yak engine
#   run_local.sh opencode <task> [task...]  run tasks with the local opencode
#   run_local.sh paired <task>              run <task> with yak(base) vs yak(candidate)
#
# Common env:
#   YAK_AI_CONFIG_FILE   tiered YAML (default: benchmarks/harbor/ai-config.yaml)
#   YAK_BENCHMARK_OUTPUT output root (default: benchmarks/harbor/results/local/<stamp>)
#
# Mode-specific env:
#   yak:       YAK_BINARY_PATH (default /usr/local/bin/yak)
#   opencode:  OPENCODE_BINARY_PATH (default ~/.opencode/bin/opencode)
#   paired:    YAK_BASE_BINARY, YAK_CANDIDATE_BINARY, [OPENCODE_BINARY_PATH]
#
# Examples:
#   YAK_BINARY_PATH=/usr/local/bin/yak \
#     bash benchmarks/harbor/scripts/run_local.sh yak direct-incident-summary
#
#   OPENCODE_BINARY_PATH=~/.opencode/bin/opencode \
#     bash benchmarks/harbor/scripts/run_local.sh opencode direct-incident-summary
#
#   YAK_BASE_BINARY=/tmp/yak-main YAK_CANDIDATE_BINARY=/tmp/yak-new \
#     bash benchmarks/harbor/scripts/run_local.sh paired direct-incident-summary
set -uo pipefail

err() { echo "[run_local] $*" >&2; }

root="$(git rev-parse --show-toplevel)"
runner="$root/benchmarks/harbor/scripts/run_local.py"
config="${YAK_AI_CONFIG_FILE:-$root/benchmarks/harbor/ai-config.yaml}"
stamp="$(date -u +%Y%m%dT%H%M%SZ)"
output="${YAK_BENCHMARK_OUTPUT:-$root/benchmarks/harbor/results/local/$stamp}"
mkdir -p "$output"

if [[ ! -f "$config" ]]; then
  err "YAK_AI_CONFIG_FILE not found: $config"
  err "generate with: python3 benchmarks/harbor/scripts/gen_ai_config_yaml.py"
  exit 2
fi

mode="${1:-}"
shift || true

case "$mode" in
  yak)
    [[ $# -ge 1 ]] || { err "usage: $0 yak <task> [task...] [--task X --attempts N ...]"; exit 2; }
    bin="${YAK_BINARY_PATH:-/usr/local/bin/yak}"
    backend="${YAK_BACKEND:-grpc}"
    # Translate bare task names into --task flags; pass through any --flag as-is.
    py_args=()
    for a in "$@"; do
      if [[ "$a" == -* ]]; then py_args+=("$a"); else py_args+=(--task "$a"); fi
    done
    python3 "$runner" yak \
      --config "$config" \
      --output "$output/yak.jsonl" \
      --yak-binary "$bin" \
      --backend "$backend" \
      --label "${YAK_AGENT_LABEL:-yak}" \
      "${py_args[@]}"
    ;;

  opencode)
    [[ $# -ge 1 ]] || { err "usage: $0 opencode <task> [task...] [--task X ...]"; exit 2; }
    bin="${OPENCODE_BINARY_PATH:-$HOME/.opencode/bin/opencode}"
    py_args=()
    for a in "$@"; do
      if [[ "$a" == -* ]]; then py_args+=("$a"); else py_args+=(--task "$a"); fi
    done
    python3 "$runner" opencode \
      --config "$config" \
      --output "$output/opencode.jsonl" \
      --opencode-binary "$bin" \
      --label opencode \
      "${py_args[@]}"
    ;;

  paired)
    [[ $# -ge 1 ]] || { err "usage: $0 paired <task>"; exit 2; }
    task="$1"
    for var in YAK_BASE_BINARY YAK_CANDIDATE_BINARY; do
      [[ -n "${!var:-}" ]] || { err "missing env: $var"; exit 2; }
    done
    backend="${YAK_BACKEND:-grpc}"
    # base run
    YAK_BINARY_PATH="$YAK_BASE_BINARY" python3 "$runner" yak \
      --config "$config" \
      --output "$output/base.jsonl" \
      --yak-binary "$YAK_BASE_BINARY" \
      --backend "$backend" \
      --label base --task "$task"
    # candidate run
    YAK_BINARY_PATH="$YAK_CANDIDATE_BINARY" python3 "$runner" yak \
      --config "$config" \
      --output "$output/candidate.jsonl" \
      --yak-binary "$YAK_CANDIDATE_BINARY" \
      --backend "$backend" \
      --label candidate --task "$task"
    # verdict
    python3 "$runner" compare \
      "$output/base.jsonl" "$output/candidate.jsonl" \
      --output "$output/verdict.json" || true
    echo "[run_local] verdict written to $output/verdict.json"
    ;;

  *)
    err "unknown mode: ${mode:-<none>}"
    err "usage: $0 {yak|opencode|paired} <task> [task...]"
    exit 2
    ;;
esac
