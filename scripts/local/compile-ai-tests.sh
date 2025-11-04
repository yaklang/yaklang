#!/usr/bin/env bash
set -euo pipefail

# 本地 AI 测试专用编译脚本
# 兼容 macOS bash 3.x 和 Linux bash 4.x

# 可覆写：并行度、输出目录、测试配置
JOBS=${JOBS:-$(getconf _NPROCESSORS_ONLN 2>/dev/null || sysctl -n hw.ncpu || echo 2)}
TEST_BIN_DIR="${TEST_BIN_DIR:-/tmp/test_binaries}"
TEST_CONFIG="${TEST_CONFIG:-}"  # 测试配置文件（JSON格式），必须提供

if [[ -z "$TEST_CONFIG" ]]; then
  echo "ERROR: TEST_CONFIG environment variable must be set"
  exit 1
fi

if [[ ! -f "$TEST_CONFIG" ]]; then
  echo "ERROR: TEST_CONFIG file not found: $TEST_CONFIG"
  exit 1
fi

if ! command -v jq >/dev/null 2>&1; then
  echo "ERROR: jq is required but not found"
  exit 1
fi

echo "=== Compile Tests from Config ==="
echo "JOBS=$JOBS"
echo "BIN_DIR=$TEST_BIN_DIR"
echo "CONFIG=$TEST_CONFIG"
echo ""

# 记录开始时间
compile_start=$(date +%s)

# 创建目录（如果不存在）
mkdir -p "$TEST_BIN_DIR"

# 清理旧的编译日志和失败记录，但保留二进制文件和缓存文件
rm -f "$TEST_BIN_DIR"/*.log
rm -f "$TEST_BIN_DIR"/failed_packages.txt
rm -f "$TEST_BIN_DIR"/compiled_tests.txt

# 创建临时文件存储 package -> race 映射（兼容 bash 3.x）
RACE_CONFIG_CACHE="$TEST_BIN_DIR/.race_config_cache"
jq -r '.[] | "\(.package)|\(.race // false)"' "$TEST_CONFIG" > "$RACE_CONFIG_CACHE"

# 获取唯一的包路径列表
CONFIG_DIRS=($(jq -r '.[].package' "$TEST_CONFIG" | sort -u))

echo "Found ${#CONFIG_DIRS[@]} unique package patterns in config"
echo ""

# 发现这些包路径下的实际测试包
echo "=== Discovering test packages ==="

# 一次性用 go list 列出所有测试包，然后用 sort -u 去重
# 兼容 bash 3.x (macOS 默认) - 不使用 mapfile
PKGS=()
while IFS= read -r pkg; do
  [[ -n "$pkg" ]] && PKGS+=("$pkg")
done < <(
  for dir in "${CONFIG_DIRS[@]}"; do
    go list -f '{{if or .TestGoFiles .XTestGoFiles}}{{.ImportPath}}{{end}}' "$dir" 2>/dev/null || true
  done | sort -u
)

if [[ ${#PKGS[@]} -eq 0 ]]; then
  echo "No test packages found"
  touch "$TEST_BIN_DIR/compiled_tests.txt"
  exit 0
fi

echo "Found ${#PKGS[@]} unique test packages to compile"
echo ""

# 检查包是否需要启用 race 检测
should_enable_race() {
  local pkg="$1"
  
  # 将包路径标准化用于比较
  local pkg_normalized="$(echo "$pkg" | sed 's|^github\.com/yaklang/yaklang/||' | sed 's|^\./||')"
  
  # 从缓存文件读取配置
  while IFS='|' read -r pattern race_enabled; do
    [[ "$race_enabled" != "true" ]] && continue
    
    local pattern_normalized="$(echo "$pattern" | sed 's|^\./||' | sed 's|/\.\.\.$||' | sed 's|/\.$||')"
    
    # 支持通配符匹配
    if [[ "$pattern" == *"..." ]]; then
      # 前缀匹配（递归）
      if [[ "$pkg_normalized" == "$pattern_normalized"* ]]; then
        return 0
      fi
    elif [[ "$pattern" == */. ]]; then
      # 精确包匹配（非递归）
      if [[ "$pkg_normalized" == "$pattern_normalized" ]]; then
        return 0
      fi
    else
      # 精确匹配
      if [[ "$pkg_normalized" == "$pattern_normalized" ]]; then
        return 0
      fi
    fi
  done < "$RACE_CONFIG_CACHE"
  
  return 1
}

compile_one() {
  local pkg="$1"
  
  # 将包路径转换为安全的文件名
  local safe="$(echo "$pkg" | sed 's|^\./||' | sed 's|github\.com/yaklang/yaklang/||' | sed 's|/|_|g')"
  local bin="$TEST_BIN_DIR/test_${safe}"
  local log="$TEST_BIN_DIR/test_${safe}_compile.log"
  local cache_key="$TEST_BIN_DIR/test_${safe}.cache"

  # 转换为相对路径用于 .package 文件
  local pkg_path="$pkg"
  if [[ "$pkg" != ./* ]]; then
    pkg_path="./$(echo "$pkg" | sed 's|github\.com/yaklang/yaklang/||')"
  fi

  # 检查是否需要重新编译 - 使用基于修改时间的缓存检查
  local need_compile=1
  if [[ -f "$bin" && -f "$cache_key" ]]; then
    # 策略1：快速检查 - 检查当前包目录和 go.mod 的修改时间
    local quick_check_passed=1
    local src_dir="${pkg_path%/...}"
    src_dir="${src_dir%/.}"
    
    # 检查 go.mod/go.sum（全局依赖）
    if [[ -f "go.mod" && "go.mod" -nt "$bin" ]]; then
      quick_check_passed=0
    elif [[ -f "go.sum" && "go.sum" -nt "$bin" ]]; then
      quick_check_passed=0
    # 检查当前包的源文件
    elif [[ -d "$src_dir" ]]; then
      local newest_src=$(find "$src_dir" -name "*.go" -type f -newer "$bin" 2>/dev/null | head -1)
      if [[ -n "$newest_src" ]]; then
        quick_check_passed=0
      fi
    fi
    
    # 如果快速检查通过，进行更深入的依赖检查
    if [[ $quick_check_passed -eq 1 ]]; then
      # 策略2：检查本地依赖（只检查本项目内的包）
      # 获取包的所有依赖，过滤出本项目内的包
      local deps=$(go list -f '{{range .Deps}}{{println .}}{{end}}' "$pkg" 2>/dev/null | \
                   grep "^github.com/yaklang/yaklang/" || true)
      
      local dep_changed=0
      if [[ -n "$deps" ]]; then
        # 对每个依赖，检查其源文件是否比二进制新
        while IFS= read -r dep_pkg; do
          local dep_dir="./$(echo "$dep_pkg" | sed 's|github\.com/yaklang/yaklang/||')"
          if [[ -d "$dep_dir" ]]; then
            local changed_dep=$(find "$dep_dir" -name "*.go" -type f -newer "$bin" 2>/dev/null | head -1)
            if [[ -n "$changed_dep" ]]; then
              dep_changed=1
              break
            fi
          fi
        done <<< "$deps"
      fi
      
      if [[ $dep_changed -eq 0 ]]; then
        need_compile=0
        echo "CACHED $pkg -> $(basename "$bin") [up to date, deps checked]"
        echo "$pkg_path" > "${bin}.package"
        return 0
      fi
    fi
  fi

  # 构建编译参数
  local compile_args=("-p=$JOBS" "-c" "-o" "$bin")
  local enable_race_for_pkg=0
  
  # 检查该包是否需要启用race检测
  if should_enable_race "$pkg"; then
    compile_args+=("-race")
    enable_race_for_pkg=1
  fi
  
  if go test "${compile_args[@]}" "$pkg" 2>"$log"; then
    if [[ $enable_race_for_pkg -eq 1 ]]; then
      echo "OK  $pkg -> $(basename "$bin") [race]"
    else
      echo "OK  $pkg -> $(basename "$bin")"
    fi
    echo "$pkg_path" > "${bin}.package"
    
    # 创建缓存标记文件（简单地记录编译时间戳）
    # 实际的缓存验证依赖于文件修改时间比较
    touch "$cache_key"
    
    rm -f "$log"
  else
    echo "FAIL $pkg"
    echo "$pkg" >> "$TEST_BIN_DIR/failed_packages.txt"
  fi
}

export -f compile_one
export -f should_enable_race
export TEST_BIN_DIR JOBS RACE_CONFIG_CACHE

echo "=== Compiling test packages ==="
printf '%s\0' "${PKGS[@]}" | xargs -0 -n1 -P "$JOBS" bash -c 'compile_one "$1"' _

echo ""
echo "=== Compile Summary ==="

# 统计
total=${#PKGS[@]}
failed_count=0
[[ -f "$TEST_BIN_DIR/failed_packages.txt" ]] && failed_count=$(wc -l < "$TEST_BIN_DIR/failed_packages.txt" | xargs)
compiled_count=$((total - failed_count))

echo "Total packages: $total"
echo "Compiled: $compiled_count"
echo "Failed: $failed_count"

# 记录编译的测试列表（兼容 macOS）
find "$TEST_BIN_DIR" -maxdepth 1 -type f -perm +111 2>/dev/null | sort > "$TEST_BIN_DIR/compiled_tests.txt" || \
  find "$TEST_BIN_DIR" -maxdepth 1 -type f -name "test_*" ! -name "*.log" ! -name "*.package" ! -name ".*" | sort > "$TEST_BIN_DIR/compiled_tests.txt"
echo "Compiled tests listed in: $TEST_BIN_DIR/compiled_tests.txt"

# 耗时
compile_end=$(date +%s)
compile_duration=$((compile_end - compile_start))
echo "Compilation took ${compile_duration}s"

if [[ $failed_count -gt 0 ]]; then
  echo ""
  echo "  Failed packages:"
  cat "$TEST_BIN_DIR/failed_packages.txt" | sed 's/^/  - /'
  exit 1
fi

echo ""
echo "All tests compiled successfully"

