#!/usr/bin/env bash
set -euo pipefail

# 可覆写：编译 worker 数、go test -p、输出目录、测试配置
DEFAULT_CPUS=$(getconf _NPROCESSORS_ONLN 2>/dev/null || sysctl -n hw.ncpu || echo 2)
COMPILE_WORKERS=${COMPILE_WORKERS:-${JOBS:-$DEFAULT_CPUS}}
GO_TEST_P="${GO_TEST_P:-}"
TEST_BIN_DIR="${TEST_BIN_DIR:-./test_binaries}"
TEST_CONFIG="${TEST_CONFIG:-}"  # 测试配置文件（JSON格式），必须提供
RESET_TEST_BIN_DIR="${RESET_TEST_BIN_DIR:-1}"  # 0=保留已有产物并增量编译

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
echo "COMPILE_WORKERS=$COMPILE_WORKERS"
if [[ -n "$GO_TEST_P" ]]; then
  echo "GO_TEST_P=$GO_TEST_P"
else
  echo "GO_TEST_P=<go default>"
fi
echo "BIN_DIR=$TEST_BIN_DIR"
echo "CONFIG=$TEST_CONFIG"
echo "RESET_TEST_BIN_DIR=$RESET_TEST_BIN_DIR"
echo ""

# 记录开始时间
compile_start=$(date +%s)

if [[ "$RESET_TEST_BIN_DIR" == "1" ]]; then
  rm -rf "$TEST_BIN_DIR"
  mkdir -p "$TEST_BIN_DIR"
else
  mkdir -p "$TEST_BIN_DIR"
  rm -f "$TEST_BIN_DIR/compiled_tests.txt" "$TEST_BIN_DIR/failed_packages.txt"
fi

# 创建临时文件存储 package -> race 映射（兼容 bash 3.x）
RACE_CONFIG_CACHE="$TEST_BIN_DIR/.race_config_cache"
jq -r '.[] | "\(.package)|\(.race // false)"' "$TEST_CONFIG" > "$RACE_CONFIG_CACHE"

# 获取唯一的包路径列表
CONFIG_DIRS=($(jq -r '.[].package' "$TEST_CONFIG" | sort -u))

echo "Found ${#CONFIG_DIRS[@]} unique package patterns in config"
echo ""

pkg_matches_pattern() {
  local pkg="$1"
  local pattern="$2"

  if [[ "$pkg" == "$pattern" ]]; then
    return 0
  fi

  if [[ "$pattern" == */... ]]; then
    local prefix="${pattern%/...}"
    if [[ "$pkg" == "$prefix" ]] || [[ "${pkg#"$prefix"/}" != "$pkg" ]]; then
      return 0
    fi
  fi

  if [[ "$pattern" == */. ]]; then
    local exact="${pattern%/.}"
    if [[ "$pkg" == "$exact" ]]; then
      return 0
    fi
  fi

  return 1
}

pkg_matches_any_exclude() {
  local pkg="$1"
  local exclude_patterns="$2"
  local pattern=""

  while IFS= read -r pattern; do
    [[ -z "$pattern" ]] && continue
    if pkg_matches_pattern "$pkg" "$pattern"; then
      return 0
    fi
  done <<< "$exclude_patterns"

  return 1
}

echo "=== Discovering test packages ==="

DISCOVERED_PKGS_FILE="$TEST_BIN_DIR/.discovered_packages"
: > "$DISCOVERED_PKGS_FILE"

config_count=$(jq 'length' "$TEST_CONFIG")
for ((idx=0; idx<config_count; idx++)); do
  pattern=$(jq -r ".[$idx].package" "$TEST_CONFIG")
  exclude_packages=$(jq -r ".[$idx].exclude_packages[]? // empty" "$TEST_CONFIG")

  while IFS= read -r pkg; do
    [[ -z "$pkg" ]] && continue

    pkg_path="./$(echo "$pkg" | sed 's|github.com/yaklang/yaklang/||')"
    if pkg_matches_any_exclude "$pkg_path" "$exclude_packages"; then
      continue
    fi

    echo "$pkg" >> "$DISCOVERED_PKGS_FILE"
  done < <(go list -f '{{if or .TestGoFiles .XTestGoFiles}}{{.ImportPath}}{{end}}' "$pattern" 2>/dev/null || true)
done

mapfile -t PKGS < <(sort -u "$DISCOVERED_PKGS_FILE")

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

  # 转换为相对路径用于 .package 文件
  local pkg_path="$pkg"
  if [[ "$pkg" != ./* ]]; then
    pkg_path="./$(echo "$pkg" | sed 's|github\.com/yaklang/yaklang/||')"
  fi

  # 构建编译参数
  local compile_args=("-c" "-o" "$bin")
  local enable_race_for_pkg=0

  if [[ -n "$GO_TEST_P" ]]; then
    compile_args=("-p=$GO_TEST_P" "${compile_args[@]}")
  fi
  
  # 检查该包是否需要启用race检测
  if should_enable_race "$pkg"; then
    compile_args+=("-race")
    enable_race_for_pkg=1
  fi
  
  # 使用 build cache 加速编译
  # GOCACHE 环境变量应该已经设置，go test 会自动使用
  if go test "${compile_args[@]}" "$pkg" 2>"$log"; then
    if [[ $enable_race_for_pkg -eq 1 ]]; then
      echo "OK  $pkg -> $(basename "$bin") [race]"
    else
      echo "OK  $pkg -> $(basename "$bin")"
    fi
    echo "$pkg_path" > "${bin}.package"
    rm -f "$log"
  else
    echo "FAIL $pkg"
    echo "$pkg" >> "$TEST_BIN_DIR/failed_packages.txt"
  fi
}

export -f compile_one
export -f should_enable_race
export TEST_BIN_DIR GO_TEST_P RACE_CONFIG_CACHE

echo "=== Compiling test packages ==="
printf '%s\0' "${PKGS[@]}" | xargs -0 -n1 -P "$COMPILE_WORKERS" bash -c 'compile_one "$1"' _

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

# 记录编译的测试列表：只保存 TEST_BIN_DIR 下的相对文件名，避免 artifact 解包后路径失效
(
  cd "$TEST_BIN_DIR"
  find . -maxdepth 1 -type f -name "test_*" ! -name "*.log" ! -name "*.package" ! -name ".*" \
    -exec basename {} \; | sort
) > "$TEST_BIN_DIR/compiled_tests.txt"
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
