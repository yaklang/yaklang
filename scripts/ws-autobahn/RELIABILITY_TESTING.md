# Yak WebSocket 可靠性与完整测试指南

本文档定义 Yak `lowhttp` WebSocket 客户端、Yakit MITM WebSocket 代理和
RFC 7692 `permessage-deflate` 的测试分层、完整执行方法、验收标准及第三轮
可靠性建设范围。

目标不是只验证握手返回 `101`，而是确认握手、帧协议、压缩、分片、关闭、
MITM 双向转发和异常连接回收均能长期稳定工作。

## 1. 测试范围

当前测试覆盖以下协议面：

- RFC 6455 握手：`Upgrade`、`Connection`、`Sec-WebSocket-Accept` 和子协议。
- RFC 6455 帧：Mask、RSV、opcode、控制帧、分片、UTF-8 和关闭码。
- RFC 7692 协商：四个 `permessage-deflate` 参数及 offer/response 校验。
- RFC 7692 数据面：8-15 位窗口、双向 context takeover 和压缩分片。
- MITM：普通 `ws`、TLS `wss`、上下行消息、压缩转码、控制帧和关闭传播。
- Vulinbox：回显、服务端首帧、压缩、Ping/Pong、Close、子协议、空闲和延迟握手。
- Autobahn：核心协议矩阵、压缩矩阵及直连/MITM 差分报告。

第三轮还需要继续补充资源上限、解压膨胀、慢连接、并发竞态和长时间 soak 测试，
详见第 8 节。

完整 compression 测试已经确认 WebSocket 流持久化在高吞吐、大消息条件下存在
内存放大风险。证据、根因、推荐背压语义和验收标准见
[WEBSOCKET_FLOW_PERSISTENCE_BACKPRESSURE.md](./WEBSOCKET_FLOW_PERSISTENCE_BACKPRESSURE.md)。

## 2. 环境要求

从仓库根目录执行所有命令：

```bash
cd /home/go0p/code/go/yaklang
```

需要：

- 可用的 Go 工具链。
- 可运行 Docker 的当前用户。
- Docker 能获取已固定 digest 的 `crossbario/autobahn-testsuite` 镜像。
- 至少一个本地空闲 TCP 端口，默认使用 `9001`。
- 完整压缩矩阵需要较长运行时间和足够的磁盘空间保存 HTML/JSON 报告。

建议为会初始化 Yak 数据库的测试提供临时目录，避免接触开发环境数据库：

```bash
export YAKIT_HOME="$(mktemp -d)"
export MOCKEY_CHECK_GCFLAGS=false
```

测试结束后可以删除该临时目录。Autobahn runner 会自行创建和清理测试用
`YAKIT_HOME`。

## 3. 提交前快速检查

每次修改 WebSocket 代码后先执行：

```bash
gofmt -w \
  common/utils/lowhttp/ws.go \
  common/utils/lowhttp/ws_client.go \
  common/utils/lowhttp/ws_compress.go \
  common/utils/lowhttp/ws_extension.go \
  common/crep/mitm_ws.go

git diff --check
git diff --exit-code -- go.mod go.sum
```

`go.mod` 和 `go.sum` 检查用于保证 WebSocket 修复没有意外引入新依赖。

运行 WebSocket 专项单元测试：

```bash
go test ./common/utils/lowhttp \
  -run 'TestWebsocket|TestStrictWebsocket|TestCompressedFragments' \
  -count=1

go test ./common/vulinbox \
  -run '^TestWebsocketScenarioEndpoints$' \
  -count=1
```

运行快速 Autobahn 冒烟测试：

```bash
AUTOBAHN_PROFILE=smoke \
AUTOBAHN_MODE=all \
scripts/ws-autobahn/run.sh

AUTOBAHN_PROFILE=compression-smoke \
AUTOBAHN_MODE=all \
scripts/ws-autobahn/run.sh
```

预期结果：命令退出码为 `0`，Yak 直连和 Yak 经 MITM 的压缩案例均为 `OK`，
不存在 `FAILED`、`MISSING` 或 `UNIMPLEMENTED`。

## 4. 完整功能回归

先运行完整 `lowhttp` 测试：

```bash
go test ./common/utils/lowhttp -count=1
```

然后运行 Vulinbox 的 `ws` 和 `wss` MITM 场景矩阵：

```bash
YAKIT_HOME="$(mktemp -d)" \
MOCKEY_CHECK_GCFLAGS=false \
go test ./common/yakgrpc \
  -run '^TestGRPCMUSTPASS_MITM_WebSocketVulinboxScenarios$' \
  -count=1 \
  -timeout=2m
```

最后检查 MITM 包至少能够独立编译：

```bash
go test ./common/crep -run '^$'
```

这些测试必须全部通过，不能依赖重试才能成功。

## 5. Autobahn 核心协议矩阵

运行 RFC 6455 核心案例，排除性能案例和压缩案例：

```bash
AUTOBAHN_PROFILE=core \
AUTOBAHN_MODE=all \
MOCKEY_CHECK_GCFLAGS=false \
scripts/ws-autobahn/run.sh
```

该 profile 当前选择 247 个案例。它会生成三个 agent：

- `yak-lowhttp-client`：Yak WebSocket 客户端直连。
- `gorilla-direct`：Gorilla 参考客户端直连。
- `gorilla-via-yak-mitm`：同一参考客户端经过 Yakit MITM。

Gorilla 直连仅作为差分基线，其自身失败不会直接判定 Yak 失败。以下情况会使
runner 返回非零退出码：

- 非基线 agent 出现 `FAILED`、`MISSING` 或 `UNIMPLEMENTED`。
- Gorilla 直连通过，但经过 Yakit MITM 后变成硬失败。
- 报告缺少预期 agent 或案例。

`INFORMATIONAL` 不属于失败。`NON-STRICT` 会作为警告保留在报告中，但不能伴随
硬失败；它通常表示代理更早关闭了非法连接，改变了 Autobahn 观察到的事件顺序。

## 6. Autobahn 完整压缩矩阵

开发阶段先运行 9 个代表性压缩案例：

```bash
AUTOBAHN_PROFILE=compression-smoke \
AUTOBAHN_MODE=all \
MOCKEY_CHECK_GCFLAGS=false \
scripts/ws-autobahn/run.sh
```

该集合覆盖重复消息、不同数据集、自动分片、context takeover、无 context
takeover 和 `client_max_window_bits=9`。最小 8 位窗口及客户端/服务端双向状态由
`lowhttp` 单元测试补充覆盖。

发布前或 nightly CI 必须运行完整的 216 个压缩案例：

```bash
set -o pipefail

AUTOBAHN_PROFILE=compression \
AUTOBAHN_MODE=all \
MOCKEY_CHECK_GCFLAGS=false \
scripts/ws-autobahn/run.sh \
  2>&1 | tee /tmp/yak-websocket-autobahn-compression.log
```

完整矩阵每个案例发送 1000 条消息，包含最大 131072 字节消息和多种自动分片
大小，单例定义的最长等待时间为 480 秒。runner 为该 profile 配置了 9 分钟
单例超时和 36 小时 Go 测试总超时，不应在普通提交前流水线中运行。

完整压缩 profile 默认设置 `AUTOBAHN_MITM_DISABLE_FLOW_STORAGE=true`。该设置只
关闭 WebSocket 流的 SQLite/UI 持久化；MITM 仍会协商、解析、解压、重新压缩并
转发每一帧。这可以避免 216000 条大消息占满异步落库队列，使此步骤专注协议
一致性。单独验证持久化负载时设置
`AUTOBAHN_MITM_DISABLE_FLOW_STORAGE=false`，并监控 RSS、数据库大小、队列深度
以及帧阻塞或丢失。

压缩 profile 使用 `yak-lowhttp-client` 和 `yak-lowhttp-via-yak-mitm`，因为 Gorilla
只支持 RFC 7692 协商响应的较窄子集。两个 agent 都必须没有硬失败；理想结果为
所有案例的 `behavior` 和 `behaviorClose` 均为 `OK`。

## 7. 报告和失败排查

报告默认写入：

```text
reports/autobahn/<profile>/clients/index.html
reports/autobahn/<profile>/clients/index.json
reports/autobahn/<profile>/clients/*_case_*.json
reports/autobahn/<profile>/clients/*_case_*.html
```

可以再次独立检查 JSON 报告：

```bash
go run scripts/ws-autobahn/check_report.go \
  reports/autobahn/compression/clients/index.json
```

按下面顺序定位失败：

1. 握手失败：检查状态码及 `Upgrade`、`Connection`、`Sec-WebSocket-Accept`、
   `Sec-WebSocket-Protocol`、`Sec-WebSocket-Extensions`。
2. 扩展失败：同时保存浏览器 offer、MITM 上游 offer、上游 response 和下游 response。
3. `1002`：检查 Mask、RSV、opcode、控制帧长度及分片状态机。
4. `1007`：检查文本消息在完整重组和解压后是否为合法 UTF-8。
5. 压缩失败：检查方向对应的 context takeover、窗口位数、RSV1 和 DEFLATE 尾部。
6. `use of closed network connection`：先寻找更早的握手、协议或写入错误；该错误通常
   是一侧已经关闭后的二次读写结果。

复现客户环境问题时至少收集：

- 浏览器 Network 中握手 request/response headers。
- 浏览器 WebSocket Messages 面板和 close code。
- Yakit 中对应连接的 request/response 与数据帧列表。
- 从握手前到连接关闭后的完整 Yak 引擎日志。
- 客户端、Yakit 和目标服务端的地址、代理链及是否启用 TLS/压缩。

## 8. 第三轮可靠性计划

第三轮按以下顺序推进。

当前进度：P0 的本地 full core/compression、Vulinbox 和 WebSocket 专项 race 已
完成；nightly CI 接入尚未完成。下一主线应进入 P1，并优先处理已被完整压缩测试
实证的流持久化字节级背压，而不是继续扩充等价的协议 happy-path 案例。

### P0：全矩阵和竞态

- 将 core 和完整 compression profile 接入 nightly CI。
- 对 WebSocket 专项包运行 race detector：

```bash
go test -race ./common/utils/lowhttp -count=1

YAKIT_HOME="$(mktemp -d)" \
MOCKEY_CHECK_GCFLAGS=false \
go test -race ./common/yakgrpc \
  -run '^TestGRPCMUSTPASS_MITM_WebSocketVulinboxScenarios$' \
  -count=1 \
  -timeout=5m
```

- 验收：无数据竞争、无 goroutine 泄漏、无硬失败。

### P1：异常输入和资源边界

- 按
  [WebSocket 流持久化背压方案](./WEBSOCKET_FLOW_PERSISTENCE_BACKPRESSURE.md)
  先建立容量测试、字节预算、过载语义和可观测性。
- 增加超大声明长度、截断帧、损坏压缩流、解压膨胀和非法关闭帧测试。
- 为单帧、单消息、解压后消息和写队列设计可配置上限。
- 验证慢读、慢写、一侧不读和连接中途断开时的背压及关闭行为。
- 验收：不 panic、不无限分配内存、不转发半条压缩消息，使用正确 close code。

### P2：长稳和可观测性

- 建立 1 小时、8 小时和 24 小时长连接测试。
- 覆盖大量短连接、稳定并发长连接及周期性大压缩消息。
- 记录连接数、goroutine、堆内存、GC、收发消息数、关闭原因和压缩错误。
- 验收：资源曲线稳定，连接结束后回落，没有持续增长的 goroutine 或压缩上下文。

## 9. 发布验收清单

WebSocket 相关版本发布前应满足：

- `git diff --check` 通过，`go.mod/go.sum` 无意外变化。
- `lowhttp` 完整测试通过。
- Vulinbox `ws/wss` MITM 矩阵通过。
- Autobahn core 无 Yak 硬失败和 MITM 差分回归。
- Autobahn compression 直连与 MITM 均无硬失败。
- race detector 无报告。
- 第三轮资源边界测试无 panic、OOM、死锁或 goroutine 泄漏。
- HTML/JSON 报告和命令日志作为 CI artifact 保存。

只有快速冒烟通过不能视为完整验证；发布验收应以 core、完整 compression、
Vulinbox、race 和资源边界五类结果共同决定。
