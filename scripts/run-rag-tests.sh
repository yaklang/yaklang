#!/usr/bin/env bash
set -euo pipefail

# Run AI infra/RAG/Knowledge tests concurrently across packages while keeping
# each package single-threaded (-parallel=1 as configured).

CONFIG='[
  {"package": "./common/ai/aid/...", "timeout": "12m", "parallel": 1},
  {"package": "./common/ai/tests/...", "timeout": "60s", "parallel": 1},
  {"package": "./common/ai/rag/pq/...", "timeout": "60s", "parallel": 1},
  {"package": "./common/ai/rag/hnsw/...", "timeout": "60s", "parallel": 1},
  {"package": "./common/ai/aispec/...", "timeout": "60s", "parallel": 1},
  {"package": "./common/aireducer/...", "timeout": "60s", "parallel": 1},
  {"package": "./common/aiforge/aibp", "timeout": "40s", "parallel": 1, "run": "^(TestBuildForgeFromYak|TestNewForgeExecutor)"},
  {"package": "./common/aiforge", "timeout": "3m", "parallel": 1},
  {"package": "./common/ai/rag/entityrepos/...", "timeout": "60s", "parallel": 1},
  {"package": "./common/ai/rag/vectorstore/...", "timeout": "1m", "run": "TestMUSTPASS", "parallel": 1},
  {"package": "./common/ai/rag", "timeout": "1m", "run": "TestMUSTPASS", "parallel": 1},
  {"package": "./common/ai/rag/plugins_rag/...", "timeout": "1m", "run": "TestMUSTPASS", "parallel": 1},
  {"package": "./common/ai/rag/ragtests/...", "timeout": "1m", "run": "TestMUSTPASS"}
]'

if ! command -v jq >/dev/null 2>&1; then
  echo "jq is required to run this script" >&2
  exit 1
fi

JOBS=${JOBS:-$(getconf _NPROCESSORS_ONLN 2>/dev/null || sysctl -n hw.ncpu 2>/dev/null || echo 4)}
LOG_DIR=${LOG_DIR:-$(mktemp -d "${TMPDIR:-/tmp}/run-rag-tests-XXXXXX")}
mkdir -p "$LOG_DIR"

echo "Running AI infra/RAG/Knowledge tests with up to $JOBS concurrent packages..."
echo "$CONFIG" | jq -r '.[] | "* " + .package'
echo
echo "Logs will be written to: $LOG_DIR"
echo

RED='\033[31m'
RESET='\033[0m'

run_test() {
  local pkg=$1 timeout=$2 parallel=$3 run=$4 logfile=$5
  local args=("-timeout=$timeout" "-parallel=$parallel")
  [[ -n "$run" ]] && args+=("-run" "$run")
  args+=("$pkg")
  echo "==> [$(date +%H:%M:%S)] START $pkg (log: $logfile)"
  local status=0
  if ! GITHUB_ACTIONS=true go test "${args[@]}" >"$logfile" 2>&1; then
    status=$?
  fi

  if grep -E '^(FAIL|panic:|fatal error:|--- FAIL:)' "$logfile" >/dev/null 2>&1; then
    echo -e "==> [$(date +%H:%M:%S)] RESULT $pkg: ${RED}failure signature detected${RESET} (log: $logfile)"
    grep -E '^(FAIL|panic:|fatal error:|--- FAIL:)' "$logfile" | head -n 10 | sed "s/^/${RED}/;s/$/${RESET}/"
  else
    echo "==> [$(date +%H:%M:%S)] RESULT $pkg: no failure signature (log: $logfile)"
  fi

  if [[ $status -eq 0 ]]; then
    echo "==> [$(date +%H:%M:%S)] PASS  $pkg (log: $logfile)"
  else
    echo "==> [$(date +%H:%M:%S)] FAIL  $pkg (log: $logfile)"
    return $status
  fi
}

if [[ "$JOBS" -lt 1 ]]; then
  echo "JOBS must be at least 1" >&2
  exit 1
fi

declare -a pids=()

while IFS=$'\t' read -r pkg timeout parallel run; do
  logfile="$LOG_DIR/$(echo "$pkg" | tr '/. ' '__').log"
  run_test "$pkg" "$timeout" "$parallel" "$run" "$logfile" &
  pids+=("$!")
  if ((${#pids[@]} >= JOBS)); then
    wait "${pids[0]}"
    pids=("${pids[@]:1}")
  fi
done < <(echo "$CONFIG" | jq -rc '.[] | [ .package, (.timeout // "60s"), (.parallel // 1), (.run // "") ] | @tsv')

for pid in "${pids[@]}"; do
  wait "$pid"
done
