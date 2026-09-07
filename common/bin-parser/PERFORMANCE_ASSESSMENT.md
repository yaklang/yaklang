# 当前样本性能、带宽与 AI 分析能力评估

2026-09-06，2026-09-07 补充有限队列回放与小配置存储对照 · 冻结测量与对照优化 · 本地离线数据分析

## 结论与范围

已按 [本次范围](ACTIVE_SCOPE.md) 将指定的 14 项延期。Cassandra 与 Memcached 当前样本字段及联合定向验证完成，随后开始本轮评估，没有继续扩展其他协议，也没有增加依赖。

当前实现适合作为**按需、异步协议详情与学习解释的基础组件**，还不能称为已交付的 AI 版 Wireshark，也不能承诺千兆实时逐包深解析。最新第四轮小配置存储优化，在本机 1/4/10 worker 各三次对照中，使封装/两协议字段解析吞吐中位数提高约 **29%–30%**，完整 JSON 导出提高约 **27%–28%**。10 worker 的新版本约 **11,146 条封装记录/秒、13.70 Mbps 输入字节率**；两协议字段与 JSON 导出约 **8,425 条/秒、8.71 Mbps 应用输入字节率**。二者是独立工作负载，不能相加、不能当作一条端到端流水线或实时链路最大带宽。[第四轮完整对照](reports/config-store-20260907/README.md) 保留旧、新程序、样本、全部运行及限制；下文首轮表格继续保留历史基线，不混算跨轮加速比。

**热点归因已修正**：首轮 `Getwd/Stat` 的高 CPU 样本占比没有转化为实际大幅提速，路径预加载原型已撤回。保留的是 `NodeToMap` 单次遍历：同程序新旧对照中，两协议完整 JSON 导出吞吐中位数提高约 12%–22%，字段和输出字节保持一致，详见 [第二轮实验](reports/performance-20260906/OPTIMIZATION_EXPERIMENTS.md)。没有依据先全面替换正则。客户端和 aid 已有入口，但尚未连接本次显式字段 profile、完整证据来源和解析状态。AI 实际回答正确率、模型时延及客户端渲染体验尚未测量。

**有限队列长测补充**：第三轮对两协议 38 条原始捕获记录循环执行封装、已知应用 profile 与完整 JSON 导出，完成 33 轮、累计 510 秒计划窗口。64 条队列下，4 worker / 2 Mbps 的 10 秒测试三轮无丢弃，但 30 秒测试两轮出现少量丢弃；8 Mbps 下，4/10 worker 的记录丢弃率中位数分别为 38.99% / 25.33%。本轮计入投递迟到和排队，没有通过减少到达量掩盖过载；仍不能把结果当作实时链路无损上限，详见 [回放评估与原始证据](reports/replay-20260907/README.md)。

**最新回放与回归**：第四轮另外进行 12 次新旧对照、累计 240 秒计划窗口。4 worker / 6 Mbps / 10 秒的丢弃率中位数由 20.073% 降至 0.465%，但新版三轮仍均有丢弃；2 Mbps / 30 秒也有一轮丢弃 84 条，仍无持续无损容量承诺。58,533 条封装记录及 11 条应用消息，两程序各两轮导出摘要一致，公共路径 race 通过。随后执行 bin-parser 包及子包完整回归，主包失败；日志可见五个顶层失败项均在旧、新程序复现，包括历史 P0 状态、RMI 分支/评分和 WinRM、GSS-API 旧合同断言。没有升级标签、删除负样本或放宽门槛；[失败对照及处理边界](reports/config-store-20260907/README.md) 单独记录，不能声称全库已通过。

**验收修复后的最新状态**：第五轮仅对齐旧测试、评分与说明，未修改解析后端或样本。四组旧验收失败已消除，完整 bin-parser 复验（193.302 s）只剩 LDAP、MySQL、PostgreSQL、SMB3 的历史 P0 `partial` 状态冲突；相关定向 race 通过（27.105 s）。RMI 原生 Schema 未纳入 YAML 审计，评分从 95/A 下调至 75/B；无效 RMI/WinRM/GSS-API 原样本仍拒绝，没有放宽 P0 门槛。[修正、原始日志及剩余边界](reports/acceptance-20260907/README.md) 独立归档。前轮性能汇总重算一致，没有以本轮测试耗时更新吞吐，也不宣称完整套件全绿。

**aid 离线工具实测补充**：第六轮在隔离 CLI / 日志 / 文件落盘的环境中，原样执行现有 PCAP 脚本和真实离线读取后端。8 个用例各三次，均复现 Cassandra 误计 DNS、过滤器无效、包数上限越界且后续回调继续、无效过滤器及坏文件仍报告成功；5 个用例每轮失败，不能折算成模型正确率。Cassandra frame 14 的误触发字节已定位，独立字段复验通过。未调用模型、未修改工具或客户端，没有用该探针耗时更新吞吐；详见 [方法、失败与限制](reports/aid-probe-20260907/README.md)。

## 版本、环境与正确性前提

| 项目 | 本轮条件 |
| --- | --- |
| 代码基线 | `wip/optimize-protocol-parse`，HEAD `0b3ed685b1b462dfc7eb342c2b754313a5e6f9c7` **加当前未提交工作区**；不能仅 checkout 此 HEAD 重现新增实现 |
| 合并顺序 | #5022、#5023 均已先合并，两代样本独立保留 |
| 测量程序 | `go test -c` 构建的 `/tmp/binparser-current-20260906-v2.test`；非 race 构建 |
| 程序 SHA-256 | `29ba128ec6ee48ba227b57336d312746ff69f21a0639ab7c414141bb6f34a80d` |
| manifest SHA-256 | `870350c644462dffd3462ac8b71adcfff1fe725c19ace6d0d39765b1ab9181ca` |
| 设备 | Apple M1 Max，10 个逻辑 CPU，64 GiB RAM |
| 系统 / Go | macOS 14.1.2 (23B92)，Go 1.22.12，darwin/arm64 |
| 运行条件 | 本地桌面环境，存在其他应用负载；未隔离 CPU，未锁定频率，不是独占基准机；各主基准顺序运行 |
| 缓存 / I/O | 样本加载、摘要校验、边界提取及规则预热在吞吐计时外；每条消息创建独立解析树 |
| 统计方法 | 各配置 3 次基准结果，报告中位数和最小–最大值；不是置信区间，不删除慢值 |

552 份捕获、58,533 条记录的完整性和全记录封装检查已在此程序重新通过；后者用时 **22.455 秒**，逐条断言比吞吐基准更强，不能与 6.513 秒吞吐计时作加速比。检查保留预期拒绝、截断记录及显式容器，不以原始字节存在冒充应用语义。当前矩阵的 327 个字段级、10 个外层、4 个识别名称是证据分类，不是完整协议支持数量。

本批两协议共 4 份捕获、38 条原始记录，其中 11 条应用消息、27 条控制记录：

| 捕获 ID | 原始记录 / 应用消息 | 此次字段范围 |
| --- | --- | --- |
| `ndpi-cassandra` | 20 / 7 | CQL v4 与 v5 初始 OPTIONS、SUPPORTED、STARTUP；internode initiate |
| `ndpi-memcached` | 10 / 2 | 文本 stats 请求；48 项统计及 END 完整响应 |
| `gen-memcache-bin` | 4 / 1 | 二进制 GET 请求，key 为 `foo` |
| `pr5023-gen-memcache-bin` | 4 / 1 | 新一代捕获同一 GET 消息，独立文件与摘要 |

原始字节、字段值、控制记录、全部短前缀、资源边界和事务回滚的实现验证见 [corpus 说明](testdata/protocol-corpus/README.md)、[Memcached 测试](protocol_corpus_memcached_fields_test.go) 和 [Cassandra 测试](protocol_corpus_cassandra_fields_test.go)。首轮执行的是这些定向测试、清单/合同检查与全记录封装检查，当时没有运行整个库的完整测试套件。第四轮公共后端优化之后已执行一次 bin-parser 包及子包完整回归，结果失败，详见新增对照报告；没有执行整个 Yaklang 仓库全量。

性能实验后的交付复核重新执行了历史 `TestP0RoadmapCovered`，确认它在 LDAP、MySQL、PostgreSQL、SMB3 的 `partial` 状态上失败。六组当前字段检查（包括 Memcached 和 Cassandra）共关联 17 份捕获、205 条记录，字段与边界联合检查及 race 检查通过；其中保留控制记录、负样本和两代数据，205 条记录不等于 205 条应用消息。这些检查没有升级目录标签、修改评分卡或替代历史门槛，不能据此宣称完整套件通过或整个 goal 已完成。[复核命令、范围及原始日志](reports/field-audit-20260906/README.md) 独立保留，不并入性能统计。

## 工作负载与吞吐

[测量入口](protocol_corpus_current_benchmark_test.go) 使用公开 `ParseBinary`、显式有界输入和指定 entry。吞吐不是直接调用原生解码函数的微基准。

- **Envelopes**：一轮为全部 58,533 条记录，合计 8,993,088 字节解析器输入，包含显式容器边界；不是所有捕获文件的磁盘大小。只测封装入口，不含 TCP 重组与应用解码。
- **ApplicationFields**：一轮为上述 11 条应用消息、1,421 字节，包含两代 GET。边界及 profile 由已知样本定位预先给出；不计自动识别、TCP 分片重组或状态选择。
- **ApplicationJSON**：同一组消息，额外构建 `NodeToMap`、附加元数据并进行 JSON 序列化；不含文件输出、RPC、数据库、模型调用及 UI。

所有 Mbps 使用十进制 `输入字节数 × 8 / 秒 / 1,000,000`。`ns/op` 和 `B/op` 的 op 是**整轮**，不是单包。表内包率从未舍入的 ns 计算；应用项单位是消息/秒。**下表及本节后续延迟/内存数字保留首轮 v2 基线**；优化后的同程序对照和显式单 CPU 延迟独立列在第二轮报告，不混合不同程序统计。

| 工作负载 | worker | 每轮耗时中位数（范围） | 条/秒中位数（范围） | 输入 Mbps 中位数 |
| --- | ---: | --- | --- | ---: |
| 全记录封装 | 1 | 27.088 s（27.043–27.135） | 2,161（2,157–2,164） | 2.656 |
| 全记录封装 | 4 | 8.879 s（8.841–8.924） | 6,592（6,559–6,621） | 8.103 |
| 全记录封装 | 8 | 6.758 s（6.735–6.790） | 8,661（8,621–8,692） | 10.645 |
| 全记录封装 | 10 | 6.513 s（6.495–6.824） | 8,987（8,577–9,011） | 11.046 |
| 两协议字段 | 1 | 2.769 ms（2.767–2.770） | 3,972（3,972–3,975） | 4.105 |
| 两协议字段 | 4 | 1.509 ms（1.491–1.601） | 7,290（6,872–7,377） | 7.534 |
| 两协议字段 | 8 | 1.374 ms（1.363–1.376） | 8,007（7,993–8,068） | 8.275 |
| 两协议字段 | 10 | 1.342 ms（1.337–1.357） | 8,197（8,105–8,227） | 8.472 |
| 字段 + JSON | 1 | 3.850 ms（3.846–3.864） | 2,857（2,846–2,860） | 2.953 |
| 字段 + JSON | 4 | 2.234 ms（2.233–2.238） | 4,923（4,916–4,927） | 5.088 |
| 字段 + JSON | 8 | 2.075 ms（2.074–2.078） | 5,301（5,293–5,304） | 5.478 |
| 字段 + JSON | 10 | 2.044 ms（2.044–2.049） | 5,380（5,367–5,381） | 5.560 |

从 8 增至 10 worker，封装、字段、JSON 中位吞吐分别仅再提高约 3.8%、2.4%、1.5%。这是此负载和批调度方式下的收益递减，不证明任何流量的最优并发就是 10。桌面交互也不宜默认占满 CPU。

完整数值及纳入统计的每次运行见 [summary.json](reports/performance-20260906/summary.json)，原始输出见 [证据目录](reports/performance-20260906/README.md)。

## 单消息延迟、复杂度与内存

单消息延迟独立测量：1 worker，预热后每轮 2,200 条消息，3 轮；每轮采用 nearest-rank 百分位，下面报告**三个百分位结果的中位数（范围）**，没有把不同轮的百分位合并成总体百分位。

| 入口 | P50（µs） | P95（µs） | P99（µs） | 三轮观察到的最大值（ms） |
| --- | --- | --- | --- | ---: |
| 字段 | 135.5（133.8–137.1） | 955.3（949.7–957.6） | 1,904.8（1,889.7–2,206.9） | 4.116 |
| 字段 + JSON | 172.5（169.8–172.8） | 1,573.7（1,571.5–1,577.2） | 3,079.0（2,997.5–3,128.9） | 4.895 |

这 11 条小消息不能代表复杂协议。补充测量继续保留已有大列表边界，没有降低字段数量换速度：

| 输入与操作 | 1 worker 耗时中位数（范围），n=3 | 累计分配中位数 / 次 |
| --- | --- | ---: |
| 当前 mgadelha SV 首条完整消息，解析 + root Result + 一次字段遍历 | 4.644 ms（4.550–5.032） | 1.357 MB |
| DICOM 4,096 个 PDV，完整构树和数量检查 | 103.465 ms（102.682–106.780） | 63.578 MB |
| DICOM association 合计 4,096 项，完整构树和数量检查 | 154.498 ms（146.438–159.395） | 77.293 MB |
| IGRP 104 条路由，完整构树和数量检查 | 42.503 ms（41.132–42.561） | 11.273 MB |

SV 来自实际捕获；DICOM 和 IGRP 是既有测试函数在内存构造的压力向量，**不是声称原始 PCAP 含有这样的长记录**。SV 基准为 300 ms，后两类每个结果为 3 次操作的均值，各再重复 3 轮。各入口操作不同，不用于横向协议排名；这里只说明字段树复杂度对成本的影响。

分配与常驻内存必须分开解释：

- 全记录封装每轮累计分配约 **19.485 GB**，平均每条约 **333 kB、6,791 次分配**；这是回收前后累计量，绝不是需要 19 GB 常驻内存或内存泄漏的证据。
- 两协议单 worker 平均每条：字段约 **185 kB、2,946 次分配**；字段 + JSON 约 **228 kB、4,108 次分配**。
- 独立 `/usr/bin/time -l` 的 4 worker 全记录封装进程，含启动、加载和预热，峰值 RSS **122,109,952 B（116.45 MiB）**；进程 wall 9.00 s、user 31.03 s、sys 0.71 s，0 swaps。这是一次进程观测，不是全客户端、10 worker 或长时间流状态的内存上限。
- 延迟测量每 2,200 条：字段 GC 26–27 次、累计 pause 1.125–1.185 ms；JSON GC 31 次、累计 pause 1.404–1.508 ms。每 32 条采样的 Go heap 最大值分别约 26.26–27.48 MB、26.91–27.81 MB；这不是精确堆峰值，也不是 RSS。
- 当前完整字段及元数据 JSON 每 11 条为 **25,312 B**，输入 1,421 B，约 **17.81 倍**。这是有重复信息的完整导出格式，不能直接当作 token 数，也不宜逐包全部塞给模型。

## 带宽与客户端判断

**本轮测到了离线处理能力，没有测到实时无丢包最大带宽。** 最新第四轮封装最高已测中位数约 13.70 Mbps，两协议字段及完整 JSON 导出约 8.71 Mbps；它们只在对应固定样本组合与成本边界内成立。不能用同一个包率乘任意 MTU 声称更大的链路能力，不能将纯捕获落盘能力与逐包深解析混为一谈。

对于当前样本分布，现有完整树路径没有接近 1 Gbps 的测量证据。2026-09-07 已补充 2/3/4/6/8 Mbps 的有限队列本地回放，并记录迟到、队列丢弃、P99、CPU 与 RSS；10 秒无丢弃结果在 30 秒补测中未保持，因此没有给出已认证的持续容量。该回放仍使用已知 profile 和预先提取的完整消息，不包含真实流状态、TCP 重组、RPC、落盘或客户端刷新。真实上限还需同一端到端路径与更有代表性的流量组合验证，且需区分投递侧调度与解析器服务成本。本轮没有向外部主机发送样本。

客户端判断目前是架构建议而非 UI 实测结论：小消息详情导出 P99 首轮约 3.1 ms、第二轮约 2.83 ms；两轮不是配对延迟试验，不能直接据此计算优化收益。完整 corpus 封装首轮即使 10 worker 也需约 6.5 s，大列表单条可需 100–155 ms，不能放在同步 UI 路径。应保留后台处理、取消、分页、按需字段、有限队列和进度；尚未测量实际客户端首屏、滚动、帧率或 RPC 端到端时延。

## 采样观察与实测优化边界

单 worker JSON 工作负载的 4.18 s CPU profile 收到 3.85 s CPU 样本，包含加载、预热、基准校准与计时阶段。它是短剖析，不代表其他协议的全局热点分布。

1. 约 **71.43%** CPU 样本落在 `ParseBinary → ParseRule → GetFileAbsDir → filepath.Abs → os.Getwd → Stat`。这是原始采样观察，**不再据此认定可节约同等比例耗时**。第二轮预加载路径使该链不再出现于摘要，却仅使字段解析计时下降约 0.8%–2.1%，封装/多并发 JSON 结果部分重叠；原型已撤回，具体原因尚未证实。保留 [ParseRule](parser/base/base.go) 和 [工作目录兼容测试](parser/base/rule_path_test.go) 的原有语义。
2. `alloc_space` 采样约 2.49 GB：`OrderedMap.Set` flat 29.63%、`Config.SetItem` flat 23.27%（其 cum 52.68%，与前项有包含关系，不能再相加）。优先评估配置写入历史、完整节点树和证据投影的分配；保持独立树、事务回滚、重复字段顺序与原始字节范围。
3. `NodeToMap` cum 占分配约 14.18%。第二轮仅去掉重复递归，保留完整导出，实测单 worker 每轮分配字节下降约 10.0%、分配次数下降约 16.5%。按需投影仍是后续候选，不把本次收益说成已经实现按需导出。累积分配 profile 不是泄漏诊断。

首轮只增加测量与报告；第二轮保留一行已做等价验证和新旧对照的重复遍历修复，撤回收益不足的路径预加载 API。第四轮基于配置分配观察，保留私有 `BaseKV` 小存储：内置 8 项，超过 16 项后索引化，维持顺序、浅共享、快照、独立树和锁；没有删除 option 写入历史或引入解析状态池。全记录封装每轮累积分配约 19.485 → 14.940 GB；2 Mbps 回放过程 CPU 等效核数中位数约 1.489 → 1.150，但 RSS 范围重叠，没有明显 RSS 改善证据。[第四轮原始数据与方法](reports/config-store-20260907/README.md) 保留慢轮和失败。没有修改通用路径语义、客户端或 aid；已有正则局部优化仍保留，没有增加依赖或依据局部匹配收益宣称全库提速。

## aid 与 AI 分析能力

### 当前连接状态：代码核验与受控脚本运行，不是模型得分

| 能力 | 现状与证据 |
| --- | --- |
| 离线 PCAP 工具 | 已有 [analyze_pcap.yak](../ai/aid/aitool/buildinaitools/yakscripttools/yakscriptforai/pcap/analyze_pcap.yak)，调用 `pcapx.OpenPcapFile`，生成 gopacket 文本和统计；并未调用本批 bin-parser 字段入口 |
| 统计可信度 | HTTP/DNS 统计使用原始字节子串启发式；受控执行在 Cassandra 20 条 TCP 记录中误计 1 次 DNS 查询，定位到 frame 14 包内零基偏移 51 的 `01 00 00 01`。独立 CQL 字段检查通过，不是凭端口判定 |
| 资源控制 | 默认 max-packets 50,000；实测设置 1 时只输出 1 条详情，却报告总数 2，全部 10 条仍进入包处理回调。标记停止不等于取消读取和先于回调发生的 TCP 处理；详细文本先累计再 Join，不是有界流式证据输出 |
| 离线过滤与错误传播 | `tcp port 1` 的独立匹配为 0，现有入口却仍输出全部 10 条；无效 BPF 以及非 PCAP 文件也未向脚本返回错误，脚本报告成功。上述行为每用例三轮一致 |
| 客户端解析入口 | [grpc_pcapx.go](../yakgrpc/grpc_pcapx.go) 的重组解析只尝试 HTTP/TLS；通用封装使用普通 `bytes.Reader`，没有传本次 `InputBitLength` 有界合同和 entry 选择 |
| 状态语义 | packet/reassembled RPC 在解析错误、仅返回 RAW 时仍可置 `OK=true`；不能用此值断言应用字段已解码；session 分支也尚未完成内容解析 |
| aid 扩展基础 | [WithTools / WithAICallback](../ai/aid/aicommon/config.go) 与 [ToolExecutionResult](../ai/aid/aitool/tool_result.go) 已支持工具回调及独立语义结果，可承载只读结构化证据；新解码器的完整集成尚未验证 |

以上没有扩大修改旧工具、RPC 或客户端的范围。[受控脚本探针](reports/aid-probe-20260907/README.md) 保留三轮完整日志及初始构建失败；它使用真实 Yak VM 与 PCAP 后端，但隔离 CLI / 日志 / 文件写入，不等于实际 aid Tool.Callback 或 UI 集成测试。[pcap 回调次序](../pcapx/pcaputil/config.go) 也说明单独停止分析回调不等于停止读取和重组。4 份原捕获生成 31,495 字节详情文本，不是结构化应用结果、token 数或性能指标。

### 能做什么，不能先声称什么

可行方向是“本地确定性解析与流索引 → 有界证据查询工具 → aid 解释、比较与教学”，不是每个包调用一次模型。每个结论应关联 capture ID、文件 SHA、frame、绝对字节/位范围、rule/profile、解码器版本和 decoded/partial/raw/deferred 状态。现有字段树与测试提供局部证据基础，源文件身份、重组片段映射和状态仍需接入层补齐。

面向用户可先做“解释选中字段”“比较两条握手”“说明哪些事实仍未知”“从解释跳回原始字节”。推荐先准备只读的捕获/流列表、指定消息详情、字段过滤与按范围取字节工具。载荷文本只作为数据，不能成为模型的操作指令。原始捕获默认留在本地，模型仅取得用户选择且受大小预算限制的证据。

下一轮可用当前样本建立以下验收题；答案依据来自本批独立字段测试与原始记录。**这里只建立评估基准，没有调用模型，不报告虚构的正确率或“12/12 AI 通过”。**

| 问题 | 正确答案必须包含的事实 / 边界 |
| --- | --- |
| stats 响应有多少项，完整吗？ | `ndpi-memcached` frame 6 有 48 项及最终 END |
| 当前与累计连接数是否相同？ | 同帧 curr_connections=10、total_connections=13；两者不可混为一项 |
| bytes_read=21 是抓包文件大小吗？ | 是响应中的服务端统计字段，不是 PCAP 大小；保留原始值和来源 |
| get_hits=0 能推出命中率为零吗？ | cmd_get 也为 0，不能给出有意义的除零比例 |
| 两代 GET 是否相同？ | frame 4 应用字节相同、key=`foo`、opaque=1；捕获文件与 SHA 不同 |
| 能否判断 GET 命中或失败？ | 两份捕获没有 GET 响应；请求不能证明结果 |
| 两次 CQL STARTUP 版本值？ | `ndpi-cassandra` frames 8/16：3.3.1 / 3.4.6 |
| 列出 v6-beta 就代表会话用了 v6 吗？ | frame 14 是能力广告，不是 READY 或成功协商的证明 |
| v5 初始 STARTUP 的驱动？ | frame 16：DataStax Python Driver，3.25.0 |
| internode 校验正确就代表会话建立吗？ | frame 20 initiate 的 CRC 可校验，但未观察到完整会话接受过程 |
| stats 响应截断后应展示什么？ | 明确不完整/原始字节；Carrier 回退不冒充完整统计解析 |
| 14 项延期是否已经实现应用解析？ | 没有；遵守本次范围，不用名称识别或外层字段升级结论 |

模型评估需分别记录事实正确、证据定位、未知情况处理、工具成功语义、总耗时和 token 用量；固定模型版本、提示与工具 schema，并区分工具错误和模型错误。当前这些模型指标及真实 aid 工具回合时延均为**未测**。

## 与 Wireshark 的位置关系

本库的实际特点是 Go 内嵌调用、可扩展 YAML、明确的有界 profile，以及与自定义工具结果相近的字段/字节证据。它们有利于在 Yak/aid 内做定制集成，但“接入方便”不等于已经交付 AI 分析链路。

Wireshark/TShark 已有捕获与离线分析、字段筛选、重组关联及多种结构化导出。TShark 同样支持 JSON、PDML、EK，当然也能作为 AI 工具后端；BPF 捕获过滤器与 Wireshark 显示过滤器并非同一种语法。[TShark 官方手册](https://www.wireshark.org/docs/man-pages/tshark.html)

Wireshark 的现成协议分析与交互能力仍是更成熟的基线；本库目前许多 profile 仅覆盖已观察到的消息，协议自动选择、完整会话状态和 GUI 连接仍有缺口。应把 bin-parser 定位为可嵌入、可追溯的解析组件，将 aid 的学习解释作为产品层建设，不能把它说成对 Wireshark 的现成全面替代。[Wireshark 用户指南](https://www.wireshark.org/docs/wsug_html_chunked/ChapterIntroduction.html)

本轮没有执行等价工作负载的 Wireshark 性能对照，因此不报告“比 Wireshark 快多少”，也不把本机 TShark 某个 profile 的解码差异泛化成整体优势。

## 后续改进规划（2026-09-07 讨论记录）

按用户要求，将分层分发图、连接级解析器缓存和双向流亲和列入 [性能改进规划](PERFORMANCE_IMPROVEMENT_PLAN.md)。同时记录当前 Go 原生字段解码、Yak 包装、规则树创建、配置写入及完整导出的成本边界，提出不可变解析计划、原生算子入口、紧凑字段表示和分阶段等价对照。当前 YAML 文档与脚本编译已有缓存，不将已有机制重复列为新增成果；也不把旧 profile 的累计调用成本当作 Yak 指令执行占比。

这次仅更新本地文档，新增方案尚未实施，没有重新测量、修改解析实现、扩大延期协议范围或改变上文冻结结果。

### 后续执行：链路与 Yak 复用（2026-09-07）

用户随后授权实现，现已完成配置写入 / 裸类型建树优化，以及受限桥接入口的独占 Yak 引擎和程序复用。新增的三个冻结版本、长短窗口测量、128 个入口的复用边界、全记录输出等价和回归结果见 [分阶段报告](reports/path-worker-20260907/README.md)。此处新增结果不改写前文历史实验。

2 s 窗口、每组 3 次，单 worker 完整 JSON 从 2.681 ms/批降至 2.343 ms/批，吞吐增加 14.4%；10 worker 从 1.321 ms/批降至 1.185 ms/批，增加 11.5%。每批仍是相同 11 条消息、1,421 B 输入及完整输出。10 worker 约 9,284 消息/s、9.59 Mbps 是这一指定应用负载的离线输入字节率，不是新测得的网卡或持续无损容量。本轮未实现流亲和、自动匹配、客户端或 aid，也未扩大延期范围。

### 后续执行：配置历史与批量构树（2026-09-07）

随后完成普通配置历史的数据化和共享字节字段树的批量初始化，完整接口、重放和回滚兼容。容量预留第一版有额外扩容成本，已根据对照结果调整；两个版本的原始证据都保留在 [本轮报告](reports/config-tree-20260907/README.md)，不改写前文历史结果。

相对于已经包含上一节 Yak 复用的基线，最终三次长窗口中位数：单 worker 完整字段从 1.919 降至 1.326 ms/批，吞吐提高 44.7%；完整 JSON 从 2.419 降至 1.825 ms/批，提高 32.5%。10 worker 完整 JSON 从 1.196 降至 0.922 ms/批，提高 29.7%，到约 11,935 消息/s、12.33 Mbps 应用输入。每批仍为 11 条消息、1,421 B 及完整输出；单 worker JSON 分配字节减少 16.1%，次数减少 50.3%。不把此输入字节率外推成客户端真实抓包容量或 AI 分析吞吐。

## 复现与后续验收

在包含本批未提交实现的工作树根目录构建，然后进入包目录运行；直接在仓库根目录启动测试二进制会找不到相对 manifest 路径。以下命令重新测量当前源码，使用新文件名，避免覆盖首轮冻结程序和 profile；当前源码含第二、四轮优化，不会重建出首轮 v2 的相同 SHA。原遍历的同程序对照见第二轮命令，小配置存储的新旧构建边界与命令见第四轮报告。

```sh
go test -c -o /tmp/binparser-remeasure-20260906.test ./common/bin-parser
cd common/bin-parser
for workers in 1 4 8 10; do
  /tmp/binparser-remeasure-20260906.test -test.run '^$' -test.bench '^BenchmarkCurrentCorpusEnvelopes$' -test.benchtime=1x -test.count=3 -test.cpu="$workers" -test.benchmem
  /tmp/binparser-remeasure-20260906.test -test.run '^$' -test.bench '^BenchmarkCurrentCorpusApplication(Fields|JSON)$' -test.benchtime=600ms -test.count=3 -test.cpu="$workers" -test.benchmem
done
GOMAXPROCS=1 BINPARSER_EVALUATE=1 /tmp/binparser-remeasure-20260906.test -test.run '^TestCurrentCorpusApplicationLatency$' -test.count=3 -test.cpu=1 -test.v
/usr/bin/time -l /tmp/binparser-remeasure-20260906.test -test.run '^$' -test.bench '^BenchmarkCurrentCorpusEnvelopes$' -test.benchtime=1x -test.count=1 -test.cpu=4 -test.benchmem
/tmp/binparser-remeasure-20260906.test -test.run '^$' -test.bench '^BenchmarkProtocolCorpusSVExecOut$/^parse-and-single-root-result-traversal$' -test.benchtime=300ms -test.count=3 -test.cpu=1 -test.benchmem
/tmp/binparser-remeasure-20260906.test -test.run '^$' -test.bench '^Benchmark(DICOMPDVList|DICOMUserInformation)$/^items-4096$' -test.benchtime=3x -test.count=3 -test.cpu=1 -test.benchmem
/tmp/binparser-remeasure-20260906.test -test.run '^$' -test.bench '^BenchmarkProtocolCorpusIGRP104Routes$' -test.benchtime=3x -test.count=3 -test.cpu=1 -test.benchmem
/tmp/binparser-remeasure-20260906.test -test.run '^$' -test.bench '^BenchmarkCurrentCorpusApplicationJSON$' -test.benchtime=3s -test.count=1 -test.cpu=1 -test.benchmem -test.cpuprofile=/tmp/binparser-remeasure-cpu-20260906.pprof -test.memprofile=/tmp/binparser-remeasure-mem-20260906.pprof
/tmp/binparser-remeasure-20260906.test -test.run '^(TestProtocolCorpusIntegrity|TestProtocolCorpusAllPacketEnvelopes|TestProtocolCorpusMemcachedFields.*|TestProtocolCorpusCassandraFields.*|TestProtocolCorpusSupplementalProfiles|TestProtocolCatalogRuleFilesExist|TestProtocolRoadmapIntegrity|TestProtocolCorpusContractReferencesExist)$' -test.count=1 -test.v -test.timeout=4m
```

测量修正透明记录：Go 1.22 的 `-benchtime=1x -cpu=1,4` 首次探测可沿用空测试阶段最后设置的 GOMAXPROCS，导致标题看似单 CPU、实际 4 worker。代码新增真实 worker 指标；原始首行保留但不纳入两组 n=3，单 worker 使用原序列第二、三行与独立 `-cpu=1` 补测。8 worker 最初的应用 1x 探索行也保留，不与正式 600 ms 应用结果混合。复现命令使用每个并发数独立进程避免这一陷阱。

下一阶段顺序建议：根据实际分配继续验证配置/节点构建成本，不再优先追逐已被干预结果削弱的路径假说；随后连接有界 profile、来源与解析状态到只读工具和客户端，再做真实端到端负载及 AI 题集。定向检查贯穿修改；一次冻结后的全量回归仍有价值，但不能代替实时容量或模型正确性的专门试验。本报告不擅自扩大到延期协议或新的客户端/aid 实现。
