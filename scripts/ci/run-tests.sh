#!/usr/bin/env bash
set -euo pipefail

BIN_DIR="${TEST_BIN_DIR:-/tmp/test_binaries}"
MANIFEST="${BIN_DIR}/compiled_tests.txt"

# 过滤：只运行包路径包含该正则的二进制（读取 .package）
PACKAGE_FILTER_REGEX="${PACKAGE_FILTER_REGEX:-.*}"   # 例如 '^./common/ai/' 只跑 AI
# 运行细节 - 默认值（可被包级别配置覆盖）
TEST_TIMEOUT="${TEST_TIMEOUT:-2m}"      # -test.timeout
TEST_VERBOSE="${TEST_VERBOSE:-1}"       # 1=开启 -test.v
TEST_PARALLEL="${TEST_PARALLEL:-}"      # -test.parallel（包内并发，留空则不设置）
TEST_RUN_PATTERN="${TEST_RUN_PATTERN:-}"  # -test.run（用来选择子集）
TEST_SKIP_PATTERN="${TEST_SKIP_PATTERN:-}"  # -test.skip（用来跳过某些测试）

# 包级别配置文件路径
TEST_CONFIG="${TEST_CONFIG:-}"  # JSON格式的测试配置文件

# 构建包路径到二进制文件的映射
declare -a ALL_TEST_BINS
declare -a ALL_TEST_PKGS

build_package_map() {
  echo "Building package to binary mapping..."
  
  while IFS= read -r bin; do
    [[ -z "$bin" ]] && continue
    pkg_file="${bin}.package"
    [[ -f "$pkg_file" ]] || continue
    pkg_path="$(cat "$pkg_file")"
    
    # 应用过滤器
    if ! [[ "$pkg_path" =~ $PACKAGE_FILTER_REGEX ]]; then
      continue
    fi
    
    ALL_TEST_BINS+=("$bin")
    ALL_TEST_PKGS+=("$pkg_path")
  done < "$MANIFEST"
  
  echo "Found ${#ALL_TEST_BINS[@]} test binaries"
}

# 检查包是否匹配配置的包模式
pkg_matches_pattern() {
  local pkg="$1"
  local pattern="$2"
  
  # 精确匹配
  if [[ "$pkg" == "$pattern" ]]; then
    return 0
  fi
  
  # 通配符匹配：pattern 以 /... 结尾
  if [[ "$pattern" == */... ]]; then
    local prefix="${pattern%/...}"
    # 检查 pkg 是否以 prefix/ 开头（子包）或等于 prefix（本身）
    if [[ "$pkg" == "$prefix" ]] || [[ "$pkg" == "$prefix"/* ]]; then
      return 0
    fi
  fi
  
  # /. 结尾的精确匹配
  if [[ "$pattern" == */. ]]; then
    local exact="${pattern%/.}"
    if [[ "$pkg" == "$exact" ]]; then
      return 0
    fi
  fi
  
  return 1
}

# 找到匹配配置的所有测试二进制
find_matching_tests() {
  local pattern="$1"
  local -a matched_indices=()
  
  for i in "${!ALL_TEST_PKGS[@]}"; do
    if pkg_matches_pattern "${ALL_TEST_PKGS[$i]}" "$pattern"; then
      matched_indices+=("$i")
    fi
  done
  
  # 安全地输出数组（避免 unbound variable 错误）
  if [[ ${#matched_indices[@]} -gt 0 ]]; then
    echo "${matched_indices[@]}"
  fi
}

if [[ ! -f "$MANIFEST" ]]; then
  echo " Manifest not found: $MANIFEST"
  exit 1
fi

# 构建包到二进制的映射
build_package_map

echo "=== Run Compiled Tests ==="
echo "Binary Dir : $BIN_DIR"
echo "Filter     : $PACKAGE_FILTER_REGEX"
echo "Default Timeout    : $TEST_TIMEOUT"
[[ -n "$TEST_PARALLEL" ]] && echo "Default Parallel   : $TEST_PARALLEL"
echo "Verbose    : $TEST_VERBOSE"
[[ -n "$TEST_RUN_PATTERN" ]] && echo "Default Run Pattern: $TEST_RUN_PATTERN"
[[ -n "$TEST_SKIP_PATTERN" ]] && echo "Default Skip Pattern: $TEST_SKIP_PATTERN"
[[ -n "$TEST_CONFIG" ]] && echo "Config File: $TEST_CONFIG (config-driven mode)"
echo ""

# 运行单个测试（带重试机制）
run_test() {
  local bin="$1"
  local pkg_path="$2"
  local timeout="$3"
  local run_pattern="$4"
  local skip_pattern="$5"
  local parallel="$6"
  local retry="$7"          # 重试次数
  local retry_delay="$8"    # 重试延迟（秒）
  local config_source="$9"  # "default" 或 "config:pattern"
  
  local name="$(basename "$bin")"
  local log="/tmp/${name}.run.log"
  
  # 默认重试参数
  local max_retries="${retry:-0}"
  local delay="${retry_delay:-5}"
  
  # 构建测试参数
  local args=( "-test.timeout=$timeout" )
  [[ -n "$parallel" ]] && args+=("-test.parallel=$parallel")
  [[ "$TEST_VERBOSE" = "1" ]] && args+=("-test.v")
  [[ -n "$run_pattern" ]] && args+=("-test.run=$run_pattern")
  [[ -n "$skip_pattern" ]] && args+=("-test.skip=$skip_pattern")
  
  # 计算包的实际源码目录
  local pkg_dir="$pkg_path"
  pkg_dir="${pkg_dir%/...}"
  pkg_dir="${pkg_dir%/.}"
  
  if [[ ! -d "$pkg_dir" ]]; then
    echo "WARNING: Package directory not found: $pkg_dir, using current directory"
    pkg_dir="."
  fi
  
  # 重试循环
  local attempt=0
  local success=0
  
  while [[ $attempt -le $max_retries ]]; do
    if [[ $attempt -gt 0 ]]; then
      echo ""
      echo " 重试测试 (尝试 $((attempt + 1))/$((max_retries + 1))): $name"
      sleep "$delay"
    fi
    
    # 创建一个临时函数来同时输出到屏幕和日志
    exec 3>&1  # 保存原始 stdout
    
    # 使用子shell而不是代码块，这样可以正确捕获exit code
    (
      # 第一行：最重要的信息 - 运行命令
      echo "Command: (cd $pkg_dir && $bin ${args[*]})"
      echo "Test: $name | Package: $pkg_path"
      [[ "$config_source" != "default" ]] && echo "Config: $config_source"
      [[ $max_retries -gt 0 ]] && echo "Retry: enabled (max=$max_retries, delay=${delay}s, attempt=$((attempt + 1)))"
      echo "----"
      
      # 在子shell中，exit会退出子shell而不是整个脚本
      cd "$pkg_dir" && "$bin" "${args[@]}"
    ) 2>&1 | tee "$log" >&3
    
    local code=${PIPESTATUS[0]}
    exec 3>&-  # 关闭文件描述符
    
    if [[ $code -eq 0 ]]; then
      echo "PASS: $name"
      [[ $attempt -gt 0 ]] && echo " 重试成功！(在第 $((attempt + 1)) 次尝试)"
      success=1
      break
    else
      echo "FAIL: $name (exit=$code, attempt=$((attempt + 1))/$((max_retries + 1)))"
      
      # 如果还有重试机会，显示详细日志摘要
      if [[ $attempt -lt $max_retries ]]; then
        echo "失败日志摘要："
        grep -E "(FAIL|--- FAIL|panic:|test timed out|TLS handshake error)" "$log" | head -20 | sed 's/^/  /'
      fi
    fi
    
    ((attempt++))
  done
  
  if [[ $success -eq 0 ]]; then
    echo ""
    echo "测试失败，已尝试 $((max_retries + 1)) 次: $name"
    echo "完整日志: $log"
    return 1
  fi
  
  return 0
}

rc=0
declare -a processed_indices=()

# 配置驱动模式：如果有配置文件，按配置执行
if [[ -n "$TEST_CONFIG" && -f "$TEST_CONFIG" ]]; then
  echo "=== Config-Driven Mode ==="
  echo "Processing test configurations..."
  echo ""
  
  if ! command -v jq >/dev/null 2>&1; then
    echo " ERROR: jq is required for config-driven mode but not found"
    echo " Please install jq or remove TEST_CONFIG to use default mode"
    exit 1
  fi
  
  # 读取配置并执行匹配的测试
  config_count=$(jq 'length' "$TEST_CONFIG")
  echo "Found $config_count config rules"
  echo ""
  
  for ((idx=0; idx<config_count; idx++)); do
    pattern=$(jq -r ".[$idx].package" "$TEST_CONFIG")
    timeout=$(jq -r ".[$idx].timeout // empty" "$TEST_CONFIG")
    run_pattern=$(jq -r ".[$idx].run // empty" "$TEST_CONFIG")
    skip_pattern=$(jq -r ".[$idx].skip // empty" "$TEST_CONFIG")
    parallel=$(jq -r ".[$idx].parallel // empty" "$TEST_CONFIG")
    retry=$(jq -r ".[$idx].retry // empty" "$TEST_CONFIG")
    retry_delay=$(jq -r ".[$idx].retry_delay // empty" "$TEST_CONFIG")
    
    # 使用默认值填充空配置
    [[ -z "$timeout" ]] && timeout="$TEST_TIMEOUT"
    # 注意：run_pattern、skip_pattern、parallel、retry、retry_delay 如果配置中没设置，就保持空值
    
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo "Config Rule #$((idx+1)): $pattern"
    [[ "$timeout" != "$TEST_TIMEOUT" ]] && echo "  Timeout: $timeout"
    [[ -n "$run_pattern" ]] && echo "  Run: $run_pattern"
    [[ -n "$skip_pattern" ]] && echo "  Skip: $skip_pattern"
    [[ -n "$parallel" ]] && echo "  Parallel: $parallel"
    [[ -n "$retry" ]] && echo "  Retry: $retry (delay: ${retry_delay:-5}s)"
    
    # 查找匹配的测试
    matched=$(find_matching_tests "$pattern")
    
    if [[ -z "$matched" ]]; then
      echo "  No matching tests found"
      echo ""
      continue
    fi
    
    matched_array=($matched)
    echo "  Found ${#matched_array[@]} matching test(s)"
    
    # 执行匹配的测试
    for test_idx in "${matched_array[@]}"; do
      processed_indices+=("$test_idx")
      run_test "${ALL_TEST_BINS[$test_idx]}" "${ALL_TEST_PKGS[$test_idx]}" \
               "$timeout" "$run_pattern" "$skip_pattern" "$parallel" \
               "$retry" "$retry_delay" "config:$pattern" || rc=1
    done
    
    echo ""
  done
  
  # 检查是否所有编译的测试都被配置覆盖
  echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
  echo "Verifying test coverage..."
  echo ""
  
  uncovered_count=0
  uncovered_tests=()
  for i in "${!ALL_TEST_BINS[@]}"; do
    # 检查是否已处理
    is_processed=0
    for pi in "${processed_indices[@]}"; do
      if [[ "$i" == "$pi" ]]; then
        is_processed=1
        break
      fi
    done
    
    if [[ $is_processed -eq 0 ]]; then
      ((uncovered_count++))
      uncovered_tests+=("${ALL_TEST_PKGS[$i]}")
    fi
  done
  
  if [[ $uncovered_count -eq 0 ]]; then
    echo " All tests are covered by config rules"
  else
    echo " WARNING: Found $uncovered_count test(s) not covered by config:"
    for pkg in "${uncovered_tests[@]}"; do
      echo "  - $pkg"
    done
    echo ""
    echo "This should not happen if TEST_CONFIG was used during compilation."
    echo "Either:"
    echo "  1. Add these packages to TEST_CONFIG"
    echo "  2. Or they were compiled from test group dirs but not in config"
  fi
  
else
  # 默认模式：按顺序执行所有测试
  echo "=== Default Mode (no config) ==="
  echo ""
  
  for i in "${!ALL_TEST_BINS[@]}"; do
    run_test "${ALL_TEST_BINS[$i]}" "${ALL_TEST_PKGS[$i]}" \
             "$TEST_TIMEOUT" "$TEST_RUN_PATTERN" "$TEST_SKIP_PATTERN" "$TEST_PARALLEL" \
             "" "" "default" || rc=1
  done
fi

echo ""
echo "=== Test Summary ==="
echo "Total tests: ${#ALL_TEST_BINS[@]}"

if [[ $rc -eq 0 ]]; then
  echo "Result: ALL PASSED"
else
  echo "Result: SOME FAILED"
  echo ""
  echo "Failed tests:"
  # 列出所有包含失败标记的日志
  for log in /tmp/test_*.run.log; do
    [[ -f "$log" ]] || continue
    if grep -q "^FAIL:" "$log" || grep -q "^--- FAIL:" "$log" || grep -q "^FAIL$" "$log"; then
      echo "  - $(basename "$log" .run.log)"
    fi
  done
fi

exit $rc

