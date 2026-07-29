# MITM V2 实时流量体验优化路线图

## 1. 背景与目标

当前 MITM 流量表采用“异步写库 → 后端每秒检查数据库变化 → DuplexConnection 推送失效通知 → 前端每秒调用 QueryHTTPFlows”的混合模型。它能够保证完整流量最终落库，但包含两层固定周期，且通知没有高水位、序号和积压量，因此在高并发下容易出现：

- 浏览器请求已经完成，列表仍需 1～3 秒才出现；
- 生产速度持续高于固定分页消费速度时，页面越来越落后；
- 空查询与通知交错时可能丢失唤醒；
- Duplex 断开后缺少明确的轮询降级；
- 数据库、gRPC、Electron IPC 与 React 各阶段耗时无法关联。

本路线图的目标是：

1. 先建立可重复、低开销的端到端观测，避免凭固定分页数字调参；
2. 优化现有链路中已经确认的热点和时序风险；
3. 建立“实时事件快路径 + 数据库游标补偿路径”；
4. 最终实现接近浏览器 Network 面板的即时反馈，同时保持数据库完整性与旧版本兼容。

## 2. 性能与正确性契约

建议以下列指标作为验收标准，而不是以单轮拉取条数作为目标：

| 指标 | 常规负载目标 | 高负载目标 |
| --- | ---: | ---: |
| `response_mirrored → react_commit` P95 | ≤ 200ms | ≤ 500ms |
| `persisted → react_commit` P95 | ≤ 100ms | ≤ 300ms |
| 实时列表主线程长任务 P95 | < 50ms | < 100ms |
| Renderer 实时摘要窗口 | 有界 | 有界 |
| Electron/后端待发送队列 | 有界 | 有界且可降级 |
| ID 缺口、重复行 | 0 个永久缺口、0 个重复 | 0 个永久缺口、0 个重复 |
| 断线恢复 | 自动补齐 | 自动补齐 |

请求本身的网络耗时不计入“列表跟手”SLO，但需要单独记录，便于区分目标服务器慢和 Yakit 展示慢。

## 3. 总体架构

目标模型：

```text
MITM request/response
        │
        ├── lightweight live event ──> Electron batcher ──> bounded UI window
        │                                      │
        └── plugins / TrafficGuard / DB commit ┘
                          │
                          └── QueryHTTPFlows: bootstrap / history / gap recovery
```

基本原则：

- 数据库仍是完整、可恢复的事实来源；
- 实时流只传表格摘要，不传完整请求/响应正文；
- 每条事件带单调 ID/Sequence 和项目/会话身份；
- 快消费者直接应用事件，慢消费者收到 `Gap` 后按游标补偿；
- UI 可以丢弃已落库的中间展示事件，但不能静默丢失数据库记录；
- 批处理按时间和字节预算，不使用 100、300、1200 作为协议语义。

## 4. 分阶段实施

### Phase 0：端到端观测

状态：有界观测链路与 Electron 自动化报告已经落地，首轮同口径 A/B 已完成。

首版时间点：

```text
request_hijack_enter
response_mirror_enter
flow_built
persist_enqueued
persist_started
persisted
database_change_detected
query_server_received
query_sql_finished
query_conversion_finished
electron_grpc_received
renderer_query_received
react_commit
```

实施约束：

- 不修改数据库表结构；
- `QueryHTTPFlows` 仅在请求显式开启 `IncludeSystemTiming` 时返回诊断字段；
- 后端只保存固定数量的最近流量时序，响应只返回固定数量的样本；
- 不逐流打印日志，不创建无界 Map/Channel；
- 诊断字段为 protobuf 增量字段，旧端会安全忽略；
- Renderer 只保留固定数量的查询、通知和流量可见性样本。

输出指标至少包括：

- MITM 处理、异步写库排队、实际写库耗时；
- 数据库提交到变化检测的耗时；
- QueryHTTPFlows SQL、模型转换、gRPC/Electron IPC 耗时；
- Renderer 收包到 React 提交耗时；
- 请求/响应/落库到 React 提交的 P50/P95/P99；
- 最新后端 ID、最新可见 ID、近似 ID backlog；
- 单次查询行数和查询频率。

自动化：

- 时序采样固定容量与 ID 覆盖测试；
- 同步、异步写库阶段顺序测试；
- QueryHTTPFlows 诊断字段开关与上限测试；
- Renderer 去重、容量、百分位和 React commit 关联测试；
- 将关键指标接入 `yak-mitm-perf` 与 Electron/React 前后对比报告。

#### Phase 0 当前实现（2026-07-22）

已完成：

- MITM V2 在请求劫持入口、响应镜像入口和 HTTPFlow 构建完成处记录时间点；
- 异步写库记录入队、开始 SQL、提交完成和队列深度/容量；
- 后端使用 8192 个固定槽保存最近持久化时序，单次查询最多返回 64 个匹配样本；
- 数据库身份使用不可逆短摘要，16 个固定槽隔离最近项目的 high-water，不返回本地数据库路径；
- 数据库 watcher 为恰好命中的 high-water 样本补充变化检测时间；
- `QueryHTTPFlows` 新增可选 `IncludeSystemTiming`，关闭时不返回诊断对象；
- MITM 流量表自动开启该字段，历史、插件等其他页面保持关闭；
- Electron Main 记录 IPC 收到、gRPC 开始/结束和 gRPC 总耗时；
- Renderer 在 `useLayoutEffect` 记录 React commit，保留查询 256 条、流量 1024 条、Duplex 256 条；
- 项目身份改变时清理数据库本地 ID 状态，不跨项目计算 backlog；
- 全链路不逐流打印日志，不修改数据库 schema，不创建无界容器。
- `yak-mitm-perf` 已自动输出后端 SQL/转换、写库排队/执行、变化检测、Duplex 投递，以及请求/响应/落库→直接 gRPC 探针收到的分位数；场景按唯一 token 隔离并对逐流样本去重。

开发模式下可在 MITM 页面 DevTools Console 中读取快照：

```js
window.__YAKIT_MITM_FLOW_OBSERVABILITY__?.snapshot()
```

清空当前窗口样本：

```js
window.__YAKIT_MITM_FLOW_OBSERVABILITY__?.reset()
```

快照包含：

- `query`：查询频率、返回行数、Renderer→Main、Main 调度、后端 SQL/转换、gRPC、Main→Renderer、响应→React commit 的分位数；
- `flow`：请求/响应/构建/入队/SQL/变化检测→React commit 的分位数；
- `duplex`：后端推送时间戳到 Renderer 收到的延迟；
- `state`：写库队列深度/容量、最新持久化/检测/可见 ID，以及近似 ID backlog；
- `bounds`：各固定窗口的硬上限，便于确认观测自身不会随运行时间增长。

兼容行为：新前端连接旧引擎时，新增请求字段会被忽略，仍可采集 Renderer/Electron 查询总耗时；后端细分和逐流时间为空。`approximateIdBacklog` 只用于趋势判断，因为 HTTPFlow ID 可能包含非 MITM 来源，不能作为精确待消费条数。

Electron WDIO 驱动现已将 Renderer 快照、Long Task、Electron/Yak CPU/RSS、逐流时序、正确性和清理状态写入独立的 `mitm-performance.json`。显式诊断模式可在负载窗口采集固定 1～60 秒的 Yak CPU profile，也可在空闲基线和恢复点各采集一次强制 GC 的 heap profile，或通过已有 WDIO Renderer CDP session 采集有界主线程 trace；三种模式互斥，均有硬期限/大小上限，并被 comparator 强制排除在正式 A/B 之外。CDP 摘要已能把 Long Task 归因到 JS、样式/布局、绘制、GC 与 IPC；直接 gRPC 探针仍明确排除 Electron 和 React，不冒充完整 UI 延迟。尚未完成的是跨进程统一 Trace ID、React 组件级 Profiler，以及将后端和 Electron 两种报告合并为单一父报告。

### Phase 1：现有链路低风险优化

这一阶段不引入新的实时协议，先处理观测确认的热点：

1. **异步写库队列**
   - 区分入队等待、SQL 执行和 after-save 工作；
   - 检查 40960 队列容量、48 条调度批次和 10ms 合并等待是否适合 MITM；
   - 检查项目切换时全局队列与当前项目 DB 的绑定；
   - 为队列增加水位、丢弃/阻塞策略和关闭语义；
   - 修正异步接口“返回 nil 被误认为已经提交”的语义和日志。

2. **QueryHTTPFlows**
   - 将 COUNT 与增量行查询解耦，实时请求默认不做精确 COUNT；
   - 使用明确的列表字段投影，详情正文按需加载；
   - 避免同一轮追赶执行多次完整 COUNT；
   - 请求目的显式传递，不再用 `Limit !== 100` 推断 top/offset；
   - 依据时间、响应字节和 backlog 自适应批次。

3. **DuplexConnection**
   - 已修复以 `ID == 0` 同时表示“未初始化”和“空表”的哨兵问题；空项目的第一次 `0 → 1` 插入现在会产生通知，并有状态机回归测试；
   - HTTPFlow/AIMemory watcher 会在广播 `enableServerPush` 前建立基线，握手期间的新写入不会被首次快照吞掉；
   - watcher 等待改为可响应 context 的 ticker，连接关闭不再额外卡在固定 `Sleep(1s)`；
   - 修复“通知到达后又被旧空查询关停”的丢唤醒窗口；
   - 连接错误/结束时恢复轮询并自动重连；
   - 避免每个连接启动一套数据库 watcher 后再全局广播；
   - 检查持锁调用 `stream.Send` 导致慢客户端阻塞其他广播的问题；
   - 通知至少携带数据库身份和高水位 ID。

4. **Electron IPC**
   - 测量 protobuf 解码、Main→Renderer structured clone 和对象体积；
   - 高频摘要考虑使用 MessagePort；
   - 以 16～50ms 时间窗和字节上限做有界合并；
   - 避免大批数据一次性跨 IPC 后再一次性提交 React。

5. **Renderer / React**
   - 保留有界摘要窗口和虚拟表格；
   - 逐批提交并限制单帧工作量；
   - 中间滚动位置只维护高水位/未读状态，不缓存无限完整行；
   - Total、字段分组、Tag/状态码聚合转为低优先级；
   - 检查筛选、收藏、标签和行样式处理中重复 O(N) 遍历。

6. **MITMV2 控制流**
   - 审计 Electron Main 固定 20ms 控制消息间隔；
   - 为手动劫持控制队列增加容量、合并和关闭处理；
   - 避免插件日志、手动包和控制响应之间发生队头阻塞。

每个优化项必须带自动化的优化前/优化后对比；允许多个问题合并为一组场景，不要求机械地一项一个 benchmark。

#### Phase 1 当前实现与证据（2026-07-22）

已完成的兼容性优化：

- `QueryHTTPFlowRequest` 增加可选 `ExcludeResponseRaw = 52` 和 `ExcludeRequestRaw = 53`，仅 MITM 实时列表开启；其他调用方默认行为不变，详情继续通过 ID 读取完整请求/响应包；
- 列表投影从 SQL 选择、Go 模型转换、protobuf 到 Electron IPC 全程省略原始请求/响应包，并且不进入完整 HTTPFlow 模型缓存，避免摘要对象污染后续详情查询；请求/响应长度、标题、状态码等表格元数据保持不变；
- HTTP 标题在新流量写入时提取一次并持久化到可空 `html_title`；旧记录的 `NULL` 值仍按原行为从响应中兼容提取，`Valid + empty` 明确表示已检查但没有标题；
- Web Fuzzer、WebSocket、CSRF、PoC、Comparer 和相关快捷键只在用户触发时按 ID 补取 Request；同一 ID 的并发补取会合并，批量动作继续遵守原有最多 10 条限制；
- 实时消费只在首次显示使用视口行数，之后依据列表实际携带的包字节按常规/追赶预算自适应；当请求/响应已投影时使用保守的 8 KiB 摘要估算，不再把详情正文声明长度误当成列表传输量。该估算只参与调度，时间预算和最大行数仍提供硬上限，不构成协议语义；
- 游标严格使用最后一条返回 ID；查询中的推送被合并为一次后续唤醒，短页不会再与同一个推送自旋竞争；
- HTTPFlow 广播改为 leading + latest trailing，每秒最多一次合并尾随通知，持续写入停止时不会丢最后一次唤醒；
- Duplex 状态变为可订阅，推送可用时取消轮询，真实断连时恢复；程序性列表高度/微小滚动不再误触发查询；
- Linux 进程名查找使用短 TTL PID 提示和固定分片锁，热点连接不再每次并发全量扫描 `/proc`；提示命中仍逐次验证 socket inode 与 `/proc/<pid>/exe`，PID 复用或多进程同 UID 不会继承错误名称，失败时精确回退全量扫描；
- minirehs Teddy 在 nibble 指纹后增加精确双字节前缀门，低熵大 Body 不再在每个字节位置进入 confirm；最短字面量至少为 2 才会进入该路径，因此不改变命中集合，随机差分、真实 TrafficGuard 规则对照和 race 均已覆盖；
- `SplitHTTPPacketEx` 在已有完整 `[]byte` 上切包时，改为根据 `bufio.Reader` 与底层 buffer 的精确未读长度一次性分配 Body 副本，不再用 `io.ReadAll` 反复几何扩容；返回 Body 仍是独立副本，不与输入包共享底层数组；
- 前端虚拟表格的 hover memo 比较器只让旧/新 hover 行失效，不再因表级 hover ID 变化重新渲染全部可见单元格；进入、离开、换行及无关行均有纯函数回归测试；
- 后端投影、缓存隔离、旧记录标题回退、通知尾随和 race 测试，以及前端游标、字节预算、调度器、按需补包和 E2E 正确性测试均已覆盖。

body 矩阵使用真实 POST 请求并按顺序单 worker 执行，每个 case 使用独立项目数据库和 Electron profile。列表查询断言 Request/Response 原始包为 0；性能观测窗口结束后，再通过 `GetHTTPFlowById` 校验详情包中的请求体和响应体字节数完全等于输入，避免详情校验本身污染 Long Task 和排空时间。

最终一次同机、同构建、同 profile 的 WSLg A/B 如下；两组 comparator 均为 `passed`：

| 场景（120 请求，并发 8） | 数据库排空 | Renderer 排空 | request → React p95 | Query 往返 p95 | Long Task 总时长 / p95 | 吞吐 |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| Request 64 KiB / Response 4 KiB | `2723 → 98 ms` | `2748 → 363 ms` | `3893 → 1156 ms` | `2125 → 116 ms` | `455 → 407 ms` / `100 → 76 ms` | `32.32 → 37.49 req/s` |
| Request 64 KiB / Response 256 KiB | `2054 → 190 ms` | `2078 → 460 ms` | `5656 → 1514 ms` | `3362 → 167 ms` | `374 → 395 ms` / `86 → 72 ms` | `17.68 → 18.95 req/s` |

Request-heavy 场景的后端列表模型转换 p95 为 `1070.469 → 0.256 ms`，列表 Request/Response 原始包均降为 0；详情校验仍得到 Request `65536` 字节、Response `4096` 字节。双向大 body 详情校验得到 Request `65536` 字节、Response `262144` 字节。Long Task 次数和“总时长 / 观测窗口”的阻塞占比只作诊断：更快排空会缩短分母，不能把占比单独当成回归；绝对总时长、p95 和最大值仍参与门禁。

#### Phase 1 CPU 归因与第二轮证据（2026-07-23）

有界 CPU profile 已定位并验证两个确定热点。request-heavy 场景中，进程名查找从 `1080 ms / 16.51%` 降到 `50 ms / 0.90%`，总 CPU 样本 `6540 → 5580 ms`，吞吐 `36.68 → 42.19 req/s`。双向大 Body 场景中，TrafficGuard 从 `3240 ms / 27.76%` 降到 `100 ms / 1.27%`，其中 minirehs C 扫描从 `3120` 降到 `30 ms`；总 CPU 样本 `11670 → 7880 ms`，吞吐 `22.29 → 30.35 req/s`。这些数字来自 profile 运行，只用于归因。

TrafficGuard 独立微基准与端到端结果相互印证：连续 `a` 的 256 KiB JSON 从约 `24 ms/op` 降到 `0.57 ms/op`（约 42 倍），普通 256 KiB JSON/HTML 从约 `3.5` 降到 `1.56 ms/op`（约 2.2 倍）。没有跳过大包、非文本或响应方向，敏感信息检测语义保持不变。

正式无 profile A/B 仍保留未通过项：

| 场景（120 请求，并发 8） | 数据库 / Renderer 排空 | request → React p95 | Query 往返 p95 | Long Task 总时长 / p95 | 吞吐 | Yak 峰值 RSS | 门禁 |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | --- |
| Request 64 KiB / Response 4 KiB | `2723 → 353` / `2748 → 374 ms` | `3893 → 1016 ms` | `2125 → 63 ms` | `455 → 400` / `100 → 96 ms` | `32.32 → 56.62 req/s` | `668 → 741 MB` | 最新重复样本通过；前一次 `780 MB` 触发门禁 |
| Request 64 KiB / Response 256 KiB | `2054 → 148` / `2078 → 853 ms` | `5656 → 1463 ms` | `3362 → 193 ms` | `374 → 436` / `86 → 72 ms` | `17.68 → 31.75 req/s` | `854 → 1071 MB` | 失败：Long Task 总时长 `+16.6%`、RSS `+25.4%` |

因此 CPU 优化可以保留，但这一轮仍不能宣称整体门禁全部通过；下面的 heap/allocation 阶段继续处理大 Body 临时分配。

#### Phase 1 Heap/allocation 归因与第三轮证据（2026-07-23）

heap 诊断模式在同一隔离 Yak 进程中先后请求两次 `/debug/pprof/heap?gc=1`：第一份位于稳定空闲基线，第二份位于负载、数据库/Renderer 排空和 CPU 恢复之后。`alloc_space` 以第一份为 base，表示该窗口累计分配；`inuse_space` 差分和结束时绝对 live heap 用于区分临时分配风暴与持续存活。原始 profile、flat/cumulative top 和结构化摘要均限制为 64 MiB，并标记 `diagnosticOnly`。

双向大 Body（120 请求、并发 8、Request 64 KiB、Response 256 KiB）首份诊断中，37.5 MiB 原始双向 Body 产生 `5,395,723,287 B` 累计分配；`io.ReadAll` 为 `3,810,906,733 B / 70.63%`，`bytes.growSlice` 为 `644,438,541 B / 11.94%`。基线到恢复点的净 live heap 差分约 14.8 MiB，结束时绝对 live heap 约 271.3 MiB，因此主要问题是同一包在读取、修复、匹配和建流路径中的短命重复复制，而不是 5 GiB 级别的泄漏。

`SplitHTTPPacketEx` 精确分配改动保持独立副本语义。256 KiB 微基准的中位数为：

| 指标 | 优化前 | 优化后 | 变化 |
| --- | ---: | ---: | ---: |
| 耗时 | `207.2 µs/op` | `67.8 µs/op` | 约 `3.05×` 更快 |
| 分配字节 | `1,191,665 B/op` | `267,750 B/op` | `-77.5%` |
| 分配次数 | `52 allocs/op` | `32 allocs/op` | `-38.5%` |

重复同一真实 heap 场景后，累计分配为 `2,999,089,540 B`，减少 `2,396,633,747 B / 44.4%`；结束时绝对 live heap 约 269.9 MiB，与优化前基本持平。`common/utils/lowhttp` 全包测试、跨 4 KiB reader 边界/独立副本测试和定向 race 均通过。诊断产物分别位于前端报告 `2026-07-23T02-45-53-407Z`（优化前）与 `2026-07-23T02-54-58-894Z`（优化后）。

正式无 profile 运行 `2026-07-23T02-59-44-866Z` 相对本次分配修复前报告通过 comparator：Renderer 排空 `853 → 432 ms`、request → React p95 `1463 → 1092 ms`、Query 往返 p95 `192.8 → 122.1 ms`、吞吐 `31.75 → 38.38 req/s`、Yak 峰值工作集 `1071 → 915 MB`。相对最初基线，Yak 峰值工作集只增加 `7.2%`，不再触发 15% 门禁；但 Renderer Long Task 总时长仍为 `374 → 443 ms / +18.4%`，所以总体 comparator 继续失败。

#### Phase 1 Renderer 归因、重复样本与第四轮证据（2026-07-23）

前端 body matrix 现支持每 case `1..10` 次严格串行重复，每次使用全新的项目 DB 和 Electron user-data，并输出 P50/P95、范围、标准差、MAD 和相对离散度。Renderer trace 复用 WDIO 已建立的主页面 CDP session，不改变正式包调试策略；使用 16 MiB buffer、30 秒停止/flush/读取硬期限和 64 MiB 产物上限。它与 CPU/heap profile 互斥、只允许单次诊断，并由 comparator 拒绝进入正式 A/B。

双向大 Body 优化前 trace（前端报告 `2026-07-23T03-47-54-677Z`）测得 6 个 Renderer Long Task、总计 `401.211 ms`，与 Long Task Observer 的 `397 ms` 基本一致。其中三次 `Receive mojo reply` 共 `180.771 ms`；其余帧任务包含 React `mouseover` 分发、`UpdateLayoutTree`、`Layout` 与 `Paint`。修复虚拟表格 hover 全可见单元格失效后，相同诊断（`2026-07-23T03-55-46-989Z`）中 React `EventDispatch` 从 `50.297` 降到 `14.850 ms`，相关函数调用从 `49.719` 降到 `14.390 ms`（约 `-71%`），trace Long Task 总时长 `401.211 → 361.068 ms`。IPC 回复 `180.771 → 184.693 ms` 基本不变，因此下一热点已经从 React hover 扇出收敛到 IPC 回复及布局/绘制。

首轮候选 `--repeat 3` 位于前端 `matrices/body-2026-07-23T04-00-31-083Z`。三次都完成 120/120 流量，无缺口/重复、正文校验或清理错误；中位数与范围为：吞吐 `36.45 [31.69..39.51] req/s`、首次可见 `669 [492..684] ms`、Query 往返 p95 `88.6 [72.7..129.3] ms`、request → React p95 `1209 [1109..1210] ms`、Long Task 总时长 `357 [295..396] ms`、Yak 峰值 RSS `882.9 [877.9..884.3] MB`。它证明重复管线可用，也暴露 WSL 短时指标的明显噪声；由于优化前没有相同的三次样本，这组不能冒充固定硬件正式 A/B。

#### Phase 1 IPC/layout 排除实验与第五轮证据（2026-07-23）

Renderer trace 现已进一步记录 native task 来源、耗时最高的嵌套事件、IPC payload/data bytes 和 Layout element/dirty/total object、`partialLayout`、layout roots。关闭 `IncludeSystemTiming` 的诊断运行（前端报告 `2026-07-23T04-20-47-726Z`）仍有 3 次 IPC task、合计 `171.066 ms`，与开启时的 `184.693 ms` 同量级，因此后端有界观测字段不是 IPC 长任务根因，默认继续开启。

MITM-only protobuf 默认字段压缩在合成对象上能明显缩小数据，但真实 trace 没有降低 IPC task；约 160 B 的 reply 仍需要 `65～68 ms`。因此实验代码已经撤回，不修改全局 proto loader、gRPC 响应契约或其他调用方。前端 `contain: layout` 也没有形成局部 layout root，额外的 Chromium invalidation tracking 则产生约 4.3 秒 Long Task 的严重观测扰动，两者都已撤回。

保留项是仅把 MITM 虚拟表格 overscan 从 10 调到 5，其他页面不变。候选 trace（前端报告 `2026-07-23T04-55-48-958Z`）中 `UpdateLayoutTree` element count `2204 → 1864 (-15.4%)`、dirty objects `2907 → 2462 (-15.3%)`、累计耗时 `81.494 → 49.580 ms (-39.2%)`；E2E 在性能窗口外自动滚动到 `1120 px` 并回顶，三次都验证首行 `120 → 84 → 120`，没有虚拟列表空窗或恢复错误。

三次基线 `body-2026-07-23T04-00-31-083Z` 与三次候选 `body-2026-07-23T04-59-24-854Z` 的中位数为：Long Task `357 → 166 ms (-53.5%)`、阻塞占比 `9.89% → 4.41%`、request → React p95 `1209 → 1130 ms`、response → React p95 `1073 → 1034 ms`、吞吐 `36.45 → 39.18 req/s`。同时首次可见 `669 → 765 ms`、Query 往返 p95 `88.6 → 112.9 ms` 反向变化；前者候选 CV 为 `31.4%`，后者主要由高波动的后端 query 阶段主导。路线图不把这轮描述为全指标改善，也不把 Renderer 收益归因给后端。

下一步按 Phase 1 的第 4 项分别拆分 gRPC/protobuf 解码、Main 对象构造、Main→Renderer structured clone、Renderer 状态提交和后端 query/conversion，并记录每批对象大小。固定硬件上必须各跑优化前/后至少 3 次，再决定是否进入 Phase 2 shadow 快路径。当前端到端可见延迟仍未达到路线图 SLO。

#### Phase 1 查询、写库队列与 Duplex 第六轮证据（2026-07-23）

`QueryHTTPFlows` 已把 `COUNT`、数据查询、模型转换和总耗时分别记录。MITM 顶部增量请求新增显式 `SkipTotal`，仅初始化、筛选/历史请求和每 10 秒校准保留精确总数。相同前端构建与双向大 Body 场景的三次基线 `body-2026-07-23T05-59-45-490Z` 和候选 `body-2026-07-23T06-06-12-227Z` 显示：后端 Query p95 中位数 `53.757 → 2.650 ms`，其中 COUNT `51.056 → 0.232 ms`，COUNT 执行比例 `1.0 → 0.2`。首次可见 `725 → 604 ms`，但 request → React p95 `1090 → 1152 ms`、Query 往返 `79.7 → 130.3 ms` 反向波动，因此只把它认定为消除精确 COUNT 竞争的局部后端收益，不宣称端到端全面改善。

异步写库内部调用现会在入队时绑定当时的项目 DB；48 条调度批次改为只贪心排空已经就绪的任务，不再固定等待 10 ms，也没有伪造事务批量收益。40960 容量、项目关闭时队列处理、外部直接写 channel 的兼容语义和明确的阻塞/丢弃策略仍未完成。Duplex 广播改为每客户端一个容量 128 的串行发送队列，全局锁外入队；高频失效通知队满时只保留同类型最新值，普通消息最多等待 50 ms 并记录丢弃，慢客户端不能再持有全局锁阻塞其他连接。定向单测、真实 gRPC 用例、全包测试和 race 均通过；合成慢发送 microbenchmark 的广播调用方耗时从约 `1.083 ms/op` 降到约 `22 ns/op`，该数字只证明调用方隔离，不代表网络端到端延迟。

以 SkipTotal 候选作为共同基线，再比较写库/Duplex 分组候选 `body-2026-07-23T06-43-03-849Z`，三次中位数如下：

| 指标 | 分组优化前 | 分组优化后 | 结论 |
| --- | ---: | ---: | --- |
| 写库队列等待 p50 | `36 ms` | `9 ms` | `-75.0%` |
| 写库队列等待 p95 | `130 ms` | `142 ms` | `+9.2%`，尾部仍受 SQLite 写竞争影响 |
| 实际写入 p95 | `55 ms` | `36 ms` | `-34.5%` |
| Duplex 投递 p95 | `142 ms` | `113 ms` | `-20.4%` |
| persist → React p95 | `868 ms` | `820 ms` | `-5.5%` |
| request → React p95 | `1152 ms` | `1065 ms` | `-7.6%` |
| response → React p95 | `1024 ms` | `977 ms` | `-4.6%` |
| 吞吐 | `36.64 req/s` | `37.33 req/s` | 基本持平 |
| 首次可见 | `604 ms` | `662 ms` | `+9.6%`，反向波动 |
| trigger → Query p95 | `1011.5 ms` | `994.2 ms` | 基本不变 |

三次候选都完成 120/120 流量，无缺口、重复、正文损坏或清理错误。最关键的结论是 `trigger → Query` 仍约 1 秒，已经显著大于 SQL、Duplex 与 React 提交阶段，现有数据库 watcher 的固定周期仍是“流量不跟手”的主导项。Phase 2 Shadow 由此进入下一实施阶段；旧通知和查询路径继续保留，不提前删除兼容链路。

#### Phase 1 SQLite 并发排除实验与 GORM 基线（2026-07-23）

SQLite 连接并发候选均使用双向大 Body、120 请求、并发 8、每组 3 次严格串行 Electron 样本。通用写连接池从 1 增加到 2 时，后端 Query p95 中位数 `28.497 → 10.744 ms`，但写库队列 p95 `51 → 89 ms`、实际写入 p95 `31 → 52 ms`、persist → React `228 → 255 ms`、Long Task `209 → 368 ms`，吞吐和 request → React 均基本不变。对应矩阵为前端 `body-2026-07-23T08-54-59-369Z` 与 `body-2026-07-23T08-46-35-937Z`。

“单写连接 + 独立只读连接”在合成 SQLite benchmark 中能消除连接池等待，但真实 Electron A/B 没有转化为稳定体感收益。同当前二进制的 read0/read1 三次中位数为：Query p95 `126.464 → 100.678 ms`，同时写库队列 p95 `62 → 113 ms`、persist → React `307 → 359 ms`、Long Task `549 → 754 ms`，request → React 和吞吐基本持平。对应矩阵为 `body-2026-07-23T09-35-44-065Z` 与 `body-2026-07-23T09-26-00-239Z`。WSL 时间漂移明显，因此这些数据只足以拒绝候选，不能作为发布门禁；产品默认继续保持写连接 1、独立读连接 0。

后端已同步到 `origin/main@8d813bd6d`，主干现已包含 `enhance/db/gorm-create-in-batches` 与 GORM 升级相关改动。同步前创建的两个安全 stash 继续保留；现有 MITM 性能工作树已在新主干上恢复，并完成 SSA、CVE、yakgrpc、yakit 与 `cmd/yak-mitm-perf` 的低并发编译/聚焦测试。

GORM fork `/home/go0p/code/go/gorm` 的正确性修复已提交为 `70430b4`，仅该仓库被推送；`master` 与注解 tag `v1.9.2-yaklang.2` 均已在远端，Yaklang `go.mod` 也已切到该 tag。修复覆盖 SQLite 精确 ID 回填、数据库默认值、混合显式/自动 ID、nil 回滚、实际列形状和 bind 上限；全包测试与定向 race 通过。500 行三次微基准中，正确实现的批插中位数约 `2.77 ms`，错误旧实现约 `2.85 ms`，分配字节约 `1.96 → 1.47 MB`；逐条 `Create` 中位数约 `11.41 ms`。该结果证明 API 已可用于后续候选，不代表 MITM 应立即切批量写入。

若后续引入 MITM typed batch lane，验收必须同时覆盖：每条流量获得真实数据库 ID、项目绑定不串库、after-save/`FlowCommitted` 只在提交后触发、单条失败的事务与重试语义、队列水位，以及 write/persist → React/吞吐的三次真实 Electron 前后对比。仅有 `CreateInBatches` 微基准提升不构成启用依据。

#### Phase 1 HTTPFlow 只读 Body View 与第七轮证据（2026-07-23）

第二份 heap profile 继续显示 `CreateHTTPFlow` 为大 Body 主要累计分配调用链。它只读取 Body 长度、Content-Type、charset 和截断条件，却多次调用必须返回独立 Body 副本的兼容切包 API。后端因此新增语义显式的只读 `SplitHTTPHeadersAndBodyFromPacketView(Ex)`：旧 API 的独立副本契约不变，View 只允许在调用栈内只读使用；真正发生截断时仍显式分配结果，避免输入包被修改。当前只迁移 `CreateHTTPFlow`、bare metadata 与 large-request spill 三处已确认的只读调用方。

自动化微基准 `BenchmarkCreateHTTPFlowBodyMatrix64K256K` 在同一工作树、每次 20 次迭代的 3 组样本中，将单次建流分配从约 `6.10 MiB/op、768 allocs/op` 降到约 `4.79 MiB/op、758～759 allocs/op`，分配字节减少约 `21.5%`；耗时中位数约 `4.99 → 5.07 ms/op`，视为持平噪声，不宣称 CPU 收益。256 KiB 切包微基准中，兼容复制 API 与只读 View 的中位数分别约为 `64.1 µs / 267,750 B/op / 32 allocs` 和 `3.7 µs / 5,603 B/op / 31 allocs`，View 分配字节减少 `97.9%`。

相同 120 请求、并发 8、Request 64 KiB、Response 256 KiB 的 heap 诊断从前端报告 `2026-07-23T02-54-58-894Z` 对比到 `2026-07-23T14-14-57-009Z`：

| 指标 | 兼容切包优化后 | HTTPFlow Body View | 变化 |
| --- | ---: | ---: | ---: |
| 窗口累计分配 | `2,999,089,540 B` | `2,764,771,622 B` | `-234,317,918 B / -7.8%` |
| Split flat 分配 | `672,897,530 B` | `449,617,316 B` | `-33.2%` |
| `CreateHTTPFlow` cumulative | `812,470,067 B` | `573,600,442 B` | `-29.4%` |
| 结束时绝对 live heap | `283,032,394 B` | `273,422,821 B` | `-3.4%` |

`io.ReadAll` 基本不变（`769,543,481 → 776,202,307 B`），说明这次改动命中了预期的建流重复副本，也把下一后端主战场明确收敛到响应读取/转储链路，而不是继续扩大 View 的使用范围。输入不变性、跨 4 KiB 边界、复制/View 所有权、完整 `common/utils/lowhttp` 与 `common/yakgrpc/yakit` 包测试和定向 race 均通过。

无 profiler 的严格串行三次候选矩阵为前端 `body-2026-07-23T14-22-29-748Z`，对照当前 Phase 3 canary 基线 `body-2026-07-23T13-47-17-325Z`。三次均完成 120/120，正文、数据库唯一性、stream Sequence/Gap/重复/乱序与进程清理全部正确。中位数为吞吐 `35.61 → 37.76 req/s`、request → React `1054 → 861 ms`、response → React `838 → 738 ms`、Yak CPU `202.98% → 202.74%`、Yak 峰值工作集 `851.3 → 858.2 MiB`。同时写库队列 p95 `54 → 163 ms`、写入 p95 `42 → 69 ms`、Long Task `337 → 428 ms` 反向波动；因此保留确定的分配收益，但不把 WSL 短样本包装成全链路提速，默认 stream 仍保持 `shadow`。比较器新增显式 case-config 白名单，以记录旧基线缺少、候选补报的同值 `700 ms` 调度元数据，其他配置仍必须完全一致。

下一步先为 `readHTTPResponseFromBufioReader` 的 `io.ReadAll`、`bytes.Buffer` 扩容和 raw/body 双份持有补独立 ownership 测试及 Body 尺寸基准，再做一次最小候选；不得直接用共享切片改变现有调用方可修改/持有 Response 的语义。

#### Phase 1 Content-Length 响应读取与第八轮证据（2026-07-23）

响应读取基准与 ownership 测试补齐后，确认 256 KiB Content-Length 响应会依次经历 `io.ReadAll` 几何扩容、raw packet Buffer、`rsp.Body` Buffer 和 httpctx 独立 bare packet。最小候选只处理两点：不超过 1 MiB 的 Content-Length 使用一次有界精确分配，超过阈值继续渐进读取，避免信任恶意超大长度；刚读出的独立 Body 直接由只读 reader 持有，不再复制进第二个 Buffer。bare packet 仍由 httpctx 独立克隆，输入 packet、`rsp.Body` 和可修改 bare response 互不别名；短读继续用换行补足历史 Content-Length 行为。

同一机器、每次 100 次迭代的三组微基准中位数为：

| 路径 | 优化前 | 优化后 | 变化 |
| --- | ---: | ---: | ---: |
| 网络 reader，耗时 | `421.8 µs/op` | `192.3 µs/op` | `-54.4%` |
| 网络 reader，分配 | `1,992,237 B/op / 84 allocs` | `806,019 B/op / 60 allocs` | 字节 `-59.5%` |
| 已有 bytes 二次解析，耗时 | `368.5 µs/op` | `123.3 µs/op` | `-66.5%` |
| 已有 bytes 二次解析，分配 | `1,721,486 B/op / 78 allocs` | `535,308 B/op / 55 allocs` | 字节 `-68.9%` |

真实 heap 报告为前端 `2026-07-23T14-47-27-344Z`，对照 Body View 报告 `2026-07-23T14-14-57-009Z`：窗口累计分配 `2,764,771,622 → 2,339,609,176 B (-15.4%)`，`io.ReadAll` flat `776,202,307 → 358,823,143 B (-53.8%)`，`readHTTPResponseFromBufioReader` cumulative `739,646,392 → 328,783,745 B (-55.5%)`，`bytes.growSlice` `615,727,253 → 552,722,791 B (-10.2%)`。绝对 live heap `273,422,821 → 265,137,425 B (-3.0%)`，正向 live delta 也从 `24,698,464` 降到 `22,258,108 B`。相对首份大 Body profile，累计分配已从 `5,395,723,287` 降至 `2,339,609,176 B (-56.6%)`。

无 profiler 三次候选矩阵为前端 `body-2026-07-23T14-53-28-951Z`，同配置基线为 `body-2026-07-23T14-22-29-748Z`，机器比较位于 `http-response-body-read-2026-07-23`。三次均完成 120/120，正文、数据库唯一性、stream Gap/顺序/重复与清理错误全部为 0。中位数为吞吐 `37.76 → 39.90 req/s (+5.7%)`、Yak 峰值工作集 `858.2 → 832.5 MiB (-3.0%)`、Long Task `428 → 292 ms (-31.8%)`、Yak CPU `202.7% → 204.7% (+1.0%)`；request → React `861 → 879 ms (+2.1%)` 和 response → React `738 → 801 ms (+8.5%)` 反向变化，但两组范围重叠。结论限定为响应读取分配优化通过，UI 延迟仍需固定硬件复验。

完整 `common/utils`、`common/utils/lowhttp`、`common/minimartian`、ownership/短读测试和定向 race 均通过。下一可测热点是 `DumpHTTPResponse` 的 Body 读取与恢复（本场景 cumulative 约 `216 MB`），必须作为独立候选验证外部 `rsp.Body` 恢复语义，不能与本轮合并。

#### Phase 1 DumpHTTPResponse Body 恢复与第九轮证据（2026-07-23）

`DumpHTTPResponse` 原先总是用 `io.ReadAll` 把 `rsp.Body` 复制到临时切片，再复制进返回 packet，最后把 Body 恢复为新 reader。新快路径只识别响应解析器内部的不可变 owned Body：读取剩余只读 view 时仍把原 reader 推进到 EOF，dump 返回包继续独立复制，函数退出后 `rsp.Body` 恢复到 dump 前尚未读取的内容。外部自定义 Body 完全保留原 `io.ReadAll` 回退。部分预读、原 Body 被消费、恢复内容、dump 输出不别名、chunked 恢复和外部 Body 回退均有自动化测试；内部类型只暴露 `ReadCloser/WriterTo`，不向调用方提供可变底层切片。

256 KiB、每次 100 次迭代的三组微基准中位数从约 `298.0 µs / 1,465,820 B/op / 38 allocs` 降到 `64.4 µs / 274,910 B/op / 16 allocs`，耗时减少 `78.4%`、分配字节减少 `81.2%`。同时在写入大 Body 前按实际输出长度为 dump buffer 预留容量，避免几何扩容；输出 packet 与恢复后的 Body 仍独立。

真实 heap 报告为前端 `2026-07-23T15-10-24-871Z`，对照响应读取候选 `2026-07-23T14-47-27-344Z`：累计分配 `2,339,609,176 → 2,157,080,041 B (-7.8%)`，`DumpHTTPResponse` cumulative `216,439,900 → 69,928,653 B (-67.7%)`，`io.ReadAll` flat `358,823,143 → 198,994,567 B (-44.5%)`。正向 live delta `22,258,108 → 4,510,483 B`，但结束绝对 live heap `265,137,425 → 278,683,416 B (+5.1%)`；后者仍低于此前 `283,032,394 B`，只记录为单次 profile 波动，不宣称 live heap 改善。相对首份大 Body profile，累计分配已从 `5,395,723,287` 降到 `2,157,080,041 B (-60.0%)`。

无 profiler 三次候选矩阵为前端 `body-2026-07-23T15-15-43-104Z`，同配置基线为 `body-2026-07-23T14-53-28-951Z`，比较报告位于 `http-response-dump-body-2026-07-23`。三次均完成 120/120，正文、数据库、stream 顺序与清理门禁全部通过。中位数为 Yak RSS `832.5 → 808.5 MiB (-2.9%)`、Yak CPU `204.7% → 203.6% (-0.6%)`、request → React `879 → 846 ms (-3.8%)`、response → React `801 → 754 ms (-5.9%)`、Long Task `292 → 283 ms (-3.1%)`；吞吐 `39.90 → 38.84 req/s (-2.7%)`，网络请求 p95 `414.7 → 415.4 ms` 基本不变。保留确定的分配收益，同时保留吞吐风险记录。

下一轮回到新 heap top 的 `bytes.growSlice` 与剩余兼容 `SplitHTTPPacketEx` 调用方；只迁移调用栈内确定只读的匹配/元数据路径，任何返回值所有权不明确的调用方继续使用复制 API。

#### Phase 1 染色 Body View 排除实验与第十轮证据（2026-07-23）

曾单独尝试只让同步 `prepareColorMatch` 使用 Body View，公共 `SplitPacket/MatchPacket` 继续保持独立副本。256 KiB 局部微基准从约 `1.374 MiB/op、154 allocs、1.181 ms/op` 降到 `1.112 MiB/op、153 allocs、1.068 ms/op`，分配字节减少 `19.1%`；复制/View 结果等价、公共所有权与全包/race 测试均通过。

但真实 heap `2026-07-23T15-32-43-017Z` 相对 dumper 基线 `2026-07-23T15-10-24-871Z` 只把 `HookColor/prepareColorMatch` cumulative 从 `217,325,436` 降到 `199,335,421 B (-8.3%)`、`splitHTTPPacketEx` flat 从 `445,604,334` 降到 `406,577,495 B (-8.8%)`，全窗口累计分配反而 `2,157,080,041 → 2,165,879,116 B (+0.4%)`，没有整体收益。

正式三次候选 `body-2026-07-23T15-37-29-530Z` 对照 `body-2026-07-23T15-15-43-104Z` 更明确失败：吞吐 `-5.3%`、request/response → React `+8.2%/+7.3%`、Long Task `+21.6%`，Yak CPU/RSS 分别约 `+1.6%/+1.0%`。因此该实现和专用测试/基准已完整撤回，报告继续保留为排除证据；当前有效代码回到第九轮 dumper 候选。下一步不再按 flat 排名机械扩大 View，而是拆分 `bytes.growSlice` 的具体调用来源，并优先验证不改变返回所有权的容量规划或算法消除。

#### Phase 1 只读 Header helper 与第十一轮证据（2026-07-23）

`bytes.growSlice` 调用链进一步拆分后，发现兼容 `SplitHTTPPacket` 的剩余 Body 副本并不都来自需要 Body 的调用方：`GetHTTPPacketHeaders` 链路约占 `89 MiB`，`GetStatusCodeFromResponse` 约占 `32.6 MiB`。这组 helper 只返回字符串、map 或状态码，既不返回也不修改 Body，因此只迁移 7 个 header/cookie/content-type/status 读取 helper 到现有只读 View 路径；公开切包和 `GetHTTPPacketBody` 的独立副本语义保持不变。

新增等价测试和 `BenchmarkHeaderOnlyHelpersLargeBody`。256 KiB Body 下，headers 读取中位数约从 `52 µs / 269,715 B/op / 67 allocs` 降到 `7.1 µs / 7,568 B/op / 66 allocs`，分配字节减少约 `97.2%`；status 读取从约 `49 µs / 269,459 B/op` 降到 `7.1 µs / 7,312 B/op`。既有 header、cookie、content-type、status 测试、定向 race 和完整 `common/utils/lowhttp` 回归均通过。

与 dumper 基线同为专用流 `canary` 的 heap 报告是前端 `2026-07-23T16-17-33-098Z`。相对 `2026-07-23T15-10-24-871Z`，全窗口累计分配 `2,157,080,041 → 2,017,235,444 B (-6.5%)`，`splitHTTPPacketEx` flat `445,604,334 → 291,504,901 B (-34.6%)`、cumulative `476,085,908 → 329,841,601 B (-30.7%)`；结束绝对 live heap `278,683,416 → 277,031,840 B (-0.6%)`，视为持平。相对首份大 Body profile，累计分配已从 `5,395,723,287` 降到 `2,017,235,444 B (-62.6%)`。

无 profiler 的严格三次候选为 `body-2026-07-23T16-11-29-679Z`，对照 `body-2026-07-23T15-15-43-104Z`，比较报告为 `http-header-readonly-view-2026-07-23`。三次均完成 120/120，Body、数据库、stream 顺序和清理门禁通过；吞吐 `+1.3%`、Yak RSS `-3.7%`、Yak CPU `-0.2%`、Long Task `-0.4%`。request/response → React 分别 `+5.0%/+7.3%`，写库 p95 `36 → 43 ms`，均保留为反向波动风险，不描述成端到端延迟改善。一次误用默认 `shadow` 的三次矩阵和 heap 被 comparator 的配置一致性门禁识别，只作为额外正确性证据，不纳入正式 A/B；产品默认仍为 `shadow`。

下一候选转向仍需 Body 的 `FixHTTPPacketCRLF`、解压和 response raw capture 路径。任何 View 迁移必须先锁定只读性与输出独立性；持续高压、慢消费者和 Chromium/nuclei 场景仍优先于切换默认流模式。

#### Phase 1 FixHTTPPacketCRLF 只读 Body 与第十二轮证据（2026-07-23）

`FixHTTPPacketCRLF` 的输入 Body 只用于长度、chunked/multipart 解析和写入新结果，既不原地修改也不直接返回。实现保留内部 `cloneBody` oracle：生产入口使用只读 View，测试可运行旧复制路径并逐字节比较。普通 256 KiB Content-Length、`noFixLength`、chunked + pipeline rest、multipart 均输出一致，输入未变且结果可独立修改；既有 CRLF 测试、定向 race 和完整 lowhttp 回归通过。

`BenchmarkFixHTTPPacketCRLFLargeBody` 中，旧复制路径约为 `131 µs / 538,745 B/op / 49 allocs`，View 路径约为 `70 µs / 276,576 B/op / 48 allocs`，分配字节减少 `48.7%`、耗时约减少 `46%`。同为 canary 的 heap 报告 `2026-07-23T16-27-08-323Z` 对照上一轮 `2026-07-23T16-17-33-098Z`：`FixHTTPPacketCRLF` cumulative `95.13 → 52.61 MiB (-44.7%)`，全窗口累计分配 `2,017,235,444 → 1,962,799,243 B (-2.7%)`，结束绝对 live heap `277,031,840 → 273,042,814 B (-1.4%)`。相对首份大 Body profile累计分配已下降 `63.6%`。

首次三次候选 `body-2026-07-23T16-31-39-456Z` 对较早的上一轮矩阵时，Renderer Long Task 与网络请求 p95 反向波动超过 15%，因此没有直接判定通过。逐样本离散较大且与微基准方向相反，随后只切内部 oracle 做了紧邻 A/B：Body-copy A 为 `body-2026-07-23T16-38-23-835Z`，Body-view B 为 `body-2026-07-23T16-45-53-696Z`，比较报告为 `http-fix-crlf-body-view-paired-2026-07-23`。两组三次均通过全部正确性与清理门禁；B 的吞吐 `+11.9%`、网络请求 p95 `-22.3%`、首次可见 `-20.9%`、Long Task `-21.1%`、Yak RSS `-2.4%`，Yak CPU `+0.5%` 基本持平。request/response → React 分别 `+4.3%/+4.0%`，Query p95 波动仍大，继续作为风险而非改善结论。最终代码恢复并保留 Body-view 路径，临时 A 构建仅存在于 source-hash 隔离报告中。

下一候选只读审计 `_unzipPacketEncodingInternal`；response raw capture 涉及精确 wire packet、parser Body 和 httpctx bare packet 三份所有权，风险更高，必须晚于持续/慢消费者基线或单独设计协议级 ownership 测试。

#### Phase 1 自动解压 Body View 排除实验与第十三轮证据（2026-07-24）

自动解压会检查每个报文；旧路径即使发现没有 Content/Transfer-Encoding、最终直接返回原包，也会先复制完整 Body。内部 copy/view oracle 在 256 KiB 未编码报文微基准中约从 `45 µs / 268,105 B/op / 41 allocs` 降到 `4.2 µs / 5,958 B/op / 40 allocs`，分配字节减少 `97.8%`；无编码、gzip、chunked、保守/非保守失败、输入不变与成功输出不别名测试及完整 lowhttp/race 均通过。

但同配置 canary heap `2026-07-24T01-40-41-012Z` 对照当前有效基线 `2026-07-23T16-27-08-323Z` 只把目标 `_unzipPacketEncodingInternal` Body-copy 从 `37.69` 降到 `1.50 MiB`，全窗口累计分配反而 `1,962,799,243 → 1,966,690,729 B (+0.2%)`，结束绝对 live heap也有反向单次波动，未形成整体收益。

因此直接使用紧邻 copy/view 3+3：A 为 `body-2026-07-24T01-45-22-529Z`，B 为 `body-2026-07-24T01-51-58-570Z`，比较报告 `http-auto-unzip-body-view-paired-2026-07-24`。六次均正确且清理完成，但 B 的吞吐 `-1.3%`、Long Task `+25.3%`、request → React `+11.9%`、persist write p95 `+63.3%`；Yak CPU/RSS 仅约 `-1.9%/-0.9%`。局部分配收益不足以覆盖端到端风险，生产改动和专用 oracle/基准已完整撤回，报告保留为排除证据。当前有效代码仍为第十二轮 `FixHTTPPacketCRLF` 候选。

下一阶段不再连续扩大 Body View，而是先补持续生产、慢消费者和大 Body 阶梯基线；response raw capture 必须有精确 wire/bare/body 三方 ownership 与畸形响应回退测试后才允许实验。

#### Phase 1 定速观测、HTTP 解析与 Linux 进程归属第十四轮证据（2026-07-24）

自动化定速场景升级到 harness v7：生产器按计划开始时间而不是请求完成时间发压，负载窗口前原子清空计数，并显式断言 committed shadow 的初始快照没有混入本轮数据。这样可以区分“代理吞吐不足”和“生产器自身被响应延迟限速”，也避免重连快照污染 backlog、delivery 与 Query 对账指标。定速矩阵只顺序运行，默认实时模式仍为 `shadow`；它是测量能力，不改变产品协议或刷新参数。

后端同时完成两组连接热路径优化。HTTP 行读取在常见单缓冲行上直接返回，request parser 的临时对象进入有界复用池；等价、长行、错误和并发测试保留。Linux 客户端进程归属由“只按源地址/端口 dump 整张 inet_diag 表”改为优先查询连接的精确源/目的 4-tuple，并复用最多 16 个 netlink 连接；不支持精确查询的内核或特殊 `net.Conn` 自动回退旧路径。`/proc/<pid>/fd` 搜索则按 32 个名称分批读取并用 `readlinkat`，命中后立即停止，不再为每个 PID 物化全部 `DirEntry` 和绝对路径。

局部基准中，inet_diag source-only 中位数约从 `488 µs / 11,048 B/op / 75 allocs` 降到精确查询的 `134 µs / 5,752 B/op / 43 allocs`；`/proc` 已命中扫描约从 `52.5 µs / 8,301 B/op` 降到 `40.8 µs / 1,369 B/op`，未命中约从 `147 µs / 14,836 B/op` 降到 `114 µs / 2,586 B/op`。CPU/heap 调用链分别减少约 `29%～65%`。严格三次 Electron 比较位于前端：

- `matrices/body-2026-07-24T05-00-04-439Z/comparison-vs-before-exact-netlink.{json,md}`；
- `matrices/body-2026-07-24T05-17-09-686Z/comparison-vs-before-fd-scan.{json,md}`。

精确 4-tuple、回退、IPv4/IPv6、netlink 连接损坏丢弃、PID/FD 消失竞态及定向 race 均有自动化覆盖。已有 `TestProcessesWatcher_Start` 是手工 10 分钟观察测试，不纳入自动回归；跨平台编译仍受仓库既有 PCRE2/CGO 条件限制，不把该限制归因于本轮代码。

#### Phase 1 无启用规则的 HookColor 快路径第十五轮证据（2026-07-24）

真实大 Body profile 发现默认未启用染色规则时，HTTPFlow 仍进入 `HookColor` goroutine/channel、切包和 Body 扫描。`HaveRules` 现在只在存在启用规则时返回 true；HTTP、LowHTTP 和 WebSocket 的空颜色/空标签写入是 no-op。若规则恰在匹配后热更新清空，快路径仍保留请求上下文中已经命中的颜色与标签，因此没有牺牲热更新竞态语义。启用规则、禁用/空规则、预过滤等价、元数据保留及 race/full-package 测试均通过。

64 KiB 无规则微基准约从 `36～39 µs / 214,452 B/op / 35 allocs` 降到 `76 ns / 16 B/op / 1 alloc`。大 Body heap 从 `1.956 → 1.815 GB (-7.2%)`，约 `207 MB` 的 HookColor 分配和约 `77 MB` 的 `bytes.Replace` 调用消失；CPU profile 总样本 `5.85 → 5.15 s (-12.0%)`、GC flat `2.41 → 2.08 s (-13.7%)`。严格 3+3 为基线 `body-2026-07-24T06-00-23-198Z`、候选 `body-2026-07-24T06-08-31-743Z`，比较文件为候选目录下 `comparison-vs-before-hookcolor.{json,md}`；吞吐中位数 `47.105 → 49.155 req/s (+4.4%)`、Yak RSS `845.85 → 810.87 MiB (-4.1%)`，request p95 中位数反向约 `+5.2%` 且样本分布重叠，继续作为风险记录。

#### Phase 1 HTTPFlow 流式兼容哈希第十六轮证据（2026-07-24）

`HTTPFlow.CalcHash` 原先通过 `fmt.Sprintf` 先物化包含完整 Request 的历史字段字符串，再转为 SHA-1 输入。新实现逐字段写入 hasher，保持旧 `[field field ...]` 字节格式和 SHA-1 结果完全一致，并只读使用字符串底层字节。256 组包含控制字符、无效 UTF-8 和随机字段的差分测试锁定兼容性。

64 KiB Request 微基准约从 `83.7 µs / 222,030 B/op / 21 allocs` 降到 `48.3 µs / 96 B/op / 2 allocs`。heap `1.815 → 1.648 GB (-9.2%, -167 MB)`，其中约 `116.8 MB` 的 CalcHash 分配消失。严格 3+3 基线为 `body-2026-07-24T06-08-31-743Z`，候选为 `body-2026-07-24T06-26-54-407Z`，比较文件为 `comparison-vs-before-streaming-hash.{json,md}`；吞吐 `49.155 → 57.748 req/s (+17.5%)`、请求 p95 `331.8 → 257.7 ms (-22.3%)`、request → React `1075 → 946 ms (-12.0%)`、Yak RSS `-1.4%`，全部正确性与配置一致性门禁通过。

#### Phase 1 POST 公共参数单次 Body 读取第十七轮证据（2026-07-24）

`GetPostCommonParams` 旧控制流会为 JSON、XML、form 依次读取并恢复同一 Request Body。现在只读取/恢复一次，再把同一份 owned bytes 交给内部解析器；普通二进制若不含 `{`、`[`、`%` 可跳过 JSON 字符串转换与 URL-unescape，不含 `<` 可跳过 XML parser。公开 `GetPostJsonParams/GetPostXMLParams/GetPostParams` 的行为保持不变。空 Body、URL-escaped JSON、XML、重复 form、嵌套 JSON、Base64 JSON、可打印和不可打印二进制均与旧控制流做参数签名差分；重复调用 Body 保持、race 和完整 `common/mutate` 回归通过。特别保留了 `application/octet-stream` 可打印 Body 仍生成一个 POST 参数的历史语义。

64 KiB 二进制微基准约从 `1.37 ms / 1,182,900 B/op / 56 allocs` 降到 `0.451 ms / 656,290 B/op / 23 allocs`。heap `1.648 → 1.550 GB (-6.0%, -98 MB)`，`GetPostCommonParams` cumulative `143.7 → 85.3 MB (-40.6%)`；CPU profile 总样本 `5.15 → 4.50 s (-12.6%)`、GC flat `2.08 → 1.74 s (-16.3%)`、目标函数 `290 → 150 ms (-48.3%)`。严格 3+3 基线为 `body-2026-07-24T06-26-54-407Z`，候选为 `body-2026-07-24T06-44-07-916Z`，比较文件为 `comparison-vs-before-single-body-read.{json,md}`。候选吞吐中位数 `-2.9%`、request p95 `+19.6%`，但吞吐均值仅约 `-1%`、后端 CPU/GC/heap 均稳定正向且候选离散更小；因此保留确定性分配优化，同时明确不宣称本轮 UI 尾延迟改善。

#### Phase 1 `ReadHTTPRequestFromBytes` 精确 Body 与所有权转移第十八轮证据（2026-07-24）

profile 显示请求二次解析仍有约 `142 MiB` 的 `io.ReadAll` 几何扩容。`ReadHTTPRequestFromBytes` 的输入固定为 `[]byte`，解析 Body 时可用 `bufio.Reader.Buffered() + bytes.Reader.Len()` 得到精确剩余长度；新路径一次分配并 `ReadFull`，公开流式 `ReadHTTPRequestFromBufioReader` 继续使用原行为。随后把这份新分配、已由 parser 独占的 Body 直接交给 `bytes.Buffer`，不再 `Write` 到第二个 Body Buffer。调用方输入、`req.Body` 和 httpctx bare packet 仍为三份独立存储；测试会分别修改输入和 bare packet，再验证 64 KiB Body 不变，并覆盖 128 并发解析、池释放及 race/full-package。

64 KiB 微基准分两步记录，避免掩盖每个改动的收益：

| 路径 | 耗时中位数 | 分配 | allocs |
| --- | ---: | ---: | ---: |
| 原始 `io.ReadAll + bodyRawBuf.Write` | `97.1 µs` | `500,369 B/op` | `66` |
| 精确剩余长度读取 | `56.4 µs` | `280,974 B/op` | `51` |
| parser-owned Body 直接交接 | `53.5 µs` | `215,485 B/op` | `51` |

第一步 heap 为 `2026-07-24T07-09-42-831Z`，对照 `2026-07-24T06-39-51-437Z`：总分配 `1.550 → 1.488 GB (-4.0%)`、`io.ReadAll` flat `218.9 → 89.9 MB (-58.9%)`、request parser cumulative `244.3 → 174.4 MB (-28.6%)`，live heap约 `+1.0%`。CPU profile `2026-07-24T07-14-30-804Z` 中目标 parser `310 → 210 ms`、`io.ReadAll 270 → 150 ms`，但总样本反向 `4.50 → 5.13 s`，只认目标归因。严格 3+3 基线 `body-2026-07-24T06-44-07-916Z`、候选 `body-2026-07-24T07-17-03-886Z`，比较为 `comparison-vs-before-exact-request-body-read.{json,md}`：吞吐中位数 `+2.7%`、request p95 `-13.9%`、request → React `-4.3%`、Yak CPU/RSS `+1.3%/+0.4%`；Query RTT 与 Renderer drain 有高方差反向样本。

第二步 heap 为 `2026-07-24T07-25-55-429Z`：相对第一步总分配 `1.488 → 1.423 GB (-4.4%)`、request parser cumulative `174.4 → 142.3 MB (-18.4%)`、`bytes.growSlice 488.6 → 442.6 MB (-9.4%)`，live heap `-0.5%`。CPU `2026-07-24T07-30-40-335Z` 相对第一步总样本 `5.13 → 4.16 s`、parser `210 → 140 ms`、Buffer grow `510 → 340 ms`。严格 3+3 基线 `body-2026-07-24T07-17-03-886Z`、候选 `body-2026-07-24T07-33-13-446Z`，比较为 `comparison-vs-before-owned-request-body.{json,md}`：Yak CPU/RSS `-1.2%/-2.4%`、首次可见 `-17.7%`、Query RTT `-39.8%`，但吞吐 `-8.1%`、request → React `+5.8%`、Long Task 中位数 `+95.7%`。后端消除一次独立 Buffer copy 无直接 Renderer 长任务扇出路径，且微基准/heap/CPU 一致正向，因此保留改动并完整记录 WSL 端到端风险，不宣称 UI 全面改善。

#### Phase 1 Response bare packet owned handoff 第十九轮证据（2026-07-24）

`readHTTPResponseFromBufioReader` 的 `rawPacket` 只在 parser 内创建，返回后唯一所有者是 request 的 httpctx；`rsp.Body` 则持有另一份独立的 `responseBody`。因此正常、未超限响应现在通过显式的 owned API 把 `rawPacket` 所有权交给 httpctx，不再先 `bytes.Clone` 一整份响应。外部或共享输入仍必须使用原有 clone setter，超限响应的 header/body 文件路径也保持原行为。ownership 测试会依次修改调用方 packet 与 bare packet，并验证两者互不影响且 `rsp.Body` 始终保持原值；短 Content-Length 补齐、bytes 输入所有权、定向 race 和 `common/utils` 全包均通过。

256 KiB Content-Length 微基准中位数由 `166.3 µs / 806,037 B/op / 60 allocs` 降到 `131.6 µs / 535,669 B/op / 58 allocs`，耗时约 `-20.9%`、分配字节 `-33.5%`。heap `2026-07-24T07-45-15-744Z` 对照 `2026-07-24T07-25-55-429Z`：`bytes.Clone 162.1 → 122.7 MB (-24.3%)`、response parser cumulative `325.8 → 291.5 MB (-10.5%)`、结束绝对 live heap `280.4 → 272.1 MB (-3.0%)`；全窗口累计分配 `1.423 → 1.437 GB (+1.0%)`，因此只认目标 clone 收益。CPU 诊断 `2026-07-24T07-50-09-857Z` 相对 `2026-07-24T07-30-40-335Z` 的总样本 `4.16 → 4.59 s`、response parser `210 → 320 ms`，方向反向且不作为收益结论。

严格 3+3 基线为 `body-2026-07-24T07-33-13-446Z`，候选为 `body-2026-07-24T07-52-15-391Z`，比较文件为 `comparison-vs-before-response-bare-handoff.{json,md}`。配置、诊断与指标覆盖一致，三次均完成正确性和清理；吞吐中位数 `+7.4%`、request → React `-0.7%`、Renderer drain `-14.5%`、Yak CPU p95 `-0.2%`，Yak RSS `+1.5%`。Electron CPU p95 `+19.2%`、后端 Query p95 `+20.4%`，其中 Query 两侧 CV 为 `82%/98%`；这些反向指标继续作为 WSL 风险记录。候选未出现“产品指标与 CPU 同向持续退化”，且微基准与目标 heap 归因一致，因此保留窄 owned handoff，不宣称全链路 CPU 改善。

#### Phase 1 MITMV2 明文响应重复缓存第二十二轮证据（2026-07-24）

`getPlainResponseBytes` 在未修改响应首次解码时已经通过 `SetPlainResponseBytes` 建立独立缓存；旧的 `handleHijackResponse` 随后又无条件调用同一 setter，把完整明文响应再克隆一次。现在未修改响应直接复用已解码缓存，只有进入函数前已经标记为 modified 的响应才重新缓存，从而继续保证 hijacked bytes 与 context cache 不别名。自动化会修改调用方的 modified 切片并验证缓存不变，同时验证未修改缓存的底层地址没有被替换；普通测试和 race 均通过。

256 KiB 微基准中，旧无条件 setter 约为 `38～40 µs / 262,233 B/op / 4 allocs`，未修改缓存快路径约为 `19 ns / 0 B/op / 0 allocs`；modified 分支仍保持约 `38～43 µs / 262,233 B/op / 4 allocs`，没有用共享切片换取性能。单次 heap 报告 `2026-07-24T11-42-26-163Z` 对照 `2026-07-24T07-45-15-744Z`：总累计分配 `1,436,882,660 → 1,395,650,410 B (-2.9%)`，`bytes.Clone` flat `122,690,190 → 90,925,766 B (-25.9%, -31.8 MB)`；结束 live heap `272.1 → 274.3 MB (+0.8%)` 基本持平。单次 CPU profile 总样本反向 `4.59 → 5.24 s`，因此不宣称 CPU profile 改善。

为避免单次 profile 决策，旧无条件缓存与候选按同一源码状态紧邻串行各跑 3 次。基线为 `body-2026-07-24T11-48-49-833Z`，候选为 `body-2026-07-24T11-57-17-023Z`，比较文件位于候选目录的 `comparison-vs-unconditional-plain-response-clone.{json,md}`。六次均完成 120/120、Body/数据库唯一性/stream/清理门禁，比较器状态为 `passed`，配置和诊断差异均为空。中位数为吞吐 `+21.8%`、网络 request p95 `-25.0%`、首次可见 `-32.0%`、数据库/Renderer 排空 `-30.9%/-53.5%`、Long Task `-59.1%`、Yak CPU p95 `-0.2%`；Yak RSS `+2.1%`，request/response/persist → React 分别 `+6.5%/+17.6%/+15.3%`，写库 p95 `+52.0%`。这些交错指标与候选吞吐、样本范围和 WSL 调度共同记录为风险，不描述成 UI 全面提速；候选因确定性消除一次大对象分配、modified 所有权不变且没有 CPU 与产品指标同向持续退化而保留。

#### Phase 1 HTTPFlow 标题 bytes 提取第二十三轮证据（2026-07-24）

最新 heap 行级归因显示，`CreateHTTPFlow` 为提取最多 128 个字符的 HTML title，会先把整个响应 `[]byte` 转成 `string`；双向 256 KiB 场景中该行累计分配约 `35.2 MB`。现在新增 bytes 入口并复用相同的大小写不敏感正则、非法 UTF-8 转义、128 rune 截断和 512 KiB 扫描上限，仅新建 HTTPFlow 的原始响应路径使用；原 string API、`SetResponse`、旧记录 fallback、数据库字段与标题匹配语义保持不变。无标题、大小写、带属性不匹配、超长标题、非法 UTF-8、扫描上限前/后/跨界及输入不变均与 string oracle 差分，focused race、完整 `common/utils`/`common/schema` 和 Yakit 标题持久化回归通过。

256 KiB 微基准 5 次中，旧整包 string 路径约 `44～50 µs / 262,309～262,316 B/op / 2 allocs`，bytes 路径约 `1.68～1.74 µs / 64 B/op / 1 alloc`。heap 候选 `2026-07-24T12-15-05-134Z` 对照上一轮 `2026-07-24T11-42-26-163Z`：目标 title 行从 `35.2 MB` 降到 profile 中不可见，`CreateHTTPFlow` flat `70.4 → 42.5 MB (-39.7%)`、cumulative `397.0 → 358.6 MB (-9.7%)`，总累计分配 `1,395,650,410 → 1,374,974,896 B (-1.5%)`。结束 live heap `274.3 → 282.1 MB (+2.9%)` 反向，只作为单次 profile 风险。

严格 3+3 使用上一轮最终候选 `body-2026-07-24T11-57-17-023Z` 作为 string 基线，bytes 候选为 `body-2026-07-24T12-19-42-548Z`，比较文件位于候选目录的 `comparison-vs-string-html-title.{json,md}`。六次均完成 120/120 和全部正确性/清理门禁，配置与诊断差异为空。中位数为 Yak RSS `-1.1%`、Yak CPU p95 `+0.8%`、吞吐 `-2.8%`、request/response → React `-1.6%/-6.8%`、Renderer drain `-19.2%`、persist write p95 `-47.4%`；Long Task `113 → 161 ms (+42.5%)`，候选范围 `112～215 ms`，必须保留为固定硬件复验风险。该后端改动没有 Renderer 计算扇出，且微基准、heap 与 RSS 一致、CPU/吞吐基本持平，因此保留 bytes 路径，但不宣称 Long Task 或 UI 全面改善。

#### Phase 1 MITMV2 明文请求缓存只读 Body 第二十四轮证据（2026-07-24）

`getPlainRequestBytes` 对 bare request 解码后，只为判断 Body 是否超过 200 KiB 缓存上限，就通过兼容 API 复制一次完整 Body；随后 `SetPlainRequestBytes` 又按 context 所有权要求克隆整包。候选只把长度检查切到显式只读 view，写入 context 的整包克隆、wire packet、parser Body 与 bare/plain packet 的独立所有权都不变。MITM v2 与旧 MITM 的等价入口共用同一 helper；阈值恰好命中、超限不缓存以及修改源 packet 后 cache 不变均有自动化覆盖，focused test/race 和 MITM v2 gzip 解码/转发回归通过。

5 次微基准中，128 KiB 可缓存请求由中位 `45.351 µs / 270,903 B/op / 19 allocs` 降到 `24.427 µs / 139,828 B/op / 18 allocs`，分别约 `-46.1%/-48.4%`；256 KiB 超限请求由 `42.648 µs / 262,619 B/op / 15 allocs` 降到 `0.717 µs / 472 B/op / 14 allocs`，分别约 `-98.3%/-99.8%`。heap 候选 `2026-07-24T13-25-18-259Z` 对照 `2026-07-24T12-15-05-134Z`：`MITMV2.func6` cumulative `27.18 → 15.45 MB (-43.2%)`，其中只为取长度的 Body split `9.57 → 0.50 MB (-94.8%)`；保留的 context owned clone 为 `6.97 MB`。全窗口累计分配 `1,374,974,896 → 1,377,352,386 B (+0.17%)`，结束 live heap `282.1 → 259.6 MB (-8.0%)`，因此只认目标 caller 收益，不把采样噪声描述成全局 allocation 改善。

严格 3+3 基线为 `body-2026-07-24T12-19-42-548Z`，候选为 `body-2026-07-24T13-30-36-835Z`，比较文件为候选目录下 `comparison-vs-cloned-request-body.{json,md}`。六次均完成 120/120 和 Body、数据库、stream、清理门禁，比较器为 `passed` 且配置/诊断差异为空。吞吐 `+2.1%`、网络 request p95 `-1.7%`、request/response → React `-6.2%/-5.2%`、Yak CPU p95 持平；Yak RSS `+3.4%`，首次可见 `+24.0%`、Electron CPU p95 `+21.1%`、Long Task `161 → 226 ms (+40.4%)`、persist write p95 `+40.0%`。候选因确定性移除只读 body clone、所有权不变且后端吞吐/CPU 未退化而保留；反向 Renderer/首显/写库指标继续作为固定硬件复验风险，不宣称 UI 提速。

#### Phase 1 response fix 与限长只读 Body 第二十五轮证据（2026-07-24）

最新 heap 中，`HTTPWithoutRetry` 的限长判断只读取 Body 长度，却通过兼容 API 复制约 `33.0 MiB`；`HTTPWithoutRetry` 和 `resolveHTTPFlowStoredResponse` 又调用 `FixHTTPResponse` 丢弃其 Body 返回值，合计复制约 `62.6 MiB`。现在公开 `FixHTTPResponse` 的独立 Body 语义保持不变，新增只返回独立重组报文的 `FixHTTPResponsePacket`；packet-only 路径以 view 读取普通 Body，对 malformed chunked 则先做防御性 clone，避免旧解码错误预览可能改写输入。限长判断也只在该调用点切 view，真正截断仍由新 buffer 重组。plain/gzip/chunked/100-Continue/错误输入逐字节差分、输入不变、输出独立、旧 Body 独立、lowhttp 全包、Yakit 定向测试及两组 race 均通过。

5 次 256 KiB 微基准中，response fix 由中位 `723.359 µs / 556,261 B/op / 86 allocs` 降到 `680.101 µs / 294,125 B/op / 85 allocs`，约 `-6.0%/-47.1%`；限长 Body 读取由 `45.214 µs / 262,777 B/op / 17 allocs` 降到 `0.894 µs / 618 B/op / 16 allocs`，约 `-98.0%/-99.8%`。heap 候选 `2026-07-24T14-09-50-940Z` 对照 `2026-07-24T13-25-18-259Z`：目标切包分配 `95.67 → 约 1 MiB`，`GetHTTPPacketBody` 的 `33.04 MiB` 从调用树消失，Fix 链 cumulative `198.07 → 121.40 MiB (-38.7%)`；全窗口累计分配 `1,377,352,386 → 1,293,933,355 B (-6.1%)`。结束 live heap `259.6 → 270.4 MB (+4.1%)` 反向，只作为单次采样风险。

严格 3+3 为 `body-2026-07-24T13-30-36-835Z` → `body-2026-07-24T14-15-45-537Z`，比较文件为候选目录的 `comparison-vs-response-body-clones.{json,md}`。六次均完成 120/120 和 Body、数据库、stream、清理门禁，配置/诊断差异为空，实际 Renderer 输入指纹同为 `fa377df501505467d37539e9407e550f7931d1ebf5607b0d6cc4ce176fe3328a`。Yak CPU p95 `-0.7%`、峰值 RSS `-4.4%`、首次可见 `-22.9%`、Long Task `-49.1%`；吞吐 `-8.5%`、网络 request p95 `+9.3%`、request/response → React `+17.9%/+6.5%`、Query p95 `3.153 → 63.438 ms`、Electron CPU p95 `+4.3%`。候选因确定性移除约 95 MiB Body clone、所有权不变且 Yak CPU/RSS 未退化而保留；UI/SQLite 反向项必须在固定硬件复验，不宣称体感提速。

#### Phase 1 response fix provenance owned handoff 第二十六轮证据（2026-07-24）

正常 HTTP/1 响应只有在 `FixHTTPResponsePacket` 成功后才标记 `ResponsePacketFixed`；`NoFixContentLength`、`NoBodyBuffer`、多响应、HTTP/2/3 及修复失败路径都不标记。parser 输入不保留的测试证明后，minimartian 才把该 owned `RawPacket` 移入 httpctx，未修改响应在创建 HTTPFlow 时一次性 take 并复用，避免存储阶段再次 response fix。响应一旦 modified 就释放 provenance packet 并走原有修复路径；`NoFixContentLength` 始终优先，不能被 pre-fixed packet 覆盖。协议、数据库字段与公开兼容 API 均未改变，四个相关 package 的 focused/完整回归及 race 通过。

5 次 256 KiB 微基准中，存储阶段重复 fix 由中位 `2.391 ms / 1,247,989 B/op / 319 allocs` 降到复用 fixed packet 的 `1.638 ms / 954,108 B/op / 236 allocs`，约 `-31.5%/-23.5%/-26.0%`。heap 候选 `2026-07-24T14-49-28-440Z` 对照 `2026-07-24T14-09-50-940Z`：`resolveHTTPFlowStoredResponse` 的 fix caller 消失，Fix 链 cumulative `127,299,117 → 65,946,384 B (-48.2%)`，`CreateHTTPFlow` cumulative `-17.5%`，`transform.Bytes` flat `-55.1%`，全窗口累计分配 `-4.4%`；结束 live heap `-2.1%` 仅为单样本。CPU 候选 `2026-07-24T15-01-46-777Z` 对照最近同配置但早于第二十五轮的 `2026-07-24T11-45-52-112Z`：目标 caller 从 `290 ms/5.53%` 降到 top 外，Fix 链 `510 → 170 ms`、`CreateHTTPFlow 980 → 720 ms`；该 CPU 差值包含第二十五、二十六轮，只作累计归因。

严格 3+3 使用紧邻第二十五轮候选 `body-2026-07-24T14-15-45-537Z` 作为基线，第二十六轮候选为 `body-2026-07-24T14-54-42-668Z`，比较文件为候选目录的 `comparison-vs-response-fix-provenance.{json,md}`。六次均完成 120/120 和 Body、数据库、stream、CPU 恢复、清理门禁，配置/诊断差异为空，实际 Renderer 输入指纹相同。Yak CPU p95/RSS 为 `+0.3%/+0.7%`，吞吐 `+6.2%`、request → React `-13.4%`、Query RTT `-11.8%`、数据库/Renderer 排空 `-51.1%/-42.3%`；风险为网络 request p95 `+27.5%`、首次可见 `+18.7%`、Long Task `115 → 284 ms (+147%)`、persist queue wait p95 `+32.7%`。候选 throughput/request/首显/Long Task 的 CV 分别约 `10.2%/16.3%/23.1%/47.5%`，因此保留由微基准、heap、CPU 和中性 Yak CPU/RSS 共同支持的 owned handoff，同时把 UI/WSL 反向项留给固定硬件复验，不宣称整体体感已提升。

下一轮先对剩余 `bytes.growSlice`、response parser、`io.ReadAll` 和 `DumpHTTPResponse` 做 caller 级归因，证明 raw/body/dump 各份数据的生命周期与唯一所有权后再提出窄优化；不能证明 ownership 的输入继续复制。`strconv.quoteWith` 属于持久化表示路径，不因排名靠前就修改格式。

#### Phase 1 response writer-only serialization 第二十七轮证据（2026-07-24）

minimartian 在 CONNECT、代理认证失败和普通 HTTP 响应三处只需要把响应写回客户端，却调用 `DumpHTTPResponse` 并立即丢弃返回值；旧实现因此在写 socket 的同时还建立一份完整序列化报文。候选保留 `DumpHTTPResponse` 的公开行为，新增只写不缓存的 `WriteHTTPResponse`，并只替换这三个 discard caller。两条路径共享协议、Header、Body 读取与恢复逻辑；writer-only 不建立 cache buffer。逐字节输出等价、Body 恢复、nil writer、focused/full test 以及 `common/utils`、`common/minimartian` race 均通过。

5 次 256 KiB 微基准中，dump-and-discard 中位数为 `63.358 µs / 274,939 B/op / 16 allocs`，writer-only 为 `2.276 µs / 4,272 B/op / 9 allocs`，分别约 `-96.4%/-98.4%/-43.8%`。heap `2026-07-24T15-24-38-358Z` 对照 `2026-07-24T14-49-28-440Z`：总累计分配 `1,237,457,392 → 1,170,515,389 B (-5.4%)`，旧 `DumpHTTPResponse` cumulative `69,909,517 B` 对应候选 `WriteHTTPResponse` cumulative `31,419,800 B`，减少约 `38.5 MB (-55.1%)`；旧 cache 的 `bytes.Buffer.Grow` caller 消失。候选仍需向客户端实际写出约 30 MiB，不能消除。结束 live heap `+3.5%`、正向 live delta `+8.9%` 均为单样本反向风险，不作为常驻内存结论。CPU `2026-07-24T15-33-54-677Z` 对照 `2026-07-24T15-01-46-777Z`：旧 dumper 为 `330 ms/7.93%`，候选 writer caller 低于 `11.5 ms` top 阈值；总样本和吞吐同时明显改善，但候选含约 500 ms 随机 RSA 生成且该 CPU case 只有 4 KiB 响应，因此只认目标 caller 消失和无 CPU 回归，不归因整段吞吐变化。

正式严格 3+3 为 `body-2026-07-24T14-54-42-668Z` → `body-2026-07-24T15-48-39-764Z`，比较文件为候选目录下 `comparison-vs-discarded-response-packet.{json,md}`。六次均完成 120/120、64 KiB request/256 KiB response、数据库、stream、CPU 恢复和清理门禁，比较器为 `passed`，配置与诊断差异为空，实际 Renderer 输入指纹均为 `fa377df501505467d37539e9407e550f7931d1ebf5607b0d6cc4ce176fe3328a`。候选三轮后端构建指纹均为 `a08ab85d69e9e44a583a`，cache 状态为 `false/true/true`。中位数为 Yak CPU p95 `-0.1%`、峰值 RSS `-2.5%`、吞吐 `+3.4%`、request p95 `-23.1%`、首次可见 `-8.7%`、Renderer drain `-30.9%`、Long Task `-42.3%`。反向项包括 Query p95 `5.179 → 11.226 ms (+116.8%)`、数据库 change detection p95 `32 → 119 ms (+271.9%)`、Electron drain CPU p95 `+18.1%` 和 producer-stop visible backlog `28 → 40`；其中 Query 两侧 CV 为 `52.8%/79.7%`，change detection 为 `87.3%/71.8%`，最终排空与正确性仍通过。这轮按确定性分配、heap caller、CPU caller 和中性 Yak CPU/RSS 证据保留，不把 WSL 端到端正向项宣称为固定硬件收益。

首轮候选矩阵因自动化构建身份不稳定而作废，没有进入上述比较。根因是 fixture 在 ChildProcess `exit` 而非 `close` 时读取输出，并逐 chunk 隐式 UTF-8 解码；约 907 KiB、包含中文的 git diff 在不同 chunk 边界产生替换字符，造成同一源码指纹漂移。fixture 现等待 `close`、先 `Buffer.concat` 再解码，加入大中文 tracked diff 回归；9 个 fixture 测试和 48 个 preflight 测试通过，真实脏工作区连续 5 次得到相同 state/build fingerprint。下一轮先验证 response parser 中调用方立即丢弃的 raw packet 是否可由显式 writer/body API 避免；当前 profile 约 `29.44 MB` 属于该返回值，保留 parser Body 与 httpctx ownership 所需的独立存储，不以共享可变切片换性能。

#### Phase 1 requestless response raw packet 第二十八轮证据（2026-07-24）

`ParseBytesToHTTPResponse` 通过 `ReadHTTPResponseFromBytes(..., nil)` 解析 lowhttp 已经持有的完整响应。旧 parser 即使没有 request、没有 httpctx 接收者，仍把 Header 与 Body 重建进一个 `bytes.Buffer`，返回时直接丢弃；`rsp.Body` 同时持有另一份独立 allocation。候选只有在 `req != nil` 时才创建 raw-packet buffer，无 request 路径仍把 Body 复制到 owned reader，继续保证调用方修改输入后状态、Header 和 Body 都不变；有 request 路径仍把独立 bare packet 交给 httpctx。新增 bytes+request 的 bare/input/body 三方 ownership 回归，原 parser input-retention、短 Content-Length、focused race 和 `common/utils`、`common/utils/lowhttp`、`common/minimartian` 完整回归均通过，公开签名、wire 与数据库没有变化。

同一 256 KiB bytes parser 的优化前/后各 5 次微基准，中位数为 `109.345 → 53.000 µs (-51.5%)`、`535,315 → 264,766 B/op (-50.5%, -270,549 B/op)`、`55 → 52 allocs`。heap `2026-07-24T16-10-40-261Z` 对照 `2026-07-24T15-24-38-358Z`：总累计分配 `1,170,515,389 → 1,050,260,546 B (-10.3%)`，`ParseBytesToHTTPResponse` cumulative `63,909,353 → 26,649,496 B (-58.3%)`，`ReadHTTPResponseFromBytes` cumulative `125,942,769 → 50,492,117 B (-59.9%)`，`bytes.growSlice` `381,323,072 → 312,605,303 B (-18.0%)`；无 request 的 raw-packet body 写入行从 profile 消失，剩余约 47.65 MiB 是 `rsp.Body` 的独立 owned 数据。结束 live heap `274,014,528 → 281,420,486 B (+2.7%)`，正向 live delta `21,937,954 → 20,144,052 B (-8.2%)`，方向交错且都是单样本，不宣称常驻内存改善。

标准 5 秒 CPU 报告 `2026-07-24T16-15-24-318Z` 对照 `2026-07-24T15-33-54-677Z`：总样本 `2.30 → 2.31 s`、吞吐 `101.39 → 102.40 req/s`，没有总 CPU 回归；但该 case 的 response 只有 4 KiB，目标 caller 低于采样阈值，request p95、Yak CPU/RSS 单样本也有反向，因此不据此宣称 CPU 提速。

正式严格 3+3 为 `body-2026-07-24T15-48-39-764Z` → `body-2026-07-24T16-17-57-516Z`，比较文件为候选目录的 `comparison-vs-requestless-response-raw-packet.{json,md}`。六次均完成 120/120、64 KiB request/256 KiB response、Body、数据库、stream、CPU 恢复和清理门禁；比较器为 `passed`，配置与诊断差异为空，实际 Renderer 输入指纹均为 `fa377df501505467d37539e9407e550f7931d1ebf5607b0d6cc4ce176fe3328a`。候选三轮后端构建指纹均为 `542887dfcc4c0913032f`，cache 为 `false/true/true`。中位数为 Yak CPU p95/RSS `+0.2%/+0.8%`、吞吐 `+5.3%`、request p95 `-12.8%`、首次可见 `-20.1%`、Query p95 `-28.4%`、Query RTT `-56.1%`。反向风险为 request → React `+4.9%`、Renderer drain `+9.5%`、persist queue wait p95 `52 → 81 ms (+55.8%)`、max persistence backlog `5 → 8 (+60%)` 和 Electron CPU p50 `+28.2%`；Electron CPU p95 `-0.2%`，候选 Renderer drain/persist wait CV 为 `29.8%/24.0%`，最终排空仍通过。候选按 ownership、微基准、heap 和中性产品 CPU/RSS 证据保留，不把交错 UI 指标描述成全面提速。下一轮继续拆分剩余 `readHTTPResponseBodyWithLimit`、`bytes.Clone` 与 `bytes.growSlice` caller，先证明 Body/httpctx/HTTPFlow 的生命周期；不会让 mutable Body 与固定报文别名，也不会仅因 `strconv.quoteWith` 可见就改变数据库表示。

#### Phase 1 HTTPFlow quote 输入只读视图第二十九轮证据（2026-07-24）

`CreateHTTPFlow` 持久化 request/response 时必须保留 `strconv.Quote` 的数据库表示，但旧代码在 quote 前用 `string(reqRaw)` 和 `string(rspRaw)` 各复制一次完整报文；这些临时 string 只在同步 quote 调用期间只读，quote 输出本身另行分配。候选用现有 `UnsafeBytesToString` 建立只读 view，调用 `strconv.Quote` 后以 `runtime.KeepAlive` 明确延长源切片生命周期；不缓存 view，也不让返回 string 与输入别名。nil/empty、ASCII、中文与控制字符、非法 UTF-8、完整 `0..255` 字节及输入随后被修改的差分/ownership 测试逐字节匹配旧实现；focused test/race 和完整 `common/yakgrpc/yakit` 回归通过，数据库格式和公开 API 未变。

64 KiB request + 256 KiB response 的 `CreateHTTPFlow` 优化前/后各 5 次微基准，中位数为 `3.187 → 3.151 ms/op (-1.1%)`、`2,734,899 → 2,390,940 B/op (-12.6%, -343,959 B/op)`、`432 → 430 allocs`。独立 256 KiB quote 基准为 `1.490 → 1.434 ms/op (-3.8%)`、`942,083 → 671,746 B/op (-28.7%, -270,337 B/op)`、`3 → 2 allocs`。heap `2026-07-24T16-33-49-322Z` 对照 `2026-07-24T16-10-40-261Z`：两处临时 string caller 从 profile 消失，`CreateHTTPFlow` cumulative `295,995,370 → 258,208,379 B (-12.8%, -37.8 MB)`；总累计分配 `+1.7%`、post live heap `+3.2%`、positive live delta `+2.0%` 均为反向单样本，且保留的 quote 输出 `91.6 → 109.2 MB` 有采样波动，因此只认目标输入副本消失。CPU `2026-07-24T16-38-03-642Z` 对照 `2026-07-24T16-15-24-318Z` 中 `CreateHTTPFlow 550 → 330 ms`、quote 链 `150 → 80 ms`、总样本 `2.31 → 1.85 s`；它仍是单次诊断，不单独用于发布结论。

正式严格比较为 `body-2026-07-24T16-17-57-516Z` → `body-2026-07-24T16-46-15-707Z`，比较文件为候选目录的 `comparison-vs-httpflow-quote-input-copy.{json,md}`。候选 3/3 均完成 120/120、精确 Body、数据库、shadow stream、详情补包、CPU 恢复和清理门禁，比较器 `passed`，配置/诊断差异为空；实际 Renderer 输入指纹仍为 `fa377df501505467d37539e9407e550f7931d1ebf5607b0d6cc4ce176fe3328a`，候选后端构建指纹为 `9b3aae4c5c388db88e54`。Yak CPU p95 `+0.4%`、Yak RSS `-3.0%`、Electron CPU p95 `-3.5%`、吞吐 `+3.6%`、request → React `+2.2%`、首次可见 `+3.3%`；风险为 request p95 `+11.7%`、Renderer drain `+44.4%`、Query RTT `+156.5%`、backend Query p95 `+571.1%`。候选 Query CV 仍达 `66.5%`，三轮最终排空正确，且 persist queue p95/max backlog 分别改善 `35.8%/37.5%`；这些交错项不支持 UI 提速声明，候选仅按确定性 allocation、格式等价、heap caller 和中性 CPU/RSS 证据保留。

正式矩阵第一次尝试 `body-2026-07-24T16-40-23-283Z` 明确保留为失败证据：虚拟容器已滚到 `1120 px`、list margin 已到 `1008 px`，但驱动只等待“两帧 + 20 ms”，在表格 `200 ms` 节流与繁忙 Renderer 下读取到旧首行 `120`。产品数据、数据库和清理门禁已通过，同一二进制的 heap/CPU 场景也都完成 `120 → 84 → 120`，定位为测试观测竞态。WDIO 门禁现改为最多 `1.5 s` 的状态等待、每次重新解析 DOM，并把恢复条件加强为首行身份也必须回到原值；它超时仍失败，不降低正确性。preflight 48 项通过，正式三轮实际下滚等待仅 `111.9..118.7 ms`、回顶 `0.7..0.9 ms`。下一轮按最新 heap 的 `bytes.growSlice`、`io.ReadAll`、`bytes.Clone` 和 response-body caller 继续做行级归因，优先选择不改变 wire/body/database ownership 的窄候选。

#### Phase 1 conn pool 成功响应延迟恢复副本第三十轮证据（2026-07-24）

`persistConn.readLoop` 旧实现会把每个已成功解析的响应先完整写入第二个 `bytes.Buffer`，但正常成功路径从不读取这份 recovery 副本；它只在解析错误或 `Connection: close` 后又读到尾随字节时才需要组合报文。候选直接保留 parser 捕获的原始 packet，异常路径仍执行原来的限时补读，仅在确有 `restBytes` 时一次性分配精确长度的独立组合报文并写回 httpctx。成功路径返回切片及其 ownership 不变，错误/close 超时与连接淘汰语义不变。空尾随切片同一性、非空组合逐字节及独立 ownership 测试、相关连接池场景、focused race 和完整 `common/utils/lowhttp` 回归均通过。

256 KiB 成功响应的 5 次定点基准中，旧 eager recovery buffer 中位约 `38.067 µs / 262,146 B/op / 1 alloc`，候选 lazy 路径约 `0.492 ns/op / 0 B/op / 0 alloc`；这只度量被删除的成功路径工作。heap `2026-07-24T17-04-36-726Z` 对照 `2026-07-24T16-33-49-322Z`：旧 `conn_pool.go:865` 的约 `28.80 MiB` 分配从正常场景消失，`persistConn.readLoop` cumulative `188,813,567 → 172,880,394 B (-8.4%)`，全窗口累计分配 `1,068,583,191 → 1,027,154,904 B (-3.9%)`，`bytes.growSlice -5.9%`，post live heap `-2.8%`；positive live delta `+10.2%` 反向，故不宣称常驻内存收益。4 KiB response 的 CPU 诊断总样本与吞吐反向，目标 copy 未进入采样，只记录为不支持 CPU 提速结论。

正式严格 3+3 为 `body-2026-07-24T16-46-15-707Z` → `body-2026-07-24T17-12-11-181Z`，比较文件为候选目录的 `comparison-vs-eager-conn-pool-recovery-copy.{json,md}`。六次均完成 120/120、精确 Body、数据库、shadow stream、详情、滚动、CPU 恢复与清理门禁，配置/诊断差异为空；候选后端指纹为 `b0f9f67fbbc613466aaf`，三轮 cache 为 `false/true/true`。Yak CPU p95/RSS `-0.5%/-0.1%`、request p95 `-18.2%`，但吞吐 `-8.5%`、Electron CPU p95 `+3.2%`、首次可见 `+6.7%`、Long Task `63 → 152 ms`、数据库 catch-up/drain `+83.8%/+61.7%`；多项候选 CV 高且范围重叠。候选因成功路径语义等价、定点 0 allocation、heap caller 消失及中性 Yak CPU/RSS 而保留，但正式矩阵不支持整体吞吐或 UI 提速声明。第三十一轮继续拆解 `respBuffer`、parser Body、httpctx packet 与持久化表示的生命周期，只有证明某一份立即丢弃或可安全移交 ownership 才修改。

#### Phase 1 MITM 中间响应 Body 丢弃第三十一轮证据（2026-07-26）

minimartian 的 HTTP/1 上游链路已经用 `responseRaw/respBuffer` 捕获完整 wire packet，lowhttp parser 还会为临时 `http.Response.Body` 建立一份独立 Body，随后 minimartian 又从 `LowhttpResponse.RawPacket` 解析最终响应；这份临时 Body 在普通 MITM 成功路径中不会被消费。候选只让 minimartian 显式开启 metadata-only 解析，并且只覆盖 `Content-Length > 0`、不超过 1 MiB、非 HEAD/TRACE/CONNECT、非 chunked/CL+TE、非 `NoBodyBuffer`、非 header callback/too-large/fix-content-length 的响应。parser 仍建立独立 final bare packet，并保持 1xx 后只保存最终响应的语义；外层 wire capture 仍保存真实线上字节。公开 parser 与 lowhttp 默认行为不变，chunked、回调、超限和大 Body 全部回退旧路径。最初“不预扩容直接 drain”的候选使 256 KiB 基准约从 `806 KB` 增到 `917 KB/op`，已拒绝；直接把含 1xx 的外层 capture 强制写回 httpctx 也因会改变 final-only bare 语义而拒绝。

逐字节、Body size、outer capture/httpctx 独立 ownership、短 Content-Length、100 Continue、chunked、callback、超过 1 MiB，以及连接池开/关的真实 HTTP 集成测试均通过；focused/full `common/utils`、`common/utils/lowhttp`、`common/minimartian` 与相关 race 均通过。5 次 256 KiB 基准中，保留临时 Body 的中位数为 `198.073 µs / 806,357 B/op / 65 allocs`，metadata-only 为 `131.973 µs / 544,555 B/op / 64 allocs`，分别约 `-33.4%/-32.5%/-1 alloc`。heap `2026-07-26T04-39-01-213Z` 对照第三十轮 `2026-07-24T17-04-36-726Z`：连接池 read loop/parser cumulative `172,880,394 → 68,004,055 B (-60.7%)`，body reader flat `105,265,512 → 65,291,266 B (-38.0%)`，parser cumulative `244,025,856 → 134,868,305 B (-44.7%)`，总累计分配 `-8.8%`、`bytes.growSlice -25.4%`、post live heap `-3.4%`、positive live delta `-11.6%`，方向全部一致。剩余 body-reader allocation 来自未开启该内部选项的上层 parser caller，不是连接池 MITM 临时 Body。

4 KiB response 的 CPU 诊断 `2026-07-26T04-44-08-160Z` 对照 `2026-07-24T17-09-48-900Z`：总样本 `2.22 → 2.40 s`、吞吐 `103.89 → 96.87 req/s`、request p95 `164.76 → 248.01 ms`，因此明确记录为反向诊断，不宣称 CPU 提升；目标 4 KiB 副本约只有 0.5 MiB/窗口且低于采样分辨率，GC 栈主导本轮变化。正式严格 3+3 为 `body-2026-07-24T17-12-11-181Z` → `body-2026-07-26T04-48-05-606Z`，比较文件为候选目录的 `comparison-vs-retained-intermediate-response-body.{json,md}`。六次均完成 120/120、64 KiB request/256 KiB response、精确 Body、数据库、shadow stream、详情、滚动、CPU 恢复与清理门禁，比较器 `passed`，配置/诊断差异为空；候选后端指纹为 `a3ca1f78d46d2c0dd0f6`，cache 为 `false/true/true`，实际 Renderer 输入指纹未变。候选中位 Yak CPU p95 近似持平、Yak RSS `-1.5%`、吞吐 `+8.5%`、数据库 catch-up/drain `-59.5%/-47.7%`、persist → React `-15.4%`；反向项为 request p95 `+16.3%`、duplex delivery p95 `82 → 288 ms` 和 Yak drain CPU `+42.4%`。duplex 两侧范围为 `75..557/90..319 ms`，候选均值与最大值没有退化，request p95 范围也重叠；候选按确定性 allocation、完整 ownership/协议门禁和 heap 主链证据保留，不把交错的 WSL 端到端指标描述成全面 UI 提速。下一轮从剩余 `bytes.growSlice`、`io.ReadAll`、`bytes.Clone` 与 `splitHTTPPacketEx` caller 做生命周期归因。

#### Phase 1 parser-owned Request Body dump 第三十二轮证据（2026-07-26）

请求 parser 已为 `req.Body` 建立独立于调用方输入和 httpctx bare packet 的 owned allocation，但 `DumpHTTPRequest` 仍会用 `io.ReadAll` 把这份 Body 再复制到临时切片，随后才写入最终 dump buffer 并恢复 Body。候选让 parser 返回包内私有的 `ownedHTTPRequestBody`，保持 `io.ReadCloser`、`io.WriterTo`、Close 和“只消费剩余 Body”的行为；dumper 对该私有类型直接取得剩余只读 view，同时把原 reader 移到 EOF，结束时恢复新的 owned reader。dump 输出仍复制且不与恢复 Body 别名。外部构造、插件替换或其他未知 Body 类型继续走原 `io.ReadAll` 兼容路径，公开 API、wire、Header/Content-Length/chunked 逻辑和 bare/body/input 三方 ownership 均未改变。

新增部分读取、原 Body 被消费、恢复剩余 Body、dump 输出独立修改及外部 Body fallback 测试；原 128 并发 parser ownership、64 KiB input/bare/body 独立性继续通过。focused race、完整 `common/utils`（28.47 s）、`common/mutate`（14.76 s）、`common/crep`（20.68 s）、`common/minimartian`、`common/utils/lowhttp`（187.58 s）及 lowhttp 定向 race 全部通过，前端 preflight 48/48 通过。5 次 64 KiB parser-request dump 基准中，旧路径中位为 `67.959 µs / 359,369 B/op / 34 allocs`，owned view 为 `16.000 µs / 74,430 B/op / 18 allocs`，分别约 `-76.5%/-79.3%/-47.1%`。

heap `2026-07-26T05-35-58-885Z` 对照第三十一轮 `2026-07-26T04-39-01-213Z`：`DumpHTTPRequest` cumulative `81,358,855 → 20,194,869 B (-75.2%)`，其 `io.ReadAll` caller 完全消失，全局 `io.ReadAll 104,305,301 → 37,514,854 B (-64.0%)`，总累计分配 `936,710,334 → 857,827,388 B (-8.4%)`，post live heap `-1.9%`；`bytes.growSlice +4.1%`、`bytes.Clone +2.8%` 和 positive live delta `+5.9%` 为反向单样本，不作常驻内存结论。64 KiB request/4 KiB response 的 CPU `2026-07-26T05-41-58-226Z` 对照 `2026-07-26T04-44-08-160Z`：总样本 `2.40 → 2.28 s (-5.0%)`，旧 `DumpHTTPRequest 190 ms` 与 `io.ReadAll 240 ms` 均降到 top 阈值外，请求 parser cumulative `280 → 130 ms (-53.6%)`；吞吐 `+5.9%`、request p95 `-45.3%` 只作单次诊断，Yak RSS `+1.6%`、Electron CPU p95 `+6.8%` 同样保留为风险。

正式严格 3+3 为 `body-2026-07-26T04-48-05-606Z` → `body-2026-07-26T05-50-09-512Z`，比较文件为候选目录的 `comparison-vs-request-body-readback.{json,md}`。六次均完成 120/120、精确 64 KiB request/256 KiB response、数据库、shadow stream、详情、滚动、CPU 恢复与清理门禁；比较器 `passed`，配置/诊断差异为空。候选后端指纹为 `e89c39ecbfba4c444c9d`，cache `false/true/true`，实际 Renderer 输入指纹未变。Yak CPU p95/RSS `+0.1%/-1.3%`、吞吐 `+1.7%`、request p95 `-1.1%`、Long Task `+1.2%`；反向项为 Electron CPU p95 `+11.1%`、首次可见 `+17.6%`、Renderer drain `+25.5%`、database catch-up/drain `+52.9%/+32.8%` 和 backend Query p95 `+226.1%`。候选吞吐范围更窄，首次可见与部分 SQLite 指标则稳定反向，故只按确定性 micro/heap/CPU caller 与 ownership 证据保留，不宣称 UI 体感提升。下一轮不会继续删除 `SetBare/Plain*` clone；先证明其跨 goroutine 生命周期，或转向 `splitHTTPPacketEx` 中可显式使用 view 的只读 caller。

#### Phase 1 unencoded auto-unzip Body 只读视图第三十三轮证据（2026-07-26）

MITM V2 的 response mirror 会调用 `DeletePacketEncoding` 生成插件可见的 plain response。旧实现即使响应没有 `Content-Encoding`、没有 chunked，也先通过公开 `SplitHTTPPacket` 复制完整 Body，检测完“无需变换”后再返回原 packet；普通 256 KiB 响应因此产生一份立即丢弃的 256 KiB 临时副本。候选只在 `_unzipPacketEncodingInternal` 内部改用 `splitHTTPPacketEx(..., copyBody=false)` 的只读 view：无编码、无变换和保守失败路径仍返回原 packet 且保持同一底层切片，成功 gzip/zlib/deflate/br/zstd 或 unchunk 仍通过 `ReplaceHTTPPacketBody` 建立独立输出。审计发现 `codec.HTTPChunkedDecode` 在部分畸形输入的诊断路径会截断其参数，因此 chunked 分支明确保留 `bytes.Clone(body)`；这份防御性副本没有为了指标被删除。`SetPlainResponseBytes` 的跨 goroutine/插件生命周期 clone 也继续保留。

新增无编码 same-pointer、输入不变、gzip 输出独立、非法 gzip 原样回退，以及畸形 chunked 不得修改输入的 ownership 门禁；原 gzip/chunked/zlib/deflate 用例、lowhttp focused race、完整 `common/utils/lowhttp`（187.53 s）和 MITM V2 gRPC 自动解压/手动劫持定向回归均通过，前端 preflight 48/48 通过。5 次 256 KiB 无编码响应微基准中，旧路径中位为 `52.909 µs / 263,137 B/op / 26 allocs`，候选为 `1.412 µs / 974 B/op / 25 allocs`，分别约 `-97.3%/-99.6%/-1 alloc`。

heap `2026-07-26T06-20-44-227Z` 对照第三十二轮 `2026-07-26T05-35-58-885Z`：`DeletePacketEncoding` 的 `36,980,867 B` cumulative 从候选榜单消失，`MITMV2.func7 63,394,592 → 36,906,146 B (-41.8%)`，剩余量与刻意保留的 plain-response clone 对齐；全局 `SplitHTTPPacket 63,488,542 → 31,979,396 B (-49.6%)`，`splitHTTPPacketEx` flat `69,523,697 → 41,460,950 B (-40.4%)`。窗口总累计分配 `857,827,388 → 815,443,386 B (-4.9%)`，`bytes.growSlice -6.5%`、`bytes.Clone -7.4%`。post live heap `+1.6%`、positive live delta `+6.0%` 反向，因此只声明热路径临时分配被消除，不声明常驻内存下降。

64 KiB request/4 KiB response 的 CPU 诊断 `2026-07-26T06-28-11-506Z` 对照 `2026-07-26T05-41-58-226Z`：总样本 `2.28 → 1.97 s (-13.6%)`、GC flat `810 → 570 ms`、Yak 峰值 RSS `-1.9%`，但目标是 256 KiB response copy，在该 4 KiB case 中前后都低于采样阈值；单次吞吐 `+15.0%`、request p95 `+2.5%`、Electron CPU p95 `+64.7%` 与首次可见 `+16.5%` 方向交错，均不归因给本轮。

正式严格 3+3 为 `body-2026-07-26T05-50-09-512Z` → `body-2026-07-26T06-33-07-606Z`，比较文件为候选目录的 `comparison-vs-unencoded-unzip-body-copy.{json,md}`。六次均完成 120/120、精确 Body、数据库、shadow stream、详情、滚动、资源恢复与清理门禁；比较器 `passed`，配置/诊断差异为空。候选后端指纹为 `a37abefdd1026d87ba27`，诊断已预热构建缓存，因此正式三轮 cache 为 `true/true/true`；实际 Renderer 输入指纹仍为 `fa377df501505467d37539e9407e550f7931d1ebf5607b0d6cc4ce176fe3328a`。中位 Yak CPU p95/RSS `+0.1%/-0.9%`、吞吐 `+0.8%`、request p95 `-10.8%`、首次可见 `-7.7%`、Renderer drain `-7.1%`、数据库 catch-up/drain `-22.6%/-15.9%`；反向风险为 Electron CPU p95 `+7.0%`、Query RTT `+64.8%`、database change detection `+77.6%` 及若干 Query 分段。Query/DB 分段三轮波动仍大，例如 backend count CV `137% → 73%`、Query RTT CV `55% → 30%`，且最终排空完全正确；候选按同指纹语义门禁、微基准和 heap caller 证据保留，不宣称全面 UI 提速。下一轮继续按 heap caller 证明 ownership，优先处理普通路径；chunked 和跨阶段 plain/bare clone 在生命周期没有重构前继续保留。

#### Phase 1 mirror response 同步过滤 Body 只读视图第三十四轮证据（2026-07-26）

第三十三轮 heap 将剩余约 `31.98 MB` 的公开 `SplitHTTPPacketFast` Body copy 精确定位到 `handleMirrorResponse`。该 Body 在默认路径只被 bundled/static JS 过滤器同步读取，旧代码却总是复制完整响应；只有启用 `MirrorHTTPFlow` 插件 hook 时，Body 才会跨异步 goroutine 使用。候选让同步过滤走 `SplitHTTPHeadersAndBodyFromPacketView`，并且仅在确实存在异步 hook 时、启动 goroutine 之前用 `bytes.Clone` 建立与旧行为等价的独立 snapshot。超大响应仍用相同 Header 截断 plain response，插件仍收到完整独立 Body；公开 split API、过滤结果、数据库 packet 和跨阶段 plain/bare clone 均未改变。

产品级门禁验证 view 的 Header/Body 与旧 split 逐字节相同、同步 Body 确实引用 plain response，而异步 hook snapshot 与原 response 双向独立。静态 JS 过滤、auto-unzip/手动劫持定向测试、focused race，以及全部 62 个 `TestGRPCMUSTPASS_MITMV2*`（211.44 s）通过。现成 256 KiB clone/view 各 5 次基准中位为 `55.616 → 0.897 µs (-98.4%)`、`262,778 → 618 B/op (-99.8%)`、`17 → 16 allocs`。

heap `2026-07-26T07-10-20-462Z` 对照第三十三轮 `2026-07-26T06-20-44-227Z`：旧 `SplitHTTPPacketFast/SplitHTTPPacket 31,979,396 B` cumulative 路径完全消失，`splitHTTPPacketEx` flat `41,460,950 → 7,084,035 B (-82.9%)`、cumulative `48,277,346 → 11,278,907 B (-76.6%)`，`MITMV2.func25 -6.1%`。总累计分配只下降 `0.1%`，因为 `bytes.growSlice/bytes.Clone` 单样本反向约 `+9.1%/+7.1%`；post live `-5.7%`、positive live delta `+5.0%` 方向交错，因此只认目标 caller 消失。4 KiB response CPU `2026-07-26T07-14-13-863Z` 对照 `2026-07-26T06-28-11-506Z` 总样本 `1.97 → 2.06 s (+4.6%)`、GC flat `570 → 870 ms`，但目标量仍低于采样阈值；Yak CPU p95 近似不变、RSS `-1.6%`、吞吐 `+2.2%`、request p95 `-0.6%`、首次可见 `+11.7%`，不作整体 CPU 声明。

第一次正式矩阵 `body-2026-07-26T07-16-32-198Z` 明确保留为失败样本：第 3 轮在负载开始前由 Electron CDP bridge 返回 `Promise was collected`，应用窗口和 Yak 均存活、清理无误。前端为安装场景 Observer 这一同步幂等动作增加仅匹配该精确传输错误、最多一次的重试；应用/断言错误不重试，第二次失败仍上抛。新增 4 项测试后完整 preflight 为 52/52。修复后没有拼接前两轮，而是从零重跑正式 3 轮。

有效严格 3+3 为 `body-2026-07-26T06-33-07-606Z` → `body-2026-07-26T07-26-19-918Z`，比较文件为候选目录的 `comparison-vs-mirror-response-body-copy.{json,md}`。六次均完成 120/120 和全部正确性/清理门禁；比较器 `passed`，配置/诊断差异为空，候选后端指纹 `578160e9bb081343ca73`、cache `true/true/true`，实际 Renderer 输入指纹不变。Yak CPU p95 `-0.1%`、RSS `+1.2%`、吞吐 `+15.3%`、request p95 `-14.6%`、首次可见 `-22.6%`、Long Task `-32.5%`；反向项为 Electron CPU p95 `+6.0%`、Renderer drain `+41.9%`、database catch-up/drain `+75.7%/+47.7%`、persist wait p95 `+118.2%` 和 duplex delivery `+623.5%`。候选吞吐范围 `71.3..79.4` 高于基线 `61.9..73.8 req/s`，request p95 CV `14.9% → 3.3%`；max-rate 下更快生产同时放大下游 backlog/drain，不能把混合结果描述成 UI 全面提速。候选按确定性 ownership、微基准、heap caller 与中性 Yak CPU/RSS 保留；下一轮 profile 重点转向 request fix/parser 与持久化/Renderer 消费瓶颈。

#### Phase 1 MITMV2 请求重复修复/解析第三十五轮证据（2026-07-26）

request hijack 已经持有 minimartian 解析完成且携带全部 httpctx 的 `originReqIns`，旧路径却对原始请求再次执行 `FixHTTPRequest`，随后调用内部还会再次执行 `FixHTTPPacketCRLF` 的 `ParseBytesToHttpRequest`。审计继续发现，这个额外得到的 `fixReqIns` 只传给手动 drop 的建流选项，而 `createHTTPFlowFromHTTP` 最后总会用 `originReqIns` 覆盖同一选项；普通流量完全不使用它，drop 流量也不会使用它。候选删除这段 eager fix/parse 和无效选项，不改变 wire packet、httpctx、插件输入、数据库结构或公开 API。手动 drop 与 mirror-response ownership 定向测试、全部 62 个 `TestGRPCMUSTPASS_MITMV2*`（211.53 s）以及 focused race（11.59 s）均通过。

256 KiB 请求的 5 次定点基准中，直接复用 `originReqIns` 约为 `0.49 ns/op / 0 B/op / 0 allocs`，旧 eager fix+parse 中位约为 `237.984 µs/op / 1,347,495 B/op / 99 allocs`。heap `2026-07-26T08-03-27-479Z` 对照第三十四轮 `2026-07-26T07-10-20-462Z`：窗口累计分配 `815,443,386 → 748,489,570 B (-8.2%)`，MITMV2 request handler cumulative `54.93 → 7.40 MiB`，旧两行约 `11.76 + 33.60 MiB` 的分配完全消失；全局 `ParseBytesToHttpRequest -37.2%`、`FixHTTPPacketCRLF -40.7%`。该报告使用诊断构建指纹 `146cbf678e5c0e211f75`，120/120 及全部 Body/数据库/stream/清理门禁通过。

正式严格 shadow 3+3 为 `body-2026-07-26T07-26-19-918Z` → `body-2026-07-26T09-01-25-649Z`，比较文件为候选目录的 `comparison-vs-eager-request-fix-parse.{json,md}`。六次均完成 120/120，比较器 `passed`，配置/诊断差异为空，实际 Renderer 输入指纹仍为 `fa377df501505467d37539e9407e550f7931d1ebf5607b0d6cc4ce176fe3328a`；候选后端指纹 `6fe8eab244e61640d25e`、cache `true/true/true`。Yak CPU p95 `-0.1%`、RSS `-9.2%`、Electron CPU p95 `-29.5%`、request/response → React `-2.2%/-4.1%`、Renderer drain `-9.4%`；反向项为吞吐 `-5.2%`、request p95 `+9.1%`、首次可见 `+13.2%`、Long Task `+47.3%`、persist write p95 `+66.7%` 和 Yak drain CPU `+188.9%`。候选按确定性的死工作删除、微基准、heap caller 消失与 RSS 证据保留，不宣称短样本中的全面端到端提速；下游瓶颈改用同一后端的固定生产速率实验单独归因。

#### Phase 1 response packet Body 受控只读视图第三十七轮证据（2026-07-26）

第三十五轮后的大 Body heap 仍显示两条 response parser Body 副本：minimartian 已经持有完整且在响应生命周期内不再修改的 `LowhttpResponse.RawPacket`，却在构造最终 `http.Response` 时再复制一次 Body；交互式响应劫持则先建立必须保留的结果快照，随后 parser 又为该快照复制一次 Body。候选增加显式 opt-in 的 `ReadHTTPResponseFromBytesWithBodyView`：返回的 Body 只读别名调用方拥有的完整 packet，并把“packet 在 response 使用结束前不可修改”写入 API 契约。原 `ReadHTTPResponseFromBytes` 继续返回独立 Body，所有未知或外部可变 caller 的兼容语义不变。minimartian 只对自己拥有的 raw packet 使用新入口；劫持路径仍先 `bytes.Clone` 一次形成独立快照，再让 Body 引用该快照。因此没有删除跨 goroutine/阶段所需的 ownership 边界，也没有 proto、数据库或现有 API 的破坏性迁移。

新增回归覆盖普通/短 `Content-Length`、`100 Continue` 后最终响应、chunked、输入不变、显式 view 别名、minimartian 实际 caller，以及劫持快照与原输入独立且 Body 引用该快照。三个相关 package 的 focused/full 测试、focused race 和全部 `TestGRPCMUSTPASS_MITMV2*`（210.885 s）通过。256 KiB 的 5 次微基准中，旧独立 Body parser 中位约 `66.299 µs / 264,974 B/op / 54 allocs`，受控 view 中位约 `5.005 µs / 2,824 B/op / 53 allocs`，分别约 `-92.5%/-98.9%/-1 alloc`。

heap `2026-07-26T10-24-19-483Z` 对照第三十五轮 `2026-07-26T08-03-27-479Z`：窗口累计分配 `748,489,570 → 692,794,542 B (-7.4%)`，两条旧 `readHTTPResponseBodyWithLimit` 调用路径（合计约 `67.35 MiB`）以及该链路的 `ParseBytesToHTTPResponse` 均从候选 profile 消失。`cloneAndParseHijackedResponse` 下约 `36.24 MB` 的 `bytes.Clone` 是把旧 `make/copy` 快照显式化后的必要 ownership 边界，不计作可删除副本。该诊断 120/120 及所有 Body、数据库、stream、滚动和清理门禁通过，后端指纹为 `53d8b443935d4979830f`；`bytes.growSlice` 等全局 caller 的单样本反向变化不归因给本轮，也不据此宣称常驻内存下降。

正式严格 shadow 3+3 为 `body-2026-07-26T09-01-25-649Z` → `body-2026-07-26T10-41-13-373Z`，比较文件为候选目录的 `comparison-vs-copied-response-body.{json,md}`。六次均完成 120/120，比较器 `passed`，配置/诊断差异为空；实际 Renderer 输入指纹仍为 `fa377df501505467d37539e9407e550f7931d1ebf5607b0d6cc4ce176fe3328a`，候选后端指纹 `10ec43a2a0d6c40ecb0b`、cache `false/true/true`。中位吞吐 `+3.7%`、request/response → React `-4.5%/-2.6%`、persist queue/write p95 `-22.4%/-43.3%`、Long Task `-25.5%`、backend conversion/flow `-49.3%`，Yak CPU p50/p95 约 `-0.2%/+0.2%`。反向项为 Yak RSS `+5.6%`、Renderer drain `+12.0%`、Electron CPU p95 `+12.9%`、request p95 `+8.3%`、首显 `+6.3%` 和高波动的 DB detection `+149.3%`。因此只声明确定性的分配/所有权收益，端到端结论仍为中性偏混合。恢复产品默认 canary 后的 smoke `body-2026-07-26T10-51-59-171Z` 完成 1000/1000、11 个 direct batch、0 Query/fallback/协议错误，吞吐 `199.86 req/s`、request → React `489 ms`、首显 `44 ms`、最大可见积压 `39`，确认优化后的后端可在默认实时链路正常工作。

#### Phase 1 parser-owned bare request 移交第三十八轮证据（2026-07-26）

第三十七轮 heap 将全局 `bytes.Clone` 继续拆到 caller 后，确认 request parser 先在 `bytes.Buffer` 中重建完整 bare packet，再调用面向外部可变输入的 `SetBareRequestBytes` 克隆整包；parser 此后不再访问该 buffer。候选增加窄契约 `SetBareRequestBytesOwned`，只让 parser 把独占 buffer 移交给 httpctx。原 `SetBareRequestBytes` 及 plain/hijacked request/response setter 继续克隆，劫持、插件和跨阶段调用的 ownership 不变；调用方输入、httpctx bare packet 与 `req.Body` 仍三者独立，wire、数据库、proto 和现有 API 行为均未改变。

httpctx 测试显式验证普通 setter 仍克隆、owned setter 保留同一底层切片；64 KiB caller-input/bare/body 双向独立、128 goroutine 并发 parser 和 reader-pool 回归继续通过。完整 `common/utils`（28.449 s）、`common/utils/lowhttp`（183.745 s）、httpctx、focused race 通过。第一次全部 MITM V2 MUSTPASS 组合运行中，`InvalidUTF8RequestDetail` 在落库查询命中固定 4 秒 deadline；该用例随后隔离 3/3 通过（2.72/1.66/1.55 s），再次完整运行也在 195.780 s 全部通过，因此记录为长组合下的 deadline flake，不掩盖也不归因给 ownership 变更。

64 KiB request parser 优化前后各 5 次微基准，中位为 `54.235 → 30.083 µs/op (-44.5%)`、`215,453 → 141,724 B/op (-34.2%)`、`50 → 49 allocs`。heap `2026-07-26T11-15-02-333Z` 对照第三十七轮 `2026-07-26T10-24-19-483Z`：parser 下约 `21.98 MiB` 的 `SetBareRequestBytes → bytes.Clone` 分支完全消失；全局 `bytes.Clone 116,717,473 → 88,642,234 B (-24.1%)`，`readHTTPRequestFromBufioReader` cumulative `128,500,199 → 94,476,908 B (-26.5%)`，窗口累计分配 `692,794,542 → 647,763,815 B (-6.5%)`。外部劫持 bare request、plain request/response 和 response snapshot clone 均仍在 profile 中。`bytes.growSlice +1.3%`、post live heap 约 `+10.0%`、positive live delta 约 `+60.4%` 为反向单样本，因此不宣称常驻内存下降；诊断后端指纹 `c8547a51c57cfa36f129`，120/120 与全部正确性/清理门禁通过。

正式严格 shadow 3+3 为 `body-2026-07-26T10-41-13-373Z` → `body-2026-07-26T11-51-53-723Z`，比较文件为候选的 `comparison-vs-cloned-request-bare-packet.{json,md}`。六次均完成 120/120，比较器 `passed`，配置/诊断差异为空；Renderer 输入指纹保持 `fa377df501505467d37539e9407e550f7931d1ebf5607b0d6cc4ce176fe3328a`，候选后端指纹 `ae8561a7c4869b2905a6`、cache `false/true/true`。request p95 `-11.9%`、首显 `-15.3%`、backend conversion `-20.1%`、Renderer drain `-7.9%`，Yak CPU p95/RSS 约 `-0.1%/0.0%`；反向项为吞吐 `-2.1%`、request → React `+4.2%`、Electron CPU p95 `+10.5%`、Long Task `+13.8%`、persist queue/write p95 `+9.6%/+47.1%` 和高波动 DB detection `+181.4%`。大多数区间重叠，故候选按确定性 ownership/micro/heap 证据保留，不宣称 UI 全指标改善。恢复默认 canary 后的 `body-2026-07-26T12-00-09-473Z` 完成 1000/1000、11 batch、0 Query/fallback/协议错误，吞吐 `200.10 req/s`、request → React `493 ms`、首显 `43 ms`、最大可见积压 `38`、SQLite queue/write p95 `1/1 ms`、Long Task `0 ms`。

#### Phase 1 有界 Content-Length 请求读取第三十九轮证据（2026-07-26）

第三十八轮 heap 将普通网络请求解析器剩余的 `io.ReadAll` 定位为确定长度 Body 的几何扩容，同时 `rawPacket` 也在逐段写入时重复扩容。候选仅对 `Content-Length <= 1 MiB` 的普通请求使用精确长度 `io.ReadFull`，为 parser-owned raw packet 做同样有界的容量预留，并把新读到的独占 Body 切片直接移交给 `req.Body`；超过 1 MiB、chunked 和未知长度继续走旧读取路径。`1 MiB` 只是防止不可信长度触发无界预分配的内部保护，不是协议或产品参数。短 Body 仍只在 `req.Body` 中补换行，bare packet 保留实际 wire bytes；`Content-Length + chunked` 歧义分支、调用方输入、bare packet 和 Body 的独立 ownership 均保持。没有 proto、数据库、前端或公开 API 变更。

64 KiB 真实 bufio parser 优化前后各 5 次微基准，中位为 `83.963 -> 34.462 us/op (-59.0%)`、`426,616 -> 141,644 B/op (-66.8%)`、`63 -> 45 allocs (-28.6%)`。完整 `common/utils`（28.353 s）、focused race、`common/utils/lowhttp/...`（主包 185.804 s）和全部 `TestGRPCMUSTPASS_MITMV2*`（208.039 s）通过；回归还覆盖完整/短 Body、`Content-Length + chunked` 短包、1 MiB 阈值外回退、caller/bare/body 双向独立和并发 parser。

heap `2026-07-26T14-24-26-460Z` 对照第三十八轮 `2026-07-26T11-15-02-333Z`：网络入口 `ReadHTTPRequestFromBufioReaderOnFirstLine` cumulative `60,929,767 -> 22,806,243 B (-62.6%)`，整个 `readHTTPRequestFromBufioReader` cumulative `94,476,908 -> 51,366,390 B (-45.6%)`；旧 `io.ReadAll` 的 `42,996,592 B` cumulative 栈消失，替代 helper 为 `11,712,507 B`。窗口总分配 `647,763,815 -> 651,135,179 B (+0.5%)`，post-live `-4.1%`、positive-live `+0.08%`，方向交错，因此只认目标 caller 收益，不宣称整体常驻内存下降；120/120 与全部门禁通过，诊断后端指纹为 `a9fd90da679f70ca70d7`。

正式严格 shadow 3+3 为 `body-2026-07-26T11-51-53-723Z` -> `body-2026-07-26T14-40-58-936Z`，比较文件为候选的 `comparison-vs-bounded-content-length-read.{json,md}`。比较器 `passed`，配置/诊断差异为空，六轮均 120/120；Renderer 输入保持 `fa377df501505467d37539e9407e550f7931d1ebf5607b0d6cc4ce176fe3328a`，候选后端指纹 `c0bdbe701f755f542dd7`、cache `false/true/true`。吞吐 `+7.4%`、Electron CPU p95 `-16.4%`、Long Task `-55.7%`、Yak CPU p95/RSS `-0.2%/-1.2%`；反向项为 request p95 `+16.8%`、首显 `+13.9%`、request -> React `+4.3%`、Renderer drain `+37.6%`，多数区间重叠。候选按 micro/heap 的直接因果证据保留，不描述成 UI 全指标改善。

恢复默认 canary 指纹后，产品 smoke `body-2026-07-26T14-49-28-551Z` 完成 1000/1000、11 个 direct batch、1000 direct row、0 Query/fallback/Gap/顺序错误，吞吐 `200.09 req/s`、request -> React `496 ms`、首显 `53 ms`、最大可见积压 `43`、SQLite queue/write p95 `1/1 ms`。Long Task 单样本为 `107 ms`，按波动记录；相关前端 Vitest `68/68`、完整 E2E preflight `52/52` 和受限 Renderer build 均通过。SQLite 在固定速率场景仍不是瓶颈，本轮不调整 GORM、连接池或前端消费逻辑；下一轮继续从新 heap 的 response raw capture、CRLF/body replace、quote/query parsing 等 caller 选择候选。

### Phase 2：提交事件快路径（Shadow）

状态：已实现并保留为兼容影子通道；真实数据表仍由 `QueryHTTPFlows` 写入。

- HTTPFlow 成功提交后立即发布 `FlowCommitted` 摘要；
- 事件带 `ID、ProjectGeneration、CommittedAt、HighWaterID`；
- 旧的 `httpflow/create` 和 QueryHTTPFlows 保持不变；
- 新链路先只接收和对账，不修改 UI；
- 比较事件与数据库查询的缺口、顺序、延迟和额外 CPU。

### Phase 3：专用 `SubscribeHTTPFlows`

状态：协议、后端 broker/RPC、Electron bridge、Renderer shadow/canary 控制器、有界观测、body-free 直接列表消费和真实 Electron A/B 已实现；自 2026-07-26 起，兼容的 MITM 顶部视图默认使用 `canary` 直接消费，Query 仍是恢复、筛选、离顶、旧引擎和异常场景的事实来源；其他 UI 消费者尚未切换。

- 新增独立 server-stream RPC，不复用 MITMV2 手动控制流；协议拆到 `httpflow_live.proto`，避免巨型 `HTTPFlow` 生成索引漂移；
- 支持 `LastSeenSequence、LastSeenID、ProjectGeneration、DatabaseIdentity、SessionID` 和 v1 `SourceType` 筛选；
- `HTTPFlowLiveSummary` 只含列表标量，协议层不存在 Request/Response 字段；新增的 request hijack、response mirror、flow built、persist enqueue/start 时间也只是可选 `int64` 标量，用于端到端归因，不重新引入正文；
- 后端按项目/代次维护单调 Sequence、2048 条重放窗口、256 条订阅队列和 4 个最近项目槽；超窗、慢消费者、项目淘汰和游标异常都显式返回 `Gap`，不静默丢数据；
- 当前发布 `Committed、Gap、Heartbeat`；`Updated/Deleted` 枚举已预留但尚未接入，仍由旧失效通知和 Query 恢复；
- 前端仅在无额外筛选的 MITM 顶部视图启动，校验项目/会话/协议/Sequence；Gap 后调用 Query 补偿，新前端连接旧引擎时对当前项目停止重试并退回旧模式；
- 旧 Duplex committed 与专用流分别控制 shadow/canary，确保双路径只有一个 UI 写入者；全局快照新增事件、重放、Gap 原因、序列异常、可用性、direct batch/row/fallback 及各后端阶段 → Renderer 分位数；
- broker/RPC、并发 `-race`、Electron 流生命周期、Renderer 游标/Gap/旧引擎回退与观测测试已通过；前端 `harnessVersion: 5` 可独立切换专用流 `off/shadow/canary`，等待流高水位并自动拒绝 Gap、Sequence 异常、重复、不可用或意外结束；
- 首个 canary 暴露了 heartbeat 越过 RPC 待发送 committed 记录的协议竞态。RPC heartbeat 现只报告该连接已经实际投递的 Sequence/high-water，broker 的 ID 恢复也改为单调 Sequence 边界；定向测试和 race 通过，真实 120 条流量不再产生伪 Sequence Gap；
- 兼容 Query 恢复路径继续采用首条立即、后续最小 `700 ms` 的 leading/trailing 合并；canary 在顶部、默认排序、无额外筛选且无进行中 Query 时，把 body-free summary 直接写入虚拟列表。Gap、断流、不可用、项目/筛选切换、离开顶部或游标不兼容时，先取消未提交 direct batch，再立即回退 Query。
- direct batch 在空闲后的首条仍立即可见；后续以 `250 ms` 稀疏间隔、积压达到 8 行后以 `500 ms` 持续间隔提交，每批最多 256 行、pending 最多 2048 行。定时器回调使用 React legacy root 所需的 `unstable_batchedUpdates`，避免同一批多次同步 render。

#### Phase 3 首轮真实 Electron A/B（2026-07-23）

未经合并的 canary 把 120 条通知放大为约 15 次实时查询，Long Task 中位数从 `284` 增到 `477 ms`；`400 ms` 版本仍有 `12` 次查询，正式 A/B 的 Long Task 为 `333 → 468 ms`，两者均被拒绝。首轮保留方案使用 `700 ms` 合并并在健康专用流期间抑制重复旧唤醒，但数据行仍来自 Query。

同一 Yak/Renderer 构建、Request `64 KiB` / Response `256 KiB`、120 请求、并发 8 的严格串行 3+3 位于前端矩阵 `body-2026-07-23T13-41-56-178Z` 与 `body-2026-07-23T13-47-17-325Z`，机器比较为 `httpflow-live-coalesced-2026-07-23`。三次候选均收到 120/120 committed，Gap、Sequence 缺口、重复、乱序、不可用、意外结束、正文错误和清理错误全部为 0。

| 指标 | stream off | canary | 变化 |
| --- | ---: | ---: | ---: |
| Query 次数中位数 | `5` | `6` | `+1`；120 commits 固定合并为 5 个 live 周期 |
| trigger → Query p95 | `918.9 ms` | `700.3 ms` | `-23.8%` |
| persisted → React p95 | `896 ms` | `693 ms` | `-22.7%` |
| response → React p95 | `971 ms` | `838 ms` | `-13.7%` |
| 首次可见 | `715 ms` | `628 ms` | `-12.2%` |
| request → React p95 | `1045 ms` | `1054 ms` | `+0.9%`，视为持平噪声 |
| Long Task 总时长 | `341 ms` | `337 ms` | `-1.2%`，不再退化 |
| 吞吐 | `35.82 req/s` | `35.61 req/s` | `-0.6%`，基本持平 |

这组证据允许进入直接列表消费实验，但不足以切默认；本阶段 QueryHTTPFlows 仍是唯一行写入者和恢复事实来源。

#### Phase 3 定速 SQLite 排除与直接列表消费 A/B（2026-07-24）

固定速率场景为 1000 请求、并发 16、目标 200 req/s、空 Request Body 和 4 KiB Response Body。SQLite writer 从 1 提到 2 的正式矩阵为 `body-2026-07-24T08-32-22-099Z` 与 `body-2026-07-24T08-38-12-845Z`，比较文件为候选目录下 `comparison-vs-writer1.{json,md}`。writer2 将 Query RTT 中位数 `117.8 → 213.2 ms (+77.9%)`、persist wait `11 → 26 ms (+136%)`、request → React `954 → 1295 ms (+35.7%)`、调度滞后 `22.38 → 554.49 ms`、Long Task `1064 → 1836 ms`，因此被拒绝；独立只读连接候选也被自动化拒绝，产品继续保持 writer1/read0。增加 SQLite 并发不是当前解法。

第一版 direct canary 固定每 `100 ms` 提交列表，正式 shadow/canary 矩阵为 `body-2026-07-24T09-26-19-594Z` 与 `body-2026-07-24T09-32-31-361Z`。它把 request → React 从 `991` 降到 `213 ms`、最大可见 backlog 从 `165` 降到 `44`，但 Long Task 从 `525` 增到 `3278 ms (+524.4%)`、Electron CPU p50 增约 `171%`；该调度方案被拒绝，证明“更频繁 setState”不能等同于更实时。

随后保留 `250/500 ms` 自适应有界批处理，并把计时器中的多次状态更新合并为一次 React batch。同一源码指纹的严格 3+3 基线为 `body-2026-07-24T09-58-01-917Z`，候选为 `body-2026-07-24T10-03-34-990Z`，比较文件为候选目录下 `comparison-vs-shadow-direct-batched.{json,md}`。六次均完成 1000/1000，正文、数据库、ID 唯一性、Gap、Sequence、重复、乱序与清理门禁全部通过；三个候选样本都 direct 1000 行、fallback 0 行、Query 0 次，共 11～12 个 React batch。

| 指标 | shadow + Query | direct canary | 变化 |
| --- | ---: | ---: | ---: |
| request → React p95 | `990 ms` | `490 ms` | `-50.5%` |
| persist → React p95 | `987 ms` | `485 ms` | `-50.9%` |
| 首次可见 | `137 ms` | `44 ms` | `-67.9%` |
| 最大可见 ID backlog | `193` | `39` | `-79.8%` |
| Renderer drain | `486 ms` | `368 ms` | `-24.3%` |
| Long Task 总时长 | `517 ms` | `0 ms` | 候选范围 `0～169 ms` |
| Electron CPU p95 | `8.67%` | `6.81%` | `-21.5%` |
| Electron CPU p50 | `2.49%` | `3.09%` | `+24.2%` 风险 |
| Yak CPU p50 | `116.6%` | `100.8%` | `-13.5%` |
| 吞吐 | `200.09 req/s` | `199.32 req/s` | `-0.4%`，基本持平 |

正式 3+3 候选中的旧 `FlowCommitted` shadow 对账 pending 曾累积到 1000，因为旧观测只认识 Query 返回，而 direct 路径不再发 Query。后续实现没有按最大 ID 粗清：它使用 `databaseIdentity + projectGeneration + ID` 精确双向匹配“旧 shadow 事件”和“已经提交到列表的专用流事件”，覆盖两种到达顺序，并分别报告 `directMatches`、`directRowsWithoutEvent`、commit/shadow → direct 延迟。单样本真实复验位于 `body-2026-07-24T10-37-01-191Z` / `2026-07-24T10-37-01-314Z`：1000/1000 direct 与 shadow 精确匹配，最终 pending 和 direct-without-shadow 均为 0，采样峰值 pending 从 1000 降到 48，11 个 batch、Query/fallback/Gap/Long Task 均为 0。该单次只验证对账正确性，不替代前述正式 3+3 性能结论。

#### Phase 3 慢消费者恢复竞态与真实 Electron 证据（2026-07-24）

慢消费者矩阵在生产进行到 25% 时把 MITM 表格滚离顶部，75% 时回顶。首个 800 条运行 `2026-07-24T10-55-49-429Z` 被精确门禁拦截：数据库和 stream 都是 800/800，但 direct 291 + Query 485 只覆盖 776 条，仍有 24 个中间 ID。问题不在分页常量，而在 direct/Query 所有权交接：回顶后 newer direct rows 可在 Query 补齐旧游标区间前进入列表并推进最大 ID，使后续 Query 从错误的高水位继续。

Renderer 现在有显式恢复闸门。第一次 direct fallback 关闭 direct 插入并记录恢复高水位；恢复期间所有后续 live 事件继续触发 Query，不允许 direct 跨越游标。只有 exhausted Query 同时覆盖 fallback high-water 与最新 stream ID，结果已经 React commit，且候选建立后没有新 stream 事件时才重新开放。新事件会使候选失效。观测也将“相邻 Query ID 回退”与“stream 高水位领先较旧 Query”分开，避免后者误报数据库 reset；快照记录 recovery required/high-water/entries/completions。

两组真实 Electron canary 均通过：

| 场景 | 精确消费 | 回顶到 Renderer 排空 | DB / Renderer 排空 | recovery entry/completion | Query p95 | Long Task |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| 800 条、120 req/s、Request 0 / Response 4 KiB | `800 = 232 direct + 568 Query` | `1856 ms` | `120 / 487 ms` | `1 / 1` | `4.608 ms` | `266 ms` |
| 240 条、30 req/s、Request 64 / Response 256 KiB | `240 = 76 direct + 164 Query` | `2095 ms` | `165 / 314 ms` | `2 / 2` | `10.022 ms` | `279 ms` |

报告为前端 `2026-07-24T11-22-46-115Z` 与 `2026-07-24T11-27-17-125Z`；最终 pending、direct/query-without-event、Gap、Sequence、重复、乱序、不可用、stream backlog 均为 0，visible/backend high-water 精确相等。第二组实际传输了 15 MiB Request Body 与 60 MiB Response Body。该结果完成“持续生产时离顶、回顶、精确追平”门禁，不等于完成断线重放或切换默认模式。

截至这组 2026-07-24 门禁，默认仍保持 `shadow`。长时间持续/突发、断线重放、项目/筛选切换、100+ 站点和 Chromium/nuclei 仍是当时扩大 canary 的必要门禁；后续第三十六轮以当前后端重新验证后更新默认决策。

#### Phase 3 固定速率归因与默认实时流第三十六轮证据（2026-07-26）

第三十五轮同一后端、同一 1000 请求 × 200 req/s 固定生产源先以 `shadow` 连续运行 3 次（`body-2026-07-26T08-06-39-849Z`）。SQLite persist queue/write p95 都只有 `1 ms`，database change detect p95 为 `20 ms`，但 trigger → Query p95 为 `996.2 ms`、persist → React p95 为 `958 ms`、最大可见积压为 `193`，明确表明这一场景的主要延迟来自 Query 唤醒/可见链路，而不是 SQLite 写入。此前把 writer 连接数从 1 提到 2 以及增加 read pool 都已被自动化否决，因此本轮没有改数据库并发参数。

只把专用 body-free stream 改为 `canary` 的同源 3 次矩阵为 `body-2026-07-26T08-12-43-176Z`，候选目录中的 `comparison-vs-shadow-phase35.{json,md}` 只允许 `httpFlowLiveStreamMode` 这一项实验差异，比较器 `passed`。每轮均精确完成 1000/1000，11 个 direct batch、1000 个 direct row，Query/fallback/Gap/Sequence gap/重复/乱序/unavailable 全为 0。中位 request → React `970 → 490 ms (-49.5%)`、persist → React `958 → 486 ms (-49.3%)`、首次可见 `117 → 42 ms (-64.1%)`、最大可见积压 `193 → 36 (-81.3%)`、Renderer drain `474 → 434 ms (-8.4%)`、Yak RSS `613.3 → 558.2 MiB (-9.0%)`；吞吐保持 `200.1 req/s`。Electron CPU p50 `+25.3%`、Yak CPU p95 `+4.3%` 和 duplex p95 `+88.0%` 作为风险保留，Electron CPU p95 `-24.9%`，Long Task 中位 `569 → 0 ms`、候选最坏 `105 ms`。

当前后端上的慢消费者复验矩阵 `body-2026-07-26T08-23-13-914Z` 也通过：800 条小 Body 场景的精确对账为 `234 direct + 566 Query = 800`（其中 live stream 记录 540 fallback row）、恢复 entry/completion `1/1`、回顶到排空 `1921 ms`；240 条 64/256 KiB 双向 Body 场景为 `89 direct + 151 Query = 240`（142 fallback row）、恢复 `1/1`、回顶到排空 `2182 ms`，两者最终 Gap、重复、未匹配和缺失 ID 均为 0。基于确定的固定速率收益和恢复正确性，产品默认切到 `canary`，但没有删除 Query，也没有协议、数据库或公开 API 迁移；旧引擎 `UNIMPLEMENTED`、Gap/断流、项目/筛选切换、离顶和恢复期间都自动降级 Query。最终默认构建 smoke 为 `body-2026-07-26T09-14-17-341Z`：1000/1000、11 batch、0 Query/fallback/协议错误，request → React `492 ms`、首次可见 `42 ms`、最大可见积压 `43`。断线重放、项目/筛选迁移、长时/突发、100+ 站点及真实 Chromium/nuclei 继续作为后续回归门禁。

### Phase 4：两阶段展示

- 请求进入代理时发布 `FlowStarted(TaskID)`，立即显示“请求中”；
- 响应与写库完成后发布 `FlowCommitted(TaskID, HTTPFlowID)` 原位更新；
- 覆盖失败、超时、丢弃、WebSocket、手动劫持和项目切换；
- 通过功能开关灰度，避免临时行与数据库行重复。

### Phase 5：收敛旧链路

只有在新链路跨版本稳定至少一个发布周期后，才评估：

- 移除后端每秒数据库 watcher；
- 移除 MITM 顶部固定 `300 × 4` 追赶；
- 降低或停止旧前端持续轮询；
- 保留低频 watchdog 与 QueryHTTPFlows 恢复能力。

这是唯一可能影响旧客户端的阶段，必须单独发布迁移说明，不能与前面阶段同时完成。

## 5. 兼容与回滚策略

- protobuf 只新增字段/RPC，不修改已有字段编号和语义；
- 新消息类型旧前端可忽略；
- 新引擎收到未设置投影字段的旧客户端请求时仍返回完整包；旧引擎忽略新前端的投影字段时只会退化为传输更多数据，不影响正确性；
- 数据库只增加可空 `html_title` 列；新记录写入标题或明确的空值，迁移前记录保持 `NULL` 并走响应解析回退，没有删除、重命名或改写历史列；
- QueryHTTPFlows 和 MITMV2 的既有请求/响应语义保持兼容，投影仅由调用方显式开启；
- 实时模式必须具备 `off / shadow / canary / on` 开关；
- 任意异常可立即退回“Duplex 通知 + QueryHTTPFlows”；
- 双路径运行时只能有一个 UI 写入者，另一条路径只做对账，避免重复行。

## 6. 自动化场景

基础场景：

1. 顶部跟随，低生产速率；
2. 顶部跟随，生产速率持续高于消费速率；
3. 用户停留列表中间；
4. 向下加载历史；
5. 突发流量后停止，测量追平时间；
6. Duplex/实时流在查询进行中断开和重连；
7. 项目切换、筛选切换和 MITM 重启；
8. Renderer 暂停或模拟慢消费者；
9. 大请求/响应、WebSocket、手动劫持和插件标签后置更新；
10. 旧前端/新引擎、新前端/旧引擎兼容矩阵。

每个场景采集：

- 各阶段 P50/P95/P99；
- QueryHTTPFlows 次数、COUNT 次数、行数和字节数；
- 后端 DB 队列、实时事件队列和 Renderer pending 数；
- CPU、内存、主线程长任务；
- 最大 backlog 与停止生产后的追平时间；
- ID 缺口、重复和错误项目数据。

## 7. 当前决策记录

- `100 / 300 / 1200` 仅视为历史实现参数，不作为协议语义；
- 优先采用“摘要事件快路径 + 数据库补偿”，不推送完整请求/响应正文；
- Phase 0 先观测，Phase 1 根据证据优化现有链路；
- `IncludeSystemTiming` 继续保持显式、有界和默认开启；已有对照证明它不是当前 IPC 长任务根因；
- 不因合成 payload 变小就修改全局 protobuf/gRPC 契约；必须先有真实 Electron trace 收益和兼容性证据；
- MITM overscan 5 是前端局部优化，不改变后端协议、查询分页或其他虚拟表格行为；
- 在新链路验证前，不删除现有查询和通知机制。

## 8. 第四十轮：connection-pool 最终响应抓包借用（2026-07-26）

第三十九轮 heap 继续定位到一份完整 response 重复存储：连接池已经为 wire 抓包保留完整响应，metadata parser 又为最终固定 `Content-Length` 响应重建相同 Body。候选只在 minimartian 显式启用、discard intermediate body、连接池、有界固定长度、没有流式回调且不是 SSE 的交集路径，让 httpctx 借用连接池最终响应后缀；`100/103` 中间响应会按最终包长度精确取 suffix。chunked、未知/超限长度、非连接池、stream/SSE、非法 callback 和所有既有公开 setter 继续使用独立 owned packet。该变更没有 proto、数据库、前端调用或普通 lowhttp ownership 的破坏性更新。

256 KiB 微基准 5 次中位为 `110.392 -> 69.911 us/op (-36.7%)`、`544,440 -> 273,959 B/op (-49.7%)`、`64 -> 61 allocs`；未 opt-in 的 owned discard 分支保持约 `113.187 us/op / 544,480 B/op`。完整 utils、lowhttp、minimartian、focused race 和所有定向 ownership/短包/103/chunked 回归通过。首次 MITMV2 长套件在约 208.8 秒后非零退出但截断日志没有保留具体失败；末尾用例单独通过，完整 JSON 重跑也没有 fail action，作为 suite flake 如实保留。

同配置 shadow heap 为 `2026-07-26T14-24-26-460Z -> 2026-07-26T16-08-37-759Z`：总累计分配 `-11.86% (-77.22 MB)`，目标 response parser `-42.36% (-30,635,884 B)`，与 `120 x 256 KiB` 基本吻合，`bytes.growSlice -22.05%`；post-live `+1.24%`、positive-live `+3.45%`，因此不宣称常驻内存下降。严格 shadow 3+3 为 `body-2026-07-26T14-40-58-936Z -> body-2026-07-26T16-11-14-215Z`，比较文件 `comparison-vs-phase39-response-borrow.{json,md}`，六轮 120/120、比较器通过。request p95 `-26.0%`、首显 `-16.2%`、Renderer drain `-22.6%`，Yak CPU p95 `+0.3%`；吞吐 `-2.9%`、Yak RSS `+1.7%`、Electron CPU p95 `+11.5%`、Long Task `62 -> 110 ms`、Query p95 `65 -> 90 ms` 为交错风险，只按直接 allocation 因果保留。

扩大资源但保持并发 8 的慢消费者报告 `2026-07-26T16-17-54-684Z` 实际传输 `15 MiB request + 60 MiB response`，240/240 精确完成，`98 direct + 142 Query`，恢复 entry/completion `1/1`，Gap、缺序、重复、乱序和最终 backlog 均为 0；回顶到排空 `2117 ms`、DB/Renderer 排空 `276/311 ms`、CPU 恢复 `2018 ms`。恢复默认 canary 后，`body-2026-07-26T16-31-25-058Z` 完成 1000/1000、11 batch、0 Query/fallback/协议错误、最大可见 backlog 37。统计仍未指向 SQLite 或前端消费为本轮目标，因此没有修改 GORM、连接池规模或 Renderer；第四十一轮继续按 heap caller 归因 quote、post-parameter parsing 与 packet rebuild。

## 9. 第四十一轮：Query 参数无缓冲切分与解码快路径（2026-07-26）

第四十轮 heap 显示，建流只为计算 `PostParamsTotal` 时，form fallback 会把已是 immutable string 的 64 KiB Body 复制进 `bytes.Buffer`，再分配同尺寸 `bufio.Reader` 和每段 `ReadString`；没有 `%`/`+` 的普通值也会进入 `%u` 正则和 URL 解码。候选改为在原 string 上按 `&` 建立只读 slice，原 handler 的 trim、首个 `=`、模板 escape、顺序与 option 语义完全保留；`ForceQueryUnescape` 只对明确没有 `%`/`+` 的输入直接返回。内部 form parser 使用已经由 `httpRequestReadBody` 拥有的 snapshot 做同步只读 string view，不借用外部 mutable packet。没有 API、proto、数据库表示或前端协议变化。

旧 bufio parser 与候选对固定边界、2000 组确定性随机 byte string 和三组 option 做完整结构差分；另外单独用旧 URL 解码组合验证空值、Unicode、`%u`、`+`、非法 `%` 和无效 UTF-8。focused race、完整 codec/mutate、lowhttp（183.474 秒）和 MITMV2 MUSTPASS（203.900 秒）通过。64 KiB 整体微基准中位为 `421.846 -> 247.209 us/op (-41.4%)`、`656,162 -> 65,967 B/op (-89.9%)`、`23 -> 10 allocs`；同二进制旧/新 query parser 为 `100.802 -> 63.411 us/op (-37.1%)`、`262,371 -> 152 B/op (-99.94%)`、`9 -> 3 allocs`。

同配置 shadow heap `2026-07-26T16-08-37-759Z -> 2026-07-26T17-01-10-097Z` 中，总累计分配 `573.91 -> 514.16 MB (-10.4%)`，`GetPostCommonParams 81.43 -> 8.92 MB (-89.0%)`，旧 `ParseQueryParams 64.70 MB` 与 URL 解码正则 `40.16 MB` 栈退出 profile，`CreateHTTPFlow -34.8%`。post-live `-1.3%`、positive-live `-6.2%` 仍只按单样本记录。heap 与普通构建指纹因 diagnostic symbols 必然不同，源码 HEAD/dirty state 与 Renderer 实际输入一致。

正式 shadow 3+3 为 `body-2026-07-26T16-11-14-215Z -> body-2026-07-26T17-06-42-680Z`，比较文件 `comparison-vs-buffered-query-parser.{json,md}`。六轮 120/120、比较器通过且配置/诊断差异为空。吞吐 `+12.1%`、request p95 `-10.4%`、request/response -> React `-8.3%/-8.0%`、Renderer drain `-21.3%`、Electron CPU p95 `-17.4%`、Query p95 `-46.4%`，Yak CPU/RSS 基本持平；首显 `+8.9%`、persistence backlog `6 -> 8`、高波动 DB detection `6 -> 25 ms` 为风险，因此只声明确定性后端 allocation 收益。

canary 慢消费者矩阵 `body-2026-07-26T17-13-20-728Z` 中，800 条场景为 `184 direct + 616 fallback`、恢复 `3/3`；75 MiB 双向 Body 场景为 `91 direct + 133 fallback`、恢复 `1/1`，所有 Gap/缺序/重复/乱序和清理门禁为 0。最终默认 smoke `body-2026-07-26T17-17-53-978Z` 为 1000/1000、`200.10 req/s`、11 direct batch、0 Query/fallback/协议错误、request -> React `489 ms`、首显 `41 ms`、最大可见 backlog 36、SQLite queue/write `1/1 ms`。本轮没有修改 GORM、数据库连接参数或 Renderer。

## 10. 第四十二轮：请求 Dump 与解码报文所有权转交（2026-07-27）

第四十一轮 heap 中的 `bytes.Clone` 不是同一类问题：外部劫持响应必须 snapshot，无压缩 plain packet 也必须与可能原地修改切片的插件隔离；但 HTTP 代理请求的 dump 是 `hijackRequestHandler` 内刚分配、立即写入同一 req context 的数据。候选只对这一条证明了生命周期的路径使用 owned setter，后续 callback 仍从 context 只读取用；公开 setter、parser 输入与外部输入的 clone 语义不变。

同时，报文去 chunk/gzip/br/zstd 成功时会返回独立新 buffer，MITM plain cache 现在可以传递这个 ownership；无编码、保守回退或解码失败时仍走 clone，防止 wire packet 被插件修改污染。测试分别锁定无编码隔离与 gzip 解码后指针相同。这没有改变 MITMV2/proto/数据库或前端通信协议。

256 KiB 请求 dump 配对微基准中，中位数为 `92.878 -> 48.778 us/op (-47.5%)`、`541,207 -> 270,851 B/op (-50.0%)`、`19 -> 18 allocs`；256 KiB gzip 响应解码/cache 为 `343.930 -> 302.931 us/op (-11.9%)`、`1,769,296 -> 1,498,949 B/op (-15.3%)`、`74 -> 73 allocs`。focused race、完整 creep、lowhttp 与 MITMV2 MUSTPASS 都通过。

同配置 shadow heap `2026-07-26T17-01-10-097Z -> 2026-07-26T18-07-33-609Z` 中，目标 `SetBareRequestBytes <- hijackRequestHandler` 从 `12.33 MB` 变为消失，全局 `bytes.Clone 82.87 -> 64.77 MB (-21.8%)`。总 allocation `+1.9%`、post-live `+4.4%`、positive-live `+14.8%` 都是反向单样本，因此只保留目标重复拷贝消失的结论。正式 shadow 3+3 为 `body-2026-07-26T17-06-42-680Z -> body-2026-07-26T18-10-31-124Z`，比较文件 `comparison-vs-request-dump-owned-transfer.{json,md}`，六轮 120/120 且 comparator 通过。吞吐 `+0.9%`、request p95 `+2.2%`、request/response -> React `+0.4%/+3.6%`、Yak CPU p95 `-0.3%`、Yak RSS `-3.0%`；Renderer drain `539 -> 879 ms` 与 Query p95 `48.2 -> 64.3 ms` 作为无直接 caller 但反向的风险保留。

额外 canary body matrix `body-2026-07-26T17-55-20-869Z` 的 small、64 KiB request、256 KiB response 和双向四档都 120/120 通过。最终 `body-2026-07-26T18-16-26-961Z` 在 200 req/s 下完成 1000/1000，11 direct batch、0 Query/fallback/协议错误、最大可见 backlog 32、最终 backlog 0、request -> React `488 ms`、首显 `55 ms`、SQLite queue/write `1/1 ms`。数据仍未指向 SQLite、GORM 或前端消费是本轮主要瓶颈；下一轮优先对 heap 中 31--39 MB 字符集 transform 做 ASCII 等价性验证，并把压缩响应纳入端到端自动化。

## 11. 第四十三轮：已验证 UTF-8 恒等解码移交与 gzip 端到端（2026-07-27）

本轮先把 gzip 纳入 Electron 自动化，而不是拿 identity Body 推断压缩链路。harness v9 新增 `responseContentEncoding` 可比性字段，target 预压缩确定性正文，producer 校验 `Content-Encoding` 和精确 wire bytes，详情门禁再校验 MITM 落库后的解压正文。120 条、256 KiB decoded Body 的首轮中，每条 wire Body 为 318 B、详情 Body 为 262,144 B，全部通过；identity 与 gzip 不能被比较器误配。

压缩 heap 基线 `2026-07-26T18-33-30-456Z` 将 `23,984,547 B` 明确归因到 `FixHTTPResponsePacket -> TryUTF8Convertor -> transform.Bytes`。`mimecharset.FromPlain` 已经验证 JSON Body 是 UTF-8，后续 UTF-8 decoder 只产生等长副本。候选只在检测结果为 `utf-8/utf8` 时复用原 bytes，并保留原来的 `converted=true`；显式 charset、HTML meta、GBK/GB18030、未知 charset 和 U+FFFD 保护均不绕过。ASCII/中文 UTF-8 与旧 decoder 差分、backing pointer、显式 UTF-8 和 replacement-rune oracle 均通过。

256 KiB 配对微基准 5 次中位为 `472.510 -> 195.724 us/op (-58.6%)`、`262,188 -> 0 B/op`、`3 -> 0 allocs/op`。focused race、完整 codec、完整 lowhttp（主包 `186.809 s`）和全部 MITMV2 MUSTPASS（`205.534 s`）通过。gzip heap 候选 `2026-07-26T18-44-00-038Z` 中目标 `23.98 MB` transform 栈消失，总 allocation `550,909,308 -> 515,346,459 B (-6.5%)`，positive-live `-7.1%`；post-live `+0.7%` 视为单样本噪声。

正式 gzip shadow 3+3 为 `body-2026-07-26T18-36-06-289Z -> body-2026-07-26T18-48-25-177Z`，比较文件 `comparison-vs-utf8-identity-decoder.{json,md}`。六轮 120/120，配置/诊断差异为空。backend conversion p95 `-22.7%`、per-flow conversion p95 `-52.9%`、吞吐 `+1.1%`、request p95 `-1.4%`、Renderer drain `-15.6%`、Yak CPU p95 `-0.2%`、RSS `+1.4%`。backend Query p95 `+72.5%`、Electron CPU p95 `+27.3%`、Long Task `56 -> 61 ms` 是反向风险；Query 候选有 `34.978 ms` 离群点且本轮无对应 caller，因此只声明恒等 decoder 的确定性收益，不宣称 UI 全指标提速。

扩大 canary `body-2026-07-26T19-08-22-255Z`（报告 `2026-07-26T19-08-22-892Z`）以 400 条、100 req/s、并发 12 传入共 100 MiB decoded response。结果 400/400、完成/调度 `99.48/99.99 req/s`、10 direct batch、400 direct row，Query/fallback/Gap/缺序/重复/乱序/unavailable 均为 0，最大可见 backlog 38、最终 0、CPU `2021 ms` 恢复。数据继续指向响应修复分配而非 SQLite、GORM 或 Renderer 消费；本轮不修改数据库连接、GORM、proto 或前端产品链路。

## 12. 第四十四轮：gzip ISIZE 有界预分配（2026-07-27）

第四十三轮压缩 heap 的最大剩余 caller 是 `io.ReadAll 287,915,925 B`。同一解压正文在 transport response 修复和隔离的 plugin plain cache 中各需要一份；直接共享两者会让落库/修复报文与插件原地修改产生 ownership 耦合，因此本轮不合并这两份必要输出，只清理每次 `io.ReadAll` 的几何扩容。gzip trailer 的 ISIZE 现在仅作为初始容量提示：不可信预分配独立限制为 1 MiB，原 32 MiB 解压上限继续由 `LimitReader` 强制执行，reader 仍必须读到 EOF 完成 CRC/长度校验。错误 hint、超限、拼接 member、校验和损坏和读取错误均保持保守回退，没有 API、proto、wire、数据库、GORM、报文 ownership 或前端产品改动。

256 KiB 配对微基准 5 次中位为 `254.150 -> 90.785 us/op (-64.3%)`、`1,227,316 -> 311,595 B/op (-74.6%)`、`28 -> 8 allocs (-71.4%)`。focused race、完整 lowhttp（`92.900 s`）和全部 MITMV2 MUSTPASS（`197.562 s`）通过。同配置 gzip heap `2026-07-26T18-44-00-038Z -> 2026-07-26T19-36-18-312Z` 中，总 allocation `515,346,459 -> 293,335,322 B (-43.1%)`，旧 `io.ReadAll 287.92 MB` 消失，`_decodeBody -75.1%`、`ContentEncodingDecode -75.4%`、`DeletePacketEncodingWithOwnership -60.8%`；新 helper 的 `62.40 MB` 基本对应两份 `120 x 256 KiB` 必要输出。positive-live `-38.8%`、post-live `-0.6%` 仍只按单样本诊断记录。

正式同构 shadow 3+3 为 `body-2026-07-26T18-48-25-177Z -> body-2026-07-26T19-46-59-578Z`，比较文件 `comparison-vs-gzip-size-hint.{json,md}`。六轮 120/120，配置/诊断差异为空。吞吐 `+24.6%`、request p95 `-7.3%`、首显 `316 -> 186 ms (-41.1%)`、Query RTT p95 `-63.8%`、Yak drain CPU p95 `-43.1%`、Yak RSS `-4.9%`；反向风险为 DB catch-up/drain `+38.2%/+26.0%`、最大可见 backlog `80 -> 104` 和 Electron CPU p95 `+4.8%`。这些下游短样本波动不影响直接 allocation 因果，但阻止“所有 UI 指标都变快”的结论。

更大 canary `body-2026-07-26T19-52-36-960Z`（报告 `2026-07-26T19-52-37-603Z`）以 400 条、100 req/s、并发 12 处理 100 MiB decoded gzip response，完成 400/400、`99.69 req/s`、request p95 `71.72 ms`，9 direct batch、400 direct row，Query/fallback/Gap/缺序/重复/乱序/unavailable 为 0，最大 persistence/visible backlog `2/14`，停止时与最终 backlog 均为 0，CPU `2019 ms` 恢复。上一轮固定速率基线只有一个样本，比较器按规则拒绝正式 A/B；其方向性改善与 DB catch-up 反向只作观察，不冒充统计结论。下一轮继续从新 heap 的 packet rebuild、quote 与 SQLite bind caller 分层选择，仍以数据决定是否进入 GORM/数据库或前端。

## 13. 第四十五轮：owned decoded body 原地组包与 MITMV2 生命周期 race（2026-07-27）

第四十四轮 heap 中，解压产生的独立 Body 仍会在重写 HTTP Header 时被完整复制一次。候选仅在内部 caller 已证明 Body 独占且尾部 capacity 足够容纳 Header 时，在同一 allocation 内先把 Body 右移、再写 Header 前缀；capacity 不足和公开 borrowed API 均继续分配。公开 `ReplaceHTTPPacketBodyEx` 不消费输入，公开 `FixHTTPResponse` 的 Body 隔离语义也不变。exact bytes、同 backing、capacity fallback、borrowed non-mutation/isolation 和 gzip wire preservation 测试全部通过。

五次 256 KiB gzip decode + packet rebuild 微基准中位为 `164.767 -> 117.762 us/op (-28.5%)`、`583,286 -> 312,691 B/op (-46.4%)`、`39 -> 36 allocs`。focused race、完整 lowhttp（`183.237 s`）和 62 个 MITMV2 MUSTPASS（`195.011 s`）通过。同配置 heap `2026-07-26T19-36-18-312Z -> 2026-07-26T20-30-29-674Z` 中，总 allocation `293.34 -> 218.18 MB (-25.6%)`，旧 `ReplaceHTTPPacketBodyEx 66.96 MB` caller 消失，`bytes.growSlice -52.7%`、`DeletePacketEncodingWithOwnership -56.8%`；post-live `-1.85%`，positive-live delta `+59.8%`，后者按单样本风险保留，不宣称常驻内存改善。

race detector 同时发现并修复了已有的 MITMV2 多 session 生命周期问题：全局 plugin caller/channel 现在通过 RW mutex 原子注册和 snapshot，只允许持有同一 caller/channel 的 session 清理自己并关闭自己的通知 channel；异步插件加载、drop 入库、response mirror 建流和 HookColor 入库不再写主协程外层 `err`。64 路并发生命周期测试与真实手动劫持 race 均通过，消除了数据竞争和潜在 double-close；这部分没有更改通信协议。

正式 shadow 3+3 为 `body-2026-07-26T19-46-59-578Z -> body-2026-07-26T20-39-54-761Z`，比较文件 `comparison-vs-owned-packet-fold.{json,md}`。六轮 120/120，比较器通过且配置/诊断差异为空。吞吐 `+14.0%`、Electron CPU p95 `-9.2%`、Yak drain CPU p95 `-8.3%`、request -> React `-1.9%`；request p95 `+3.8%`、首显 `+17.7%`、DB catch-up/drain `+22.1%/+25.9%`、Renderer drain `+23.2%`、Query RTT `+88.0%` 为反向短样本风险，因此仅声明确定性的后端分配收益。

扩大 canary `body-2026-07-26T20-45-36-491Z`（报告 `2026-07-26T20-45-37-147Z`）完成 400/400、`99.69 req/s`、request p95 `80.51 ms`，9 direct batch、400 direct row，Query/fallback/Gap/缺序/重复/乱序/unavailable 为 0，最大 persistence/visible backlog `6/48`，最终 backlog 0，CPU `2021 ms` 恢复。本轮没有前端产品、proto、数据库 schema、连接池或 GORM 改动；下一轮先对 `quoteHTTPPacket` 的数据库表示与兼容边界做定点差分，不能为省分配直接改变历史存储格式。

## 14. 第四十六轮：quoted HTTP packet 输出 ownership（2026-07-27）

HTTPFlow 的 request/response 历史上以 `strconv.Quote` 的逐字节结果存储，并由 `strconv.Unquote` 读取。标准库路径先生成保守预分配的 byte buffer，最后再把整份输出复制成 string。候选继续调用标准库 `strconv.AppendQuote`，只把函数内新建且之后绝不修改的输出 buffer 移交给 immutable string；输入 packet 仍为只读 view 并通过 `runtime.KeepAlive` 保活。nil/empty、普通 HTTP、Unicode、全 256 byte、非法 UTF-8 和输入突变隔离 oracle 全部通过，数据库格式、schema 和历史读取兼容性不变。

五次 256 KiB 微基准中，当前 read-only input/copy output 路径中位为 `1.450 ms/op / 671,746 B/op / 2 allocs`，候选为 `1.407 ms/op (-3.0%) / 401,409 B/op (-40.2%) / 1 alloc`。focused race、完整 yakit persistence（`49.375 s`）和 62 个 MITMV2 MUSTPASS（`192.017 s`）通过。同配置 heap `2026-07-26T20-30-29-674Z -> 2026-07-26T21-01-00-184Z` 中，`quoteHTTPPacket 79.80 -> 53.28 MB (-33.2%)`，总 allocation `218.18 -> 206.00 MB (-5.6%)`；gzip reader caller 单样本反向 `+13.3%` 抵消部分总量。positive-live `-33.3%`、post-live `+0.7%`，不作常驻内存声明。

正式 shadow 3+3 为 `body-2026-07-26T20-39-54-761Z -> body-2026-07-26T21-04-57-340Z`，比较文件 `comparison-vs-quote-output-handoff.{json,md}`。六轮 120/120、比较器通过、配置/诊断差异为空。吞吐 `+6.6%`、request p95 `-11.9%`、Yak drain CPU p95 `-37.0%`、Yak RSS `-2.1%`、Query RTT `-5.7%`；DB catch-up/drain `+15.1%/+7.1%`、Renderer drain `+19.7%`、request -> React `+6.8%`、Electron CPU p95 `+2.8%` 为反向风险。

扩大 canary `body-2026-07-26T21-10-40-565Z`（报告 `2026-07-26T21-10-41-230Z`）完成 400/400、`100.14 req/s`、request p95 `64.52 ms`，9 direct batch、400 direct row，Query/fallback/Gap/缺序/重复/乱序/unavailable 为 0，最大 persistence/visible backlog `2/48`，最终 backlog 0，CPU `2020 ms` 恢复。

剩余约 30 MB SQLite bind 分配位于外部 driver 的 `string -> []byte -> SQLITE_TRANSIENT`。从 GORM 传 `[]byte` 会按 BLOB 绑定，可能改变 TEXT affinity、比较和检索语义；`CAST` 方案又引入 SQLite 专属存储契约，因此本轮不采用，也没有修改或推送 SQLite driver/GORM。下一轮优先处理 heap 中可以由后端安全控制的 gzip reader/flate 重复初始化，再依据新 profile 决定是否需要用户授权扩展 driver 边界。

## 15. 第四十七轮：gzip reader/flate 状态复用（2026-07-27）

第四十六轮 heap 将约 9 MB 累计分配归因到每次 gzip 解码重新创建 reader/flate。候选通过 `sync.Pool` 复用同时拥有 `gzip.Reader` 与 `bytes.Reader` 的 wrapper，按标准库 `Reset` 契约切换输入；归还前关闭流、清空 source，并把 gzip reader 重置到 wrapper 自己的空 source，避免池对象持有历史响应。错误路径同样归还，原 32 MiB 上限、ISIZE 有界 hint、EOF/CRC、multistream 和失败时保留 wire packet 的语义不变。错误恢复、校验失败后复用、32 路并发、focused race、完整 lowhttp 与 62 个 MITMV2 MUSTPASS 全部通过。

五次 256 KiB 配对微基准中位为 `98.340 -> 89.620 us/op (-8.9%)`、`311,595 -> 270,388 B/op (-13.2%)`、`8 -> 2 allocs`。同配置 heap `2026-07-26T21-01-00-184Z -> 2026-07-26T21-30-52-650Z` 中，总 allocation `206.00 -> 195.01 MB (-5.3%)`，约 `9.18 MB` reader 创建与 `8.65 MB` flate dictionary 初始化栈消失；post-live `-0.56%`，positive-live `+42.4%` 作为 forced-GC 单样本风险保留。

正式 shadow 3+3 为 `body-2026-07-26T21-04-57-340Z -> body-2026-07-26T21-35-03-174Z`，比较文件 `comparison-vs-gzip-reader-pool.{json,md}`。六轮 120/120、比较器通过且配置/诊断差异为空。DB catch-up/drain `-21.0%/-18.3%`、Electron CPU p95 `-7.7%`、Query RTT `-54.1%`；吞吐 `-3.6%`、request p95 `+9.4%`、首显 `+12.0%`、Renderer drain `+39.7%`、Yak drain CPU p95 `+71.7%` 为反向短样本风险，只声明直接 allocation 收益。

扩大 canary `body-2026-07-26T21-40-43-180Z`（报告 `2026-07-26T21-40-43-821Z`）完成 400/400、`99.38 req/s`、request p95 `65.02 ms`，9 direct batch、400 direct row，0 Query/fallback/Gap/缺序/重复/乱序/unavailable，最大 persistence/visible backlog `6/53`，最终 backlog 0，CPU `2022 ms` 恢复。本轮没有前端产品、proto、数据库 schema、连接池或 GORM 改动；下一轮继续按 heap 选择不改变存储和 ownership 契约的后端 caller。

## 16. 第四十八轮：HTTPFlow quote 自适应容量（2026-07-27）

第四十七轮 heap 中 `quoteHTTPPacket` 仍为约 47 MB：标准库的 50% 保守预留适合未知字符串，但普通 HTTP 只需外层引号和少量 Header 转义。候选最多抽样 packet 首尾共 4 KiB：普通文本预留 12.5%，转义密集或非法 UTF-8 保留原 50%。最终编码仍完全由 `strconv.AppendQuote` 完成，误判只会触发标准 slice 扩容，不改变 quoted bytes、数据库 TEXT 或历史读取。全 byte/非法 UTF-8/Unicode/输入突变 oracle 与容量分支测试通过。

五次 256 KiB 普通 HTTP 基准中位为 `1.417 -> 1.432 ms/op (+1.0%)`、`401,409 -> 303,104 B/op (-24.5%)`、两者均一次分配。控制字符和全 byte 输入保持原扩容次数，分配字节仅 `+0.18%/+0.35%`，中位耗时约 `+1.9%/+0.6%`。focused race、完整 yakit persistence（`85.610 s`）和 62 个 MITMV2 MUSTPASS（`191.504 s`）通过。

同配置 heap `2026-07-26T21-30-52-650Z -> 2026-07-26T21-59-36-191Z` 中，`quoteHTTPPacket 47.27 -> 35.90 MB (-24.1%)`，总 allocation `195.01 -> 178.41 MB (-8.5%)`，`bytes.growSlice -9.1%`，positive-live `-11.6%`；post-live `+2.2%` 作为单样本风险。正式 shadow 3+3 为 `body-2026-07-26T21-35-03-174Z -> body-2026-07-26T22-09-02-145Z`，比较文件 `comparison-vs-adaptive-quote-capacity.{json,md}`，六轮 120/120、配置/诊断差异为空。DB catch-up/drain、Renderer drain、可见 backlog 与 Yak RSS 改善；吞吐 `-4.1%`、request p95 `+4.7%`、Query RTT `+207.6%`、Long Task `0 -> 53 ms` 等反向项如实保留，只声明直接分配收益。

扩大 canary `body-2026-07-26T22-14-43-186Z`（报告 `2026-07-26T22-14-43-829Z`）完成 400/400、`100.12 req/s`、request p95 `71.57 ms`，9 direct batch、400 direct row，0 Query/fallback/Gap/缺序/重复/乱序/unavailable，最大 persistence/visible backlog `4/50`，最终 backlog 0，CPU `2018 ms` 恢复。本轮没有前端产品、proto、数据库 schema、GORM 或 driver 改动；后续将重新按 heap/CPU 归因剩余 decoded output、SQLite bind 与列表侧波动，避免为了总量改变 ownership 或 TEXT affinity。

## 17. 第四十九轮：ASCII HTTPFlow quote 快路径（2026-07-27）

第四十八轮 400 条固定速率 CPU profile `2026-07-26T22-25-45-716Z` 中，5 秒采样 `6.15 CPU s`，memory/GC flat `2.07 s`，`quoteHTTPPacket` 累计 `1.16 s`，其中标准 `strconv` quote 为 `0.74 s`。候选仅对全 ASCII packet 使用逐 byte 等价 encoder；quote/backslash、命名控制字符、其他控制字节和 DEL 映射与标准库完全相同，遇到任一非 ASCII byte 就用同一 buffer 回退 `strconv.AppendQuote`。全 128 ASCII、全 256 byte、Unicode、非法 UTF-8 与输入突变 oracle 锁定数据库 TEXT 字节兼容。

五次 256 KiB ASCII HTTP 配对基准中位 `1.408 ms -> 216.288 us (-84.6%)`，两者都是约 `303,105 B/op / 1 alloc`；Unicode 只出现在末尾的最坏回退为 `1.413 -> 1.420 ms (+0.5%)`。focused race、完整 yakit persistence（`56.103 s`）和 62 个 MITMV2 MUSTPASS（`199.129 s`）通过。对应 CPU profile `2026-07-26T22-40-14-236Z` 中，总样本 `6.15 -> 5.30 CPU s (-13.8%)`、平均 CPU `123% -> 106%`、`quoteHTTPPacket 1.16 -> 0.38 s (-67.2%)`、memory/GC flat `2.07 -> 1.86 s (-10.1%)`；标准 quote 栈消失，ASCII encoder仅 `0.11 s`。

正式 shadow 3+3 为 `body-2026-07-26T22-09-02-145Z -> body-2026-07-26T22-44-22-972Z`，比较文件 `comparison-vs-ascii-quote-fast-path.{json,md}`。六轮 120/120、配置/诊断差异为空。吞吐 `+44.8%`、request p95 `-20.8%`、Electron CPU p95 `-12.1%`、Yak drain CPU p95 `-30.3%`、Long Task `53 -> 0 ms`；Renderer drain `+60.7%`、最大 visible backlog `+32.2%`、Yak CPU p95 `+2.4%`、RSS `+3.6%` 为反向项，部分来自无速率上限时更快的生产端向下游施压。

固定速率 canary `body-2026-07-26T22-50-06-164Z`（报告 `2026-07-26T22-50-06-812Z`）完成 400/400、`100.15 req/s`、request p95 `42.55 ms`、DB/Renderer drain `291/329 ms`，9 direct batch、0 Query/fallback/Gap/缺序/重复/乱序/unavailable，最大 persistence/visible backlog `4/18`，最终 backlog 0，CPU `2019 ms` 恢复。相对第四十八轮单次 canary 的 request p95 `71.57 ms`、drain `358/392 ms`、visible backlog 50，方向一致但不冒充重复 A/B。

本轮还试验并拒绝了 gzip exact-hint 输出分配：反转顺序的 7 次基准中，候选 `91.35 us/op` 对原路径 `82.95 us/op (+10.1%)`，只省 `3.0% B/op` 且分配次数不变，已完整撤回。两份独立 decoded output 和 SQLite TEXT bind 继续保留；本轮没有前端产品、proto、schema、GORM 或 driver 改动。

## 18. 第五十轮：低转义密度 quote 容量与扩大 A/B/A（2026-07-27）

第四十九轮已消除 ASCII quote 的主要 CPU，但普通 HTTP 仍固定预留 1/8，256 KiB 输出落在约 303 KiB size class。候选把同一 4 KiB 抽样分布到首/中/尾：转义密度不高于 1/64 时只预留 1/64，中等密度保持 1/8，高于 1/8 或非法 UTF-8 保持 1/2。中段抽样避免只看 Header/尾部漏掉 Body 中央的二进制或控制字符；容量仍只是 hint，最终 ASCII encoder/标准库 fallback、数据库 TEXT、历史 Unquote 和扩容正确性均不变。普通、轻转义、Unicode、中转义、高转义、中部控制字符、非法 UTF-8 和中部非 ASCII 测试通过。

五次 256 KiB 微基准中位为 `214.735 -> 206.292 us/op (-3.9%)`、`303,105 -> 270,336 B/op (-10.8%)`，均为一次分配；末尾 Unicode fallback 与直接标准路径均约 `1.402 ms/op / 270,336 B/op / 1 alloc`。focused race、完整 yakit persistence（`68.069 s`，峰值 `876,944 KiB`）和 62 个 MITMV2 MUSTPASS（`191.994 s`，峰值 `3,286,952 KiB`）通过，均无 swap。

120 条 heap 同代对照仍被 Go 大对象采样方差淹没，因此没有挑选有利结果。扩大到 400 条、100 req/s、100 MiB decoded gzip response 后，同配置 `1/8` 控制 `2026-07-26T23-48-02-204Z` 与 `1/64` 候选 `2026-07-26T23-45-50-519Z` 均 400/400，`quoteHTTPPacket 125.65 -> 109.90 MB (-12.5%)`、总 allocation `577.61 -> 560.02 MB (-3.0%)`，`bytes.growSlice +0.3%`、SQLite bind `-0.2%` 基本持平；positive-live `-3.1%`、post-live `-0.2%` 只作 forced-GC 诊断。

同配置 5 秒 CPU `2026-07-26T22-40-14-236Z -> 2026-07-26T23-50-40-369Z` 中，目标 `quoteHTTPPacket 380 -> 290 ms (-23.7%)`、ASCII encoder `110 -> 60 ms`；总样本 `5.30 -> 5.45 CPU s (+2.8%)`、memory/GC `1.86 -> 2.20 s`、scanobject `1.37 -> 1.63 s` 反向，因此不宣称全局 CPU/GC 改善。

正式无 profile 门禁使用候选 A1 `body-2026-07-26T23-53-13-414Z`、`1/8` 控制 B `body-2026-07-26T23-58-57-106Z`、候选 A2 `body-2026-07-27T00-05-18-037Z` 的 A/B/A 夹心设计，每组 3 轮、每轮 400 条。两个比较文件分别为 `comparison-vs-low-density-quote-capacity.{json,md}` 和 `comparison-vs-low-density-quote-capacity-sandwich.{json,md}`，均 passed、配置/诊断差异为空，九轮全部 400/400。A1 对 B 的 request p95/Electron CPU p95/DB drain/Renderer drain 为 `+50.9%/+18.5%/-14.8%/-14.2%`，A2 对 B 则反向为 `-27.7%/-0.2%/+27.2%/+24.1%`，证明这些短样本指标主要随运行顺序波动。稳定项为吞吐约 100 req/s、两组 request -> React 均 `-0.2%`、response -> React `-1.4%/+1.2%`、Long Task 0，所有 Query/fallback/Gap/缺序/重复/乱序/unavailable 与清理错误为 0。

本轮保留确定性的 quote/总分配收益，不宣称 UI 全指标或全局 CPU 提升。前端产品、proto、schema、连接池、GORM 与 SQLite driver 均未修改；下一轮重新按扩大后的 heap/CPU 选择后端 caller，优先避开已经证实会改变 TEXT affinity 或 ownership 契约的激进方案。

## 19. 第五十一轮：标准 HTTP packet View 快路径（2026-07-27）

第五十轮 heap 中 `splitHTTPPacketEx` cumulative 仍有 `29.36 MB`。原只读 Body View 虽已不复制 256 KiB Body，仍逐行经过 bufio、为每个 Header 建 string，再由 `strings.Builder` 重建完整 Header。候选只在没有逐 Header hook 且报文严格符合标准 CRLF 时直取边界：首行不得需要 trim，Header 行不得含异常 CR/LF，请求中的空白终止行继续回退；请求首行 callback 仍由原 parser 解析并保持 abort 语义。返回 Header 只复制一次且与输入独立，Body 仍为只读 alias。LF-only、prefix、额外 CR、折叠/非标准边界和公开 Body clone API 继续走旧 parser，没有公开签名或 ownership 变化。

请求/HTTP/RTSP 响应、folded Header、大小写混合 Content-Length、空白/二进制 Body、畸形换行、prefix、callback/abort、Header 独立性、Body alias 和并发差分全部通过；15 秒 fuzz 执行 81,845 个输入无差异。focused race、完整 lowhttp（`188.127 s`，峰值 `487,104 KiB`）、完整 yakit persistence（`49.381 s`，峰值 `876,184 KiB`）和 62 个 MITMV2 MUSTPASS（`190.365 s`，峰值 `3,315,576 KiB`）通过，均无 swap。

五次 256 KiB 同二进制配对基准中，无回调 View 为 `809.7 -> 172.6 ns/op (-78.7%)`、`618 -> 96 B/op (-84.5%)`、`16 -> 1 alloc`；`CreateHTTPFlow` 使用的请求首行 callback 形式为 `731.2 -> 206.6 ns/op (-71.7%)`、`512 -> 104 B/op (-79.7%)`、`15 -> 2 allocs`。

同配置 400 条 gzip heap `2026-07-26T23-45-50-519Z -> 2026-07-27T00-33-58-280Z` 均 400/400，100 req/s、318 B wire 与 262,144 B decoded detail 精确一致。总 allocation `560.02 -> 552.38 MB (-1.36%)`；旧 parser cumulative `29.36 -> 17.30 MB (-41.1%)`，计入新快路 `3.15 MB` 后整个 split 实现约 `20.45 MB (-30.4%)`；builder `-55.0%`、bufio reader `-55.6%`。positive-live `-39.0%`、post-live `+1.9%` 只作 forced-GC 诊断。

同配置 CPU `2026-07-26T23-50-40-369Z -> 2026-07-27T00-47-15-795Z` 总样本 `5.45 -> 5.52 CPU s (+1.3%)`，`scanobject 1.63 -> 1.32 s (-19.0%)`，目标 split 栈低于稳定归因量级，因此不宣称全局 CPU 改善。候选仍 400/400、最终 backlog 0、CPU 正常恢复。本轮没有前端产品、proto、schema、连接池、GORM 或 SQLite driver 改动；下一轮按剩余 17.30 MB 旧 parser caller 区分逐 Header hook 与真正需要规范化输出的路径。

## 20. 第五十二轮：逐 Header hook 快路与完整预校验（2026-07-27）

第五十一轮剩余旧 parser 主要来自 MITM response fix 与建流逐 Header hook。候选先验证完整 Header block，再复制一次不可变 Header string，并把其中的逐行 slice 交给 hook；标准 CRLF 请求/响应不再为每行单独建 string 或经 bufio/builder 重建。folded response、LF-only 和其他非规范形式会在执行任何 hook 前回退旧 parser，Body clone API 与公开签名不变。完整预校验同时修复了一个边缘语义：第五十一轮若首行规范而后续 Header 非规范，request callback 可能先执行、回退后再执行；现在所有副作用只发生一次。

静态差分覆盖请求、响应、folded/LF-only、hook 留存所有权和 callback abort；15 秒 fuzz 同时对拍无 hook、逐 Header hook 与请求 callback，75,911 次无差异。focused race 峰值 `538,760 KiB`，完整 lowhttp `182.475 s / 488,080 KiB`、yakit persistence `52.822 s / 876,888 KiB`、62 个 MITMV2 MUSTPASS `195.540 s / 3,316,704 KiB` 全部通过且无 swap。

五次 256 KiB 微基准中位：逐 Header hook 为 `792.5 -> 211.0 ns/op (-73.4%)`、`618 -> 96 B/op (-84.5%)`、`16 -> 1 alloc`；request callback 为 `730.0 -> 191.8 ns/op (-73.7%)`、`512 -> 80 B/op (-84.4%)`、`15 -> 1 alloc`。同配置 400 条 gzip heap `2026-07-27T00-33-58-280Z -> 2026-07-27T01-14-29-233Z` 中，旧 parser cumulative `17.30 -> 8.91 MB (-48.5%)`，计入新快路后 split 总量约 `20.45 -> 11.54 MB (-43.6%)`；总 sampled allocation `552.38 -> 510.16 MB (-7.6%)`，但只将直接 split 差值归因给本轮。positive-live `+27.7%`、post-live `+5.2%` 反向，明确不宣称常驻内存下降。

CPU 报告 `2026-07-27T00-47-15-795Z -> 2026-07-27T01-20-18-423Z` 总采样 `5.52 -> 5.15 CPU s (-6.7%)`、平均 CPU `110.4% -> 103.0%`，但 split 低于采样阈值且 `scanobject` flat `1.32 -> 1.38 s`，因此不作全局 CPU 改善结论。heap/CPU 两轮均 400/400、9 direct batch、0 Query/fallback/Gap/顺序错误，最终 backlog 0、CPU 恢复。前端产品、proto、schema、数据库配置、GORM 与 SQLite driver 均未修改；下一轮由最新 heap/CPU 的剩余后端 caller 决定目标。

## 21. 第五十三轮：response Header 分类去重复 lowercase（2026-07-27）

最新 heap 继续指向 `fixHTTPResponse` 的逐 Header hook：旧实现为识别 Content-Type 先 lowercase 一次，再为 Transfer/Content-Encoding 对同一行 lowercase 一次。候选改为 ASCII case-folded 前缀判断，只在真正命中 Transfer-Encoding 或 Content-Encoding 时 lowercase；Content-Type 保留原值，传给解码器的完整小写 Content-Encoding 字符串也保持历史契约。大小写混合 gzip/chunked 端到端用例、静态 legacy 差分、15 秒 fuzz（29,446 次）、focused race、完整 lowhttp/yakit persistence 和 62 个 MITMV2 MUSTPASS 全部通过，长门禁峰值 `3,287,024 KiB` 且无 swap。

六条常见响应 Header 的五次微基准中位为 `700.0 -> 149.1 ns/op (-78.7%)`、`304 -> 24 B/op (-92.1%)`、`12 -> 1 alloc`。完整 256 KiB API 受 Body 主成本和运行噪声支配：clone-Body 路径 `+2.7%`、packet-only `-1.6%`，但两者均减少 4 次分配，因此只将 Header 分类微基准作为确定性收益。

同配置 heap `2026-07-27T01-14-29-233Z -> 2026-07-27T02-49-20-863Z` 中，`fixHTTPResponse -> strings.ToLower` 的约 `1.0 MiB` 采样调用栈消失；剩余 `0.5 MiB` 来自另一条 MITMV2 plain-response decode/cache 路径。总 sampled allocation `+3.8%`、positive-live `+16.8%`、post-live `-3.2%` 方向交错，不作全局 heap 或常驻内存声明。CPU `2026-07-27T01-20-18-423Z -> 2026-07-27T02-55-06-007Z` 总样本 `5.15 -> 5.62 CPU s`，目标低于整链路分辨率，也不宣称全局 CPU 改善。

heap/CPU 两轮均完成 400/400、约 100 req/s、400 direct row，318 B gzip wire 与 262,144 B decoded Body 精确一致；Query/Gap/缺序/重复/乱序/unavailable 为 0，最终 persistence/visible backlog 为 0，CPU 正常恢复。本轮没有前端产品、通信协议、proto、schema、数据库配置、GORM 或 SQLite driver 变更。下一轮从最新 heap/CPU 重新选择可由后端控制且不改变 decoded-output、TEXT affinity 或 ownership 契约的 caller。

## 22. 第五十四轮：复用已有 response writer（2026-07-27）

最新 heap 的 `bufio.NewWriterSize` 约 5.8 MiB，其中 `dumpHTTPResponse` 每流都会在已有 `bufio.ReadWriter`、`bytes.Buffer` 或 `MultiWriter` 外再套一层 4 KiB buffer；频繁 Flush 只搬到内层 writer，并不真正刷新 socket。候选直接复用已有 `io.StringWriter`，中间 Flush 为 no-op；只实现 `io.Writer` 的通用目标继续使用原 bufio fallback，因而不改变网络 flush 所有权、公开 API、报文字节或 Body 恢复契约。direct/fallback 逐字节测试、focused race、完整 utils/minimartian/crep/yakit persistence 与 62 个 MITMV2 MUSTPASS 全部通过，长门禁峰值 `3,317,980 KiB` 且无 swap。

五次 256 KiB 微基准中，writer-only 中位 `2.209 -> 1.140 us/op (-48.4%)`、`4,272 -> 176 B/op (-95.9%)`、`9 -> 8 allocs`；内存 Dump 为 `72.557 -> 58.233 us/op (-19.7%)`、`274,851 -> 270,755 B/op`、`13 -> 12 allocs`。所有路径都精确消除 `4,096 B / 1 alloc`，没有确定性耗时回退。

同配置 heap `2026-07-27T02-49-20-863Z -> 2026-07-27T03-17-09-362Z` 中，`dumpHTTPResponse -> bufio.NewWriterSize` 调用栈完全消失，全部 `bufio.NewWriterSize 5,789,723 -> 4,210,708 B (-27.3%)`；约 1.58 MiB 采样差值接近 400 流理论值 1.56 MiB。总 allocation `+1.8%`，positive-live `-64.8%`、post-live `-3.1%`，只声明直接 writer 分配收益。

CPU `2026-07-27T02-55-06-007Z -> 2026-07-27T03-21-49-203Z` 总样本 `5.62 -> 5.72 CPU s (+1.8%)`，GC/scanobject 则分别 `-11.8%/-7.7%`，目标低于采样分辨率，不作全局 CPU 结论；CPU 单次 request p95 `66.87 -> 122.61 ms` 反向，而候选 heap 轮为 `69.57 ms`，保留为短样本时序风险。heap/CPU 均 400/400、约 100 req/s、400 direct row、0 Query/Gap/缺序/重复/乱序/unavailable，最终 backlog 0、CPU 恢复。本轮没有前端产品、协议、proto、schema、数据库、GORM 或 driver 改动；下一轮继续从真实剩余连接 writer、GORM clone 与 parser caller 中按 profile 选点。

## 23. 第五十五轮：bare-flow KV 单语句 SQLite upsert（2026-07-27）

最新 GORM clone 主要由上层每流 bare wire 存储放大：400 个 gzip flow 的 key 天然唯一，旧 `FirstOrCreate` 仍逐条先 SELECT 再 INSERT。候选只对 SQLite 项目库且只对 `BARE_REQUEST/BARE_RESPONSE` 两组使用 fork 已发布的 `ON CONFLICT("key") DO UPDATE`；其他 group/dialect 原样保留 FirstOrCreate。冲突时只更新 value/group/updated_at，ID、CreatedAt、ExpiredAt、ProcessEnv、Verbose 与读取/Quote 格式均由测试锁定，没有 schema 或迁移。

事务内连续唯一 key 的五次微基准中位为 `75.928 -> 34.486 us/op (-54.6%)`、`24,961 -> 12,915 B/op (-48.3%)`、`380 -> 189 allocs (-50.3%)`。focused race、完整 yakit persistence 与 62 个 MITMV2 MUSTPASS 通过，峰值分别约 `945 MiB/856 MiB/3.15 GiB`，均无 swap。

同配置 heap `2026-07-27T03-17-09-362Z -> 2026-07-27T03-40-23-266Z` 中，bare-KV caller 的 FirstOrCreate/query callback 消失，全局 GORM DB/search clone flat 约 `4.20 -> 2.62 MiB (-37.5%)`，总 allocation `-1.7%`；positive-live 反向、post-live `+2.9%`，不作常驻内存声明。DB catch-up/drain `211/401 -> 184/291 ms`、request p95 `69.57 -> 61.48 ms` 是有利单样本方向，不冒充重复 A/B。

CPU `2026-07-27T03-21-49-203Z -> 2026-07-27T03-44-49-149Z` 总样本 `5.72 -> 5.37 CPU s (-6.1%)`，cgo/SQLite bind/exec/commit 分别约 `-21.3%/-43.6%/-35.0%/-43.2%`；GC/scanobject `+10.8%/+4.6%` 反向。CPU 轮 request p95 改善但首显与可见 backlog 反向，最终 drain/正确性仍通过，因此只声明直接 SQL/ORM 栈收益。heap/CPU 均 400/400、400 direct row、0 Query/Gap/缺序/重复/乱序/unavailable、最终 backlog 0、CPU 恢复。本轮没有修改或发布 GORM，也没有前端产品、协议、proto、schema、连接配置或 SQLite driver 变更；下一轮继续按新 profile 选择剩余 create callback、quote/bind 或连接 caller。

## 24. 第五十六轮：bare-flow SQLite direct upsert 与正式 3+3（2026-07-27）

第五十五轮虽已去掉 SELECT，bare KV 仍为每条流量进入 GORM Create 的反射、callback、scope clone 与事务包装。候选只在固定项目 schema 的 SQLite bare request/response 分支，通过 fork 已有的 transaction-aware `CommonDB()` 执行同一条参数化 upsert；通用 KV、非 SQLite、Quote/Unquote、冲突更新字段与读取格式均不变。默认字段、软删除/过期/环境字段保留、重复 key、可读值和外层事务 rollback 均有自动化 oracle，没有 schema 或迁移。

五次同外层事务微基准中位为 `30.044 -> 13.228 us/op (-56.0%)`、`12,886 -> 2,567 B/op (-80.1%)`、`188 -> 31 allocs (-83.5%)`。focused race、完整 yakit persistence 和 62 个 MITMV2 MUSTPASS 通过，峰值分别 `941,792/886,252/3,293,816 KiB`，均无 swap。

heap `2026-07-27T03-40-23-266Z -> 2026-07-27T04-04-44-329Z` 中，bare caller `-40.9%`、GORM DB/search clone flat `-20.0%`、总 allocation `-1.5%`，OnConflict clause 构造栈消失；positive-live 与 post-live 有利但仍只作 forced-GC 诊断。CPU `2026-07-27T03-44-49-149Z -> 2026-07-27T04-09-23-245Z` 总样本 `-7.1%`、memory/GC flat `-7.3%`，但 scanobject 与 SQLite bind/exec/commit 反向，因此不把单次 profile 描述成全局数据库 CPU 改善。

正式无 profile 控制/候选为 `body-2026-07-27T04-12-27-919Z -> body-2026-07-27T04-19-55-374Z`，各严格串行 3 轮、每轮 400 条，比较文件 `comparison-vs-gorm-bare-upsert.{json,md}`。比较器 passed，配置/诊断/正确性差异为 0；候选最大可见 backlog `20 -> 16 (-20.0%)`、DB drain `366 -> 359 ms`、request -> React p95 `508 -> 498 ms`、Yak CPU p50 `-3.3%`，但首显 `42 -> 47 ms`、Yak CPU p95 `+11.4%`、RSS `+2.0%` 反向。六轮共 2400/2400，精确 wire/detail、400 direct row/轮、0 Query/fallback/Gap/缺序/重复/乱序/unavailable、最终 backlog 0、CPU 恢复和清理全部通过。

本轮保留确定性的 direct caller/分配收益，并明确拒绝 UI 全面提速或常驻内存声明；前端产品、通信协议、proto、schema、连接配置、GORM 与 SQLite driver 均未修改。下一轮继续由扩大后的真实 profile 决定后端 create/quote/bind 或生命周期目标，批量化必须先解决 ID 回填、广播顺序与锁时长约束。

## 25. 第五十七轮：TrafficGuard 原位 ASCII fold（2026-07-27）

第五十六轮 heap 将 `4,026,125 B` 直接归因到 CGO prefilter 扫描前的整份正文小写副本。候选不改原始 packet，而是在 C Teddy/AC 内核读取字节时仅把 ASCII `A-Z` 映射为小写；SSSE3、NEON、标量、双字节 prefix、精确 confirm、AC 根跳过和状态转移使用同一规则，标点与高位字节不变，纯 Go non-CGO 路径不变。混合大小写/标点/高字节和 4,800 组随机语料的 SIMD、标量、C-AC、Go-AC 差分全部一致。

五次 256 KiB 内核微基准中，warm scratch 中位 `189.992 -> 55.578 us/op (-70.7%)`；cold scratch `265.855 -> 108.444 us/op (-59.2%)`、`532,488 -> 270,339 B/op (-49.2%)`、`2 -> 1 alloc`。59 条真实 TrafficGuard 规则的自然正文中位 `1.606 -> 0.762 ms/op (-52.5%)`，低熵正文 `0.575 -> 0.471 ms/op (-18.1%)`。完整 minirehs/TrafficGuard、focused race 和 62 个 MITMV2 MUSTPASS 通过，峰值分别 `818,560/904,536/3,236,336 KiB`，均无 swap。

同配置 heap `2026-07-27T04-04-44-329Z -> 2026-07-27T04-48-04-936Z` 中，`asciiLowerInto 4,026,125 -> 0 B`，`scanHitsImpl` cumulative `-66.7%`、`MatchedIndexes -61.3%`；总 allocation `+0.3%`、positive-live `+3.9%`、post-live `+5.5%`，因此只声明目标栈收益。CPU `2026-07-27T04-09-23-245Z -> 2026-07-27T04-52-47-809Z` 中 TrafficGuard cumulative `340 -> 210 ms (-38.2%)`，minirehs CGO scan 从 `250 ms` 降到约 `28 ms` 报告阈值以下；总 CPU `+10.6%`、GC/scanobject 反向，不作全局 CPU 声明。

正式无 profile 比较为 `body-2026-07-27T04-19-55-374Z -> body-2026-07-27T04-55-17-328Z`，各 3 轮 400 条，比较文件 `comparison-vs-phase56-ascii-fold.{json,md}` passed。DB catch-up/drain、Renderer drain、Yak CPU p95、RSS 中位分别改善 `29.3%/21.4%/18.1%/7.1%/1.1%`，request -> React `+1.2%` 基本持平；最大 visible backlog `16 -> 22`、Long Task `0 -> 141 ms`、Electron/Yak drain CPU 反向，作为后续前端消费/到达突发风险保留。六轮共 2400/2400，精确 wire/detail、400 direct row/轮、0 Query/fallback/Gap/缺序/重复/乱序/unavailable、最终 backlog 0、CPU 恢复和清理全部通过。

本轮没有前端产品、协议、proto、schema、数据库设置、GORM 或 SQLite driver 改动。下一轮从最新 heap/CPU 继续选择后端 `bytes.growSlice`、SQLite TEXT bind 和普通 flow Create 的安全 caller；若优化后到达更突发持续触发 Renderer Long Task，则以 trace 和严格 A/B 为依据调整前端批次调度，而不是降低正确性或积压门禁。

## 26. 第五十八轮：fixed response 跨消费阶段复用（2026-07-27）

第五十七轮 heap 将最大的 `bytes.growSlice` 拆成两份近似 100 MiB decoded output：lowhttp 已为响应对象生成一次 fixed/display packet，但 MITMV2 劫持处理器仍无条件从 wire 再解压到 plain cache；将其惰性化后，建流持久化又在取走 fixed packet 后因 plain cache 为空而重复解压。候选现在只在响应插件真正读取响应 closure 时生成独立 plain packet；无异步 mirror hook 的同步路径只读借用 fixed packet，持久化取走其独占所有权后直接同时用作 plain input 和 fixed provenance，不写入共享 mutable cache。修改响应、异步 mirror hook 与插件劫持仍使用独立 packet，原 ownership/竞态边界不变。

本轮同时修复 extended response hook 将 replacement 与 request 比较的既有错误，改为与原 response 比较。惰性求值、fixed 借用、async hook 独立副本、modified response、hot-patch、手动响应劫持、auto-unzip 和建流 provenance 测试通过；focused race 通过。完整 yakit persistence `57.079 s / 887,584 KiB`，62 个 MITMV2 MUSTPASS `198.679 s / 3,318,388 KiB`，均 0 swap。

同配置 400 条 gzip heap `2026-07-27T04-48-04-936Z -> 2026-07-27T05-44-11-701Z` 中，总 allocation `523,576,173 -> 432,425,521 B (-17.4%)`，`bytes.growSlice -43.7%`，约 `97.91 MiB` 的第二次 `decodeAndCachePlainResponseBytes` 分支完全消失；剩余约 `111.99 MiB` 是生成唯一 fixed/display packet 的必要解压。post-live `-11.2%`，positive-live 反向，仍按 forced-GC 风险记录。

CPU `2026-07-27T04-52-47-809Z -> 2026-07-27T05-49-11-900Z` 中，总采样 `5.52 -> 4.85 CPU s (-12.1%)`、memory/GC flat `-29.0%`、scanobject `-26.5%`、重复解压 `390 ms -> 0`、响应处理累计 `1.01 -> 0.83 s (-17.8%)`；TrafficGuard/cgo 子树反向，不隐藏短 profile 方差。

正式无 profile 比较为 `body-2026-07-27T04-55-17-328Z -> body-2026-07-27T05-51-28-746Z`，各 3 轮 400 条，比较文件 `comparison-vs-phase57-response-reuse.{json,md}` passed，配置/诊断/正确性差异为 0。DB catch-up/drain、Renderer drain、request -> React、Yak CPU p50、Yak drain CPU p95 中位分别改善 `13.6%/11.0%/10.6%/3.0%/11.5%/34.2%`；最大 visible backlog `22 -> 43`、停止时 visible backlog `2 -> 43`、request p95 `+9.7%`、Yak CPU p95 `+6.1%` 反向。六轮共 2400/2400，精确 wire/detail、400 direct row/轮、0 Query/fallback/Gap/顺序错误、最终 backlog 0、CPU 恢复和清理全部通过。

本轮保留可直接归因的单次解压与全窗口 allocation/CPU 收益，但不宣称所有 UI 瞬时指标改善。前端产品、通信协议、proto、schema、数据库设置、GORM 和 SQLite driver 均未修改。下一轮优先分析剩余约 107 MiB quote 与 115 MiB SQLite TEXT bind；涉及存储格式、TEXT affinity 或 driver API 的方案必须先建立兼容 oracle，不能用破坏性迁移换性能。

## 27. 第五十九轮：Discord token 候选向量门禁（2026-07-27）

第五十八轮 CPU profile 中，TrafficGuard 的 Discord token 固定形态门禁单独占 `110 ms`。旧实现为寻找 `[MN][A-Za-z0-9_-]{23}.[A-Za-z0-9_-]{6}.[A-Za-z0-9_-]{27}`，会对每个 256 KiB response 逐 byte 检查是否为 `M/N`。候选先用标准库 `bytes.IndexByte` 的向量化实现确认必需点号，再分别定位 `M/N` 候选，只对命中的少量位置执行原字符形态校验；若任意连续 8 个同前缀挤在 256 B 内，则回退原线性算法，避免前缀和点号同时密集的对抗输入放大索引调用。规则、PCRE2 extractor、偏移、大小写、字符集和命中结果均不变。

原实现作为独立 oracle 与候选完成 10,000 组固定种子随机 byte corpus 差分，并覆盖有效/无效 token、有效 token 位于 256 KiB 尾部、全部精确规则候选和高风险扫描。完整 TrafficGuard `0.301 s / 821,712 KiB`、focused race `1.742 s / 899,048 KiB`、62 个 MITMV2 MUSTPASS `190.786 s / 3,297,760 KiB` 全部通过，均 0 swap。

五次 256 KiB 微基准中位数：低熵 JSON `123.626 -> 2.381 us/op (-98.1%)`，自然 JSON `124.931 -> 4.817 us/op (-96.1%)`，点号密集输入 `125.895 -> 5.234 us/op (-95.8%)`，无点号的 `MN` 密集输入 `3.691 ms -> 2.512 us/op (-99.9%)`，有效 token 位于尾部 `125.535 -> 4.777 us/op (-96.2%)`。`MN.` 同时密集的回退样本为 `372.105 -> 372.202 us/op (+0.03%)`，视为持平；所有分支都是 `0 B/op / 0 allocs`，因此本轮不重复跑没有对应 allocation 假设的 heap profile。

同配置 400 条 gzip CPU 报告 `2026-07-27T05-49-11-900Z -> 2026-07-27T06-25-10-732Z`，候选矩阵 `body-2026-07-27T06-25-10-068Z`。`hasDiscordTokenCandidate 110 ms` 从叶子热点消失，整个 TrafficGuard cumulative `330 -> 230 ms (-30.3%)`，总采样 `4.85 -> 4.55 CPU s (-6.2%)`、平均 CPU `97% -> 91%`；新的 `IndexByte` 栈采到 `30 ms`。memory/GC flat `1.49 -> 1.67 s (+12.1%)`、scanobject `1.11 -> 1.27 s (+14.4%)` 反向，因此只声明目标门禁和该次全窗口方向。该轮 400/400、精确 318 B gzip wire/262,144 B decoded detail、400 direct row、0 Query/fallback/Gap/顺序错误、最终 backlog 0、CPU 恢复和清理通过；整轮峰值 `3,865,664 KiB`、0 swap。

正式无 profile 比较为第五十八轮 `body-2026-07-27T05-51-28-746Z` 与候选 `body-2026-07-27T06-28-29-151Z`，各严格串行 3 轮，比较文件 `comparison-vs-phase58-discord-gate.{json,md}` passed，配置/诊断/正确性差异为空。Yak CPU p95 `168.252% -> 149.159% (-11.3%)`、Electron CPU p95 `6.792% -> 4.953% (-27.1%)`、最大 visible backlog `43 -> 24 (-44.2%)`、停止时 visible backlog `43 -> 1 (-97.7%)`、request p95 `54.340 -> 51.497 ms (-5.2%)`、Yak RSS `591.941 -> 589.219 MiB (-0.5%)`。

同一比较中 DB catch-up/drain `152/251 -> 230/341 ms`、Renderer drain `288 -> 376 ms`、duplex delivery p95 `33 -> 67 ms`、Yak CPU p50 `+3.7%`、request -> React p95 `489 -> 504 ms` 和最大 persistence backlog `1 -> 4` 反向；Long Task 保持 0。六轮共 2400/2400，所有最终一致性、恢复与清理门禁通过。本轮不修改前端产品、通信协议、proto、schema、数据库表示、GORM 或 SQLite driver。下一轮继续量化 quote/SQLite TEXT bind 随 Body 大小的斜率；任何改变历史存储格式或 driver 生命周期的方案必须作为显式兼容阶段，而不是无提示落地。

## 28. 第六十轮：SQLite 大文本绑定去拷贝（2026-07-27）

第五十九轮 400 条 gzip heap 将 `115,122,656 B` 直接归因到 SQLite driver 的 string bind：当前 driver 会先执行 `[]byte(v)`，因此每条约 256 KiB 的 response 在进入 SQLite 前又产生一份 Go heap 副本。GORM fork `v1.9.2-yaklang.3` 新增 `CreateWithColumnExpressions`，允许 Create callback 在不绕过 hook、默认值、ID 回填和时间戳的前提下，仅替换指定列的绑定表达式。后端只在 SQLite 且 Request/Response 单字段至少 64 KiB 时，通过 `CAST(? AS TEXT)` 绑定只读 byte view；小字段和非 SQLite 仍走原 `Create`。SQLite `typeof == text`、`LIKE`、逐字节读回、ID/hash/after-save 语义均有 oracle，没有 schema、历史数据、proto 或通信协议变化。

五次同进程微基准中，大字段（64 KiB request + 256 KiB response）每次分配约 `358 -> 32 KiB (-91%)`，耗时约 `3.74 -> 3.58 ms/op`，但墙钟差异接近噪声；16 KiB 中等字段使用候选表达式时曾有约 5% 反向，因此最终 64 KiB 自适应门槛保留旧小字段路径。focused 功能与 race、GORM 全套测试以及 62 个 MITMV2 MUSTPASS 均通过；本轮长门禁执行 `210.806 s`，所有 Go 冷编译/测试均使用一次性隔离缓存。

同配置 heap `2026-07-27T05-44-11-701Z -> 2026-07-27T07-46-31-959Z` 中，总 allocation `432,425,521 -> 303,064,972 B (-29.9%)`，SQLite bind `115,122,656 -> 1,574,464 B (-98.6%)`，database-persistence 类别 `118,796,482 -> 4,724,354 B (-96.0%)`；`quoteHTTPPacket` 与 `bytes.growSlice` 仍分别约 101/107 MiB，成为下一阶段候选。CPU `2026-07-27T06-25-10-732Z -> 2026-07-27T07-39-15-157Z` 总样本 `4.55 -> 4.31 CPU s (-5.3%)`，SQLite bind `430 -> 30 ms (-93.0%)`、`runtime.stringtoslicebyte 430 ms -> 0`、InsertHTTPFlow cumulative `770 -> 400 ms (-48.1%)`、scanobject flat `1.27 -> 0.97 s (-23.6%)`；quote/gzip/growSlice 反向，仍按短 profile 方差公开。

正式无 profile 比较为第五十九轮 `body-2026-07-27T06-28-29-151Z` 与候选 `body-2026-07-27T07-50-48-295Z`，各严格串行 3 轮，比较文件 `comparison-vs-phase59-sqlite-text-bind.{json,md}` passed，配置、诊断和正确性差异为空。DB catch-up/drain `230/341 -> 175/280 ms`、persist write p95 `10 -> 4 ms`、Renderer drain `376 -> 314 ms`、最大 persistence/visible backlog `4/24 -> 3/22`；吞吐和 request/response -> React 基本持平。反向项为 Yak CPU p95 `149.159% -> 178.735%`、request p95 `51.497 -> 60.855 ms`、首次可见 `45 -> 54 ms`、Electron CPU p95 `4.953% -> 6.206%`，Yak RSS `+1.2%`。六轮共 2400/2400，精确 318 B wire/262,144 B detail、0 Query/fallback/Gap/顺序错误、最终 backlog 0、CPU 恢复和清理全部通过。

本轮只声明大文本 bind 的确定性分配和直接数据库栈收益，不声称所有 CPU/UI 指标改善。下一轮优先从剩余 `quoteHTTPPacket` 与 decoded-output `bytes.growSlice` 建立 ownership/capacity 假设；扩大到 1000 条以上前先保持串行、磁盘余量门禁和隔离缓存，避免资源事故污染性能结论。

## 29. 第六十一轮：GORM Scope.Fields 连续存储（2026-07-27）

第六十轮剩余最大的 `bytes.growSlice` 约 107 MiB 是生成唯一 decoded/fixed packet 的输出，`quoteHTTPPacket` 约 101 MiB 是历史 SQLite TEXT 表示需要的最终 quoted packet；两者都不是可直接删除的重复副本。下一个可安全归因的热点是 GORM `Scope.Fields`：旧实现为每条记录的每个字段单独 `new(Field)`，在第六十轮 heap 中累计约 6.29 MiB。

GORM fork `v1.9.2-yaklang.4` 将同一 Scope 的 Field 元数据放入一块连续 `[]Field`，公开的 `[]*Field`、指针稳定性、`Set`、reflect value、blank 判定和嵌入指针初始化语义保持不变。五次微基准中位约为 `6.15 -> 5.29 us/op (-14%)`、`2,928 -> 2,000 B/op (-31.7%)`、`45 -> 7 allocs/op (-84.4%)`。GORM 全套与 focused race 通过；真实 HTTPFlow Create 基准中，小字段约 `435 -> 380 allocs/op`，大字段自适应路径约 `456 -> 402 allocs/op`，墙钟没有确定性回退。最终以 `.4` 重跑 62 个 MITMV2 MUSTPASS，通过耗时 `211.984 s`。

同配置 heap 为第六十轮 `2026-07-27T07-46-31-959Z` 与第六十一轮 `2026-07-27T08-32-52-494Z`。总 sampled allocation `303,064,972 -> 289,174,010 B (-4.58%)`，`Scope.Fields 6,292,552 -> 1,050,624 B (-83.3%)`；`bytes.growSlice` 与 quote 分别约 `107.83/104.01 MiB`，仍是必要大对象和下一阶段 ownership/capacity 审计对象。positive-live `13.18 -> 28.23 MiB` 反向，post-live `307.88 -> 269.25 MiB` 有利，两者继续只作 forced-GC 诊断，不声明常驻内存下降。

正式无 profile 比较为第六十轮 `body-2026-07-27T07-50-48-295Z` 与候选 `body-2026-07-27T08-40-24-891Z`，各严格串行 3 轮，比较文件 `comparison-vs-phase60-gorm-scope-fields.{json,md}` passed，配置、诊断和正确性差异为空。DB catch-up `175 -> 167 ms`、duplex p95 `38 -> 25 ms`、首次可见 `54 -> 45 ms`、最大 visible backlog `22 -> 14`、request p95 `60.855 -> 46.650 ms`、Yak CPU p50/p95 `118.218/178.735% -> 108.982/158.853%` 为有利方向；DB drain `280 -> 315 ms`、Renderer drain `314 -> 369 ms`、Yak drain CPU p95 `59.336 -> 139.035%` 为反向项，吞吐基本持平。六轮全部完成 2400/2400、精确 wire/detail、0 Query/fallback/Gap/顺序错误、最终 backlog 0、CPU 恢复和清理。

本轮发布并推送的唯一仓库是经授权的 GORM fork：commit `3b16dee`、tag `v1.9.2-yaklang.4`；yaklang 只更新依赖并保留未提交工作区，前端产品、协议、proto 与 schema 未修改。中断恢复时观察到旧隔离缓存和全局 Go cache 各约 3.2 GiB，均先清理；冷门禁的临时 build/tmp 峰值约 `3.2/3.4 GiB`，退出后四个相关目录全部不存在，全局 cache 为 `768 KiB`。这些是本轮清理结果，不能用于淡化用户已确认的历史 `290G` 事故。下一轮继续从最新 profile 选择可证明重复的 caller；必要 decoded output 与历史 quote 在没有新 ownership/表示契约前不做破坏性改写。

## 30. 第六十二轮：GORM Create 绑定状态缓存与容量预分配（2026-07-27）

第六十一轮 heap 中，普通 HTTPFlow Create 仍有 `Scope.InstanceGet 1,572,936 B`、`Scope.AddToVars 1,574,144 B`、`createCallback 9,442,361 B cumulative`。根因是同一个 Scope 每绑定一列都重新拼接实例 key、查询只在子查询中存在的 `skip_bindvar`；同时 Create 的 columns/placeholders 从 nil slice 多次扩容。候选在 Scope 首次查询后缓存 `skip_bindvar` 的“是否存在”语义，`InstanceSet("skip_bindvar", value)` 会立即更新缓存；即使 value 为 false，历史逻辑也按 key 存在而跳过 dialect bind，兼容测试显式锁定了“先绑定、后设置”和“预先设置”两种顺序。Create callback 只读取一次 Fields，并按字段数预分配两个局部字符串 slice；公开 API、SQL、hook、默认值、ID、schema 与 dialect 行为不变。

64 次连续绑定的五次微基准中位约为 `6.24 -> 2.24 us/op (-64.1%)`、`7,602 -> 4,073 B/op (-46.4%)`、`143 -> 17 allocs/op (-88.1%)`。使用发布版 `.4` 与本地候选、同缓存严格顺序跑真实 HTTPFlow Create 五次 A/B：small adaptive 为 `2.820 -> 2.678 ms/op (-5.0%)`、`33,134 -> 28,134 B/op (-15.1%)`、`380 -> 286 allocs/op (-24.7%)`；medium adaptive 为 `2.976 -> 2.732 ms/op (-8.2%)`、`60,695 -> 55,548 B/op (-8.5%)`、`380 -> 286 allocs/op`；large adaptive 为 `3.603 -> 3.405 ms/op (-5.5%)`、`29,653 -> 24,500 B/op (-17.4%)`、`402 -> 304 allocs/op (-24.4%)`。

GORM 全套与 focused race 通过；yaklang focused 功能/race 通过，使用本地候选的 62 个 MITMV2 MUSTPASS 通过，执行 `193.624 s`。相对第六十一轮的 `211.984 s` 为有利跨次方向，但不是严格同代墙钟 A/B，不单独宣称 `8.7%` 端到端提速。GORM fork 经授权发布 commit `7eadd03`、tag `v1.9.2-yaklang.5`，yaklang `go.mod/go.sum` 升级到 `.5`，未提交或推送 yaklang/yakit。

同配置 heap 为 `2026-07-27T08-32-52-494Z -> 2026-07-27T14-07-30-521Z`。目标 `Scope.InstanceGet 1,572,936 B -> 未采样到`、`AddToVars 1,574,144 -> 524,864 B (-66.7%)`、`createCallback cumulative 9,442,361 -> 7,346,650 B (-22.2%)`、整个 `DB.Create cumulative 9,967,673 -> 8,920,722 B (-10.5%)`。但总 sampled allocation `289,174,010 -> 324,228,327 B (+12.1%)`，主要伴随必要 decoded growSlice `107,825,500 -> 127,956,534 B (+18.7%)` 与 quote `104,008,231 -> 110,047,419 B (+5.8%)` 的大对象采样反向；database-persistence flat 基本不变。positive-live `-25.1%` 而 post-live `+3.1%`，同样不支持常驻内存下降声明。

正式无 profile 比较为第六十一轮 `body-2026-07-27T08-40-24-891Z` 与候选 `body-2026-07-27T14-14-27-605Z`，各 3 轮 400 条，比较文件 `comparison-vs-phase61-gorm-create-binding.{json,md}` passed，配置/诊断差异为空。DB catch-up/drain `167/315 -> 152/257 ms`、Renderer drain `369 -> 295 ms`、首次可见 `45 -> 43 ms`、request -> React `500 -> 492 ms`、Yak drain CPU p95 `139.035% -> 88.543%`、Yak RSS `600.996 -> 590.980 MiB` 为有利方向；最大 visible/shadow backlog `14 -> 48`、停止时 visible backlog `0 -> 48`、Electron CPU p95 `6.176% -> 7.432%`、duplex p95 `25 -> 27 ms`、request p95 `46.650 -> 48.443 ms`、Long Task `50 -> 53 ms` 反向。吞吐与 Yak CPU p95 基本持平。

六轮共 2400/2400，精确 wire/detail、400 direct row/轮、0 Query/fallback/Gap/缺序/重复/乱序/unavailable、最终 backlog 0、CPU 恢复和清理全部通过。本轮前端产品、协议、proto 和 schema 未修改。heap 与正式矩阵退出后 E2E build/tmp 均不存在，Yak 二进制缓存保持 6 份/约 1.4 GiB，全局 Go cache 为 `768 KiB`；这仍只是新流程有效，不能否认历史 `290G` 事故。下一轮优先评估 `Scope.scan` 的通用 query 分配或标准 CRLF 修复 caller，并继续把更快后端导致的到达突发作为独立前端风险观察，不以降低正确性换平滑。

## 31. 第六十三轮：GORM Query 扫描计划复用（2026-07-27）

第六十二轮 heap 中，GORM `Scope.scan` 为每一行重建 column-name map、selected-column map 和重置 map，400 行查询累计采样约 `3,148,496 B`。GORM fork 候选在拿到 `rows.Columns()` 后只构建一次 column -> Field 下标计划，再由每行扫描复用；每行仅保留 SQL Scan 必需的目标 slice 和本行实际命中 Field 的重置列表。重复列按历史顺序消费，NULL、非指针、指针、`sql.Scanner`、嵌入字段和 preload join 语义均由独立 oracle 锁定，公开 API、SQL、模型、schema 与协议不变。

400 行纯元数据微基准五次中位约为 `2.806 ms -> 49.376 us`、`2,246,449 -> 118,698 B/op`、`6,001 -> 408 allocs/op`；它只表示扫描元数据上界，不冒充完整数据库查询。使用已发布 `.5` 与本地候选、同一 SQLite 数据和五组各 `10x` 的真实 `QueryHTTPFlow` A/B，中位约为 `12.319 -> 9.423 ms/op (-23.5%)`、`4,755,052 -> 2,755,321 B/op (-42.1%)`、`53,932 -> 48,739 allocs/op (-9.6%)`。GORM 全套与 focused race、yaklang `common/yakgrpc/yakit` 全包、focused query race 和 62 个 MITMV2 MUSTPASS 全部通过；长门禁耗时 `200.365 s`。经授权发布的唯一仓库为 GORM fork：commit `d06871f`、tag `v1.9.2-yaklang.6`，yaklang 仅升级依赖并保留未提交工作区。

同配置诊断 heap 为 `2026-07-27T14-07-30-521Z -> 2026-07-27T14-56-48-088Z`。目标 `Scope.scan 3,148,496 B -> 未采样到`，`QueryHTTPFlow/SelectHTTPFlowFromDB cumulative 5,248,336 -> 3,148,885 B (-40.0%)`，服务端 `QueryHTTPFlows cumulative 7,346,028 -> 3,673,185 B (-50.0%)`，总 sampled allocation `324,228,327 -> 281,511,879 B (-13.2%)`。这是带 forced-GC 的 caller 诊断证据；它不单独证明常驻内存或 UI 延迟改善。

正式无 profile 比较为第六十二轮 `body-2026-07-27T14-14-27-605Z` 与候选 `body-2026-07-27T15-03-54-384Z`，各严格串行 3 轮，比较文件 `comparison-vs-phase62-gorm-scan-plan.{json,md}` passed，配置、历史诊断覆盖和实验差异为空。六轮完成 2400/2400；候选每轮 400 direct row、0 Query/fallback/Gap/缺序/重复/乱序/unavailable，最终 backlog 0、CPU 恢复和清理通过。最大 visible/shadow backlog `48 -> 21 (-56.3%)`、停止时 visible backlog `48 -> 0`、Yak drain CPU p95 `88.543% -> 79.370% (-10.4%)`、Yak RSS `590.980 -> 583.617 MiB (-1.2%)` 为有利方向。

反向中位包括 DB catch-up/drain `152/257 -> 169/275 ms`、Renderer drain `295 -> 313 ms`、首次可见 `43 -> 49 ms`、duplex p95 `27 -> 63 ms`、request -> React `492 -> 497 ms`、Yak CPU p50 `+6.3%` 和 Long Task `53 -> 155 ms`；request p95、Yak/Electron CPU p95 则基本持平或小幅有利。该固定速率场景全程命中 direct stream、`queryCount == 0`，因此自动化不会把查询回退路径的确定性收益包装成所有实时 UI 指标改善。

本轮未修改前端产品、通信协议、proto、schema 或 SQLite driver。heap 冷构建隔离 build/tmp 峰值约 `3.3/3.5 GiB`，正式冷构建约 `2.3/2.3 GiB`；两轮退出后均删除，Yak 二进制缓存保持 6 份/约 1.4 GiB，全局 Go cache 约 26 MiB。该状态只验证新资源流程，仍保留用户确认的历史 `/home/go0p/.cache/go-build = 290G` 事故。下一轮优先使用 query/fallback 专用端到端场景复验扫描计划；若 Renderer 长任务或 direct burst 在夹心 A/B 中复现，再独立进入前端调度 phase。

## 32. 第六十四轮：Query Shadow 专用 3+3（2026-07-27）

为避免第六十三轮 `queryCount == 0` 的 direct 场景错误评价 Query 优化，本轮在完全相同的 400 请求、12 并发、100 req/s、gzip 256 KiB 场景中强制 `httpflow-live-stream-mode=shadow`。基线仅把 yaklang 的 GORM 依赖临时切到已发布 `.5`，候选恢复 `.6`；两边各严格串行 3 次，测试后工作区版本断言恢复 `.6`。前端 runbook 固化完整 matrix 命令，不修改会导致 Renderer 构建指纹失效的根 `package.json`，也不改变产品默认模式。

正式矩阵为 `.5` 的 `body-2026-07-27T15-18-52-745Z` 与 `.6` 的 `body-2026-07-27T15-29-06-961Z`，比较文件 `comparison-vs-gorm5-scan-plan-shadow.{json,md}` passed，配置、历史诊断覆盖和实验差异为空。六轮完成 2400/2400；每轮 `queryCount == 6`、400 shadow Query match、0 direct row、0 row without event、最终 backlog 0、CPU 恢复和清理通过。

目标后端中位得到端到端 Query-path 复验：DataQuery p95 `37.644 -> 17.106 ms (-54.6%)`，完整 backend Query p95 `37.923 -> 17.148 ms (-54.8%)`，query round-trip p95 `55.4 -> 38.5 ms (-30.5%)`；COUNT p95 `0.791 -> 0.792 ms` 持平，执行比例均为 `1/6`。Conversion p95 `0.759 -> 0.943 ms` 与 per-flow 样本反向，但绝对值很小且三轮范围重叠较大；它不是 scan-plan 的目标 caller。

UI/整机仍为混合：request/response -> React p95 `1001/983 -> 968/965 ms` 小幅有利，Yak drain CPU p95 `-44.8%`、Electron RSS `-1.9%`；首次可见 `106 -> 185 ms`、最大 visible backlog `24 -> 95`、停止时 visible backlog `0 -> 87`、Long Task `179 -> 375 ms`、Yak RSS `584.9 -> 620.5 MiB` 和 Yak CPU p95 `+6.5%` 反向。DB drain 与 Renderer drain 基本持平，吞吐保持约 100 req/s。

因此 `.6` 接受为 Query SQL/扫描路径的确定性性能提升，但本轮同时把“约 1 秒轮询触发间隔仍主导用户体感”和“更快结果批次可能加重 Renderer burst”登记为后续问题，不以降低正确性或回退能力换平滑。两次冷构建隔离 build/tmp 峰值均约 `2.6/2.6 GiB`，退出后删除；Yak 二进制缓存保持 6 份/约 1.4 GiB。下一轮优先通过 Renderer trace 分离 shadow 批次到达、React commit 与虚拟表更新成本，再决定前端调度是否需要实改。

## 33. 第六十五轮：Renderer 到达节奏归因与高负载直推复验（2026-07-27）

第六十四轮后的 shadow 与纯 Query Renderer trace 都确认：后端 `.6` 已缩短实际 SQL/扫描阶段，但 fallback polling 的约 1 秒触发周期仍决定 shadow 体感；零 Query 的默认 canary trace 则把 direct 路径问题定位为旧 `500 ms` 持续 batch 间隔和批次后的 style/layout。400 条、100 req/s 默认 trace 的 request -> React p95 约 `496 ms`、最大/停止时 visible backlog 均为 `46`，9 个 batch 约 50 行/批，主线程 Long Task 为 `51–62 ms` 的样式与布局。

前端因此把 direct 最小/持续间隔改为 `100/100 ms`，MITM 虚拟表 overscan `5 -> 2`，并将全部调度参数写入 E2E case identity。对旧 `500 ms + overscan 5` 的 3 次证据，最终 `100 ms + overscan 2` 将 request/response -> React p95 约 `497/494 -> 120/110 ms`、最大 visible backlog `21 -> 9`、Long Task `155 -> 0 ms`；Electron CPU 的绝对代价约为 p50/p95 `3.0/7.12 -> 6.82/8.68%`，RSS 约 `+8.6%`，作为明确取舍保留。

直推状态提交再加入快照安全路径：选择合法新行时不构造完整 merged array，React state 未变化时只 prepend/crop；发生并发变化时继续走完整去重。1000+10 行微基准约快 `1.82x`。同配置严格 3+3 `body-2026-07-27T16-44-56-959Z -> body-2026-07-27T16-57-15-146Z` 的比较文件 `comparison-vs-double-direct-merge.{json,md}` passed：Electron CPU p50/p95 `-8.1%/-3.6%`、request -> React `-7.6%`、Renderer drain `-24.7%`，候选三轮 Long Task 为 0；RSS `+1.5%`、visible backlog `10 -> 11` 与高波动 Yak drain CPU 反向，不包装成后端改善。

扩大矩阵 `body-2026-07-27T17-03-45-330Z` 在 3 个隔离环境各跑 1000 条、200 req/s、4 KiB response。每轮均 1000/1000、51 direct batch、batch p95 22 行、0 Query/fallback/Gap/缺序/重复/乱序/unavailable；request -> React p95 `106–107 ms`、最大 visible backlog `17–21`、最终为 0，Electron CPU p95 `8.67–9.01%`。这证明当前 direct 消费能跟上 200 req/s，剩余 `53–104 ms` Long Task 继续归入 Renderer style/layout，而不是重新归因给 SQL 或通信积压。

本轮不修改后端协议、proto、schema、SQLite 参数或 GORM `.6`。自动化新增无副作用 `--help`、无效 trace 组合快速拒绝和 SIGINT/SIGTERM 完整 WDIO 进程树回收；真实中断验证无 Electron/Yak/chromedriver 与临时目录残留。全局 Go cache 收尾约 53 MiB、Yak 二进制缓存 6 份/约 1.3 GiB、磁盘可用约 840 GiB；这些只证明当前资源纪律有效，不覆盖用户确认的历史 290 GiB 事故。

下一阶段回到后端 profile 主线，在当前 100/200 req/s 自描述场景中分别量化 quote、必要 decoded output、SQLite persistence、live publish 与 gRPC 序列化。只有 trace 在相同产品设置下重复确认 layout/row decoration 才继续前端 phase；Phase 4 两阶段展示与 Phase 5 旧链路收敛仍未启动，也不在本轮做破坏性协议更新。

## 34. 第六十六轮：下游连接 bufio 生命周期复用（2026-07-27）

1000 条、200 req/s、4 KiB response 的 CPU 诊断 `2026-07-27T17-17-11-550Z` 显示后端总样本 `4.33 CPU s`，其中 `scanobject` flat/cumulative 为 `720/1840 ms`，`gcDrain` cumulative 为 `1760 ms`；GC/对象扫描比 SQLite exec/bind 更值得优先处理。对应 heap `2026-07-27T17-24-01-284Z` 的流量窗口累计分配约 `178.0 MB`，数据库持久化 leaf 约占 8%，而每个下游连接创建的 4 KiB `bufio.Reader`/`Writer` 是可直接消除的短连接分配。

候选为 MITM 下游连接增加成对缓冲池。缓冲在连接结束、`handleLoop` 完整返回后才归还；归还前将 Reader 重置到 EOF reader、Writer 重置到 `io.Discard`，不保留连接、context 或未读报文。CONNECT/TLS 期间继续对同一对缓冲执行原有 `Reset`；SOCKS5 重建 proxy context 时先释放第一对未使用缓冲；重复释放为 no-op。协议字节、flush 所有权、Session/Context 公开 API、HTTPFlow、proto、schema、SQLite 和 GORM `.6` 均未改变。

同进程微基准五次中，旧逐连接分配约 `1575–1652 ns/op`、`8368 B/op`、`5 allocs/op`；池化复用约 `24.3–24.8 ns/op`、`0 B/op`、`0 allocs/op`。功能测试覆盖跨连接 input/output 隔离、未 flush 数据不泄漏和幂等释放；完整 `common/minimartian` 与定向 race 均通过。候选 heap `2026-07-27T17-34-38-775Z` 中 `CreateProxyHandleContext` cumulative 约 `8.53 -> 1.00 MiB`，池 acquire 仅采样到约 `0.50 MiB` 预热量；但整轮 sampled allocation `178.0 -> 196.8 MB` 受其他 parser/GORM 大样本反向影响，因此只接受目标 caller，不声明全局 heap 下降。

同配置 5 秒 CPU `2026-07-27T17-17-11-550Z -> 2026-07-27T17-51-24-513Z` 的总样本 `4.33 -> 3.81 CPU s (-12.0%)`，`scanobject` flat `720 -> 620 ms (-13.9%)`、cumulative `1840 -> 1680 ms (-8.7%)`；`gcDrain` cumulative `1760 -> 1730 ms`。RSA 启动噪声约只解释其中 `30 ms`，目标方向与微基准、caller heap 一致。

正式无 profile 3+3 为 `body-2026-07-27T17-03-45-330Z -> body-2026-07-27T17-41-25-958Z`，比较文件 `comparison-vs-pre-proxy-buffer-pool.{json,md}` passed，配置/诊断差异为空。六轮全部 1000/1000、约 200.1 req/s、1000 direct row、0 Query/fallback/Gap/缺序/重复/乱序/unavailable，最终 backlog 0、CPU 恢复和清理通过。候选中位 Yak CPU p50 `79.6 -> 59.5% (-25.4%)`、p95 基本持平，Yak RSS `592.5 -> 565.8 MiB (-4.5%)`、request p95 `8.80 -> 6.06 ms (-31.1%)`；DB drain `325 -> 403 ms`、Renderer drain `364 -> 439 ms`、Yak drain CPU p95 `47.8 -> 90.9%` 和最大 visible backlog `17 -> 21` 反向，继续作为固定速率短样本风险，不包装成 UI 全面提速。

全部构建/测试保持串行和有界并发。两份验证用隔离 Go cache（约 `423/877 MiB`）测试后均移入系统回收站；E2E 临时目录与 Electron/Yak/WDIO/chromedriver 无残留，Yak 二进制缓存保持 6 份/约 1.4 GiB，全局 Go cache 约 57 MiB。该状态仍只是用户手工清理后的结果，不否认历史 `/home/go0p/.cache/go-build = 290G` 事故。下一轮继续从 `bytes.growSlice`、`strings.Builder`、`splitHTTPPacketEx` 与 response parser 的真实 caller 中选低风险后端目标；前端只在重复 trace 明确指向 style/layout 时继续修改。

## 35. 第六十七轮：HTTP Header 扫描 ownership 快路径（2026-07-27）

第六十六轮候选 heap 的 `alloc_objects` 重新把目标定位到 response parser。旧 `ScanHTTPHeaderWithHeaderFolding` 中，`ReadLine` 已为每个 Header 返回独立 allocation，但 scanner 仍将普通 Header 再 append-copy 到 `headerRawCache`；折叠 Header 还会先构造一份临时 `CRLF + line`。候选在首行直接接管 `ReadLine` 的 allocation，只有确认存在 continuation line 时才扩容，并将 CRLF 直接追加到目标 buffer。callback 获得的 slice 仍由该次扫描独占，emit 后不复用其 backing array；协议字节、folding/prefix/LF-only/EOF、nil terminal callback 和公开 API 均未改变。

新增 retained-slice ownership 测试，并保留一份旧实现作为 fuzz oracle。10 秒差分 fuzz 执行 `138,815` 次无差异；完整 `common/utils`、完整 `common/utils/lowhttp` 与定向 race 均通过。五次微基准中，普通 Header 约从 `542–563 ns/op / 280 B/op / 11 allocs` 降到 `373–387 ns/op / 144 B/op / 6 allocs`，约改善 `31%` 时间、`48.6%` 字节和 `45.5%` 分配次数；folded Header 从 `468–481 ns/op / 208 B/op / 10 allocs` 降到 `346–363 ns/op / 144 B/op / 6 allocs`。

真实 heap 为 `2026-07-27T17-34-38-775Z -> 2026-07-27T18-16-19-882Z`。目标重复拷贝节点 `ScanHTTPHeaderWithHeaderFolding.func3` 从 `2,097,236 B / 57,344 objects` 降为 0；scanner 累计对象数约 `250,413 -> 232,238 (-7.3%)`，response parser 累计字节约 `20.48 -> 16.80 MB (-18.0%)`。整轮 sampled allocation `196.85 -> 194.45 MB (-1.2%)` 仅作低幅度方向性观察。5 秒 CPU `2026-07-27T17-51-24-513Z -> 2026-07-27T18-23-31-611Z` 的总样本 `3.81 -> 3.66 CPU s (-3.9%)`、平均 CPU `76.2% -> 73.2%`；目标低于稳定 CPU 采样分辨率，因此只用于排除明显回退。

正式无 profile 3+3 为 `body-2026-07-27T17-41-25-958Z -> body-2026-07-27T18-25-48-136Z`，比较文件 `comparison-vs-phase66-header-scan.{json,md}` passed，配置、诊断与历史 metric coverage 差异为空。六轮全部完成 1000/1000；候选每轮数据库总数/唯一 ID 均为 1000，1000 direct row，0 Query/fallback/Gap/缺序/重复/乱序/unavailable，最终 persistence/visible backlog 为 0，CPU 恢复和清理通过。中位 Yak CPU p50 `-7.8%`、Renderer drain `-9.3%`、最大 visible backlog `21 -> 20`、request/response -> React 基本持平或略有利；duplex p95、首显、Electron CPU/RSS 与 Yak RSS 有反向短窗波动，不能归因于这个后端局部变更。

本轮没有修改前端产品逻辑、通信协议、proto、schema、数据库配置、GORM 或 driver。测试隔离 Go cache 峰值约 1.3 GiB 后已永久删除；E2E build/tmp 在退出时不存在，Yak 二进制缓存保持 6 份/约 1.4 GiB，全局 Go cache 约 57 MiB，磁盘可用约 838 GiB。以上仍是用户清理后的当前状态，不覆盖历史 `/home/go0p/.cache/go-build = 290G` 事故。下一轮继续由最新 heap/CPU 选择 `splitHTTPPacketEx`、builder、数据库或 parser 的可归因 caller；前端仅在重复 trace 明确指向 style/layout 时进入独立优化。

## 36. 第六十八轮：ASCII Header 分类去 lowercase 分配（2026-07-27）

第六十七轮 heap 的 `alloc_objects` 显示 `strings.Builder.grow` 仍约占对象数 8.5%，其中 `splitHTTPPacketEx` 与 `fixHTTPPacketCRLF` 为识别 Content-Length、Content-Type 和 Transfer-Encoding，会对每条 Header 的 key/value 构造 lowercase string。候选改为对 ASCII Header 直接执行 allocation-free case fold；任一输入 byte 非 ASCII 时继续回退旧 `strings.ToLower` 路径，保留 Unicode、非法 UTF-8、空 prefix/needle 和异常报文语义。

旧/新分类器同二进制五次配对微基准中位约为 `923 -> 439 ns/op (-52.4%)`、`176 -> 0 B/op`、`12 -> 0 allocs/op`。10 秒差分 fuzz 执行 `99,943` 次无差异；`FixHTTPPacketCRLF`/split parser 聚焦回归、定向 race 和完整 `common/utils/lowhttp`（`180.800 s`）全部通过。

真实 heap 为 `2026-07-27T18-16-19-882Z -> 2026-07-27T18-53-48-984Z`。整轮 sampled allocation `194.45 -> 183.82 MB (-5.5%)`，sampled objects 约 `2.10 -> 1.91 M (-8.9%)`；`strings.Builder.grow` 对象数约 `179,381 -> 88,950 (-50.4%)`，`splitHTTPPacketEx.func1` cumulative objects `178,313 -> 83,558 (-53.1%)`，`fixHTTPPacketCRLF.func3` 分配节点消失，完整 fix/split cumulative objects 分别约 `-45.1%/-38.7%`。对应 5 秒 CPU `2026-07-27T18-23-31-611Z -> 2026-07-27T19-00-25-967Z` 总样本 `3.66 -> 3.68 CPU s (+0.5%)` 基本持平；目标 lowercase/builder/split 节点降到采样阈值以下，但 scanobject 反向，因此不宣称全局 CPU 改善。

正式无 profile 3+3 为 `body-2026-07-27T18-25-48-136Z -> body-2026-07-27T19-02-35-441Z`，比较文件 `comparison-vs-phase67-header-classification.{json,md}` passed，配置、诊断与 metric coverage 差异为空。六轮均完成 1000/1000；候选每轮数据库总数和唯一 ID 均为 1000、1000 direct row、0 Query/fallback/Gap/缺序/重复/乱序/unavailable，最终 backlog 0、CPU 恢复和清理通过。候选中位 DB catch-up `-14.8%`、duplex p95 `-21.7%`、Renderer drain `-3.0%`、Yak RSS `-1.9%`，Long Task `52 -> 0 ms`；request p95 `+21.3%`、Yak CPU p50 `+8.5%`、Yak drain CPU `+3.8%` 反向，作为 WSL 短窗风险保留。

本轮没有前端产品、通信协议、proto、schema、数据库、GORM 或 driver 变化。1.3 GiB 测试隔离缓存已永久删除而非留在回收站；E2E build/tmp 退出后不存在，Yak 缓存仍为 6 份/约 1.4 GiB，全局 Go cache 约 64 MiB、磁盘可用约 839 GiB。它们仍是用户手工清理后的当前值，不能覆盖历史 `/home/go0p/.cache/go-build = 290G` 事故。下一轮重新按最新 heap/CPU 排序剩余 builder、GORM quote、packet split、stream serialization 与数据库 caller。

## 37. 第六十九轮：GORM commonDialect Quote 去 fmt 分配（2026-07-27）

第六十八轮 heap 的对象 profile 将 `github.com/yaklang/gorm.commonDialect.Quote` 定位为剩余数据库小对象热点：约 `65,537` 个 flat、`81,921` 个 cumulative sampled objects。GORM fork 将等价的 `fmt.Sprintf("\"%s\"", key)` 改为字符串拼接，不改变 identifier quoting、dialect API、SQL 文本或参数顺序。新增精确用例、旧实现 fuzz oracle 和配对 benchmark；10 秒 fuzz 执行 `301,458` 次无差异，完整 GORM 测试与定向 race 均通过。常见 `request` identifier 的中位微基准约 `83.9 -> 27.0 ns/op (-67.8%)`、`32 -> 16 B/op`、`2 -> 1 allocs/op`。

真实 `HTTPFlow` Create A/B/A 中，候选 wall time 位于两次 `.6` 基线之间，故不声明耗时改善；分配约 `286 -> 241 allocs/op (-15.7%)`，字节约下降 2%。GORM 变更已按授权提交并推送为 `40342e7`，发布轻量 tag `v1.9.2-yaklang.7`；yaklang 依赖已解析到 `.7`。完整 `common/yakgrpc/yakit` 与 SQLite TEXT 定向 race 通过。

真实 heap 为 `2026-07-27T18-53-48-984Z -> 2026-07-27T19-31-43-403Z`。整轮 sampled allocation `183.82 -> 182.73 MB (-0.59%)`、sampled objects `1.914 -> 1.835 M (-4.12%)`；基线 Quote 节点约 `65,537 flat / 81,921 cumulative objects`，候选降到报告阈值以下。5 秒 CPU `2026-07-27T19-00-25-967Z -> 2026-07-27T19-38-13-641Z` 总样本 `3.68 -> 3.57 s (-3.0%)`、平均 CPU `73.6% -> 71.4%`，但 Create/Quote 低于采样阈值，因此只作为无明显 CPU tradeoff 的方向性证据。

正式无 profile 3+3 为 `body-2026-07-27T19-02-35-441Z -> body-2026-07-27T19-40-24-435Z`，比较文件 `comparison-vs-phase68-gorm-quote.{json,md}` passed，配置、诊断与 metric coverage 差异为空。候选三轮均为 1000/1000、数据库唯一 ID 1000、1000 direct row、0 Query/fallback/Gap/缺序/重复/乱序/unavailable，最终 backlog 0、CPU 恢复且清理成功。duplex p95 `-14.9%`、request p95 `-13.9%`、首显 `-12.5%`、Electron CPU p95 `-6.4%`；DB catch-up `+7.8%`、Renderer drain `+4.1%`、Yak CPU p50 `+5.1%`、Yak RSS `+3.1%` 反向，继续视为 WSL 短窗波动，不归因给局部 GORM 变更。

本轮没有前端产品、通信协议、proto、schema 或数据库配置变化。GORM 与 yaklang 验证缓存峰值约 2 GiB，收尾已永久删除；E2E 临时目录和相关进程不存在，全局 Go cache 约 72 MiB、磁盘可用约 839 GiB。这些仍是用户手工清理后的当前值，不能用于否认历史 `/home/go0p/.cache/go-build = 290G` 事故。下一轮继续按最新 profile 排序 persistence、stream serialization 与 response parser，不根据短窗 UI 指标盲调前端。

## 38. 第七十轮：MITM 静态 glob 规则预编译（2026-07-27）

第六十九轮 heap 显示 `MITMFilter.IsPassed -> YakMatcher -> gobwas/glob.Compile` 为可归因热点：相同默认 hostname/method glob 在每条流量上重复编译，累计约 `150,188 sampled objects`，占整轮对象数 `8.18%`。候选在 `MITMFilter` 更新规则时预编译无编码的静态 glob，并在发布给并发读者后只读复用；非法规则、encoded group 和运行后新增 pattern 继续走原 compile-on-execute 路径，因此没有改变过滤结果或错误行为。

同二进制五次配对 benchmark 中位约从 `6159 -> 2448 ns/op (-60.3%)`、`4000 -> 600 B/op (-85.0%)`、`123 -> 35 allocs/op (-71.5%)`。差分 fuzz 执行 `9,026` 次无差异；完整 `common/yak/httptpl`、MITM filter manager 回归与预编译 matcher 并发 race 均通过。

真实 heap 为 `2026-07-27T19-31-43-403Z -> 2026-07-27T20-25-50-802Z`。`gobwas/glob` 累计 sampled objects `150,188 -> 8,192 (-94.5%)`；候选剩余采样全部来自响应 MIME 检查，request 的静态 hostname/method filter 编译链路降到报告阈值以下。整轮 sampled objects `1.835 -> 1.920 M (+4.6%)`、sampled allocation `174.27 -> 177.56 MB (+1.9%)` 受 parser/GORM 等 caller 采样反向，因此只接受目标 caller，不声明全局 heap 下降。

5 秒 CPU 为 `2026-07-27T19-38-13-641Z -> 2026-07-27T20-32-36-941Z`。目标 request filter 路径累计样本约 `60 -> 20 ms (-66.7%)`，但整轮总样本 `3.57 -> 4.03 CPU s (+12.9%)` 反向；该轮只证明目标方向，不作全局 CPU 改善宣称。

正式无 profile 3+3 为 `body-2026-07-27T19-40-24-435Z -> body-2026-07-27T20-34-45-131Z`，比较文件 `comparison-vs-phase69-static-glob.{json,md}` passed，配置、诊断与 metric coverage 差异为空。候选三轮均 1000/1000、数据库唯一 ID 1000、1000 direct row、0 Query/fallback/Gap/缺序/重复/乱序/unavailable，最终 backlog 0、CPU 恢复且清理成功。duplex p95 `-15.0%`、Yak CPU p50 `-12.9%`、首显 `-12.2%`、Electron RSS `-3.2%`；DB catch-up `+22.3%`、Electron CPU p95 `+10.5%`、request p95 `+11.9%`、Long Task `0 -> 50 ms` 反向，继续保留为 WSL 短窗风险。

本轮没有前端产品、协议、proto、schema、数据库或 GORM 变化。测试/fuzz/race 共用的隔离 Go cache 峰值 7.1 GiB 后已永久删除；E2E build/tmp 和相关进程退出后不存在，Yak 二进制缓存保持上限 6 份/约 1.4 GiB，全局 Go cache 约 73 MiB、磁盘可用约 839 GiB。这些仍是用户清理后的当前值，不能覆盖历史 `/home/go0p/.cache/go-build = 290G` 事故。下一轮继续按最新 heap 的 request/response parser、context metadata、stream serialization 与 persistence caller 排序。

## 39. 第七十一轮：复用 Parser 已拥有的 Header 行（2026-07-27）

第七十轮 heap 将普通请求/响应解析器定位为下一批小对象热点。Header scanner 已在第六十七轮建立“每条逻辑 Header 行由 parser 独占、callback 后不再修改”的 ownership 契约，但 request/response callback 仍把 key 和 value 各复制成新 string，并为分支判断再次 lowercase key。候选现在让 Header map 中的 key/value string 直接借用这条独占 line allocation；常见且已经是规范大小写的 Header name 进一步替换为进程生命周期静态字符串，并缓存其 lowercase 名称。异常大小写、非 ASCII、非法 UTF-8、无冒号和运行时解析语义继续与旧实现一致。

旧实现作为同二进制 oracle 的五次配对 benchmark 中位约为 `98.02 -> 43.36 ns/op (-55.8%)`、`42 -> 4 B/op (-90.5%)`、`3 -> 0 allocs/op`。差分 fuzz 在 2 个 worker 下执行 `74,917` 次无差异。请求和响应生命周期测试会在解析后覆写调用者传入的原始 packet、强制 GC，再验证 Host 与普通 Header；聚焦 parser、完整 `common/utils` 和定向 race 全部通过。

真实 heap 为 `2026-07-27T20-25-50-802Z -> 2026-07-27T20-59-52-643Z`。request parser cumulative sampled objects `234,835 -> 90,120 (-61.6%)`，其 Header callback `97,708 -> 40,513 (-58.5%)`；response parser cumulative objects `332,238 -> 155,295 (-53.3%)`，其 Header callback `68,266 -> 33,142 (-51.5%)`。整轮 sampled objects `1.920 -> 1.394 M (-27.4%)`、sampled allocation `186,181,770 -> 171,375,677 B (-8.0%)`，但 response cumulative bytes 受大对象采样反向，因此本轮以对象计数和配对微基准作为主要因果证据。

5 秒 CPU 为 `2026-07-27T20-32-36-941Z -> 2026-07-27T21-06-48-931Z`。总采样 `4.03 -> 3.55 CPU s (-11.9%)`，response parser cumulative `220 -> 170 ms (-22.7%)`，folding scanner cumulative `180 -> 100 ms (-44.4%)`；request parser 低于稳定报告阈值。该单次 CPU 轮只作为无明显 tradeoff 和目标路径方向，不替代正式重复矩阵。

正式无 profile 3+3 为 `body-2026-07-27T20-34-45-131Z -> body-2026-07-27T21-08-49-299Z`，比较文件 `comparison-vs-phase70-owned-header-strings.{json,md}` passed，配置、诊断和 metric coverage 差异为空。候选三轮均完成 1000/1000、数据库总数和唯一 ID 均为 1000、1000 direct row、0 Query/fallback/Gap/缺序/重复/乱序/unavailable，停止与最终 backlog 为 0、CPU 恢复且清理成功。

候选中位 DB catch-up `-27.1%`、Yak CPU p50 `-9.4%`、Yak RSS `-3.1%`、Electron CPU p95 `-6.5%`，request/response -> React 分别改善 `2.7%/0.9%`；first visible `+23.3%`、request p95 `+11.4%`、duplex p95 `+11.8%`、Renderer drain `+6.8%` 和 Electron drain CPU `+26.9%` 反向，继续作为 WSL 短窗波动公开。本轮保留可直接归因的 parser 对象收益，但不声称所有交互指标改善。

本轮没有前端产品、协议、proto、schema、数据库、GORM 或 driver 变化。验证隔离 Go cache 峰值约 1.2 GiB，已永久删除；E2E build/tmp、测试 home 和相关进程退出后不存在，Yak 缓存维持 6 份/约 1.4 GiB，全局 Go cache 约 88 MiB、磁盘可用约 839 GiB。这些仍是用户手工清理后的当前状态，不能覆盖历史 `/home/go0p/.cache/go-build = 290G` 事故。下一轮继续从最新 heap/CPU 的 packet split、Header map、context 与 persistence caller 中选择可证明重复的工作。

## 40. 第七十二轮：Header value 查询规范化快路径（2026-07-27）

第七十一轮 heap 中，HTTP dump 路径的 `getHeaderValueList` 每次查询 `content-length`、`host` 或 `transfer-encoding`，都会把 lowercase key 重新 canonicalize，并在 Header 只有一个正常值时仍创建结果 slice 和去重 map。候选为已知常见 lowercase key 直接复用静态 canonical key；当 lowercase/canonical 两种存储中只有一侧存在，且最多 8 个值、无空值和重复值时，直接返回原 value slice。混合双 key、空值、重复值、未知或异常大小写和超过 8 个值全部回退原 merge/dedupe 路径，避免恶意大 Header 触发二次复杂度。

旧实现作为同二进制 oracle 的五次配对 benchmark 中位约为 `139.4 -> 54.07 ns/op (-61.2%)`、`40 -> 8 B/op (-80.0%)`、`2 -> 0 allocs/op`。固定语义用例覆盖 canonical/lower/mixed storage、空值、重复值、missing 和大列表回退；差分 fuzz 执行 `55,356` 次无差异。聚焦 dump/parser、完整 `common/utils` 与定向 race 均通过。

真实 heap 为 `2026-07-27T20-59-52-643Z -> 2026-07-27T21-33-20-051Z`。基线 `getHeaderValueList` 约 `32,768 flat / 54,613 cumulative sampled objects`、约 `0.5/1.0 MiB sampled allocation`，`canonicalMIMEHeaderKey` 另有约 `21,845 objects / 0.5 MiB`；候选两条目标节点均降到报告阈值以下。整轮 sampled allocation `171,375,677 -> 164,300,988 B (-4.1%)`、sampled objects `1,394,070 -> 1,363,713 (-2.2%)`、positive-live `-14.0%`，但 post-live `+11.6%` 反向，因此不声明常驻内存改善。

5 秒 CPU 为 `2026-07-27T21-06-48-931Z -> 2026-07-27T21-39-40-932Z`。总样本 `3.55 -> 3.65 CPU s (+2.8%)` 基本处于短 profile 波动，`scanobject` flat `690 -> 600 ms (-13.0%)`、cumulative `1610 -> 1630 ms` 基本持平；目标 lookup 在两轮均低于 CPU 分辨率。本轮只接受确定的 allocation/object 收益，不作 CPU 提速宣称。

首次正式矩阵 `body-2026-07-27T21-41-43-728Z` 在第 1 轮报告生成并完成 MITM stop 后，Electron CDP 返回已知瞬态 `Promise was collected`；应用/Yak 未 panic，build/tmp、测试 home 与进程清理成功。该失败矩阵保持 failed 且没有混入性能样本。复用已缓存发布版二进制完整重跑，正式 3+3 为 `body-2026-07-27T21-08-49-299Z -> body-2026-07-27T21-48-21-509Z`，比较文件 `comparison-vs-phase71-header-value-fast-path.{json,md}` passed，配置、诊断和 metric coverage 差异为空。

候选三轮均完成 1000/1000、数据库总数/唯一 ID 和 direct row 均为 1000、0 Query/fallback/Gap/缺序/重复/乱序/unavailable，停止与最终 backlog 0、CPU 恢复且清理成功。duplex p95 `-21.1%`、request p95 `-25.0%`、最大 visible backlog `20 -> 17`、Yak CPU p95 `-8.7%`、Electron CPU p50 `-5.0%`、RSS `-2.2%`、首显 `-5.7%` 为有利方向；DB catch-up `+11.6%`、最大 persistence backlog `3 -> 4`、Yak CPU p50 `+20.7%` 和 Electron CPU p95 `+3.0%` 反向，仍作为 WSL 短窗风险公开。

本轮没有前端产品、协议、proto、schema、数据库、GORM 或 driver 变化。验证隔离 Go cache 峰值约 1.2 GiB，已永久删除；E2E build/tmp、测试 home 和相关进程退出后不存在，Yak 缓存维持 6 份/约 1.4 GiB，全局 Go cache 约 95 MiB、磁盘可用约 839 GiB。这些仍是清理后的当前状态，不能覆盖历史 `/home/go0p/.cache/go-build = 290G` 事故。下一轮优先拆分最新 `splitHTTPPacketEx`、`bufio.ReadBytes`、response parser 和 context metadata 的 ownership/caller。

## 41. 第七十三轮：split parser 复用 owned line string（2026-07-27）

第七十二轮 heap 将 `splitHTTPPacketEx` 定位为 `233,811 cumulative sampled objects` 的热点，约 52% 来自 `fixHTTPPacketCRLF`、35% 来自公开 `SplitHTTPPacketEx`。该 parser 的 first line 来自 `BufioReadLine` 独占 allocation，Header 行来自第六十七轮已建立不再修改契约的 scanner，但旧实现仍为 raw-first-line hook、request/response callback、重建 Header 和每个 Header hook 分别执行 `string([]byte)` 拷贝。

候选将 first line 只建立一次零拷贝 string view 并在所有 callback/重建路径复用；Header hook 与最终重建字符串直接借用各自 owned line。完整 headers 最终仍由 `strings.Join` 生成独立结果，body view/copy 契约不变。生命周期测试在调用返回后覆写原 packet、强制 GC，再验证 raw first line、request line 三段、所有 hook line 和重建 headers；完整 lowhttp `184.144 s` 与 reader-pool/retained-line 定向 race 均通过。

同一热缓存临时切回原实现再恢复候选的严格五次 A/B 中，256 KiB 普通 legacy view 中位约 `664.5 -> 589.9 ns/op (-11.2%)`、`514 -> 426 B/op (-17.1%)`、`12 -> 9 allocs/op (-25.0%)`；request callback view `674.4 -> 591.0 ns/op (-12.4%)`、`482 -> 386 B/op (-19.9%)`、`13 -> 9 allocs/op (-30.8%)`；Header hook view `689.9 -> 597.6 ns/op (-13.4%)`、`514 -> 426 B/op`、`12 -> 9 allocs/op`。单行 copy/borrow 微基准为约 `19.22 -> 0.714 ns/op`、`32 -> 0 B/op`、`1 -> 0 allocs/op`。

真实 heap 为 `2026-07-27T21-33-20-051Z -> 2026-07-27T22-07-17-076Z`。`splitHTTPPacketEx` flat sampled objects `67,150 -> 10,058 (-85.0%)`、cumulative `233,811 -> 95,529 (-59.1%)`，Header callback cumulative `105,130 -> 64,171 (-39.0%)`。整轮 sampled objects `+5.6%`、allocation `+2.0%` 受 GORM Quote、Builder 和 reflect 等 caller 反向采样影响；positive-live 反向而 post-live `-8.9%`，因此只接受目标 split caller 与严格微基准证据。

5 秒 CPU 为 `2026-07-27T21-39-40-932Z -> 2026-07-27T22-13-41-550Z`。总样本 `3.65 -> 3.58 CPU s (-1.9%)`，目标 split 节点在两轮均低于 CPU 分辨率；scanobject flat 反向、cumulative 小幅有利，不作 CPU 提速宣称。

首次正式矩阵 `body-2026-07-27T22-15-49-617Z` 前两轮通过，第 3 轮启动时 Electron CDP bridge 暂时不可用，截图也超时；失败矩阵保持 failed、未混入性能比较。自动化将精确错误 `CDP bridge is not available, API is disabled` 加入既有瞬态白名单，只在幂等 window-state 查询内最多重试一次，并由外层 `waitUntil` 继续受 15/30 秒硬超时约束；应用断言、后端错误和持续不可用仍直接失败。CDP 5 项单测通过。

修复后完整重跑的正式 3+3 为 `body-2026-07-27T21-48-21-509Z -> body-2026-07-27T22-26-18-523Z`，比较文件 `comparison-vs-phase72-owned-split-lines.{json,md}` passed，配置、诊断和 metric coverage 差异为空。候选三轮均完成 1000/1000、数据库总数/唯一 ID 与 direct row 均为 1000、0 Query/fallback/Gap/缺序/重复/乱序/unavailable，停止与最终 backlog 0、CPU 恢复且清理成功。

候选中位 DB catch-up/drain `-26.0%/-17.1%`、duplex p95 `-20.0%`、Renderer drain `-13.7%`、首显 `-22.0%`、Yak CPU p50 `-16.6%`、Electron CPU p95 `-2.9%`；flow committed delivery p95 `+6.6%`、最大 visible backlog `17 -> 18`、Electron CPU p50/drain p95 `+4.0%/+12.5%`、Electron/Yak RSS `+2.7%/+1.1%`、Yak CPU p95 `+3.0%` 反向，继续作为 WSL 短窗风险公开。

本轮没有前端产品消费、协议、proto、schema、数据库、GORM 或 driver 变化；前端只增强 E2E 瞬态 CDP 容错。873 MiB 验证隔离 cache 已永久删除；E2E build/tmp、测试 home 和相关进程退出后不存在，Yak 缓存维持 6 份/约 1.4 GiB，全局 Go cache 约 101 MiB、磁盘可用约 839 GiB。这些仍是清理后的当前状态，不能覆盖历史 `/home/go0p/.cache/go-build = 290G` 事故。下一轮继续拆分 Builder、response parser、context 与 glob MIME caller。

## 42. 第七十四轮：MITM 静态 MIME glob 预编译（2026-07-28）

第七十三轮 heap 证明第七十轮仍有一个未覆盖分支：默认 `ExcludeMIME` 会经过 `MITMFilter.IsMIMEPassed -> YakMatcher -> MIMEGlobRuleCheck`，对每条响应重新编译 `image/*`、`audio/*`、`video/*` 和 `*zip`。基线 `MIMEGlobRuleCheck` 累计约 `65,537 sampled objects`，其中 `glob.Compile/parserMain` 约 `32,768 objects`。候选在 filter 更新、并发发布前预编译无编码的静态 MIME 规则；运行时只读复用。规则有/无 `/`、component wildcard、bare wildcard、大小写 contains、非法 glob、encoded group 和运行后新增 pattern 均保留旧分支语义，不改变 filter 配置、RPC 或持久化格式。

同二进制五次配对 benchmark 中位约为 `4151 -> 2061 ns/op (-50.3%)`、`2808 -> 841 B/op (-70.0%)`、`77 -> 22 allocs/op (-71.4%)`。固定 oracle 覆盖 MIME 的全部旧分支，差分 fuzz 在 2 个 worker 下执行 `7,897` 次无差异；完整 `common/yak/httptpl`、预编译 matcher 并发 race 和现有 MITM V2 Content-Type gRPC 过滤测试均通过。

真实 heap 为 `2026-07-27T22-07-17-076Z -> 2026-07-28T02-54-28-837Z`。候选的 `MIMEGlobRuleCheck`、`glob.Compile` 和 `parserMain` 均降到差分报告阈值以下，完整 YakMatcher cumulative sampled objects 约 `132,780 -> 10,923 (-91.8%)`。同时出现的 Builder、GORM 和大 packet allocation 波动没有直接调用因果，因此不据此声明全局 heap 或常驻内存改善。

5 秒 CPU 为 `2026-07-27T22-13-41-550Z -> 2026-07-28T03-01-05-758Z`。YakMatcher cumulative 样本约 `40 -> 10 ms`，`IsMIMEPassed` 约 `30 -> 10 ms`；整轮总样本 `3.58 -> 1.99 CPU s`，但 GC 与 GORM Create 同时大幅波动，不能把总量变化归因给 MIME 规则。本轮 CPU 只作为目标 heap/微基准的同方向佐证。

正式无 profile 3+3 为 `body-2026-07-27T22-26-18-523Z -> body-2026-07-28T03-03-30-690Z`，比较文件 `comparison-vs-phase73-static-mime-precompile.{json,md}` passed，配置、诊断和 metric coverage 差异为空。候选三轮均完成 1000/1000，数据库总数/唯一 ID 与 direct row 均为 1000，0 Query/fallback/Gap/缺序/重复/乱序/unavailable，停止与最终 backlog 0、CPU 恢复且清理成功。

固定速率产品指标是混合结果：Renderer drain `-4.6%`、Yak RSS `-4.3%`、Yak drain CPU p95 `-32.8%` 有利；DB catch-up `+21.8%`、duplex p95 `24 -> 66 ms`、request -> React p95 `+10.4%`、visible backlog `18 -> 20`、Yak CPU p50/p95 `+9.6%/+13.3%` 和 Electron CPU p50/p95 `+7.4%/+10.3%` 反向。候选保留确定的 MIME matcher 局部收益，但正式矩阵没有证明整机 CPU 或交互延迟改善，后续不会据此盲调前端。

本轮没有前端产品、通信协议、proto、schema、数据库、GORM 或 driver 变化。fuzz/race 隔离 cache/tmp 峰值约 `5.7/1.9 GiB` 后已永久删除；E2E 专用 build/tmp 在退出后不存在，Electron/Yak/WDIO/chromedriver 无残留，Yak 二进制缓存约 1.4 GiB、全局 Go cache 约 102 MiB、磁盘可用约 839 GiB。这些仍是用户手工清理后的状态，不能覆盖历史 `/home/go0p/.cache/go-build = 290G` 事故。下一轮优先拆分最新 response reader、packet grow/quote 和 context metadata 的可归因成本。

## 43. 第七十五轮：YakMatcher raw material 去 hash/cache（2026-07-28）

第七十四轮去掉 MIME glob 编译后，heap 中剩余 YakMatcher 对象全部来自通用 `getMaterial`：空 scope 或显式 `raw` 的结果本来就是当前 packet，却仍计算完整 `SHA1(req+rsp+scope)`、hex 编码并访问一分钟 TTL cache。基线 `YakMatcher -> cacheHash -> CalcSha1 -> hex.EncodeToString` 约 `10,923 sampled objects`。该 cache 对 raw material 不做解析复用，hash 成本反而远高于短 Content-Type 的匹配本身。

候选仅对空 scope 和显式 `raw` 返回调用期只读 byte-to-string view；matcher 在本次调用内同步消费，不跨调用保留。表达式继续取得 owned string，header/body/request/interactsh/unknown scope 继续走旧解析与 cache 路径。无编码 Group 直接只读遍历原 slice，不再复制；编码 Group 继续解码到独立预分配 slice。`binary` matcher 强制 hex 的旧语义有独立回归用例，防止局部 encoding 归一化改变行为。

同缓存、改动前后五次 A/B 中，完整预编译默认 MIME matcher 中位约 `2176 -> 263 ns/op (-87.9%)`、`841 -> 0 B/op`、`22 -> 0 allocs/op`。只隔离 raw material 的旧 hash/cache oracle 与新调用期借用约为 `1647 -> 38.36 ns/op (-97.7%)`、`425 -> 0 B/op`、`18 -> 0 allocs/op`。测试覆盖默认/显式 raw、word/suffix/regexp/glob/MIME、and/negative、hex/binary、packet 原地修改、NUL/非法 UTF-8/Unicode；差分 fuzz 执行 `8,933` 次无差异，完整 `common/yak/httptpl`、定向 race 和现有 MITM V2 Content-Type gRPC 测试均通过。

真实 heap 为 `2026-07-28T02-54-28-837Z -> 2026-07-28T03-31-16-037Z`。基线的 YakMatcher、`cacheHash`、`CalcSha1`、hex 与 MIME 节点在候选差分报告中全部无匹配。整轮 sampled objects `115,130 -> 214,431` 受进程名查询、SQLite Query、Header canonical 与 pprof 自身采样反向影响，因此只接受目标 caller 消失，不声明全局 heap 或常驻内存改善。

5 秒 CPU 为 `2026-07-28T03-01-05-758Z -> 2026-07-28T03-37-46-431Z`。基线 YakMatcher/cacheHash/IsMIMEPassed 各约 `10 ms`，候选均低于 10 ms 分辨率；总样本 `1.99 -> 1.61 CPU s (-19.1%)`，`scanobject` cumulative `1.55 -> 1.16 s (-25.2%)`。该单次 profile 与目标分配方向一致，但目标本身已低于 CPU 采样粒度，不能把整轮下降完全归因给本次改动。

正式无 profile 3+3 为 `body-2026-07-28T03-03-30-690Z -> body-2026-07-28T03-39-59-596Z`，比较文件 `comparison-vs-phase74-raw-material-fast-path.{json,md}` passed，配置、诊断与 metric coverage 差异为空。候选三轮均完成 1000/1000，数据库总数/唯一 ID 和 direct row 均为 1000，0 Query/fallback/Gap/缺序/重复/乱序/unavailable，停止与最终 backlog 0、CPU 恢复且清理成功。

固定速率中位仍为混合：request/response -> React p95 `-7.7%/-7.8%`、flow committed delivery p95 `-9.2%`、visible backlog `20 -> 19`、Yak CPU p95 `-21.0%`、Electron CPU p50/p95 `-1.7%/-2.8%` 有利；Yak CPU p50 `+36.0%`、request p95 `+27.0%`、首显 `42 -> 63 ms`、DB catch-up/drain `+11.0%/+9.6%`、Renderer drain `+9.7%` 和 Yak RSS `+2.6%` 反向。保留确定的 matcher 局部收益，但不宣称本轮稳定改善整机或首屏体感。

本轮没有前端产品、通信协议、proto、schema、数据库、GORM 或 driver 变化。隔离 fuzz/race cache 峰值约 7.1 GiB 后永久删除；E2E 专用 build/tmp 冷构建峰值约 `3.1/3.1 GiB`，退出后不存在，Electron/Yak/WDIO/chromedriver 无残留。Yak 二进制缓存仍为 6 份/约 1.4 GiB，全局 Go cache 约 104 MiB、磁盘可用约 839 GiB。这些仍是用户手工清理后的状态，不能覆盖历史 290 GiB Go cache 事故。下一轮回到最新 response reader、Header canonical、packet grow/quote 和 context caller。

## 44. 第七十六轮：未修改响应跳过 snapshot/reparse（2026-07-28）

第七十五轮大 Body heap 中，普通 MITM V2 自动转发响应仍经过通用 crep 劫持回调的保守边界：即使插件、规则和人工交互都没有修改 packet，回调返回的就是原始 response packet，crep 仍会 `bytes.Clone` 整包并再次调用 response parser。该边界对未知旧回调是必要的，但 V2 自身会在所有修改路径设置 `ResponseModified`，能够提供更强的显式契约。基线 `cloneAndParseHijackedResponse` 为 `39,296,860 B`，全局 `bytes.Clone` 为 `51,736,565 B`。

候选新增 modification-aware response hijack option，由 MITM V2 返回 packet 与 `modified`。只有 `modified=false` 且返回切片仍与原 packet 具有相同长度和起始地址时，才保留已经解析好的 `http.Response`；显式修改、独立返回切片、错误标记和所有旧 callback 继续 snapshot/reparse。旧 option 不变，两个 option 按最后配置者替换，避免第三方调用方或原地修改回调发生破坏性行为。

256 KiB 定点五次基准中，旧 snapshot/reparse 中位约 `49.7 us/op / 272,682 B/op / 38 allocs/op`，显式未修改 fast path 约 `2.5 ns/op / 0 B/op / 0 allocs/op`。新增测试覆盖旧回调保守语义、option 替换、同 packet fast path、显式修改和“独立 packet 却误报未修改”的回退；完整 `common/crep`、定向 race 和全部 `TestGRPCMUSTPASS_MITMV2*`（`198.797 s`）通过。组合运行时一项 replacer 测试曾遇到一次 gRPC deadline，单独复现 `2.23 s` 通过并验证规则修改标签，未用重跑替代语义断言。

同配置 forced-GC heap 为 `2026-07-28T03-31-16-037Z -> 2026-07-28T04-20-06-196Z`。`cloneAndParseHijackedResponse 39,296,860 B -> 0`，`bytes.Clone 51,736,565 -> 5,058,141 B (-90.2%)`，response handler cumulative `125,316,409 -> 88,387,165 B (-29.5%)`；窗口 allocation delta `347,607,036 -> 297,616,058 B (-14.4%)`。剩余 clone 来自请求隔离和辅助流量。旧 CPU profile 中目标低于 10 ms 采样分辨率，因此没有制造一轮无法归因的 CPU profile。

正式无 profile 3+3 为 `body-2026-07-28T03-39-59-596Z -> body-2026-07-28T04-27-08-051Z`，比较文件 `comparison-vs-phase75-unmodified-response-fast-path.{json,md}` passed，配置、诊断和 metric coverage 差异为空。候选每轮均为 producer/target/database/unique/direct `1000/1000`，0 Query/fallback/gap/缺序/重复/乱序/unavailable，停止与最终 backlog 0、CPU 恢复且清理成功。

产品中位仍混合：DB catch-up/drain `-24.0%/-17.2%`、duplex p95 `-22.7%`、首显 `-28.6%`、request p95 `-13.4%`、Renderer drain `-15.2%`、Yak CPU p50 `-13.3%` 有利；visible backlog `19 -> 25`、request/response -> React `+10.2%/+9.3%`、Yak CPU p95 `+19.4%`、Yak RSS `+3.6%`、Electron CPU p50/p95 `+6.0%/+3.0%` 反向。候选按确定的 ownership、微基准、heap 和完整正确性证据保留，不宣称整机稳定提速，也不据此调整前端调度。

本轮没有 proto、schema、数据库、GORM、driver 或前端产品逻辑变化。3.7 GiB 隔离 Go cache 已永久删除；E2E build/tmp 和相关进程无残留，Yak 二进制缓存保持 6 份/约 1.4 GiB，全局 Go cache 约 151 MiB、磁盘可用约 839 GiB。这些仍是用户清理后的当前值，不覆盖历史 290 GiB Go cache 事故。下一轮继续从 response/request packet grow、process lookup 与 context caller 中按 profile 选点。

## 45. 第七十七轮：未修改请求跳过重复 parse（2026-07-28）

第七十六轮大 Body heap 显示请求侧也存在同类保守边界：MITM V2 没有命中过滤、规则、插件或人工修改时返回原 request packet，crep 仍无条件执行 `ParseBytesToHttpRequest` 并替换已经解析好的请求。该行为对未知旧 callback 必须保留，但 V2 会在所有修改路径写入 `RequestModified`，可以提供显式契约。

候选新增 modification-aware request hijack option。只有 `modified=false` 且返回切片与原 packet 长度、起始地址都相同时，才保留现有 `http.Request`；旧 callback、显式修改、独立结果、错误标记和 drop 继续走保守解析。两个 request option 按最后配置者替换，原地修改方必须显式报告 true，因此是兼容增加而不是破坏性更新。

256 KiB 五次基准中，旧重复解析中位约 `152.7 us/op / 807,492 B/op / 67 allocs/op`，显式未修改约 `2.917 ns/op / 0 B/op / 0 allocs/op`。测试覆盖旧回调保守语义、option 双向替换、同 packet 快路径、显式修改和“独立 packet 却误报未修改”的回退；完整 `common/crep`、定向 race 和全部 `TestGRPCMUSTPASS_MITMV2*`（`197.079 s`）通过。

同配置 forced-GC heap 为 `2026-07-28T04-20-06-196Z -> 2026-07-28T04-57-46-431Z`。`hijackRequestHandler` cumulative `43,526,824 -> 21,617,855 B (-50.3%)`，`ParseBytesToHttpRequest -45.5%`、`FixHTTPPacketCRLF -42.0%`、`ReadHTTPRequestFromBytes -41.1%`，底层 request reader `-16.7%`；初次 wire parse 与 owned bare dump 仍保留。整窗 allocation 仅 `-2.7%`，positive-live/post-live 虽有利但属于 forced-GC 诊断，不声明整机或常驻内存下降。旧 CPU profile 中目标低于有效分辨率，因此没有追加无法归因的 CPU 轮次。

正式无 profile 3+3 为 `body-2026-07-28T04-27-08-051Z -> body-2026-07-28T05-05-07-631Z`，比较文件 `comparison-vs-phase76-unmodified-request-fast-path.{json,md}` passed，case 配置、诊断与 metric coverage 差异为空。候选每轮 producer/target/database/unique/direct 均为 `1000/1000`，0 Query/fallback/gap/缺序/重复/乱序/unavailable，停止与最终 backlog 0、CPU 恢复且清理成功。

产品中位仍混合：Yak CPU p50 `-22.4%`、request p95 `-5.7%`、request/response -> React `-2.5%/-4.3%`、Long Task 总时长 `-49.5%`、Electron CPU p50/p95 `-4.0%/-3.4%` 有利；visible backlog `25 -> 40`、duplex p95 `+31.0%`、DB catch-up/drain `+27.4%/+18.5%`、Renderer drain `+15.1%`、Yak drain CPU `+25.9%` 反向。保留确定的请求解析收益，但不宣称整机全面提速；后续把“后端更快后 direct burst/消费峰值变大”作为前端 trace 与调度候选，而不是凭单轮直接改参数。

本轮没有前端产品、通信协议、proto、schema、数据库、GORM 或 driver 变化。3.7 GiB 验证 cache 与约 2.9 GiB tmp 已永久删除；E2E profile/正式 build/tmp 峰值约 `2.0/2.0 GiB` 和 `1.7/1.7 GiB`，退出后不存在，Electron/Yak/WDIO/chromedriver 无残留。Yak 缓存保持 6 份/约 1.4 GiB，全局 Go cache 约 158 MiB、磁盘可用约 839 GiB。这些是用户手工清理后的当前值，不能覆盖历史 `/home/go0p/.cache/go-build = 290G` 事故。下一轮以后端最新 heap 为主，同时用重复 Renderer trace 验证 burst 是否真落在前端 commit/layout。

## 46. 第七十八轮：连接级 context I/O 取消替代逐操作复制（2026-07-28）

第七十七轮大响应 heap 中，`CreateProxyHandleContext` 对连接同时使用 `ctxio.NewReader/NewWriter`。旧包装为了让每一次 Read/Write 都能与 context 取消竞争，会逐操作创建等长 buffer、channel 和 goroutine；256 KiB 响应因此在最终 HTTP packet 之外又产生一份传输复制。候选把取消生命周期绑定到 minimartian 连接：读写直接把调用方 packet 交给 `net.Conn`，每连接仅一个 watcher，取消时用 deadline 中断阻塞操作，不支持 deadline 时才关闭连接；release 会停止并等待 watcher，再把 reader/writer 放回池。通用 `ctxio` 与其他调用方没有变化。

五次同二进制基准中，64 KiB read 中位约 `14.107 us / 65,720 B / 4 allocs -> 1.019 us / 0 B / 0 allocs`；256 KiB write 的包装开销约 `40.461 us / 262,330 B / 4 allocs -> 7.614 ns / 0 B / 0 allocs`。write 使用无分配 sink，只证明旧 wrapper 的复制成本，不代表网络吞吐。测试覆盖读写 packet identity、阻塞 read 取消、release 后取消不污染连接和池生命周期；完整 minimartian、定向 race 与全部 MITM V2 MUSTPASS（`198.890 s`）通过。

forced-GC heap `2026-07-28T04-57-46-431Z -> 2026-07-28T05-33-50-436Z` 中，旧 `ctxReader.Read 11.65 MB` 与 downstream `bufio.Writer.Write 39.97 MB` 分配消失，`Proxy.handleRequest` cumulative `220.53 -> 162.20 MB (-26.5%)`，整窗 sampled allocation `289.59 -> 230.51 MB (-20.4%)`；必要的最终 packet `bytes.growSlice` 基本持平。positive-live 反向、post-GC live 小幅有利，均不用于常驻内存结论。CPU 诊断中目标栈消失、GC scan 有利，但总样本和 handleRequest 反向；它不是相邻受控 A/B，因此不宣称全局 CPU 提速。

正式无 profile 3+3 为 `body-2026-07-28T05-05-07-631Z -> body-2026-07-28T05-44-17-656Z`，比较文件 `comparison-vs-phase77-connection-bound-direct-io.{json,md}` passed，配置、诊断和 metric coverage 差异为空。候选三轮均完成 producer/target/database/unique/direct `1000/1000`，0 Query/fallback/gap/缺序/重复/乱序/unavailable，停止与最终 backlog 0、CPU 恢复且清理成功。

产品中位以有利方向为主：visible backlog `40 -> 19 (-52.5%)`、duplex p95 `76 -> 45 ms (-40.8%)`、request -> React p95 `116 -> 107 ms (-7.8%)`、request p95 `-10.2%`、Renderer drain `-5.5%`、Electron CPU `-2.8%~-5.3%`；DB catch-up `+3.8%`、Yak drain CPU p95 `+14.5%` 反向，Yak 常态 CPU/RSS 基本持平。第七十七轮的前端 burst 信号没有在本轮重复，因此暂不修改前端 batch/interval；下一轮继续以后端最新 packet ownership、GORM quote/DB 写入和 GC profile 选点，只有重复 trace 明确落到 React commit/layout 才进入前端产品优化。

本轮没有前端产品、协议、proto、schema、数据库、GORM 或 driver 变化。冷构建专用 build/tmp 峰值约 `3.3/3.3 GiB`，退出后已删除；Electron/Yak/WDIO/chromedriver 和测试 home 无残留。Yak 二进制缓存保持最多 6 份/约 1.4 GiB，全局 Go cache 约 165 MiB、磁盘可用约 839 GiB。这些仍是用户清理后的当前值，不能覆盖历史 `/home/go0p/.cache/go-build = 290G` 事故；yaklang/yakit 均未提交或推送。

## 47. 第七十九轮：HTTPFlow 参数统计复用已解析请求（2026-07-28）

第七十八轮大 Body heap 将 `CreateHTTPFlow` 内的参数统计定位为新的可归因热点：MITM pipeline 已经持有解析后的 `*http.Request`，旧实现却先 `DumpHTTPRequest`，再通过 `NewFuzzHTTPRequest` 重新 parse，只为取得 GET、POST、Cookie 三组参数列表的长度。候选新增 `mutate.CountHTTPRequestParams` 直接复用该 request；只有调用方没有传入解析实例时才从 raw packet parse 一次。函数继续调用现有 fuzz 参数语义，保留 query/Cookie JSON 与 base64、form、JSON body、XML、重复项和空 body 的计数行为，并通过原 POST 路径恢复 `Body`，不持有 request。

同二进制五次基准中位约为 `18.147 -> 9.863 us/op (-45.6%)`、`10,071 -> 5,081 B/op (-49.5%)`、`225 -> 155 allocs/op (-31.1%)`。差分用例逐类比较旧列表物化与新入口的三个计数；HTTPFlow 产品测试验证 parsed/raw 两条路径计数一致且 body 仍可读取。完整 `common/mutate`、`common/yakgrpc/yakit`、定向 race 与全部 MITM V2 MUSTPASS（`193.596 s`）通过。

forced-GC heap `2026-07-28T05-33-50-436Z -> 2026-07-28T06-23-26-506Z` 中，`CreateHTTPFlow` cumulative sampled allocation `28.89 -> 10.11 MB (-65.0%)`，旧 `NewFuzzHTTPRequest -> DumpHTTPRequest` 与 query getter 内的 request reparse 栈消失；剩余约 10.11 MB 是现有 64 KiB POST 参数物化。整窗 `bytes.growSlice -5.7%`、sampled allocation `-6.0%`，最终数据库 packet quote 基本不变。positive-live 有利而 post-live `+5.7%` 反向，不用于 RSS 结论。CPU `2026-07-28T06-30-52-200Z` 中目标栈降到 10 ms 以下，但总样本和 GC scan 反向，因此不声明整机 CPU 改善。

首次正式 3+3 为 `body-2026-07-28T05-44-17-656Z -> body-2026-07-28T06-33-18-310Z`，比较文件 `comparison-vs-phase78-reuse-parsed-request-param-counts.{json,md}` passed，但多数产品中位反向。为避免把 Electron/WSL 抖动归因给后端，未改代码立即重跑 `body-2026-07-28T06-44-50-029Z`。A/A 自身出现 duplex p95 `87 -> 53 ms`、request -> React `128 -> 109 ms`、Yak CPU p95 `163.49% -> 148.37%`、Long Task `290 -> 103 ms` 的大幅漂移；第二组相对 Phase 78 的首显仅 `45 -> 46 ms`，request/response -> React 为 `107/107 -> 109/108 ms`，request p95 与 Yak CPU p50 近乎持平，DB/Renderer drain 仍混合。这证明首组广泛回退不是本次局部代码的稳定因果。

两组候选共 6 轮均完成 producer/target/database/unique/shadow-direct/live-direct `1000/1000`，Query、fallback、gap、缺序、重复、乱序、replay、recovery、unavailable 均为 0，最终 backlog 0、CPU 恢复且清理成功。本轮按确定的语义、微基准和目标 heap 收益保留；整机产品证据归类为中性且有明显环境噪声，不据此修改前端 batch/interval。

本轮没有前端产品、协议、proto、schema、数据库、GORM 或 driver 变化。验证/E2E 专用 build/tmp、测试 home 与 Electron/Yak/WDIO/chromedriver 均无残留；Yak 二进制缓存维持最多 6 份/约 1.4 GiB，全局 Go cache 约 174 MiB、磁盘可用约 839 GiB。这仍是用户清理后的当前状态，不能覆盖历史 `/home/go0p/.cache/go-build = 290G` 事故。下一轮可在相同差分 oracle 约束下探索 count-only visitor，避免当前剩余的参数对象列表物化；同时继续以后端最新 heap/CPU 而不是猜测驱动选点。

## 48. 第八十轮：HTTPFlow 参数 count-only visitor（2026-07-28）

第七十九轮已经去掉 dump/reparse，但仍为三个总数构造所有 `FuzzHTTPRequestParam`、JSON/GJSON path 和 XML XPath。候选保持公共 fuzz 参数列表 API 不变，只在 HTTPFlow 计数入口按相同 query/form、JSON、base64、Cookie whitelist、XML/SOAP traversal 规则累计数字；POST body 继续保持 consume-and-restore 语义。

固定差分覆盖重复/不可见/keyless query、忽略 Cookie、JSON/base64 JSON、嵌套数组、form fallback、XML declaration/comment、SOAP 和空 body；2 worker 有界 fuzz 执行 `48,956` 次无差异。完整 mutate/yakit、定向 race 和全部 MITM V2 MUSTPASS（`192.823 s`）通过。五次同二进制 Phase 79 materialization 与 count-only 中位约为 `10.205 -> 3.164 us/op (-69.0%)`、`5,134 -> 1,252 B/op (-75.6%)`、`155 -> 34 allocs/op (-78.1%)`。

forced-GC heap `2026-07-28T06-23-26-506Z -> 2026-07-28T07-17-18-286Z` 中，`CountHTTPRequestParams 10.60 -> 8.92 MB (-15.8%)`，参数对象/path 节点降到阈值以下；剩余 8.92 MB 全部来自 `httpRequestReadBody` 复制 64 KiB parser-owned body。`CreateHTTPFlow -7.3%`、整窗 sampled allocation `-7.9%`，post-GC live 方向有利。CPU `2026-07-28T06-30-52-200Z -> 2026-07-28T07-25-10-025Z` 总样本 `-14.6%`、scanobject cumulative `-23.6%`，但 CreateHTTPFlow `260 -> 320 ms` 反向，因此仍不声明整机 CPU 提速。

正式无 profile 3+3 为 `body-2026-07-28T06-44-50-029Z -> body-2026-07-28T07-28-10-244Z`，比较文件 `comparison-vs-phase79-count-only-param-totals.{json,md}` passed，诊断一致。候选三轮全部完成 producer/target/database/unique/shadow-direct/live-direct `1000/1000`，Query、fallback、gap、缺序、重复、乱序、replay、recovery、unavailable 为 0，最终 backlog 0、CPU 恢复且清理成功。

产品中位中 duplex p95 `53 -> 43 ms`、首显 `46 -> 42 ms`、request -> React `109 -> 107 ms`、visible backlog `20 -> 19`、Yak CPU p50 `49.65% -> 44.86%` 有利；request p95、Yak CPU p95/RSS、Long Task 和 Electron CPU 近中性；DB catch-up/drain 与 Renderer drain 分别反向 `7.1%/8.4%/7.2%`。本轮保留确定的对象/分配和正确性收益，不包装成所有产品指标改善，也不据此修改前端参数。

验证隔离 cache/tmp 峰值约 `4.6/2.7 GiB` 后已永久删除；E2E build/tmp、测试 home 与 Electron/Yak/WDIO/chromedriver 无残留。Yak 缓存维持最多 6 份/约 1.4 GiB，全局 Go cache 约 174 MiB、磁盘可用约 839 GiB。这些仍是清理后的值，不能覆盖历史 290 GiB Go cache 事故。下一轮 Phase 81 已由 heap 明确指向 parser-owned request body 的只读 view/恢复契约，目标是消除剩余约 8.9 MB 快照；先补 ownership/lifecycle oracle，再做实现。

## 49. 第八十一轮：parser-owned Request Body 只读 view（2026-07-28）

第八十轮剩余 8.92 MB 已完整归因于参数计数读取 parser-owned request body 时的复制/重装。候选为解析器自有 Body 增加同步只读 view + reset 契约；只有内部 owned 类型走零复制，foreign/custom `io.ReadCloser` 保留原 copy-and-restore。局部读取、foreign body 不消费、强制 GC、重复读取均有 ownership/lifecycle 测试，34,644 次有界差分 fuzz 无计数差异；完整 utils/mutate/yakit、定向 race 和全部 MITM V2 MUSTPASS（`201.469 s`）通过。

64 KiB 五次局部基准中位约 `12.407 us / 65,600 B / 3 allocs -> 5.596 ns / 0 B / 0 allocs`，只代表同步 Body 读取契约。forced-GC heap `2026-07-28T07-17-18-286Z -> 2026-07-28T08-01-35-167Z` 中旧 8,923,815 B Body copy 栈消失，`CreateHTTPFlow -19.7%`、整窗 sampled allocation `-11.1%`。CPU `2026-07-28T07-25-10-025Z -> 2026-07-28T08-08-07-756Z` 中 Count 目标降到 10 ms 以下、CreateHTTPFlow `320 -> 220 ms (-31.3%)`，但整窗 CPU/GC scan 反向，仍只接受目标 caller。

首次正式矩阵 `body-2026-07-28T08-10-11-405Z` 与同代码重复 `body-2026-07-28T08-20-30-981Z` 均正确，但相对第八十轮仍稳定显示 duplex/Yak CPU 较高。为避免继续用跨时段矩阵误归因，前端 E2E 新增严格的受管缓存 Yak 指纹选择：只接受已有 20 位小写十六进制 executable，缺失或与 profile/trace 混用立即失败，并同时记录 selected/current-source fingerprint 与匹配状态；fixture 13 项测试通过。

小包同窗旧/新 3+3 为 `body-2026-07-28T08-32-58-073Z -> body-2026-07-28T08-38-43-195Z`，精确使用第八十轮 `536fe35700419c447fcc` 与第八十一轮 `cd3f035a183b867b86e2`。旧二进制在当前时段自身变为 duplex `81 ms`、request -> React `117 ms`、Yak CPU p50 `49.56%`，而早先同一旧二进制为 `43 ms/107 ms/44.86%`，直接证明此前广泛反向主要是时段/环境漂移。相邻 A/B 中当前版 Yak CPU p50 `+0.3%` 近中性，duplex `-11.1%`、request -> React `-3.4%`、DB drain `-26.3%`、Renderer drain `-19.5%` 有利；request latency 与 Long Task 反向且分布宽，不作全面产品提速结论。

直接覆盖改动的 64 KiB request-body 同窗 3+3 为 `body-2026-07-28T08-45-35-379Z -> body-2026-07-28T08-50-45-504Z`。吞吐 `+1.1%` 近中性、Yak CPU p50 方向有利 `-4.8%`、DB/Renderer drain `-13.8%/-12.0%`；request latency `+10.6%`、request/response -> React `+5.8%/+15.7%`、Yak RSS `+2.8%` 反向。120 条 max-rate 组离散较大，只作为无明确整机 tradeoff 与完整正确性门禁。

两组共 12 轮全部精确完成：小包每轮 producer/target/database/unique/direct `1000/1000`；64 KiB 每轮 `120/120` 且 target request body 恰为 `7,864,320 B`。fallback、gap、缺序、重复、乱序、replay、recovery、unavailable 与 cleanup error 全为 0。候选按 ownership、微基准、heap、CPU target 与端到端正确性证据保留；前端产品、协议、proto、schema、数据库和 GORM 均未修改。

本轮同窗验证全部复用缓存二进制，没有触发 Go build；E2E build/tmp、临时 home 与 Electron/Yak/WDIO/chromedriver 无残留。Yak 缓存约 1.4 GiB、全局 Go cache 约 181 MiB、磁盘可用约 838 GiB；仍明确是用户清理后的当前值，不覆盖历史 290 GiB Go cache 事故。下一阶段继续从最新 Phase 81 heap 的必要 packet quote、剩余 growSlice、SQLite persistence 与 stream publish caller 中按可归因占比选点，不依据短窗 UI 波动猜测优化。

## 50. 第八十二轮：request-owned PlainRequest 缓存借用（2026-07-28）

第八十一轮 heap 中，未编码请求会先保存 request context 自己拥有的 bare packet，随后 `decodeAndCachePlainRequestBytesIfStorable` 仍经 `SetPlainRequestBytes` 再克隆一次完整 packet。候选增加严格的只读借用入口：只有待缓存 slice 与 context bare packet 的起点、长度完全一致时才借用；外部等值 slice、子 slice、编码后新 buffer 和其他非 owned 输入继续克隆，独立 decode buffer 则直接转移所有权。生命周期、外部覆写、强制 GC、别名拒绝和旧 clone oracle 均有测试。

64 KiB 五次微基准中位约为 `13,993 -> 1,468 ns/op (-89.5%)`、`74,507 -> 776 B/op (-99.0%)`、`19 -> 18 allocs/op`；128 KiB 为 `23,905 -> 1,467 ns/op (-93.9%)`、`140,045 -> 776 B/op (-99.4%)`、`19 -> 18 allocs/op`。`httpctx` 完整测试、plain-request 定向测试/race 和全部 `TestGRPCMUSTPASS_MITMV2*`（`197.454 s`）通过。完整 `common/yakgrpc` 在仓库既有的 10 分钟 package timeout 中被终止，日志包含既有 AI/facade 等长任务，没有候选断言失败；没有通过增加 timeout 或资源掩盖该结果。

真实 heap 为 `2026-07-28T08-01-35-167Z -> 2026-07-28T09-39-45-988Z`。基线 `bytes.Clone -> SetPlainRequestBytes -> decodeAndCachePlainRequestBytesIfStorable` 的 `10,116,282 B` flat/cumulative 栈在候选完全消失；同值 `reserveHTTPRequestPacketBody` 仍存在，是请求 parser 自身的 body buffer reservation，不再经过 plain cache clone。整窗 sampled allocation `177,333,317 -> 188,729,189 B` 受 `bytes.growSlice` 和 packet quote 反向采样影响，不能声明全局 allocation 改善；post-live heap `267,815,622 -> 261,270,100 B` 有利。5 秒 CPU 中基线 plain-cache clone 链约 `60–70 ms / 3.45%–4.02%`，候选同类 `1.75 CPU s` 样本中降到报告阈值以下。

120 条 64 KiB 请求体的首组同窗 3+3 表面吞吐 `-11.2%`，但紧接着反向补跑旧二进制后，旧版自身吞吐较前组下降 `9.6%`；候选与后置旧版仅 `-1.7%`，request p95 `-3.8%`、Yak CPU p50 `-3.0%`、Yak RSS `-4.6%`。因此该小样本不再被误报为代码回退。

为提高信噪比，自动化新增资源受控的 `request-64k-medium`：600 条、并发 12、每轮 37.5 MiB 请求体。正式 3+3 为 `body-2026-07-28T10-14-58-858Z -> body-2026-07-28T10-20-06-695Z`，比较文件 `comparison-vs-phase81-cached-request-64k-medium-same-window.{json,md}` passed。候选中位吞吐 `+14.3%`、request p95 `-10.9%`、DB/Renderer drain `-4.6%/-5.6%`、request/response -> React `-10.9%/-19.4%`、Long Task total `-81.8%`；Yak CPU p50 `-1.0%`、RSS `-0.2%` 近中性。first visible `64 -> 156 ms`、duplex p95 `65 -> 127 ms` 和瞬时 visible backlog `21 -> 69` 反向，可能与更快生产及 700 ms 调度相位有关，继续公开保留而不包装成全链路全面改善。

中等矩阵六轮每轮均精确完成 producer/target/database/unique `600/600`，request body `39,321,600 B`；fallback、gap、缺序、重复、乱序、replay、recovery、unavailable 和 cleanup error 均为 0。前端产品消费、通信协议、proto、schema、数据库、GORM 和 driver 未变化；前端只新增受控矩阵用例及门禁。E2E 保持单实例串行、受管 Yak 缓存最多 6 份/约 1.4 GiB，全局 Go cache 约 183 MiB、磁盘可用约 839 GiB，且无 Electron/Yak/chromedriver 残留。这些仍是用户清理后的当前值，不能覆盖历史 `/home/go0p/.cache/go-build = 290G` 事故。下一轮继续以后端 heap 的 packet quote、growSlice 和 parser reservation caller 为主，先证明重复 ownership 再修改。

## 51. 第八十三轮：TrafficGuard prefilter 首次命中缓冲有界化（2026-07-28）

第八十二轮 heap 的剩余 `bytes.growSlice`、packet quote 和 request body/parser allocation 均已有必要输出或 body/bare 独立性契约，不能仅凭 flat 大小删除。可归因的新候选是 TrafficGuard CGO prefilter：旧实现按 `len(data)/8+64` 为每个并发 scratch 预分配 `(end, literalID)` 对，即使 256 KiB 正文零命中也分配约 256 KiB；forced-GC 后 `scanHitsImpl` 约 3.92 MB。

候选把首次容量限制为 8192 对（64 KiB）。C 内核本来就返回未截断的命中总数；若真实命中超过首次容量，Go 端按精确总数扩容并重扫，不漏报。扩容后的 scratch 下次仍先做有界扫描，但需要重扫时会复用已经足够的 backing array，不再重复分配。

2048 对的初版在 256 KiB/4000 命中对抗样本中因额外重扫出现约 43% CPU 回退，已在正式测试前主动否决。调到 8192 后，三次配对基准中：零命中中位约 `102.658 -> 67.553 us/op (-34.2%)`、`270,339 -> 65,537 B/op (-75.8%)`、`1 -> 1 allocs/op`；4000 命中中位约 `200.869 -> 152.681 us/op (-24.0%)`、`398,590 -> 193,787 B/op (-51.4%)`、`17 -> 17 allocs/op`。测试覆盖零命中上限、3000 稠密命中的精确扩容/重扫和第二次 backing buffer 复用。

完整 `common/minirehs`（`42.612 s`）、TrafficGuard、定向 race 和全部 `TestGRPCMUSTPASS_MITMV2*`（`197.776 s`）通过。真实 heap 为 `2026-07-28T09-39-45-988Z -> 2026-07-28T10-49-57-181Z`：`scanHitsImpl 3,917,119 B` 与 `MatchedIndexes 4,441,419 B cumulative` 均降到报告阈值以下；整窗 sampled allocation `188,729,189 -> 184,998,744 B (-2.0%)`、post-live `261,270,100 -> 259,080,023 B (-0.8%)`，positive-live 单样本反向，不声明常驻内存改善。

5 秒 CPU 总样本 `1.75 -> 2.38 s`，prefilter cumulative `60 -> 80 ms`，占比 `3.43% -> 3.36%` 近中性；单次全进程 profile 不支持 CPU 提速宣称，保留配对微基准的直接结论。

正式无 profile 3+3 为 `body-2026-07-28T11-05-00-470Z -> body-2026-07-28T11-10-17-929Z`，比较文件 `comparison-vs-phase82-prefilter-pair-cap.{json,md}` passed。候选中位吞吐 `+6.4%`、response -> React `-25.2%`、Long Task total `-49.5%`、Yak drain CPU p95 `-22.8%`；Yak CPU p50/p95 与 RSS 近中性。DB/Renderer drain `+26.5%/+22.7%`、duplex p95 `+68.7%` 反向且各轮离散较大，因此不作统一产品提速结论。

六轮每轮 producer/target/database/unique 均精确 `120/120`，request body `7,864,320 B`，detail 阶段再次校验 64 KiB request/256 KiB response；fallback、gap、缺序、重复、乱序、replay、recovery、unavailable 和 cleanup error 为 0。前端产品、协议、proto、schema、数据库、GORM 和 driver 未变化。隔离测试/E2E build/tmp 已清理，Yak 缓存仍为最多 6 份/约 1.4 GiB，全局 Go cache 约 183 MiB、磁盘可用约 839 GiB，无 Electron/Yak/chromedriver 残留；这些仍不能覆盖历史 290 GiB Go cache 事故。下一阶段继续从最新 profile 选择有直接 caller 证据的后端点，不破坏 packet/body 独立性。
