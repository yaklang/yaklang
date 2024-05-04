# 使用基础镜像，这里使用 Ubuntu 作为示例
FROM ubuntu:20.04

# 设置工作目录
WORKDIR /app


ARG VERSION=latest
# Install Wget and download from https://aliyun-oss.yaklang.com/yak/${VERSION}/yak_linux_${ARCH}
RUN apt-get update -y && apt-get install -y wget curl iputils-ping libpcap0.8=1.9.1-3 && \
    wget -q -O /app/yak https://aliyun-oss.yaklang.com/yak/${VERSION}/yak_linux_$(if [ "$(uname -m)" = "aarch64" ]; then echo "arm64"; else echo "amd64"; fi) && \
    mkdir -p /app/ && chmod +x /app/yak && \
    cd /app/ && ./yak -v

# 设置容器启动时默认执行的命令
CMD ["/app/yak", "-v"]