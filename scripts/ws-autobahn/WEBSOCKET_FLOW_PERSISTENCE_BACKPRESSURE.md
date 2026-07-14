# Yakit WebSocket 流持久化背压问题与治理方案

## 1. 结论

Yak `lowhttp` WebSocket 客户端和 Yakit MITM 的 RFC 6455 / RFC 7692
协议路径已经通过完整 Autobahn 验证：

- `yak-lowhttp-client`：216/216 个压缩案例 `behavior=OK`，
  `behaviorClose=OK`。
- `yak-lowhttp-via-yak-mitm`：216/216 个压缩案例 `behavior=OK`，
  `behaviorClose=OK`。
- 持久化开启的 9 个 compression-smoke 案例、Vulinbox `ws/wss` 矩阵和
  MITM race 测试均通过。

但完整压缩矩阵在同时开启 WebSocket 流持久化时暴露了一个独立的资源治理问题：
高吞吐、大消息场景会让待写 SQLite 的 WebSocket 数据在内存中快速积压。该问题
不属于 RFC 协议错误，但可能在真实高流量连接中造成 Yakit 内存膨胀、GC 压力、
swap、代理延迟，最终影响连接可靠性。

在实现正式治理前，完整 Autobahn compression profile 默认设置：

```text
AUTOBAHN_MITM_DISABLE_FLOW_STORAGE=true
```

它只关闭测试期间的 SQLite/UI 流记录，不会绕过 MITM 握手、帧解析、压缩协商、
解压、重新压缩、context takeover 或双向转发。compression-smoke 和 Vulinbox
仍保持持久化开启，用于防止正常捕获路径失去回归覆盖。

## 2. 观测证据

### 2.1 协议路径

直连完整压缩矩阵：

```text
ok github.com/yaklang/yaklang/common/utils/lowhttp 569.944s
yak-lowhttp-client behavior: OK=216
yak-lowhttp-client close: OK=216
```

关闭流持久化后的 MITM 完整压缩矩阵：

```text
ok github.com/yaklang/yaklang/common/yakgrpc 696.190s
yak-lowhttp-via-yak-mitm behavior: OK=216
yak-lowhttp-via-yak-mitm close: OK=216
```

报告保存在：

```text
reports/autobahn/compression-final/clients/index.json
reports/autobahn/compression-mitm-final/clients/index.json
reports/autobahn/compression-mitm-final/run.log
```

### 2.2 持久化压力路径

使用完整 compression profile，并显式设置：

```text
AUTOBAHN_MITM_DISABLE_FLOW_STORAGE=false
```

运行到约 `case_31/216` 时观测到：

```text
yakgrpc.test RSS        约 4.7 GB
临时项目数据库          约 611 MB
系统 swap 已使用        约 4 GB
```

测试进程仍在工作，并未出现协议断言失败；为了避免影响开发环境，在内存继续增长前
主动终止。因此当前结论是“确认存在高内存放大风险”，不是“已证明最终 OOM”，也
不能把这次主动终止记为 RFC 失败。

关闭持久化后，同类完整 MITM 运行的 RSS 稳定在约 680 MB，并在约 11.6 分钟内
完成 216 个案例。这一对照说明主要放大来自捕获数据的构造和落库积压，而不是
RFC 7692 编解码本身。

## 3. 根因路径

当前 WebSocket 数据帧进入持久化前会经过以下转换：

```text
frame []byte
  -> string(data)
  -> strconv.Quote(...)
  -> schema.WebsocketFlow.QuotedData
  -> map[string]interface{}
  -> DBSaveAsyncChannel
  -> SQLite FirstOrCreate
```

关键代码：

- `common/yakgrpc/yakit/websocketflow.go`：`BuildWebsocketFlow` 在入队前执行
  `strconv.Quote(string(data))`，同时保留多个对象和字符串副本。
- `common/yakgrpc/yakit/websocketflow.go`：`CreateOrUpdateWebsocketFlowEx` 将包含
  完整 payload 的闭包发送到全局异步保存队列。
- `common/yakgrpc/yakit/base.go`：`DBSaveAsyncChannel` 容量为 40960，限制的是任务
  数量，不是队列中 payload 的总字节数。
- `common/yakgrpc/grpc_mitm.go` 和 `grpc_mitm_v2.go`：上下行消息都会构造并保存
  `WebsocketFlow`。
- `common/crep/mitm_ws.go`：透明 mirror 路径还会为每条消息启动 goroutine；如果
  下游保存速度长期低于帧到达速度，待执行 goroutine 也缺少明确上限。

Autobahn full compression 每个案例发送 1000 条消息，包含最大 131072 字节消息
和多种分片、窗口及 context takeover 组合。SQLite 消费速度低于消息产生速度时，
40960 个按“条”计数的队列槽可以同时持有数 GB 的大字符串和闭包。

## 4. 影响边界

已确认不受该问题影响的结论：

- WebSocket 握手和 `Sec-WebSocket-Extensions` 协商正确性。
- Mask、RSV、opcode、分片、控制帧、UTF-8 和 Close 处理。
- `permessage-deflate` 双向窗口、context takeover 和分片压缩。
- 不记录流时的 Yakit MITM 双向转发正确性。
- 正常规模下开启持久化的 Vulinbox 和 compression-smoke 场景。

仍需治理的生产风险：

- 高吞吐或大消息连接导致堆内存和 GC 压力持续上升。
- 全局 DB 队列被 WebSocket 大 payload 占据，影响其他 HTTP 流或任务保存。
- swap 或 GC 抖动增加代理延迟，间接触发客户端/服务端超时。
- 透明 mirror 路径可能积累大量等待保存的 goroutine。
- 当前没有面向用户的“队列字节数、降级次数、未记录帧数”指标。

## 5. 设计原则

正式修复应遵守以下顺序：

1. 优先保证被代理的 WebSocket 连接和业务数据转发。
2. 捕获持久化必须有确定的内存上限，不能只按消息条数限流。
3. 过载行为必须显式、可统计、可向 UI 报告，不能静默缺帧。
4. 同一连接内已接受记录的 `FrameIndex` 顺序必须稳定。
5. 数据库慢、关闭中或上下文取消时，不能泄漏 goroutine 或无限等待。
6. 正常负载下保持当前完整记录能力和查询兼容性。

## 6. 推荐方案

推荐采用“连接优先、专用有界队列、字节预算、延迟序列化、批量落库”的方案。

### 6.1 专用 WebSocket 保存队列

- 不再让大 payload 直接占用通用 `DBSaveAsyncChannel` 的大量槽位。
- 建立 WebSocket 专用 spool/worker，配置全局和单连接两级预算。
- 队列同时限制 item 数和 payload 总字节数；字节预算是主要限制。
- 初始建议值应通过基准测试确定，例如全局 64 MiB、单连接 16 MiB，而不是直接
  作为不可调整的产品常量。

### 6.2 延迟序列化和减少复制

- 只有成功获得队列预算后才构造持久化对象。
- 不在帧转发 goroutine 中提前执行 `strconv.Quote`。
- 队列中优先保留必要元数据和一份 payload，转义/序列化放到 worker 中执行。
- 评估数据库使用 BLOB 保存原始数据、查询时再编码；若暂不修改 schema，至少避免
  入队前同时保留 `[]byte`、`string` 和 quoted string 多份副本。

### 6.3 过载策略

默认建议采用 connection-first：

- 队列预算不足时继续转发业务帧，不阻塞协议链路等待 SQLite。
- 为该连接累计 `capture_dropped_frames`、`capture_dropped_bytes` 和时间范围。
- 在 WebSocket 数据列表插入一个可识别的 gap/降级标记，避免 UI 看起来像完整记录。
- 使用限频日志和指标报告，而不是每丢一帧打印一次日志。

可以额外提供 strict-capture 模式：队列满时施加背压，适用于用户明确要求完整取证且
接受业务连接变慢的场景。它不应成为默认行为。

### 6.4 批量落库

- worker 按固定条数或短时间窗口聚合插入，例如 100 条或 10 ms。
- 保证单连接 `FrameIndex` 顺序，并验证多连接公平性。
- 连接关闭时允许有限时间 flush；超时后生成明确的 gap 统计并释放资源。

## 7. 实施顺序

### 阶段 A：先建立可重复的容量测试和指标

- 增加不依赖 Docker 的 WebSocket flow queue 单元测试。
- 增加 1000 x 128 KiB 上下行消息的持久化负载测试。
- 记录队列 item 数、排队字节数、入队/落库/降级数量、处理延迟和峰值 RSS。
- 为透明 mirror 路径增加 goroutine 数量回落断言。

### 阶段 B：实现有界 spool 和过载语义

- 引入专用队列及全局/单连接字节预算。
- 将序列化移动到成功入队之后或 worker 内。
- 实现 connection-first 降级、gap 标记和限频告警。
- 保持正常负载下数据库记录数量、方向和 `FrameIndex` 与现有行为一致。

### 阶段 C：批量写入和长稳验证

- 实现或复用批量 SQLite 插入。
- 运行 1 小时、8 小时和 24 小时 soak。
- 覆盖大量短连接、多个并发长连接、慢磁盘和周期性大压缩消息。
- 将资源曲线和降级指标保存为 nightly CI artifact。

## 8. 验收标准

协议回归：

- Autobahn core 无 Yak 硬失败和 MITM 差分回归。
- Autobahn compression 直连与 MITM 均为 216/216 `OK`。
- Vulinbox `ws/wss` 场景、compression-smoke 和 race 测试通过。

资源和行为：

- 队列中的 WebSocket payload 总字节数永不超过配置预算。
- 参考 1000 x 128 KiB 测试中，峰值 RSS 相对基线增长有稳定上限；初始验收目标为
  不超过 256 MiB，最终值以基准环境校准后固化。
- SQLite 暂停或显著变慢时，代理仍能按配置选择转发优先或 strict-capture 背压。
- connection-first 模式下，所有未记录帧和字节都能通过计数器/gap 对账。
- 正常负载不降级、不缺帧，`FrameIndex` 单调且方向正确。
- 连接关闭或取消后，队列预算、worker 和 goroutine 在限定时间内回落。
- `go test -race` 无新增报告，无 panic、OOM、死锁或无限阻塞。

## 9. 当前操作方式

协议一致性完整验证：

```bash
AUTOBAHN_PROFILE=compression \
AUTOBAHN_MODE=all \
MOCKEY_CHECK_GCFLAGS=false \
scripts/ws-autobahn/run.sh
```

该命令对 full compression 的 MITM 阶段默认关闭流持久化。

小规模持久化回归：

```bash
AUTOBAHN_PROFILE=compression-smoke \
AUTOBAHN_MODE=all \
MOCKEY_CHECK_GCFLAGS=false \
scripts/ws-autobahn/run.sh
```

仅在受控环境执行未治理的完整持久化压力测试：

```bash
AUTOBAHN_PROFILE=compression \
AUTOBAHN_MODE=mitm \
AUTOBAHN_MITM_DISABLE_FLOW_STORAGE=false \
MOCKEY_CHECK_GCFLAGS=false \
scripts/ws-autobahn/run.sh
```

执行该命令时必须监控 RSS、swap、临时数据库大小和 DB 保存队列，不应作为普通
开发机或提交前检查。

## 10. 状态

- RFC 6455 / RFC 7692 协议完整性：已通过当前矩阵。
- MITM 劫持后的 reader 所有权和专项竞态：已修复并通过 race。
- 完整压缩协议矩阵：直连和 MITM 均已通过。
- 正常规模 WebSocket 流持久化：已有冒烟和 Vulinbox 覆盖。
- 高吞吐持久化内存治理：已确认问题，尚未实施正式背压方案。
- 下一项主任务：阶段 A，建立字节级容量测试与指标，再进入阶段 B 的行为修改。
