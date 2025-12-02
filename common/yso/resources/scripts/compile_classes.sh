#!/bin/bash

# 编译 java_source/classes 目录下的所有 Java 文件到 static/classes 目录
# 这些 Java 文件属于 payload 包，编译后需要将 class 文件从 payload 子目录移动到 classes 根目录

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
BASE_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
SOURCE_DIR="$BASE_DIR/java_source/classes"
OUTPUT_DIR="$BASE_DIR/static/classes"
TEMP_DIR="$BASE_DIR/temp_compile"
LIBS_DIR="$BASE_DIR/libs"

# Tomcat 版本和下载地址
TOMCAT_VERSION="9.0.83"
TOMCAT_JAR_URL="https://repo1.maven.org/maven2/org/apache/tomcat/tomcat-coyote/${TOMCAT_VERSION}/tomcat-coyote-${TOMCAT_VERSION}.jar"
TOMCAT_UTIL_URL="https://repo1.maven.org/maven2/org/apache/tomcat/tomcat-util/${TOMCAT_VERSION}/tomcat-util-${TOMCAT_VERSION}.jar"

# 检查源目录是否存在
if [ ! -d "$SOURCE_DIR" ]; then
    echo "错误: 源目录不存在: $SOURCE_DIR"
    exit 1
fi

# 创建目录
rm -rf "$TEMP_DIR"
mkdir -p "$TEMP_DIR"
mkdir -p "$LIBS_DIR"
mkdir -p "$OUTPUT_DIR"

# 下载依赖
download_deps() {
    echo "检查并下载依赖..."
    
    if [ ! -f "$LIBS_DIR/tomcat-coyote-${TOMCAT_VERSION}.jar" ]; then
        echo "下载 tomcat-coyote..."
        curl -sL "$TOMCAT_JAR_URL" -o "$LIBS_DIR/tomcat-coyote-${TOMCAT_VERSION}.jar"
        if [ $? -ne 0 ]; then
            echo "警告: 无法下载 tomcat-coyote，某些文件可能无法编译"
        fi
    fi
    
    if [ ! -f "$LIBS_DIR/tomcat-util-${TOMCAT_VERSION}.jar" ]; then
        echo "下载 tomcat-util..."
        curl -sL "$TOMCAT_UTIL_URL" -o "$LIBS_DIR/tomcat-util-${TOMCAT_VERSION}.jar"
        if [ $? -ne 0 ]; then
            echo "警告: 无法下载 tomcat-util，某些文件可能无法编译"
        fi
    fi
    
    echo "依赖检查完成"
    echo ""
}

# 构建 classpath
build_classpath() {
    local cp=""
    for jar in "$LIBS_DIR"/*.jar; do
        if [ -f "$jar" ]; then
            if [ -z "$cp" ]; then
                cp="$jar"
            else
                cp="$cp:$jar"
            fi
        fi
    done
    echo "$cp"
}

# 查找所有 Java 文件
JAVA_FILES=$(find "$SOURCE_DIR" -name "*.java" -type f)

if [ -z "$JAVA_FILES" ]; then
    echo "错误: 没有找到 Java 文件"
    exit 1
fi

echo "找到以下 Java 文件:"
echo "$JAVA_FILES"
echo ""

# 下载依赖
download_deps

# 构建 classpath
CLASSPATH=$(build_classpath)
echo "Classpath: $CLASSPATH"
echo ""

# 检查 Java 版本
JAVA_VERSION=$(java -version 2>&1 | head -1 | cut -d'"' -f2 | cut -d'.' -f1)
echo "检测到 Java 主版本: $JAVA_VERSION"

# 根据 Java 版本设置编译参数
if [ "$JAVA_VERSION" -ge 9 ] 2>/dev/null; then
    JAVAC_OPTS="-XDignore.symbol.file"
    JAVAC_OPTS="$JAVAC_OPTS --add-exports=java.xml/com.sun.org.apache.xalan.internal.xsltc.runtime=ALL-UNNAMED"
    JAVAC_OPTS="$JAVAC_OPTS --add-exports=java.xml/com.sun.org.apache.xalan.internal.xsltc=ALL-UNNAMED"
    JAVAC_OPTS="$JAVAC_OPTS --add-exports=java.xml/com.sun.org.apache.xml.internal.dtm=ALL-UNNAMED"
    JAVAC_OPTS="$JAVAC_OPTS --add-exports=java.xml/com.sun.org.apache.xml.internal.serializer=ALL-UNNAMED"
else
    JAVAC_OPTS="-XDignore.symbol.file"
fi

# 添加 classpath
if [ -n "$CLASSPATH" ]; then
    JAVAC_OPTS="$JAVAC_OPTS -cp $CLASSPATH"
fi

echo "编译参数: $JAVAC_OPTS"
echo ""

# 统计
SUCCESS_COUNT=0
FAIL_COUNT=0
SUCCESS_FILES=""
FAIL_FILES=""

# 单独编译每个 Java 文件
echo "开始编译..."
echo "============================================"

for java_file in $JAVA_FILES; do
    filename=$(basename "$java_file" .java)
    echo -n "编译 $filename.java ... "
    
    # 清理临时目录
    rm -rf "$TEMP_DIR"/*
    
    # 编译单个文件
    javac $JAVAC_OPTS -d "$TEMP_DIR" "$java_file" 2>/dev/null
    
    if [ $? -eq 0 ]; then
        # 编译成功，移动 class 文件
        class_file="$TEMP_DIR/payload/$filename.class"
        if [ -f "$class_file" ]; then
            cp -f "$class_file" "$OUTPUT_DIR/$filename.class"
            echo "✓ 成功"
            SUCCESS_COUNT=$((SUCCESS_COUNT + 1))
            SUCCESS_FILES="$SUCCESS_FILES $filename"
        else
            # 检查是否在其他位置
            found_class=$(find "$TEMP_DIR" -name "$filename.class" -type f 2>/dev/null | head -1)
            if [ -n "$found_class" ]; then
                cp -f "$found_class" "$OUTPUT_DIR/$filename.class"
                echo "✓ 成功"
                SUCCESS_COUNT=$((SUCCESS_COUNT + 1))
                SUCCESS_FILES="$SUCCESS_FILES $filename"
            else
                echo "✗ 失败 (找不到 class 文件)"
                FAIL_COUNT=$((FAIL_COUNT + 1))
                FAIL_FILES="$FAIL_FILES $filename"
            fi
        fi
    else
        echo "✗ 失败 (编译错误)"
        FAIL_COUNT=$((FAIL_COUNT + 1))
        FAIL_FILES="$FAIL_FILES $filename"
    fi
done

echo "============================================"
echo ""

# 清理临时目录
rm -rf "$TEMP_DIR"

# 输出结果
echo "编译完成!"
echo "成功: $SUCCESS_COUNT 个文件"
echo "失败: $FAIL_COUNT 个文件"
echo ""

if [ -n "$SUCCESS_FILES" ]; then
    echo "成功编译的文件:$SUCCESS_FILES"
fi

if [ -n "$FAIL_FILES" ]; then
    echo "编译失败的文件:$FAIL_FILES"
    echo ""
    echo "提示: 失败的文件可能需要额外的依赖库"
fi

echo ""
echo "class 文件已保存到: $OUTPUT_DIR"
echo ""
echo "当前 class 文件列表:"
ls -la "$OUTPUT_DIR"/*.class 2>/dev/null || echo "没有找到 class 文件"
