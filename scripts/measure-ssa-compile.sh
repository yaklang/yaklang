#!/usr/bin/env bash
# Run the existing yak ssa-compile CLI with heap/profile measurement defaults.
#
# Typical:
#   scripts/measure-ssa-compile.sh ~/Target/spring-demo -l java
#
# Useful env overrides:
#   YAK_BIN=/path/to/yak                         use a built yak binary instead of go run
#   OUT_DIR=build/ssa-measure/run1               where logs and pprof summaries are written
#   YAKIT_HOME="$PWD/.db"                        worktree-local SSA database
#   YAK_DIAGNOSTICS_LOG_LEVEL=trace              diagnostics trace level
#   YAK_SSA_DIAGNOSTICS=1                        enable nested SSA diagnostics TRACE
#   YAK_SSA_HEAP_LOG=1                           print retained heap by SSA compile phase; set off/none/0 to disable
#   YAK_SSA_HEAP_PROFILE_DIR=<dir>               write phase heap profiles; set off/none/0 to disable
#   YAK_SSA_COMPILE_UNIT_LOG=1                   print compile unit graph/order details
#   YAK_SSA_COMPILE_UNIT_LOG_DIR=<dir>           write compile unit graph/order JSON
#   YAK_MEASURE_FILE_PERF_LOG=0                  disable --file-perf-log for large benchmark runs
#   YAK_SSA_AST_IN_FLIGHT_FILES=32               cap source files queued before AST parse
#   YAK_SSA_AST_BUILD_WINDOW_FILES=2             cap parsed ASTs awaiting pass1 build cleanup
#   YAK_SSA_ORDERED_AST_MAX_FILES=1024           downgrade ordered AST mode above this count
#   YAK_SSA_LARGE_PROJECT_CONCURRENCY=2          cap large-project AST parse concurrency
#   YAK_ANTLR_CACHE_RESET_FILES=25               reset ANTLR runtime caches by file count
#   YAK_ANTLR_CACHE_RESET_BYTES=8MiB             reset ANTLR runtime caches by parsed bytes
set -euo pipefail

usage() {
  cat >&2 <<'USAGE'
usage: scripts/measure-ssa-compile.sh <target-path> [ssa-compile flags...]

This is a measurement wrapper around the existing yak ssa-compile command.
Pass normal ssa-compile flags after the target path, for example:
  scripts/measure-ssa-compile.sh ~/Target/project -l java --file-perf-log
USAGE
}

if [[ "${1:-}" == "-h" || "${1:-}" == "--help" ]]; then
  usage
  exit 0
fi

TARGET="${TARGET:-}"
if [[ $# -gt 0 && "${1:0:1}" != "-" ]]; then
  TARGET="$1"
  shift
fi
if [[ -z "$TARGET" ]]; then
  usage
  exit 2
fi

RUN_LABEL="${RUN_LABEL:-$(basename "$TARGET")}"
SAFE_LABEL="$(printf '%s' "$RUN_LABEL" | tr -c 'A-Za-z0-9_.-' '-')"
STAMP="${YAK_MEASURE_STAMP:-$(date +%Y%m%d-%H%M%S)}"
OUT_DIR="${OUT_DIR:-build/ssa-measure/${SAFE_LABEL}-${STAMP}}"
LOG_FILE="${LOG_FILE:-$OUT_DIR/ssa-compile.log}"
PPROF_FILE="${YAK_SSA_MONITOR_PPROF:-$OUT_DIR/heap-monitor.pprof}"

export YAKIT_HOME="${YAKIT_HOME:-$PWD/.db}"
export YAK_DIAGNOSTICS_LOG_LEVEL="${YAK_DIAGNOSTICS_LOG_LEVEL:-trace}"
export YAK_SSA_DIAGNOSTICS="${YAK_SSA_DIAGNOSTICS:-1}"
case "${YAK_SSA_HEAP_LOG:-}" in
  0|false|FALSE|off|OFF|none|NONE)
    unset YAK_SSA_HEAP_LOG
    ;;
  "")
    export YAK_SSA_HEAP_LOG=1
    ;;
  *)
    export YAK_SSA_HEAP_LOG
    ;;
esac
case "${YAK_SSA_HEAP_PROFILE_DIR:-}" in
  0|false|FALSE|off|OFF|none|NONE)
    unset YAK_SSA_HEAP_PROFILE_DIR
    ;;
  "")
    export YAK_SSA_HEAP_PROFILE_DIR="$OUT_DIR/heap-profiles"
    ;;
  *)
    export YAK_SSA_HEAP_PROFILE_DIR
    ;;
esac
export YAK_SSA_COMPILE_UNIT_LOG="${YAK_SSA_COMPILE_UNIT_LOG:-0}"
if [[ "$YAK_SSA_COMPILE_UNIT_LOG" != "0" && -z "${YAK_SSA_COMPILE_UNIT_LOG_DIR:-}" ]]; then
  export YAK_SSA_COMPILE_UNIT_LOG_DIR="$OUT_DIR/compile-units"
fi
export YAK_SSA_AST_IN_FLIGHT_FILES="${YAK_SSA_AST_IN_FLIGHT_FILES:-32}"
export YAK_SSA_AST_BUILD_WINDOW_FILES="${YAK_SSA_AST_BUILD_WINDOW_FILES:-2}"
export YAK_SSA_ORDERED_AST_MAX_FILES="${YAK_SSA_ORDERED_AST_MAX_FILES:-1024}"
export YAK_SSA_LARGE_PROJECT_CONCURRENCY="${YAK_SSA_LARGE_PROJECT_CONCURRENCY:-2}"
export YAK_ANTLR_CACHE_RESET_FILES="${YAK_ANTLR_CACHE_RESET_FILES:-25}"
export YAK_ANTLR_CACHE_RESET_BYTES="${YAK_ANTLR_CACHE_RESET_BYTES:-8MiB}"

LANGUAGE="${LANGUAGE:-java}"
PROGRAM="${PROGRAM:-ssa-measure-${SAFE_LABEL}-${STAMP}}"
LOG_LEVEL="${LOG_LEVEL:-info}"

mkdir -p "$OUT_DIR" "$YAKIT_HOME"
if [[ -n "${YAK_SSA_HEAP_PROFILE_DIR:-}" ]]; then
  mkdir -p "$YAK_SSA_HEAP_PROFILE_DIR"
fi

if [[ -n "${YAK_BIN:-}" ]]; then
  YAK_CMD=("$YAK_BIN")
else
  YAK_CMD=(go run ./common/yak/cmd)
fi

CLI_ARGS=(
  ssa-compile
  --target "$TARGET"
  --program "$PROGRAM"
  --language "$LANGUAGE"
  --re-compile
  --pprof "$PPROF_FILE"
  --log "$LOG_LEVEL"
)
if [[ "${YAK_MEASURE_FILE_PERF_LOG:-1}" != "0" ]]; then
  CLI_ARGS+=(--file-perf-log)
fi
if [[ "$YAK_SSA_DIAGNOSTICS" != "0" ]]; then
  CLI_ARGS+=(--diagnostics)
fi
CLI_ARGS+=("$@")

{
  echo "[measure] target=$TARGET"
  echo "[measure] program=$PROGRAM language=$LANGUAGE out_dir=$OUT_DIR"
  echo "[measure] heap_log=${YAK_SSA_HEAP_LOG:-disabled} heap_profiles=${YAK_SSA_HEAP_PROFILE_DIR:-disabled} monitor_pprof=$PPROF_FILE"
  echo "[measure] compile_unit_log=$YAK_SSA_COMPILE_UNIT_LOG compile_unit_log_dir=${YAK_SSA_COMPILE_UNIT_LOG_DIR:-}"
  echo "[measure] ast_in_flight=$YAK_SSA_AST_IN_FLIGHT_FILES ast_build_window=$YAK_SSA_AST_BUILD_WINDOW_FILES ordered_ast_limit=$YAK_SSA_ORDERED_AST_MAX_FILES large_project_concurrency=$YAK_SSA_LARGE_PROJECT_CONCURRENCY"
  echo "[measure] antlr_reset_files=$YAK_ANTLR_CACHE_RESET_FILES antlr_reset_bytes=$YAK_ANTLR_CACHE_RESET_BYTES"
  echo "[measure] yakit_home=$YAKIT_HOME diagnostics=$YAK_DIAGNOSTICS_LOG_LEVEL ssa_diagnostics=$YAK_SSA_DIAGNOSTICS"
} | tee "$LOG_FILE"

if command -v /usr/bin/time >/dev/null 2>&1 && [[ "${YAK_MEASURE_TIME:-1}" != "0" ]]; then
  /usr/bin/time -v "${YAK_CMD[@]}" "${CLI_ARGS[@]}" 2>&1 | tee -a "$LOG_FILE"
else
  "${YAK_CMD[@]}" "${CLI_ARGS[@]}" 2>&1 | tee -a "$LOG_FILE"
fi

if [[ "${YAK_MEASURE_ANALYZE_HEAP:-1}" != "0" && -n "${YAK_SSA_HEAP_PROFILE_DIR:-}" ]]; then
  scripts/analyze-heap-profiles.sh "$YAK_SSA_HEAP_PROFILE_DIR" "$OUT_DIR/pprof-top" | tee -a "$LOG_FILE"
fi

echo "[measure] wrote log to $LOG_FILE"
