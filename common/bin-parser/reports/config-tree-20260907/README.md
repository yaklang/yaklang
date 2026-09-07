# 配置历史数据化与批量构树：分阶段对照

2026-09-07。本轮基线已包含上一轮的链路和 Yak 引擎复用。本轮只改变配置历史的内部表示、普通配置的构建方式和共享字节字段树的批量初始化；没有新增依赖、修改 Yak VM、协议规则或捕获样本，没有扩展延期的 14 项。

最终长窗口对照中，单 worker 完整字段吞吐提高 44.7%，完整 JSON 提高 32.5%；10 worker 分别提高 44.7% 和 29.7%。单 worker JSON 累计分配字节减少 16.1%、分配次数减少 50.3%。这些是相同两协议混合样本、相同完整输出下的局部收益，不代表实时捕获容量。

## 实现与兼容边界

### A：普通配置历史使用数据记录

[config_store.go](../../parser/base/config_store.go) 将尚未公开的赋值历史保存为有序键值记录。普通写入不再逐次创建闭包和装箱函数切片；AppendConfig 可以直接重放内部记录。删除当前值仍保留原赋值历史，重放顺序和值的浅共享保持不变。

公开读取 `CfgOptionFuns` 或遍历全部配置时，按原顺序生成 `[]NodeConfigFun`，随后保留原来的函数切片路径。逐项 append 保留同一 Go 运行时的切片容量，避免改变公开切片修改、浅复制与自定义回调的行为。显式替换、非法类型、nil 回调和自定义函数不走数据化捷径。重放时不持有源配置锁，保留回调重入以及修改后续回调的行为。

表达式恢复通过 `ReplayHistoryLen` 查询历史状态，不为只检查长度而公开函数切片。`CopyConfig` 仍采用原来的公开快照语义，会按需物化历史；没有把整个库的任意历史复制改为共享内部日志。

### B：私有配置批量初始化和子节点预分配

[config_batch.go](../../parser/base/config_batch.go) 一次读取可继承的 endian / parser / unit，在尚未发布的新配置中按顺序初始化全部已知项；容量按本次赋值数向上分档，保留后续初始化的扩容余量。需要修改既有配置时，SetItems 在一次锁内依次写入，保留发生非法历史类型时“先写当前项、再报错、停止后续项”的行为。

[共享字节字段树](../../parser/stream_parser/tls_certificate_tree.go) 将字段跨度、字节序、父节点、长度、列表状态和元素索引一起初始化；已知字段数量时预分配子节点切片。裸类型可直接批量构建，复杂类型描述仍走通用解析器。保留原有配置写入顺序、InitNode 初始化和最终父指针重绑。

没有池化公开可变节点，没有共享不同消息的可变状态，也没有减少字段、列表项、元数据或导出内容。空子节点仍为 nil，已观测空列表仍有零宽跨度。解码和建树全部完成后才发布；后段字段非法也保留原结果并回滚读取事务。

## 实验身份与方法

硬件为 Apple M1 Max、10 CPU、64 GiB；macOS 14.1.2、Go 1.22.12 darwin/arm64，非独占桌面，未锁频。分支 wip/optimize-protocol-parse，HEAD 为 `0b3ed685b1b462dfc7eb342c2b754313a5e6f9c7` 加此前未提交工作和本轮变更，不能直接 checkout HEAD 重现。

[reserved-identity.json](reserved-identity.json) 保存最终三个冻结程序、被测源文件和 manifest 的 SHA-256。baseline 是本轮修改前，journal 只含 A，reserved 含 A+B 的最终容量策略。第一版精确容量的 batch 及其源文件身份另外保存在 [identity.json](identity.json)，该版本没有保留为最终实现。临时程序及控制源文件保存在 identity 指定目录，可能被系统清理；它们不是完整历史工作树快照。

应用基准仍是两协议的 11 条消息、每批 1,421 B，预先指定规则、入口和完整消息边界。Fields 保留完整节点树；JSON 还包含 NodeToMap、元数据和完整 JSON 序列化。样本加载和预热在计时外；原基准的每批 goroutine / channel 调度、独立树构建仍计时，没有改换工作量或调度器。

两次正式实验均使用 2 s 窗口、1 / 10 worker，每组 3 个独立进程。每次顺序为 baseline/journal/候选、journal/候选/baseline、候选/baseline/journal。最终候选 reserved 的原始日志使用 reserved-long 前缀；第一版候选 batch 的日志使用 long 前缀。不同轮次不混算。中位数及最小–最大范围分别报告，范围不是置信区间；保留慢值，不删异常点。短窗口探索另存 pilot-baseline/journal/reserved.txt，不与正式数据混算。

所有正式性能运行串行，并与本任务的编译、回归和 profile 分开。正确性作业可以互相并行，其日志中的耗时不能用作版本性能比较。命令、起止时间、退出码见 [commands.jsonl](commands.jsonl)。

## 结果

### 最终容量策略

每批仍为 11 条消息；下表耗时为 ms/批，括号为最小–最大。吞吐增幅按相同工作量的中位耗时比值计算，A 和 B 的比例相乘，不直接相加。

| 路径 / 并发 | baseline ms ↓ | journal ms ↓ | reserved ms ↓ | A 吞吐 ↑ | B 额外吞吐 ↑ | 合计吞吐 ↑ |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| Fields / 1 | 1.919（1.914–1.947） | 1.492（1.480–1.509） | 1.326（1.319–1.400） | 28.7% | 12.5% | 44.7% |
| Fields / 10 | 0.931（0.917–0.992） | 0.728（0.724–0.737） | 0.643（0.641–0.711） | 27.9% | 13.1% | 44.7% |
| JSON / 1 | 2.419（2.403–2.611） | 2.005（2.000–2.009） | 1.825（1.808–1.883） | 20.6% | 9.9% | 32.5% |
| JSON / 10 | 1.196（1.192–1.242） | 1.002（0.999–1.025） | 0.922（0.914–0.936） | 19.3% | 8.7% | 29.7% |

单 worker 字段耗时减少约 0.593 ms/批，完整 JSON 也减少约 0.593 ms/批。这里是批处理摊销耗时，不是单条消息的延迟分位数。

| 单 worker 路径 | baseline B/批 ↓ | reserved B/批 ↓ | 字节减少 | baseline 分配次数/批 ↓ | reserved 分配次数/批 ↓ | 次数减少 |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| Fields | 1,443,852 | 1,176,139 | 18.5% | 23,628 | 9,077 | 61.6% |
| JSON | 1,665,860 | 1,398,166 | 16.1% | 28,938 | 14,387 | 50.3% |

分别少分配 267,713 B 和 267,694 B/批。JSON 平均每条消息由约 151.4 kB / 2,631 次分配降至 127.1 kB / 1,308 次；这是混合样本的累计分配摊销，不是 RSS、常驻内存、峰值或泄漏指标。

10 worker 完整 JSON 吞吐由约 9,200 增至 11,935 消息/s，应用输入字节率由 9.51 增至 12.33 Mbps。完整原始值和换算见 [reserved-summary.json](reserved-summary.json)。

### 未采用的精确容量版本也保留

第一版 batch 按当时项数精确预留容量，后续 InitNode / 状态写入触发更多增长。其单 worker JSON 比 journal 额外快约 2.4%，但累计分配从 1,568,093 增至 1,619,259 B/批；单 worker Fields 的中位耗时几乎没有差别，范围重叠。结果不足以支持保留这一容量策略。

后续仅调整 config_batch.go 的容量分档，重新冻结 reserved，并重新跑完整三阶段对照；没有将前一轮较慢的基线和后一轮较快的候选混算。第一版的全部原始结果见 [exact-capacity-summary.json](exact-capacity-summary.json) 及 long-1/2/3-* 日志。

这些指标仅适用于本次 Memcached / Cassandra 混合输入，不代表所有协议收益。应用输入 Mbps 不包含真实网卡采集、协议识别、TCP 重组、队列背压、客户端渲染或 AI 调用，不能当作无丢包带宽。

## 等价性和回归

三个冻结程序分别运行全记录导出摘要检查，包含 58,533 条封装记录和 11 条应用消息，检查字段、元数据、具体数值类型、空值形状、最终读取位置与完整拒绝错误；每份捕获摘要和记录数由样本加载器校验。封装覆盖不等于完整应用层语义覆盖，摘要等价也不能替代独立正确性断言。

新增测试覆盖：延迟公开与函数切片容量、公开后原地修改、删除并重建历史、非法类型写入后报错、自定义回调重入及浅共享、并发写入和公开读取；批量继承的 8 种存在性组合、present nil、配置覆盖顺序、复杂描述回退；共享字段树的空列表、混合字节序、后段无效跨度 / 描述及回滚、1,024 项证书列表。

第一版 batch 和最终 reserved 的 605 个选定顶层样本测试分别全部通过，均为 0 失败、0 跳过；专门针对延期协议的 23 个测试名未选入，导出摘要测试单独执行，样本清单没有删除任何捕获。选取规则和确切列表见 corpus-selection.json 与 reserved-corpus-selection.json；最终日志见 [reserved-corpus-regression.txt](reserved-corpus-regression.txt)。没有运行整个 bin-parser 或 Yaklang 仓库全量，也没有重新裁定历史协议评分门槛。

最终 baseline / journal / reserved 的 8 个固定分区摘要完全相同，也与第一版 batch 相同，见 reserved-summary.json 和各版本 export-digest 日志。全记录封装断言另覆盖 58,533 条记录，包括预期拒绝、截断记录和明确的捕获容器边界；没有把这些全部标成完整应用协议解析成功。

[收尾审计](audit.json) 确认所有选定测试都有 PASS、最终源文件摘要与冻结身份一致、四个冻结版本导出摘要一致；记录在 commands.jsonl 的 18 个正式性能进程互相及与记录的校验 / profile 作业没有重叠，全部正常退出。源码仍为本地未提交变更，原有其他工作保留，go.mod / go.sum 无改动。

最终 parser 三包回归及选定 race 检查通过，日志见 reserved-parser-regression.txt、reserved-parser-race.txt；两协议公开字段入口的 race 日志见 reserved-application-race.txt。无前缀文件是第一版结果。macOS linker 的 LC_DYSYMTAB 警告原样保留，不是 race 报告。

新增字段树测试首次运行时，其保留子节点夹具缺少 Config，触发原消费长度代码的 nil 指针异常。随后将夹具补成完整节点，并按现有事务测试建立公开结果运行时和边界；未为该夹具修改生产行为，新增测试重跑通过。

一次 reserved-identity 命令因在包目录内重复书写 common/bin-parser 路径而找不到脚本，随后在仓库根目录成功执行；发生在正式最终性能运行前，没有丢弃或改变任何正式性能结果。

## 剩余分配成本

最终程序单独采集 3 s 的 alloc_space profile，不与吞吐基准或回归重叠。[采样结果](reserved-allocation-top.txt) 的 flat 项中，配置写入约 23.7%，NewEmptyConfig 约 16.2%，NewConfigWithItems 约 15.2%。配置和通用树仍有构建成本；这些是累计分配份额，不能当成 CPU 占比，也不能承诺消除同样比例的执行时间。记录容量调整的干预结果比跨版本比较采样百分比更可靠。

## 复现

使用 [run.mjs](run.mjs)，只依赖系统 Node.js 内置模块，不改变项目依赖：

```sh
node common/bin-parser/reports/config-tree-20260907/run.mjs reserved-identity /absolute/frozen-binary-directory
node common/bin-parser/reports/config-tree-20260907/run.mjs reserved-corpus /absolute/frozen-binary-directory
node common/bin-parser/reports/config-tree-20260907/run.mjs reserved-digest /absolute/frozen-binary-directory
node common/bin-parser/reports/config-tree-20260907/run.mjs reserved-bench /absolute/frozen-binary-directory
node common/bin-parser/reports/config-tree-20260907/run.mjs reserved-profile /absolute/frozen-binary-directory
node common/bin-parser/reports/config-tree-20260907/run.mjs reserved-summary /absolute/frozen-binary-directory
```

最终目录内程序须命名 baseline.test、journal.test、reserved.test；去掉命令中的 reserved- 前缀则使用第一版 batch.test 的对照。脚本拒绝覆盖已有原始日志；复测须将脚本放入同级的新报告目录并准备对应阶段程序。仅编译当前代码不能重新生成过去两个阶段。性能计时期间不要同时构建或运行回归。

本轮 parser 回归命令：

```sh
go test ./common/bin-parser/parser/base ./common/bin-parser/parser/stream_parser ./common/bin-parser/parser -count=1 -timeout=180s
go test -race ./common/bin-parser/parser/base ./common/bin-parser/parser/stream_parser ./common/bin-parser/parser -run 'Test(Config|CompactReplay|BareTerminal|ExactByteBatch|Expression|Out|Bridge|Operator|Memcached|Cassandra|.*RuntimeIsolation)' -count=1 -timeout=180s
go test -race ./common/bin-parser -run '^TestProtocolCorpus(Memcached|Cassandra)Fields' -count=1 -timeout=180s
```

## 尚未实施

不可变解析计划、完整 JSON 直接导出、长期 worker / 双向流亲和、真实捕获容量、客户端与 aid 端到端能力仍是后续工作。本轮不扩大复杂 Yak 脚本的复用范围，也不从局部分配占比承诺倍数提速。
