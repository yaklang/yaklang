# SSA 并行单元编译：开发与门禁主文档

本文件是这条开发线的**唯一主文档**，随分支提交演进。证据目录
`/mnt/data/scan-outputs/ssa-review-20261019/artex-ir-delta-20261020/` 只保存原始
产物和本文件的同步副本，不另行维护第二套结论。

- 分支：`test-1/for-artex/enhance/ssa/parallel-unit-compile`
- Worktree：`/home/wlz/Developer/yaklang-workspace/test-1-for-artex-enhance-ssa-parallel-unit-compile`
- 当前 HEAD：`c0e7b8358912718be98e8e1a620e9532914be052`
- 推进顺序（硬约束）：**功能修好 → 小项目达到预期 → 大项目**。不跳步、不并行抢跑。
- 状态约定：**已证实**＝有可复现命令/夹具或直读产物支撑；**待验证**＝仅推断或样本不足。

---

## 0. 当前状态看板

| 项目 | 状态 |
|---|---|
| 阶段 | **功能修复 / 小项目验证中**（未进入大项目） |
| 小项目（Go / ARTEX 821 文件快照） | **未达标**：多次运行结构计数、callee 语义、offset 源区间集合均不稳定 |
| 大项目（Java/PHP/Python/Go/C） | **未启动**（门禁禁止） |
| yaklang 全量自扫描 | **未启动**（门禁禁止） |
| 语言门禁体系 | 仅完成 A 线；B 的语言能力盘点与统一入口**未实现**，只做了口径设计 |
| 最近一次运行 | `signature-regression/go2ssa-all`（`2026-10-21T12:11+08:00` 起，exit 1，wall 463.14s；唯一失败 `Test_InterfaceMethodArgumentTypeCheckRegression`，报 `database is locked`） |
| 最近一次确定性结论 | 声明绑定阶段建签名后，指针/值/多返回工厂与跨包提前 import 的正反 body 顺序已绿（`declaration-order-regressions` exit 0，0.581s）；但 go2ssa 全包未绿，且 ARTEX 尚未用新二进制复跑 |
| 未解决高优先级问题 | ISSUE-006 / 007 / 008 / 009 / 010 / 011 / 012 / 015（见 §6） |

### 0.1 各语言门禁状态

| 语言 | 小项目 | 大项目候选 | 门禁状态 |
|---|---|---|---|
| Go | ARTEX（821 文件冻结快照） | yaklang 自身（10333 go） | **进行中，未达标** |
| Java | 未选定/未运行 | javacms/core（7767）或 spring-project（32377） | 未开始 |
| PHP | 未选定/未运行 | moodle（7732） | 未开始 |
| Python | 未选定/未运行 | pythoncms（4281） | 未开始 |
| C | 待确认现有夹具与支持程度 | — | 未开始 |
| JavaScript / Yak | 未盘点 | — | 未开始 |

---

## 1. 小项目验收口径（硬门禁，不可放宽）

小项目（当前为 ARTEX）必须**同时**满足：

1. 多次运行的结构计数与调用边一致。任何计数浮动必须有机制解释，并证明不影响语义；
   不允许"给出一个区间就算通过"。
2. 调用点 callee 解析正确，`Undefined` 有效性分类（`UndefinedMemberValid` /
   `UndefinedMemberInValid`）不随运行变化。
3. `ir_offsets` 的**源区间集合**稳定，不是只比较行数。
4. 原有测试、`-race`、go2ssa 全包全绿。历史旧 SHA 的绿不能替代最新修改后的验证。
5. 运行在既有时间/内存预算内，3GiB `MemAvailable` 护栏不触发；不改默认并发/超时/预算。
6. 门禁脚本可重复执行，第二次运行与同身份基线一致；失败样本不得用来生成允许漂移的基线。

每次运行（小项目/大项目/诊断对照）强制：独立 run 目录、独立 DB、独立
`YAKIT_HOME`/`TMPDIR`/`GOTMPDIR`，并保留 CPU / heap / goroutine(debug=2) /
mutex / block / allocs / runtime / db-stats 全部证据。profile 是后续优化输入，
不是可选附件。

---

## 2. 提交记录

> 规则：每次 commit 追加一节。格式固定为「SHA + 标题 + 文件/行数」「问题与根因（已证实/待验证）」
> 「测试：命令、范围、真实退出码」「随后运行记录」。本文件建立时对此前提交做回溯补录。

### 2.1 `c732096e4e` perf(ssa): drop settled ordinary versions from scope chains

- 改动：`common/yak/ssa/scope_value_release.go`、`common/yak/ssa/ssautil/limited_link.go`、
  `common/yak/ssa/ssautil/scope.go` 及 2 个测试；5 文件 +120 / −2。
- 问题：版本链（scope chain）保留所有已 settled 的普通版本，长链让遍历与查找随版本数线性变慢。
- 根因（已证实）：`limited_link`/`scope` 的读路径逐节点遍历；普通版本一旦 settled 不再影响后续语义，
  却仍留在链上。
- 测试：`common/yak/ssa`、`common/yak/ssa/ssautil` 相关用例；`-race` 在上述两包通过（详见
  `ssa-review-20261019/yak-memory-20261020/`）。
- 运行记录：见 §3 R-1 / R-2（ARTEX 与 rebase 后复跑）。该提交后 ARTEX `ir_codes` 与
  `ir_offsets` 出现变化，归因仍未完全闭合（见 ISSUE-005）。

### 2.2 `65f4f3dbfa` fix(ssa): read version chains under the scope lock

- 改动：`common/yak/ssa/ssautil/scope.go`、`common/yak/ssa/ssautil/concurrent_link_test.go`；
  2 文件 +88 / −5。
- 问题：并发读版本链时未持锁，`-race` 报 21 个 data race（`Last`/`Append`、
  `GetAllVariableNames`/`ForEach`）。
- 根因（已证实）：链节点的读操作绕过了既有 `linkMu`；不是缺少新锁，而是读路径没进锁。
- 决策：**不**在 `LinkNode` 内新增 mutex（会引入新的锁层次与拷贝成本）。
- 测试：`TestConcurrentReadTrim` 加入回归；`-race` 三包通过。
- 运行记录：无独立项目运行（race-only 修复）。

### 2.3 `60d30c12d5` fix(ssa): settle index dedup only for flushed ids

- 改动：9 文件 +429 / −25（`database_cache_index.go` 为主，含 `index_release_test.go` 等）。
- 问题：索引去重水位把"已分配但尚未 flush"的 ID 也当作已落盘，导致后续新关系被丢掉，
  `ir_indices_v1` 与 offset 关联不完整。
- 根因（已证实）：水位取的是 `nextID+1` 而非"最后一次成功 flush 的 value id"；`IrIndex`
  本身没有唯一约束，只有非唯一 `idx_ir_indices_program_value`。
- 测试：新增 `TestReleaseSettledKeepsUnflushedIndexWritable`、`FindByVariableEx`、去重用例；
  `-race` 三包通过。
- 运行记录：R-3（`artex-20261020-1600-60d30c12d571`）**未通过门禁**：
  `ir_codes=426884`（相对参考 +37799）、`ir_offsets=247012`（−3912）。

### 2.4 `6480924e2a` fix(ssadb): restore dialect index check after rebase

- 改动：`common/yak/ssa/ssadb/database.go`；1 文件 +9。
- 问题：rebase 后 `hasNamedIndex` 方言判断丢失，索引存在性检查恒走错分支。
- 根因（已证实）：rebase 冲突解决时该helper被丢。
- 测试：ssadb 包用例 + 后续 core/ARTEX 复跑。
- 运行记录：R-4（`core-20261020-142004-6480924e2a55`，core 50 分钟上限内完成，用户认可 core 达标）。

### 2.5 `958fd9e8b4` fix(ssa): 导出集在首次 PreHandler 确定，import 绑定延迟到 Binding 阶段

- 改动：4 文件 +496 / −13
  - `common/yak/go2ssa/builder_ast.go` +163
  - `common/yak/ssa/member_relations.go` +16
  - `common/yak/go2ssa/test/export_method_resolution_test.go` 新增 94
  - `common/yak/ssaapi/go_declaration_reference_test.go` 新增 236
- 问题：并行单元编译下，跨包静态方法调用的 callee 解析到新发射的 `Undefined`，而不是声明处
  创建的方法函数；同时 import 命名空间在不同运行拿到不同的导出成员数（少 60 个 `DB$*`、
  整文件缺 `DB$SetBool` 键），且成员关系正反两向用了两个不同 key。
- 根因（已证实，三处独立）：
  1. 方法分类过晚：`buildMethodDeclFront` 只在 lazy body 真正构建时才
     `fun.Type.IsMethod=true`，导出遍历 `!IsMethod()` 于是把接收者方法发布成包级函数。
  2. 导出集物化时序：`handleImportPackage` 在依赖的 prehandler 尚未跑完全部文件时就
     `ExportValue.ForEach`，读到顺序相关的部分快照。
  3. 成员关系键：`AddObjectKeyPair`（反）+ `AddMember`（正）各发射一个 const 键，只共享字符串。
- 修复要点：
  - 导出集在首次 prehandler 按 Go 规则确定：排除接收者方法、`main`、未导出名
    （`goExportedName`：首字符 `unicode.IsUpper`）、与类型同名的合成构造函数；全局变量同样过滤。
  - 方法在声明阶段即 `SetMethod(true)`，`bindGoMethod` 挂到接收者类型方法集与蓝图；
    类型未就绪时捕获声明库并注册 `DeferredBuildKindBinding` 重试。
  - import 命名空间改在 Binding 阶段一次性绑定完整 `ExportValue`。
  - 新增 `BindMemberRelation`：同一 key 写正向与反向，幂等。
  - **方法调用保持成员调用表示**（承载接收者与参数，保数据流），身份经
    `GetMethod(receiver.Type, name)` 解析到声明函数。首版曾直接返回函数本体，
    破坏 topdef/side-effect/接口/闭包断言，已回退。
- 测试（真实退出码）：
  - `scripts/ssa-test.sh`：`./common/yak/go2ssa/...` exit 0（`go2ssa` 0.009s、`go2ssa/test` 18.214s）。
  - 四个新回归全 PASS：`TestGoImportedStaticMethodCallees`（并发 1/4 × 值/指针/返回值接收者）、
    `TestGoPreHandlerMethodDeclarationIdentity`、`TestGoPreHandlerEarlyImportBindsCompleteExportSet`、
    `TestGoPreHandlerImportRelationUsesSingleKey`。
  - `-race`：`ssa` 14.748s、`ssautil` 1.162s、`ssaapi` 48.001s、`go2ssa/test` 90.506s，
    全部 `ok`，`DATA RACE=0 FAIL=0`。
  - `go build ./common/yak/cmd/yak.go` exit 0。
- 运行记录：R-5（`artex-958fd9e8b4-20261021-033904`）——见 §3，**未通过**。

### 2.6 `c0e7b83589` fix(ssa): 具名非结构类型的方法也挂到类型方法集

- 改动：`common/yak/go2ssa/builder_ast.go`、`common/yak/go2ssa/test/export_method_resolution_test.go`；
  2 文件 +42 / −7。
- 问题：`type Status string`、`type companyScopeInputs []db.ScopeInput` 这类定义类型的接收者方法
  在导出表里是 `AliasType`，而 `bindGoMethod` 只对 `*ssa.ObjectType` 调 `AddMethod`，
  方法函数存在且 `is_method=true`，但调用方 `s.Terminal()` 无法从接收者类型解析回声明。
- 根因（已证实）：`bindGoMethod` 的类型判断过窄；`AliasType` 同样有方法集
  （`baseType.AddMethod`），被跳过。
- 修复：只要类型实现 `AddMethod(string, *ssa.Function)` 就挂上；指针接收者 verbose 标记仍只用于结构体。
- 测试：新增 `TestGoNamedNonStructMethodResolves`；与既有导入静态方法回归一起
  `-count=1` exit 0（`go2ssa/test` 0.161s）。
- 运行记录：R-6（`artex-c0e7b83589-20261021-040658`）——见 §3，**未通过**。
- 说明：本次提交**未**附带 `-race` 全量重跑；`-race` 绿仍是 `958fd9e8b4` 的结论，不能自动覆盖
  本次改动（见 ISSUE-010 旁注与 §5 待办）。

---

## 3. 运行记录

> 目录约定：`/mnt/data/scan-outputs/ssa-review-20261019/artex-ir-delta-20261020/`。
> 除注明外，目标均为 phase2 冻结快照 `phase2-20261020-213104/source-snapshots/artex`
> （821 文件，manifest sha256 `f969ffeb972f95932b65909dabb39341facac06a5a07f4affbc7443eb81310a8`）。

### R-1 ARTEX 参考运行（历史参考，不冒称单并发）

- 项目/版本：ARTEX，同上快照。
- 编译器：`1c952ca9c7`。该早期 run 的二进制 hash/快照 manifest 尚未从本目录证据恢复，记为 **待核对**；不可把 phase2 后来重建的 `736484cd…` hash 冒充为此 run 的 binary hash。
- run 目录：`artex-optimization-20261016/artex-20261019-1934-1c952ca9c7cf/`；DB 同目录 `ssadb.db`。
- 命令：`yak code-scan -t <snapshot> -l golang --database <run>/ssadb.db --format irify --output <run>/report/artex --no-save-risk --debug <run> --log-level info`。
- 结果：exit 0；关键计数 `ir_programs=1, ir_codes=389085, ir_offsets=250924, ir_sources=396`；
  `ConstInst=194152`，`Call=20729`，`Function=4258`。
- profile：`cpu-pprof/`、`memory-pprof/`、`goroutine-pprof/`、`mutex-pprof/`、`block-pprof/`、
  `allocs-pprof/`、`db-stats/`、`runtime-stats/` 均非空。
- 差异/判定：作为历史参考。**注意**：其命令未显式设置 `compile_concurrency=1`，
  不能称为"已证实单并发基线"（本文件早期版本曾误称，已更正）。
- 暴露问题：无（但它是后续比较的计数基准之一）。
- 下一步：以同快照做多次同构建重复。

### R-2 ARTEX 修复链中段（60d30c12d5，并行实现）

- 编译器：`60d30c12d5`，binary sha256 `4aa904a6803bad04397f2cb12adb139b1621d204b74d45c87e818ea0fcb46159`。
- run 目录：`artex-optimization-20261016/artex-20261020-1600-60d30c12d571/`。
- 结果：compile exit 0，`ir_programs=1, ir_codes=426884, ir_offsets=247012, ir_sources=396,
  ir_types=311304, ir_indices_v1=204799`；`DB$SetBool` 常量数以本 run 的只读 DB 普查为准，不纳入严格同二进制方差组。
- profile：八类目录齐全（`*-pprof`、`db-stats`、`runtime-stats`）。
- 差异/判定：相对 R-1 `ir_codes +37799`、`ir_offsets −3912` → **门禁未过**。
- 暴露问题：ISSUE-005（offset 身份漂移）、ISSUE-006（重复运行计数不稳）。
- 下一步：同 SHA 重复 4 次量化方差。

### R-3 同构建重复（`60d30c12d5`，4 次，默认并发）

- 目录：`phase2-20261020-213104/nondeterminism-default-20261021-003417/run-01..04`。
- 结果：`ir_codes` = 430604 / 419690 / 415784 / 419267；`ir_offsets` = 247047 / 246296 / 246279 / 248549；
  四次 `Call=20729`。
- 判定：**同二进制、同输入、默认参数下已存在显著运行期方差**。已证实。
- 暴露问题：ISSUE-006。
- 下一步：加单并发对照，判断是否并行边界导致。

### R-4 core 项目复跑（`6480924e2a`）

- 目录：`ssa-review-20261019/core-rerun-final-20261020/core-20261020-142004-6480924e2a55/`。
- 结果：wall 2297s（<50min 上限），`scan_exit=0`，`Risk=10893`，final barrier 干净。
- profile：五/十/十五/二十/二十五分钟快照与 final 共六件套齐全。
- 判定：core 达标（用户认可）。
- 暴露问题：无新增。

### R-5 ARTEX `958fd9e8b4`（修复后首跑，默认并发）

- 编译器：`958fd9e8b4`（clean HEAD），binary sha256 `de98cdd64280aa1fd6eaa81c015d44ee0d1159fa7805e7e2ebe302a8f30ff0cb`。
- run 目录：`phase3-20261021-0210/artex-958fd9e8b4-20261021-033904/`；
  DB `<run>/ssadb.db`；`YAKIT_HOME=<run>/yakit-home`；`TMPDIR=<run>/tmp`。
- 结果：exit 0，wall 109s；`ir_programs=1, ir_codes=290608, ir_offsets=246871, ir_sources=396,
  ir_types=175096, ir_indices_v1=203563`；`ConstInst=97120`；`DB$SetBool` 常量 0。
- 关键结构 vs R-1：`Call/BinOp/If/Jump/Return/Parameter/Next/Loop/Switch/ExternLib/UnOp` **完全一致**。
- profile：八类目录非空（cpu 2 / memory 3 / goroutine 6 / mutex 3 / block 3 / allocs 2 /
  db-stats 3 / runtime-stats 3）。该构建的 collector 不打印 phase2 的 `[observe]` 行与
  `127.0.0.1` 监听地址，wrapper 的严格探针据此提前退出；**扫描与产物本身完整**
  （已人工核对全部目录与 `code scan done`）。此为 wrapper 缺陷，见 ISSUE-013。
- 差异/判定：
  - `ConstInst` 少约 97k：接收者方法与未导出函数不再被发布成包级成员键（符合修复预期，
    `Type$method` 每 body 常量 290 种/10624 行 → 0）。
  - `DB$SetBool` 常量为 0，与 R-1 一致；R-2 的 144 行 `DB$*` 键才是要修的问题。
  - 但 `Phi/FreeValue/ParameterMember/Undefined/SideEffect` 与 R-6 相差数百 → 未过门禁。
- 暴露问题：ISSUE-006/007/008/011。
- 下一步：同构建重复。

### R-6 ARTEX `c0e7b83589`（别名方法修复后，默认并发）

- 编译器：`c0e7b83589`，binary sha256 `c758f9118a5d50a3e79252e11a39e9cc1850e75cafcd51ea7b158b54d6044536`。
- run 目录：`phase3-20261021-0210/artex-c0e7b83589-20261021-040658/`。
- 结果：exit 0，wall 111.02s；`ir_codes=290616, ir_offsets=249025, ir_indices_v1=203734,
  ir_types=175174`；`DB$SetBool` 常量 0。
- profile：八类目录非空。
- 差异/判定：与 R-5 相比 `Phi 7912→8136`、`FreeValue 14825→14517`、`ParameterMember 10004→9880`、
  `Undefined 24564→24495`、`SideEffect 5297→5562`。两构建不同，**不能当作同二进制方差证据**。
- 暴露问题：ISSUE-006/007/008。
- 下一步：固定单一构建做 4 次默认 + 单并发对照（即 R-7）。

### R-7 A 稳定性实验（同一构建 `c0e7b83589`，9 次串行隔离运行）

- 目录：`phase3-20261021-0210/A-stability-20261021-1025/`。
- 二进制：`binary/yak`，sha256 `c758f9118a5d50a3e79252e11a39e9cc1850e75cafcd51ea7b158b54d6044536`（全部扫描同一二进制；扫描期间 tracked diff 为空）。
- 输入：同上快照；每次运行前后逐文件校验 manifest，全部通过。
- 顺序：完整 smoke（106.11s，不计入正式样本）→ 默认 4 次 → 配置入口默认对照 1 次 →
  `compile_concurrency=1` 3 次。全串行，无大项目。
- 命令：同上；单并发通过 `--config` 的 `SSACompile.compile_concurrency` 设置，日志确认
  `concurrency=1 peak_active=1`。**没有**用 `GOMAXPROCS` 冒充。
- 结果（默认）：

| 样本 | ir_codes | Phi | FreeValue | ParameterMember | Undefined | SideEffect | ir_offsets |
|---|---:|---:|---:|---:|---:|---:|---:|
| default-01 | 290837 | 8049 | 14909 | 9987 | 24502 | 5421 | 246692 |
| default-02 | 291736 | 8237 | 15323 | 9920 | 24520 | 5814 | 251787 |
| default-03 | 289323 | 7851 | 14022 | 9879 | 24474 | 5125 | 245854 |
| default-04 | 290739 | 8095 | 14636 | 10010 | 24483 | 5551 | 250381 |

- 结果（单并发诊断）：

| 样本 | ir_codes | Phi | FreeValue | ParameterMember | Undefined | SideEffect | ir_offsets | 产品结果 |
|---|---:|---:|---:|---:|---:|---:|---:|---|
| config-auto | 290379 | 7997 | 14493 | 10038 | 24484 | 5347 | 247673 | 完成 |
| single-01 | 291419 | 7738 | 14277 | 10600 | 24687 | 5533 | 250654 | 完成 |
| single-02 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | **失败，不纳入方差** |
| single-03 | 292048 | 7796 | 14456 | 10688 | 24698 | 5728 | 251796 | 完成 |

- `single-02`：CLI 进程退出码确为 0，但日志明确
  `SaveIrIndexBatch failed: database is locked` → `FlushAuxSavers: index store flush failed` →
  `ssa_compile_project.go:264 cleaning up program data due to error` → review `status=failed`。
  **判失败**，不得当作"全零稳定样本"。见 ISSUE-009/010。
- 语义核查（源锚点归一化，非跨 run 原始 ID）：

| 对照 | 调用点 | callee 有效性分类变化 | callee opcode 变化 | offset 独立区间 removed/added |
|---|---|---:|---:|---:|
| default-01 vs 02 | 20729 全匹配 | 24 | 15 | 145 / 192 |
| default-01 vs 03 | 20729 全匹配 | 14 | 5 | 163 / 183 |
| default-01 vs 04 | 20729 全匹配 | 268 | 12 | 432 / 177 |
| single-01 vs 03 | 20729 全匹配 | 31 | 14 | 233 / 243 |

- 已证实：`Function` 亦出现 `4261/4261/4261/4259`，`BasicBlock` `39691/39691/39691/39689`，
  浮动不止用户列出的五类。
- 已证实：**单并发完整样本之间也存在 callee 语义与 offset 集合差异**，故不能归因为"并行边界"
  或"Phi 合并顺序"单一原因。
- 已证实（跨修复）：phase2 两个严格同二进制旧样本（`60d30c12d5`，codes 430604 / 419690）
  同样有 callee kind 变化 29 处、opcode 变化 1 处、offset 集合 59/63。语义不确定性早于本次修复存在。
  待验证：当前剩余差异是否全部同一根因。
- profile：9 次运行八类目录均非空，goroutine debug=2 保留；CPU/heap/allocs/mutex/block 全部
  经 `go tool pprof` 符号化，落在各 `profile-analysis/*-top.txt` 与 `profile-summary.json`。
- 预算：默认 4 次 wall 110.08–114.08s，RSS 峰值 3.68–4.20GiB；全轮最低 `MemAvailable`
  约 19.66GiB（>3GiB），guard 未触发。全部运行在 300s 前自然结束，故无 5 分钟快照时点。
- 判定：**未通过**。`verify_a_gate.py` → `validation-01.json`：`passed=false`,
  `large_project_allowed=false`, `baseline_promoted=false`，真实退出码 1。
- 暴露问题：ISSUE-006/007/008/009/010/011/012/013。
- 下一步：修 `serial-signature` 路径（§4），再以新构建复跑。

---

## 4. 机制分析：为什么 body 顺序会改变 callee 语义

> 当前阶段（2026-10-21T12:30+08:00）：声明绑定阶段签名与位置 Tuple 键已落到未提交工作区；精确顺序回归已绿，但 go2ssa 全包因到 SQLite `database is locked`，不能说修复已完成，更不能说 ARTEX 已达标。

### 4.1 现象

夹具（真实 Go 前端，单线程，显式指定 body 顺序，无 sleep、无概率复跑）：

```go
type Service struct{}
func (s *Service) Value() string { return "value" }
func New() *Service { return &Service{} }
func Use() { s := New(); s.Value() }
```

`TestGoSerialBodyOrderPreservesStaticCallee`，`-count=1`：

- 顺序 `Service$Value → New → Use`：PASS。
- 顺序 `Use → New → Service$Value`：FAIL，
  `undefined[#23.Value]` 的 Kind 为 `UndefinedMemberInValid`（2）。
- 真实退出码 1；测试自身 0.109s（含前置构建，整轮 wall 91.309s）。
- 证据：`A-stability-20261021-1025/serial-body-order-red/`。

### 4.2 链路

1. `builder_ast.go` `buildFunctionDeclFront` 用 `b.NewFunc(funcName)` 建出函数身份并登记变量，
   但 `buildSignature` 与 `handleFunctionType` 仍在 `AddLazyBuilder(...)` 的闭包内执行。
   ⇒ **函数类型（参数类型、返回类型）在声明阶段是空的**，只有 body 真正构建后才补齐。
   `buildMethodDeclFront` 同理。
2. `Use` 的 body 先构建时，`s := New()` 的返回类型未知（`New` 的类型还没有 result），
   于是 `s` 的类型不是 `*Service`。
3. `s.Value()` 走 `member_call.go getFieldValue` → `checkCanMemberCallExistEx`；
   后者用 `value.GetProgram().getMethod(objTyp, "Value", true)`（peek=true，不触发构建）。
   接收者类型不是 `*Service`，方法集查不到 ⇒ `res.exist=false`。
4. `member_call.go createDefaultMember` 的 `writeUndefined()` 依据 `res.exist` 固化
   `un.Kind = UndefinedMemberInValid`。
5. 之后 `New` 与 `Service$Value` 的 body 构建补齐了签名与方法体，**但已经固化的 callee
   分类不会被回填修正**。⇒ 最终 IR 里留下 invalid callee。

### 4.3 为什么"并发=1"仍然受影响

- `compile_unit.go` 的 `deferMethodBuildOnLookup/buildOnLookup` 是**前端调度策略**，
  与 worker 数无关：并发单元构建启用后，lookup 不再顺手 drain 对方的 body，改由调度器决定。
- 因此 worker=1 仍可能出现"调用方先建 body、被调方后建 body"。
- 另外 `lazy_builder.go drainLazyBuilders` → `forEachFunction` → `p.Funcs.Keys()`，
  以及蓝图方法集遍历，都是 Go map 迭代；`common/utils/map.go` 的 `SafeMapWithKey.Keys`
  直接 range。官方规范：map 迭代顺序未指定、不同次迭代不保证相同
  （核验 2026-10-21T11:12:46+08:00，https://go.dev/ref/spec#For_range）。
  ⇒ 即使串行，**body 构建顺序本身**在多次运行间也不确定。

### 4.4 能解释多少 ARTEX 差异（已证实 / 待验证）

- **已证实**：这条路径足以把"静态可判定方法调用"变成 `UndefinedMemberInValid`，
  并在单线程下稳定复现，因此它能解释至少一部分 callee 有效性分类变化。
- **已证实**：`single-01 vs single-03` 的 31 处 kind 变化 / 14 处 opcode 变化中，至少
  `norma/llm/accumulate.go Accumulator.Add` 的 `a.flushThinking()`（ParameterMember ↔
  UndefinedMemberInValid）与 `agent/tools_insert.go` 的 `t.as.GetByIDsInScope(...)`
  属于"接收者类型/方法可见性未就绪"这一族。
- **待验证**：24 / 15 / 5 类浮动各自有多大比例由本条解释；是否还存在第二条独立路径
  （例如版本链裁剪、Phi incoming 合并、offset 写入竞争）。
  **处理办法**：修复本条后以同构建复跑；若仍有剩余差异，继续抓下一条，直到清零或全部可解释。
  在此之前不宣布"同源"。

### 4.5 修复方案选择

- 方案 A（采用，验证中）：**在声明绑定阶段（所有 Front/PreHandler 类型声明已收集、body 尚未构建时）就固定签名**——参数类型、返回类型、接收者归属。沿用现有签名解析器一次性建立参数/命名返回值的声明节点与作用域，不重复发射；lazy body 只构建语句、块、表达式。不提前构建任何函数 body。
- 方案 B（排除）：首次被引用时"确定性先建签名"。理由是它把构建副作用带进查询路径：
  查询时机不同仍会改变 ID 分配与类型实例归属，且并发查询会引入锁与重入风险；
  同时 `getMethod(peek=true)` 的既有契约就是"查询不构建"，破坏它会波及
  `DeferredBuildKind*` 调度与所有前端。
- 补充：必须把 `drainLazyBuilders` 等"遍历 lazy 集合"的地方改为确定性顺序，
  或保证语义与顺序无关。这里两者都做——签名先固定（语义无关），遍历排序（可复现）。
- **不得**用"全量 eager 构建"换稳定：lazy 的性能收益（大全局字面量、大批量 flush 后低驻留）
  必须保留。

---

## 5. 待办（按顺序）

1. 已完成：扩展 RED（指针/值/多返回/命名多返回工厂 × 正反 body 顺序；跨包跨文件提前 import 两种文件顺序）并保存失败输出。
2. 已落到工作区、未提交：声明绑定阶段一次性建签名（不提前构建 body）；Tuple 结果槽用位置键；lazy 候选按源位置排序。
3. 当前阻塞：重跑被 SQLite 锁打败的 `Test_InterfaceMethodArgumentTypeCheckRegression`，区分环境竞争与产品回归；然后跑 go2ssa 全包、既有新回归、`-race`（`ssa`/`ssautil`/`ssaapi`/`go2ssa/test`）。
4. 全绿后才提交并按 §2 格式补本节；用新二进制在同一快照复跑 ARTEX：≥2 次默认 + 1 次单并发，
   全部独立 run/DB/`YAKIT_HOME`/`TMPDIR` + 八类 profile。
5. 复跑后核对：24 处 kind 变化、15 处 opcode 变化是否消失；结构计数、调用边、
   offset 源区间是否收敛；耗时与内存是否仍在预算内。
6. 通过后才进入多语言小项目门禁盘点与实现；未通过则继续定位剩余差异。

---

## 6. 问题清单

| ID | 发现时间 | 现象 | 根因 | 状态 | 关联 commit |
|---|---|---|---|---|---|
| ISSUE-001 | 2026-10-20 21:xx | 接收者方法被当作包级导出发布 | 方法分类延迟到 body 构建 | 已修 | `958fd9e8b4` |
| ISSUE-002 | 2026-10-20 23:35 | import 成员随运行少 60 个 `DB$*`、整文件缺键 | prehandler 期读到部分导出快照 | 已修 | `958fd9e8b4` |
| ISSUE-003 | 2026-10-21 00:0x | 正/反向成员关系重复、键身份不一致 | 两向各自发射 const 键 | 已修 | `958fd9e8b4` |
| ISSUE-004 | 2026-10-21 03:5x | `type Status string` 方法无法解析 | `bindGoMethod` 只认 `ObjectType` | 已修 | `c0e7b83589` |
| ISSUE-005 | 2026-10-20 16:02 | ARTEX `ir_offsets −3912`，v3 规范化后仍有 145839 matched | 身份漂移（多因）；非单一提交可归因 | 待验证 | `df79d3d142` 起 |
| ISSUE-006 | 2026-10-21 00:34 | 同二进制同输入默认并发 4 次 `ir_codes` 415784–430604 | 未知；已排除"仅 426884 样本" | 开放 | — |
| ISSUE-007 | 2026-10-21 10:5x | 同二进制 callee `Undefined` 有效性/opcode 随运行变化；与工作线程数无关 | 至少一条：声明期签名缺失 + lazy 顺序 | 开放（已定位子路径） | 待修 |
| ISSUE-008 | 2026-10-21 10:5x | `ir_offsets` 源区间集合每次不同（单并发 233/243） | 未知，可能与本条/写入时序相关 | 开放 | — |
| ISSUE-009 | 2026-10-21 10:50 | 独立 DB 仍出现 `database is locked`，编译数据被清理 | SQLite 写并发/连接复用；待定位具体 saver | 开放 | — |
| ISSUE-010 | 2026-10-21 10:51 | 产品 stage `failed`（review 失败、DB 清空）但 CLI 退出码 0 | CLI 未把 stage 失败映射为退出码 | 开放 | — |
| ISSUE-011 | 2026-10-21 10:4x | `Function` 4261/4261/4261/4259、`BasicBlock` 39691/39691/39691/39689 | 与 ISSUE-007 同族或独立 | 开放 | — |
| ISSUE-012 | 2026-10-21 10:5x | 默认并发下 11 个 SetBool 调用有 1 个操作数指纹不同（单并发两样本全匹配） | 待验证 | 待验证 | — |
| ISSUE-013 | 2026-10-21 03:40 | `958fd9e8b4` 构建不打印 `[observe]` 行，wrapper 误判"profile 初始化缺失"提前退出 | wrapper 探针假设了 phase2 overlay 的日志格式 | 开放（证据需人工核对） | — |
| ISSUE-014 | 2026-10-21 11:1x | 显式 body 顺序 `Use→New→Method` 时静态调用退化为 `UndefinedMemberInValid`，反序正常 | 签名在 lazy body 内建立；固化后不回填 | 工作区已绿，未提交；ARTEX 未复跑 | — |
| ISSUE-015 | 2026-10-21 12:1x | 多返回 `(T, error)` 工厂在声明期建签名后仍把接收者解成 `error` | `buildResult` 把每个返回槽都用键 0，后面的 `error` 覆盖第 0 返回 | 工作区已绿，未提交 | — |

### 6.1 已修问题的回归发现记录

- ISSUE-001/002/003：在 R-5 得到验证（`Type$method` 常量 290 种/10624 行 → 0；
  `DB$SetBool` 常量 0 与 R-1 一致）。
- ISSUE-004：由 ARTEX `c0e7b83589` 前的类型对比发现（3 个 `AliasType` 方法不是 object member），
  修复后单测 `TestGoNamedNonStructMethodResolves` 覆盖。
- ISSUE-007/008/014：在 R-7（A 稳定性实验）中被发现并量化。工作区修复后，精确顺序回归已绿，但尚未用新二进制复跑 ARTEX，不能宣布它们已消失。
- ISSUE-015：由签名提前后的多返回失败暴露；改为位置键后，`declaration-order-regressions` 已覆盖 `(T, error)` 与命名多返回。

---

## 7. 证据索引

| 证据 | 路径 |
|---|---|
| ARTEX 修复后首跑 | `phase3-20261021-0210/artex-958fd9e8b4-20261021-033904/` |
| 别名修复后运行 | `phase3-20261021-0210/artex-c0e7b83589-20261021-040658/` |
| A 稳定性实验（9 次） | `phase3-20261021-0210/A-stability-20261021-1025/` |
| A 判定报告 | `phase3-20261021-0210/A-stability-20261021-1025/report.md` |
| fail-closed 校验 | `phase3-20261021-0210/A-stability-20261021-1025/validation-01.json` |
| 确定性顺序 RED | `phase3-20261021-0210/A-stability-20261021-1025/serial-body-order-red/` |
| 语义审计器 | `phase3-20261021-0210/A-stability-20261021-1025/audit_semantics.py` |
| 对照比较器 | `phase3-20261021-0210/A-stability-20261021-1025/compare_semantics.py` |
| phase2 同构建重复 | `phase2-20261020-213104/nondeterminism-default-20261021-003417/` |
| phase3 早期门禁说明 | `phase3-20261021-0210/artex-semantic-gate.md` |


## 8. R-7 逐次运行明细（由原始 JSON/pprof 补录）

本节每次运行独立记录。源码版本与二进制身份不因日期或 program 名改变；风险数不是等价性证明。全部路径为实际绝对路径。

### R-7/smoke-full — 2026-10-21T10:35:24.322940+08:00

- 项目/输入：ARTEX，821 文件；manifest `f969ffeb972f95932b65909dabb39341facac06a5a07f4affbc7443eb81310a8`。
- 编译器：`c0e7b8358912718be98e8e1a620e9532914be052`；binary sha256 `c758f9118a5d50a3e79252e11a39e9cc1850e75cafcd51ea7b158b54d6044536`。
- run：`/mnt/data/scan-outputs/ssa-review-20261019/artex-ir-delta-20261020/phase3-20261021-0210/A-stability-20261021-1025/smoke-full`；DB：`/mnt/data/scan-outputs/ssa-review-20261019/artex-ir-delta-20261020/phase3-20261021-0210/A-stability-20261021-1025/smoke-full/ssadb.db`。
- 隔离：`YAKIT_HOME=/mnt/data/scan-outputs/ssa-review-20261019/artex-ir-delta-20261020/phase3-20261021-0210/A-stability-20261021-1025/smoke-full/yakit-home`；`TMPDIR=/mnt/data/scan-outputs/ssa-review-20261019/artex-ir-delta-20261020/phase3-20261021-0210/A-stability-20261021-1025/smoke-full/tmp`；GOTMPDIR 同 run；无并发/规则预算环境覆盖，workers 配置=None（1 仅诊断）。
- 命令：

```sh
/mnt/data/scan-outputs/ssa-review-20261019/artex-ir-delta-20261020/phase3-20261021-0210/A-stability-20261021-1025/binary/yak code-scan -t /mnt/data/scan-outputs/ssa-review-20261019/artex-ir-delta-20261020/phase2-20261020-213104/source-snapshots/artex -l golang --database /mnt/data/scan-outputs/ssa-review-20261019/artex-ir-delta-20261020/phase3-20261021-0210/A-stability-20261021-1025/smoke-full/ssadb.db --format irify --output /mnt/data/scan-outputs/ssa-review-20261019/artex-ir-delta-20261020/phase3-20261021-0210/A-stability-20261021-1025/smoke-full/report/artex --no-save-risk --debug /mnt/data/scan-outputs/ssa-review-20261019/artex-ir-delta-20261020/phase3-20261021-0210/A-stability-20261021-1025/smoke-full --log-level info
```

- 进程 exit **0**；产品阶段：完成；wall **106.106s**；RSS 峰值 **4097148KiB**；最低 MemAvailable **21387308KiB**；guard=None。
- 计数：`{"ir_codes": 290242, "ir_indices_v1": 203203, "ir_offsets": 247205, "ir_programs": 1, "ir_sources": 396, "ir_types": 174970, "ssa_risks": 0}`；Risk：`{"RiskNums": 590, "actual_risks_len": 590}`。
- opcode 全表（未省略派生类）：

```json
{"BasicBlock": 39691, "BinOp": 8624, "Call": 20729, "ConstInst": 97059, "ExternLib": 1765, "FreeValue": 14499, "Function": 4261, "If": 8256, "Jump": 19537, "Loop": 1423, "Make": 8846, "Next": 1106, "Parameter": 7177, "ParameterMember": 9926, "Phi": 7987, "Return": 8100, "SideEffect": 5349, "Switch": 282, "TypeValue": 387, "UnOp": 744, "Undefined": 24494}
```

- Profile 实际文件（同 run、均非空）：
  - `/mnt/data/scan-outputs/ssa-review-20261019/artex-ir-delta-20261020/phase3-20261021-0210/A-stability-20261021-1025/smoke-full/cpu-pprof`：`20261021-103527-initial.cpu.prof`, `20261021-103627-103627.cpu.prof`。
  - `/mnt/data/scan-outputs/ssa-review-20261019/artex-ir-delta-20261020/phase3-20261021-0210/A-stability-20261021-1025/smoke-full/memory-pprof`：`20261021-103527-initial.mem.prof`, `20261021-103627-early01.mem.prof`, `20261021-103703-final.mem.prof`。
  - `/mnt/data/scan-outputs/ssa-review-20261019/artex-ir-delta-20261020/phase3-20261021-0210/A-stability-20261021-1025/smoke-full/goroutine-pprof`：`20261021-103527-initial.goroutine.debug2.txt`, `20261021-103527-initial.goroutine.prof`, `20261021-103627-early01.goroutine.debug2.txt`, `20261021-103627-early01.goroutine.prof`, `20261021-103703-final.goroutine.debug2.txt`, `20261021-103703-final.goroutine.prof`。
  - `/mnt/data/scan-outputs/ssa-review-20261019/artex-ir-delta-20261020/phase3-20261021-0210/A-stability-20261021-1025/smoke-full/mutex-pprof`：`20261021-103527-initial.mutex.prof`, `20261021-103627-early01.mutex.prof`, `20261021-103703-final.mutex.prof`。
  - `/mnt/data/scan-outputs/ssa-review-20261019/artex-ir-delta-20261020/phase3-20261021-0210/A-stability-20261021-1025/smoke-full/block-pprof`：`20261021-103527-initial.block.prof`, `20261021-103627-early01.block.prof`, `20261021-103703-final.block.prof`。
  - `/mnt/data/scan-outputs/ssa-review-20261019/artex-ir-delta-20261020/phase3-20261021-0210/A-stability-20261021-1025/smoke-full/allocs-pprof`：`20261021-103527-initial.allocs.prof`, `20261021-103703-final.allocs.prof`。
  - `/mnt/data/scan-outputs/ssa-review-20261019/artex-ir-delta-20261020/phase3-20261021-0210/A-stability-20261021-1025/smoke-full/db-stats`：`20261021-103527-initial.db.json`, `20261021-103627-early01.db.json`, `20261021-103703-final.db.json`。
  - `/mnt/data/scan-outputs/ssa-review-20261019/artex-ir-delta-20261020/phase3-20261021-0210/A-stability-20261021-1025/smoke-full/runtime-stats`：`20261021-103527-initial.runtime.json`, `20261021-103627-early01.runtime.json`, `20261021-103703-final.runtime.json`。
- CPU 要点（initial 完整样本的符号化 flat top，不把总 CPU 等同 wall）：63.92s 14.82% 14.82%    139.33s 32.31%  runtime.scanObjectsSmall；50.84s 11.79% 26.61%     50.84s 11.79%  runtime.spanClass.sizeclass (inline)；43.71s 10.14% 36.74%    105.16s 24.38%  runtime.tryDeferToSpanScan。
- 内存要点（final retained heap）：Showing nodes accounting for 210.09MB, 58.23% of 360.78MB total；分配累计见 `allocs-top.txt`，不据此声称泄漏。
- goroutine debug=2：20261021-103527-initial.goroutine.debug2.txt=618, 20261021-103627-early01.goroutine.debug2.txt=669, 20261021-103703-final.goroutine.debug2.txt=647；mutex/block aggregate wait 不当作死锁证明。
- 对照判定/问题/下一步：只证明 profiling smoke 自然运行超过30s且各类profile可读，不晋升稳定性基线；下一步4次默认采样。

### R-7/default-01 — 2026-10-21T10:37:11.456356+08:00

- 项目/输入：ARTEX，821 文件；manifest `f969ffeb972f95932b65909dabb39341facac06a5a07f4affbc7443eb81310a8`。
- 编译器：`c0e7b8358912718be98e8e1a620e9532914be052`；binary sha256 `c758f9118a5d50a3e79252e11a39e9cc1850e75cafcd51ea7b158b54d6044536`。
- run：`/mnt/data/scan-outputs/ssa-review-20261019/artex-ir-delta-20261020/phase3-20261021-0210/A-stability-20261021-1025/default-01`；DB：`/mnt/data/scan-outputs/ssa-review-20261019/artex-ir-delta-20261020/phase3-20261021-0210/A-stability-20261021-1025/default-01/ssadb.db`。
- 隔离：`YAKIT_HOME=/mnt/data/scan-outputs/ssa-review-20261019/artex-ir-delta-20261020/phase3-20261021-0210/A-stability-20261021-1025/default-01/yakit-home`；`TMPDIR=/mnt/data/scan-outputs/ssa-review-20261019/artex-ir-delta-20261020/phase3-20261021-0210/A-stability-20261021-1025/default-01/tmp`；GOTMPDIR 同 run；无并发/规则预算环境覆盖，workers 配置=None（1 仅诊断）。
- 命令：

```sh
/mnt/data/scan-outputs/ssa-review-20261019/artex-ir-delta-20261020/phase3-20261021-0210/A-stability-20261021-1025/binary/yak code-scan -t /mnt/data/scan-outputs/ssa-review-20261019/artex-ir-delta-20261020/phase2-20261020-213104/source-snapshots/artex -l golang --database /mnt/data/scan-outputs/ssa-review-20261019/artex-ir-delta-20261020/phase3-20261021-0210/A-stability-20261021-1025/default-01/ssadb.db --format irify --output /mnt/data/scan-outputs/ssa-review-20261019/artex-ir-delta-20261020/phase3-20261021-0210/A-stability-20261021-1025/default-01/report/artex --no-save-risk --debug /mnt/data/scan-outputs/ssa-review-20261019/artex-ir-delta-20261020/phase3-20261021-0210/A-stability-20261021-1025/default-01 --log-level info
```

- 进程 exit **0**；产品阶段：完成；wall **110.099s**；RSS 峰值 **4401568KiB**；最低 MemAvailable **21084404KiB**；guard=None。
- 计数：`{"ir_codes": 290837, "ir_indices_v1": 203784, "ir_offsets": 246692, "ir_programs": 1, "ir_sources": 396, "ir_types": 175177, "ssa_risks": 0}`；Risk：`{"RiskNums": 590, "actual_risks_len": 590}`。
- opcode 全表（未省略派生类）：

```json
{"BasicBlock": 39691, "BinOp": 8624, "Call": 20729, "ConstInst": 97069, "ExternLib": 1765, "FreeValue": 14909, "Function": 4261, "If": 8256, "Jump": 19537, "Loop": 1423, "Make": 8818, "Next": 1106, "Parameter": 7177, "ParameterMember": 9987, "Phi": 8049, "Return": 8100, "SideEffect": 5421, "Switch": 282, "TypeValue": 387, "UnOp": 744, "Undefined": 24502}
```

- Profile 实际文件（同 run、均非空）：
  - `/mnt/data/scan-outputs/ssa-review-20261019/artex-ir-delta-20261020/phase3-20261021-0210/A-stability-20261021-1025/default-01/cpu-pprof`：`20261021-103714-initial.cpu.prof`, `20261021-103814-103814.cpu.prof`。
  - `/mnt/data/scan-outputs/ssa-review-20261019/artex-ir-delta-20261020/phase3-20261021-0210/A-stability-20261021-1025/default-01/memory-pprof`：`20261021-103714-initial.mem.prof`, `20261021-103814-early01.mem.prof`, `20261021-103853-final.mem.prof`。
  - `/mnt/data/scan-outputs/ssa-review-20261019/artex-ir-delta-20261020/phase3-20261021-0210/A-stability-20261021-1025/default-01/goroutine-pprof`：`20261021-103714-initial.goroutine.debug2.txt`, `20261021-103714-initial.goroutine.prof`, `20261021-103814-early01.goroutine.debug2.txt`, `20261021-103814-early01.goroutine.prof`, `20261021-103853-final.goroutine.debug2.txt`, `20261021-103853-final.goroutine.prof`。
  - `/mnt/data/scan-outputs/ssa-review-20261019/artex-ir-delta-20261020/phase3-20261021-0210/A-stability-20261021-1025/default-01/mutex-pprof`：`20261021-103714-initial.mutex.prof`, `20261021-103814-early01.mutex.prof`, `20261021-103853-final.mutex.prof`。
  - `/mnt/data/scan-outputs/ssa-review-20261019/artex-ir-delta-20261020/phase3-20261021-0210/A-stability-20261021-1025/default-01/block-pprof`：`20261021-103714-initial.block.prof`, `20261021-103814-early01.block.prof`, `20261021-103853-final.block.prof`。
  - `/mnt/data/scan-outputs/ssa-review-20261019/artex-ir-delta-20261020/phase3-20261021-0210/A-stability-20261021-1025/default-01/allocs-pprof`：`20261021-103714-initial.allocs.prof`, `20261021-103853-final.allocs.prof`。
  - `/mnt/data/scan-outputs/ssa-review-20261019/artex-ir-delta-20261020/phase3-20261021-0210/A-stability-20261021-1025/default-01/db-stats`：`20261021-103714-initial.db.json`, `20261021-103814-early01.db.json`, `20261021-103853-final.db.json`。
  - `/mnt/data/scan-outputs/ssa-review-20261019/artex-ir-delta-20261020/phase3-20261021-0210/A-stability-20261021-1025/default-01/runtime-stats`：`20261021-103714-initial.runtime.json`, `20261021-103814-early01.runtime.json`, `20261021-103853-final.runtime.json`。
- CPU 要点（initial 完整样本的符号化 flat top，不把总 CPU 等同 wall）：59.89s 14.89% 14.89%    131.95s 32.79%  runtime.scanObjectsSmall；47.95s 11.92% 26.80%     47.95s 11.92%  runtime.spanClass.sizeclass (inline)；41.91s 10.42% 37.22%    100.74s 25.04%  runtime.tryDeferToSpanScan。
- 内存要点（final retained heap）：Showing nodes accounting for 172.16MB, 54.74% of 314.48MB total；分配累计见 `allocs-top.txt`，不据此声称泄漏。
- goroutine debug=2：20261021-103714-initial.goroutine.debug2.txt=618, 20261021-103814-early01.goroutine.debug2.txt=669, 20261021-103853-final.goroutine.debug2.txt=647；mutex/block aggregate wait 不当作死锁证明。
- 对照判定/问题/下一步：FAIL（整轮对照）：调用点数量/位置相同，但callee、结构/派生计数与offset区间变化，不是已证明无害浮动。问题 ISSUE-006/007/008/011/012；下一步声明签名顺序修复与复测。

### R-7/default-02 — 2026-10-21T10:39:02.585932+08:00

- 项目/输入：ARTEX，821 文件；manifest `f969ffeb972f95932b65909dabb39341facac06a5a07f4affbc7443eb81310a8`。
- 编译器：`c0e7b8358912718be98e8e1a620e9532914be052`；binary sha256 `c758f9118a5d50a3e79252e11a39e9cc1850e75cafcd51ea7b158b54d6044536`。
- run：`/mnt/data/scan-outputs/ssa-review-20261019/artex-ir-delta-20261020/phase3-20261021-0210/A-stability-20261021-1025/default-02`；DB：`/mnt/data/scan-outputs/ssa-review-20261019/artex-ir-delta-20261020/phase3-20261021-0210/A-stability-20261021-1025/default-02/ssadb.db`。
- 隔离：`YAKIT_HOME=/mnt/data/scan-outputs/ssa-review-20261019/artex-ir-delta-20261020/phase3-20261021-0210/A-stability-20261021-1025/default-02/yakit-home`；`TMPDIR=/mnt/data/scan-outputs/ssa-review-20261019/artex-ir-delta-20261020/phase3-20261021-0210/A-stability-20261021-1025/default-02/tmp`；GOTMPDIR 同 run；无并发/规则预算环境覆盖，workers 配置=None（1 仅诊断）。
- 命令：

```sh
/mnt/data/scan-outputs/ssa-review-20261019/artex-ir-delta-20261020/phase3-20261021-0210/A-stability-20261021-1025/binary/yak code-scan -t /mnt/data/scan-outputs/ssa-review-20261019/artex-ir-delta-20261020/phase2-20261020-213104/source-snapshots/artex -l golang --database /mnt/data/scan-outputs/ssa-review-20261019/artex-ir-delta-20261020/phase3-20261021-0210/A-stability-20261021-1025/default-02/ssadb.db --format irify --output /mnt/data/scan-outputs/ssa-review-20261019/artex-ir-delta-20261020/phase3-20261021-0210/A-stability-20261021-1025/default-02/report/artex --no-save-risk --debug /mnt/data/scan-outputs/ssa-review-20261019/artex-ir-delta-20261020/phase3-20261021-0210/A-stability-20261021-1025/default-02 --log-level info
```

- 进程 exit **0**；产品阶段：完成；wall **114.083s**；RSS 峰值 **3879112KiB**；最低 MemAvailable **21651116KiB**；guard=None。
- 计数：`{"ir_codes": 291736, "ir_indices_v1": 204403, "ir_offsets": 251787, "ir_programs": 1, "ir_sources": 396, "ir_types": 174972, "ssa_risks": 0}`；Risk：`{"RiskNums": 590, "actual_risks_len": 590}`。
- opcode 全表（未省略派生类）：

```json
{"BasicBlock": 39691, "BinOp": 8624, "Call": 20729, "ConstInst": 96989, "ExternLib": 1765, "FreeValue": 15323, "Function": 4261, "If": 8256, "Jump": 19537, "Loop": 1423, "Make": 8851, "Next": 1106, "Parameter": 7177, "ParameterMember": 9920, "Phi": 8237, "Return": 8100, "SideEffect": 5814, "Switch": 282, "TypeValue": 387, "UnOp": 744, "Undefined": 24520}
```

- Profile 实际文件（同 run、均非空）：
  - `/mnt/data/scan-outputs/ssa-review-20261019/artex-ir-delta-20261020/phase3-20261021-0210/A-stability-20261021-1025/default-02/cpu-pprof`：`20261021-103905-initial.cpu.prof`, `20261021-104005-104005.cpu.prof`。
  - `/mnt/data/scan-outputs/ssa-review-20261019/artex-ir-delta-20261020/phase3-20261021-0210/A-stability-20261021-1025/default-02/memory-pprof`：`20261021-103905-initial.mem.prof`, `20261021-104005-early01.mem.prof`, `20261021-104048-final.mem.prof`。
  - `/mnt/data/scan-outputs/ssa-review-20261019/artex-ir-delta-20261020/phase3-20261021-0210/A-stability-20261021-1025/default-02/goroutine-pprof`：`20261021-103905-initial.goroutine.debug2.txt`, `20261021-103905-initial.goroutine.prof`, `20261021-104005-early01.goroutine.debug2.txt`, `20261021-104005-early01.goroutine.prof`, `20261021-104048-final.goroutine.debug2.txt`, `20261021-104048-final.goroutine.prof`。
  - `/mnt/data/scan-outputs/ssa-review-20261019/artex-ir-delta-20261020/phase3-20261021-0210/A-stability-20261021-1025/default-02/mutex-pprof`：`20261021-103905-initial.mutex.prof`, `20261021-104005-early01.mutex.prof`, `20261021-104048-final.mutex.prof`。
  - `/mnt/data/scan-outputs/ssa-review-20261019/artex-ir-delta-20261020/phase3-20261021-0210/A-stability-20261021-1025/default-02/block-pprof`：`20261021-103905-initial.block.prof`, `20261021-104005-early01.block.prof`, `20261021-104048-final.block.prof`。
  - `/mnt/data/scan-outputs/ssa-review-20261019/artex-ir-delta-20261020/phase3-20261021-0210/A-stability-20261021-1025/default-02/allocs-pprof`：`20261021-103905-initial.allocs.prof`, `20261021-104048-final.allocs.prof`。
  - `/mnt/data/scan-outputs/ssa-review-20261019/artex-ir-delta-20261020/phase3-20261021-0210/A-stability-20261021-1025/default-02/db-stats`：`20261021-103905-initial.db.json`, `20261021-104005-early01.db.json`, `20261021-104048-final.db.json`。
  - `/mnt/data/scan-outputs/ssa-review-20261019/artex-ir-delta-20261020/phase3-20261021-0210/A-stability-20261021-1025/default-02/runtime-stats`：`20261021-103905-initial.runtime.json`, `20261021-104005-early01.runtime.json`, `20261021-104048-final.runtime.json`。
- CPU 要点（initial 完整样本的符号化 flat top，不把总 CPU 等同 wall）：57.68s 14.78% 14.78%    126.05s 32.31%  runtime.scanObjectsSmall；46.08s 11.81% 26.59%     46.08s 11.81%  runtime.spanClass.sizeclass (inline)；40.65s 10.42% 37.01%     97.30s 24.94%  runtime.tryDeferToSpanScan。
- 内存要点（final retained heap）：Showing nodes accounting for 169.79MB, 54.84% of 309.63MB total；分配累计见 `allocs-top.txt`，不据此声称泄漏。
- goroutine debug=2：20261021-103905-initial.goroutine.debug2.txt=618, 20261021-104005-early01.goroutine.debug2.txt=663, 20261021-104048-final.goroutine.debug2.txt=647；mutex/block aggregate wait 不当作死锁证明。
- 对照判定/问题/下一步：FAIL（整轮对照）：调用点数量/位置相同，但callee、结构/派生计数与offset区间变化，不是已证明无害浮动。问题 ISSUE-006/007/008/011/012；下一步声明签名顺序修复与复测。

### R-7/default-03 — 2026-10-21T10:40:57.724620+08:00

- 项目/输入：ARTEX，821 文件；manifest `f969ffeb972f95932b65909dabb39341facac06a5a07f4affbc7443eb81310a8`。
- 编译器：`c0e7b8358912718be98e8e1a620e9532914be052`；binary sha256 `c758f9118a5d50a3e79252e11a39e9cc1850e75cafcd51ea7b158b54d6044536`。
- run：`/mnt/data/scan-outputs/ssa-review-20261019/artex-ir-delta-20261020/phase3-20261021-0210/A-stability-20261021-1025/default-03`；DB：`/mnt/data/scan-outputs/ssa-review-20261019/artex-ir-delta-20261020/phase3-20261021-0210/A-stability-20261021-1025/default-03/ssadb.db`。
- 隔离：`YAKIT_HOME=/mnt/data/scan-outputs/ssa-review-20261019/artex-ir-delta-20261020/phase3-20261021-0210/A-stability-20261021-1025/default-03/yakit-home`；`TMPDIR=/mnt/data/scan-outputs/ssa-review-20261019/artex-ir-delta-20261020/phase3-20261021-0210/A-stability-20261021-1025/default-03/tmp`；GOTMPDIR 同 run；无并发/规则预算环境覆盖，workers 配置=None（1 仅诊断）。
- 命令：

```sh
/mnt/data/scan-outputs/ssa-review-20261019/artex-ir-delta-20261020/phase3-20261021-0210/A-stability-20261021-1025/binary/yak code-scan -t /mnt/data/scan-outputs/ssa-review-20261019/artex-ir-delta-20261020/phase2-20261020-213104/source-snapshots/artex -l golang --database /mnt/data/scan-outputs/ssa-review-20261019/artex-ir-delta-20261020/phase3-20261021-0210/A-stability-20261021-1025/default-03/ssadb.db --format irify --output /mnt/data/scan-outputs/ssa-review-20261019/artex-ir-delta-20261020/phase3-20261021-0210/A-stability-20261021-1025/default-03/report/artex --no-save-risk --debug /mnt/data/scan-outputs/ssa-review-20261019/artex-ir-delta-20261020/phase3-20261021-0210/A-stability-20261021-1025/default-03 --log-level info
```

- 进程 exit **0**；产品阶段：完成；wall **113.086s**；RSS 峰值 **3855104KiB**；最低 MemAvailable **21253184KiB**；guard=None。
- 计数：`{"ir_codes": 289323, "ir_indices_v1": 202593, "ir_offsets": 245854, "ir_programs": 1, "ir_sources": 396, "ir_types": 175024, "ssa_risks": 0}`；Risk：`{"RiskNums": 590, "actual_risks_len": 590}`。
- opcode 全表（未省略派生类）：

```json
{"BasicBlock": 39691, "BinOp": 8624, "Call": 20729, "ConstInst": 97069, "ExternLib": 1765, "FreeValue": 14022, "Function": 4261, "If": 8256, "Jump": 19537, "Loop": 1423, "Make": 8821, "Next": 1106, "Parameter": 7177, "ParameterMember": 9879, "Phi": 7851, "Return": 8100, "SideEffect": 5125, "Switch": 282, "TypeValue": 387, "UnOp": 744, "Undefined": 24474}
```

- Profile 实际文件（同 run、均非空）：
  - `/mnt/data/scan-outputs/ssa-review-20261019/artex-ir-delta-20261020/phase3-20261021-0210/A-stability-20261021-1025/default-03/cpu-pprof`：`20261021-104101-initial.cpu.prof`, `20261021-104201-104201.cpu.prof`。
  - `/mnt/data/scan-outputs/ssa-review-20261019/artex-ir-delta-20261020/phase3-20261021-0210/A-stability-20261021-1025/default-03/memory-pprof`：`20261021-104101-initial.mem.prof`, `20261021-104201-early01.mem.prof`, `20261021-104242-final.mem.prof`。
  - `/mnt/data/scan-outputs/ssa-review-20261019/artex-ir-delta-20261020/phase3-20261021-0210/A-stability-20261021-1025/default-03/goroutine-pprof`：`20261021-104101-initial.goroutine.debug2.txt`, `20261021-104101-initial.goroutine.prof`, `20261021-104201-early01.goroutine.debug2.txt`, `20261021-104201-early01.goroutine.prof`, `20261021-104242-final.goroutine.debug2.txt`, `20261021-104242-final.goroutine.prof`。
  - `/mnt/data/scan-outputs/ssa-review-20261019/artex-ir-delta-20261020/phase3-20261021-0210/A-stability-20261021-1025/default-03/mutex-pprof`：`20261021-104101-initial.mutex.prof`, `20261021-104201-early01.mutex.prof`, `20261021-104242-final.mutex.prof`。
  - `/mnt/data/scan-outputs/ssa-review-20261019/artex-ir-delta-20261020/phase3-20261021-0210/A-stability-20261021-1025/default-03/block-pprof`：`20261021-104101-initial.block.prof`, `20261021-104201-early01.block.prof`, `20261021-104242-final.block.prof`。
  - `/mnt/data/scan-outputs/ssa-review-20261019/artex-ir-delta-20261020/phase3-20261021-0210/A-stability-20261021-1025/default-03/allocs-pprof`：`20261021-104101-initial.allocs.prof`, `20261021-104242-final.allocs.prof`。
  - `/mnt/data/scan-outputs/ssa-review-20261019/artex-ir-delta-20261020/phase3-20261021-0210/A-stability-20261021-1025/default-03/db-stats`：`20261021-104101-initial.db.json`, `20261021-104201-early01.db.json`, `20261021-104242-final.db.json`。
  - `/mnt/data/scan-outputs/ssa-review-20261019/artex-ir-delta-20261020/phase3-20261021-0210/A-stability-20261021-1025/default-03/runtime-stats`：`20261021-104101-initial.runtime.json`, `20261021-104201-early01.runtime.json`, `20261021-104242-final.runtime.json`。
- CPU 要点（initial 完整样本的符号化 flat top，不把总 CPU 等同 wall）：60.93s 15.25% 15.25%    134.39s 33.64%  runtime.scanObjectsSmall；49.35s 12.35% 27.60%     49.35s 12.35%  runtime.spanClass.sizeclass (inline)；42.27s 10.58% 38.19%    101.93s 25.51%  runtime.tryDeferToSpanScan。
- 内存要点（final retained heap）：Showing nodes accounting for 202.60MB, 56.96% of 355.70MB total；分配累计见 `allocs-top.txt`，不据此声称泄漏。
- goroutine debug=2：20261021-104101-initial.goroutine.debug2.txt=618, 20261021-104201-early01.goroutine.debug2.txt=663, 20261021-104242-final.goroutine.debug2.txt=647；mutex/block aggregate wait 不当作死锁证明。
- 对照判定/问题/下一步：FAIL（整轮对照）：调用点数量/位置相同，但callee、结构/派生计数与offset区间变化，不是已证明无害浮动。问题 ISSUE-006/007/008/011/012；下一步声明签名顺序修复与复测。

### R-7/default-04 — 2026-10-21T10:42:51.917406+08:00

- 项目/输入：ARTEX，821 文件；manifest `f969ffeb972f95932b65909dabb39341facac06a5a07f4affbc7443eb81310a8`。
- 编译器：`c0e7b8358912718be98e8e1a620e9532914be052`；binary sha256 `c758f9118a5d50a3e79252e11a39e9cc1850e75cafcd51ea7b158b54d6044536`。
- run：`/mnt/data/scan-outputs/ssa-review-20261019/artex-ir-delta-20261020/phase3-20261021-0210/A-stability-20261021-1025/default-04`；DB：`/mnt/data/scan-outputs/ssa-review-20261019/artex-ir-delta-20261020/phase3-20261021-0210/A-stability-20261021-1025/default-04/ssadb.db`。
- 隔离：`YAKIT_HOME=/mnt/data/scan-outputs/ssa-review-20261019/artex-ir-delta-20261020/phase3-20261021-0210/A-stability-20261021-1025/default-04/yakit-home`；`TMPDIR=/mnt/data/scan-outputs/ssa-review-20261019/artex-ir-delta-20261020/phase3-20261021-0210/A-stability-20261021-1025/default-04/tmp`；GOTMPDIR 同 run；无并发/规则预算环境覆盖，workers 配置=None（1 仅诊断）。
- 命令：

```sh
/mnt/data/scan-outputs/ssa-review-20261019/artex-ir-delta-20261020/phase3-20261021-0210/A-stability-20261021-1025/binary/yak code-scan -t /mnt/data/scan-outputs/ssa-review-20261019/artex-ir-delta-20261020/phase2-20261020-213104/source-snapshots/artex -l golang --database /mnt/data/scan-outputs/ssa-review-20261019/artex-ir-delta-20261020/phase3-20261021-0210/A-stability-20261021-1025/default-04/ssadb.db --format irify --output /mnt/data/scan-outputs/ssa-review-20261019/artex-ir-delta-20261020/phase3-20261021-0210/A-stability-20261021-1025/default-04/report/artex --no-save-risk --debug /mnt/data/scan-outputs/ssa-review-20261019/artex-ir-delta-20261020/phase3-20261021-0210/A-stability-20261021-1025/default-04 --log-level info
```

- 进程 exit **0**；产品阶段：完成；wall **110.082s**；RSS 峰值 **4314780KiB**；最低 MemAvailable **20610912KiB**；guard=None。
- 计数：`{"ir_codes": 290739, "ir_indices_v1": 203968, "ir_offsets": 250381, "ir_programs": 1, "ir_sources": 396, "ir_types": 175276, "ssa_risks": 0}`；Risk：`{"RiskNums": 590, "actual_risks_len": 590}`。
- opcode 全表（未省略派生类）：

```json
{"BasicBlock": 39689, "BinOp": 8624, "Call": 20729, "ConstInst": 97064, "ExternLib": 1765, "FreeValue": 14636, "Function": 4259, "If": 8256, "Jump": 19537, "Loop": 1423, "Make": 8822, "Next": 1106, "Parameter": 7177, "ParameterMember": 10010, "Phi": 8095, "Return": 8100, "SideEffect": 5551, "Switch": 282, "TypeValue": 387, "UnOp": 744, "Undefined": 24483}
```

- Profile 实际文件（同 run、均非空）：
  - `/mnt/data/scan-outputs/ssa-review-20261019/artex-ir-delta-20261020/phase3-20261021-0210/A-stability-20261021-1025/default-04/cpu-pprof`：`20261021-104254-initial.cpu.prof`, `20261021-104354-104354.cpu.prof`。
  - `/mnt/data/scan-outputs/ssa-review-20261019/artex-ir-delta-20261020/phase3-20261021-0210/A-stability-20261021-1025/default-04/memory-pprof`：`20261021-104254-initial.mem.prof`, `20261021-104355-early01.mem.prof`, `20261021-104434-final.mem.prof`。
  - `/mnt/data/scan-outputs/ssa-review-20261019/artex-ir-delta-20261020/phase3-20261021-0210/A-stability-20261021-1025/default-04/goroutine-pprof`：`20261021-104254-initial.goroutine.debug2.txt`, `20261021-104254-initial.goroutine.prof`, `20261021-104355-early01.goroutine.debug2.txt`, `20261021-104355-early01.goroutine.prof`, `20261021-104434-final.goroutine.debug2.txt`, `20261021-104434-final.goroutine.prof`。
  - `/mnt/data/scan-outputs/ssa-review-20261019/artex-ir-delta-20261020/phase3-20261021-0210/A-stability-20261021-1025/default-04/mutex-pprof`：`20261021-104254-initial.mutex.prof`, `20261021-104355-early01.mutex.prof`, `20261021-104434-final.mutex.prof`。
  - `/mnt/data/scan-outputs/ssa-review-20261019/artex-ir-delta-20261020/phase3-20261021-0210/A-stability-20261021-1025/default-04/block-pprof`：`20261021-104254-initial.block.prof`, `20261021-104355-early01.block.prof`, `20261021-104434-final.block.prof`。
  - `/mnt/data/scan-outputs/ssa-review-20261019/artex-ir-delta-20261020/phase3-20261021-0210/A-stability-20261021-1025/default-04/allocs-pprof`：`20261021-104254-initial.allocs.prof`, `20261021-104434-final.allocs.prof`。
  - `/mnt/data/scan-outputs/ssa-review-20261019/artex-ir-delta-20261020/phase3-20261021-0210/A-stability-20261021-1025/default-04/db-stats`：`20261021-104254-initial.db.json`, `20261021-104355-early01.db.json`, `20261021-104434-final.db.json`。
  - `/mnt/data/scan-outputs/ssa-review-20261019/artex-ir-delta-20261020/phase3-20261021-0210/A-stability-20261021-1025/default-04/runtime-stats`：`20261021-104254-initial.runtime.json`, `20261021-104355-early01.runtime.json`, `20261021-104434-final.runtime.json`。
- CPU 要点（initial 完整样本的符号化 flat top，不把总 CPU 等同 wall）：63.08s 15.12% 15.12%    136.58s 32.73%  runtime.scanObjectsSmall；49.81s 11.94% 27.06%     49.81s 11.94%  runtime.spanClass.sizeclass (inline)；41.28s  9.89% 36.95%    102.11s 24.47%  runtime.tryDeferToSpanScan。
- 内存要点（final retained heap）：Showing nodes accounting for 187.50MB, 52.16% of 359.48MB total；分配累计见 `allocs-top.txt`，不据此声称泄漏。
- goroutine debug=2：20261021-104254-initial.goroutine.debug2.txt=618, 20261021-104355-early01.goroutine.debug2.txt=668, 20261021-104434-final.goroutine.debug2.txt=647；mutex/block aggregate wait 不当作死锁证明。
- 对照判定/问题/下一步：FAIL（整轮对照）：调用点数量/位置相同，但callee、结构/派生计数与offset区间变化，不是已证明无害浮动。问题 ISSUE-006/007/008/011/012；下一步声明签名顺序修复与复测。

### R-7/config-auto — 2026-10-21T10:44:43.090097+08:00

- 项目/输入：ARTEX，821 文件；manifest `f969ffeb972f95932b65909dabb39341facac06a5a07f4affbc7443eb81310a8`。
- 编译器：`c0e7b8358912718be98e8e1a620e9532914be052`；binary sha256 `c758f9118a5d50a3e79252e11a39e9cc1850e75cafcd51ea7b158b54d6044536`。
- run：`/mnt/data/scan-outputs/ssa-review-20261019/artex-ir-delta-20261020/phase3-20261021-0210/A-stability-20261021-1025/config-auto`；DB：`/mnt/data/scan-outputs/ssa-review-20261019/artex-ir-delta-20261020/phase3-20261021-0210/A-stability-20261021-1025/config-auto/ssadb.db`。
- 隔离：`YAKIT_HOME=/mnt/data/scan-outputs/ssa-review-20261019/artex-ir-delta-20261020/phase3-20261021-0210/A-stability-20261021-1025/config-auto/yakit-home`；`TMPDIR=/mnt/data/scan-outputs/ssa-review-20261019/artex-ir-delta-20261020/phase3-20261021-0210/A-stability-20261021-1025/config-auto/tmp`；GOTMPDIR 同 run；无并发/规则预算环境覆盖，workers 配置=0（1 仅诊断）。
- 命令：

```sh
/mnt/data/scan-outputs/ssa-review-20261019/artex-ir-delta-20261020/phase3-20261021-0210/A-stability-20261021-1025/binary/yak code-scan -t /mnt/data/scan-outputs/ssa-review-20261019/artex-ir-delta-20261020/phase2-20261020-213104/source-snapshots/artex -l golang --database /mnt/data/scan-outputs/ssa-review-20261019/artex-ir-delta-20261020/phase3-20261021-0210/A-stability-20261021-1025/config-auto/ssadb.db --format irify --output /mnt/data/scan-outputs/ssa-review-20261019/artex-ir-delta-20261020/phase3-20261021-0210/A-stability-20261021-1025/config-auto/report/artex --no-save-risk --debug /mnt/data/scan-outputs/ssa-review-20261019/artex-ir-delta-20261020/phase3-20261021-0210/A-stability-20261021-1025/config-auto --log-level info --config /mnt/data/scan-outputs/ssa-review-20261019/artex-ir-delta-20261020/phase3-20261021-0210/A-stability-20261021-1025/config-auto/diagnostic-config.json
```

- 进程 exit **0**；产品阶段：完成；wall **109.086s**；RSS 峰值 **4202192KiB**；最低 MemAvailable **21137868KiB**；guard=None。
- 计数：`{"ir_codes": 290379, "ir_indices_v1": 203498, "ir_offsets": 247673, "ir_programs": 1, "ir_sources": 396, "ir_types": 175242, "ssa_risks": 0}`；Risk：`{"RiskNums": 590, "actual_risks_len": 590}`。
- opcode 全表（未省略派生类）：

```json
{"BasicBlock": 39691, "BinOp": 8624, "Call": 20729, "ConstInst": 97113, "ExternLib": 1765, "FreeValue": 14493, "Function": 4261, "If": 8256, "Jump": 19537, "Loop": 1423, "Make": 8825, "Next": 1106, "Parameter": 7177, "ParameterMember": 10038, "Phi": 7997, "Return": 8100, "SideEffect": 5347, "Switch": 282, "TypeValue": 387, "UnOp": 744, "Undefined": 24484}
```

- Profile 实际文件（同 run、均非空）：
  - `/mnt/data/scan-outputs/ssa-review-20261019/artex-ir-delta-20261020/phase3-20261021-0210/A-stability-20261021-1025/config-auto/cpu-pprof`：`20261021-104445-initial.cpu.prof`, `20261021-104545-104545.cpu.prof`。
  - `/mnt/data/scan-outputs/ssa-review-20261019/artex-ir-delta-20261020/phase3-20261021-0210/A-stability-20261021-1025/config-auto/memory-pprof`：`20261021-104445-initial.mem.prof`, `20261021-104546-early01.mem.prof`, `20261021-104624-final.mem.prof`。
  - `/mnt/data/scan-outputs/ssa-review-20261019/artex-ir-delta-20261020/phase3-20261021-0210/A-stability-20261021-1025/config-auto/goroutine-pprof`：`20261021-104445-initial.goroutine.debug2.txt`, `20261021-104445-initial.goroutine.prof`, `20261021-104546-early01.goroutine.debug2.txt`, `20261021-104546-early01.goroutine.prof`, `20261021-104624-final.goroutine.debug2.txt`, `20261021-104624-final.goroutine.prof`。
  - `/mnt/data/scan-outputs/ssa-review-20261019/artex-ir-delta-20261020/phase3-20261021-0210/A-stability-20261021-1025/config-auto/mutex-pprof`：`20261021-104445-initial.mutex.prof`, `20261021-104546-early01.mutex.prof`, `20261021-104624-final.mutex.prof`。
  - `/mnt/data/scan-outputs/ssa-review-20261019/artex-ir-delta-20261020/phase3-20261021-0210/A-stability-20261021-1025/config-auto/block-pprof`：`20261021-104445-initial.block.prof`, `20261021-104546-early01.block.prof`, `20261021-104624-final.block.prof`。
  - `/mnt/data/scan-outputs/ssa-review-20261019/artex-ir-delta-20261020/phase3-20261021-0210/A-stability-20261021-1025/config-auto/allocs-pprof`：`20261021-104445-initial.allocs.prof`, `20261021-104624-final.allocs.prof`。
  - `/mnt/data/scan-outputs/ssa-review-20261019/artex-ir-delta-20261020/phase3-20261021-0210/A-stability-20261021-1025/config-auto/db-stats`：`20261021-104445-initial.db.json`, `20261021-104546-early01.db.json`, `20261021-104624-final.db.json`。
  - `/mnt/data/scan-outputs/ssa-review-20261019/artex-ir-delta-20261020/phase3-20261021-0210/A-stability-20261021-1025/config-auto/runtime-stats`：`20261021-104445-initial.runtime.json`, `20261021-104546-early01.runtime.json`, `20261021-104624-final.runtime.json`。
- CPU 要点（initial 完整样本的符号化 flat top，不把总 CPU 等同 wall）：63.25s 14.77% 14.77%    139.44s 32.57%  runtime.scanObjectsSmall；49.69s 11.61% 26.38%     49.69s 11.61%  runtime.spanClass.sizeclass (inline)；43.24s 10.10% 36.48%    104.29s 24.36%  runtime.tryDeferToSpanScan。
- 内存要点（final retained heap）：Showing nodes accounting for 331.99MB, 46.60% of 712.35MB total；分配累计见 `allocs-top.txt`，不据此声称泄漏。
- goroutine debug=2：20261021-104445-initial.goroutine.debug2.txt=618, 20261021-104546-early01.goroutine.debug2.txt=666, 20261021-104624-final.goroutine.debug2.txt=647；mutex/block aggregate wait 不当作死锁证明。
- 对照判定/问题/下一步：仅用于控制 --config 入口变量，worker=0恢复机器默认；不是稳定性通过样本。下一步与显式worker=1比较。

### R-7/single-01 — 2026-10-21T10:46:33.232504+08:00

- 项目/输入：ARTEX，821 文件；manifest `f969ffeb972f95932b65909dabb39341facac06a5a07f4affbc7443eb81310a8`。
- 编译器：`c0e7b8358912718be98e8e1a620e9532914be052`；binary sha256 `c758f9118a5d50a3e79252e11a39e9cc1850e75cafcd51ea7b158b54d6044536`。
- run：`/mnt/data/scan-outputs/ssa-review-20261019/artex-ir-delta-20261020/phase3-20261021-0210/A-stability-20261021-1025/single-01`；DB：`/mnt/data/scan-outputs/ssa-review-20261019/artex-ir-delta-20261020/phase3-20261021-0210/A-stability-20261021-1025/single-01/ssadb.db`。
- 隔离：`YAKIT_HOME=/mnt/data/scan-outputs/ssa-review-20261019/artex-ir-delta-20261020/phase3-20261021-0210/A-stability-20261021-1025/single-01/yakit-home`；`TMPDIR=/mnt/data/scan-outputs/ssa-review-20261019/artex-ir-delta-20261020/phase3-20261021-0210/A-stability-20261021-1025/single-01/tmp`；GOTMPDIR 同 run；无并发/规则预算环境覆盖，workers 配置=1（1 仅诊断）。
- 命令：

```sh
/mnt/data/scan-outputs/ssa-review-20261019/artex-ir-delta-20261020/phase3-20261021-0210/A-stability-20261021-1025/binary/yak code-scan -t /mnt/data/scan-outputs/ssa-review-20261019/artex-ir-delta-20261020/phase2-20261020-213104/source-snapshots/artex -l golang --database /mnt/data/scan-outputs/ssa-review-20261019/artex-ir-delta-20261020/phase3-20261021-0210/A-stability-20261021-1025/single-01/ssadb.db --format irify --output /mnt/data/scan-outputs/ssa-review-20261019/artex-ir-delta-20261020/phase3-20261021-0210/A-stability-20261021-1025/single-01/report/artex --no-save-risk --debug /mnt/data/scan-outputs/ssa-review-20261019/artex-ir-delta-20261020/phase3-20261021-0210/A-stability-20261021-1025/single-01 --log-level info --config /mnt/data/scan-outputs/ssa-review-20261019/artex-ir-delta-20261020/phase3-20261021-0210/A-stability-20261021-1025/single-01/diagnostic-config.json
```

- 进程 exit **0**；产品阶段：完成；wall **123.088s**；RSS 峰值 **2342728KiB**；最低 MemAvailable **22912040KiB**；guard=None。
- 计数：`{"ir_codes": 291419, "ir_indices_v1": 204393, "ir_offsets": 250654, "ir_programs": 1, "ir_sources": 396, "ir_types": 174349, "ssa_risks": 0}`；Risk：`{"RiskNums": 590, "actual_risks_len": 590}`。
- opcode 全表（未省略派生类）：

```json
{"BasicBlock": 39692, "BinOp": 8624, "Call": 20729, "ConstInst": 97657, "ExternLib": 1765, "FreeValue": 14277, "Function": 4262, "If": 8256, "Jump": 19537, "Loop": 1423, "Make": 8843, "Next": 1106, "Parameter": 7177, "ParameterMember": 10600, "Phi": 7738, "Return": 8100, "SideEffect": 5533, "Switch": 282, "TypeValue": 387, "UnOp": 744, "Undefined": 24687}
```

- Profile 实际文件（同 run、均非空）：
  - `/mnt/data/scan-outputs/ssa-review-20261019/artex-ir-delta-20261020/phase3-20261021-0210/A-stability-20261021-1025/single-01/cpu-pprof`：`20261021-104636-initial.cpu.prof`, `20261021-104736-104736.cpu.prof`。
  - `/mnt/data/scan-outputs/ssa-review-20261019/artex-ir-delta-20261020/phase3-20261021-0210/A-stability-20261021-1025/single-01/memory-pprof`：`20261021-104636-initial.mem.prof`, `20261021-104736-early01.mem.prof`, `20261021-104831-final.mem.prof`。
  - `/mnt/data/scan-outputs/ssa-review-20261019/artex-ir-delta-20261020/phase3-20261021-0210/A-stability-20261021-1025/single-01/goroutine-pprof`：`20261021-104636-initial.goroutine.debug2.txt`, `20261021-104636-initial.goroutine.prof`, `20261021-104736-early01.goroutine.debug2.txt`, `20261021-104736-early01.goroutine.prof`, `20261021-104831-final.goroutine.debug2.txt`, `20261021-104831-final.goroutine.prof`。
  - `/mnt/data/scan-outputs/ssa-review-20261019/artex-ir-delta-20261020/phase3-20261021-0210/A-stability-20261021-1025/single-01/mutex-pprof`：`20261021-104636-initial.mutex.prof`, `20261021-104736-early01.mutex.prof`, `20261021-104831-final.mutex.prof`。
  - `/mnt/data/scan-outputs/ssa-review-20261019/artex-ir-delta-20261020/phase3-20261021-0210/A-stability-20261021-1025/single-01/block-pprof`：`20261021-104636-initial.block.prof`, `20261021-104736-early01.block.prof`, `20261021-104831-final.block.prof`。
  - `/mnt/data/scan-outputs/ssa-review-20261019/artex-ir-delta-20261020/phase3-20261021-0210/A-stability-20261021-1025/single-01/allocs-pprof`：`20261021-104636-initial.allocs.prof`, `20261021-104831-final.allocs.prof`。
  - `/mnt/data/scan-outputs/ssa-review-20261019/artex-ir-delta-20261020/phase3-20261021-0210/A-stability-20261021-1025/single-01/db-stats`：`20261021-104636-initial.db.json`, `20261021-104736-early01.db.json`, `20261021-104831-final.db.json`。
  - `/mnt/data/scan-outputs/ssa-review-20261019/artex-ir-delta-20261020/phase3-20261021-0210/A-stability-20261021-1025/single-01/runtime-stats`：`20261021-104636-initial.runtime.json`, `20261021-104736-early01.runtime.json`, `20261021-104831-final.runtime.json`。
- CPU 要点（initial 完整样本的符号化 flat top，不把总 CPU 等同 wall）：53.29s 16.15% 16.15%    119.08s 36.09%  runtime.scanObjectsSmall；44.22s 13.40% 29.55%     44.22s 13.40%  runtime.spanClass.sizeclass (inline)；37.37s 11.33% 40.88%     91.48s 27.73%  runtime.tryDeferToSpanScan。
- 内存要点（final retained heap）：Showing nodes accounting for 348.89MB, 46.43% of 751.37MB total；分配累计见 `allocs-top.txt`，不据此声称泄漏。
- goroutine debug=2：20261021-104636-initial.goroutine.debug2.txt=618, 20261021-104736-early01.goroutine.debug2.txt=666, 20261021-104831-final.goroutine.debug2.txt=647；mutex/block aggregate wait 不当作死锁证明。
- 对照判定/问题/下一步：FAIL（与另一完整单并发样本对照）：31处callee有效性、14处opcode差异、offset集合233/243。问题 ISSUE-007/008/014；下一步确定性RED→GREEN。

### R-7/single-02 — 2026-10-21T10:48:37.653975+08:00

- 项目/输入：ARTEX，821 文件；manifest `f969ffeb972f95932b65909dabb39341facac06a5a07f4affbc7443eb81310a8`。
- 编译器：`c0e7b8358912718be98e8e1a620e9532914be052`；binary sha256 `c758f9118a5d50a3e79252e11a39e9cc1850e75cafcd51ea7b158b54d6044536`。
- run：`/mnt/data/scan-outputs/ssa-review-20261019/artex-ir-delta-20261020/phase3-20261021-0210/A-stability-20261021-1025/single-02`；DB：`/mnt/data/scan-outputs/ssa-review-20261019/artex-ir-delta-20261020/phase3-20261021-0210/A-stability-20261021-1025/single-02/ssadb.db`。
- 隔离：`YAKIT_HOME=/mnt/data/scan-outputs/ssa-review-20261019/artex-ir-delta-20261020/phase3-20261021-0210/A-stability-20261021-1025/single-02/yakit-home`；`TMPDIR=/mnt/data/scan-outputs/ssa-review-20261019/artex-ir-delta-20261020/phase3-20261021-0210/A-stability-20261021-1025/single-02/tmp`；GOTMPDIR 同 run；无并发/规则预算环境覆盖，workers 配置=1（1 仅诊断）。
- 命令：

```sh
/mnt/data/scan-outputs/ssa-review-20261019/artex-ir-delta-20261020/phase3-20261021-0210/A-stability-20261021-1025/binary/yak code-scan -t /mnt/data/scan-outputs/ssa-review-20261019/artex-ir-delta-20261020/phase2-20261020-213104/source-snapshots/artex -l golang --database /mnt/data/scan-outputs/ssa-review-20261019/artex-ir-delta-20261020/phase3-20261021-0210/A-stability-20261021-1025/single-02/ssadb.db --format irify --output /mnt/data/scan-outputs/ssa-review-20261019/artex-ir-delta-20261020/phase3-20261021-0210/A-stability-20261021-1025/single-02/report/artex --no-save-risk --debug /mnt/data/scan-outputs/ssa-review-20261019/artex-ir-delta-20261020/phase3-20261021-0210/A-stability-20261021-1025/single-02 --log-level info --config /mnt/data/scan-outputs/ssa-review-20261019/artex-ir-delta-20261020/phase3-20261021-0210/A-stability-20261021-1025/single-02/diagnostic-config.json
```

- 进程 exit **0**；产品阶段：**失败**（review failed，空库，不能以 exit=0 放行）；wall **189.145s**；RSS 峰值 **2156644KiB**；最低 MemAvailable **22460028KiB**；guard=None。
- 计数：`{"ir_codes": 0, "ir_indices_v1": 0, "ir_offsets": 0, "ir_programs": 0, "ir_sources": 0, "ir_types": 0, "ssa_risks": 0}`；Risk：`{"RiskNums": 567, "actual_risks_len": 567}`。
- opcode 全表（未省略派生类）：

```json
{}
```

- Profile 实际文件（同 run、均非空）：
  - `/mnt/data/scan-outputs/ssa-review-20261019/artex-ir-delta-20261020/phase3-20261021-0210/A-stability-20261021-1025/single-02/cpu-pprof`：`20261021-104840-initial.cpu.prof`, `20261021-104940-104940.cpu.prof`, `20261021-105040-105040.cpu.prof`, `20261021-105140-105140.cpu.prof`。
  - `/mnt/data/scan-outputs/ssa-review-20261019/artex-ir-delta-20261020/phase3-20261021-0210/A-stability-20261021-1025/single-02/memory-pprof`：`20261021-104840-initial.mem.prof`, `20261021-104941-early01.mem.prof`, `20261021-105041-early02.mem.prof`, `20261021-105141-early03.mem.prof`, `20261021-105145-final.mem.prof`。
  - `/mnt/data/scan-outputs/ssa-review-20261019/artex-ir-delta-20261020/phase3-20261021-0210/A-stability-20261021-1025/single-02/goroutine-pprof`：`20261021-104840-initial.goroutine.debug2.txt`, `20261021-104840-initial.goroutine.prof`, `20261021-104941-early01.goroutine.debug2.txt`, `20261021-104941-early01.goroutine.prof`, `20261021-105041-early02.goroutine.debug2.txt`, `20261021-105041-early02.goroutine.prof`, `20261021-105141-early03.goroutine.debug2.txt`, `20261021-105141-early03.goroutine.prof`, `20261021-105145-final.goroutine.debug2.txt`, `20261021-105145-final.goroutine.prof`。
  - `/mnt/data/scan-outputs/ssa-review-20261019/artex-ir-delta-20261020/phase3-20261021-0210/A-stability-20261021-1025/single-02/mutex-pprof`：`20261021-104840-initial.mutex.prof`, `20261021-104941-early01.mutex.prof`, `20261021-105041-early02.mutex.prof`, `20261021-105141-early03.mutex.prof`, `20261021-105145-final.mutex.prof`。
  - `/mnt/data/scan-outputs/ssa-review-20261019/artex-ir-delta-20261020/phase3-20261021-0210/A-stability-20261021-1025/single-02/block-pprof`：`20261021-104840-initial.block.prof`, `20261021-104941-early01.block.prof`, `20261021-105041-early02.block.prof`, `20261021-105141-early03.block.prof`, `20261021-105145-final.block.prof`。
  - `/mnt/data/scan-outputs/ssa-review-20261019/artex-ir-delta-20261020/phase3-20261021-0210/A-stability-20261021-1025/single-02/allocs-pprof`：`20261021-104840-initial.allocs.prof`, `20261021-105145-final.allocs.prof`。
  - `/mnt/data/scan-outputs/ssa-review-20261019/artex-ir-delta-20261020/phase3-20261021-0210/A-stability-20261021-1025/single-02/db-stats`：`20261021-104840-initial.db.json`, `20261021-104941-early01.db.json`, `20261021-105041-early02.db.json`, `20261021-105141-early03.db.json`, `20261021-105145-final.db.json`。
  - `/mnt/data/scan-outputs/ssa-review-20261019/artex-ir-delta-20261020/phase3-20261021-0210/A-stability-20261021-1025/single-02/runtime-stats`：`20261021-104840-initial.runtime.json`, `20261021-104941-early01.runtime.json`, `20261021-105041-early02.runtime.json`, `20261021-105141-early03.runtime.json`, `20261021-105145-final.runtime.json`。
- CPU 要点（initial 完整样本的符号化 flat top，不把总 CPU 等同 wall）：54.06s 16.27% 16.27%    120.70s 36.32%  runtime.scanObjectsSmall；43.82s 13.19% 29.46%     43.82s 13.19%  runtime.spanClass.sizeclass (inline)；37.26s 11.21% 40.67%     91.69s 27.59%  runtime.tryDeferToSpanScan。
- 内存要点（final retained heap）：Showing nodes accounting for 284.13MB, 44.97% of 631.87MB total；分配累计见 `allocs-top.txt`，不据此声称泄漏。
- goroutine debug=2：20261021-104840-initial.goroutine.debug2.txt=618, 20261021-104941-early01.goroutine.debug2.txt=667, 20261021-105041-early02.goroutine.debug2.txt=653, 20261021-105141-early03.goroutine.debug2.txt=653, 20261021-105145-final.goroutine.debug2.txt=650；mutex/block aggregate wait 不当作死锁证明。
- 对照判定/问题/下一步：FAIL，数据库锁失败导致清库；与基线不具备可比的完整IR，**不纳入方差区间**。问题 ISSUE-009/010；下一步查 saver/事务与CLI错误传播。

### R-7/single-03 — 2026-10-21T10:51:47.424414+08:00

- 项目/输入：ARTEX，821 文件；manifest `f969ffeb972f95932b65909dabb39341facac06a5a07f4affbc7443eb81310a8`。
- 编译器：`c0e7b8358912718be98e8e1a620e9532914be052`；binary sha256 `c758f9118a5d50a3e79252e11a39e9cc1850e75cafcd51ea7b158b54d6044536`。
- run：`/mnt/data/scan-outputs/ssa-review-20261019/artex-ir-delta-20261020/phase3-20261021-0210/A-stability-20261021-1025/single-03`；DB：`/mnt/data/scan-outputs/ssa-review-20261019/artex-ir-delta-20261020/phase3-20261021-0210/A-stability-20261021-1025/single-03/ssadb.db`。
- 隔离：`YAKIT_HOME=/mnt/data/scan-outputs/ssa-review-20261019/artex-ir-delta-20261020/phase3-20261021-0210/A-stability-20261021-1025/single-03/yakit-home`；`TMPDIR=/mnt/data/scan-outputs/ssa-review-20261019/artex-ir-delta-20261020/phase3-20261021-0210/A-stability-20261021-1025/single-03/tmp`；GOTMPDIR 同 run；无并发/规则预算环境覆盖，workers 配置=1（1 仅诊断）。
- 命令：

```sh
/mnt/data/scan-outputs/ssa-review-20261019/artex-ir-delta-20261020/phase3-20261021-0210/A-stability-20261021-1025/binary/yak code-scan -t /mnt/data/scan-outputs/ssa-review-20261019/artex-ir-delta-20261020/phase2-20261020-213104/source-snapshots/artex -l golang --database /mnt/data/scan-outputs/ssa-review-20261019/artex-ir-delta-20261020/phase3-20261021-0210/A-stability-20261021-1025/single-03/ssadb.db --format irify --output /mnt/data/scan-outputs/ssa-review-20261019/artex-ir-delta-20261020/phase3-20261021-0210/A-stability-20261021-1025/single-03/report/artex --no-save-risk --debug /mnt/data/scan-outputs/ssa-review-20261019/artex-ir-delta-20261020/phase3-20261021-0210/A-stability-20261021-1025/single-03 --log-level info --config /mnt/data/scan-outputs/ssa-review-20261019/artex-ir-delta-20261020/phase3-20261021-0210/A-stability-20261021-1025/single-03/diagnostic-config.json
```

- 进程 exit **0**；产品阶段：完成；wall **152.101s**；RSS 峰值 **2484936KiB**；最低 MemAvailable **22670896KiB**；guard=None。
- 计数：`{"ir_codes": 292048, "ir_indices_v1": 204897, "ir_offsets": 251796, "ir_programs": 1, "ir_sources": 396, "ir_types": 174627, "ssa_risks": 0}`；Risk：`{"RiskNums": 590, "actual_risks_len": 590}`。
- opcode 全表（未省略派生类）：

```json
{"BasicBlock": 39692, "BinOp": 8624, "Call": 20729, "ConstInst": 97758, "ExternLib": 1765, "FreeValue": 14456, "Function": 4262, "If": 8256, "Jump": 19537, "Loop": 1423, "Make": 8840, "Next": 1106, "Parameter": 7177, "ParameterMember": 10688, "Phi": 7796, "Return": 8100, "SideEffect": 5728, "Switch": 282, "TypeValue": 387, "UnOp": 744, "Undefined": 24698}
```

- Profile 实际文件（同 run、均非空）：
  - `/mnt/data/scan-outputs/ssa-review-20261019/artex-ir-delta-20261020/phase3-20261021-0210/A-stability-20261021-1025/single-03/cpu-pprof`：`20261021-105150-initial.cpu.prof`, `20261021-105250-105250.cpu.prof`, `20261021-105350-105350.cpu.prof`。
  - `/mnt/data/scan-outputs/ssa-review-20261019/artex-ir-delta-20261020/phase3-20261021-0210/A-stability-20261021-1025/single-03/memory-pprof`：`20261021-105150-initial.mem.prof`, `20261021-105250-early01.mem.prof`, `20261021-105350-early02.mem.prof`, `20261021-105415-final.mem.prof`。
  - `/mnt/data/scan-outputs/ssa-review-20261019/artex-ir-delta-20261020/phase3-20261021-0210/A-stability-20261021-1025/single-03/goroutine-pprof`：`20261021-105150-initial.goroutine.debug2.txt`, `20261021-105150-initial.goroutine.prof`, `20261021-105250-early01.goroutine.debug2.txt`, `20261021-105250-early01.goroutine.prof`, `20261021-105350-early02.goroutine.debug2.txt`, `20261021-105350-early02.goroutine.prof`, `20261021-105415-final.goroutine.debug2.txt`, `20261021-105415-final.goroutine.prof`。
  - `/mnt/data/scan-outputs/ssa-review-20261019/artex-ir-delta-20261020/phase3-20261021-0210/A-stability-20261021-1025/single-03/mutex-pprof`：`20261021-105150-initial.mutex.prof`, `20261021-105250-early01.mutex.prof`, `20261021-105350-early02.mutex.prof`, `20261021-105415-final.mutex.prof`。
  - `/mnt/data/scan-outputs/ssa-review-20261019/artex-ir-delta-20261020/phase3-20261021-0210/A-stability-20261021-1025/single-03/block-pprof`：`20261021-105150-initial.block.prof`, `20261021-105250-early01.block.prof`, `20261021-105350-early02.block.prof`, `20261021-105415-final.block.prof`。
  - `/mnt/data/scan-outputs/ssa-review-20261019/artex-ir-delta-20261020/phase3-20261021-0210/A-stability-20261021-1025/single-03/allocs-pprof`：`20261021-105150-initial.allocs.prof`, `20261021-105415-final.allocs.prof`。
  - `/mnt/data/scan-outputs/ssa-review-20261019/artex-ir-delta-20261020/phase3-20261021-0210/A-stability-20261021-1025/single-03/db-stats`：`20261021-105150-initial.db.json`, `20261021-105250-early01.db.json`, `20261021-105350-early02.db.json`, `20261021-105415-final.db.json`。
  - `/mnt/data/scan-outputs/ssa-review-20261019/artex-ir-delta-20261020/phase3-20261021-0210/A-stability-20261021-1025/single-03/runtime-stats`：`20261021-105150-initial.runtime.json`, `20261021-105250-early01.runtime.json`, `20261021-105350-early02.runtime.json`, `20261021-105415-final.runtime.json`。
- CPU 要点（initial 完整样本的符号化 flat top，不把总 CPU 等同 wall）：51.47s 15.71% 15.71%    113.73s 34.71%  runtime.scanObjectsSmall；43.11s 13.16% 28.87%     43.11s 13.16%  runtime.spanClass.sizeclass (inline)；36.98s 11.29% 40.15%     89.56s 27.33%  runtime.tryDeferToSpanScan。
- 内存要点（final retained heap）：Showing nodes accounting for 336.24MB, 45.32% of 741.95MB total；分配累计见 `allocs-top.txt`，不据此声称泄漏。
- goroutine debug=2：20261021-105150-initial.goroutine.debug2.txt=618, 20261021-105250-early01.goroutine.debug2.txt=667, 20261021-105350-early02.goroutine.debug2.txt=658, 20261021-105415-final.goroutine.debug2.txt=647；mutex/block aggregate wait 不当作死锁证明。
- 对照判定/问题/下一步：FAIL（与另一完整单并发样本对照）：31处callee有效性、14处opcode差异、offset集合233/243。问题 ISSUE-007/008/014；下一步确定性RED→GREEN。
