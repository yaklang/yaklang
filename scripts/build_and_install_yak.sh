#!/usr/bin/env bash
# 用法：在仓库根目录执行 ./scripts/build_and_install_yak.sh
# 依赖：若 go build 报缺包，可先执行 go mod download 或 go mod tidy
set -e

# 在仓库根目录执行（脚本在 scripts/ 下）
REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$REPO_ROOT"

# ---------- 1. 使用当前检出的工具，避免 PATH 中的旧版本 ----------
GZIP_EMBED="$(bash scripts/build-gzip-embed.sh)"
export GZIP_EMBED

# ---------- 2. 生成 gzip_embed 所需的 .tar.gz 资源 ----------
echo "[build] generating gzip embed resources..."
bash scripts/generate-compressed-assets.sh
"$GZIP_EMBED" -cache --source ./common/ai/aid/aitool/buildinaitools/yakscripttools/yakscriptforai --gz ./common/ai/aid/aitool/buildinaitools/yakscripttools/yakscriptforai.tar.gz --no-embed
"$GZIP_EMBED" -cache --source ./common/ai/aid/aireact/skills --gz ./common/ai/aid/aireact/skills.tar.gz --root-path --no-embed
"$GZIP_EMBED" -cache --source ./common/coreplugin/base-yak-plugin --gz ./common/coreplugin/base-yak-plugin.tar.gz --root-path --no-embed
"$GZIP_EMBED" -cache --source ./common/syntaxflow/sfbuildin/buildin --gz ./common/syntaxflow/sfbuildin/buildin.tar.gz --no-embed
"$GZIP_EMBED" -cache --source ./common/aiforge/buildinforge --gz ./common/aiforge/buildinforge.tar.gz --no-embed

# ---------- 3. 编译（-tags gzip_embed 会编入 //go:build gzip_embed 的代码） ----------
echo "[build] building yak..."
go build -tags gzip_embed -ldflags "-X 'main.goVersion=$(go version)' -X 'main.gitHash=$(git show -s --format=%H)' -X 'main.buildTime=$(git show -s --format=%cd)' -X 'main.yakVersion=$(git describe --tag)'" -o "yak$(go env GOEXE)" common/yak/cmd/yak.go

# ---------- 4. 安装到系统（可选，不需要就注释掉下面两行） ----------
# 若未安装到 /usr/local/bin，可直接用当前目录的 ./yak 运行
# sudo mv ./yak /usr/local/bin
# echo "[build] yak installed to /usr/local/bin"

# 若只想编译不安装，可注释掉上面两行，并取消下面注释：
# echo "[build] binary: $REPO_ROOT/yak"
