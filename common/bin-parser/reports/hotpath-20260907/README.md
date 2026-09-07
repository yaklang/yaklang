# #5013 完整解析性能与 P0 验收

本轮继续开发 `wip/optimize-protocol-parse`。性能基线为 `5b6cbe1f8f7e7e505f8f3aaa542d4ac12f0d3741`；P0 修复后的正确性参考为 `b6cfeef97`；首个主指标达标候选为 `aa8d5b36cb858da5d0d15df350766c6e13278389`，它未通过 AllJoyn 代表负载验收。针对回退的修复冻结在 `35607643c484b3d13c8e6fb08320c6c527b2b896`；后续最终结果与首次候选分开记录。PR 保持不合并。

## 最终完整 JSON 验收

最终代码 `35607643c`，正式证据为 `qualification-final/`。仍是原公开调用链、11 条消息、1,421 B、完整字段和元数据 JSON。两种 worker 数各五次独立 3 秒窗口，基线/候选交替串行执行；所有慢值都保留，未混入 profile 或回归。以用户约定的中位数验收：单 worker 吞吐 **2.008859 倍**，达到至少 2 倍目标；10 worker 为 **1.892561 倍**。

| worker | 原始基线 ms/批 | 最终 ms/批 | 吞吐倍数 | 基线 B/批 | 最终 B/批 | 基线分配/批 | 最终分配/批 |
| ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| 1 | 1.693982 | 0.843256 | 2.008859× | 1,397,293 | 721,059 | 14,376 | 6,481 |
| 10 | 0.882223 | 0.466153 | 1.892561× | 1,419,958 | 739,941 | 14,404 | 6,507 |

单 worker 原始范围 1.690730–1.702709 ms，最终 0.840191–0.847291 ms；10 worker 原始范围 0.876314–0.889323 ms，最终 0.463226–0.484146 ms。单 worker 约 6,494 → 13,045 条消息/秒，分配字节下降约 48.4%，分配次数下降约 54.9%。这是指定环境的五次中位数，不承诺每个单独窗口都恰好超过两倍。

AllJoyn 大计数回退已修复；最终 40 个代表负载组合全部通过，最大耗时增加为 0.96%。全部 58,533 条封装记录和 11 条应用消息摘要一致，P0、全包回归、相关 race 与最终代码 CI 均通过。详见下文的独立证据。

## 首次候选的完整 JSON 主指标

沿用原 `BenchmarkCurrentCorpusApplicationJSON`：每批 11 条消息、1,421 B，每次创建独立读取器，通过公开 `ParseBinary`、`NodeToMap` 和 `encoding/json` 输出完整字段与 `additionInfo` 元数据。输入、预热、每批 goroutine 调度和输出工作量不变。没有以精简字段、预制 JSON、替代 API 或吞吐外推代替主指标。

正式独立验收 `qualification-pwd-matched`：每种 worker 数分别运行五个独立 3 秒窗口，基线/候选交替串行执行。以下均为五次中位数；每批单位适用于耗时、字节和分配次数。

| worker | 基线 ms | 首次候选 ms | 吞吐倍数 | 基线 B | 首次候选 B | 基线分配 | 首次候选分配 |
| ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| 1 | 1.708869 | 0.838594 | 2.04× | 1,397,288 | 721,063 | 14,376 | 6,481 |
| 10 | 0.898050 | 0.466913 | 1.92× | 1,420,222 | 739,401 | 14,403 | 6,506 |

单 worker 基线范围 1.703554–1.723082 ms，候选 0.828798–0.847948 ms；10 worker 基线范围 0.884591–0.900122 ms，候选 0.465412–0.476422 ms。单 worker 每秒约 6,437 → 13,117 条消息，分配字节下降 48.4%，分配次数下降 54.9%。这是指定离线工作量的结果，不是实时持续无丢包带宽承诺。

### 首轮未达标与启动环境诊断

首轮 `qualification` 完整保留：单 worker 1.808409 → 0.944649 ms（1.91×），10 worker 0.911038 → 0.501102 ms（1.82×），未达到主目标。Python `subprocess(cwd=...)` 改变了实际目录，却继承了仓库根目录的 `PWD`；Go 的目录解析因此走额外查询路径。原来的直接 shell / `go test` 调用两者一致。

测量器随后对两边同时显式设置相同的 `cwd` 和 `PWD`，重新开始五次独立窗口，没有挑选旧窗口或混合两种环境。代码仍保留原路径解析，没有缓存调用方工作目录。独立路径微基准中，错误环境中位数为 10,218 ns / 706 B / 6 次分配，匹配环境为 2,592 ns / 626 B / 5 次分配。源码和全部值见 `cwd_cost_test.go.txt`、`cwd-cost-inherited.txt`、`cwd-cost-matched.txt`。该实验解释环境差异，不能直接从应用总耗时扣除路径累计成本或 GC 成本。

## 实现与可撤回阶段

| 提交 | 变化与边界 |
| --- | --- |
| `5819a5b17` | 添加原生解码、字段构树、公开解析、投影和序列化成本拆分工具 |
| `b6cfeef97` | 独立 P0 修复：LDAP 六个显式消息入口、SMB3 完整 Transform 载荷边界及可执行范围验收 |
| `8b15fd133` | 私有配置键编号，共享当前值与赋值历史的值存储；未知键及公开历史修改保留兼容路径 |
| `93573afb1` | 直接读取结果字节范围，保留非整字节转换、原始字节所有权和精确标量类型 |
| `a70f37551` | 编译嵌入 YAML 的不可变构树操作计划，按原顺序执行配置和继承 |
| `7a008559e` | 按字段数量为一次结果准备节点、配置、子节点；不池化公开结果 |
| `6a26dad1a` | 只直接执行十个精确匹配的 Memcached/Cassandra 原生桥；失败通过原 Yak 程序重放原错误，回调与读取事务只执行一次 |
| `27fca80fb` | 共享不可变默认配置前缀，后续写入与结果配置独立 |
| `cc4acef59` | 整棵规则树按计划节点数批量准备空间，包括未选择的公开入口 |
| `93ddd19be` | 读取一致的投影配置，预分配 map/list；保留重复名称、nil、空列表与自定义 out 的既有区别 |
| `18152b701` | 减少 Memcached 临时字段描述与类型装箱 |
| `97494dd99` | YAML 可变文档克隆计划实验；后续撤回，不能作为最终实现 |
| `69d3d823d` | 显式 import 目标保持紧凑配置存储 |
| `2b4af4c6c` | 将结果元数据写入紧凑配置路径 |
| `aa8d5b36c` | 撤回无稳定收益的克隆计划，恢复通用克隆，补空文档 Origin 差分回归 |
| `a9c1053e2` | bin-parser CI 超时从 1 分钟调整为 5 分钟，其余任务保持原设置 |
| `35607643c` | 长度计算一次实时读取结果范围与 consumed bits，减少重复锁与查找，修复 AllJoyn 大计数回退 |

公开 Go API 保持兼容。静态计划不共享 VM、读取器、事务、结果树或嵌套可变配置；动态构造规则继续使用通用路径。公开配置切片修改、浅复制、删除后重放、present nil、自定义回调重入及异常写入顺序均保留。

## 各阶段完整路径对照

每个阶段、每种 worker 数各五次独立 3 秒窗口。下表使用该阶段相邻交替运行的五次原始基线作分母，避免把整场 70 个基线窗口混为一个配对基线。全量原始值、慢值、失败实验程序均保留在 `phase-application/`；`summary.json` 中原始基线的全场汇总只供环境观察。各阶段是累计变化，不据此宣称相邻提交具有可独立相加的收益。

| 阶段 | 1 worker 基线/阶段 ms | 吞吐倍数 | 10 worker 基线/阶段 ms | 吞吐倍数 | 1 worker B/批 | 分配/批 |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| p0 | 1.681443 / 1.670739 | 1.006× | 0.887804 / 0.887045 | 1.001× | 1,397,287 | 14,376 |
| compact | 1.681786 / 1.449953 | 1.160× | 0.888764 / 0.760531 | 1.169× | 1,029,611 | 12,815 |
| windows | 1.684758 / 1.390191 | 1.212× | 0.886368 / 0.726535 | 1.220× | 958,738 | 10,905 |
| rules | 1.678456 / 1.343797 | 1.249× | 0.888853 / 0.724890 | 1.226× | 942,717 | 10,459 |
| node_batch | 1.696600 / 1.291466 | 1.314× | 0.887303 / 0.708289 | 1.253× | 952,612 | 9,027 |
| native_bridge | 1.683786 / 1.009679 | 1.668× | 0.890525 / 0.553062 | 1.610× | 907,871 | 8,352 |
| prefix | 1.681587 / 0.969238 | 1.735× | 0.889606 / 0.534972 | 1.663× | 793,556 | 8,230 |
| rule_batch | 1.676037 / 0.959502 | 1.747× | 0.888823 / 0.536302 | 1.657× | 817,839 | 7,626 |
| projection | 1.679854 / 0.914930 | 1.836× | 0.887754 / 0.509003 | 1.744× | 815,770 | 7,609 |
| descriptors | 1.690993 / 0.880105 | 1.921× | 0.889859 / 0.484661 | 1.836× | 791,464 | 6,791 |
| clone_plan | 1.678589 / 0.876919 | 1.914× | 0.885667 / 0.485311 | 1.825× | 791,199 | 6,780 |
| import_config | 1.673995 / 0.849171 | 1.971× | 0.884583 / 0.474695 | 1.863× | 737,258 | 6,525 |
| candidate | 1.686219 / 0.830195 | 2.031× | 0.884665 / 0.470200 | 1.881× | 720,794 | 6,470 |
| candidate_final | 1.690493 / 0.829952 | 2.037× | 0.890172 / 0.477259 | 1.865× | 721,059 | 6,481 |

`clone_plan`、`import_config` 和 `candidate` 含后来撤回的克隆计划；它们是过程证据；表中 `candidate_final` 指当时的 `aa8d5b36c` 候选，仍含后来发现的 AllJoyn 回退，最终交付程序为 `consumed_snapshot`（`35607643c`）。撤回后单 worker 中位数基本不变（0.830195 → 0.829952 ms）；10 worker 耗时变化约 +1.5%，低于 5% 复测阈值，空文档兼容问题已消除。

## 最终代表负载验收

最终代码 `35607643c` 的 20 种负载、两种 GOMAXPROCS 设置共 40 个组合全部通过；每组基线与最终程序各五次独立 3 秒交替窗口，没有超过 5% 的回退，最大耗时增加为 0.96%。普通负载的 38 个组合来自 `representatives-final/analysis.json`，AllJoyn 大计数的两个组合来自同一冻结程序的 `alljoyn-fixed/analysis.json`。两组均在独立进程中串行测量，未混入首次候选窗口。

应用与封装基准按原实现创建 worker，其余微基准保持原调度；GOMAXPROCS=10 不表示每个微基准都创建十条解析 worker。表中负数表示耗时减少。完整慢值、每次样本、程序与输入身份见各目录的 `runs.jsonl`、原始文本和 `analysis.json`。

| 负载 | 1：基线/最终 ms | 耗时变化 | 10：基线/最终 ms | 耗时变化 |
| --- | ---: | ---: | ---: | ---: |
| CurrentCorpusApplicationFields | 1.250909 / 0.473919 | -62.11% | 0.626323 / 0.267203 | -57.34% |
| CurrentCorpusEnvelopes | 14349.144083 / 8009.798209 | -44.18% | 3856.920083 / 2046.587708 | -46.94% |
| NHRPClientList/32/legacy-false | 3.464381 / 3.371368 | -2.68% | 2.695864 / 2.646723 | -1.82% |
| NHRPClientList/4096/legacy-false | 355.780144 / 347.801317 | -2.24% | 247.645393 / 244.765738 | -1.16% |
| DICOMPDVList/items-128 | 2.343873 / 2.324160 | -0.84% | 1.762581 / 1.752064 | -0.60% |
| DICOMPDVList/items-4096 | 74.096442 / 74.367137 | +0.37% | 51.031292 / 51.520939 | +0.96% |
| DICOMUserInformation/items-128 | 6.668117 / 6.427423 | -3.61% | 5.468535 / 5.282915 | -3.39% |
| DICOMUserInformation/items-512 | 16.361892 / 15.560312 | -4.90% | 12.795019 / 12.212646 | -4.55% |
| DICOMUserInformation/items-1024 | 29.665133 / 27.958618 | -5.75% | 22.613209 / 21.394362 | -5.39% |
| DICOMUserInformation/items-2048 | 58.705278 / 55.722479 | -5.08% | 42.723458 / 40.855238 | -4.37% |
| DICOMUserInformation/items-4096 | 116.393379 / 107.757837 | -7.42% | 82.002940 / 78.644035 | -4.10% |
| ProtocolCorpusIPX/direct | 0.599085 / 0.345526 | -42.32% | 0.466057 / 0.283717 | -39.12% |
| ProtocolCorpusIPX/standalone | 0.144348 / 0.132007 | -8.55% | 0.118932 / 0.110261 | -7.29% |
| ProtocolCorpusIPX/ethernet | 0.963476 / 0.880020 | -8.66% | 0.801118 / 0.741004 | -7.50% |
| ProtocolCorpusSVExecOut/parse-only | 3.811304 / 3.776390 | -0.92% | 3.437794 / 3.422455 | -0.45% |
| ProtocolCorpusSVExecOut/parse-and-root-result | 3.847025 / 3.810237 | -0.96% | 3.464994 / 3.450415 | -0.42% |
| ProtocolCorpusSVExecOut/parse-root-and-repeated-field-results | 3.876505 / 3.832952 | -1.12% | 3.486530 / 3.478467 | -0.23% |
| ProtocolCorpusSVExecOut/parse-and-single-root-result-traversal | 3.848756 / 3.810774 | -0.99% | 3.463919 / 3.479866 | +0.46% |
| ParserConsumedLength/ordinary-184B/AllJoynNS/DirectLookup | 5.335384 / 5.270682 | -1.21% | 4.747602 / 4.759987 | +0.26% |
| ParserConsumedLength/counts-1454B/AllJoynNS/DirectLookup | 623.285842 / 626.755183 | +0.56% | 590.765056 / 594.849965 | +0.69% |

## aa8d5b36c 代表负载首次验收

原始证据见 `representatives/`。20 种负载分别在 `GOMAXPROCS=1/10` 下完成基线与候选各五次独立 3 秒窗口。应用和封装基准按原实现创建 worker；其余微基准保留各自的调度方式，GOMAXPROCS 不表示它们都创建了十条解析 worker。按原始日志独立重算的 `analysis.json` 为正式汇总，负载规模后缀不会被合并。

**这一候选未通过代表负载验收**：AllJoyn 1454 B 大计数负载在两个设置下分别回退 6.4% / 8.2%；后续同规模复测确认持续回退，已按下文记录修复。其余 38 个组合没有超过 5% 的回退。耗时变化为候选/基线减一，负数表示改善。

| 负载 | 1：基线/候选 ms | 耗时变化 | 10：基线/候选 ms | 耗时变化 |
| --- | ---: | ---: | ---: | ---: |
| CurrentCorpusApplicationFields | 1.225177 / 0.468251 | -61.78% | 0.625192 / 0.267449 | -57.22% |
| CurrentCorpusEnvelopes | 14259.241791 / 8008.014667 | -43.84% | 3832.309166 / 2058.327771 | -46.29% |
| NHRPClientList/32/legacy-false | 3.450743 / 3.392266 | -1.69% | 2.694506 / 2.662700 | -1.18% |
| NHRPClientList/4096/legacy-false | 360.003722 / 345.579250 | -4.01% | 248.364065 / 244.696688 | -1.48% |
| DICOMPDVList/items-128 | 2.358640 / 2.354952 | -0.16% | 1.757616 / 1.763353 | +0.33% |
| DICOMPDVList/items-4096 | 75.490514 / 75.949287 | +0.61% | 51.125442 / 52.514476 | +2.72% |
| DICOMUserInformation/items-128 | 6.670279 / 6.529746 | -2.11% | 5.468641 / 5.322096 | -2.68% |
| DICOMUserInformation/items-512 | 16.285174 / 15.638163 | -3.97% | 12.702468 / 12.235060 | -3.68% |
| DICOMUserInformation/items-1024 | 29.414649 / 28.454509 | -3.26% | 22.231170 / 21.385259 | -3.81% |
| DICOMUserInformation/items-2048 | 58.364633 / 55.679028 | -4.60% | 42.176138 / 40.552269 | -3.85% |
| DICOMUserInformation/items-4096 | 114.495650 / 108.065987 | -5.62% | 81.393234 / 79.015336 | -2.92% |
| ProtocolCorpusIPX/direct | 0.598259 / 0.348940 | -41.67% | 0.466265 / 0.286818 | -38.49% |
| ProtocolCorpusIPX/standalone | 0.143972 / 0.132317 | -8.10% | 0.120226 / 0.110196 | -8.34% |
| ProtocolCorpusIPX/ethernet | 0.955024 / 0.880301 | -7.82% | 0.805356 / 0.742348 | -7.82% |
| ProtocolCorpusSVExecOut/parse-only | 3.817397 / 3.803624 | -0.36% | 3.438351 / 3.441981 | +0.11% |
| ProtocolCorpusSVExecOut/parse-and-root-result | 3.851453 / 3.828918 | -0.59% | 3.466564 / 3.466142 | -0.01% |
| ProtocolCorpusSVExecOut/parse-root-and-repeated-field-results | 3.876445 / 3.893740 | +0.45% | 3.493954 / 3.491740 | -0.06% |
| ProtocolCorpusSVExecOut/parse-and-single-root-result-traversal | 3.847065 / 3.832058 | -0.39% | 3.490022 / 3.465963 | -0.69% |
| ParserConsumedLength/ordinary-184B/AllJoynNS/DirectLookup | 5.293576 / 5.285670 | -0.15% | 4.757400 / 4.768627 | +0.24% |
| ParserConsumedLength/counts-1454B/AllJoynNS/DirectLookup | 629.933833 / 670.222242 | +6.40% | 592.078639 / 640.667575 | +8.21% |

### AllJoyn 同规模复测

`alljoyn-retest/` 使用相同冻结程序和完整大计数负载，两个 GOMAXPROCS 设置各五次独立 3 秒交替窗口。单 worker 631.524783 → 674.331817 ms（+6.78%），10 worker 596.464674 → 650.956217 ms（+9.14%），确认回退持续。不能用主指标倍速或分配下降代替这项验收；随后用 `probe.py` 对冻结阶段作诊断定位，独立采集 CPU 和分配 profile。

### 回退修复与独立验收

`35607643c` 为长度遍历增加一次实时的结果范围 / consumed bits 查询，两个关联值在同一把读锁下取得。存在结果时，显式 consumed bits（包括 nil 和零）仍优先；不存在结果时继续遍历当前子节点。没有跨调用缓存长度、跳过节点、改变 benchmark 或扩展 Yak VM 复用范围。公开配置修改后的读取、类型断言和事务回滚仍由差分测试约束。

`alljoyn-stages/` 的逐阶段诊断显示回退始于紧凑配置阶段。CPU profile 的长度遍历路径反复进入配置查询；因此优化实际重复读取与加锁，而不是将累计 VM 调用栈比例视为可省去的成本。该目录的单次阶段测量仅用于定位，不代替正式验收。

修复后 `alljoyn-fixed/` 重新完成两个 GOMAXPROCS 设置各五次独立 3 秒交替窗口：单 worker 623.285842 → 626.755183 ms（+0.56%）；10 worker 590.765056 → 594.849965 ms（+0.69%）。原始基线、失败候选、复测和修复后数据均保留。此结果消除了持续超过 5% 的回退。

## P0 的消息范围

四项能力目录继续标记 `partial`，P0 完成只针对 [消息合同及独立证据](../../P0_SCOPE.md)。`TestP0RoadmapCovered` 无条件执行固定范围清单，修改名称、状态或增加豁免不能代替证据。

- LDAP：历史 BindRequest 保留；增加 BindResponse、UnbindRequest、SearchRequest、SearchResultEntry/Done/Reference，检查 BER 长度、十类过滤器、属性和值的顺序及 controls 边界。
- MySQL：V10/41 握手、SSLRequest、已有经典命令、OK/ERR/EOF 和已有文本结果集 profiles。
- PostgreSQL：显式指定方向和阶段的 Startup、SSL/GSS、认证、查询、扩展查询及响应 profiles。
- SMB3：完整 52 字节 Transform 头和不透明加密载荷边界，以及已有 3.0/3.0.2/3.1.1 NEGOTIATE 与 contexts。没有解密、鉴权或会话猜测。

评分按现有维度重新核对：LDAP 80/B、MySQL 75/B、PostgreSQL 81/B、SMB3 75/B。新原生入口不自动取得完整 YAML schema 或穷尽分支分数。保留历史字段形状、两代捕获、负例及十四项延期；不纳入流亲和、TCP 重组、真实捕获容量、客户端或 aid 集成。

## 失败与撤回记录

- 更小配置索引：分配字节降低但无稳定速度收益，撤回；保留 `rejected-smaller-store.patch`、`pilot-store-size.txt` 和兼容检查。
- 合并字段初始化遍历：没有稳定收益，撤回；保留 `rejected-fused-initialization.patch`、`pilot-fused-tree.txt` 和测试日志。
- YAML 克隆计划：空 `MapSlice` 在缓存替换后产生 nil 与非 nil 空值差异，`clone-empty-regression.txt` 记录失败；恢复通用克隆后通过，见 `clone-revert-tests.txt`。其他输入的样本摘要一致不能免除这个兼容失败。
- 投影首版误将 stream 的 `list` 和 base 的 `isList` 混用：公开回归发现后修复；`pilot-export.txt` 是无效候选成绩，仅作失败记录；有效结果见 `pilot-export-fixed.txt`。
- `pilot-prefix.txt` 的错误运行目录导致 manifest 查找失败；保留日志，不计入性能验收。
- 原 CI 同时存在四项 P0 状态冲突和 1 分钟超时；见 `original-ci-failure.txt`。本地完整回归耗时约 168 秒，因此只将对应 CI 包的超时改为计划要求的 5 分钟。

短测、成本拆分和 profile 只用于诊断，不与正式窗口混合统计。各阶段是累计实现对照，相邻阶段差异受调度与 GC 影响，不将累计调用栈百分比相加为可实现加速。

## 成本拆分与最终 profile

`diagnostics/` 保存成本拆分程序与 fixture 的摘要、构建及运行命令。原始实现只增加诊断测试文件；原先与读取事务写在同一函数内的构树代码原样提取到测试辅助函数，没有改动基线生产实现。全部构建结束后才开始诊断计时；CPU 与分配 profile 在不同进程中分别采集。

下表为三个 3 秒诊断窗口的中位数，单进程 `-test.count=3`，不替代正式独立验收。原生解码使用固定语义 fixture，构树使用 48 行 STAT 的预解码描述；投影及序列化使用准备好的原 11 条消息树/值，互相不能直接相加为整批总成本。

| 拆分 | 原始 ns/op | 最终 ns/op | 原始 B/op / 分配 | 最终 B/op / 分配 |
| --- | ---: | ---: | ---: | ---: |
| 原生 Memcached stats request | 412.2 | 417.0 | 640 / 5 | 640 / 5 |
| 原生 Memcached stats response | 8,124 | 6,800 | 14,194 / 107 | 11,242 / 100 |
| 原生 Memcached binary get | 2,101 | 2,104 | 4,714 / 14 | 4,714 / 14 |
| 48 行 STAT 独立字段构树 | 326,046 | 112,243 | 475,952 / 2,969 | 164,928 / 400 |
| 11 条消息 NodeToMap | 165,711 | 61,916 | 105,653 / 2,668 | 36,004 / 785 |
| 11 条消息已准备值的 JSON 序列化 | 260,514 | 258,526 | 112,721 / 2,620 | 112,721 / 2,620 |

公开 `ParseBinary` 的整批成本由正式 `BenchmarkCurrentCorpusApplicationFields` 测量，见代表负载表；完整调用链由 `BenchmarkCurrentCorpusApplicationJSON` 单独验收。原生解码和准备好的树/值没有代替公开入口。

最终分配 profile 的最大单项是每次独立拥有的 `NewNodeBatch`（采样 alloc_space 约 44.8%）；标准 JSON 反射、原生解码描述和投影仍占有成本。它描述累计分配，不是泄漏或常驻内存。CPU 样本主要落在 syscall/目录查询调用链，但独立路径微基准和完整耗时对照不支持把该累计占比等同于可消除的应用成本。profile 包含启动、预热和 benchmark 校准，其时间及分配总量不充当无 profile 窗口的吞吐或每批分配。

原始 CPU、原始 alloc、最终 CPU、最终 alloc 四个 profile 分开运行；二进制 profile 保存在永久产物目录，SHA-256 和程序身份在 `diagnostics/diagnostics.json`，文本 top 表保存在本目录。此前的单次 profile、错误候选与环境诊断仍保留。

## 正确性、兼容和隔离

`aa8d5b36c` 上的 `go test ./common/bin-parser/... -count=1 -timeout=5m` 退出 0，包含 `TestP0RoadmapCovered`；主包 168.181 秒，parser / base / stream_parser / protocol-impl 全部通过，完整输出见 `final-full-tests.txt`。该次是首次候选的验证，后续长度读取修复另行完成最终验证。

相关 race 检查退出 0：

```sh
go test -race ./common/bin-parser/parser/... -count=1 -timeout=5m
go test -race ./common/bin-parser -run 'TestP0RoadmapCovered|TestNodeToMapSinglePassSemantics|TestProtocolCorpusNodeToMapSinglePassApplication|TestProtocolNativeBridge|Test.*(Concurrent|Isolation|Retain)' -count=1 -timeout=5m
go test -race ./common/bin-parser/parser/base -count=1 -timeout=5m
```

首两项在克隆计划撤回前通过；撤回后重跑受影响的 base 全包 race（7.233 秒）和最终完整回归。日志见 `race-parser.txt`、`race-public.txt`、`final-race-base.txt`。独立测试覆盖公开配置赋值/删除/重放/函数切片与浅复制、异常写入、全部嵌入规则构树、YAML 继承顺序、Origin 与嵌套配置隔离、重复字段和空列表、动态 operator/out、导入、生成、位范围、旧结果持有及并发输入。位窗口差分对照保留原 bit-reader 实现，遍历两种端序、14 种类型、起始位 0–24、长度 0–80、三类 pending writer 状态及原始字节所有权。

### 最终代码与全量摘要

`35607643c` 上再次执行 `go test ./common/bin-parser/... -count=1 -timeout=5m`，全部退出 0，主包 180.822 秒，包含 P0。原命令与结果保存在 `final-consumed-full-tests.txt`。

```sh
go test -race ./common/bin-parser/parser/... -count=1 -timeout=5m
go test -race ./common/bin-parser -run 'TestP0RoadmapCovered|TestNodeToMapSinglePassSemantics|TestNativeBridgePublicDifferential|TestProtocolCorpus(AllJoynConcurrentIsolation|ConsumedLengthPublicDifferential)' -count=1 -timeout=5m
```

最终两项 race 退出 0，原始输出见 `final-consumed-race-parser.txt` 和 `final-consumed-race-public.txt`。公共原生桥差分检查覆盖原 11 条消息的每个短前缀、追加字节、字段、精确类型、元数据、字节及完整错误。长度修复沿用既有动态覆盖、present nil、无效类型、父子修改、分隔符和回滚差分，并新增公开查询与合并读取的对照。

`digests/analysis.json` 校验原始日志 SHA-256 后，确认 16 个冻结阶段全部 8 个分区摘要完全相同：58,533 条封装记录和 11 条应用消息无遗漏。它包含原始性能基线、P0 正确性参考、撤回实验和最终长度读取修复。摘要记录公开字段、类型与 nilness、元数据、错误全文及剩余读取字节；独立协议字段断言和专门兼容测试另行决定正确性，不能仅靠摘要相同证明协议完整。

## 可复现环境与证据布局

Go 1.22.12，darwin/arm64，macOS 14.1.2，Apple M1 Max，10 个逻辑 CPU，64 GiB 内存。正式计时期间不编译、不运行回归或 profile；worker 数用 `GOMAXPROCS=1/10` 指定，其他运行时设置记录在每组 `run.json`。

永久本地产物目录：`/Users/v1ll4n/Documents/Codex/artifacts/bin-parser-5013-20260907`。包含原始完整源码归档、每阶段相对原始基线的二进制补丁、冻结测试程序和构建日志。`freeze.json` 记录完整提交、Git tree、源码归档/补丁/程序/manifest 的 SHA-256；每个窗口的 `runs.jsonl` 记录实际命令、目录、PWD、时间、退出码、原始日志摘要和全部指标。大程序与源码归档不加入 Git。

在仓库根目录运行测量器；首次冻结与计时分开执行，冻结期间不做性能结论。追加最终阶段是为了保留被撤回实验的完整程序身份：

```sh
python3 common/bin-parser/reports/hotpath-20260907/measure.py freeze --artifacts /absolute/new-artifacts --candidate 379f6099b
python3 common/bin-parser/reports/hotpath-20260907/measure.py freeze --artifacts /absolute/new-artifacts --candidate aa8d5b36c --label candidate_final
python3 common/bin-parser/reports/hotpath-20260907/measure.py freeze --artifacts /absolute/new-artifacts --candidate 35607643c --label consumed_snapshot
python3 common/bin-parser/reports/hotpath-20260907/measure.py qualification --artifacts /absolute/new-artifacts --tag qualification
python3 common/bin-parser/reports/hotpath-20260907/measure.py application --artifacts /absolute/new-artifacts --tag phases
python3 common/bin-parser/reports/hotpath-20260907/measure.py representatives --artifacts /absolute/new-artifacts --tag representatives
python3 common/bin-parser/reports/hotpath-20260907/measure.py digests --artifacts /absolute/new-artifacts --tag digests
python3 common/bin-parser/reports/hotpath-20260907/analyze.py benchmarks /absolute/new-artifacts/representatives
python3 common/bin-parser/reports/hotpath-20260907/analyze.py digests /absolute/new-artifacts/digests
python3 common/bin-parser/reports/hotpath-20260907/diagnose.py --artifacts /absolute/new-artifacts
```

新环境的结果应按其实际值重新验收。冻结脚本拒绝覆盖已有程序或实验目录。`analyze.py` 校验每个原始日志的 SHA-256，再按成对窗口计算结果；它保留 `items-128` 等负载规模后缀。早期测量器对单 worker 无 CPU 后缀的名称进行了多余剥离，原始日志和时间值不受影响；代表负载以重新解析原始日志的 `analysis.json` 为准，保留原 `runs.jsonl` 和 `summary.json` 以便追溯。

## CI 与交付身份

最终可执行代码固定为 `35607643c484b3d13c8e6fb08320c6c527b2b896`。本次证据提交只更新报告、原始测量和复现工具，不改变 Go 代码、规则、输入、`go.mod` 或 `go.sum`；本地全量回归、race、摘要和正式性能均对应这一实现。

- [最终代码 Essential Tests](https://github.com/yaklang/yaklang/actions/runs/34082549160)：success；完整任务状态见 `ci-35607643c-essential.json`。
- [最终代码 yaklang engine test](https://github.com/yaklang/yaklang/actions/runs/34082549151)：success。
- [最终代码 Diff Check](https://github.com/yaklang/yaklang/actions/runs/34082549143)：success。
- [前一候选 Essential Tests](https://github.com/yaklang/yaklang/actions/runs/34080153457)：success，保留 `ci-a9c1053e2-essential.json`。此前失败及超时原因见 `original-ci-failure.txt`。

报告提交后的最新 CI 链接与状态记在 [PR #5013](https://github.com/yaklang/yaklang/pull/5013) 描述和检查页，避免为写入自身提交号而反复创建新的代码身份。PR 保持开放，不合并。
