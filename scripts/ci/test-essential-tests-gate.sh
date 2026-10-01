#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
workflow="${1:-$repo_root/.github/workflows/essential-tests.yml}"
test_dir="$(mktemp -d)"
trap 'rm -rf "$test_dir"' EXIT

# Exercise the actual inline gate, including its workflow environment bindings.
# This avoids a second implementation that can drift away from the CI decision.
awk '
  /^  essential-tests-gate:/ { gate=1; next }
  gate && /^  [[:alnum:]_-]+:/ { exit }
  gate { print }
' "$workflow" > "$test_dir/gate.yml"
awk '
  /^        run: \|$/ { script=1; next }
  script && /^          / { print substr($0, 11); next }
  script && /^[[:space:]]*$/ { print; next }
  script { exit }
' "$test_dir/gate.yml" > "$test_dir/gate.sh"
test -s "$test_dir/gate.sh"
bash -n "$test_dir/gate.sh"

while IFS= read -r binding; do
  if ! grep -Fxq "          $binding" "$test_dir/gate.yml"; then
    echo "Missing gate environment binding: $binding" >&2
    exit 1
  fi
done <<'BINDINGS'
PRECHECK_RESULT: ${{ needs.check-wip.result }}
SHOULD_RUN_TESTS: ${{ needs.check-wip.outputs.should_run_tests }}
TEST_SCOPE: ${{ needs.check-wip.outputs.test_scope }}
SCANNODE_RESULT: ${{ needs.scannode-focused.result }}
SLIM_RESULT: ${{ needs.slim-gate.result }}
RESOURCES_RESULT: ${{ needs.embedded-resources.result }}
IPC_RESULT: ${{ needs.ipc-smoke.result }}
STARTUP_RESULT: ${{ needs.prepared-startup.result }}
LOCAL_RESULT: ${{ needs.test-local.result }}
YAKGRPC_RESULT: ${{ needs.test-prepared-yakgrpc.result }}
COREPLUGIN_RESULT: ${{ needs.test-prepared-coreplugin.result }}
BINDINGS

case_count=0
check_gate() {
  local expected="$1" label="$2" rc=0
  shift 2
  env -i PATH="$PATH" \
    PRECHECK_RESULT=success SHOULD_RUN_TESTS=true TEST_SCOPE=full \
    SCANNODE_RESULT=success SLIM_RESULT=success RESOURCES_RESULT=success \
    IPC_RESULT=success STARTUP_RESULT=success LOCAL_RESULT=success \
    YAKGRPC_RESULT=success COREPLUGIN_RESULT=success \
    "$@" bash -e "$test_dir/gate.sh" > "$test_dir/output" 2>&1 || rc=$?
  if [[ ( "$expected" = pass && "$rc" -ne 0 ) || ( "$expected" = fail && "$rc" -eq 0 ) ]]; then
    echo "Gate regression: $label expected $expected, got exit $rc" >&2
    cat "$test_dir/output" >&2
    exit 1
  fi
  case_count=$((case_count + 1))
}

# Missing output after cancellation must never look like an intentional skip.
# A successful pre-check may skip only with an explicit false run flag and a
# known scope; either scope otherwise requires its selected jobs to succeed.
for precheck in success failure cancelled skipped ''; do
  for flag in true false '' invalid; do
    for scope in full scannode '' unknown; do
      expected=fail
      if [[ "$precheck" = success && ( "$flag" = true || "$flag" = false ) && ( "$scope" = full || "$scope" = scannode ) ]]; then
        expected=pass
      fi
      check_gate "$expected" "precheck=$precheck flag=$flag scope=$scope" \
        "PRECHECK_RESULT=$precheck" "SHOULD_RUN_TESTS=$flag" "TEST_SCOPE=$scope"
    done
  done
done

for result in failure cancelled skipped queued in_progress ''; do
  for job in RESOURCES SLIM IPC STARTUP LOCAL YAKGRPC COREPLUGIN; do
    check_gate fail "full scope: $job=$result" "${job}_RESULT=$result"
  done
  check_gate fail "ScanNode scope: result=$result" TEST_SCOPE=scannode "SCANNODE_RESULT=$result"
done

check_gate pass 'ScanNode scope ignores unselected full suites' TEST_SCOPE=scannode \
  RESOURCES_RESULT=skipped SLIM_RESULT=skipped IPC_RESULT=skipped STARTUP_RESULT=skipped \
  LOCAL_RESULT=skipped YAKGRPC_RESULT=skipped COREPLUGIN_RESULT=skipped
check_gate pass 'full scope ignores unselected ScanNode suite' SCANNODE_RESULT=skipped
check_gate pass 'cached full scope explicitly skips suites' SHOULD_RUN_TESTS=false \
  SCANNODE_RESULT=skipped RESOURCES_RESULT=skipped SLIM_RESULT=skipped IPC_RESULT=skipped \
  STARTUP_RESULT=skipped LOCAL_RESULT=skipped YAKGRPC_RESULT=skipped COREPLUGIN_RESULT=skipped
check_gate pass 'cached ScanNode scope explicitly skips suites' SHOULD_RUN_TESTS=false \
  TEST_SCOPE=scannode SCANNODE_RESULT=skipped

echo "Essential Tests Gate: $case_count regression cases passed."
