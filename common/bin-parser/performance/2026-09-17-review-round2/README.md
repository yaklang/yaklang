# PR #5013：2026-09-17 首轮定向优化复测

本轮落实评审中的调度参数解耦、H2 HPACK 单次结构扫描、layout-only 去字段树、历史成本缓存及超大项预判、HTTP 增量分隔符查找、库输出缓冲。完整解析接口和字段输出保持不变。

## 版本、环境与口径

- 基线：`2de6e21e4e8aabcb343440bbe4dc7e023e6594f9`，包含评审固定版本之后其他 worker 的 LDAP/PostgreSQL/WebSocket 提交。
- 优化代码：`59de5abdef`；后续文档提交只保存验证记录。
- Apple M1 Max，macOS 14.1.2，Go 1.22.12，darwin/arm64。共享开发机，不是隔离实验室；不在基准期间运行本任务的编译、测试或 profile。
- 前后版本分别编译 test binary，基线复制**同一份基准代码**，按 A/B、B/A 顺序交替运行五轮；每项 300 ms，GOMAXPROCS=4。表中报告中位数与最小/最大值。时间下降与吞吐提升不是相同百分比。
- 在观察正式结果前确定复查规则：同语义重复样本中位数退化超过 5% 必须复查。单轮调度矩阵仅用于探索，不能判断小幅性能回归。
- synthetic 混合语料：24 TCP flows，HTTP/MQTT/TLS，512-byte bodies，128 rounds；每批完整解析 3,080 条消息。它包含 worker 初始化/EOF drain，属于有限文件 warm-cache replay，不代表持续网卡线速。
- HTTP 分片局部基准：约 16 KiB 头部/尾部，逐字节增量输入；一 op 是整条消息所有分片的分帧工作。H2 layout 一 op 是一个帧；HPACK 一 op 是一个完整块；会话基准一 op 是一份完整短会话回放。
- 保存基准：完整字段解析 + 每批新建、Flush、Close、删除 pcap。无 fsync，测到的缓存文件写入收益不代表磁盘耐久带宽。CLI 原来已有缓冲，本次改的是库 `WithOutputFile`。

## 实测结论

五轮中位数（完整原始数据见 [tables.md](tables.md)、[raw](raw)）：

| 场景 | 基线 → 优化耗时 | 耗时变化 | 分配变化 |
|---|---:|---:|---|
| H2 HEADERS layout | 464.4 → 41.49 ns/frame | −91.1% | 1,648 B / 4 alloc → 0 |
| H2 32 项 SETTINGS layout | 5,221 → 125.4 ns/frame | −97.6% | 13,169 B / 70 alloc → 0 |
| HPACK 4 / 128 headers | 2,705 → 2,633 / 84,803 → 82,185 ns/block | −2.7% / −3.1% | 不变 |
| 64 headers 会话历史保留 | 31,285 → 15,439 ns/event | −50.7% | 不变，减少重复遍历 |
| 超预算历史拒绝 | 15,761 → 3,576 ns/event | −77.3% | 23,720 B / 134 alloc → 0 |
| HTTP 16 KiB header，逐字节输入 | 2.841 → 0.215 ms/message | −92.4% | 基本不变 |
| HTTP chunked trailers，逐字节输入 | 2.363 → 0.229 ms/message | −90.3% | 基本不变 |
| 完整混合解析 + 库保存 | 22.114 → 10.059 ms/batch | −54.5% | 增加约 256 KiB 固定缓冲 |
| HTTP/2 完整短会话回放 | 412.526 → 396.826 µs/batch | −3.8% | 2,757 → 2,705 alloc |
| MySQL 完整短会话回放 | 111.162 → 113.999 µs/batch | +2.6% | 不变 |

完整混合 full/full-history 的 1/2/4 workers 六种场景：耗时变化范围 −4.70% 至 +2.08%，没有触发预定的 5% 退化复查线。**这些场景多数变化很小，不能宣传为整体解析吞吐大幅提升。** 原始每批/每消息和 CPU 数据均保留。单轮矩阵也有明显调度噪声，例如 workers=1/procs=4；不以单点判断回归或推荐唯一配置。

调度矩阵覆盖混合 24 TCP flows、单 TCP flow（129 messages/batch）和既有 DNS UDP corpus（4 messages/batch）× workers 1/2/4 × GOMAXPROCS 1/2/4/8，前后均验证完整结果计数。DNS 小文件和单流增加 worker 可能更慢，因为初始化与 drain 占比高；UDP 尚未并行化。该矩阵证明参数可以独立变化，不等同于稳态 DNS pps 扩展性测试。

独立 CPU profile（不纳入性能样本）见 [profile-top.txt](raw/profile-top.txt) 和 [profile-application.txt](raw/profile-application.txt)。该有限批次在 Darwin 上有大量 runtime/pthread/调度采样，GC/分配也可见；不能仅凭这些数据认定某把锁是新瓶颈。下一步应补长期运行、按流倾斜和 block/mutex profile，再决定共享布局或调度结构改造。

## 行为与边界

- `pcap-inspect -workers` 只设 TCP worker；新增 `-gomaxprocs=0` 保留运行时/环境设置，正数显式覆盖，负数报错。JSON 报告记录 Workers/GOMAXPROCS/GoVersion。
- H2 两种路径共用一个 validator，layout-only 跳过字段和 metadata 分配。HPACK 普通入口仍扫描，session decoder 复用一次扫描的边界；没有可从包外绕过验证的入口。
- 历史项缓存成本，超预算消息在复制之前计入 received/evicted。预算仍为近似值；历史中的 byte slices 按逻辑长度计价，避免把输入的富余容量误算成保留成本。运行中 session/capture 的容量预算不变。缓存成本只存在于内部历史项，不扩大公开事件。
- HTTP header、chunk-size line、trailers 均保留分隔符重叠字节。完整消息、停止与重新识别时重置方向游标；`net/http` 的 CL/TE、请求方法关联等语义继续保留。
- `WithOutputFile` 使用 256 KiB 有界缓冲；Flush 失败仍 Close，两个错误都传播。同步背压语义保留。

## 正确性验证

- `go test ./common/bin-parser/... ./common/pcapx/pcaputil ./common/pcapx/cmd/pcap-inspect -count=1 -timeout=10m` 全部通过。
- H2/HTTP/Inspector/写盘/CLI/会话定向测试与 race 全部通过。macOS race 链接器有既存 LC_DYSYMTAB warning，运行通过，无 race 报告。
- 10 份语料、141 个完整公开事件逐份 SHA-256 一致：包含所有 Fields、Metadata、Structured、Raw、时间戳、Session，而不只对比消息数。范围为 8 份既有回放 corpus + HTTP/2/MySQL 会话；其中既有 TLS/CONNECT 缺口保持原分类。不能据此外推全部协议语义。
- 新测试覆盖 H2 类型/flags/stream/长度验证一致性、有效 layout 零分配、历史覆盖/超预算/到达顺序/输入修改隔离、HTTP 多切片分帧和 pipelining、Flush/Close 双错误、调度参数保留及恢复。

## 复现

在两份 worktree 上用本提交的三个 benchmark 文件构建；仅把测试文件复制到基线，不能复制生产实现。`protocol_review_export_test.go` 用于完整输出差分。

```sh
go test -c -o /tmp/stream.test ./common/bin-parser/parser/stream_parser
go test -c -o /tmp/pcap.test ./common/pcapx/pcaputil
# 在对应包目录运行二进制，保持相对 fixture 路径可用。
/tmp/stream.test -test.run '^$' -test.bench '^BenchmarkHTTP2Review$' -test.benchmem -test.benchtime 300ms -test.cpu 4 -test.count 5
/tmp/pcap.test -test.run '^$' -test.bench '^(BenchmarkProtocolReview|BenchmarkLiveProtocolSessions|BenchmarkProtocolReviewRecording)$' -test.benchmem -test.benchtime 300ms -test.cpu 4 -test.count 5
/tmp/pcap.test -test.run '^$' -test.bench '^BenchmarkPcapBinParser$/mixed=true/full' -test.benchmem -test.benchtime 300ms -test.cpu 4 -test.count 5
/tmp/pcap.test -test.run '^$' -test.bench '^BenchmarkPcapBinParser$/mixed=true/full$' -test.benchmem -test.benchtime 200ms -test.cpu 1,2,4,8
/tmp/pcap.test -test.run '^$' -test.bench '^BenchmarkProtocolReviewScheduler$' -test.benchmem -test.benchtime 200ms -test.cpu 1,2,4,8
```

正式 A/B 原始日志记录每次完整命令。Go benchmark 的 `-2/-4/-8` 后缀以及子基准中报告的 `gomaxprocs` 是实际调度值，无后缀表示 1。初测曾把外层调度值写入名称；发现 Go 的 -cpu 在子基准内切换后修正标注并重跑，初测不混入下列结果。

## 尚未实施的后续阶段

HTTP 共享已验证 layout、UDP 有界并行、compact fields、full-result history cache、异步 recorder/dispatcher 仍属后续设计；本轮不改变工作量或采用 deferred 模式制造完整解析成绩。物理网卡长跑、丢包、p99、另一 CPU/平台及同机 TShark 同语义对照尚未验证，本文不声称达到 Wireshark 或某个网卡线速。
