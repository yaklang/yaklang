#!/bin/bash

# 综合构建脚本
# 按顺序执行: 编译 classes -> 打补丁 -> 生成序列化文件
#
# 用法: ./build_all.sh --java8 <java8_home>
#   --java8 <path>  必须指定 Java 8 的 JAVA_HOME 路径
#
# 示例: ./build_all.sh --java8 /Library/Java/JavaVirtualMachines/zulu-8.jdk/Contents/Home/

set -e  # 遇到错误立即退出

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

# 颜色输出
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

print_step() {
    echo ""
    echo -e "${BLUE}============================================${NC}"
    echo -e "${BLUE}  $1${NC}"
    echo -e "${BLUE}============================================${NC}"
    echo ""
}

print_success() {
    echo -e "${GREEN}✓ $1${NC}"
}

print_error() {
    echo -e "${RED}✗ $1${NC}"
}

print_warning() {
    echo -e "${YELLOW}⚠ $1${NC}"
}

# 解析命令行参数
JAVA8_HOME=""
while [[ $# -gt 0 ]]; do
    case $1 in
        --java8)
            if [ -n "$2" ] && [ -d "$2" ]; then
                JAVA8_HOME="$2"
                shift 2
            else
                print_error "错误: 请指定有效的 Java 8 路径"
                exit 1
            fi
            ;;
        -h|--help)
            echo "用法: $0 --java8 <java8_home>"
            echo ""
            echo "参数:"
            echo "  --java8 <path>  必须指定 Java 8 的 JAVA_HOME 路径"
            echo ""
            echo "示例:"
            echo "  $0 --java8 /Library/Java/JavaVirtualMachines/zulu-8.jdk/Contents/Home/"
            exit 0
            ;;
        *)
            shift
            ;;
    esac
done

# 检查 Java 8 路径
if [ -z "$JAVA8_HOME" ]; then
    print_error "错误: 必须指定 Java 8 路径"
    echo ""
    echo "用法: $0 --java8 <java8_home>"
    echo ""
    echo "示例:"
    echo "  $0 --java8 /Library/Java/JavaVirtualMachines/zulu-8.jdk/Contents/Home/"
    exit 1
fi

# 验证 Java 8
if [ ! -f "$JAVA8_HOME/bin/java" ]; then
    print_error "错误: 无效的 Java 8 路径: $JAVA8_HOME"
    exit 1
fi

JAVA8_VERSION=$("$JAVA8_HOME/bin/java" -version 2>&1 | head -1)
echo "Java 8 路径: $JAVA8_HOME"
echo "Java 版本: $JAVA8_VERSION"

echo ""
echo -e "${GREEN}============================================${NC}"
echo -e "${GREEN}  YSO 资源综合构建脚本${NC}"
echo -e "${GREEN}============================================${NC}"
echo ""
echo "将按顺序执行以下步骤:"
echo "  1. 编译 Java classes"
echo "  2. 打补丁生成 patched jar"
echo "  3. 生成序列化文件 (.ser)"
echo ""

# Step 1: 编译 classes
print_step "Step 1/3: 编译 Java Classes"
if [ -f "$SCRIPT_DIR/compile_classes.sh" ]; then
    "$SCRIPT_DIR/compile_classes.sh"
    print_success "Classes 编译完成"
else
    print_warning "跳过: compile_classes.sh 不存在"
fi

# Step 2: 打补丁
print_step "Step 2/3: 打补丁生成 Patched Jar"
if [ -f "$SCRIPT_DIR/patch_jar.sh" ]; then
    "$SCRIPT_DIR/patch_jar.sh"
    print_success "Patched jar 生成完成"
else
    print_warning "跳过: patch_jar.sh 不存在"
fi

# Step 3: 生成序列化文件
print_step "Step 3/3: 生成序列化文件"
if [ -f "$SCRIPT_DIR/compile_sers.sh" ]; then
    "$SCRIPT_DIR/compile_sers.sh" --java8 "$JAVA8_HOME"
    print_success "序列化文件生成完成"
else
    print_error "错误: compile_sers.sh 不存在"
    exit 1
fi

# 完成
echo ""
echo -e "${GREEN}============================================${NC}"
echo -e "${GREEN}  构建完成!${NC}"
echo -e "${GREEN}============================================${NC}"
echo ""
echo "生成的资源:"
echo "  - Classes: common/yso/resources/static/classes/"
echo "  - Patched jar: common/yso/resources/libs/ysoserial-for-woodpecker-0.5.2-patched.jar"
echo "  - 序列化文件: common/yso/resources/static/gadgets/"
echo ""

