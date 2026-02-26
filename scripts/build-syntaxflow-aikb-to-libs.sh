#!/usr/bin/env bash
# 构建 syntaxflow-aikb.rag 与 syntaxflow-aikb.zip 到 thirdparty 安装路径
# 用法：在 yaklang 仓库根目录执行 ./scripts/build-syntaxflow-aikb-to-libs.sh
# 输出路径：${YAKIT_HOME:-$HOME/yakit-projects}/projects/libs/
set -e

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$REPO_ROOT"

YAKIT_HOME="${YAKIT_HOME:-$HOME/yakit-projects}"
LIBS_DIR="$YAKIT_HOME/projects/libs"
mkdir -p "$LIBS_DIR"

SF_BASE="syntaxflow-aikb"
if [[ ! -d "$SF_BASE" ]]; then
  echo "Error: $SF_BASE not found, run from yaklang repo root" >&2
  exit 1
fi

if ! command -v yak >/dev/null 2>&1; then
  echo "Error: yak not in PATH" >&2
  exit 1
fi

echo "[build] Install dir: $LIBS_DIR"
echo "[build] Building syntaxflow-aikb.zip..."
yak "$SF_BASE/scripts/merge-in-one-text.yak" --base "$SF_BASE" --output "$LIBS_DIR/syntaxflow-aikb.zip"

echo "[build] Building syntaxflow-aikb.rag..."
yak "$SF_BASE/scripts/build-syntaxflow-aikb-rag.yak" --base "$SF_BASE" --output "$LIBS_DIR/syntaxflow-aikb.rag"

echo "[build] Done:"
echo "  - $LIBS_DIR/syntaxflow-aikb.zip"
echo "  - $LIBS_DIR/syntaxflow-aikb.rag"
