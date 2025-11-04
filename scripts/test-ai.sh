#!/bin/bash -e

clear
clear

set -e
set -o pipefail

export GITHUB_ACTIONS=true

# Parse arguments
FORCE_REBUILD=0
if [[ "$1" == "--force" || "$1" == "-f" ]]; then
    FORCE_REBUILD=1
    shift
fi

echo "================================================================"
echo "         AI Tests - Compile & Execute with Performance         "
echo "================================================================"
echo ""
echo "Features:"
echo "  * Separate compilation and execution phases"
echo "  * Performance tracking for each test"
echo "  * Smart caching: only recompile changed packages"
echo "  * Auto-filter tests < 1s (passed only)"
echo "  * Show tests > 10s prominently"
echo "  * Stop immediately on errors"
echo "  * Provide optimization recommendations"
echo ""
echo "Usage: $0 [--force|-f]"
echo "  --force, -f: Force rebuild all test binaries (ignore cache)"
echo ""

# ============================================================================
# Configuration - Aligned with essential-tests.yml
# ============================================================================

TEST_BIN_DIR="/tmp/ai_test_binaries"
TEST_CONFIG="/tmp/ai_test_config.json"
PERF_LOG="/tmp/ai_test_performance.log"
COMPILE_LOG="/tmp/ai_compile_performance.log"

# Clean up logs and optionally binaries based on --force flag
rm -f "$TEST_CONFIG" "$PERF_LOG" "$COMPILE_LOG"

if [[ $FORCE_REBUILD -eq 1 ]]; then
    echo "[INFO] Force rebuild requested - clearing all cached binaries"
    rm -rf "$TEST_BIN_DIR"
    mkdir -p "$TEST_BIN_DIR"
else
    echo "[INFO] Using cached test binaries from: $TEST_BIN_DIR"
    echo "[INFO] Binaries will be recompiled only if source files changed"
    rm -f "$TEST_BIN_DIR"/*.log "$TEST_BIN_DIR"/failed_packages.txt
    mkdir -p "$TEST_BIN_DIR"
fi
echo ""

# Generate test configuration (aligned with essential-tests.yml line 254-264)
cat > "$TEST_CONFIG" <<'EOF'
[
  {"package": "./common/ai/aid/...", "timeout": "12m", "parallel": 1},
  {"package": "./common/ai/tests/...", "timeout": "60s", "parallel": 1},
  {"package": "./common/ai/rag/pq/...", "timeout": "60s", "parallel": 1},
  {"package": "./common/ai/rag/hnsw/...", "timeout": "60s", "parallel": 1},
  {"package": "./common/ai/aispec/...", "timeout": "60s", "parallel": 1},
  {"package": "./common/aireducer/...", "timeout": "60s", "parallel": 1},
  {"package": "./common/aiforge/aibp", "timeout": "40s", "parallel": 1, "run": "^(TestBuildForgeFromYak|TestNewForgeExecutor)"},
  {"package": "./common/aiforge", "timeout": "3m", "parallel": 1},
  {"package": "./common/ai/rag/entityrepos/...", "timeout": "60s", "parallel": 1},
  {"package": "./common/ai/rag", "timeout": "1m", "run": "TestMUSTPASS", "parallel": 1},
  {"package": "./common/ai/rag/plugins_rag/...", "timeout": "1m", "run": "TestMUSTPASS", "parallel": 1}
]
EOF

echo "[OK] Configuration generated from essential-tests.yml"
echo ""

# ============================================================================
# Phase 1: Compilation
# ============================================================================

echo "================================================================"
echo "PHASE 1: Compiling Test Binaries"
echo "================================================================"
echo ""

compile_start=$(date +%s)

# Use local compile script if available, otherwise fall back to inline
if [[ -f "./scripts/local/compile-ai-tests.sh" ]]; then
    echo "* Using scripts/local/compile-ai-tests.sh (macOS compatible)..."
    export TEST_BIN_DIR="$TEST_BIN_DIR"
    export TEST_CONFIG="$TEST_CONFIG"
    export JOBS=4  # Parallel compilation
    
    if ! ./scripts/local/compile-ai-tests.sh 2>&1 | tee "$COMPILE_LOG"; then
        echo ""
        echo "[ERROR] Compilation failed! See details above."
        exit 1
    fi
elif [[ -f "./scripts/ci/compile-tests.sh" ]]; then
    echo "* Using scripts/ci/compile-tests.sh..."
    export TEST_BIN_DIR="$TEST_BIN_DIR"
    export TEST_CONFIG="$TEST_CONFIG"
    export JOBS=4  # Parallel compilation
    
    if ! ./scripts/ci/compile-tests.sh 2>&1 | tee "$COMPILE_LOG"; then
        echo ""
        echo "[ERROR] Compilation failed! See details above."
        exit 1
    fi
else
    echo "[WARN] scripts/ci/compile-tests.sh not found, using inline compilation..."
    
    # Inline compilation logic
    packages=$(jq -r '.[].package' "$TEST_CONFIG" | sort -u)
    
    echo "Discovering test packages..."
    test_pkgs=()
    for pkg_pattern in $packages; do
        while IFS= read -r pkg; do
            [[ -n "$pkg" ]] && test_pkgs+=("$pkg")
        done < <(go list -f '{{if or .TestGoFiles .XTestGoFiles}}{{.ImportPath}}{{end}}' "$pkg_pattern" 2>/dev/null || true)
    done
    
    total_pkgs=${#test_pkgs[@]}
    echo "Found $total_pkgs test packages to compile"
    echo ""
    
    compiled=0
    failed=0
    
    for pkg in "${test_pkgs[@]}"; do
        safe_name=$(echo "$pkg" | sed 's|^\./||' | sed 's|github\.com/yaklang/yaklang/||' | sed 's|/|_|g')
        bin="$TEST_BIN_DIR/test_${safe_name}"
        
        pkg_compile_start=$(date +%s)
        if go test -p=1 -c -o "$bin" "$pkg" 2>&1 | grep -E "(FAIL|error)" || [[ ${PIPESTATUS[0]} -ne 0 ]]; then
            pkg_compile_end=$(date +%s)
            pkg_compile_time=$((pkg_compile_end - pkg_compile_start))
            echo "[FAIL] $pkg (${pkg_compile_time}s)"
            ((failed++))
            echo "Compilation failed for $pkg" >&2
            exit 1  # Stop immediately on error
        else
            pkg_compile_end=$(date +%s)
            pkg_compile_time=$((pkg_compile_end - pkg_compile_start))
            
            # Save package path for later
            pkg_path="$pkg"
            [[ "$pkg" != ./* ]] && pkg_path="./$(echo "$pkg" | sed 's|github\.com/yaklang/yaklang/||')"
            echo "$pkg_path" > "${bin}.package"
            
            # Log compilation time
            echo "compile|${pkg}|${pkg_compile_time}" >> "$COMPILE_LOG"
            
            if [[ $pkg_compile_time -ge 10 ]]; then
                echo "[OK] $pkg -> $(basename "$bin") [${pkg_compile_time}s] [SLOW]"
            elif [[ $pkg_compile_time -ge 3 ]]; then
                echo "[OK] $pkg -> $(basename "$bin") [${pkg_compile_time}s]"
            else
                echo "[OK] $pkg -> $(basename "$bin")"
            fi
            ((compiled++))
        fi
    done
    
    # Generate compiled tests list
    find "$TEST_BIN_DIR" -maxdepth 1 -type f -name "test_*" ! -name "*.log" ! -name "*.package" | sort > "$TEST_BIN_DIR/compiled_tests.txt"
fi

compile_end=$(date +%s)
compile_duration=$((compile_end - compile_start))

echo ""
echo "Compilation Summary:"
echo "   Total time: ${compile_duration}s"
echo ""

# Show slow compilations
if [[ -f "$COMPILE_LOG" ]]; then
    slow_compiles=$(grep "^compile|" "$COMPILE_LOG" 2>/dev/null | awk -F'|' '$3 >= 10 {print $2, $3"s"}' || true)
    if [[ -n "$slow_compiles" ]]; then
        echo "[WARN] Slow compilations (>=10s):"
        echo "$slow_compiles" | while read -r pkg time; do
            echo "   * $pkg: $time"
        done
        echo ""
    fi
fi

# ============================================================================
# Phase 2: Execution
# ============================================================================

echo "================================================================"
echo "PHASE 2: Executing Tests"
echo "================================================================"
echo ""
echo "Note: Tests run serially (-p=1) to avoid database lock conflicts"
echo ""

exec_start=$(date +%s)

# Use run-tests.sh if available, otherwise run inline
if [[ -f "./scripts/ci/run-tests.sh" ]]; then
    echo "* Using scripts/ci/run-tests.sh..."
    export TEST_BIN_DIR="$TEST_BIN_DIR"
    export TEST_CONFIG="$TEST_CONFIG"
    export TEST_VERBOSE="1"
    
    # Wrap execution to capture timing
    run_start=$(date +%s)
    run_exit_code=0
    
    # Run with enhanced output processing
    ./scripts/ci/run-tests.sh 2>&1 | while IFS= read -r line; do
        echo "$line"
        
        # Extract test execution info
        if [[ "$line" =~ ^PASS:\ test_(.+)$ ]] || [[ "$line" =~ ^FAIL:\ test_(.+)\ \(exit= ]]; then
            test_name="${BASH_REMATCH[1]}"
            # Record in performance log (will be post-processed)
        fi
    done || run_exit_code=$?
    
    run_end=$(date +%s)
    
    if [[ $run_exit_code -ne 0 ]]; then
        echo ""
        echo "[ERROR] Tests failed! Stopping immediately."
        exit $run_exit_code
    fi
else
    echo "[WARN] scripts/ci/run-tests.sh not found, using inline execution..."
    
    # Inline execution logic
    manifest="$TEST_BIN_DIR/compiled_tests.txt"
    [[ ! -f "$manifest" ]] && echo "[ERROR] No compiled tests found" && exit 1
    
    # Pre-build config lookup map to avoid repeated jq calls
    # This is CRITICAL for performance when there are many tests
    echo "[PERF] Building configuration lookup map..."
    declare -A pkg_timeout_map
    declare -A pkg_parallel_map
    declare -A pkg_run_pattern_map
    
    if [[ -f "$TEST_CONFIG" ]]; then
        config_count=$(jq 'length' "$TEST_CONFIG" 2>/dev/null || echo 0)
        for ((i=0; i<config_count; i++)); do
            pkg_pattern=$(jq -r ".[$i].package" "$TEST_CONFIG")
            pkg_timeout=$(jq -r ".[$i].timeout // \"60s\"" "$TEST_CONFIG")
            pkg_parallel=$(jq -r ".[$i].parallel // 1" "$TEST_CONFIG")
            pkg_run=$(jq -r ".[$i].run // \"\"" "$TEST_CONFIG")
            
            pkg_timeout_map["$pkg_pattern"]="$pkg_timeout"
            pkg_parallel_map["$pkg_pattern"]="$pkg_parallel"
            pkg_run_pattern_map["$pkg_pattern"]="$pkg_run"
        done
        echo "[PERF] Loaded $config_count config rules"
    fi
    
    test_count=0
    passed_count=0
    failed_count=0
    
    while IFS= read -r bin; do
        [[ -z "$bin" || ! -f "$bin" ]] && continue
        
        pkg_file="${bin}.package"
        [[ ! -f "$pkg_file" ]] && continue
        pkg_path=$(cat "$pkg_file")
        
        # Get test configuration from pre-built map
        timeout="60s"
        parallel="1"
        run_pattern=""
        
        # Match package to config - check exact match and prefix match
        for pattern in "${!pkg_timeout_map[@]}"; do
            if [[ "$pkg_path" == "$pattern" ]] || [[ "$pkg_path" == "${pattern%/...}/"* ]]; then
                timeout="${pkg_timeout_map[$pattern]}"
                parallel="${pkg_parallel_map[$pattern]}"
                run_pattern="${pkg_run_pattern_map[$pattern]}"
                break
            fi
        done
        
        ((test_count++))
        
        name=$(basename "$bin")
        log="/tmp/${name}.run.log"
        
        # Prepare test directory
        test_dir="$pkg_path"
        test_dir="${test_dir%/...}"
        test_dir="${test_dir%/.}"
        [[ ! -d "$test_dir" ]] && test_dir="."
        
        # Build test arguments
        args=("-test.timeout=$timeout" "-test.parallel=$parallel" "-test.v")
        [[ -n "$run_pattern" ]] && args+=("-test.run=$run_pattern")
        
        # Run test with timing
        echo "================================================================"
        echo "Test: $pkg_path"
        echo "Timeout: $timeout | Parallel: $parallel"
        [[ -n "$run_pattern" ]] && echo "Run pattern: $run_pattern"
        echo "----------------------------------------------------------------"
        
        test_start=$(date +%s)
        test_exit_code=0
        
        # Run test with real-time output and accurate timing
        # Use process substitution to avoid subshell issues
        exec 3>&1  # Save stdout
        
        # 启动后台计时器 - 异步更新最后一行显示状态和已过时间
        timer_pid=""
        
        # 启动计时器 - 输出到 stderr
        (
            while true; do
                elapsed=$(( $(date +%s) - test_start ))
                mins=$(( elapsed / 60 ))
                secs=$(( elapsed % 60 ))
                # 使用 \r 回到行首，始终覆盖同一行（如果是 TTY）
                # 如果不是 TTY，\r 会被忽略，每次都输出新行（也可以看到进度）
                printf "\r⏱️  [%02d:%02d] Running: %s..." "$mins" "$secs" "$pkg_path" >&2
                sleep 1
            done
        ) &
        timer_pid=$!
        
        (cd "$test_dir" && "$bin" "${args[@]}") 2>&1 | tee "$log" >&3
        test_exit_code=${PIPESTATUS[0]}
        
        # 停止计时器
        if [[ -n "$timer_pid" ]]; then
            kill "$timer_pid" 2>/dev/null || true
            wait "$timer_pid" 2>/dev/null || true
        fi
        
        test_end=$(date +%s)
        test_duration=$((test_end - test_start))
        
        # 清除计时器行并显示最终时间
        printf "\r\033[K" >&2  # 清除计时器行（如果是 TTY 会清除，否则只是换行）
        
        final_mins=$(( test_duration / 60 ))
        final_secs=$(( test_duration % 60 ))
        if [[ $test_exit_code -eq 0 ]]; then
            echo "✅ Completed in ${final_mins}m${final_secs}s"
        else
            echo "❌ Failed after ${final_mins}m${final_secs}s"
        fi
        
        exec 3>&-  # Close fd
        
        # Parse actual test runtime from go test output
        # Format: "PASS" or "ok  	package_name	1.234s"
        actual_test_time=""
        if [[ -f "$log" ]]; then
            # Try to extract time from "ok" line (e.g., "ok  	github.com/xxx	1.234s")
            actual_test_time=$(grep -E "^ok\s+" "$log" | tail -1 | awk '{print $NF}' | sed 's/s$//')
            
            # If not found, try to extract from summary line (e.g., "PASS" followed by time)
            if [[ -z "$actual_test_time" ]]; then
                actual_test_time=$(grep -E "^PASS$" "$log" -A 1 | tail -1 | grep -oE "[0-9]+\.[0-9]+s" | sed 's/s$//')
            fi
        fi
        
        # Calculate initialization overhead
        init_overhead=""
        if [[ -n "$actual_test_time" ]]; then
            # Convert actual_test_time to integer seconds for comparison
            actual_int=$(echo "$actual_test_time" | awk '{printf "%.0f", $1}')
            init_overhead=$((test_duration - actual_int))
            
            # Show overhead if significant (>= 1 second or >= 20% of total time)
            if [[ $init_overhead -ge 1 ]] || [[ $init_overhead -ge $((test_duration / 5)) ]]; then
                overhead_pct=$((init_overhead * 100 / test_duration))
                init_overhead_msg=" [init overhead: ${init_overhead}s / ${overhead_pct}%]"
            else
                init_overhead_msg=""
            fi
        else
            init_overhead_msg=""
            actual_test_time="N/A"
        fi
        
        # Record performance
        if [[ $test_exit_code -eq 0 ]]; then
            status="PASS"
            ((passed_count++))
            echo "execute|${pkg_path}|${test_duration}|${actual_test_time}|${init_overhead}|PASS" >> "$PERF_LOG"
            
            # Filter output for passed tests < 1s
            if [[ $test_duration -lt 1 ]]; then
                echo "[PASS] (wall: ${test_duration}s, test: ${actual_test_time}s)${init_overhead_msg} [filtered: fast pass]"
            elif [[ $test_duration -ge 10 ]]; then
                echo "[PASS] (wall: ${test_duration}s, test: ${actual_test_time}s)${init_overhead_msg} [SLOW TEST]"
            else
                echo "[PASS] (wall: ${test_duration}s, test: ${actual_test_time}s)${init_overhead_msg}"
            fi
        else
            status="FAIL"
            ((failed_count++))
            echo "execute|${pkg_path}|${test_duration}|FAIL" >> "$PERF_LOG"
            echo "[FAIL] (${test_duration}s) [exit code: $test_exit_code]"
            echo ""
            echo "Failed test log: $log"
            echo ""
            echo "[ERROR] Stopping immediately due to test failure"
            exit 1
        fi
        
        echo ""
    done < "$manifest"
    
    echo "================================================================"
fi

exec_end=$(date +%s)
exec_duration=$((exec_end - exec_start))

# ============================================================================
# Phase 3: Performance Analysis & Recommendations
# ============================================================================

echo ""
echo "================================================================"
echo "PERFORMANCE ANALYSIS"
echo "================================================================"
echo ""

total_duration=$((compile_duration + exec_duration))

# Calculate actual test execution time (sum of wall clock time and go test time)
wall_time_total=0
test_time_total=0
init_overhead_total=0

if [[ -f "$PERF_LOG" ]]; then
    # Parse the log format: execute|package|wall_time|test_time|init_overhead|status
    while IFS='|' read -r prefix package_path wall_time test_time init_overhead status; do
        if [[ "$prefix" == "execute" && "$status" == "PASS" ]]; then
            wall_time_total=$((wall_time_total + wall_time))
            
            # Only add test_time if it's not N/A
            if [[ "$test_time" != "N/A" && -n "$test_time" ]]; then
                test_time_int=$(echo "$test_time" | awk '{printf "%.0f", $1}')
                test_time_total=$((test_time_total + test_time_int))
            fi
            
            # Add init overhead if present
            if [[ -n "$init_overhead" && "$init_overhead" != "" ]]; then
                init_overhead_total=$((init_overhead_total + init_overhead))
            fi
        fi
    done < "$PERF_LOG"
fi

# Calculate overall overhead (test orchestration + initialization)
orchestration_overhead=$((exec_duration - wall_time_total))
orchestration_pct=0
init_overhead_pct=0

if [[ $exec_duration -gt 0 ]]; then
    orchestration_pct=$((orchestration_overhead * 100 / exec_duration))
    if [[ $wall_time_total -gt 0 ]]; then
        init_overhead_pct=$((init_overhead_total * 100 / wall_time_total))
    fi
fi

echo "Time Summary:"
echo "   Compilation:           ${compile_duration}s"
echo "   Execution:             ${exec_duration}s"
if [[ $wall_time_total -gt 0 ]]; then
    echo "     - Wall time total:   ${wall_time_total}s (test binary execution)"
    if [[ $test_time_total -gt 0 ]]; then
        echo "     - Test time total:   ${test_time_total}s (go test reported)"
        echo "     - Init overhead:     ${init_overhead_total}s (${init_overhead_pct}% of wall time)"
    fi
    echo "     - Orchestration:     ${orchestration_overhead}s (${orchestration_pct}% - framework)"
fi
echo "   ───────────────────────"
echo "   Total:                 ${total_duration}s"
echo ""

# Analyze execution performance
if [[ -f "$PERF_LOG" ]]; then
    echo "Test Execution Performance:"
    echo ""
    
    # Slow tests (>=10s)
    slow_tests=$(grep "^execute|" "$PERF_LOG" | awk -F'|' '$3 >= 10 {printf "   * %-50s %3ds [%s]\n", $2, $3, $4}')
    if [[ -n "$slow_tests" ]]; then
        echo "   [SLOW] Slow Tests (>=10s):"
        echo "$slow_tests"
        echo ""
    else
        echo "   [OK] No slow tests (all <10s)"
        echo ""
    fi
    
    # Medium tests (3-9s)
    medium_tests=$(grep "^execute|" "$PERF_LOG" | awk -F'|' '$3 >= 3 && $3 < 10 {printf "   * %-50s %3ds [%s]\n", $2, $3, $4}')
    if [[ -n "$medium_tests" ]]; then
        echo "   Medium Tests (3-9s):"
        echo "$medium_tests"
        echo ""
    fi
    
    # Fast tests (summary only)
    fast_count=$(grep "^execute|" "$PERF_LOG" | awk -F'|' '$3 < 3' | wc -l | xargs)
    if [[ $fast_count -gt 0 ]]; then
        echo "   Fast Tests (<3s): $fast_count tests [not shown individually]"
        echo ""
    fi
    
    # Initialization Overhead Analysis
    echo "   ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo "   INITIALIZATION OVERHEAD ANALYSIS"
    echo "   ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo ""
    
    # Find tests with significant initialization overhead
    # Format: execute|package|wall_time|test_time|init_overhead|status
    high_overhead_tests=$(grep "^execute|" "$PERF_LOG" | awk -F'|' '
        $5 != "" && $5 >= 1 {
            overhead_pct = ($5 * 100 / $3)
            printf "   * %-45s  wall:%3ds  test:%5ss  init:%3ds (%2d%%)\n", $2, $3, $4, $5, overhead_pct
        }
    ' | sort -t: -k4 -nr)  # Sort by init overhead (descending)
    
    if [[ -n "$high_overhead_tests" ]]; then
        echo "   [WARNING] Tests with High Initialization Overhead (>=1s):"
        echo ""
        echo "   These tests spend significant time in package initialization"
        echo "   (init() functions, package-level variables, imports, etc.)"
        echo ""
        echo "$high_overhead_tests"
        echo ""
        echo "   💡 Optimization hints:"
        echo "      - Move expensive computations from init() to lazy initialization"
        echo "      - Use sync.Once for one-time setup instead of package-level vars"
        echo "      - Defer heavy imports or use build tags to exclude test-only deps"
        echo "      - Profile with 'go test -cpuprofile' to identify hot spots"
    else
        echo "   [OK] No significant initialization overhead detected"
        echo "   All tests have init overhead < 1s"
    fi
    echo ""
    
    # Statistics
    total_tests=$(grep "^execute|" "$PERF_LOG" | wc -l | xargs)
    passed_tests=$(grep "^execute|" "$PERF_LOG" | grep -c "|PASS$" || echo 0)
    failed_tests=$(grep "^execute|" "$PERF_LOG" | grep -c "|FAIL$" || echo 0)
    
    echo "   Statistics:"
    echo "      Total tests:  $total_tests"
    echo "      Passed:       $passed_tests"
    echo "      Failed:       $failed_tests"
    echo ""
fi

echo "================================================================"
echo "OPTIMIZATION RECOMMENDATIONS"
echo "================================================================"
echo ""

# Generate recommendations based on performance data
recommendations=()

# Check for slow compilations
if [[ -f "$COMPILE_LOG" ]]; then
    slow_compile_count=$(grep "^compile|" "$COMPILE_LOG" 2>/dev/null | awk -F'|' '$3 >= 10' | wc -l | xargs)
    # Ensure we have a valid number
    slow_compile_count=${slow_compile_count:-0}
    if [[ $slow_compile_count -gt 0 ]]; then
        recommendations+=("[COMPILE] $slow_compile_count package(s) have slow compilation (>=10s)")
        recommendations+=("   -> Consider splitting large test files into smaller packages")
        recommendations+=("   -> Review test dependencies and imports")
    fi
fi

# Check for slow tests
if [[ -f "$PERF_LOG" ]]; then
    slow_test_count=$(grep "^execute|" "$PERF_LOG" 2>/dev/null | awk -F'|' '$3 >= 10' | wc -l | xargs)
    # Ensure we have a valid number
    slow_test_count=${slow_test_count:-0}
    if [[ $slow_test_count -gt 0 ]]; then
        recommendations+=("[SLOW] $slow_test_count test(s) take >=10s to execute")
        recommendations+=("   -> Review test logic for unnecessary delays or operations")
        recommendations+=("   -> Consider using mocks for slow external dependencies")
        recommendations+=("   -> Check for inefficient database operations")
    fi
    
    # Check execution vs compilation ratio
    if [[ $exec_duration -gt $((compile_duration * 3)) ]]; then
        recommendations+=("[PERF] Execution time is ${exec_duration}s vs compilation ${compile_duration}s (ratio: $(echo "scale=1; $exec_duration / $compile_duration" | bc 2>/dev/null || echo "3+")x)")
        recommendations+=("   -> Focus on optimizing test execution logic")
        recommendations+=("   -> Profile slow-running tests to identify bottlenecks")
    fi
fi

# Output recommendations
if [[ ${#recommendations[@]} -eq 0 ]]; then
    echo "[OK] No significant performance issues detected!"
    echo ""
    echo "   Your AI tests are well-optimized. Great job!"
else
    for rec in "${recommendations[@]}"; do
        echo "$rec"
    done
fi

echo ""
echo "================================================================"
echo "[SUCCESS] All AI tests completed successfully!"
echo "================================================================"
echo ""

# Preserve performance logs for analysis
echo "Performance logs saved to:"
echo "   * $PERF_LOG (execution times)"
echo "   * $COMPILE_LOG (compilation times)"
echo ""

exit 0
