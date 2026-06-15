#!/usr/bin/env bash
# Run yak ssa-compile or code-scan with heap/phase profiles, RSS/CPU monitor,
# and HTTP pprof snapshots until the run exits.
#
# Examples:
#   YAK_BIN=/tmp/yak-test YAK_MEASURE_TIME=0 scripts/measure-ssa-compile.sh --code-scan \
#     ~/Target/moodle -l php --exclude-file 'vendor/*' -o /tmp/moodle.sarif
#
# Env:
#   MODE=ssa-compile|code-scan (or --code-scan)
#   YAK_BIN, YAKIT_HOME, OUT_DIR, GOGC
#   MONITOR_INTERVAL=120          RSS/CPU sample interval (seconds)
#   MONITOR_PPROF_INTERVAL=600    periodic goroutine+heap snapshots
#   YAK_MEASURE_TIME=0            skip /usr/bin/time wrapper (recommended with YAK_BIN)
#   YAK_SSA_HEAP_LOG=1, YAK_SSA_HEAP_PROFILE_DIR
#   YAK_SSA_AST_MEMORY_BUDGET     optional compiler-side AST memory budget, e.g. 10GiB
#   YAK_SSA_AST_BUILD_WINDOW_FILES optional manual AST build window override
#   YAK_SSA_GC_PERCENT            optional compiler-side large-project GC percent; GOGC wins
#   GOGC / GOMEMLIMIT             optional Go runtime GC and soft memory limit controls
set -euo pipefail

usage() {
  cat >&2 <<'USAGE'
usage: scripts/measure-ssa-compile.sh <target-path> [flags...]

Wrapper for yak ssa-compile (default) or code-scan (--code-scan / MODE=code-scan).
USAGE
}

MODE="${MODE:-ssa-compile}"
TARGET="${TARGET:-}"
EXTRA_ARGS=()

while [[ $# -gt 0 ]]; do
  case "$1" in
    -h|--help) usage; exit 0 ;;
    --code-scan) MODE=code-scan; shift ;;
    --ssa-compile) MODE=ssa-compile; shift ;;
    -l|--language) LANGUAGE="$2"; shift 2 ;;
    *)
      if [[ -z "$TARGET" && "${1:0:1}" != "-" ]]; then TARGET="$1"; shift
      else EXTRA_ARGS+=("$1"); shift; fi
      ;;
  esac
done

[[ -n "$TARGET" ]] || { usage; exit 2; }

RUN_LABEL="${RUN_LABEL:-$(basename "$TARGET")}"
SAFE_LABEL="$(printf '%s' "$RUN_LABEL" | tr -c 'A-Za-z0-9_.-' '-')"
STAMP="${YAK_MEASURE_STAMP:-$(date +%Y%m%d-%H%M%S)}"
OUT_DIR="${OUT_DIR:-build/ssa-measure/${SAFE_LABEL}-${STAMP}}"
LOG_FILE="${LOG_FILE:-$OUT_DIR/measure.log}"
MONITOR_LOG="${MONITOR_LOG:-$OUT_DIR/monitor.log}"
PPROF_FILE="${YAK_SSA_MONITOR_PPROF:-$OUT_DIR/heap-monitor.pprof}"
SNAPSHOT_DIR="${SNAPSHOT_DIR:-$OUT_DIR/pprof-snapshots}"
MONITOR_INTERVAL="${MONITOR_INTERVAL:-120}"
MONITOR_PPROF_INTERVAL="${MONITOR_PPROF_INTERVAL:-600}"
PPROF_HTTP="${PPROF_HTTP:-http://127.0.0.1:18080}"
SARIF_OUT="${SARIF_OUT:-}"

export YAKIT_HOME="${YAKIT_HOME:-$PWD/.db}"
export YAK_SSA_HEAP_LOG="${YAK_SSA_HEAP_LOG:-1}"
export YAK_SSA_HEAP_PROFILE_DIR="${YAK_SSA_HEAP_PROFILE_DIR:-$OUT_DIR/heap-profiles}"
export GOGC="${GOGC:-}"

LANGUAGE="${LANGUAGE:-java}"
PROGRAM="${PROGRAM:-ssa-measure-${SAFE_LABEL}-${STAMP}}"
LOG_LEVEL="${LOG_LEVEL:-info}"

if [[ "$MODE" == "code-scan" ]]; then
  export YAK_SSA_DIAGNOSTICS="${YAK_SSA_DIAGNOSTICS:-0}"
  export YAK_DIAGNOSTICS_LOG_LEVEL="${YAK_DIAGNOSTICS_LOG_LEVEL:-warn}"
else
  export YAK_SSA_DIAGNOSTICS="${YAK_SSA_DIAGNOSTICS:-1}"
  export YAK_DIAGNOSTICS_LOG_LEVEL="${YAK_DIAGNOSTICS_LOG_LEVEL:-trace}"
fi

mkdir -p "$OUT_DIR" "$YAKIT_HOME" "$YAK_SSA_HEAP_PROFILE_DIR" "$SNAPSHOT_DIR"

if [[ -n "${YAK_BIN:-}" ]]; then
  YAK_CMD=("$YAK_BIN")
  YAK_MEASURE_TIME="${YAK_MEASURE_TIME:-0}"
else
  YAK_CMD=(go run ./common/yak/cmd)
  YAK_MEASURE_TIME="${YAK_MEASURE_TIME:-1}"
fi

if [[ "$MODE" == "code-scan" ]]; then
  CLI_ARGS=(code-scan --target "$TARGET" --language "$LANGUAGE" --memory --log-level "$LOG_LEVEL" --pprof "$PPROF_FILE")
  for i in "${!EXTRA_ARGS[@]}"; do
  if [[ "${EXTRA_ARGS[$i]}" == "-o" && -n "${EXTRA_ARGS[$i+1]:-}" ]]; then
      SARIF_OUT="${EXTRA_ARGS[$i+1]}"
    fi
  done
else
  CLI_ARGS=(ssa-compile --target "$TARGET" --program "$PROGRAM" --language "$LANGUAGE" --re-compile --file-perf-log --pprof "$PPROF_FILE" --log "$LOG_LEVEL")
  [[ "$YAK_SSA_DIAGNOSTICS" != "0" ]] && CLI_ARGS+=(--diagnostics)
fi
CLI_ARGS+=("${EXTRA_ARGS[@]}")

if [[ -n "${PID_PATTERN:-}" ]]; then :
elif [[ -n "${YAK_BIN:-}" ]]; then
  if [[ "$MODE" == "code-scan" ]]; then PID_PATTERN="${YAK_BIN} code-scan --target ${TARGET}"
  else PID_PATTERN="${YAK_BIN} ssa-compile --target ${TARGET}"; fi
elif [[ "$MODE" == "code-scan" ]]; then PID_PATTERN="code-scan --target ${TARGET}"
else PID_PATTERN="${PROGRAM}"; fi

pick_yak_pid() {
  local pattern="$1" p cmd best=0 best_rss=0 rss
  while read -r p; do
    cmd=$(ps -p "$p" -o args= 2>/dev/null || true)
    [[ -z "$cmd" ]] && continue
    [[ "$cmd" == *"/usr/bin/time"* ]] && continue
    if [[ -n "${YAK_BIN:-}" ]]; then
      [[ "$cmd" != *"$YAK_BIN"* ]] && continue
    else
      [[ "$cmd" != *"yak-test"* && "$cmd" != *"/yak "* && "$cmd" != *"yak code-scan"* && "$cmd" != *"yak ssa-compile"* ]] && continue
    fi
    rss=$(ps -p "$p" -o rss= 2>/dev/null | tr -d ' '); rss=${rss:-0}
    [[ "$rss" -ge "$best_rss" ]] && { best_rss=$rss; best=$p; }
  done < <(pgrep -f "$pattern" 2>/dev/null || true)
  echo "$best"
}

instant_cpu_pct() {
  local pid="$1" line
  line=$(top -b -n 2 -d 1 -p "$pid" 2>/dev/null | awk -v p="$pid" '$1==p {cpu=$9} END {print cpu+0}')
  echo "${line:-0}"
}

snapshot_pprof() {
  local tag="$1" ts; ts=$(date +%H%M%S)
  curl -s -m 120 -o "${SNAPSHOT_DIR}/${ts}_${tag}_goroutine.pb.gz" "${PPROF_HTTP}/debug/pprof/goroutine" 2>/dev/null || true
  curl -s -m 120 -o "${SNAPSHOT_DIR}/${ts}_${tag}_heap.pb.gz" "${PPROF_HTTP}/debug/pprof/heap" 2>/dev/null || true
}

monitor_loop() {
  local log_file="$1" pid_pattern="$2" measure_log="$3" sarif_path="$4"
  local prev_rss=0 low_cpu_streak=0 last_pprof=0 iter=0 start_ts yakpid
  start_ts=$(date +%s)
  for _ in $(seq 1 120); do
    yakpid=$(pick_yak_pid "$pid_pattern")
    [[ -n "$yakpid" && "$yakpid" != "0" ]] && break
    sleep 2
  done
  while true; do
    iter=$((iter + 1))
    local ts elapsed rss pcpu stat wchan rss_mb heap_phase sarif_sz instant_cpu
    ts=$(date +%H%M%S)
    elapsed=$(( $(date +%s) - start_ts ))
    yakpid=$(pick_yak_pid "$pid_pattern")
    if [[ -z "$yakpid" || "$yakpid" == "0" ]]; then
      sarif_sz=0; [[ -n "$sarif_path" && -f "$sarif_path" ]] && sarif_sz=$(stat -c%s "$sarif_path" 2>/dev/null || echo 0)
      echo "[$ts] iter=$iter elapsed=${elapsed}s PROCESS_EXITED sarif_bytes=$sarif_sz" >>"$log_file"
      grep -E 'code scan done|\[ssa\.heap\]|panic|fatal' "$measure_log" 2>/dev/null | tail -8 >>"$log_file" || true
      echo 'AGENT_LOOP_WAKE_moodle_track {"prompt":"Measure run exited. Summarize monitor.log and SARIF."}'
      break
    fi
    read -r rss pcpu stat wchan <<< "$(ps -p "$yakpid" -o rss=,pcpu=,stat=,wchan= 2>/dev/null | awk '{print $1,$2,$3,$4}')"
    instant_cpu=$(instant_cpu_pct "$yakpid")
    rss_mb=$(awk -v r="${rss:-0}" 'BEGIN{printf "%.1f", r/1024}')
    heap_phase=$(grep -oE 'f[0-9]_[a-z_]+' "$measure_log" 2>/dev/null | tail -1 || echo unknown)
    sarif_sz=0; [[ -n "$sarif_path" && -f "$sarif_path" ]] && sarif_sz=$(stat -c%s "$sarif_path" 2>/dev/null || echo 0)
    echo "[$ts] iter=$iter elapsed=${elapsed}s pid=$yakpid rss_mb=$rss_mb cpu_inst=${instant_cpu}% cpu_cum=$pcpu stat=$stat wchan=$wchan heap=$heap_phase sarif=$sarif_sz" >>"$log_file"

    local now; now=$(date +%s)
    if [[ "$last_pprof" -eq 0 || $((now - last_pprof)) -ge "$MONITOR_PPROF_INTERVAL" ]]; then
      snapshot_pprof "periodic_${iter}"; last_pprof=$now
    fi

    local cpu_check=${instant_cpu%.*}
    [[ -z "$cpu_check" ]] && cpu_check=0
    if [[ "${rss:-0}" -gt 2000000 && "$cpu_check" -lt 5 ]]; then
      low_cpu_streak=$((low_cpu_streak + 1))
    else low_cpu_streak=0; fi
    if [[ "$low_cpu_streak" -ge 2 ]]; then
      snapshot_pprof "cpu_stall"
      curl -s -m 60 "${PPROF_HTTP}/debug/pprof/goroutine?debug=2" -o "${SNAPSHOT_DIR}/${ts}_goroutine_debug2.txt" 2>/dev/null || true
      echo ">>> ANOMALY_CPU_STALL rss_mb=$rss_mb cpu_inst=${instant_cpu}% wchan=$wchan" >>"$log_file"
      echo 'AGENT_LOOP_WAKE_moodle_track {"prompt":"CPU stall detected in measure monitor"}'
      low_cpu_streak=0
    fi
    if [[ "$prev_rss" -gt 0 ]]; then
      local delta=$((rss - prev_rss))
      if [[ "$delta" -gt 1500000 || "$rss" -gt 11500000 ]]; then
        snapshot_pprof "mem_spike"
        echo ">>> ANOMALY_MEM_SPIKE delta_kb=$delta rss_mb=$rss_mb" >>"$log_file"
        echo 'AGENT_LOOP_WAKE_moodle_track {"prompt":"Memory spike in measure monitor"}'
      fi
    fi
    prev_rss=${rss:-0}
    echo 'AGENT_LOOP_WAKE_moodle_track {"prompt":"Periodic measure monitor tick"}'
    sleep "$MONITOR_INTERVAL"
  done
}

{
  echo "[measure] mode=$MODE target=$TARGET out_dir=$OUT_DIR"
  echo "[measure] heap_profiles=$YAK_SSA_HEAP_PROFILE_DIR monitor=$MONITOR_LOG snapshots=$SNAPSHOT_DIR"
  echo "[measure] monitor_interval=${MONITOR_INTERVAL}s pid_pattern=$PID_PATTERN sarif=${SARIF_OUT:-<none>}"
  echo "[measure] yakit_home=$YAKIT_HOME go_gc=${GOGC:-<unset>} go_memlimit=${GOMEMLIMIT:-<unset>} ssa_gc=${YAK_SSA_GC_PERCENT:-<auto>} use_time=$YAK_MEASURE_TIME"
  echo "[measure] cmd=${YAK_CMD[*]} ${CLI_ARGS[*]}"
} | tee "$LOG_FILE"

: >"$MONITOR_LOG"
echo "monitor_start=$(date -Iseconds) pattern=${PID_PATTERN}" >>"$MONITOR_LOG"
monitor_loop "$MONITOR_LOG" "$PID_PATTERN" "$LOG_FILE" "${SARIF_OUT}" &
MONITOR_PID=$!
trap 'kill "$MONITOR_PID" 2>/dev/null || true' EXIT

EXIT_CODE=0
if [[ "$YAK_MEASURE_TIME" != "0" ]] && command -v /usr/bin/time >/dev/null 2>&1; then
  /usr/bin/time -v "${YAK_CMD[@]}" "${CLI_ARGS[@]}" 2>&1 | tee -a "$LOG_FILE" || EXIT_CODE=$?
else
  "${YAK_CMD[@]}" "${CLI_ARGS[@]}" 2>&1 | tee -a "$LOG_FILE" || EXIT_CODE=$?
fi

sleep 2
kill "$MONITOR_PID" 2>/dev/null || true
wait "$MONITOR_PID" 2>/dev/null || true
trap - EXIT

if [[ "${YAK_MEASURE_ANALYZE_HEAP:-1}" != "0" && -d "$YAK_SSA_HEAP_PROFILE_DIR" ]]; then
  scripts/analyze-heap-profiles.sh "$YAK_SSA_HEAP_PROFILE_DIR" "$OUT_DIR/pprof-top" | tee -a "$LOG_FILE" || true
fi

echo "[measure] exit_code=$EXIT_CODE log=$LOG_FILE monitor=$MONITOR_LOG"
exit "$EXIT_CODE"
