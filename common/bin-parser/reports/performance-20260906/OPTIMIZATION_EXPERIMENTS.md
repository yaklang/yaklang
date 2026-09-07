# 路径预加载与单次字段投影对照

2026-09-06 · 第二轮离线实验 · 结论以实际计时和输出等价为准

## 结果

保留一处有测量收益的修改：[NodeToMap](../../utils.go) 在结构节点中复用已经计算的子节点值，不再递归计算第二遍。两协议当前样本的完整字段及 JSON 导出吞吐中位数提高 **约 12%–22%**；这是本次工作负载的结果，不是全库或任意协议的统一收益。

撤回预加载路径的临时 `PreparedRule` / `RuleTemplate` API：字段解析实际耗时只下降约 0.8%–2.1%，不能支持此前将约 71% 的路径调用 CPU 样本视为可回收耗时的归因。生产接口恢复原状；实验代码和测试保存在 [复现补丁](prepared-rule-experiment.patch)，没有删除原始捕获、旧接口或此前的规则文档缓存。

本次不新增依赖，不扩展 14 项延期协议，不修改客户端、aid 或公开流量。

## 环境和方法

硬件、系统、Go、manifest 与 [首轮评估](../../PERFORMANCE_ASSESSMENT.md) 相同：M1 Max、10 logical CPU、64 GiB、macOS 14.1.2、Go 1.22.12。仍是有其他应用负载的桌面，未独占 CPU、锁频或随机化执行顺序。两组实验分别冻结二进制，**只在各自同一个二进制内比较**，不将两个程序之间的时间差归因于某一修改。

| 实验 | 冻结程序 SHA-256 | 入口与统计 |
| --- | --- | --- |
| 预加载路径 | `d6f93b563daf6f8c437d65ee92c56f58dec0ca77c7f70805d0db4b20dabd3a03` | 原公开入口 / 显式 PreparedRule；字段及 JSON 600 ms、1/4/8/10 worker，各 3 次；封装 1x、1/4/10 worker，各 3 次 |
| 单次字段投影 | `c8c7fafd9202f96a00fabad6f87a7c90f67f87bfe1f3f68b7d4d2daf0e6645ac` | 同一公开 ParseBinary、同一调度和 JSON；只选择测试中冻结的旧遍历或新的 NodeToMap，600 ms、1/4/8/10 worker，各 3 次 |

全部配置正确的慢值均保留。每组中位数及最小–最大范围，n=3；不是置信区间。基准报告的 worker 与设置逐一匹配；封装 1x 使用每个 CPU 数独立启动进程，避开多 CPU 列表首次校准的歧义。样本加载、摘要检查、profile 定位和规则预热在吞吐计时外，每条消息仍创建独立解析树。

两协议一轮仍为 **11 条消息、1,421 输入字节、25,312 JSON 输出字节**。完整数值、每次原始结果、源文件摘要及单消息延迟记录见 [optimization-summary.json](optimization-summary.json)，没有把统计 op 当作单条消息。

## 路径假说：干预没有证实大的提速空间

预加载原型只在准备阶段读取 cwd；每条消息仍克隆 YAML 并构建独立树、配置和上下文。导入保持原有 ParseRule 语义。全 58,533 条记录比较了公开结果类型与顺序、树结构、位范围、元数据、原始字节、错误和读取位置；11 条应用消息还比较完整 JSON 字节，均通过。专用路径变更、配置隔离、并发及 race 检查通过。

但实际计时变化很小：

- 字段解析中位耗时下降 0.8%–2.1%；每条仅少 10 次分配。
- 封装：1 worker 从 26.935 s 到 26.777 s；4 worker 从 8.918 s 到 8.930 s，略慢；10 worker 从 6.723 s 到 6.565 s，原型与旧入口范围明显重叠。
- JSON：1 worker 中位耗时下降约 5.7%，4/10 worker 不到 1%，8 worker 略慢约 0.4%；不能选择其中最好一组推导全局增益。

同一冻结程序再次收集两个独立短 profile：旧入口约 63.25% CPU 样本在 `syscall.syscall` / 路径 Stat 链；预加载后该路径不再出现于摘要，但 profiled ns/op 仅从 3.933 ms 到 3.739 ms。**采样归属不等于可回收的 wall time。** 本轮没有证实为何该环境的采样占比与干预计时不一致；不能据此断言是 Go / macOS 缺陷或二进制符号错配。先前 71.43% 的原始 profile 保留，但撤回其足以确定主要提速方向的结论。

证据：[应用](prepared-application.txt)、[封装 1](prepared-envelopes-cpu1.txt)、[封装 4](prepared-envelopes-cpu4.txt)、[封装 10](prepared-envelopes-cpu10.txt)、[旧入口 CPU](prepared-cpu-baseline.txt)、[原型 CPU](prepared-cpu-candidate.txt)、[验证摘要](prepared-verification-summary.txt)。

## 保留的优化：避免重复递归投影

旧结构分支先执行 `d := NodeToMap(sub)`，确认非 nil 后又执行一次 `NodeToMap(sub)`。当前改为直接保存 `d`。没有跳过字段、关闭元数据、改变数值类型、执行自定义 out、截断列表或缓存可变结果。其余解析、路径、输入边界及节点生命周期不变。

| worker | 旧版每轮 ms 中位数（范围） | 新版每轮 ms 中位数（范围） | 新版消息/秒 | 新版输入 Mbps | 吞吐中位数变化 |
| --- | --- | --- | ---: | ---: | ---: |
| 1 | 3.902（3.896–3.908） | 3.407（3.378–3.447） | 3229 | 3.337 | +14.5% |
| 4 | 2.254（2.250–2.264） | 2.014（1.917–2.033） | 5461 | 5.644 | +11.9% |
| 8 | 2.062（2.060–2.066） | 1.693（1.684–1.694） | 6496 | 6.714 | +21.8% |
| 10 | 2.042（2.026–2.051） | 1.672（1.659–1.674） | 6579 | 6.799 | +22.2% |

单 worker 每轮累计分配从 **2,513,318 B / 45,190 次** 降到 **2,261,000 B / 37,713 次**，分别少约 10.0% 和 16.5%。10 worker 对应约少 10.1% 字节和 16.5% 次数。仍包含完整构树成本，远未消除配置/节点分配；不是 RSS 下降的测量，也没有改善纯封装基准的证据。

同一新程序独立测量单消息延迟：显式 `GOMAXPROCS=1`，各入口每轮 2,200 条、3 轮。字段 + JSON P50/P95/P99 的三轮中位数为 **166.9 / 1,260.0 / 2,828.3 µs**，P99 范围 2,754.3–2,859.3 µs，三轮观察到的最大值 4.457 ms。每轮累计 GC pause 1.297–1.348 ms、29 次 GC。它不是新旧同时配对的 P99 实验，不能把首轮与本轮百分位差直接算成优化收益。

同阶段一次未指定 GOMAXPROCS 的延迟探索单独保留，不混入单 CPU 统计；其中 10.805 ms 慢值也未删除。原始记录：[同程序吞吐对照](single-pass-application.txt)、[显式单 CPU 延迟](single-pass-latency-cpu1.txt)、[默认并发探索](single-pass-latency-default-exploratory.txt)。

## 正确性与变更边界

[新增测试](../../node_to_map_single_pass_test.go) 使用冻结的原遍历为对照：

- 全部 552 份捕获经摘要与记录数校验；58,533 条记录逐条解析、检查预期拒绝或完整字节消费，成功节点的具体类型、值与完整 JSON 导出保持相等。4 个 worker 分别检查 14,634 / 14,633 / 14,633 / 14,633 条；最慢约 7.02 s。这是导出等价与封装检查，不是所有协议的应用语义全量回归。
- 两协议 11 条应用消息含旧、新 GET，原始字节与完整字段/元数据 JSON 一致。
- 独立向量覆盖空列表与无结果列表的区别、重复字段名保留原来的覆盖语义、重复列表项顺序、raw 与数值类型、非字节对齐字段、8 层结构、自定义 out 不执行及保留结果隔离。
- 联合 Memcached、Cassandra、AllJoyn、TLS 空证书列表、消费长度差分等定向测试通过；原路径接口的并发、变更工作目录和 race 回归另行检查。完整输出见 [定向验证](single-pass-verification.txt) 与 [race 日志](single-pass-race.txt)。

测试向量首轮曾把 base 的 `isList` 常量误用于 stream parser 的 `list` 配置，导致预期列表被构造成结构；修正的是新增测试构造器，未降低断言或修改生产列表语义。未运行整个仓库或整个库的完整套件，未升级任何协议支持标签。首轮报告中历史 P0 标签检查的不一致仍单列，不掩盖为全量通过。

## 复现

从包含当前未提交实现的工作树构建，随后进入包目录：

```sh
go test -c -o /tmp/binparser-single-pass-20260906.test ./common/bin-parser
cd common/bin-parser
/tmp/binparser-single-pass-20260906.test -test.run '^$' -test.bench '^BenchmarkCurrentCorpusApplicationJSON(RepeatedReference)?$' -test.benchtime=600ms -test.count=3 -test.cpu=1,4,8,10 -test.benchmem
GOMAXPROCS=1 BINPARSER_EVALUATE=1 /tmp/binparser-single-pass-20260906.test -test.run '^TestCurrentCorpusApplicationLatency$' -test.count=3 -test.cpu=1 -test.v
/tmp/binparser-single-pass-20260906.test -test.run '^(TestNodeToMapSinglePassSemantics|TestProtocolCorpusNodeToMapSinglePass.*|TestProtocolCorpusMemcachedFields.*|TestProtocolCorpusCassandraFields.*|TestProtocolCorpusTLSCertificateInlineFramingAndBoundaries|TestProtocolCorpusAllJoyn.*|TestProtocolCorpusConsumedLengthPublicDifferential)$' -test.count=1 -test.v -test.timeout=3m
```

路径原型仅供实验：`prepared-rule-experiment.patch` 从当前源文件恢复被测原型和专用测试，同时暂时撤销本轮单次投影及相关测试，已通过 `git apply --check`。只能在包含当前未提交基线的**隔离副本**中应用；不要在用户工作树直接切换。原程序在 `/tmp/binparser-prepared-20260906.test`，临时文件可能清理；持久补丁和输出保留了复现材料。

应用对照命令使用 `^BenchmarkCurrentCorpus(Prepared)?Application(Fields|JSON)$`，600 ms、3 次、1/4/8/10 CPU。封装使用 `^BenchmarkCurrentCorpus(Prepared)?Envelopes$`，每个 1/4/10 CPU 单独进程、1x、3 次。剖析使用各 JSON 入口、3 s、1 CPU，分别指定 `-test.cpuprofile`。

## 对带宽、客户端与 AI 的影响

最新导出约 6.80 Mbps 是固定两协议应用输入字节率。首轮全记录封装约 11.05 Mbps 仍是另一个独立工作负载，不能相加或推导实时无丢包上限。没有新增 TCP 重组、流状态、队列负载、RPC/UI 或 LLM 计时，不报告千兆容量或已交付 AI 客户端。

这处优化降低结构化证据导出的成本；aid 连接状态、未知事实处理、证据定位及模型评估设计仍按 [主报告](../../PERFORMANCE_ASSESSMENT.md)。真实客户端体验和模型正确率不能由解析微基准替代。
