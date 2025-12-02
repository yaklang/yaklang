#!/bin/bash

# 编译 Start.java 并运行生成序列化文件
# 依赖: ysoserial-for-woodpecker-0.5.2.jar, shiro-core-1.1.0.jar
# 
# 用法: ./compile_sers.sh [--java8 <java8_home>]
#   --java8 <path>  指定 Java 8 的 JAVA_HOME 路径（推荐用于 template 类型 gadget）
#
# 注意: template 类型的 gadget 在 Java 9+ 下可能无法正常生成，建议使用 Java 8

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
BASE_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
LIBS_DIR="$BASE_DIR/libs"
OUTPUT_DIR="$BASE_DIR/static/gadgets"
CONFIG_FILE="$BASE_DIR/static/config.yaml"
TEMP_DIR="$BASE_DIR/temp_compile_sers"

# 依赖 JAR 文件
# 优先使用 patched jar（如果存在），否则使用原始 jar
YSOSERIAL_JAR_PATCHED="$LIBS_DIR/ysoserial-for-woodpecker-0.5.2-patched.jar"
YSOSERIAL_JAR_ORIGINAL="$LIBS_DIR/ysoserial-for-woodpecker-0.5.2.jar"
if [ -f "$YSOSERIAL_JAR_PATCHED" ]; then
    YSOSERIAL_JAR="$YSOSERIAL_JAR_PATCHED"
    echo "使用 patched jar: $YSOSERIAL_JAR_PATCHED"
else
    YSOSERIAL_JAR="$YSOSERIAL_JAR_ORIGINAL"
fi
SHIRO_JAR="$LIBS_DIR/shiro-core-1.1.0.jar"

# 解析命令行参数
JAVA_CMD="java"
JAVAC_CMD="javac"
while [[ $# -gt 0 ]]; do
    case $1 in
        --java8)
            if [ -n "$2" ] && [ -d "$2" ]; then
                JAVA8_HOME="$2"
                JAVA_CMD="$JAVA8_HOME/bin/java"
                JAVAC_CMD="$JAVA8_HOME/bin/javac"
                echo "使用 Java 8: $JAVA8_HOME"
            else
                echo "错误: 请指定有效的 Java 8 路径"
                exit 1
            fi
            shift 2
            ;;
        *)
            shift
            ;;
    esac
done

echo "============================================"
echo "序列化文件生成工具"
echo "============================================"
echo ""

# 检查依赖
check_deps() {
    echo "检查依赖..."
    
    if [ ! -f "$YSOSERIAL_JAR" ]; then
        echo "错误: 未找到 ysoserial-for-woodpecker-0.5.2.jar"
        echo "请将 jar 文件放置到: $LIBS_DIR"
        exit 1
    fi
    
    if [ ! -f "$SHIRO_JAR" ]; then
        echo "错误: 未找到 shiro-core-1.1.0.jar"
        echo "请将 jar 文件放置到: $LIBS_DIR"
        exit 1
    fi
    
    echo "依赖检查通过"
    echo ""
}

# 构建 classpath
build_classpath() {
    local cp="$YSOSERIAL_JAR:$SHIRO_JAR"
    # 添加其他 jar 文件
    for jar in "$LIBS_DIR"/*.jar; do
        if [ -f "$jar" ] && [ "$jar" != "$YSOSERIAL_JAR" ] && [ "$jar" != "$SHIRO_JAR" ]; then
            cp="$cp:$jar"
        fi
    done
    echo "$cp"
}

# 检查依赖
check_deps

# 创建目录
rm -rf "$TEMP_DIR"
mkdir -p "$TEMP_DIR"
mkdir -p "$OUTPUT_DIR"

# 构建 classpath
CLASSPATH=$(build_classpath)
echo "Classpath: $CLASSPATH"
echo ""

# 检查 Java 版本
JAVA_VERSION=$($JAVA_CMD -version 2>&1 | head -1 | cut -d'"' -f2 | cut -d'.' -f1)
echo "检测到 Java 主版本: $JAVA_VERSION"

# 设置编译参数
JAVAC_OPTS=""
JAVA_OPTS=""

if [ "$JAVA_VERSION" -ge 9 ] 2>/dev/null; then
    echo ""
    echo "⚠️  警告: 检测到 Java 9+，template 类型的 gadget 可能无法正常生成"
    echo "   建议使用 Java 8: ./compile_sers.sh --java8 /path/to/java8"
    echo ""
    
    # Java 9+ 需要模块导出参数
    MODULE_OPTS="--add-opens=java.base/java.lang=ALL-UNNAMED"
    MODULE_OPTS="$MODULE_OPTS --add-opens=java.base/java.util=ALL-UNNAMED"
    MODULE_OPTS="$MODULE_OPTS --add-opens=java.base/java.lang.reflect=ALL-UNNAMED"
    MODULE_OPTS="$MODULE_OPTS --add-opens=java.base/java.text=ALL-UNNAMED"
    MODULE_OPTS="$MODULE_OPTS --add-opens=java.base/java.io=ALL-UNNAMED"
    MODULE_OPTS="$MODULE_OPTS --add-opens=java.base/java.net=ALL-UNNAMED"
    MODULE_OPTS="$MODULE_OPTS --add-opens=java.base/java.nio=ALL-UNNAMED"
    MODULE_OPTS="$MODULE_OPTS --add-opens=java.base/sun.reflect.annotation=ALL-UNNAMED"
    MODULE_OPTS="$MODULE_OPTS --add-opens=java.desktop/java.awt.font=ALL-UNNAMED"
    MODULE_OPTS="$MODULE_OPTS --add-opens=java.rmi/sun.rmi.transport=ALL-UNNAMED"
    MODULE_OPTS="$MODULE_OPTS --add-opens=java.rmi/sun.rmi.server=ALL-UNNAMED"
    # 设置 Javassist 使用系统类加载器
    JAVA_OPTS="$MODULE_OPTS -Djavassist.useSystemClassLoader=true"
fi

echo ""

# 复制 Start.java 到临时目录
cp "$SCRIPT_DIR/Start.java" "$TEMP_DIR/"

# 编译 Start.java
echo "编译 Start.java..."
cd "$TEMP_DIR"
$JAVAC_CMD $JAVAC_OPTS -cp "$CLASSPATH" Start.java

if [ $? -ne 0 ]; then
    echo "编译失败!"
    rm -rf "$TEMP_DIR"
    exit 1
fi

echo "编译成功!"
echo ""

# 运行 Start
echo "生成序列化文件..."
echo "============================================"
$JAVA_CMD $JAVA_OPTS -cp ".:$CLASSPATH" Start --config "$CONFIG_FILE" --output "$OUTPUT_DIR"
RESULT=$?
echo "============================================"
echo ""

# 清理临时目录
rm -rf "$TEMP_DIR"

if [ $RESULT -eq 0 ]; then
    echo "序列化文件已保存到: $OUTPUT_DIR"
    echo ""
    echo "生成的文件数量: $(ls -1 "$OUTPUT_DIR"/*.ser 2>/dev/null | wc -l | tr -d ' ')"
else
    echo "生成过程中出现错误"
    exit 1
fi

