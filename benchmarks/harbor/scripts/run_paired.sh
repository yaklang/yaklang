#!/usr/bin/env bash
# Run paired base/candidate Yak Agent benchmark trials under Harbor 0.6.5.
#
# Required env:
#   YAK_BASE_BINARY       absolute path to the "main" yak build (linux/arm64)
#   YAK_CANDIDATE_BINARY  absolute path to the candidate yak build
#   YAK_AI_SERVICE        provider type for the gateway (e.g. openai)
#   YAK_AI_MODEL          exact model id, identical for base + candidate
#   YAK_AI_CONFIG_FILE    path to the tiered-ai-config YAML (see
#                         gen_ai_config_yaml.py). Seeded into the container
#                         before each run so the gateway uses the real model
#                         instead of silently falling back to the free one.
#
# Optional env:
#   YAK_BENCHMARK_ATTEMPTS  attempts per (binary, task) (default 5)
#   YAK_BENCHMARK_OUTPUT    output root (default: results/<UTC stamp>)
set -uo pipefail

err() { echo "[run_paired] $*" >&2; }

for var in YAK_BASE_BINARY YAK_CANDIDATE_BINARY YAK_AI_SERVICE YAK_AI_MODEL YAK_AI_CONFIG_FILE; do
  if [[ -z "${!var:-}" ]]; then
    err "missing required env: $var"
    exit 2
  fi
done

# --- Harbor version pin -----------------------------------------------------
if ! command -v harbor >/dev/null 2>&1; then
  err "harbor is not installed; run: uv tool install harbor==0.6.5"
  exit 1
fi
harbor_version="$(harbor --version 2>&1)"
if [[ "$harbor_version" != *0.6.5* ]]; then
  err "harbor 0.6.5 required, got: $harbor_version"
  err "fix with: uv tool install harbor==0.6.5"
  exit 1
fi

if [[ ! -f "$YAK_AI_CONFIG_FILE" ]]; then
  err "YAK_AI_CONFIG_FILE not found: $YAK_AI_CONFIG_FILE"
  err "generate it with: python3 benchmarks/harbor/scripts/gen_ai_config_yaml.py"
  exit 2
fi

attempts="${YAK_BENCHMARK_ATTEMPTS:-5}"
root="$(git rev-parse --show-toplevel)"
dataset="$root/benchmarks/harbor/datasets/yak-agent-v1"
agent_import="benchmarks.harbor.agents.yak_agent:YakAgent"
stamp="$(date -u +%Y%m%dT%H%M%SZ)"
output_root="${YAK_BENCHMARK_OUTPUT:-$root/benchmarks/harbor/results/$stamp}"
mkdir -p "$output_root"

# Absolute path for the config so Harbor can mount/pass it regardless of cwd.
config_abs="$(cd "$(dirname "$YAK_AI_CONFIG_FILE")" && pwd)/$(basename "$YAK_AI_CONFIG_FILE")"

run_one() {
  local label="$1"
  local binary="$2"
  local attempt="$3"
  local job_name="yak-v1-${stamp}-${attempt}-${label}"
  local job_dir="$output_root/$job_name"

  echo "=== running $label attempt $attempt (binary=$binary job=$job_name) ==="

  # Per-run isolation: a single failed harbor run must not abort the matrix.
  # (Script head uses `set -uo pipefail` without -e, so we capture rc directly.)
  harbor run \
    -p "$dataset" \
    --agent-import-path "$agent_import" \
    -m "${YAK_AI_SERVICE}/${YAK_AI_MODEL}" \
    --agent-env "YAK_BINARY_PATH=$binary" \
    --agent-env "YAK_AGENT_VERSION=$label" \
    --agent-env "YAK_AI_SERVICE=$YAK_AI_SERVICE" \
    --agent-env "YAK_AI_MODEL=$YAK_AI_MODEL" \
    --agent-env "YAK_AI_CONFIG_FILE=$config_abs" \
    --force-build \
    --n-concurrent 1 \
    -k 1 \
    --job-name "$job_name" \
    -o "$output_root" \
    -y
  local rc=$?

  if [[ $rc -ne 0 ]]; then
    echo "FAILED:$job_name (exit $rc)" >>"$output_root/${label}-jobs.txt"
    err "$label attempt $attempt FAILED (exit $rc); continuing"
    return 1
  fi
  printf '%s\n' "$job_name" >>"$output_root/${label}-jobs.txt"
  return 0
}

# --- Interleaved base/candidate runs ----------------------------------------
for attempt in $(seq 1 "$attempts"); do
  if (( attempt % 2 == 1 )); then
    run_one base "$YAK_BASE_BINARY" "$attempt" || true
    run_one candidate "$YAK_CANDIDATE_BINARY" "$attempt" || true
  else
    run_one candidate "$YAK_CANDIDATE_BINARY" "$attempt" || true
    run_one base "$YAK_BASE_BINARY" "$attempt" || true
  fi
done

# --- Convert trial results to JSONL and compare -----------------------------
echo "=== converting results to JSONL ==="
converter="$root/benchmarks/harbor/scripts/harbor_results_to_jsonl.py"
base_jsonl="$output_root/base.jsonl"
candidate_jsonl="$output_root/candidate.jsonl"

# Collect job dirs listed in each jobs.txt (strip FAILED: markers). Each line
# is a bare job-name; resolve it to an absolute job directory under output_root.
base_job_dirs=()
while IFS= read -r name; do
  [[ -z "$name" ]] && continue
  base_job_dirs+=("$output_root/$name")
done < <(grep -v '^FAILED:' "$output_root/base-jobs.txt" 2>/dev/null || true)
candidate_job_dirs=()
while IFS= read -r name; do
  [[ -z "$name" ]] && continue
  candidate_job_dirs+=("$output_root/$name")
done < <(grep -v '^FAILED:' "$output_root/candidate-jobs.txt" 2>/dev/null || true)

if [[ ${#base_job_dirs[@]} -gt 0 ]]; then
  python3 "$converter" "${base_job_dirs[@]}" --label base --output "$base_jsonl"
fi
if [[ ${#candidate_job_dirs[@]} -gt 0 ]]; then
  python3 "$converter" "${candidate_job_dirs[@]}" --label candidate --output "$candidate_jsonl"
fi

if [[ -s "$base_jsonl" && -s "$candidate_jsonl" ]]; then
  echo "=== comparison ==="
  python3 "$root/benchmarks/harbor/scripts/compare_results.py" \
    "$base_jsonl" "$candidate_jsonl" --output "$output_root/verdict.json" || true
  echo "verdict written to $output_root/verdict.json"
else
  err "one or both JSONL files are empty; skipping comparison"
  err "  base: $base_jsonl"
  err "  candidate: $candidate_jsonl"
fi

echo "done. results under $output_root"
