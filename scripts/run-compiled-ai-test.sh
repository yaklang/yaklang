#!/bin/bash -e

# 运行预编译的 AI 测试二进制文件
# 支持 -run 参数过滤测试，并提供与 test-ai.sh 相同的日志优化

set -e
set -o pipefail

export GITHUB_ACTIONS=true

# 默认测试二进制文件目录
TEST_BIN_DIR="${TEST_BIN_DIR:-/tmp/ai_test_binaries}"

# 解析命令行参数
RUN_PATTERN=""
VERBOSE="-test.v"
TIMEOUT_DURATION=""

usage() {
    echo "Usage: $0 [OPTIONS]"
    echo ""
    echo "Options:"
    echo "  -run PATTERN    Run only tests matching the pattern (equivalent to go test -run)"
    echo "  -timeout TIME   Set test timeout (e.g., 3m, 60s)"
    echo "  -q             Quiet mode (disable verbose output)"
    echo "  -h, --help     Show this help message"
    echo ""
    echo "Environment Variables:"
    echo "  TEST_BIN_DIR   Directory containing compiled test binaries (default: /tmp/ai_test_binaries)"
    echo ""
    echo "Examples:"
    echo "  $0                           # Run all tests"
    echo "  $0 -run TestMUSTPASS         # Run only tests matching TestMUSTPASS"
    echo "  $0 -run 'Test.*PASS' -timeout 5m  # Run tests matching pattern with timeout"
    echo ""
}

# 解析命令行参数
while [[ $# -gt 0 ]]; do
    case $1 in
        -run)
            RUN_PATTERN="$2"
            shift 2
            ;;
        -timeout)
            TIMEOUT_DURATION="$2"
            shift 2
            ;;
        -q|--quiet)
            VERBOSE=""
            shift
            ;;
        -h|--help)
            usage
            exit 0
            ;;
        *)
            echo "Unknown option: $1"
            usage
            exit 1
            ;;
    esac
done

# 检查测试二进制文件目录是否存在
if [ ! -d "$TEST_BIN_DIR" ]; then
    echo "❌ Test binary directory not found: $TEST_BIN_DIR"
    echo "Please run compile-ai-tests.sh first to compile the test binaries"
    exit 1
fi

# 检查编译清单文件是否存在
MANIFEST_FILE="$TEST_BIN_DIR/compiled_tests.txt"
if [ ! -f "$MANIFEST_FILE" ]; then
    echo "❌ Compilation manifest not found: $MANIFEST_FILE"
    echo "Please run compile-ai-tests.sh first to compile the test binaries"
    exit 1
fi

# 读取编译清单
if [ ! -s "$MANIFEST_FILE" ]; then
    echo "❌ No compiled test binaries found in manifest"
    echo "Please run compile-ai-tests.sh first to compile the test binaries"
    exit 1
fi

echo "Running compiled AI tests from: $TEST_BIN_DIR"
echo "Method: Pre-compiled test binary execution"
if [ -n "$RUN_PATTERN" ]; then
    echo "Test pattern: $RUN_PATTERN"
fi
if [ -n "$TIMEOUT_DURATION" ]; then
    echo "Test timeout: $TIMEOUT_DURATION"
fi

# 记录开始时间
start_time=$(date +%s)

# 统计测试二进制文件数量
total_binaries=$(wc -l < "$MANIFEST_FILE")
echo "Found $total_binaries compiled test binaries"

# 运行每个测试二进制文件
failed_tests=0
passed_tests=0
test_counter=0

while IFS= read -r binary_path; do
    test_counter=$((test_counter + 1))
    
    # 检查二进制文件是否存在
    if [ ! -f "$binary_path" ]; then
        echo "⚠ Binary not found: $binary_path"
        continue
    fi
    
    # 获取包信息
    package_file="${binary_path}.package"
    if [ -f "$package_file" ]; then
        package_name=$(cat "$package_file")
    else
        # 从二进制文件名推断包名
        binary_name=$(basename "$binary_path")
        package_name=$(echo "$binary_name" | sed 's/^test_//' | sed 's/_/\//g')
        package_name="./$package_name"
    fi
    
    echo ""
    echo "[$test_counter/$total_binaries] Running: $package_name"
    
    # 构建测试命令参数
    test_args=()
    if [ -n "$VERBOSE" ]; then
        test_args+=("$VERBOSE")
    fi
    if [ -n "$RUN_PATTERN" ]; then
        test_args+=("-test.run" "$RUN_PATTERN")
    fi
    if [ -n "$TIMEOUT_DURATION" ]; then
        test_args+=("-test.timeout" "$TIMEOUT_DURATION")
    fi
    
    # 生成日志文件名（基于包名）
    safe_package_name=$(echo "$package_name" | sed 's|^\./||' | sed 's|/|_|g')
    log_file="/tmp/${safe_package_name}_test.log"
    
    # 运行测试二进制文件
    echo "Executing: $binary_path ${test_args[*]}"
    
    if "$binary_path" "${test_args[@]}" 2>&1 | tee "$log_file" | { 
        grep -E -A10 -B10 "(FAIL|--- FAIL|panic:|test timed out)" || 
        grep -E "(PASS|RUN|=== RUN|--- PASS|TestTemplate|panic:|goroutine.*\[(running|sleep)\]|testing\..*panic|recovered)" "$log_file"
    }; then
        echo "✓ PASSED: $package_name"
        passed_tests=$((passed_tests + 1))
    else
        echo "✗ FAILED: $package_name"
        failed_tests=$((failed_tests + 1))
        
        # 显示失败的详细信息
        echo "--- Failure details for $package_name ---"
        if [ -f "$log_file" ]; then
            grep -E -A5 -B5 "(FAIL|--- FAIL|panic:|test timed out)" "$log_file" || echo "No specific failure pattern found in log"
        fi
        echo "--- End failure details ---"
    fi
    
done < "$MANIFEST_FILE"

# 计算总执行时间
end_time=$(date +%s)
total_duration=$((end_time - start_time))

echo ""
echo "=== AI Compiled Tests Execution Summary ==="
echo "Total test binaries: $total_binaries"
echo "Passed: $passed_tests"
echo "Failed: $failed_tests"
echo "Success rate: $(( (passed_tests * 100) / total_binaries ))%"
echo "Total execution time: ${total_duration}s"

if [ -n "$RUN_PATTERN" ]; then
    echo "Test pattern used: $RUN_PATTERN"
fi

# 清理临时日志文件（可选）
if [ "${KEEP_LOGS:-}" != "true" ]; then
    echo "Cleaning up temporary log files..."
    rm -f /tmp/*_test.log
fi

# 退出状态
if [ $failed_tests -gt 0 ]; then
    echo ""
    echo "❌ Some tests failed. Check the output above for details."
    echo "Set KEEP_LOGS=true to preserve log files for debugging."
    exit 1
else
    echo ""
    echo "✅ All tests passed successfully!"
    exit 0
fi
