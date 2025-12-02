#!/bin/bash

# 编译修改后的 CommonsCollectionsUtil.java 并生成新的 patched jar 包
# 不修改原始 jar 文件

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
BASE_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
LIBS_DIR="$BASE_DIR/libs"
PATCH_DIR="$SCRIPT_DIR/patch"
TEMP_DIR="$BASE_DIR/temp_patch"

YSOSERIAL_JAR="$LIBS_DIR/ysoserial-for-woodpecker-0.5.2.jar"
PATCHED_JAR="$LIBS_DIR/ysoserial-for-woodpecker-0.5.2-patched.jar"

echo "============================================"
echo "Jar 包补丁工具 - 添加 mozilla_defining_class_loader 支持"
echo "============================================"
echo ""

# 检查原始 jar 文件
if [ ! -f "$YSOSERIAL_JAR" ]; then
    echo "错误: 未找到原始 jar: $YSOSERIAL_JAR"
    exit 1
fi

# 检查源文件
if [ ! -f "$PATCH_DIR/CommonsCollectionsUtil.java" ]; then
    echo "错误: 未找到 $PATCH_DIR/CommonsCollectionsUtil.java"
    exit 1
fi

# 创建临时目录
rm -rf "$TEMP_DIR"
mkdir -p "$TEMP_DIR"

echo "编译 CommonsCollectionsUtil.java (Java 8 兼容)..."

# 编译 (使用 -source 8 -target 8 确保兼容性)
javac -source 8 -target 8 -cp "$YSOSERIAL_JAR" -d "$TEMP_DIR" "$PATCH_DIR/CommonsCollectionsUtil.java" 2>&1

if [ $? -ne 0 ]; then
    echo "编译失败!"
    rm -rf "$TEMP_DIR"
    exit 1
fi

echo "编译成功!"
echo ""

# 复制原始 jar 到新文件
echo "创建 patched jar 文件..."
cp "$YSOSERIAL_JAR" "$PATCHED_JAR"

# 更新 patched jar 包
echo "更新 patched jar 包..."
cd "$TEMP_DIR"
jar uf "$PATCHED_JAR" me/gv7/woodpecker/yso/payloads/custom/CommonsCollectionsUtil.class

if [ $? -ne 0 ]; then
    echo "更新 jar 失败!"
    rm -rf "$TEMP_DIR"
    exit 1
fi

echo "更新成功!"
echo ""

# 验证
echo "验证更新..."
unzip -l "$PATCHED_JAR" | grep "CommonsCollectionsUtil.class"

# 清理
rm -rf "$TEMP_DIR"

echo ""
echo "============================================"
echo "完成!"
echo "原始 jar (未修改): $YSOSERIAL_JAR"
echo "Patched jar: $PATCHED_JAR"
echo "============================================"
