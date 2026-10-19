# SSA 编译与扫描性能状态

核验时间：`2026-10-09T13:25:31+08:00`。工作区：`enhance-ssa-compile-large-porject-0917`；分支：`enhance/ssa/parallel-unit-compile`；HEAD：`19fd2ebc2c1f7467930f38c88e913f2d59b1c0b3`，另有未提交修改。

## 当前结论与验证范围

1. **2026-10-08 的优化源码快照完成了实际 CLI 对照。**同一 GoBlog 输入下，纯编译耗时减少 47.26%，包含规则执行的扫描耗时减少 36.53%。扫描峰值 RSS 减少 12.32%；纯编译峰值 RSS 增加 1.02%。
2. **最新工作区包含测试后新增的优化。**本次比对实测源码 manifest 和当前文件，确认指令查找、作用域版本链、变量查找、版本赋值标记和成员搜索有后续修改。它们尚未取得修改后的构建、回归、CLI 与 pprof 对照结果。
3. **数据库共享重构仍是设计。**Offset 打包列表、共享 range、Type payload/ref 和不可变 Type ID 复用见 [存储共享方案](ssa-storage-sharing-followup.md)。没有将这些设计的推算收益计入以下实测。

## 已完成的 CLI 对照

### 条件与来源

- 固定输入：`/home/wlz/Target/gocms/GoBlog`，SHA `b93d2284033e3a394489dc9d4429aae78378770e`。
- 对照：rebase 后基线 `yak-rebased` 与 `yak-optimized-final`，各单次串行执行，独立数据库、YAKIT_HOME 和 debug 目录。
- 环境：`GOMEMLIMIT=20GiB`；GOGC 和 SSA GC 覆盖参数未设置；诊断级别 trace。
- 优化二进制 SHA256：`dd7a27ae74651b15fd2ffcac7f13d9caea525c3cdc485d5c985187091784f9eb`。
- 扫描执行 source、struct、ssa 模式，`--rule-work-limit 50000 --rule-timeout 4h`。规则目录指纹一致：`6c393a67fbf7638697a724655af89e89ca3b36587553613374f44b0afd5a1ccf`。
- 单次对照存在动态构建、CPU 和 I/O 噪声，不能外推所有项目或其他 GC/并发设置的收益。

### 时间、CPU 和内存

| 指标 | ssa-compile 基线 | ssa-compile 优化快照 | 变化 | code-scan 基线 | code-scan 优化快照 | 变化 |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| 端到端耗时 | 275.20 s | 145.13 s | -47.26% | 290.27 s | 184.23 s | -36.53% |
| user CPU | 2,398.71 s | 1,704.06 s | -28.96% | 3,336.76 s | 2,721.58 s | -18.44% |
| 峰值 RSS | 2.290 GiB | 2.314 GiB | +1.02% | 2.799 GiB | 2.454 GiB | -12.32% |
| 最终 pprof live heap | 1,090.90 MiB | 817.78 MiB | -25.04% | 1,302.58 MiB | 1,037.59 MiB | -20.34% |
| 最终 pprof live 对象 | 10,598,837 | 8,234,678 | -22.31% | 14,485,589 | 12,060,980 | -16.74% |
| 累计分配 | 约 64.10 GiB | 52.01 GiB | 约 -18.87% | 84.63 GiB | 72.88 GiB | -13.88% |

user CPU 是全部线程累计 CPU 时间；最终 heap 是 pprof 的采样估计，不能替代进程峰值 RSS。

扫描阶段：源码规则 1.905 → 1.936 秒；编译与结构规则 232.416 → 128.784 秒；SSA 规则 37.208 → 32.478 秒。编译与结构规则阶段还包含持久化和回载交接，不能当作纯编译耗时。阶段计时也不覆盖全部 CLI 初始化及导出工作。

### 完成状态与正确性边界

- 纯编译与两版扫描均 exit 0；扫描各阶段 succeeded。
- 两版记录的 source/struct/ssa 阶段风险指标均为 123/286/19；其中 struct 的 286 使用了结果对象数量，实际保存的 struct 风险为 20 条，见下方三段规则分析。SSA 规则完成 348/348，其中成功 73、按适用条件跳过 275、失败 0。
- SARIF 均为 162 条结果，各 ruleId 的结果数量一致；这不等于已逐条证明全部证据路径或对象图相同。
- 两版未触及 work limit 或 wall timeout，同样有 183 条既有 recursive visit limit 提示。
- 没有发现这些完成运行中的 panic、SQLite lock、保存失败、扫描死锁或编译线程无限循环。纯编译报告仍记录历史悬空引用与部分动态对象图差异；动态编译语义问题按任务范围由其他分支处理。
- 已验证快照的 SSA 整包 race、dbcache/ssadb/memedit race 及 Go、Java、C#、PHP、Yak、Python、TS/JS、C 相关回归通过。SSAAPI Graph 和 ssa_compile 项目识别/创建测试存在与 HEAD 对照相同的失败集合，整个大测试组未被标为全绿。

## 重复生成、dirty 和数据库写入

纯编译数据库对照：

| 指标 | 基线 | 优化快照 |
| --- | ---: | ---: |
| Code | 503,970 | 239,903 |
| placeholder Code | 298,302 | 32,149 |
| Type | 424,138 | 158,427 |
| Index | 475,568 | 206,147 |
| 完全重复 Index | 271,451 | 0 |
| Offset | 227,473 | 221,848 |

表、列和 SQL 索引定义与基线一致，quick_check=ok。239,903 个唯一 IR 对应 240,575 次写入，剩余少量写入是 dirty 更新；结束时未清 dirty=0、pending=0、errors=0。相同 Index 绑定在入队前去重。Type 减行主要来自 placeholder 默认类型构造减少，仍有大量独立身份保存相同表示。

元数据 line_count 从重复累计的 89,431 修正为全部唯一保存文件的 44,789；源码内容一致。该口径包含 go.mod，且 MemEditor 将末尾 LF 后的空行计为一行。

匿名 IR 释放的单独诊断对照中，最终 live heap 835.69 → 817.78 MiB，resident IR 207,494 → 148,650；保留版本 wall 为 140.34 秒，释放版本为 145.13 秒。本次释放确实减少驻留，但不能将整体提速归因于这一个策略。

## pprof 热点与处理进度

### CPU

纯编译两个完整 60 秒窗口的 GC 累计占比为 75.32% 和 88.59%；code-scan 两个窗口为 72.17% 和 89.43%。`runtime.gcDrain`、`scanSpan`、`scanObjectsSmall` 等占据主要 CPU 样本。百分比属于各采样窗口，不能相加，也不能当作全程 GC 百分比。

code-scan CPU 窗口主要覆盖编译，尚无独立覆盖 SSA 规则阶段的完整 CPU 样本。累计分配和最终 heap 覆盖编译加扫描全过程，能发现分配来源，但不足以直接确定具体规则的 CPU 热点。

现阶段继续减少重复构造、临时分配及可释放引用；ANTLR 和 GC 参数保持此前约定，后续再评估。

### 分配与保留

下表数值来自已完成的优化版 code-scan pprof。累计分配是 flat 分配量；最终 heap 也是分配栈归属，不能据此证明引用链或内存泄漏。

| 热点 | 已观察成本 | 当前处理情况 |
| --- | --- | --- |
| `GetExs` 临时结果切片 | 累计 4.31 GiB | 最新代码让单 ID 直接查找、批量结果首次成功时一次分配；回归已添加，修改后未验证 |
| `LinkNode.All` 快照 | 累计 0.98 GiB | 最新版本链支持从新到旧短路 Find；首个变量和捕获查找避开整链快照，完整快照语义继续保留；修改后未验证 |
| `NewVersioned` | 最终 heap 93.01 MiB | 最新代码内嵌赋值 AtomicBool，减少独立小对象；状态/分配回归已添加，修改后未验证 |
| `NewValue`、SSAAPI Value 对象池分配入口 | 最终 heap 98.53 MiB；对象池 New 累计 2.64 GiB | 最新成员搜索先匹配原始 IR 元数据，再包装命中结果；仅覆盖候选键包装，其他包装创建成本待定位；修改后未验证 |
| GORM 构造、SQL 参数转换 | AddToVars 累计 1.29 GiB；Fields 1.25 GiB | 减少重复写入已经实测；进一步减少每批构造和转换开销尚未实施 |
| IR 数据库回载和行缓存 | 批量回载最终 heap 88.09 MiB；单条回载 40.04 MiB | 后续检查缓存及结果生命周期，在晚期 dirty 和回载仍正确的前提下减少保留 |
| OrderedMap、变量/作用域版本 | OrderedMap.Set 最终 heap 52.56 MiB；NewVariable 41.50 MiB | 先验证版本查找修改；有顺序要求的容器保留顺序，不能统一替换成无序 map |
| ANTLR `NewATNConfig` | 累计 20.44 GiB | 按当前范围暂不修改；还有 PredictionContext 等其他 ANTLR 分配，不能把 20.44 GiB 当作 ANTLR 总成本 |

纯编译结束 goroutine profile 为 571 个，569 个停在 gopark，其中约 250 个 TTL cache worker 和 250 个 singleflight 清理 worker。进程完成不证明长期服务中的缓存生命周期合理，后续需检查创建、停止和引用保留，不能仅凭阻塞状态判为死锁。

两组 CLI 均不足五分钟，五分钟 HTTP 快照不适用；已有 collector 初始、每分钟和最终快照。

## 三段规则的执行方式和优化候选

本节核验时间为 `2026-10-09T13:52:29+08:00`，依据本 worktree 的代码、既有日志、只读数据库与 pprof。以下是分析和优化候选，尚未实施新的扫描运行时修改。

### 执行顺序和计时口径

本次同时选择 source、struct、ssa：收集本地项目 → source 检查 → 编译单元构建期间执行 struct → 保存和回载为 DBRead 程序 → SSA 检查。

| 规则段 | 实际执行方式 | 本次优化快照指标 |
| --- | --- | --- |
| source / inspect | 对源码快照执行文件 glob 和内容匹配，不依赖 SSA 编译 | 595 条规则；阶段 1.936 秒 |
| struct / review | 每个完成的编译单元上执行局部语义规则，与编译、保存流程交织 | 13 种规则 × 22 个单元 = 286 次成功执行；整个 review 128.784 秒 |
| ssa / analyze | 编译后读取 DBRead 程序，执行数据流和污染传播等规则 | 348 条候选，73 条执行成功、275 条按适用条件跳过；阶段 32.478 秒 |

依据：`common/yak/syntaxflow_scan/project.go` 的 `ScanProject`，以及 `common/yak/ssaapi/struct_scan_runtime.go` 的 `scanStructWithOpcodeIndex`。本次 live source 已执行后，不会再执行 compiled-source 那一路。struct 完成标记也用于避免重复执行。关闭的 DBWrite 程序必须回载，不能为了省时间直接传给 SSA 扫描。

三个阶段占端到端 184.231 秒的约 1.1%、69.9%、17.6%，其余约 21.032 秒来自阶段外的工作。review 包含编译和持久化，不能把它全部当作 struct 规则耗时，也不能与独立 ssa-compile 的 145.131 秒相减来推导规则开销。

### source：共享只读输入和匹配准备，优先级较低

已经有的优化：文件内容在 `SourceQueryTarget` 中一次加载；`PatternRoot` 缓存同一查询的命中和 editor；大量命中分批执行；一个过滤器中的多个内容模式有组合匹配候选；源码临时 Program 不启动 audit-node TTL worker。

进一步可评估：

- 同一次扫描共享文件目录索引、相同 glob 的文件集合，以及不可变的匹配准备结果。当前 `MatchRegexpHits` 对每个过滤器遍历 files；不应为每条规则重复遍历所有路径。
- 共享文件/editor 的只读描述，规则的 `PatternRoot`、batch offset、变量和结果状态保持独立。当前 SourceQueryTarget 和 QuerySyntaxflow 各创建一次 root，且跨规则不共享 root 的 editor 缓存；不能直接共享带可修改 batch 状态的完整 root。
- 对确定没有匹配文件的过滤器快速完成，保留现有无匹配、错误和检查消息的区别。

本例整段仅 1.936 秒，优先做小改动；不据此优先重写整个正则引擎。

### struct：检查重复解码和空结果处理

已经有的优化：内置规则带编译 opcodes，不会每个单元都解析规则源码；完成批次按 opcode 建 resident 索引，单元再缓存自己的指令集合；批次/单元索引完成后释放；持久化成功后清理结果持有的主要图引用。

仍然可减少的重复工作：

- **复用解码后的不可变 opcodes。**`frameForRule → VM.Load → ToOpCodes` 仍会 JSON 解码；本例 13 种规则有 286 次执行。缓存 key 应包含规则内容、opcode schema 和编译版本；每次建立独立 frame、上下文、变量表和执行栈，不共享整个可修改 frame。
- **空结果走轻量保存。**日志中 283 次执行没有 alert；只读 SQL 确认 286 条 audit_results 中 283 条 risk_count=0。空结果没有 audit_nodes，已无整图节点保存；可减少 audit 保存器初始化、逐结果任务和重复元数据编码，并批量保存结果头。完成记录、检查消息、错误、结果 ID 与报告读取约定仍须保留。
- **异步保存任务可合并。**当前一个单元启动一个保存 goroutine，再逐结果 Save；评估有界批处理，避免排队结果继续持有编译图。不能先释放后保存，也不能用增加保存并发代替减少写入。
- 给纯 struct 执行、IR 保存及 audit 保存分别计时。当前规则统计使用整秒时间，短规则无法据此精确排序。

同时发现统计口径问题：`StructScanCounts` 返回规则种数和结果对象数；`stageOutcomeRecorder.observeStruct` 把后者用于 RiskCount。此库实际有 20 条 struct ssa_risks，SARIF 为 exec.Command 16 条、gob.Decode 4 条。286 是执行/结果数量，不能当作独立风险数量。后续统计修正需单独回归，不计为性能收益。

### SSA：优先检查回载、包装和跨规则重复查询

最值得优先验证的已有修改是 GetEx/GetExs、单项变量版本查找以及未命中成员不创建包装。本轮对既有 heap 做 `go tool pprof -alloc_space -list='ssaapi.init.func23'`，确认 2.64 GiB 对应 `common/yak/ssaapi/values.go` 中 Value 对象池的 `New: return &Value{}`。旧报告将该自动生成函数名解释为 native callback 不准确，已更正本文件。

其他候选：

- **IR 回载和基础查询复用。**最后 57.282 秒 SQL 统计窗口有 81,646 次 query、累计 42.436 秒；窗口同时覆盖 review 尾部与 analyze，不能全部归给 SSA 或直接相加为 wall 时间。该窗口已记录的慢 SQL 集中于 ir_codes 批量和单条回载。按 rule 标记统计查询种类、重复 ID 和缓存 miss，再评估批量预取、同 ID 回载合并、不可变候选 ID 缓存。
- **完善已有 include 缓存。**`sf_native_call_include.go` 已缓存成功结果，不能再把“增加 include 缓存”当作全新优化。当前相同 key 的并发 miss 可以重复执行；无 alert 值的完成结果不进入缓存。可评估同 key 查询合并、完整空结果的独立缓存标记；保留现有空结果返回语义，不将超时、取消、部分结果或错误视为已证明为空。key 要保留 program、规则内容、输入、struct 单元和有效扫描范围。
- **只共享不可变信息。**可复用 IR 行、类型描述、基础检索候选；`Value` 的 UID、Predecessors、anchor、EffectOn/DependOn 和规则上下文保持隔离。现有 include cache 命中也会创建独立 Value；不能直接以 IR ID 合并所有查询包装。
- **按最慢规则定位遍历。**状态日志中 Go 文件上传约 33 秒、开放重定向约 21.9 秒，是 wall 粗计时，不能作为 CPU 排名或相加。文件上传规则包含多个 include 和重复数据流过滤，适合下一轮补规则/opcode 计时；遍历缓存必须考虑方向、条件、输入、anchor 和预算，不能仅按 value ID 跳过。
- **并发按内存成本分组评估。**默认扫描并发由 `GOMAXPROCS-1` 推导，多条重数据流规则会同时构造图。先测在途图大小和 DB 争用，再评估重/轻规则的并发上限；不要直接提高并发或改变规则预算来宣称优化。

### 三段共用的调度、进度和保存

- `StartQuerySF` 当前先取得并发槽、创建 goroutine，再在 `Query` 判断语言/mode 是否匹配；跳过后也 defer SaveTask。本次 275 条 SSA skip 可以在调度前判定，继续保留 skip 原因、完成计数、任务顺序和恢复约定。
- `SaveTask` 对每个规则任务持锁、JSON 序列化整个 config 并保存任务。可先冻结/缓存不变的配置编码，再统计保存次数，评估 dirty 状态合并检查点；终态、暂停和恢复语义须有回归，不能直接取消所有检查点。
- 实测日志包含 inspect/review/analyze 进度输出 599/950/263 次。可在 CLI 显示层合并同进度和中间快照；完成事件、结果回调、取消响应和错误保留。VM 已限制长规则的 opcode 进度回调，不能简单重复设计这一层。
- 任务结果缓存和 include cache 会保留结果/Value。明确报告、查询和回调消费者结束后的生命周期，再缩短不需要继续读取的引用；现有 ResetInterRuleState 有数量阈值，未超过时已快速返回，不能改成每条规则都全量清理。

规则之外还存在冷启动成本：此运行日志约 18:02:12 开始内置规则同步，18:02:26 进入扫描阶段，另有进程初始化。同步已有内容哈希判断，这次使用全新 run-local profile DB，不能把冷库同步说成“每次都重复更新”。可单独比较冷库/已同步库和规则解码成本，保持规则版本与内容一致。

建议顺序：先验证最新分配修改 → 调度前 skip、配置编码复用、空结果保存批处理与 opcode 解码复用 → 分段/分规则采样确认 IR/include 重复查询 → 依据实测决定图状态生命周期和并发策略。以上候选均不要求拆分当前 IR 数据库表。

## 最新修改与当前验证阻碍

本次对 `goblog-final/source-manifest.json` 与当前源码进行 SHA256 比对，发现已记录的 `common/yak/ssa/variable.go` 又被修改；下列运行时文件在该快照之外新增了修改：

- `common/yak/ssa/instructoinId.go`
- `common/yak/ssa/ssautil/limited_link.go`
- `common/yak/ssa/ssautil/scope.go`
- `common/yak/ssa/ssautil/versioned.go`
- `common/yak/ssaapi/sf_search.go`

新增回归覆盖单条/批量指令查找、版本/父作用域查找顺序、快照独立性、赋值状态、未命中成员不创建包装以及命中结果独立状态。测试中的零/一次分配是验收目标，尚未证明最新实现满足目标。

本轮实际尝试 `go build -o yak ./common/yak/cmd/yak.go`，退出码 1。默认 `GOCACHE=/mnt/data/cache/go-build`，错误为 `open /mnt/data/cache/go-build/...: read-only file system`，构建未完成；这不是修改后测试通过或源码编译成功的证据。

当前沙箱允许写 worktree 和 `/tmp`，不允许写 `/mnt/data/scan-outputs`。依照工作区输出规则，扫描、扫描编译与相关测试的数据库、profiles、日志和报告必须位于 `/mnt/data/scan-outputs`。因此本轮未启动新的扫描或扫描相关测试，也没有另建缓存来绕过默认路径限制。

后续验证顺序：恢复默认缓存和固定扫描输出路径的写入能力 → 通过 `scripts/ssa-test.sh` 执行最新回归及相关语言/race 检查 → 构建最新 CLI → 在相同输入/规则/GC 条件下分别重跑 ssa-compile 和 code-scan → 分别采集编译与 SSA 规则阶段 CPU，比较 peak/live heap、累计分配、SQL 写次数和输出。完成前，旧快照的 145.13/184.23 秒不能当作最新源码测试结果。

## 本机证据目录

根目录：`/mnt/data/scan-outputs/ssa-memory-reuse-20261008-1550/`。

- `goblog-final/summary.md`、`timing.json`、`comparison.json`、`source-manifest.json`、`run-meta.json`：纯编译对照及源码/二进制来源。
- `codescan-summary.md`、`codescan-comparison.json`：扫描对照、规则指纹和结果计数。
- `goblog-codescan-baseline/`、`goblog-codescan-optimized/`：命令、计时、数据库、SARIF、CPU/heap/goroutine profiles 与 runtime 时间序列。
- `tests/baseline-failure-comparison.json`、语言及 race 日志：已验证快照的回归结果。
- `final-verification.json`：2026-10-08 18:13 的源码、CLI 和扫描来源核验，仅代表当时快照。
