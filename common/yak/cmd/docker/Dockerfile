# 使用基础镜像，这里使用 Ubuntu 作为示例
FROM ubuntu:20.04

# 设置工作目录
WORKDIR /app

# 定义构建时的架构参数
ARG ARCH

# 根据架构参数复制对应的二进制文件到容器中
COPY yak_linux_${ARCH} /app/yak

# 赋予执行权限
RUN chmod +x /app/yak

# 运行程序来验证其正常工作
RUN /app/yak -v

# 设置容器启动时默认执行的命令
CMD ["/app/yak", "-v"]