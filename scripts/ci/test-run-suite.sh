#!/usr/bin/env bash
set -euo pipefail

YAK_BINARY_PATH="${YAK_BINARY_PATH:-}"
TEST_BIN_DIR="${TEST_BIN_DIR:-}"
TEST_CONFIG="${TEST_CONFIG:-}"
SUITE_NAME="${SUITE_NAME:-suite}"
SUITE_SYNC_RULE="${SUITE_SYNC_RULE:-0}"
TEST_TIMEOUT="${TEST_TIMEOUT:-2m}"
TEST_VERBOSE="${TEST_VERBOSE:-1}"
SKIP_SYNC_EMBED_RULE_IN_GITHUB="${SKIP_SYNC_EMBED_RULE_IN_GITHUB:-true}"
TEST_LOG_DIR="${TEST_LOG_DIR:-$TEST_BIN_DIR}"

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

trap stop_grpc EXIT

suite_log="${TEST_LOG_DIR}/suite.log"
grpc_log="${TEST_LOG_DIR}/grpc_suite.log"
grpc_ready=0

mkdir -p "$TEST_LOG_DIR"
rm -f "$TEST_LOG_DIR"/test_*.run.log "$grpc_log" "$suite_log"

echo "=== Running suite: ${SUITE_NAME} ==="

nohup env SKIP_SYNC_EMBED_RULE_IN_GITHUB="$SKIP_SYNC_EMBED_RULE_IN_GITHUB" "$YAK_BINARY_PATH" grpc >"$grpc_log" 2>&1 < /dev/null &
grpc_pid=$!

for _ in {1..60}; do
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
  echo "GRPC server failed to start" | tee -a "$suite_log"
  cat "$grpc_log" | tee -a "$suite_log"
  exit 1
fi

if [[ "$SUITE_SYNC_RULE" == "1" ]]; then
  "$YAK_BINARY_PATH" sync-rule 2>&1 | tee -a "$suite_log"
fi

export TEST_LOG_DIR
./scripts/ci/test-run.sh 2>&1 | tee -a "$suite_log"
