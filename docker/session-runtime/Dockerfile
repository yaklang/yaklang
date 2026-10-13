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
#   LEGION_SESSIONMGR_URL, LEGION_AI_SESSION_ID, LEGION_AI_RUNTIME
# 并通过 -agent-installation-id ai-session-<sessionID> 传 ephemeral identity。

# The Node compiler and C toolchain share the host/portable musl recipe. Only
# the operating-system image used to run the static executable remains Debian.
ARG GO_BUILDER_IMAGE=docker.io/library/golang:1.22.12-alpine3.21@sha256:1699c10032ca2582ec89a24a1312d986a3f094aed3d5c1147b19880afe40e052
ARG RUNTIME_BASE_IMAGE=debian:bookworm-slim

# ---------- 构建阶段 ----------
FROM ${GO_BUILDER_IMAGE} AS builder

ARG TARGETOS
ARG TARGETARCH
ARG BUILDARCH
ARG GO_BUILDER_IMAGE
ARG NODE_MUSL_ALLOW_EMULATION=false

WORKDIR /src
COPY . .

# 编译 legion-smoke-node（ai_session 容器不需要 HIDS tag，精简依赖）。
# Yaklang's embedded pcre2 and pcap dependencies require CGO. The same helper
# configures all their C sources and archives for the musl static linker.
ENV GOTOOLCHAIN=local GOMODCACHE=/go/pkg/mod GOCACHE=/root/.cache/go-build
RUN ./.github/scripts/build-legion-linux-musl.sh --install-toolchain
RUN --mount=type=cache,id=legion-musl-modules-${TARGETARCH},target=/go/pkg/mod \
    --mount=type=cache,id=legion-musl-build-${TARGETARCH},target=/root/.cache/go-build \
    test "$TARGETOS" = "linux" \
    && if [ "$BUILDARCH" = "$TARGETARCH" ]; then execution=native; \
       else test "$NODE_MUSL_ALLOW_EMULATION" = true && execution=qemu-emulated; fi \
    && NODE_SOURCE_DIR=/src NODE_GOARCH="$TARGETARCH" NODE_BUILD_PROFILE=runtime \
       NODE_BUILDVCS=false NODE_MUSL_BUILDER_IMAGE="$GO_BUILDER_IMAGE" \
       NODE_MUSL_BUILDER_EXECUTION="$execution" \
       NODE_OUTPUT_BINARY=/out/legion-session-runtime \
       NODE_MUSL_METADATA_FILE=/out/session-runtime-musl-build.json \
       ./.github/scripts/build-legion-linux-musl.sh --in-container

# ---------- 运行阶段 ----------
FROM ${RUNTIME_BASE_IMAGE}

ARG SOURCE_SHA=unknown
ARG RUNTIME_VERSION=dev
LABEL org.opencontainers.image.title="Legion AI Session Runtime" \
      org.opencontainers.image.source="https://github.com/yaklang/yaklang" \
      org.opencontainers.image.revision="${SOURCE_SHA}" \
      org.opencontainers.image.version="${RUNTIME_VERSION}"

# ai_session 容器需要 ca-certificates 调平台 HTTPS bootstrap。
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/*

COPY --from=builder /out/legion-session-runtime /usr/local/bin/legion-session-runtime
COPY --from=builder /out/session-runtime-musl-build.json /usr/local/share/legion/session-runtime-musl-build.json

# Keep standalone container launches aligned with sessionmgr. Set this to
# "stateful" only for a temporary rollback.
ENV LEGION_AI_RUNTIME=stateless

# 默认入口：作为 ai_session 容器 runtime 启动。
# sessionmgr 起容器时会覆盖 command 注入 -api-url / -enrollment-token /
# -kind ai_session / -agent-installation-id 等参数。
ENTRYPOINT ["/usr/local/bin/legion-session-runtime"]
