# 使用基础镜像，这里使用 Ubuntu 作为示例
FROM ubuntu:20.04

# 设置工作目录
WORKDIR /app


# Always fetch version from active_versions.txt (excluding -yakit- and -irify- versions)
# VERSION build arg is ignored, version is always determined from active_versions.txt
# Note: This logic matches scripts/get-yak-version.sh (from latest to oldest, exclude -yakit- and -irify-)
RUN apt-get update -y && apt-get install -y wget curl iputils-ping libpcap0.8=1.9.1-3 && \
    mkdir -p /app/ && \
    ARCH=$(if [ "$(uname -m)" = "aarch64" ]; then echo "arm64"; else echo "amd64"; fi) && \
    echo "Fetching version from active_versions.txt..." && \
    VERSIONS_FILE=$(mktemp) && \
    if curl -sS -L https://aliyun-oss.yaklang.com/yak/version-info/active_versions.txt -o "$VERSIONS_FILE" 2>/dev/null && [ -s "$VERSIONS_FILE" ]; then \
        SELECTED_VERSION="" && \
        while IFS= read -r version || [ -n "$version" ]; do \
            version=$(echo "$version" | tr -d '\r\n' | xargs) && \
            if [ -n "$version" ] && ! echo "$version" | grep -qE -- '-yakit-|-irify-'; then \
                SELECTED_VERSION="$version" && \
                echo "✅ Found suitable version: $SELECTED_VERSION" && \
                break; \
            fi; \
        done < "$VERSIONS_FILE" && \
        rm -f "$VERSIONS_FILE" && \
        if [ -n "$SELECTED_VERSION" ]; then \
            VERSION="$SELECTED_VERSION"; \
        else \
            echo "⚠️ No suitable version found, using 'latest'" && \
            VERSION="latest"; \
        fi; \
    else \
        rm -f "$VERSIONS_FILE" && \
        echo "⚠️ Failed to fetch active_versions.txt, using 'latest'" && \
        VERSION="latest"; \
    fi && \
    echo "Using version: $VERSION" && \
    wget -q -O /app/yak https://aliyun-oss.yaklang.com/yak/${VERSION}/yak_linux_${ARCH} && \
    if [ ! -f /app/yak ] || [ ! -s /app/yak ]; then \
        echo "ERROR: Failed to download yak binary" && exit 1; \
    fi && \
    chmod +x /app/yak && \
    cd /app/ && ./yak -v

# 设置容器启动时默认执行的命令
CMD ["/app/yak", "-v"]