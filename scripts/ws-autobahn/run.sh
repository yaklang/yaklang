#!/usr/bin/env bash

set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
PROFILE="$(printenv AUTOBAHN_PROFILE 2>/dev/null || true)"
PORT="$(printenv AUTOBAHN_PORT 2>/dev/null || true)"
IMAGE="$(printenv AUTOBAHN_IMAGE 2>/dev/null || true)"
MODE="$(printenv AUTOBAHN_MODE 2>/dev/null || true)"
REPORT_DIR="$(printenv AUTOBAHN_REPORT_DIR 2>/dev/null || true)"
CASE_TIMEOUT="$(printenv AUTOBAHN_CASE_TIMEOUT 2>/dev/null || true)"
LOWHTTP_TEST_TIMEOUT="$(printenv AUTOBAHN_LOWHTTP_TEST_TIMEOUT 2>/dev/null || true)"
MITM_TEST_TIMEOUT="$(printenv AUTOBAHN_MITM_TEST_TIMEOUT 2>/dev/null || true)"
MITM_TEST_CLIENT="$(printenv AUTOBAHN_MITM_TEST_CLIENT 2>/dev/null || true)"
SUITE_TIMEOUT="$(printenv AUTOBAHN_SUITE_TIMEOUT 2>/dev/null || true)"
MITM_DISABLE_FLOW_STORAGE="$(printenv AUTOBAHN_MITM_DISABLE_FLOW_STORAGE 2>/dev/null || true)"

[[ -n "$PROFILE" ]] || PROFILE="smoke"
[[ -n "$PORT" ]] || PORT="9001"
[[ -n "$IMAGE" ]] || IMAGE="crossbario/autobahn-testsuite@sha256:519915fb568b04c9383f70a1c405ae3ff44ab9e35835b085239c258b6fac3074"
[[ -n "$MODE" ]] || MODE="all"
[[ -n "$REPORT_DIR" ]] || REPORT_DIR="$ROOT/reports/autobahn/$PROFILE"
if [[ "$PROFILE" == "compression" ]]; then
  [[ -n "$CASE_TIMEOUT" ]] || CASE_TIMEOUT="9m"
  [[ -n "$LOWHTTP_TEST_TIMEOUT" ]] || LOWHTTP_TEST_TIMEOUT="36h"
  [[ -n "$MITM_TEST_TIMEOUT" ]] || MITM_TEST_TIMEOUT="36h"
  [[ -n "$MITM_TEST_CLIENT" ]] || MITM_TEST_CLIENT="lowhttp"
  [[ -n "$SUITE_TIMEOUT" ]] || SUITE_TIMEOUT="36h"
  [[ -n "$MITM_DISABLE_FLOW_STORAGE" ]] || MITM_DISABLE_FLOW_STORAGE="true"
elif [[ "$PROFILE" == "compression-smoke" ]]; then
  [[ -n "$CASE_TIMEOUT" ]] || CASE_TIMEOUT="90s"
  [[ -n "$LOWHTTP_TEST_TIMEOUT" ]] || LOWHTTP_TEST_TIMEOUT="30m"
  [[ -n "$MITM_TEST_TIMEOUT" ]] || MITM_TEST_TIMEOUT="45m"
  [[ -n "$MITM_TEST_CLIENT" ]] || MITM_TEST_CLIENT="lowhttp"
  [[ -n "$SUITE_TIMEOUT" ]] || SUITE_TIMEOUT="45m"
  [[ -n "$MITM_DISABLE_FLOW_STORAGE" ]] || MITM_DISABLE_FLOW_STORAGE="false"
else
  [[ -n "$CASE_TIMEOUT" ]] || CASE_TIMEOUT="15s"
  [[ -n "$LOWHTTP_TEST_TIMEOUT" ]] || LOWHTTP_TEST_TIMEOUT="20m"
  [[ -n "$MITM_TEST_TIMEOUT" ]] || MITM_TEST_TIMEOUT="30m"
  [[ -n "$MITM_TEST_CLIENT" ]] || MITM_TEST_CLIENT="gorilla"
  [[ -n "$SUITE_TIMEOUT" ]] || SUITE_TIMEOUT="30m"
  [[ -n "$MITM_DISABLE_FLOW_STORAGE" ]] || MITM_DISABLE_FLOW_STORAGE="false"
fi

CONFIG_DIR="$ROOT/common/yakgrpc/testdata/autobahn"
CONFIG_FILE="$CONFIG_DIR/fuzzingserver-$PROFILE.json"
CONTAINER_NAME="yak-autobahn-$PROFILE-$$"
TEST_HOME="$(mktemp -d)"
RUNTIME_CONFIG_DIR="$TEST_HOME/config"

if [[ ! -f "$CONFIG_FILE" ]]; then
  echo "unknown Autobahn profile: $PROFILE" >&2
  exit 2
fi

# Autobahn validates the HTTP Host authority against the port in its URL
# configuration. Keep custom host ports usable by running the test server on
# the same internal port instead of publishing a different external port.
mkdir -p "$RUNTIME_CONFIG_DIR"
cp "$CONFIG_FILE" "$RUNTIME_CONFIG_DIR/config.json"
if [[ "$PORT" != "9001" ]]; then
  sed -i "s/:9001/:$PORT/g" "$RUNTIME_CONFIG_DIR/config.json"
fi

mkdir -p "$REPORT_DIR/clients"
rm -rf "$REPORT_DIR/clients/"*

cleanup() {
  docker rm -f "$CONTAINER_NAME" >/dev/null 2>&1 || true
  rm -rf "$TEST_HOME"
}
trap cleanup EXIT

docker run --detach --rm \
  --volume "$RUNTIME_CONFIG_DIR:/config:ro" \
  --volume "$REPORT_DIR:/reports" \
  --publish "127.0.0.1:$PORT:$PORT" \
  --name "$CONTAINER_NAME" \
  "$IMAGE" \
  wstest --mode fuzzingserver --spec "/config/config.json" >/dev/null

for _ in $(seq 1 60); do
  if (echo >/dev/tcp/127.0.0.1/"$PORT") >/dev/null 2>&1; then
    break
  fi
  if ! docker inspect "$CONTAINER_NAME" >/dev/null 2>&1; then
    docker logs "$CONTAINER_NAME" >&2 || true
    exit 1
  fi
  sleep 0.25
done

if ! (echo >/dev/tcp/127.0.0.1/"$PORT") >/dev/null 2>&1; then
  docker logs "$CONTAINER_NAME" >&2 || true
  echo "Autobahn fuzzing server did not become ready" >&2
  exit 1
fi

export AUTOBAHN_SERVER_HOSTPORT="127.0.0.1:$PORT"
export AUTOBAHN_CASE_TIMEOUT="$CASE_TIMEOUT"
export AUTOBAHN_MITM_TEST_CLIENT="$MITM_TEST_CLIENT"
export AUTOBAHN_SUITE_TIMEOUT="$SUITE_TIMEOUT"
export AUTOBAHN_MITM_DISABLE_FLOW_STORAGE="$MITM_DISABLE_FLOW_STORAGE"
export YAKIT_HOME="$TEST_HOME"

if [[ "$MODE" == "all" || "$MODE" == "yak-client" ]]; then
  go test "$ROOT/common/utils/lowhttp" \
    -run '^TestWebsocket_AutobahnClient$' \
    -count=1 \
    -timeout="$LOWHTTP_TEST_TIMEOUT" \
    -v
fi

if [[ "$MODE" == "all" || "$MODE" == "mitm" ]]; then
  go test "$ROOT/common/yakgrpc" \
    -run '^TestGRPCMUSTPASS_MITM_WebSocketAutobahnDifferential$' \
    -count=1 \
    -timeout="$MITM_TEST_TIMEOUT" \
    -v
fi

if [[ ! -f "$REPORT_DIR/clients/index.html" || ! -f "$REPORT_DIR/clients/index.json" ]]; then
  docker logs "$CONTAINER_NAME" >&2 || true
  echo "Autobahn report index was not generated" >&2
  exit 1
fi

go run "$ROOT/scripts/ws-autobahn/check_report.go" "$REPORT_DIR/clients/index.json"

echo "Autobahn report: $REPORT_DIR/clients/index.html"
