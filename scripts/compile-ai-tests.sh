#!/bin/bash -e

# AI 测试编译脚本 - 并行编译所有 AI 测试包
# 确保所有包都能编译通过，避免文件名冲突

clear
clear

set -e
set -o pipefail

echo "=== AI Tests Compilation Phase ==="

# 发现所有包含测试文件的 AI 相关包
discover_test_packages() {
    local base_dirs=(
        "./common/ai/aid"
        "./common/ai/tests"
        "./common/ai/rag/pq"
        "./common/ai/rag/hnsw"
        "./common/ai/aispec"
        "./common/aireducer"
        "./common/aiforge"
        "./common/ai/rag/entityrepos"
        "./common/ai/rag"
    )
    
    local packages=()
    
    for base_dir in "${base_dirs[@]}"; do
        if [ -d "$base_dir" ]; then
            # 查找包含 *_test.go 文件的目录
            while IFS= read -r -d '' dir; do
                # 检查目录是否包含测试文件
                if ls "$dir"/*_test.go >/dev/null 2>&1; then
                    packages+=("$dir")
                fi
            done < <(find "$base_dir" -type d -print0)
        fi
    done
    
    # 去重并排序
    printf '%s\n' "${packages[@]}" | sort -u
}

# 生成安全的二进制文件名，包含完整路径信息
generate_binary_name() {
    local package="$1"
    local safe_name=$(echo "$package" | sed 's|^\./||' | sed 's|/|_|g')
    echo "test_${safe_name}"
}

echo "Discovering test packages..."
# 使用兼容性更好的方式读取数组
TEST_PACKAGES=()
while IFS= read -r package; do
    TEST_PACKAGES+=("$package")
done < <(discover_test_packages)

echo "Found ${#TEST_PACKAGES[@]} test packages:"
for pkg in "${TEST_PACKAGES[@]}"; do
    echo "  - $pkg"
done

# 创建测试二进制文件目录
TEST_BIN_DIR="${TEST_BIN_DIR:-/tmp/ai_test_binaries}"
rm -rf "$TEST_BIN_DIR"  # 清理旧的编译产物
mkdir -p "$TEST_BIN_DIR"

echo "Compiling AI test packages to: $TEST_BIN_DIR"
compile_start=$(date +%s)

# 串行编译所有测试包（避免并发问题和资源竞争）
echo "Compiling packages sequentially to ensure stability..."

total_packages=${#TEST_PACKAGES[@]}

for ((i=0; i<total_packages; i++)); do
    package="${TEST_PACKAGES[i]}"
    
    echo "[$((i+1))/$total_packages] Compiling: $package"
    
    # 生成包含路径信息的二进制文件名
    binary_name=$(generate_binary_name "$package")
    binary_path="$TEST_BIN_DIR/$binary_name"
    
    # 编译测试包，重定向错误输出到日志文件
    compile_log="$TEST_BIN_DIR/${binary_name}_compile.log"
    if go test -p=1 -c -o "$binary_path" "$package" 2>"$compile_log"; then
        echo "✓ Compiled: $package -> $binary_name"
        # 创建包信息文件，用于后续执行
        echo "$package" > "${binary_path}.package"
        # 删除成功的编译日志
        rm -f "$compile_log"
    else
        echo "✗ Failed to compile: $package"
        echo "  Error log: $compile_log"
        # 保留失败的编译日志用于调试
        echo "$package" >> "$TEST_BIN_DIR/failed_packages.txt"
        # 不要退出，让其他包继续编译
    fi
done

compile_end=$(date +%s)
compile_duration=$((compile_end - compile_start))

# 统计编译结果
compiled_count=$(find "$TEST_BIN_DIR" -name "test_*" -type f ! -name "*.package" ! -name "*_compile.log" 2>/dev/null | wc -l)
failed_count=0
if [ -f "$TEST_BIN_DIR/failed_packages.txt" ]; then
    failed_count=$(wc -l < "$TEST_BIN_DIR/failed_packages.txt")
fi

echo "=== Compilation Results ==="
echo "Total packages: ${#TEST_PACKAGES[@]}"
echo "Successfully compiled: $compiled_count"
echo "Failed to compile: $failed_count"
echo "Compilation time: ${compile_duration}s"

# 显示失败的包（如果有）
if [ $failed_count -gt 0 ]; then
    echo ""
    echo "Failed packages:"
    sort -u "$TEST_BIN_DIR/failed_packages.txt"
    echo ""
    echo "Compilation logs for failed packages are available in: $TEST_BIN_DIR/*_compile.log"
fi

# 生成编译清单（只包含成功编译的包）
echo "Generating compilation manifest..."
find "$TEST_BIN_DIR" -name "test_*" -type f ! -name "*.package" ! -name "*_compile.log" | sort > "$TEST_BIN_DIR/compiled_tests.txt"

# 验证清单文件生成成功
if [ -f "$TEST_BIN_DIR/compiled_tests.txt" ]; then
    manifest_count=$(wc -l < "$TEST_BIN_DIR/compiled_tests.txt")
    echo "Generated manifest with $manifest_count entries"
else
    echo "✗ Failed to generate compilation manifest"
    exit 1
fi

echo "=== Compilation Phase Completed ==="
echo "Binary directory: $TEST_BIN_DIR"
echo "Successfully compiled tests: $compiled_count"
echo "Total compilation time: ${compile_duration}s"

# 严格验证：不允许任何编译失败
if [ $failed_count -gt 0 ]; then
    echo ""
    echo "❌ COMPILATION VALIDATION FAILED"
    echo "❌ Found $failed_count failed package(s) - this is not allowed"
    echo "❌ All AI test packages MUST compile successfully"
    echo ""
    echo "Detailed error information:"
    if [ -f "$TEST_BIN_DIR/failed_packages.txt" ]; then
        while IFS= read -r failed_pkg; do
            safe_name=$(echo "$failed_pkg" | sed 's|^\./||' | sed 's|/|_|g')
            error_log="$TEST_BIN_DIR/test_${safe_name}_compile.log"
            if [ -f "$error_log" ]; then
                echo "--- Compilation error for $failed_pkg ---"
                cat "$error_log"
                echo ""
            fi
        done < "$TEST_BIN_DIR/failed_packages.txt"
    fi
    echo "❌ Please fix all compilation errors before proceeding"
    echo "❌ Expected: 100% compilation success rate"
    echo "❌ Actual: $((total_packages - failed_count))/$total_packages packages compiled successfully"
    exit 1
fi

# 验证编译成功率必须是 100%
total_packages=${#TEST_PACKAGES[@]}
if [ $compiled_count -ne $total_packages ]; then
    echo "❌ COMPILATION VALIDATION FAILED"
    echo "❌ Expected $total_packages compiled packages, but got $compiled_count"
    echo "❌ All packages must compile successfully"
    exit 1
fi

echo "Use the compiled test binaries with their package information files (.package) for execution"