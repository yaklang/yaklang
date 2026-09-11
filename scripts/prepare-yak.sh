#!/bin/bash
#
# prepare-yak.sh - 准备一个可用的 yak 二进制
#
# 策略:
#   1. 从 active_versions 列表取最新发布版本（不是 latest 别名），下载对应二进制
#   2. 检查下载的二进制是否支持所需 CLI 参数（可配置）
#   3. 支持则直接使用；不支持或下载失败则从当前 checkout go build 一个
#
# 用法:
#   YAK="$(bash scripts/prepare-yak.sh --out /tmp/yak)"
#   YAK="$(bash scripts/prepare-yak.sh --out /tmp/yak \
#         --require "verify-builtin-risk --dir")"
#
# 选项:
#   --out PATH          yak 输出路径（默认 $RUNNER_TEMP/yak 或 /tmp/yak）
#   --require "CMD FLAG" 必须支持的参数探测；可重复。默认探测:
#                          embed-fs-hash --output-file
#                          syntaxflow-format --rule-version-output
#   --quiet             只输出版本/路径到 stdout，日志走 stderr
#
# 成功时只输出最终的 yak 路径到 stdout，方便 CI 直接捕获:
#   echo "YAK=$(bash scripts/prepare-yak.sh --out ...)" >> $GITHUB_ENV

set -euo pipefail

OUTPUT="${YAK_OUTPUT:-}"
QUIET=0
REQUIRES=()

log() {
  if [ "$QUIET" != "1" ]; then
    echo "prepare-yak.sh: $*" >&2
  fi
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --out)
      OUTPUT="$2"
      shift 2
      ;;
    --require)
      REQUIRES+=("$2")
      shift 2
      ;;
    --quiet)
      QUIET=1
      shift
      ;;
    *)
      echo "prepare-yak.sh: unknown argument: $1" >&2
      exit 1
      ;;
  esac
done

if [ -z "$OUTPUT" ]; then
  OUTPUT="${RUNNER_TEMP:-/tmp}/yak"
fi
mkdir -p "$(dirname "$OUTPUT")"

# 默认探测 embed hash 同步链路需要的参数
if [ "${#REQUIRES[@]}" -eq 0 ]; then
  REQUIRES=(
    "embed-fs-hash --output-file"
    "syntaxflow-format --rule-version-output"
  )
fi

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

# 1. 从 active_versions 列表取最新发布版本（与 diff-code-check 相同的版本源）。
#    跳过 dev/ 构建：它们不是发布版，且可能缺少正式版才有的 CLI 参数。
VERSION=""
if VERSION=$(bash "$SCRIPT_DIR/get-yak-version.sh" --quiet --pattern '^[^/]+$' 2>/dev/null) && [ -n "$VERSION" ]; then
  log "newest active version: $VERSION"
else
  log "failed to resolve newest active version from active_versions list"
fi

# 2. 平台/架构 + 下载 URL（与 diff-code-check 保持一致）
OS="linux"
ARCH="amd64"
case "$(uname -s)" in
  Linux*)   OS="linux" ;;
  Darwin*)  OS="darwin" ;;
  MINGW*|MSYS*|CYGWIN*) OS="windows" ;;
  *)        log "unsupported OS: $(uname -s)" ;;
esac
case "$(uname -m)" in
  x86_64|amd64) ARCH="amd64" ;;
  aarch64|arm64) ARCH="arm64" ;;
esac
BINARY_NAME="yak_${OS}_${ARCH}"
if [ "$OS" = "windows" ]; then
  BINARY_NAME="${BINARY_NAME}.exe"
fi

# 检查二进制是否满足所有 --require 参数
binary_satisfies() {
  local dest="$1" req cmd flag help_file
  if ! "$dest" version >/dev/null 2>&1; then
    return 1
  fi
  help_file="$(mktemp)"
  for req in "${REQUIRES[@]}"; do
    cmd="${req%% *}"
    flag="${req#* }"
    # 先把 help 写入文件再 grep，避免 stdout 管道被提前关闭时 yak 收到
    # SIGPIPE（141），导致已发布的二进制被误判为缺少所需参数。
    if ! "$dest" "$cmd" --help >"$help_file" 2>&1 || ! grep -q -- "$flag" "$help_file"; then
      rm -f "$help_file"
      return 1
    fi
  done
  rm -f "$help_file"
  return 0
}

# 3. 优先使用列表最新发布版
if [ -n "$VERSION" ]; then
  DOWNLOAD_URL="https://aliyun-oss.yaklang.com/yak/${VERSION}/${BINARY_NAME}"
  log "downloading $DOWNLOAD_URL"
  if curl -fsSL "$DOWNLOAD_URL" -o "$OUTPUT" && binary_satisfies "$OUTPUT"; then
    log "using published yak $VERSION ($BINARY_NAME)"
    echo "$OUTPUT"
    exit 0
  fi
  log "published yak $VERSION missing required flags or failed verification"
fi

# 4. 下载不可用（缺参数/下载失败）时，从当前 checkout 编译
log "building yak from current checkout"
go build -o "$OUTPUT" ./common/yak/cmd/yak.go
if ! "$OUTPUT" version >/dev/null 2>&1; then
  echo "prepare-yak.sh: built yak failed version check: $OUTPUT" >&2
  exit 1
fi
echo "$OUTPUT"
