#!/bin/bash
#
# test-get-yak-version.sh - 测试 get-yak-version.sh 脚本
#
# 用法:
#   ./scripts/test-get-yak-version.sh
#

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SCRIPT="$SCRIPT_DIR/get-yak-version.sh"

# 颜色输出
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# 测试计数器
TESTS_PASSED=0
TESTS_FAILED=0

# 兼容的计数器递增函数
increment_passed() {
  TESTS_PASSED=$((TESTS_PASSED + 1))
}

increment_failed() {
  TESTS_FAILED=$((TESTS_FAILED + 1))
}

# 测试函数
test_case() {
  local test_name="$1"
  local expected_result="$2"
  shift 2
  local cmd_args=("$@")
  
  echo -e "${YELLOW}测试: $test_name${NC}"
  echo "  命令: $SCRIPT ${cmd_args[*]}"
  
  local result
  local exit_code=0
  result=$("$SCRIPT" "${cmd_args[@]}" 2>&1) || exit_code=$?
  
  if [ "$exit_code" -eq 0 ]; then
    # 命令成功，检查结果
    if [ "$expected_result" = "ERROR" ]; then
      echo -e "${RED}  ✗ 失败: 期望错误但命令成功，输出: '$result'${NC}"
      increment_failed
      return 1
    elif [ -z "$expected_result" ] || echo "$result" | grep -q "^$expected_result$"; then
      echo -e "${GREEN}  ✓ 通过: $result${NC}"
      increment_passed
      return 0
    else
      echo -e "${RED}  ✗ 失败: 期望包含 '$expected_result', 得到 '$result'${NC}"
      increment_failed
      return 1
    fi
  else
    # 命令失败
    if [ "$expected_result" = "ERROR" ]; then
      echo -e "${GREEN}  ✓ 通过（预期错误）${NC}"
      increment_passed
      return 0
    else
      echo -e "${RED}  ✗ 失败: 命令返回非零退出码，输出: '$result'${NC}"
      increment_failed
      return 1
    fi
  fi
}

# 创建临时测试文件
TEMP_DIR=$(mktemp -d)
trap "rm -rf $TEMP_DIR" EXIT

# 测试文件 1: 包含各种版本的正常列表
cat > "$TEMP_DIR/versions1.txt" <<EOF
1.4.5-beta1
1.4.5-irify-beta1
1.4.4-alpha-diff-check
1.4.4-yakit-alpha1
1.4.3-beta2
1.4.3-irify-alpha1
1.4.2
EOF

# 测试文件 2: 所有版本都包含 -yakit- 或 -irify-
cat > "$TEMP_DIR/versions2.txt" <<EOF
1.4.5-yakit-beta1
1.4.5-irify-beta1
1.4.4-yakit-alpha1
1.4.4-irify-alpha1
EOF

# 测试文件 3: 包含模式匹配的版本
cat > "$TEMP_DIR/versions3.txt" <<EOF
1.4.5-alpha-diff-check
1.4.5-alpha-code-scan
1.4.5-beta1
1.4.5-irify-alpha-diff-check
1.4.4-beta2
EOF

echo "=========================================="
echo "开始测试 get-yak-version.sh"
echo "=========================================="
echo ""

# 测试 1: 基础模式 - 从本地文件读取，应该返回第一个不包含 -yakit- 或 -irify- 的版本
test_case \
  "基础模式：从本地文件读取，排除 -yakit- 和 -irify-" \
  "1.4.5-beta1" \
  --file "$TEMP_DIR/versions1.txt" \
  --quiet

# 测试 2: 模式匹配 - 匹配 beta 版本
test_case \
  "模式匹配：匹配 beta 版本" \
  "1.4.5-beta1" \
  --file "$TEMP_DIR/versions1.txt" \
  --pattern '.*-beta[0-9]+$' \
  --quiet

# 测试 3: 模式匹配 - 匹配 alpha-diff-check
test_case \
  "模式匹配：匹配 alpha-diff-check 版本" \
  "1.4.4-alpha-diff-check" \
  --file "$TEMP_DIR/versions1.txt" \
  --pattern '.*-alpha.*-diff-check' \
  --quiet

# 测试 4: 模式匹配 - 匹配 alpha-diff-check 或 alpha-code-scan（应该优先匹配第一个）
test_case \
  "模式匹配：匹配 alpha-diff-check 或 alpha-code-scan" \
  "1.4.5-alpha-diff-check" \
  --file "$TEMP_DIR/versions3.txt" \
  --pattern '.*-alpha.*-diff-check|.*-alpha.*-code-scan' \
  --quiet

# 测试 5: 静默模式（验证静默模式不输出日志）
test_case \
  "静默模式：不输出日志信息" \
  "1.4.5-beta1" \
  --file "$TEMP_DIR/versions1.txt" \
  --quiet

# 测试 6: 错误情况 - 所有版本都包含 -yakit- 或 -irify-
test_case \
  "错误情况：所有版本都包含 -yakit- 或 -irify-" \
  "ERROR" \
  --file "$TEMP_DIR/versions2.txt" \
  --quiet

# 测试 7: 错误情况 - 模式不匹配
test_case \
  "错误情况：模式不匹配任何版本" \
  "ERROR" \
  --file "$TEMP_DIR/versions1.txt" \
  --pattern '.*-nonexistent.*' \
  --quiet

# 测试 8: 错误情况 - 文件不存在
test_case \
  "错误情况：文件不存在" \
  "ERROR" \
  --file "$TEMP_DIR/nonexistent.txt" \
  --quiet

# 测试 9: 帮助信息
echo -e "${YELLOW}测试: 帮助信息${NC}"
if "$SCRIPT" --help | grep -q "从 active_versions.txt 获取合适的 yak 版本"; then
  echo -e "${GREEN}  ✓ 通过${NC}"
  increment_passed
else
  echo -e "${RED}  ✗ 失败${NC}"
  increment_failed
fi

# 测试 10: 从 URL 获取（如果网络可用，跳过如果失败）
echo -e "${YELLOW}测试: 从 URL 获取版本（可选，需要网络）${NC}"
if result=$("$SCRIPT" --quiet 2>/dev/null); then
  if [ -n "$result" ]; then
    echo -e "${GREEN}  ✓ 通过: 获取到版本 $result${NC}"
    increment_passed
  else
    echo -e "${RED}  ✗ 失败: 未获取到版本${NC}"
    increment_failed
  fi
else
  echo -e "${YELLOW}  ⚠ 跳过: 网络不可用或获取失败${NC}"
fi

echo ""
echo "=========================================="
echo "测试结果"
echo "=========================================="
echo -e "${GREEN}通过: $TESTS_PASSED${NC}"
if [ $TESTS_FAILED -gt 0 ]; then
  echo -e "${RED}失败: $TESTS_FAILED${NC}"
  exit 1
else
  echo -e "${GREEN}失败: $TESTS_FAILED${NC}"
  exit 0
fi

