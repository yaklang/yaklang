#!/usr/bin/env bash
set -euo pipefail

YAK_BINARY_PATH="${YAK_BINARY_PATH:-}"
TEST_BIN_DIR="${TEST_BIN_DIR:-}"
TEST_CONFIG="${TEST_CONFIG:-}"
SUITE_NAME="${SUITE_NAME:-suite}"
SUITE_SYNC_RULE="${SUITE_SYNC_RULE:-0}"
# Pure static checks can run without starting the engine. Default stays strict.
SUITE_NEEDS_GRPC="${SUITE_NEEDS_GRPC:-1}"
TEST_TIMEOUT="${TEST_TIMEOUT:-2m}"
TEST_VERBOSE="${TEST_VERBOSE:-1}"
SKIP_SYNC_EMBED_RULE_IN_GITHUB="${SKIP_SYNC_EMBED_RULE_IN_GITHUB:-true}"
TEST_LOG_DIR="${TEST_LOG_DIR:-$TEST_BIN_DIR}"
SUITE_TIMEOUT="${SUITE_TIMEOUT:-50m}"
# How long to wait for the engine to bind its listener and emit the structured
# ready event. Cold runners rebuild the coreplugin DB and engine caches, which
# has been observed to take well over 90s under load, so the window is generous
# and the failure path reports *why* the wait ended instead of a generic message.
GRPC_READY_TIMEOUT="${GRPC_READY_TIMEOUT:-240}"

if [[ "$SUITE_NEEDS_GRPC" != "0" && "$SUITE_NEEDS_GRPC" != "1" ]]; then
  echo "ERROR: SUITE_NEEDS_GRPC must be 0 or 1"
  exit 1
fi
if [[ -z "$TEST_BIN_DIR" || -z "$TEST_CONFIG" ]]; then
  echo "ERROR: TEST_BIN_DIR and TEST_CONFIG must be set"
  exit 1
fi

if [[ ( "$SUITE_NEEDS_GRPC" == "1" || "$SUITE_SYNC_RULE" == "1" ) && ! -x "$YAK_BINARY_PATH" ]]; then
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

stop_children() {
  pkill -P $$ 2>/dev/null || true
}

terminate_suite() {
  echo "Suite interrupted, stopping child processes..." | tee -a "${suite_log:-/dev/stderr}"
  stop_children
  stop_grpc
  exit 143
}

trap terminate_suite INT TERM
trap stop_grpc EXIT

suite_log="${TEST_LOG_DIR}/suite.log"
grpc_log="${TEST_LOG_DIR}/grpc_suite.log"
grpc_ready=0

mkdir -p "$TEST_LOG_DIR"
rm -f "$TEST_LOG_DIR"/test_*.run.log "$grpc_log" "$suite_log"

echo "=== Running suite: ${SUITE_NAME} ==="
echo "Suite timeout: ${SUITE_TIMEOUT}"

if [[ "$SUITE_NEEDS_GRPC" == "1" ]]; then
  nohup env SKIP_SYNC_EMBED_RULE_IN_GITHUB="$SKIP_SYNC_EMBED_RULE_IN_GITHUB" "$YAK_BINARY_PATH" grpc >"$grpc_log" 2>&1 < /dev/null &
  grpc_pid=$!

  # Wait for the structured ready event, which the engine writes immediately after
  # the listener is bound. Polling only the port is not enough: a healthy engine
  # can hold the listener open while still initializing, and `grep` on a log that
  # is being written concurrently can transiently miss the marker line.
  grpc_exited=0
  waited=0
  for ((waited = 0; waited < GRPC_READY_TIMEOUT; waited++)); do
    if ! kill -0 "$grpc_pid" 2>/dev/null; then
      grpc_exited=1
      break
    fi
    if grep -q '^yak grpc ready {' "$grpc_log" 2>/dev/null; then
      grpc_ready=1
      break
    fi
    sleep 1
  done

  if [[ "$grpc_ready" -ne 1 ]]; then
    # Distinguish "engine reported a startup failure" from "engine never became
    # ready in time" so the log and the exit code point at the real cause.
    # `|| true` matters: grep exits 1 when the marker is absent, and under
    # `set -e`/pipefail that would abort before the diagnostic below is printed.
    grpc_failure_event="$(grep -a '^yak grpc failed ' "$grpc_log" 2>/dev/null | tail -1 || true)"
    if [[ -n "$grpc_failure_event" ]]; then
      echo "GRPC server failed to start: $grpc_failure_event" | tee -a "$suite_log"
    elif [[ "$grpc_exited" -eq 1 ]]; then
      echo "GRPC server exited before becoming ready (after ${waited}s)" | tee -a "$suite_log"
    else
      echo "GRPC server did not become ready within ${GRPC_READY_TIMEOUT}s" | tee -a "$suite_log"
    fi
    cat "$grpc_log" | tee -a "$suite_log"
    exit 1
  fi

  echo "GRPC ready after ${waited}s"
else
  echo "Suite uses in-process services: external gRPC engine startup is not required"
fi

if [[ "$SUITE_SYNC_RULE" == "1" ]]; then
  "$YAK_BINARY_PATH" sync-rule 2>&1 | tee -a "$suite_log"
fi

export TEST_LOG_DIR
set +e
TEST_RUNNER="${TEST_RUNNER:-./scripts/ci/test-run.sh}"
timeout --signal=TERM --kill-after=30s "$SUITE_TIMEOUT" "$TEST_RUNNER" 2>&1 | tee -a "$suite_log"
suite_rc=${PIPESTATUS[0]}
set -e

if [[ "$suite_rc" -eq 124 ]]; then
  echo "Suite timed out after ${SUITE_TIMEOUT}" | tee -a "$suite_log"
fi

exit "$suite_rc"
