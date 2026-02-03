# 使用基础镜像，这里使用 Ubuntu 作为示例
FROM ubuntu:20.04

# 设置工作目录
WORKDIR /app

# 复制版本获取脚本
COPY scripts/get-yak-version.sh /get-yak-version

# Always fetch version from active_versions.txt (excluding -yakit- and -irify- versions)
# VERSION build arg is ignored, version is always determined from active_versions.txt
RUN apt-get update -y && apt-get install -y wget curl iputils-ping libpcap0.8=1.9.1-3 && \
    ARCH=$(if [ "$(uname -m)" = "aarch64" ]; then echo "arm64"; else echo "amd64"; fi) && \
    VERSION=$(bash /get-yak-version --quiet || echo "latest") && \
    wget -q -O /app/yak https://aliyun-oss.yaklang.com/yak/${VERSION}/yak_linux_${ARCH} && \
    chmod +x /app/yak && \
    /app/yak -v

# 设置容器启动时默认执行的命令
CMD ["/app/yak", "-v"]