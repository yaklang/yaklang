#!/bin/bash
set -euo pipefail
# gorm v1 -> v2 机械迁移脚本：import 路径、方言导入、struct tag 改名
# 使用说明：
#   cd /home/wlz/Developer/yaklang-workspace/refactor-upgrade-gorm
#   ./scripts/migrate-gorm-imports.sh
# 执行后必须人工 review diff，尤其是 unique_index/primary_key/columns 是否误伤非 tag 字符串。

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

if ! command -v rg >/dev/null 2>&1; then
  echo "requires ripgrep (rg)"
  exit 1
fi

GO_FILES=$(rg -l 'github.com/jinzhu/gorm' --glob '*.go' || true)
if [ -z "$GO_FILES" ]; then
  echo "no files contain github.com/jinzhu/gorm import"
fi

echo "[1/4] replace gorm import path"
echo "$GO_FILES" | xargs -r sed -i 's|github.com/jinzhu/gorm|gorm.io/gorm|g'

echo "[2/4] replace dialect imports"
for pair in \
  'github.com/jinzhu/gorm/dialects/sqlite|gorm.io/driver/sqlite' \
  'github.com/jinzhu/gorm/dialects/mysql|gorm.io/driver/mysql' \
  'github.com/jinzhu/gorm/dialects/postgres|gorm.io/driver/postgres'; do
  old=${pair%|*}
  new=${pair#*|}
  files=$(rg -l "$old" --glob '*.go' || true)
  if [ -n "$files" ]; then
    echo "    $old -> $new"
    echo "$files" | xargs -r sed -i "s|$old|$new|g"
  fi
done

echo "[3/4] rename struct tags"
# 只在 gorm:"..." 内部出现的 tag 关键字做替换
rg -l 'gorm:"[^"]*unique_index' --glob '*.go' | xargs -r sed -i 's|unique_index|uniqueIndex|g'
rg -l 'gorm:"[^"]*primary_key' --glob '*.go' | xargs -r sed -i 's|primary_key|primaryKey|g'
rg -l 'gorm:"[^"]*columns:' --glob '*.go' | xargs -r sed -i 's|columns:|column:|g'

echo "[4/4] gofmt modified files"
MODIFIED=$(git diff --name-only -- '*.go' 2>/dev/null || true)
if [ -n "$MODIFIED" ]; then
  echo "$MODIFIED" | xargs -r gofmt -w
fi

echo "done. review with: git diff -- '*.go'"
