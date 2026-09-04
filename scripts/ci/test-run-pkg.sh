#!/usr/bin/env bash
# Run compiled-free Go tests straight from package patterns in TEST_CONFIG.
#
# Why this exists: the compile-then-run pipeline (test-compile.sh + test-run.sh)
# only pays off when one compiled binary is reused by several CI jobs, e.g. the
# yakgrpc / coreplugin suites. For a package that exactly one job exercises,
# splitting compile from run costs a whole extra pass AND aborts the job on the
# first build error, so zero test results come back. Here `go test` builds and
# runs per package: a broken package fails only itself, and the shared Go build
# cache still absorbs the compilation cost.
set -uo pipefail

TEST_CONFIG="${TEST_CONFIG:-}"
TEST_TIMEOUT="${TEST_TIMEOUT:-2m}"
TEST_VERBOSE="${TEST_VERBOSE:-1}"
TEST_LOG_DIR="${TEST_LOG_DIR:-${TEST_BIN_DIR:-./test_binaries}}"
PACKAGE_PARALLEL="${PACKAGE_PARALLEL:-}"   # extra -p for go test package loading

if [[ -z "$TEST_CONFIG" || ! -f "$TEST_CONFIG" ]]; then
  echo "ERROR: TEST_CONFIG must point to an existing file"
  exit 1
fi

mkdir -p "$TEST_LOG_DIR"

list_test_pkgs() {
  # Fail loudly: an empty or erroring package listing must never look like
  # "no tests to run, therefore all passed".
  local pattern="$1" out
  if ! out=$(go list -f '{{if or .TestGoFiles .XTestGoFiles}}{{.ImportPath}}{{end}}' "$pattern" 2>&1); then
    # Diagnostics go to stderr only: stdout of this function is the package list.
    printf '::error::go list failed for %s\n' "$pattern" >&2
    printf '%s\n' "$out" >&2
    return 1
  fi
  printf '%s\n' "$out" | grep -v '^$' || true
}

needs_race() {
  local pkg="$1" pattern race normalized pnorm
  while IFS='|' read -r pattern race; do
    [[ "$race" == "true" ]] || continue
    normalized="${pattern#./}"; normalized="${normalized%/...}"; normalized="${normalized%/.}"
    pnorm="${pkg#./}"
    if [[ "$pattern" == *"/..." ]]; then
      [[ "$pnorm" == "$normalized"* ]] && return 0
    elif [[ "$pnorm" == "$normalized" ]]; then
      return 0
    fi
  done < <(jq -r '.[] | "\(.package)|\(.race // false)"' "$TEST_CONFIG")
  return 1
}

run_package() {
  local pkg="$1" timeout="$2" run_pat="$3" skip_pat="$4" parallel="$5"
  local retry="$6" retry_delay="$7"
  local safe="$(printf '%s' "$pkg" | sed 's|^\./||; s|/|_|g; s|[.*]|_|g')"
  local log="$TEST_LOG_DIR/pkg_${safe}.run.log"
  local max_retries="${retry:-0}" delay="${retry_delay:-5}"
  local attempt=0 rc=1

  local args=("-count=1" "-timeout" "$timeout")
  [[ "$TEST_VERBOSE" = "1" ]] && args+=("-v")
  [[ -n "$run_pat" ]] && args+=("-run" "$run_pat")
  [[ -n "$skip_pat" ]] && args+=("-skip" "$skip_pat")
  [[ -n "$parallel" ]] && args+=("-parallel" "$parallel")
  [[ -n "$PACKAGE_PARALLEL" ]] && args+=("-p" "$PACKAGE_PARALLEL")
  needs_race "$pkg" && args+=("-race")

  while (( attempt <= max_retries )); do
    if (( attempt > 0 )); then
      echo " retry ($((attempt + 1))/$((max_retries + 1))): $pkg"
      sleep "$delay"
    fi
    echo "===== go test $pkg ${args[*]} ====="
    if go test "$pkg" "${args[@]}" 2>&1 | tee -a "$log"; then
      echo "PASS: $pkg"
      rc=0
      break
    fi
    echo "FAIL: $pkg (attempt $((attempt + 1))/$((max_retries + 1)))" | tee -a "$log"
    attempt=$((attempt + 1))
  done
  return $rc
}

rc=0
total_run=0
count=$(jq 'length' "$TEST_CONFIG")
for ((idx = 0; idx < count; idx++)); do
  pattern=$(jq -r ".[$idx].package" "$TEST_CONFIG")
  timeout=$(jq -r ".[$idx].timeout // empty" "$TEST_CONFIG")
  run_pat=$(jq -r ".[$idx].run // empty" "$TEST_CONFIG")
  skip_pat=$(jq -r ".[$idx].skip // empty" "$TEST_CONFIG")
  parallel=$(jq -r ".[$idx].parallel // empty" "$TEST_CONFIG")
  retry=$(jq -r ".[$idx].retry // empty" "$TEST_CONFIG")
  retry_delay=$(jq -r ".[$idx].retry_delay // empty" "$TEST_CONFIG")
  excludes=$(jq -r ".[$idx].exclude_packages[]? // empty" "$TEST_CONFIG")
  [[ -z "$timeout" ]] && timeout="$TEST_TIMEOUT"

  echo "----------------------------------------"
  echo "config #$((idx + 1)): $pattern (timeout $timeout)"

  pkglist="$TEST_LOG_DIR/.pkglist.$$"
  if ! list_test_pkgs "$pattern" > "$pkglist"; then
    rm -f "$pkglist"
    rc=1
    continue
  fi

  while IFS= read -r pkg; do
    [[ -z "$pkg" ]] && continue
    rel="./${pkg#github.com/yaklang/yaklang/}"
    skip_this=0
    while IFS= read -r ex; do
      [[ -z "$ex" ]] && continue
      prefix="${ex%/\.\.\.}"
      if [[ "$rel" == "$prefix" || "$rel" == "$prefix"/* ]]; then
        skip_this=1
        break
      fi
    done <<< "$excludes"
    (( skip_this )) && continue

    total_run=$((total_run + 1))
    run_package "$rel" "$timeout" "$run_pat" "$skip_pat" "$parallel" "$retry" "$retry_delay" || rc=1
  done < "$pkglist"
  rm -f "$pkglist"
done

if (( total_run == 0 )); then
  echo "::error::no test packages were discovered from $TEST_CONFIG - refusing to report success"
  exit 1
fi

echo ""
echo "=== go test summary ($total_run package runs) ==="
if (( rc == 0 )); then
  echo "Result: ALL PASSED"
else
  echo "Result: SOME FAILED"
  grep -alE '^FAIL: |--- FAIL: |test timed out|^panic: |^FAIL\[' "$TEST_LOG_DIR"/pkg_*.run.log 2>/dev/null | while IFS= read -r f; do
    echo "  - $(basename "$f" .run.log)"
    grep -aE '^--- FAIL: ' "$f" | head -5 | sed 's/^/      /'
  done
fi
exit $rc
