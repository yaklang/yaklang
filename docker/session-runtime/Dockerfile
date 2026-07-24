# Legion AI session 容器 runtime 镜像。
#
# 每个 AI session 容器内跑一个 legion-smoke-node（kind=ai_session），
# 它自连平台 /v1/nodes/bootstrap 注册成容器-node，连 NATS 订阅自己的
# legion.command.node.<nodeID>，处理 ai.session.* 命令，
# bootstrap 成功后回调 sessionmgr register 端点上报 node id。
#
# 构建上下文为 scannode 仓库根目录：
#   docker build -t legion/session-runtime:latest -f docker/session-runtime/Dockerfile .
#
# sessionmgr 起容器时注入环境变量：
#   LEGION_NATS_URL, LEGION_API_URL, LEGION_ENROLLMENT_TOKEN,
#   LEGION_SESSIONMGR_URL, LEGION_AI_SESSION_ID
# 并通过 -agent-installation-id ai-session-<sessionID> 传 ephemeral identity。

# ---------- 构建阶段 ----------
FROM golang:1.24-bookworm AS builder

WORKDIR /src
COPY . .

# 编译 legion-smoke-node（ai_session 容器不需要 HIDS tag，精简依赖）。
RUN CGO_ENABLED=0 go build -ldflags "-s -w" -o /out/legion-session-runtime ./cmd/legion-smoke-node

# ---------- 运行阶段 ----------
FROM debian:bookworm-slim

# ai_session 容器需要 ca-certificates（调平台 HTTPS bootstrap）+ curl（调试）。
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/*

COPY --from=builder /out/legion-session-runtime /usr/local/bin/legion-session-runtime

# 默认入口：作为 ai_session 容器 runtime 启动。
# sessionmgr 起容器时会覆盖 command 注入 -api-url / -enrollment-token /
# -kind ai_session / -agent-installation-id 等参数。
ENTRYPOINT ["/usr/local/bin/legion-session-runtime"]