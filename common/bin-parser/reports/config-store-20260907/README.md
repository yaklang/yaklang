# 小配置存储：字段等价、吞吐与有限队列对照

2026-09-07。保留这项私有实现优化，不升级协议支持状态，不增加依赖。指定的 14 项仍按 [本次范围](../../ACTIVE_SCOPE.md) 延期；本轮没有扩展其他协议、客户端或 aid。

后续验收修正见 [第五轮复验](../acceptance-20260907/README.md)：四组旧测试/评分不一致已修正，完整复验只剩历史 P0 状态门槛；本报告的冻结程序、测量、原始失败及统计保持不变。

## 结论

在本机当前样本上，小配置存储替换使全记录封装、两协议字段解析的吞吐中位数提高约 **29%–30%**，完整 JSON 导出提高约 **27%–28%**。全记录封装的累计分配字节减少约 **23.3%**。10 worker 下，候选版封装解析约 **11,146 条/秒、13.70 Mbps**，两协议字段与完整 JSON 导出约 **8,425 条/秒、8.71 Mbps**。两者属于独立负载，不能相加或视为端到端实时容量。

4 worker、64 条队列、6 Mbps / 10 秒回放中，记录丢弃率中位数从 **20.073%** 降至 **0.465%**，但三轮仍全部有丢弃；2 Mbps / 30 秒下，候选版也有一轮丢弃 84 条。因此，优化减轻了解析成本和已测过载，**没有证明持续无损带宽，更没有证明千兆逐包解析或真实客户端流畅**。

全部 58,533 条封装记录及 11 条应用消息，旧/新程序各两轮导出摘要一致；相关 race 检查通过。但随后执行的 bin-parser 包及子包完整回归失败，可见的五个顶层失败项均在旧程序复现。不能将本次等价验证当作“全库全部通过”。

## 改动与对照身份

原配置后端为 `omap.OrderedMap[string, any]`；每个小配置会单独构造有序 map 对象、锁、hash map 和 key slice。新 [configStore](../../parser/base/config_store.go) 仅用于 [BaseKV](../../parser/base/node.go) 的私有存储：

- 内置 8 项连续空间；超过 16 项后建立查找索引。小对象减少分散分配，大上下文仍有索引。
- 保留插入顺序、覆盖顺序、删除后重新插入、存在的 nil 与不存在值，以及浅层值共享。
- `ForEach` 在锁内快照键和值，解锁后调用回调；保留重入修改和提前结束语义。
- 不共享解析树，不池化可变状态，不删除 `Config.SetItem` 的 option 历史，不改回滚、规则路径或公开 API。
- 首次溢出内置空间后清除旧引用；删除元素时清除尾部引用。保留各次 KV 操作的锁，不宣称原有多步配置写入突然成为原子事务。

基线仍为 `wip/optimize-protocol-parse`，HEAD `0b3ed685b1b462dfc7eb342c2b754313a5e6f9c7` 加现有未提交工作树。#5022 和 #5023 已先合并；直接 checkout HEAD 无法复现当前全部实现。

| 冻结对象 | SHA-256 |
| --- | --- |
| 旧后端 `/tmp/binparser-config-baseline-20260907.test` | `9a0b2c87a396f51b09cbad1fd4269a23b30cafca63d117fb1a507031783c9589` |
| 新后端 `/tmp/binparser-config-compact-20260907.test` | `9e23a790bf5a6a2454653c47dc8c492585c32d59541f8f121db4e2162b0b6bbd` |
| 旧 `node.go`，见 [源文件归档](baseline-node.go.txt) | `11451ac34797c1299e34b91ca4f006610cabdb400db65a597c28f6af39fac8f5` |
| 新 `parser/base/node.go` | `35d9af67f35e0b2401c0c646018e0366d3a9595f7b99823b9101f02ffb11546b` |
| 新 `parser/base/config_store.go` | `4ac74d6b6e31b3b2d1853d830af07ab2046999a6c67d2f1686a22a40ba233835` |
| 导出摘要测试 | `c05ba411ea15681f30d74da01bcae673f90bf4a6bb2509bf223c9526e06a602d` |
| corpus manifest | `870350c644462dffd3462ac8b71adcfff1fe725c19ace6d0d39765b1ab9181ca` |

这是两个独立编译的非 race 程序，不是同程序运行时切换实现。旧程序已包含摘要测试与未接入 BaseKV 的存储原型；新程序额外完善原型溢出清理及对应测试。真正接入解析路径的差异仅为新后端及四个配置构造点。归档与 [恢复旧后端的窄补丁](restore-ordered-backend.patch) 用于独立副本中的重新对照，不保证重建出原二进制相同 SHA；不得在当前脏工作树里直接覆盖文件。

环境：Apple M1 Max，10 CPU、64 GiB，macOS 14.1.2，Go 1.22.12，darwin/arm64。本地非独占桌面，未锁定频率。36 次正式吞吐程序和 12 次回放程序均串行；每个配置的顺序为旧/新、新/旧、旧/新。编译、race 和完整回归均不与正式性能测量重叠。初始 [存储微基准探测](store-pilot-with-overlap.txt) 与早期 race/编译可能重叠，明确排除在正式性能结论之外。

## 字段与行为验证

[导出摘要测试](../../protocol_corpus_export_digest_test.go) 对每条输入固定记录 ID、rule、entry、最终读取位置；成功时记录完整 `NodeToMap`、附加元数据及具体值类型/空值形状，预期失败时检查拒绝合同并记录完整错误。每个工作负载固定四个分区；每个程序重复两轮，8 个分区摘要每轮都相等。[旧日志](baseline-export-digest.txt)、[新日志](compact-export-digest.txt) 与 [summary.json](summary.json) 保留具体摘要。

摘要只证明这些输出未变，不单独证明协议语义正确；58,533 条封装记录也不是 58,533 条完整应用消息。独立字段断言仍是正确性依据。

- [存储层检查](../../parser/base/config_store_test.go)：与原 OrderedMap 进行 4,000 次确定性混合操作对照，覆盖索引切换、覆盖/删除/重新插入、nil、浅共享、回调快照、并发及引用释放。初始 [race](store-race.txt) 通过；接入后 [base 全部测试 race](base-race.txt) 通过，4.572 s。
- [公共路径 race](public-path-race.txt)：选择并发、隔离、事务、回滚、缓存、复制、快照，以及六组字段证据和清单变异检查。主包通过，50.254 s；parser/base/stream_parser 对应检查也通过。Apple `LC_DYSYMTAB` linker 警告原样保留。
- 性能计时之后执行 `go test ./common/bin-parser/... -count=1 -timeout=5m`。主包失败，191.912 s；parser、base、stream_parser、protocol-impl 子包通过，msrdp 无匹配测试；另四个子包无测试文件。该检查不是整个 Yaklang 仓库全量，也不是全库 race。

### 完整回归中的遗留失败

[完整命令日志](library-regression.txt) 含工具的轻微输出截断标记，不能当作逐项完整日志。随后将日志可见的五个顶层失败项在两个冻结程序中分别重跑，保存 [未截断的失败对照](failure-comparison.txt)，两程序均退出 1，失败项和错误一致：

| 检查 | 现有不一致及处理边界 |
| --- | --- |
| `TestP0RoadmapCovered` | LDAP、MySQL、PostgreSQL、SMB3 目录仍为 partial；不升级标签或放宽历史 G8 门槛 |
| `TestP1BranchRows` | 旧 RMI ping 断言要求 Message 节点，call 向量只给序列化头；当前显式 JRMP phase/完整消息合同不满足这些旧预期 |
| `TestP1ScorecardsCovered` | RMI/JRMP 声称 schema 20，而 YAML-only 审计在当前 native rule 上计 0；需证据对齐，不能增加名称豁免 |
| `TestProtocolCorpusPR5023CorrectedCompanionsEveryRecordAndBoundary` | `gen-winrm-http-valid` 的 `<s:Env/>` 无命名空间绑定。当前 manifest、拒绝合同和专门 WinRM 测试已保留该负样本，并另有 `gen-winrm-identify-valid`；旧汇总测试仍把前者当作应用正样本 |
| `TestSPNEGOScoresMatchCurrentRuleScope` | 尾部仍要求 GSS-API 的 corpus 合同 HTTP outer-only；当前合同已是 `application-layer/gssapi.yaml` / `GSSAPIHTTP`，与专门 GSS-API 字段测试一致 |

这证明上述失败不是本次后端替换新引入的差异，不证明不存在其他问题。本轮没有修改这些测试、原始捕获、评分卡、目录状态或协议实现；整体 goal 不据此宣布完成。后续应按真实字段与拒绝合同修正旧验收材料，不删负样本、不让无效 XML/不完整消息变成成功。

## 正式吞吐：相同工作量与完整输出

公开 `ParseBinary` 入口，独立树，规则预热及样本加载在计时外。`Envelopes` 每 op 为 58,533 条、8,993,088 输入字节，仅封装；应用项每 op 为两协议 11 条、1,421 输入字节，profile 和边界预先给定；JSON 项另外导出完整字段及元数据，不计 RPC、磁盘、重组或模型。

每组 n=3，下表为中位数（最小–最大）；范围不是置信区间，不删除慢值。封装 `benchtime=1x`，每次进程只指定一个 CPU 配置，以实际报告的 workers 校验；应用 `benchtime=600ms`。吞吐提升按旧/新耗时中位数之比计算，不等同于耗时下降百分比。

| 负载 | worker | 旧每轮耗时 | 新每轮耗时 | 吞吐提升 | 新输入 Mbps |
| --- | ---: | --- | --- | ---: | ---: |
| 封装 | 1 | 26.771 s（26.679–26.792） | 20.700 s（20.660–20.705） | 29.3% | 3.476 |
| 封装 | 4 | 8.806 s（8.779–9.385） | 6.780 s（6.738–6.801） | 29.9% | 10.611 |
| 封装 | 10 | 6.775 s（6.641–6.798） | 5.252 s（5.068–5.305） | 29.0% | 13.700 |
| 应用字段 | 1 | 2.776 ms（2.773–2.789） | 2.130 ms（2.129–2.141） | 30.3% | 5.336 |
| 应用字段 | 4 | 1.505 ms（1.503–1.506） | 1.159 ms（1.149–1.160） | 29.8% | 9.810 |
| 应用字段 | 10 | 1.330 ms（1.328–1.348） | 1.027 ms（1.025–1.038） | 29.5% | 11.068 |
| 字段 + JSON | 1 | 3.319 ms（3.312–3.479） | 2.615 ms（2.610–2.629） | 26.9% | 4.347 |
| 字段 + JSON | 4 | 1.861 ms（1.844–1.867） | 1.454 ms（1.452–1.460） | 28.0% | 7.819 |
| 字段 + JSON | 10 | 1.652 ms（1.647–1.679） | 1.306 ms（1.300–1.306） | 26.5% | 8.706 |

1 worker 的分配中位数（op 是整轮）：封装 19,484,500,856 → 14,939,868,360 B，397,518,379 → 313,114,626 次；应用字段 2,039,173 → 1,618,194 B，32,403 → 26,853 次；JSON 2,261,148 → 1,840,228 B，37,714 → 32,164 次。它们都是累积分配，不是常驻需求或泄漏指标。

完整数值、来源行与范围见 [summary.json](summary.json)，原始运行见 [封装](envelope-benchmarks.txt) 与 [应用](application-benchmarks.txt)。不将这轮新旧程序数据与上轮单次投影实验混算加速比。

## 有限队列：改进成立，无损容量仍未证明

使用未变的 [回放器](../../protocol_corpus_replay_test.go)：4 份捕获，38 条记录/循环，其中 11 条应用消息；3,897 捕获输入字节、33,820 JSON 输出字节/循环。按输入字节累计安排到达时间，4 worker、64 条队列，非阻塞准入，满队列记丢弃；完整封装、显式应用字段及 JSON 均执行。预提取边界，无自动分类、TCP 重组、真实流状态、RPC/落盘/UI/模型调用，没有向外部主机发包。

12 轮、累计 240 秒计划窗口，共 877,644 次重复输入、89,998,572 B，47,627 次记录丢弃。它们是 12 个独立阶段，不是连续 240 秒；每轮 73,137 次输入、7,499,881 B。输入/完成/丢弃的字节数、记录数及逐帧计数均校验守恒，全部 parse_errors=0。延迟从计划到达时刻计算，含投递迟到、排队和服务；P99 只覆盖接受的记录，不能替代丢弃率。

| 输入与窗口 | 后端 | 三轮无丢弃次数 | 丢弃率中位数（范围） | 总延迟 P99 中位数（范围） |
| --- | --- | ---: | --- | --- |
| 2 Mbps / 30 s | 旧 | 1/3 | 0.0123%（0–0.1025%） | 4.611 ms（3.563–4.706） |
| 2 Mbps / 30 s | 新 | 2/3 | 0（0–0.1149%） | 2.710 ms（1.982–4.269） |
| 6 Mbps / 10 s | 旧 | 0/3 | 20.0733%（19.4799–23.2167%） | 14.760 ms（14.452–16.603） |
| 6 Mbps / 10 s | 新 | 0/3 | 0.4649%（0.4239–1.2319%） | 5.471 ms（4.884–10.304） |

2 Mbps 三轮丢弃，旧为 9 / 75 / 0，新为 0 / 84 / 0；候选版仍存在慢轮，不能宣称稳定性已解决。6 Mbps 新版每轮仍丢弃 310–901 条。过载下存活的协议组合可变化，不将完成字节率推断为完整原始组合的无损容量。投递与解析仍在同进程，不将迟到直接归因为 GC、OS 或解析器某一个原因。

2 Mbps / 30 秒各三个进程的 CPU 等效核数 `(user+sys)/real` 中位数，旧 1.489（1.467–1.511），新 1.150（1.126–1.160）；过程含启动/加载/日志及回放。峰值 RSS 旧 94.48 MiB（93.89–95.05），新 94.44 MiB（91.84–96.94），范围重叠，**没有明显 RSS 改善证据**。每轮累积分配中位数约 28.72 → 22.23 GB，GC 次数 1,604 → 1,267；接受的记录并非每轮完全相同，不当作精确等工作量分配比例。详细时间、RSS、队列和逐帧计数见 [原始回放](replay-comparison.txt)。

## 复现与审核

从仓库根目录重新生成统计，只读原始日志并输出 JSON：

```sh
python3 common/bin-parser/reports/config-store-20260907/summarize.py
```

脚本检查正式运行数量、实际 worker、字节/消息数量、逐帧计数守恒、零解析错误及两程序各两轮导出摘要相等；不会隐藏失败的正确性检查或把回放 PASS 解释成无丢弃。

构建当前源码使用新的临时文件名，进入包目录后执行；原冻结二进制只在本地 `/tmp`，可能被系统清理。重测旧后端需在包含相同未提交实现的独立副本中应用归档窄补丁，不修改当前工作树。以下是单个配置的测量命令，正式实验的完整 CPU、重复与交替顺序已写入原始日志。

```sh
go test -c -o /tmp/binparser-config-remeasure-20260907.test ./common/bin-parser
cd common/bin-parser
/tmp/binparser-config-remeasure-20260907.test -test.run '^$' -test.bench '^BenchmarkCurrentCorpusApplication(Fields|JSON)$' -test.benchtime=600ms -test.count=3 -test.cpu=4 -test.benchmem
/tmp/binparser-config-remeasure-20260907.test -test.run '^$' -test.bench '^BenchmarkCurrentCorpusEnvelopes$' -test.benchtime=1x -test.count=3 -test.cpu=4 -test.benchmem
BINPARSER_EVALUATE=1 /tmp/binparser-config-remeasure-20260907.test -test.run '^TestProtocolCorpusExportDigest$' -test.count=2 -test.cpu=4 -test.v -test.timeout=3m
BINPARSER_EVALUATE=1 BINPARSER_REPLAY_MBPS=2 BINPARSER_REPLAY_DURATION=30s /usr/bin/time -l /tmp/binparser-config-remeasure-20260907.test -test.run '^TestCurrentCorpusBoundedReplay$' -test.count=3 -test.cpu=4 -test.v -test.timeout=2m
```

客户端及 aid 结论维持 [主评估](../../PERFORMANCE_ASSESSMENT.md)：现有字段可成为按需证据工具的基础，但旧 PCAP 工具尚未接入本次 profile/状态/来源，实际模型正确率、模型时延和 UI 体验未测。性能优化不是 AI 产品集成，也不替代真实捕获链路容量验证。
