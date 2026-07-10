#!/bin/bash
# scan-project.sh — Run yak code-scan on a single project, capture all metrics + pprof + observations.
# Usage: ./scripts/scan-project.sh <project_name> <target_path> [language] [code_lines] [file_count]
set -uo pipefail

PROJECT_NAME="$1"
TARGET_PATH="$2"
LANG_OVERRIDE="${3:-}"
CODE_LINES_PRE="${4:-}"
FILE_COUNT_PRE="${5:-}"

WORKDIR="$(cd "$(dirname "$0")/.." && pwd)"
YAK_BIN="$WORKDIR/yak"
BUILD_DIR="$WORKDIR/build"
PROJECT_DIR="$BUILD_DIR/$PROJECT_NAME"
YAKIT_HOME_DIR="$WORKDIR/.db"

mkdir -p "$PROJECT_DIR" "$YAKIT_HOME_DIR" "$PROJECT_DIR/pprof/cpu" "$PROJECT_DIR/pprof/mem" "$PROJECT_DIR/pprof/goroutine"

LOG_FILE="$PROJECT_DIR/scan.log"
REPORT_FILE="$PROJECT_DIR/report.sarif"
SSA_DB="$PROJECT_DIR/ssa.db"
META_FILE="$PROJECT_DIR/metrics.txt"
OBS_FILE="$PROJECT_DIR/observations.md"
RESOURCE_CSV="$PROJECT_DIR/resource-snapshots.csv"
HEAP_PPROF="$PROJECT_DIR/pprof/heap_before_gc.pb.gz"
PPROF_PORT="127.0.0.1:18080"

echo "=========================================="
echo "  Project:  $PROJECT_NAME"
echo "  Target:   $TARGET_PATH"
echo "  Language: ${LANG_OVERRIDE:-auto-detect}"
echo "  Started:  $(date -Iseconds)"
echo "=========================================="

detect_language() {
    local dir="$1"
    if find "$dir" -maxdepth 3 -name 'go.mod' 2>/dev/null | grep -q .; then
        echo "go"
    elif find "$dir" -maxdepth 3 -name 'pom.xml' -o -name 'build.gradle' 2>/dev/null | grep -q .; then
        echo "java"
    elif find "$dir" -maxdepth 2 \( -name 'composer.json' -o -name '*.php' \) 2>/dev/null | head -1 | grep -q .; then
        echo "php"
    elif find "$dir" -maxdepth 2 \( -name '*.py' -o -name 'requirements.txt' \) 2>/dev/null | head -1 | grep -q .; then
        echo "python"
    else
        echo "unknown"
    fi
}

if [ -n "$LANG_OVERRIDE" ] && [ "$LANG_OVERRIDE" != "auto" ]; then
    LANG_DETECTED="$LANG_OVERRIDE"
else
    LANG_DETECTED=$(detect_language "$TARGET_PATH")
fi
echo "[info] language: $LANG_DETECTED"

count_code() {
    local dir="$1" lang="$2"
    case "$lang" in
        go)      find "$dir" -type f -name '*.go' -not -path '*/vendor/*' -not -path '*/.git/*' -exec cat {} + 2>/dev/null | wc -l ;;
        java)    find "$dir" -type f -name '*.java' -not -path '*/.git/*' -exec cat {} + 2>/dev/null | wc -l ;;
        php)     find "$dir" -type f -name '*.php' -not -path '*/vendor/*' -not -path '*/.git/*' -exec cat {} + 2>/dev/null | wc -l ;;
        python)  find "$dir" -type f -name '*.py' -not -path '*/node_modules/*' -not -path '*/.git/*' -exec cat {} + 2>/dev/null | wc -l ;;
        *)       find "$dir" -type f \( -name '*.go' -o -name '*.java' -o -name '*.php' -o -name '*.py' \) -not -path '*/vendor/*' -not -path '*/.git/*' -exec cat {} + 2>/dev/null | wc -l ;;
    esac
}

count_files() {
    local dir="$1" lang="$2"
    case "$lang" in
        go)      find "$dir" -type f -name '*.go' -not -path '*/vendor/*' -not -path '*/.git/*' 2>/dev/null | wc -l ;;
        java)    find "$dir" -type f -name '*.java' -not -path '*/.git/*' 2>/dev/null | wc -l ;;
        php)     find "$dir" -type f -name '*.php' -not -path '*/vendor/*' -not -path '*/.git/*' 2>/dev/null | wc -l ;;
        python)  find "$dir" -type f -name '*.py' -not -path '*/node_modules/*' -not -path '*/.git/*' 2>/dev/null | wc -l ;;
        *)       find "$dir" -type f \( -name '*.go' -o -name '*.java' -o -name '*.php' -o -name '*.py' \) -not -path '*/vendor/*' -not -path '*/.git/*' 2>/dev/null | wc -l ;;
    esac
}

if [ -n "$CODE_LINES_PRE" ] && [ -n "$FILE_COUNT_PRE" ]; then
    CODE_LINES="$CODE_LINES_PRE"
    FILE_COUNT="$FILE_COUNT_PRE"
else
    CODE_LINES=$(count_code "$TARGET_PATH" "$LANG_DETECTED")
    FILE_COUNT=$(count_files "$TARGET_PATH" "$LANG_DETECTED")
fi
echo "[info] code lines ($LANG_DETECTED): $CODE_LINES  files: $FILE_COUNT"

echo "timestamp,cpu_pct,rss_mb,mem_pct,swap_mb,progress_pct,ssa_db_mb,gc_count,heap_alloc_mb,heap_objects" > "$RESOURCE_CSV"

monitor_loop() {
    local pid=$1
    local interval=30
    local pprof_counter=0
    
    while kill -0 "$pid" 2>/dev/null; do
        local ts=$(date -Iseconds)
        local proc_info=$(ps -p "$pid" -o %cpu,rss,%mem --no-headers 2>/dev/null)
        local cpu_pct=$(echo "$proc_info" | awk '{print $1}')
        local rss_kb=$(echo "$proc_info" | awk '{print $2}')
        local mem_pct=$(echo "$proc_info" | awk '{print $3}')
        local rss_mb=0; [ -n "$rss_kb" ] && rss_mb=$((rss_kb / 1024))
        cpu_pct=${cpu_pct:-0}; mem_pct=${mem_pct:-0}
        
        local swap_mb=$(awk '/^SwapTotal/{t=$2} /^SwapFree/{f=$2} END{print (t-f)/1024}' /proc/meminfo 2>/dev/null || echo 0)
        
        local progress=$(grep -a 'compile finish' "$LOG_FILE" 2>/dev/null | tail -1 | grep -oP '0\.\d+' || echo "0")
        local db_mb=$(du -m "$SSA_DB" 2>/dev/null | cut -f1 || echo 0)
        
        local heap_line=$(grep -a 'heap.*num_gc' "$LOG_FILE" 2>/dev/null | tail -1)
        local gc_count=$(echo "$heap_line" | grep -oP 'num_gc=\d+' | grep -oP '\d+' || echo 0)
        local heap_alloc=$(echo "$heap_line" | grep -oP 'alloc=\d+MB' | grep -oP '\d+' || echo 0)
        local heap_obj=$(echo "$heap_line" | grep -oP 'heap_objects=\d+' | grep -oP '\d+' || echo 0)
        
        echo "$ts,$cpu_pct,$rss_mb,$mem_pct,$swap_mb,$progress,$db_mb,$gc_count,$heap_alloc,$heap_obj" >> "$RESOURCE_CSV"
        
        pprof_counter=$((pprof_counter + 1))
        if [ $pprof_counter -ge 10 ]; then
            pprof_counter=0
            local pprof_ts=$(date +%H%M%S)
            if curl -s --connect-timeout 2 "http://$PPROF_PORT/debug/pprof/" >/dev/null 2>&1; then
                curl -s "http://$PPROF_PORT/debug/pprof/heap" > "$PROJECT_DIR/pprof/mem/${pprof_ts}.mem.prof" 2>/dev/null &
                curl -s "http://$PPROF_PORT/debug/pprof/goroutine" > "$PROJECT_DIR/pprof/goroutine/${pprof_ts}.goroutine.prof" 2>/dev/null &
                curl -s "http://$PPROF_PORT/debug/pprof/profile?seconds=30" > "$PROJECT_DIR/pprof/cpu/${pprof_ts}.cpu.prof" 2>/dev/null &
                echo "[monitor] pprof snapshots saved at $pprof_ts" >> "$LOG_FILE"
            fi
        fi
        
        sleep "$interval"
    done
}

YAK_ARGS="code-scan -t $TARGET_PATH --db $SSA_DB --pprof $HEAP_PPROF --rule-perf-log --file-perf-log --rule-timeout 10m --format sarif -o $REPORT_FILE --log-level info"
if [ -n "$LANG_OVERRIDE" ] && [ "$LANG_OVERRIDE" != "auto" ] && [ "$LANG_OVERRIDE" != "unknown" ]; then
    YAK_ARGS="$YAK_ARGS -l $LANG_OVERRIDE"
fi

echo "[info] starting yak code-scan..."
echo "[info] command: $YAK_BIN $YAK_ARGS"
START_TS=$(date +%s.%N)
START_ISO=$(date -Iseconds)

export YAKIT_HOME="$YAKIT_HOME_DIR"
export YAK_DIAGNOSTICS_LOG_LEVEL=trace

eval "$YAK_BIN $YAK_ARGS" > "$LOG_FILE" 2>&1 &
SCAN_PID=$!
echo "[info] scan PID=$SCAN_PID"

monitor_loop "$SCAN_PID" &
MONITOR_PID=$!
echo "[info] monitor PID=$MONITOR_PID"

wait "$SCAN_PID"
SCAN_EXIT=$?

kill "$MONITOR_PID" 2>/dev/null || true
wait "$MONITOR_PID" 2>/dev/null || true

END_TS=$(date +%s.%N)
END_ISO=$(date -Iseconds)

TOTAL_WALL=$(echo "$END_TS - $START_TS" | bc)
echo "[info] scan exit code: $SCAN_EXIT"
echo "[info] total wall time: ${TOTAL_WALL}s"

if curl -s --connect-timeout 2 "http://$PPROF_PORT/debug/pprof/" >/dev/null 2>&1; then
    curl -s "http://$PPROF_PORT/debug/pprof/heap" > "$PROJECT_DIR/pprof/mem/final.mem.prof" 2>/dev/null
    curl -s "http://$PPROF_PORT/debug/pprof/goroutine" > "$PROJECT_DIR/pprof/goroutine/final.goroutine.prof" 2>/dev/null
    curl -s "http://$PPROF_PORT/debug/pprof/profile?seconds=10" > "$PROJECT_DIR/pprof/cpu/final.cpu.prof" 2>/dev/null
    echo "[info] final pprof captured"
fi

parse_table_val() {
    local key="$1"
    grep -a "$key" "$LOG_FILE" 2>/dev/null | tail -1 | awk -F'|' '{gsub(/ /,"",$3); print $3}'
}

SCAN_TIME=$(parse_table_val "Total Scan Time")
TOTAL_RULES=$(parse_table_val "Total Rules")
RISK_COUNT=$(parse_table_val "Risk Count")
SUCCESS_RULES=$(parse_table_val "Success Rules")
FAILED_RULES=$(parse_table_val "Failed Rules")

STORAGE_TIME=$(grep -a 'IR cache flush finished' "$LOG_FILE" 2>/dev/null | tail -1 | sed 's/.*cost //' | sed 's/:.*//')
META_SAVE_COST=$(grep -a 'save to database cost:' "$LOG_FILE" 2>/dev/null | tail -1 | sed 's/.*cost: //')
RULE_SYNC_COST=$(grep -a 'sync rule from embed to database success' "$LOG_FILE" 2>/dev/null | tail -1 | sed 's/.*cost //')

COMPILE_START=$(grep -a 'get program from target path' "$LOG_FILE" 2>/dev/null | head -1 | grep -oP '\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}')
COMPILE_END=$(grep -a 'IR cache flush finished' "$LOG_FILE" 2>/dev/null | tail -1 | grep -oP '\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}')

if [ -n "$COMPILE_START" ] && [ -n "$COMPILE_END" ]; then
    COMPILE_TIME=$(echo "$(date -d "$COMPILE_END" +%s) - $(date -d "$COMPILE_START" +%s)" | bc)
else
    COMPILE_TIME=""
fi

FINAL_PROGRESS=$(grep -a 'status=.*progress=' "$LOG_FILE" 2>/dev/null | tail -1 | grep -oP 'progress=\K[0-9.]+' || echo "")
LAST_PHASE=$(grep -a 'f1_units\|f3_unit_build\|f4_finish\|f5_save_db\|start to scan\|get program\|IR cache flush\|status=done\|status=executing' "$LOG_FILE" 2>/dev/null | tail -1 | grep -oP 'f[1-5]_\w+\|get program\|IR cache flush\|start to scan\|scan done\|executing' || echo "")

cat > "$META_FILE" << EOF
project_name=$PROJECT_NAME
target_path=$TARGET_PATH
language=$LANG_DETECTED
code_lines=$CODE_LINES
file_count=$FILE_COUNT
total_wall_seconds=$TOTAL_WALL
scan_time=$SCAN_TIME
total_rules=$TOTAL_RULES
success_rules=$SUCCESS_RULES
failed_rules=$FAILED_RULES
risk_count=$RISK_COUNT
storage_time=$STORAGE_TIME
meta_save_cost=$META_SAVE_COST
rule_sync_cost=$RULE_SYNC_COST
compile_time_seconds=$COMPILE_TIME
scan_exit_code=$SCAN_EXIT
start_iso=$START_ISO
end_iso=$END_ISO
final_progress=$FINAL_PROGRESS
last_phase=$LAST_PHASE
EOF

MAX_RSS=$(awk -F',' 'NR>1{if($3>m) m=$3} END{print m+0}' "$RESOURCE_CSV" 2>/dev/null || echo 0)
MAX_CPU=$(awk -F',' 'NR>1{if($2>m) m=$2} END{print m+0}' "$RESOURCE_CSV" 2>/dev/null || echo 0)
MAX_GC=$(awk -F',' 'NR>1{if($8>m) m=$8} END{print m+0}' "$RESOURCE_CSV" 2>/dev/null || echo 0)
MAX_HEAP=$(awk -F',' 'NR>1{if($9>m) m=$9} END{print m+0}' "$RESOURCE_CSV" 2>/dev/null || echo 0)
MAX_HEAP_OBJ=$(awk -F',' 'NR>1{if($10>m) m=$10} END{print m+0}' "$RESOURCE_CSV" 2>/dev/null || echo 0)
MAX_SWAP=$(awk -F',' 'NR>1{if($5>m) m=$5} END{print m+0}' "$RESOURCE_CSV" 2>/dev/null || echo 0)
SNAPSHOTS=$(awk 'NR>1' "$RESOURCE_CSV" 2>/dev/null | wc -l)

LARGE_PROJECT_INFO=$(grep -a 'large project detected' "$LOG_FILE" 2>/dev/null | tail -1 | sed 's/\x1b\[[0-9;]*m//g' || echo "")
ANTLR_RESETS=$(grep -ac 'ANTLR cache reset' "$LOG_FILE" 2>/dev/null || echo 0)

cat > "$OBS_FILE" << EOF
# Observations: $PROJECT_NAME

## Run Status
- **Exit Code:** $SCAN_EXIT
- **Completed:** $([ "$SCAN_EXIT" = "0" ] && echo "YES" || echo "NO (partial/incomplete)")
- **Final Progress:** $FINAL_PROGRESS
- **Last Phase:** $LAST_PHASE
- **Start Time:** $START_ISO
- **End Time:** $END_ISO
- **Total Wall Time:** ${TOTAL_WALL}s

## Resource Usage (from resource-snapshots.csv)
- **Peak RSS:** ${MAX_RSS} MB
- **Peak CPU:** ${MAX_CPU}%
- **Peak Swap:** ${MAX_SWAP} MB
- **Peak Heap Alloc:** ${MAX_HEAP} MB
- **Peak Heap Objects:** ${MAX_HEAP_OBJ}
- **Peak GC Count:** $MAX_GC
- **Snapshot Count:** $SNAPSHOTS
- **ANTLR Cache Resets:** $ANTLR_RESETS

## Large Project Auto-Tuning
$(echo "$LARGE_PROJECT_INFO" | head -1)

## CPU/Memory Stability Notes
EOF

if [ "$MAX_GC" -gt 500 ] 2>/dev/null; then
    echo "- **GC pressure high**: $MAX_GC GC cycles detected, heap oscillating between values" >> "$OBS_FILE"
fi
if [ "$MAX_RSS" -gt 8000 ] 2>/dev/null; then
    echo "- **High memory usage**: Peak RSS ${MAX_RSS}MB" >> "$OBS_FILE"
fi
if [ "$MAX_SWAP" -gt 1000 ] 2>/dev/null; then
    echo "- **Significant swap usage**: Peak swap ${MAX_SWAP}MB, indicating memory pressure beyond physical RAM" >> "$OBS_FILE"
fi
if [ "$ANTLR_RESETS" -gt 10 ] 2>/dev/null; then
    echo "- **Frequent ANTLR cache resets**: $ANTLR_RESETS resets, indicating memory management pressure" >> "$OBS_FILE"
fi
if [ "$MAX_CPU" -gt 300 ] 2>/dev/null; then
    echo "- **Multi-core utilization**: Peak CPU ${MAX_CPU}%, indicating concurrent compilation workers" >> "$OBS_FILE"
fi
if [ -n "$FINAL_PROGRESS" ] && [ "$(echo "$FINAL_PROGRESS < 0.5" | bc 2>/dev/null)" = "1" ] && [ "$SCAN_EXIT" != "0" ]; then
    echo "- **Scan killed during early compilation**: only ${FINAL_PROGRESS} progress before termination" >> "$OBS_FILE"
fi
if [ "$SCAN_EXIT" = "0" ]; then
    echo "- **Scan completed successfully**: all phases finished, exit code 0" >> "$OBS_FILE"
else
    echo "- **Scan did not complete**: exit code $SCAN_EXIT, see log for details" >> "$OBS_FILE"
fi

cat >> "$OBS_FILE" << EOF

## Artifacts
- **Scan Log:** \`build/$PROJECT_NAME/scan.log\`
- **Report:** \`build/$PROJECT_NAME/report.sarif\`
- **SSA DB:** \`build/$PROJECT_NAME/ssa.db\`
- **Resource CSV:** \`build/$PROJECT_NAME/resource-snapshots.csv\`
- **Heap pprof (auto):** \`build/$PROJECT_NAME/pprof/heap_before_gc.pb.gz\`
- **HTTP pprof snapshots:** \`build/$PROJECT_NAME/pprof/{cpu,mem,goroutine}/\`
- **Metrics:** \`build/$PROJECT_NAME/metrics.txt\`
EOF

echo "[info] metrics saved to $META_FILE"
echo "[info] observations saved to $OBS_FILE"
echo "[info] resource snapshots saved to $RESOURCE_CSV ($SNAPSHOTS snapshots)"
echo "[info] pprof files: $(find "$PROJECT_DIR/pprof/" -type f 2>/dev/null | wc -l) files"
echo "=========================================="
echo "  Project:  $PROJECT_NAME  DONE (exit=$SCAN_EXIT, wall=${TOTAL_WALL}s)"
echo "=========================================="
