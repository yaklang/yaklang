# pcapx 当前性能与 TCP 重组验收

本实现优化经典 pcap 读取、被动 TCP 重组、流式内存管理，并提供按双向连接分片的多 worker。
默认使用同步入口；大流建议开启 Stream，按实际回调成本选择 worker 数量。
本页仅描述当前版本。原始数据、环境、源码 SHA-256 和逐项命令见
[testdata/performance/current/validation.json](testdata/performance/current/validation.json)。

## 本机性能

环境：Apple M1 Max、64 GiB RAM、darwin/arm64、Go 1.22.12、GOMAXPROCS=10。
每项 500 ms × 5 样本，顺序运行取中位数。文件入口计入打开、读取、解码、重组、回调及排空。

64 条交错流每次共 23,920,640 字节 payload，每包 1460 字节，聚合回调块为 64 KiB。
轻量回调仅作原子字节计数；SHA-256 回调还逐流计算并核对摘要。同一行使用相同文件和输出契约。

| 64 条交错流 | 1 worker MB/s | 2 workers MB/s | 4 workers MB/s | 8 workers MB/s | 最佳加速 | 最佳 payload Gbit/s |
|---|---:|---:|---:|---:|---:|---:|
| 轻量回调 | 2161.38 | 3333.07 | 3482.99 | 3287.02 | 1.61× | 27.86 |
| 逐流 SHA-256 | 1103.02 | 1914.44 | 2691.29 | 3041.66 | 2.76× | 24.33 |

轻量消费建议先用 4 workers；较重的跨流计算可测试 8 workers。worker 数量不能线性换算吞吐。

| 单条流，同样总 payload/包大小 | 1 worker MB/s | 2 workers MB/s | 4 workers MB/s | 8 workers MB/s |
|---|---:|---:|---:|---:|
| 轻量回调 | 2721.91 | 3584.59 | 3574.97 | 3575.79 |
| SHA-256 | 1254.28 | 1405.43 | 1410.05 | 1401.01 |

单流只有一个 worker 处理 TCP 状态，收益来自读取与重组流水执行及维护策略；2 workers 已接近这组单流输入的最佳值。

同步入口的补充基准每次处理 4 MiB、每包 1024 字节 payload：

| 当前同步路径 | MB/s |
|---|---:|
| 单流顺序，默认可读模式 | 2198.68 |
| 单流逆序，默认可读模式 | 1286.68 |
| pcap 文件，默认可读模式 | 1498.94 |
| 1024 条交错短连接 | 756.12 |
| pcap 文件，Stream | 2247.58 |

带宽换算为十进制 `MB/s × 0.008`。输入为确定性合成文件，通常命中操作系统缓存；
不包含冷盘、网卡驱动、真实 ACK 比例、HTTP 业务、数据库或落盘开销。
当前多流文件处理约 **24.3–27.9 Gbit/s 有效载荷**，不能据此承诺 25 Gbit/s 真实网卡持续无丢包。

原始输出：[workers.txt](testdata/performance/current/workers.txt)、[core.txt](testdata/performance/current/core.txt)；
中位数：[workers-medians.json](testdata/performance/current/workers-medians.json)、[core-medians.json](testdata/performance/current/core-medians.json)。

## 使用与内存边界

```go
var bytes atomic.Uint64
var stats pcaputil.TCPReassemblyStats
err := pcaputil.OpenPcapFile("large.pcap",
    pcaputil.WithTCPReassemblyStream(64 << 10),
    pcaputil.WithTCPReassemblyWorkers(4),
    pcaputil.WithOnTrafficFlowOnDataFrameReassembled(
        func(_ *pcaputil.TrafficFlow, _ *pcaputil.TrafficConnection, f *pcaputil.TrafficFrame) {
            bytes.Add(uint64(len(f.Payload)))
        }),
    pcaputil.WithTCPReassemblyStats(func(s pcaputil.TCPReassemblyStats) { stats = s }),
)
// 检查 err、stats 和业务数据长度/哈希，再接受结果。
```

对应 Yak 选项为 `pcap_tcpReassemblyStream(65536)`、`pcap_tcpReassemblyWorkers(4)`、
`pcap_onFlowDataFrame(callback)`、`pcap_tcpReassemblyStats(callback)`，均由 pcapx 导出。

同一双向连接固定到一个 worker；该连接的数据回调顺序执行，不同连接可并发。
共享消费者状态需要并发保护，回调不得递归调用同一池的 Feed、NewFlow 或 Close。
EveryPacket 与异步数据回调之间没有同步先后保证。内建 HTTP 解析器也有自己的读协程。
多 worker 需要独占捕获 handle，不能与共享 capture cache 组合。

默认每 worker 有深度 2 的批队列，批次容量为 256 包/256 KiB，低流量刷新间隔为 1 ms。
预分配 arena 容量为 `workers × (queueDepth + 2) × batchBytes`，4 workers 默认共 4 MiB；
固定包记录位置、流对象、pcap 和消费者内存另计。队列满时阻塞上游。
EOF/取消发布尾批，等待所有已接受数据、回调和清理完成；回调不返回时排空也不会完成。

全池默认最多 65,536 个流，乱序数据共 64 MiB/65,536 个缓存段，每方向最多 8 MiB/8,192 段。
全局预算不随 worker 数增加。全局连接数满时尝试淘汰当前分片旧流，无法腾出容量则拒绝建流，
活跃流被资源限制关闭会报告错误。不是跨分片全局 LRU。
积压期间暂停空闲淘汰，避免将调度延迟当作 TCP 空闲；持续繁忙时仍受数量预算约束。

Stream 不保留完整可读副本，回调可以保留自己拥有的帧。
默认 Read/GetBuffer 模式仍允许晚读，未消费数据和完整方向聚合帧可随流增长，不能视为有界大流模式。
Stream 不能与内建完整 HTTP/TLS 解析组合。
可使用 `WithTCPReassemblyOptions` 调整资源限制；该选项设置完整配置，按传入顺序覆盖之前的选项。

## 畸形输入的处理约定

- TCP 序号使用仓库已有的 gVisor seqnum 回绕规则。新增默认 64 MiB 的前向序号跨度上限，
  `MaxSequenceGap` 可配置但必须小于 2³¹。恰好相差 2³¹ 的歧义报文、过远未来数据和 FIN 被拒绝，
  不推进接收游标，不填充缺口。纯 ACK 不推进游标，中途开始捕获的有 payload 流仍可建立。
- 同 ISN 的 SYN 重传不重置游标；已建立方向收到不同 ISN 的 SYN，或 SYN 与 FIN/RST 组合时拒绝。
  已初始化方向的 RST 必须匹配当前接收游标，避免旧/未来 RST 直接截断流；有效 RST 的 payload 不交付。
  未初始化方向缺少足够序号上下文，仍保留既有 RST 关闭语义。
- 首个可接受的 FIN 固定该方向结束位置。乱序 FIN、FIN 重传、跨回绕 FIN，以及与 FIN 重叠的长重传
  都不得延长流尾；FIN 之前的缺口仍必须补齐，结束位置之后的数据不会交付。
- 已交付字节不被重传改写。相同缓存起点保留先到前缀、允许补充更长后缀；不同起点按接收游标和序号
  处理。缓存重传/重叠字节发生冲突时保留确定性输出，但记录错误，调用者不能把该流视为无歧义成功。
  不保留全部历史字节来比对旧重传，也不承诺复现所有操作系统的 TCP 重叠策略。
- 错误的 TCP data offset、option 长度、截断 MPTCP、错误 IP 版本/头长度以及声明长度超出捕获的数据
  不进入重组状态。解码失败与用户回调 panic 分别计数，坏包之后继续处理合法报文。
  完整 Packet 路径检查 ErrorLayer，不能因解码器已暴露一个不完整 TCP 对象就接受其 payload。
- 合法未知 option、NOP/EOL、最大 TCP 头、窗口为零、ECN/URG/保留位、捕获 checksum offload、
  IPv4 长度为零的 TSO 记录均有兼容测试。当前不验证捕获 checksum，不实现 URG 带外交付或 MPTCP 连接级重组。
- IP 分片不当成完整 TCP 流拼接；本实现没有加入 IP 分片重组。IPv4/IPv6、VLAN、Raw 等支持的封装
  使用现有解码能力，不对未知封装或丢失的流头/流尾作完整性承诺。

错误输入会继续隔离处理，并由 `OpenPcapFile`/`Start` 的最终错误或 `TrafficPool.Err()` 报告。
只保留首次错误，避免异常洪泛无限累积错误对象。
多 worker 的 `Stats()`/最终 stats 回调额外提供 InvalidSegments、DecodeErrors、CallbackPanics、
ResourceLimitEvents、TruncatedCaptures、UnreassembledBytes/Segments、RejectedPackets、反压时间和设备统计。
`AccountingAvailable` 只在多 worker 时为 true。

`AcceptedPackets == ProcessedPackets` 表示队列任务都已尝试处理，不能替代错误检查或业务哈希校验。
实时抓包结束后读取 pcap dropped/interface dropped；报告丢包或无法取得统计时返回错误。
反压不能暂停网卡，pcap/驱动统计也不能补回上游已丢失的数据。

## 边缘与对抗测试

以下 342 个组合子场景覆盖同步/4 workers、Stream/晚读、IPv4/IPv6 及私有/完整 Packet 路径。
随机重叠另有 80 个确定性种子，期望字节直接来自独立源数组。

| 组别 | 明确覆盖的场景 |
|---|---|
| SYN | 同 ISN 重传、不同 ISN 注入、SYN+FIN、SYN+RST、SYN 携带数据 |
| RST / ACK | 过旧 RST、未来 RST、精确游标 RST、RST 携带 payload、远序号纯 ACK、keepalive |
| 序号 | 零序号、UINT32 回绕、重叠回绕、FIN 回绕、2³¹ 歧义、巨大前向缺口、巨大未来 FIN |
| 重传/重叠 | 已交付数据重传、同起点较长重传、嵌套重叠、相同结尾、一次跨多个缓存段、不同到达次序的冲突重叠 |
| FIN | 缺口前到达、裁剪后到长数据、裁剪已有缓存、尾后数据、长重传不能移除 FIN、不同 FIN 不能移动尾、旧 FIN |
| TCP 头 | offset 小于 5、offset 超过捕获、option 长度 0/1、option 越界、最后一字节缺少长度 |
| MPTCP | 末字节 option、缺少 subtype、超大 option、截断 DSS；均验证坏包后 SAFE 数据完整 |
| IP 头 | IPv4/IPv6 版本错误、IHL 太短/太长、total length 小于头、payload 截断、IPv6 头截断 |
| 兼容输入 | 未知 option、NOP、EOL 后垃圾填充、60 字节 TCP 头、零窗口、ECN/URG/保留位、checksum offload、TSO |
| 资源与隔离 | 2,000 个越界序号洪泛不污染游标、超长首包不初始化游标、预算归零、IP 分片不注入流字节、无关畸形 UDP 隔离 |

测试入口为 [reassembly_adversarial_test.go](reassembly_adversarial_test.go)、
[capture_adversarial_test.go](capture_adversarial_test.go)、[reassembly_fuzz_test.go](reassembly_fuzz_test.go)。
已有用例继续覆盖双向同端口、流 Index、并发提交、队列反压与取消排空、资源限制、HTTP 配对及回调异常。

当前源码通过完整 pcaputil/tests 测试、race、vet 和 `common/yakgrpc TestServer_PcapX`。
普通测试共 436 个通过的测试/子测试事件，其中上述对抗组合为 342 个；
分类计数和完整 JSON 日志摘要见 [test-summary.json](testdata/performance/current/test-summary.json)。

两项 fuzz 各运行 30 秒：重组参考模型 **366,696 次**，畸形包 **289,295 次**，合计 **655,991 次**，均通过。
覆盖随机分段/重叠/回绕与字节级报文变异；有限时长 fuzz 不能证明不存在所有未知输入缺陷。

4 workers 完成 **5 GiB + 17 字节**主流及三条各 40 MiB 并发流，全部 SHA-256 一致，
接受/处理均为 **167,685 包**，交付 **5,494,538,257 字节**。记录 **1,909 次反压**，
拒绝、未重组、解码错误、资源失败、InvalidSegments、回调异常均为零。
1–5 GiB 的 GC 后存活堆检查点未超过第一个检查点；这不等同于进程 RSS 峰值。

实际生成并回放 1 GiB + 17 字节 payload 的 pcap，哈希一致，GC 后存活堆检查点增长为 2,272 字节。
真实 lo0 捕获在 race 下交付 221,184 字节，接受/处理 62/62 包，哈希一致，pcap/interface dropped 为 0；
1/4 worker 的空闲捕获取消均通过。该回环测试没有进行 10/25 Gbit/s 持续网卡压力验收。

日志：[fuzz-overlap.txt](testdata/performance/current/fuzz-overlap.txt)、
[fuzz-packets.txt](testdata/performance/current/fuzz-packets.txt)、
[large.txt](testdata/performance/current/large.txt)、[live.txt](testdata/performance/current/live.txt)。

## 复现

在仓库根目录运行。实时用例需要本机抓包权限，只向本机 listener 发送可校验数据。

```sh
go test ./common/pcapx/pcaputil ./common/pcapx/pcaputil/tests -count=1
go test -race ./common/pcapx/pcaputil ./common/pcapx/pcaputil/tests -count=1
go vet ./common/pcapx/pcaputil ./common/pcapx/pcaputil/tests
go test ./common/pcapx/pcaputil -run '^$' -fuzz '^FuzzTCPReassemblyOverlap$' -fuzztime=30s -parallel=4
go test ./common/pcapx/pcaputil -run '^$' -fuzz '^FuzzTCPMalformedPacket$' -fuzztime=30s -parallel=4
PCAPX_LARGE_TEST=1 PCAPX_LARGE_WORKERS=4 go test ./common/pcapx/pcaputil -run '^(TestTCPWorkersLargeStream|TestLargePcapFileStream)$' -v -count=1
PCAPX_LIVE_TEST_IFACE=lo0 go test -race ./common/pcapx/pcaputil -run '^(TestTCPWorkersLiveIntegrity|TestLiveCaptureCancellation)$' -v -count=1
go test ./common/yakgrpc -run '^TestServer_PcapX$' -count=1
go test ./common/pcapx/pcaputil -run '^$' -bench '^BenchmarkTCPWorkers(SingleFlow)?$' -benchmem -benchtime=500ms -count=5
```
