#!/usr/bin/env bash
set -euo pipefail

YAK_BINARY_PATH="${YAK_BINARY_PATH:-}"
TEST_BIN_DIR="${TEST_BIN_DIR:-}"
TEST_CONFIG="${TEST_CONFIG:-}"
SUITE_NAME="${SUITE_NAME:-suite}"
SUITE_RETRIES="${SUITE_RETRIES:-1}"
SUITE_SYNC_RULE="${SUITE_SYNC_RULE:-0}"
TEST_TIMEOUT="${TEST_TIMEOUT:-2m}"
TEST_VERBOSE="${TEST_VERBOSE:-1}"
SKIP_SYNC_EMBED_RULE_IN_GITHUB="${SKIP_SYNC_EMBED_RULE_IN_GITHUB:-true}"

if [[ -z "$YAK_BINARY_PATH" || -z "$TEST_BIN_DIR" || -z "$TEST_CONFIG" ]]; then
  echo "ERROR: YAK_BINARY_PATH, TEST_BIN_DIR, and TEST_CONFIG must be set"
  exit 1
fi

if [[ ! -x "$YAK_BINARY_PATH" ]]; then
  echo "ERROR: YAK binary is missing or not executable: $YAK_BINARY_PATH"
  exit 1
fi

if [[ ! -f "$TEST_CONFIG" ]]; then
  echo "ERROR: TEST_CONFIG file not found: $TEST_CONFIG"
  exit 1
fi

stop_grpc() {
  if [[ -n "${grpc_pid:-}" ]]; then
    kill "$grpc_pid" 2>/dev/null || true
    wait "$grpc_pid" 2>/dev/null || true
    grpc_pid=""
  fi
}

snapshot_attempt_logs() {
  local attempt="$1"

  for path in /tmp/test_*.run.log; do
    [[ -f "$path" ]] || continue
    cp "$path" "/tmp/attempt${attempt}_$(basename "$path")"
  done
}

trap stop_grpc EXIT

attempt=1
max_attempts=$((SUITE_RETRIES + 1))

while [[ "$attempt" -le "$max_attempts" ]]; do
  grpc_log="/tmp/grpc_attempt_${attempt}.log"
  suite_log="/tmp/suite_attempt_${attempt}.log"
  grpc_ready=0

  rm -f /tmp/test_*.run.log "$grpc_log" "$suite_log"

  echo "=== Suite attempt ${attempt}/${max_attempts}: ${SUITE_NAME} ==="

  nohup env SKIP_SYNC_EMBED_RULE_IN_GITHUB="$SKIP_SYNC_EMBED_RULE_IN_GITHUB" "$YAK_BINARY_PATH" grpc >"$grpc_log" 2>&1 < /dev/null &
  grpc_pid=$!

  for i in {1..60}; do
    if ! kill -0 "$grpc_pid" 2>/dev/null; then
      break
    fi
    if nc -z localhost 8087 && grep -q "yak grpc ok" "$grpc_log" 2>/dev/null; then
      grpc_ready=1
      break
    fi
    sleep 1
  done

  if [[ "$grpc_ready" -ne 1 ]]; then
    echo "GRPC server failed to start on attempt ${attempt}" | tee -a "$suite_log"
    cat "$grpc_log" | tee -a "$suite_log"
    stop_grpc
    if [[ "$attempt" -lt "$max_attempts" ]]; then
      echo "Retrying suite without recompiling..." | tee -a "$suite_log"
      attempt=$((attempt + 1))
      sleep 3
      continue
    fi
    exit 1
  fi

  if [[ "$SUITE_SYNC_RULE" == "1" ]]; then
    if ! "$YAK_BINARY_PATH" sync-rule 2>&1 | tee -a "$suite_log"; then
      stop_grpc
      if [[ "$attempt" -lt "$max_attempts" ]]; then
        echo "sync-rule failed on attempt ${attempt}, retrying without recompiling..." | tee -a "$suite_log"
        attempt=$((attempt + 1))
        sleep 3
        continue
      fi
      exit 1
    fi
  fi

  set +e
  ./scripts/ci/run-tests.sh 2>&1 | tee -a "$suite_log"
  run_code=${PIPESTATUS[0]}
  set -e

  snapshot_attempt_logs "$attempt"
  stop_grpc

  if [[ "$run_code" -eq 0 ]]; then
    echo "Suite passed on attempt ${attempt}/${max_attempts}"
    exit 0
  fi

  if [[ "$attempt" -lt "$max_attempts" ]]; then
    echo "Suite failed on attempt ${attempt}/${max_attempts}, retrying without recompiling..." | tee -a "$suite_log"
    attempt=$((attempt + 1))
    sleep 3
    continue
  fi

  echo "Suite failed after ${max_attempts} attempt(s)" | tee -a "$suite_log"
  exit "$run_code"
done
