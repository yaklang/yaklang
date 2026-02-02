#!/bin/bash
#
# get-yak-version.sh - 从 active_versions.txt 获取合适的 yak 版本
#
# 功能:
#   从版本列表中获取第一个不包含 -yakit- 和 -irify- 的版本（从新到旧遍历）
#   支持可选的额外模式匹配（正则表达式）
#
# 用法:
#   SELECTED_VERSION=$(scripts/get-yak-version.sh)
#   SELECTED_VERSION=$(scripts/get-yak-version.sh --pattern '.*-alpha.*-diff-check|.*-beta[0-9]+$')
#   SELECTED_VERSION=$(scripts/get-yak-version.sh --url "https://custom-url.com/versions.txt")
#   SELECTED_VERSION=$(scripts/get-yak-version.sh --file ./versions.txt)
#   SELECTED_VERSION=$(scripts/get-yak-version.sh --quiet)
#
# 选项:
#   --pattern PATTERN    可选的额外匹配模式（正则表达式）
#   --url URL           版本列表 URL（默认: https://aliyun-oss.yaklang.com/yak/version-info/active_versions.txt）
#   --file FILE         从本地文件读取版本列表（而不是从 URL）
#   --quiet             静默模式，只输出版本号，不输出日志
#   --help              显示帮助信息
#
# 返回:
#   成功时输出选中的版本号到 stdout，失败时返回非零退出码
#
# 使用场景和位置:
#   1. 基础模式（无 --pattern）:
#      - .github/workflows/update-syntaxflow-meta.yml
#        位置: 第 67 行
#        用途: 获取最新版本用于更新 SyntaxFlow 元数据
#
#   2. 静默模式（--quiet）:
#      - .github/workflows/exp-cross-build.yml
#        位置: 第 176 行
#        用途: 在交叉构建流程中静默获取版本，避免输出干扰
#
#   3. 模式匹配模式（--pattern）:
#      - .github/workflows/diff-code-check.yml
#        位置: 第 120 行
#        模式: '.*-alpha.*-diff-check|.*-alpha.*-code-scan|.*-beta[0-9]+$'
#        用途: 在代码差异检查中，优先选择 alpha-diff-check、alpha-code-scan 或 beta 版本
#
#   4. 本地文件模式（--file）:
#      - 主要用于测试和本地开发场景
#
# 注意:
#   - 所有模式都会自动排除包含 -yakit- 或 -irify- 的版本
#   - 版本列表按从新到旧的顺序遍历
#   - 返回第一个匹配的版本
#

set -euo pipefail

# 默认值
VERSIONS_URL="https://aliyun-oss.yaklang.com/yak/version-info/active_versions.txt"
PATTERN=""
VERSIONS_FILE=""
QUIET=false

# 解析参数
while [[ $# -gt 0 ]]; do
  case $1 in
    --pattern)
      PATTERN="$2"
      shift 2
      ;;
    --url)
      VERSIONS_URL="$2"
      shift 2
      ;;
    --file)
      VERSIONS_FILE="$2"
      shift 2
      ;;
    --quiet)
      QUIET=true
      shift
      ;;
    --help)
      cat <<EOF
从 active_versions.txt 获取合适的 yak 版本

功能:
  从版本列表中获取第一个不包含 -yakit- 和 -irify- 的版本（从新到旧遍历）
  支持可选的额外模式匹配（正则表达式）

用法:
  $0 [选项]

选项:
  --pattern PATTERN    可选的额外匹配模式（正则表达式）
  --url URL           版本列表 URL（默认: https://aliyun-oss.yaklang.com/yak/version-info/active_versions.txt）
  --file FILE         从本地文件读取版本列表（而不是从 URL）
  --quiet             静默模式，只输出版本号，不输出日志
  --help              显示帮助信息

示例:
  # 基础用法：只排除 -yakit- 和 -irify- 版本
  VERSION=\$($0)

  # 带模式匹配：匹配 alpha-diff-check 或 beta 版本
  VERSION=\$($0 --pattern '.*-alpha.*-diff-check|.*-beta[0-9]+\$')

  # 从本地文件读取
  VERSION=\$($0 --file ./versions.txt)

  # 静默模式
  VERSION=\$($0 --quiet)

使用场景和位置:
  1. 基础模式（无 --pattern）:
     - .github/workflows/update-syntaxflow-meta.yml (第 67 行)
       用途: 获取最新版本用于更新 SyntaxFlow 元数据

  2. 静默模式（--quiet）:
     - .github/workflows/exp-cross-build.yml (第 176 行)
       用途: 在交叉构建流程中静默获取版本

  3. 模式匹配模式（--pattern）:
     - .github/workflows/diff-code-check.yml (第 120 行)
       模式: '.*-alpha.*-diff-check|.*-alpha.*-code-scan|.*-beta[0-9]+\$'
       用途: 在代码差异检查中，优先选择特定类型的版本

  4. 本地文件模式（--file）:
     - 主要用于测试和本地开发场景

注意:
  - 所有模式都会自动排除包含 -yakit- 或 -irify- 的版本
  - 版本列表按从新到旧的顺序遍历
  - 返回第一个匹配的版本
EOF
      exit 0
      ;;
    *)
      echo "错误: 未知参数: $1" >&2
      echo "使用 --help 查看帮助信息" >&2
      exit 1
      ;;
  esac
done

# 获取版本列表
if [ -n "$VERSIONS_FILE" ]; then
  # 从本地文件读取
  if [ ! -f "$VERSIONS_FILE" ]; then
    if [ "$QUIET" = false ]; then
      echo "错误: 文件不存在: $VERSIONS_FILE" >&2
    fi
    exit 1
  fi
  VERSIONS=$(cat "$VERSIONS_FILE" | grep -v '^$' || true)
else
  # 从 URL 获取
  if [ "$QUIET" = false ]; then
    echo "正在从 $VERSIONS_URL 获取版本列表..." >&2
  fi
  
  VERSIONS=$(curl -sS -L "$VERSIONS_URL" 2>&1 | grep -v '^$' || true)
  
  if [ -z "$VERSIONS" ]; then
    if [ "$QUIET" = false ]; then
      echo "错误: 无法获取版本列表或列表为空" >&2
    fi
    exit 1
  fi
fi

# 从最新到最旧挨个匹配第一个非 -yakit- 和 -irify- 的标签
SELECTED_VERSION=""

while IFS= read -r version || [ -n "$version" ]; do
  # 清理版本字符串（去除回车、换行和前后空格）
  version=$(echo "$version" | tr -d '\r\n' | xargs)
  
  # 跳过空行
  if [ -z "$version" ]; then
    continue
  fi
  
  # 排除包含 -yakit- 或 -irify- 的版本
  if echo "$version" | grep -qE -- '-yakit-|-irify-'; then
    continue
  fi
  
  # 如果指定了额外模式，检查是否匹配
  if [ -n "$PATTERN" ]; then
    if ! echo "$version" | grep -qE "$PATTERN"; then
      continue
    fi
  fi
  
  # 找到匹配的版本
  SELECTED_VERSION="$version"
  if [ "$QUIET" = false ]; then
    echo "✅ 找到合适的版本: $SELECTED_VERSION" >&2
  fi
  break
done <<< "$VERSIONS"

# 检查是否找到版本
if [ -z "$SELECTED_VERSION" ]; then
  if [ "$QUIET" = false ]; then
    if [ -n "$PATTERN" ]; then
      echo "错误: 未找到匹配模式的版本（排除 -yakit- 和 -irify- 版本）" >&2
    else
      echo "错误: 未找到合适的版本（所有版本都包含 -yakit- 或 -irify-）" >&2
    fi
  fi
  exit 1
fi

# 输出选中的版本
echo "$SELECTED_VERSION"

