# 解码链路与 Yak 引擎复用：分阶段对照

2026-09-07。保留两阶段优化，不新增依赖、不修改协议规则或样本、不扩展 [延期的 14 项](../../ACTIVE_SCOPE.md)。本轮未实现分层自动识别、TCP 重组、流亲和、客户端或 aid 集成。

## 结论与范围

在相同 11 条 Memcached / Cassandra 消息、1,421 B 输入及完整字段输出下，长窗口（2 s、每组 n=3）的单 worker 字段解析吞吐提高 **17.4%**，完整 JSON 提高 **14.4%**；10 worker 对应提高 **16.1%** 和 **11.5%**。单 worker JSON 的链路阶段提高 7.4%，Yak 阶段在链路版基础上再提高 6.5%，两阶段按比值相乘，不直接相加。

10 worker 完整 JSON 从 8,328 增至 9,284 消息/s，对应应用输入字节率 8.61 → 9.59 Mbps。这里只计预先指定规则、入口和完整消息边界的离线解析，不能当作网卡抓包带宽、无丢包容量或 UI / AI 端到端性能。

三个独立冻结程序的全部 58,533 条封装记录、11 条应用消息导出摘要一致；受影响协议族定向回归、parser 三包回归与选定 race 检查通过。未执行整个 bin-parser 或 Yaklang 的全量测试，不据此宣称所有应用协议、历史状态门槛或整个 goal 完成。

## 实现

### A：解码链路

- [configStore](../../parser/base/config_store.go) 将配置值写入与重放历史追加合并在一次锁内，减少重复查找与加解锁，同时避免并发追加丢失历史。仍保留原来的重放闭包、插入顺序、存在 nil 的语义，以及非法 option 类型写入后再报错的行为；不在锁内执行重放回调。这不等于将整棵树变成可并发修改对象。
- [newNodeTree](../../parser/base/node.go) 为没有长度、分隔符等选项的裸类型增加快路径，省去三层字符串切分；列表后缀、空类型、原始 Origin 和配置写入顺序保持一致。复杂描述仍走原解析器。
- 没有删字段、缩短列表、关闭元数据，亦未池化可变结果树。

### B：Yak 链路

[operator_worker.go](../../parser/stream_parser/operator_worker.go) 为受限桥接程序复用引擎、库绑定和已经装载的原始 Yak 程序。借出的执行器独占使用；每次创建独立执行作用域，执行结束后清空变量及节点、回调、模式引用。继续通过 Yak VM 执行原程序，保留原始源码位置和完整错误，不以直接 Go 调用替换 Yak 的报错语义。

池最多保留 32 个空闲引擎，每个最多 16 个已装载程序、源码最多 512 B；活跃并发不足时创建新执行器，归还时超过空闲预算则不保留。嵌套调用不等待父调用归还引擎，因此没有全局单引擎互斥造成的串行化或重入死锁。此池不等于固定 goroutine / CPU 绑定，也没有实现双向流亲和。

当前闭合语法仅接受“已知原生桥接函数 + 不可变标量参数 + 检查错误”两条语句。扫描当前 YAML，1,248 个 operator 中 128 个入口符合，涉及 42 个原生绑定；本次 11 条应用基准消息的字段入口全部符合。这个数量不是完整协议覆盖率。含 eval、include、异步、闭包、可变参数或其他逻辑的自定义程序，以及 carrier operator 和非标量 out，仍保持原来的新引擎 / 独立装载路径。即使仅改变为本语法未接受的等价格式，也会正确回退；不能扩展白名单后未经隔离验证就复用任意脚本。

未改 Yak VM 核心、公共 API 或外部依赖。[vm.go](../../parser/stream_parser/vm.go) 将原库绑定提取为 invocation 工厂；普通脚本拥有独立 invocation，受限桥接引擎才重绑其私有 invocation。

## 实验身份和方法

机器为 Apple M1 Max、10 CPU、64 GiB、macOS 14.1.2、Go 1.22.12 darwin/arm64，非独占桌面，未锁频。分支为 wip/optimize-protocol-parse，HEAD 为 0b3ed685b1b462dfc7eb342c2b754313a5e6f9c7 加此前未提交工作；#5022 与 #5023 已在这之前合并。不能直接 checkout HEAD 重现当前功能。

三个程序依次冻结：baseline 是本轮改动前，chain 只含 A，worker 含 A+B；使用完全相同的基准与样本。完整二进制路径、SHA-256、manifest 摘要在 [identity.txt](identity.txt)，当前被测源文件摘要在 [source-sha256.txt](source-sha256.txt)。[原 ExecOperator 函数](baseline-exec-operator.go.txt) 单独归档用于审阅 B 阶段差异；它不是整个历史工作树的源码快照。临时二进制可能被系统清理。

应用基准每 op 是整批 11 条消息，不是一条消息；预热和样本加载在计时外，但每轮原有 goroutine / channel 调度及独立树构建仍计时。本轮没有通过改变调度器取得不同工作量的对比结果，也没有另测长期 worker 的持续队列表现。每个配置分别报告真实 worker 数，统计中位数与最小–最大范围；范围不是置信区间。

两轮正式实验均串行，没有与本任务的编译、回归或 profile 重叠：

1. 600 ms 窗口，1 / 4 / 10 worker，每组 3 次；
2. 因短窗口出现较大桌面波动，追加 2 s 窗口，1 / 10 worker，每组 3 次。版本顺序分别为 baseline/chain/worker、chain/worker/baseline、worker/baseline/chain。

保留两轮全部慢值，不合并不同窗口计算统计。最早 baseline/chain 两次探索误并发启动，明确排除在所有正式比较之外，见 [被排除的观测值](excluded-overlap-pilot.tsv) 与 identity 中的审计记录。

## 长窗口结果

下表每批耗时单位 ms，越低越好，括号为最小–最大。后三列为吞吐增加，越高越好；Yak 增加量相对于 chain，而合计相对于 baseline。

| 路径 / 并发 | baseline ms ↓ | chain ms ↓ | worker ms ↓ | A 吞吐 ↑ | B 额外吞吐 ↑ | 合计吞吐 ↑ |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| Fields / 1 worker | 2.164（2.160–2.166） | 1.982（1.972–1.997） | 1.843（1.839–1.881） | 9.2% | 7.5% | 17.4% |
| Fields / 10 worker | 1.048（1.046–1.048） | 0.945（0.942–0.950） | 0.903（0.901–0.903） | 10.9% | 4.7% | 16.1% |
| JSON / 1 worker | 2.681（2.672–2.727） | 2.496（2.483–2.496） | 2.343（2.338–2.353） | 7.4% | 6.5% | 14.4% |
| JSON / 10 worker | 1.321（1.316–1.331） | 1.221（1.218–1.223） | 1.185（1.181–1.210） | 8.1% | 3.1% | 11.5% |

单 worker 字段路径的累计分配由 1,618,204 B / 26,853 次降至 1,442,977 B / 23,617 次，分别减少 10.8% / 12.1%；JSON 由 1,840,202 B / 32,164 次降至 1,665,001 B / 28,927 次，减少 9.5% / 10.1%。这些都是每批 11 条消息的累计分配，不是 RSS、常驻内存或泄漏指标。

原始值为 long-1/2/3-baseline/chain/worker.txt；机器可读统计、吞吐换算和全部等价摘要见 [summary.json](summary.json)。速度只适用于本次两协议混合，不将这些比例推及所有 42 个绑定或整个库。

### 短窗口结果也保留

此轮出现了范围重叠和个别方向相反的结果，例如 10 worker JSON 的 B 阶段中位耗时比 A 高约 3.2%。长窗口随后观察到不同方向和较窄范围，不能据此删除短窗口或宣称已消除调度噪声。

| 路径 / 并发 | baseline ms ↓ | chain ms ↓ | worker ms ↓ | A 吞吐 ↑ | B 额外吞吐 ↑ | 合计吞吐 ↑ |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| Fields / 1 worker | 2.269（2.156–2.272） | 2.056（1.988–2.363） | 2.048（1.947–2.092） | 10.4% | 0.4% | 10.8% |
| Fields / 4 worker | 1.182（1.124–1.291） | 1.080（1.051–1.280） | 1.060（1.011–1.500） | 9.5% | 1.9% | 11.6% |
| Fields / 10 worker | 1.083（1.039–1.284） | 0.987（0.978–1.109） | 0.971（0.926–1.107） | 9.7% | 1.7% | 11.6% |
| JSON / 1 worker | 2.709（2.590–2.914） | 2.597（2.511–2.711） | 2.393（2.365–2.782） | 4.3% | 8.5% | 13.2% |
| JSON / 4 worker | 1.486（1.411–1.665） | 1.435（1.373–1.584） | 1.317（1.311–1.895） | 3.6% | 9.0% | 12.9% |
| JSON / 10 worker | 1.373（1.300–1.523） | 1.277（1.246–1.423） | 1.319（1.207–1.454） | 7.5% | -3.1% | 4.1% |

原始文件为 round-1/2/3-baseline/chain/worker.txt。

## 等价性和回归

[baseline](baseline-export-digest.txt)、[chain](chain-export-digest.txt)、[worker](worker-export-digest.txt) 各运行一次全记录导出摘要检查。8 个固定分区摘要完全一致：封装 4 分区共 58,533 条，应用 4 分区共 11 条。摘要纳入记录 ID、rule、entry、剩余位置、完整 NodeToMap、元数据、具体数值类型和空值形状，以及预期拒绝时的完整错误。检查包含 manifest SHA、每份捕获 SHA 和记录数。58,533 条是封装记录，不能说成所有完整应用消息；摘要相等也不能替代独立字段正确性断言。正确性检查允许互相并行，其耗时不用于性能比较。

- [新增配置测试](../../parser/base/config_write_test.go)：16 个并发写入者、1,600 条重放记录无丢失；重放结果和顺序一致；裸类型 / 列表 / 空值与旧写法对照，复杂选项和非法长度保持回退。
- [新增执行器测试](../../parser/stream_parser/operator_worker_test.go)：同一个引擎和指令对象确实复用；连续成功/失败后作用域与引用清空；0–7 位偏移下 commit / invalid / short / callback 事务；8 路并发、嵌套租借、取消、LRU 淘汰、完整源码错误对照；动态 eval、可变字面量和返回闭包走独立回退。闭合语法拒绝动态参数、插值、字节数组、额外语句和延期入口。
- Memcached / Cassandra 的原始两代样本、字段和负样本先单独通过；随后受影响原生桥接协议族定向检查通过，见 [corpus-tests.txt](corpus-tests.txt)。
- parser/base、parser/stream_parser、parser 三包全部测试通过，见 [parser-regression-tests.txt](parser-regression-tests.txt)；选定配置、桥接、operator / out、两协议与 runtime 隔离 race 检查通过，公开 Memcached / Cassandra 字段入口的 race 检查也通过，命令与日志见 [validation.txt](validation.txt)。macOS linker 的 LC_DYSYMTAB 警告保留，不是 race 报告。
- 新增测试最初将 Yak undefined 误断言为 Go nil，修正断言后通过，没有放宽生产隔离行为。没有运行整个库或仓库全量，也没有重新裁定历史协议评分状态。

## 剩余成本：不能归咎于 Yak 指令执行

对最终冻结版单独采样，CPU 为 5 s、分配为 3 s，不将带 profile 的吞吐混入正式结果。[分配采样](worker-alloc-top.txt) 的 flat 部分中，配置历史追加约 34.7%，NewConfig 约 14.4%，配置存储扩容约 13.3%；它们指向剩余的配置与字段对象成本。不要把这些 flat 数字与重叠的 cumulative 调用栈相加，也不要将累计分配当成泄漏。

[CPU 样本](worker-cpu-top.txt) 再次有约 69.2% 归属 GetFileAbsDir / os.Getwd / Stat 链。但此前 [预加载路径反证实验](../performance-20260906/OPTIMIZATION_EXPERIMENTS.md) 已出现采样占比大、实际干预收益小的矛盾，本轮没有重新验证其原因，不能宣称消掉该调用就能回收 69% 时间。Yak 调用栈还包含原生解码与建树，亦不代表纯解释执行的独占占比。

当前实测支持：减少准备成本有收益，单靠复用引擎不足以带来数倍整体提速。后续仍优先评估不可变字段描述 / 解析计划、配置重放历史表示和紧凑建树；任一改动继续要求完整输出与回滚等价。分层匹配和流亲和保持规划状态，不能用它们解释本次已指定入口基准的收益。

## 复测

在包含当前未提交改动的根目录构建到新的临时目录，随后从包目录启动，勿覆盖本轮三个冻结程序：

```sh
go test -c -o /tmp/binparser-next-remeasure.test ./common/bin-parser
cd common/bin-parser
/tmp/binparser-next-remeasure.test -test.run '^$' -test.bench '^BenchmarkCurrentCorpusApplication(Fields|JSON)$' -test.benchtime=2s -test.count=3 -test.cpu=1,10 -test.benchmem
BINPARSER_EVALUATE=1 /tmp/binparser-next-remeasure.test -test.run '^TestProtocolCorpusExportDigest$' -test.count=1 -test.v -test.timeout=180s
```

完整验证命令、profile 命令及已运行结果见 validation.txt；重测当前源码只能得到当前阶段，不能当成重新生成改动前二进制。
