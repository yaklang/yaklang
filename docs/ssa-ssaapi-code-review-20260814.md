# SSA / SSAAPI 近期代码 Review（DeepSeek 时代改动）

> 时间：2026-08-14 15:47 CST（`date -Iseconds` 已核实）
> 范围：main 分支 `common/yak/ssa`、`common/yak/ssaapi` 及同批改动的
> `common/yak/ssa/ssadb`、`common/utils/dbcache`、`common/utils/bitvector.go`、
> `common/utils/memedit`、`common/syntaxflow/sfvm`（2026-08-10 ~ 08-13 提交）。
> 方法：直接读 main 当前代码，不做 diff 对比；gofmt 用 `gofmt -l` 实测。

## A. 逻辑 / 正确性风险（建议优先处理）

### A1. fastPathMatch 的符号集合缓存无失效机制，且无锁读 SymbolTable

文件：`common/yak/ssaapi/sf_config.go:319-390`

- `fastMatchSymbolIDs` 在第一次 `check()` 时把 `$source`/`$sink` 的 id 集合缓存进
  `fastMatchIDs`，之后**永不失效**。而 `contextResult.SymbolTable` 在 descent 过程中
  会被 `clearup -> MergeByResultLocked` 持续更新。若符号集合在首次 check 之后变大
  （例如 `$sink` 在路径枚举中才被绑定），fast path 会一直用旧集合，include/exclude
  结果与完整子查询不一致——这是静默的扫描结果错误。
- `fastMatchSymbolIDs` 读 `SymbolTable.Get(name)` 时没有持有 `config.Mutex`
  （SymbolTable 的写都在该锁下），虽然当前单 rule 单 goroutine 未触发 race，
  但契约很脆弱。
- 建议：不缓存；或每次在 `config.Mutex` 下重建/校验；或给 SymbolTable 加版本号，
  缓存 key 带上版本。

### A2. MarkDirtyAsync 永久关闭 persistLimit 背压

文件：`common/utils/dbcache/databasex.go:946-951`（`persistLimitBypass.Store(true)`）

- `persistLimitBypass` 只 `Store(true)`，**从不复位**。第一次 mid-compile flush 之后，
  整个 cache 生命周期内 `enqueuePersist` 的 persistLimit 检查（databasex.go:162）
  永久失效，`BeginFinalDrain` 的"final drain 才豁免"语义也被破坏。
- 建议：把 bypass 作为 `MarkDirtyAsync` 调用作用域内的参数/临时状态，或在 enqueue
  完成后复位；不要用 cache 级原子量做一次性开关。

### A3. offsetSaved 去重：保存失败不重试 + key 与 DB 唯一索引不一致 + 无界增长

文件：`common/yak/ssa/database_cache_index.go:279-300`

- `saveOffsetDedup` 在 `offsetSaver.Save(offset)` **之前**就把 key 写入 `offsetSaved`。
  若 saver 失败，该 offset 永远不会被重试 → 数据丢失。
- 内存去重 key 是 `valueID|fileHash|start|end`，而 DB 唯一索引是
  `(program_name, value_id, file_hash, start_offset, end_offset, COALESCE(variable_name,''))`
  ——**漏了 variable_name**。两个同 range 不同 variable_name 的合法行会被内存去重误删。
- `offsetSaved` 无清理、无上限，大项目上随唯一 offset 数线性增长。
- 建议：key 与 DB 索引完全一致；保存成功后再标记（或失败时回滚标记）；考虑按
  program 生命周期清理。

### A4. Value pool 的 releaseValue 依赖"证明不可达"注释契约

文件：`common/yak/ssaapi/values.go:44-60`、`exclusive_z_top_defs.go:88-100,160-170`

- `releaseValue(shadow)` / `releaseValue(normalizedKey)` 的合法性完全靠注释声称
  （"never appended / GetMember never retains it"）。Value 被大量缓存引用
  （EffectOn/DependOn/users/operands/Predecessors/nodeId2ValueCache），任何一条
  路径保留指针，pool 复用就会静默产生错误结果（零值复用）。
- 用户此前已明确"ssaapi.Value 不能直接复用，因为保存分析路径相关的信息"。
- 建议：要么去掉 release 只靠 GC 兜底（保留 acquire 池化），要么给每个 release
  点加可验证的回归测试；不要靠注释维持安全。

### A5. FunctionType.String() 无锁写 Name/stringCache，重入保护并发下失效

文件：`common/yak/ssa/type.go:1304-1325`

- `String()` 写 `s.Name`（重入标记）和 `s.stringCache`，无任何锁。扫描并发
  （scan concurrency 默认 N-1）下两个 goroutine 同时调用会互相踩踏：一个 goroutine
  看到 `Name=="..."` 直接返回 `"..."`，或重入保护失效导致无限递归。
- `resetStringCache` 只在 `SetName`/`SetSideEffect` 调用；`Parameter`/`ReturnType`
  等字段在 `return.go:252-265`、`extern_instance.go:412` 等处被直接赋值/修改，
  缓存不会失效 → 返回旧签名。
- 建议：加 mutex 或用 `atomic.Pointer[string]`；所有变更点统一走 setter 并 reset。

### A6. AssignVariable 无条件更新全局 StaticMember：遮蔽污染 + 无界增长

文件：`common/yak/ssa/value.go:374-375`、`blue_print_member.go:12-20`

- 每次 `AssignVariable` 都查 `GetStaticMember(name)`，命中就
  `RegisterStaticMember(name, value, false)`。局部变量遮蔽全局名时，会把局部值
  写进全局 StaticMember → 跨文件/跨函数读到错误全局值。
- `appendBlueprintMember` 只对**最后一个**值去重；全局被交替赋不同值时 slice
  无界增长（A、B、A、B…），大项目热全局变量是内存泄漏。
- 建议：只在 builder 明确知道是全局写入时更新（如 `TryUpdateGlobalVariableByName`
  路径），并限制 StaticMember 历史长度。

### A7. native SQL 硬编码 opcode=5 / const_type='normal'，REGEXP 仅 SQLite 可用

文件：`common/yak/ssa/ssadb/native_read.go:250-279`

- `nativeGetIrCodeIDsByConstType` 硬编码 `opcode = 5`、`const_type = 'normal'`，
  与 GORM 回退路径（loader.go 的 `5, "normal"`）重复。任何一边改动，另一边静默
  分叉。
- `"string" REGEXP ?` 是 SQLite 专有语法；项目支持 PostgreSQL
  （`sanitize_for_pg.go`），PG 下该查询直接报错，然后回退 GORM——但 GORM 路径
  同样写 `REGEXP`？需确认回退是否也只在 SQLite 下可用。至少应把方言判断/参数化
  统一。
- 建议：提取命名常量；REGEXP 分支按 dialect 处理或复用 GORM 的表达式构造。

### A8. 单行 native read 静默吞错，与批量路径错误约定不一致

文件：`common/yak/ssa/ssadb/native_read.go:36-90`

- `nativeGetIrCodeItemById` / `nativeGetIrTypeItemById` 在 Scan 出错时返回 nil，
  调用方无法区分"无记录"和"DB 错误"（虽然会回退 GORM，但错误被掩盖，且每次
  DB 错误都变成双倍查询）。批量路径 `nativeGetIrCodesByIds` 则明确返回 error。
- 建议：单行路径也返回 `(row, error)`，与批量路径约定一致。

### A9. yieldIrCodes 原生路径全量物化，失去流式

文件：`common/yak/ssa/ssadb/loader.go:73-105`

- `nativeGetIrCodesByIds` 一次性返回全部行，`yieldIrCodes` 攒完整个 slice 才往
  channel 喂；旧 `FastPagination` 是边查边流。超大 id 集合（百万级）时内存峰值
  上升。
- 建议：按 `nativeIrCodeBatchChunk` 分块查询、边查边 `SafeFeed`。

### A10. 启动时全表查重 + 重复时不建索引导致保护缺失

文件：`common/yak/ssa/ssadb/database.go:125-270`

- `ensureUniqueIrCodesProgramCodeIndex` / `ensureUniqueIrOffsetsIndex` 在每次打开
  DB 时对整表 `GROUP BY` 查重；Hadoop 级大库启动开销大。
- 发现重复时"不建索引并期望 INSERT 失败"——但没有索引时 INSERT 不会失败，
  重复保护实际缺失，与"hard database invariant"注释矛盾。
- `sqlite_master` 是 SQLite 专有表，PG 下 exists 检查失效（靠 IF NOT EXISTS 兜底，
  但每次启动仍会跑全表查重）。
- 所有 `Row().Scan` 错误都被忽略。
- 建议：把唯一索引迁移做成显式 migration（一次性、带 dialect 分支），不要放在
  `patchIrCodeIndex` 每次启动路径里。

### A11. FromDatabase 的 staleIR 用 UpdatedAt 时间戳判断

文件：`common/yak/ssaapi/ssa_load_program.go:40-75`

- `!loaded.UpdatedAt.Equal(prog.irProgram.UpdatedAt)` 判断"是否被别的进程重编译"：
  时间戳相等性脆弱（同精度重编译、时钟/精度问题）；`prog.irProgram == nil` 时
  检测直接失效。
- 每次 `FromDatabase` 缓存命中都多一次 `ssadb.GetProgram` 查询，缓存收益下降。
- 建议：用 ir_programs 的版本/行数/compile_hash 等业务字段，或只在已知重编译
  场景（如 compile 后）主动失效缓存。

### A12. GOMEMLIMIT 全局副作用 + 拼写错误

文件：`common/yak/ssaapi/ssa_compile_utils.go:407-430`

- `gomeMemLimitOnce` 拼写错误（应为 goMem/gomem）。
- `debug.SetMemoryLimit(80% 系统内存)` 在编译路径里静默覆盖用户/进程已有设置；
  共享机器上 80% 软限制会引发过度 GC。
- `systemMemoryTotalBytes` 读 `/proc/meminfo`，仅 Linux 有效，macOS/Windows 静默
  失效。
- 建议：尊重已有 GOMEMLIMIT；把限制做成显式配置；非 Linux 提供替代实现或文档说明。

### A13. 重编译删除旧数据：错误被吞

文件：`common/yak/ssaapi/ssa_compile_project.go:207-218`、
`common/yak/ssa/ssadb/database.go:288-300`

- `ssadb.DeleteProgramIrCode(...)` 返回值被忽略，内部 `GormTransaction` 的错误也
  不返回。删除失败时继续重编译，最终以 UNIQUE 约束错误暴露，且错误信息与根因
  脱节。
- `GetProgram` 返回非"not found"错误时（DB 故障）也会静默跳过删除。
- 建议：删除失败直接返回错误并中止重编译。

### A14. FinishPersist 的 WaitGroup Done 先于 retry Add

文件：`common/utils/dbcache/cache.go:212-270`

- `updatedWhilePersisting` 重试路径：先 `persistWG.Done()` / `pendingCount.Add(-1)`，
  再为 retry `Add(1)`。并发 `Wait()`/`Barrier` 可能观察到 0 提前返回，漏掉重试项。
- 建议：先 Add 再 Done（或把 retry 的计数调整放在 Done 之前）。

### A15. Save.processBuffer 自适应 batch 无上限

文件：`common/utils/dbcache/save.go:200-260`

- `currentSaveSize` 可 10x/5x 无界增长；大积压时单批 slice 可能巨大（百万级
  Instruction），内存峰值不可控。
- 建议：给 currentSaveSize 设上限（如 maxSaveSize 的倍数）。

### A16. ReloadProgramFromDatabase 强制 GC/FreeOSMemory + 保 import hack

文件：`common/yak/ssaapi/path_a_reload.go:50-100`

- `CleanBaseline()` 内已 `runtime.GC()`，随后又 `runtime.GC()` + `debug.FreeOSMemory()`，
  生产扫描路径强制 STW/归还内存，可能造成明显停顿。
- `var _ = ssa.ProgramCacheKind(0) // keep ssa import used` 是保 import 的 hack，
  应直接删掉无用 import。
- `CleanBaseline` 注释声称 "The Program is removed from ssaapi.ProgramCache"，
  但 ssa 包无法引用 ssaapi，实际移除发生在 `ReloadProgramFromDatabase`——注释误导。

### A17. ssa_compile_fs.go 阈值与注释不符

文件：`common/yak/ssaapi/ssa_compile_fs.go:430-470`

- 注释说 "The threshold avoids flushing on small projects"，但 batch-boundary 的
  `FlushCompileUnit(strings.Join(unitKeys, ","))` 是**无条件**执行的；阈值只在
  per-unit callback 里检查。小项目仍会在每个 batch 边界触发 flush。
- 注释重复一句 "Skip both for incremental compile to preserve overlay."
  （同一段出现两次）。

## B. 样式 / 可维护性

### B1. gofmt 未过

- `common/yak/ssa/ssadb/database.go`：import 顺序错误（`strings` 应排在 gorm 前），
  `gofmt -l` 实测报出。

### B2. 注释矛盾 / 过时

- `database_cache_instruction.go` `flushCompileUnitWriter`：注释先写 "Use FlushKeys
  (synchronous...)"，紧接着又写 "Use MarkDirtyAsync (async...)"，自相矛盾，最终
  代码用的是 MarkDirtyAsync。
- `program_unit.go` `ReleaseCompletedUnitMemory`：注释说 "the GC itself runs once
  in FlushCompileUnit after this returns"，实际 `FlushCompileUnit` 已删除
  `runtime.GC()`。
- `database_cache.go` `CloseWithoutSave`：注释说 "Type/index/source stores don't
  have CloseWithoutSave"，实际 `typeStore.close` / `indexStore.Close` 存在。
- `databasex.go` `FlushStats`：注释说 "stub with zero values — will be populated"，
  实际字段已填充。
- `database_cache.go` `CleanBaseline`：见 A16。

### B3. 中英混杂注释

- `deferred_build.go` 同一函数先中文后英文两套注释；`lazy_builder.go`、
  `memedit/editor.go` 等文件中文注释与英文注释混排。建议统一英文（项目主体）。

### B4. 测试钩子进生产代码

- `ssadb/native_read.go`：`NativeIrCodeBatchReads()` / `NativeConstTypeIDQueries()`
  导出计数器。
- `dbcache/databasex.go`：`MarkDirtyForTest`、`FlushKeysStats` 等 test-only 方法。
- 建议移到 `_test.go` 或加 build tag。

### B5. 魔数 / 硬编码

- `database_cache_instruction.go`：`maxPasses = 16`；`databasex.go`：
  `maxPasses = 8`、`resolvePersistLimit` 的 `512`；`ssa_globalBulePrint.go`：
  `len(rawAv.memberPairs) <= 5000`；`full_type_name.go`：`maxFullTypeNameEntries
  = 200`；`pprof_collector.go`：`5*time.Minute + 2*time.Second`。
- `fullTypeName` 截断 200 是静默数据丢失（append 顺序截尾，可能丢掉重要名字），
  建议至少记录/可配置。

### B6. 死代码 / 无用状态

- `dbcache/databasex.go:556` `enqueueCloseRequests` 无调用方（仅测试注释提及）。
- `ssa/lazy_builder.go:64` `l.build.Store(true)` 只写不读，`build atomic.Bool`
  已成死状态。

### B7. 重复代码

- `native_read.go`：单行与批量路径各有一份 50 列 `Scan`，列清单虽已抽
  `nativeIrCodeColumns`，Scan 仍重复。建议抽 `scanIrCode(rows) (*IrCode, error)`。

### B8. 命名 / 小问题

- `gomeMemLimitOnce` 拼写（见 A12）。
- `flushedUnits map[string]bool` 应使用 `map[string]struct{}`。
- `ssadb/database.go` 注释里出现 Unicode 弯引号 `COALESCE(variable_name, ”)`。

### B9. 行为变更未充分评估（默认值/语义）

- `ssaconfig/config.go`：scan concurrency 默认从 5 改为 `GOMAXPROCS-1`，
  compile concurrency 从 `GOMAXPROCS/2` 改为 `GOMAXPROCS-1`。32 核机器上
  scan 并发 5 → 31，规则并发、内存、时序都变，需要回归验证。
- `ssa_globalBulePrint.go`：全局变量从 container memberPairs 改为 StaticMember
  直查；`TryUpdateGlobalVariableByName` 不再更新 container 关系，任何仍走
  `GetLastWinsMemberPairs`/`GetLatestMemberByKeyString` 的路径看不到更新。
- `ssa/value.go` `readValueEx` 新增全局回退：每次变量读 miss 都会触发
  `GetGlobalVariable`（内部 `Build()`），热路径上需要确认开销。

## C. 建议的修改顺序

1. 先修正确性：A1（fast path 缓存失效）、A2（背压永久关闭）、A3（offset 去重）、
   A5（String 并发）、A6（全局遮蔽）。
2. 再修数据可靠性：A7/A8（native SQL 方言与错误）、A10（唯一索引迁移）、A13
   （删除错误传播）、A14（WaitGroup 顺序）。
3. 最后清理样式：B1 gofmt、B2 注释、B4 测试钩子、B6 死代码、B8 命名。

> 本报告只做 review，未修改任何生产代码。

## 更新记录（2026-08-19，`test/scan/large_projects` @ 55dfad995）

本轮按本报告做了“代码结构与逻辑”修复，并逐项复核当前源码：

### 已修复（本轮）

- **A1**：`fastMatchSymbolIDs` 不再缓存 `fastMatchIDs`。符号集合在 descent 中会增长，旧缓存会静默错判 include/exclude；现在每次在 `config.Mutex` 下重建（集合通常很小），并删除 `fastMatchMu`/`fastMatchIDs` 字段。
- **A2**：`persistLimitBypass` 由一次性 `atomic.Bool` 改为作用域计数（`atomic.Int32`），`MarkDirtyAsync` 返回即复位；新增回归测试 `TestMarkDirtyAsync_ResetsPersistLimitBypass`。
- **A3（部分）**：内存去重 key 加入 `variable_name`，与 DB 唯一索引 `(program_name, value_id, file_hash, start_offset, end_offset, COALESCE(variable_name,''))` 对齐；标记移到 enqueue 之后，避免 enqueue 失败即“永久 poison”。`offsetSaved` 无界增长仍未处理（保留为后续项）。
- **A8**：`nativeGetIrCodeItemById` / `nativeGetIrTypeItemById` 改为返回 `(row, error)`；`sql.ErrNoRows` 返回 `(nil, nil)`，其余错误返回 error，not-found 不再走 GORM 双查，DB 错误仍回退 GORM。
- **A13**：`DeleteProgramIrCode` 返回 error，`deleteProgramCodeOnly`/`deleteProgramAuditResult` 上抛 Exec 错误；compile 的两个删除路径失败直接报错中止；`DeleteProgram` 记录删除错误。
- **A15**：`processBuffer` 自适应 batch 上限 16x 基础 saveSize，防止积压时单批 slice 无界增长。
- **A17**：batch-boundary 的 `FlushCompileUnit` 与注释一致，同样按 `flushCompileUnitThreshold()` 门控；删除重复注释。
- **B7**：抽取 `scanIrCode(rowScanner)`，单行/批量 50 列 `Scan` 共用，不再各自维护。
- **B6**：删除无调用方的 `enqueueCloseRequests`、随之变死的 `waitPendingBelow`，以及 `lazyBuilder.build` 只写不读状态。
- **B8（部分）**：`flushedUnits` 改 `map[string]struct{}`；`gomeMemLimitOnce` 拼写修复；`path_a_reload.go` 删除 `var _ = ssa.ProgramCacheKind(0)` 保 import hack。
- **B2（部分）**：`database_cache_instruction.go`、`program_unit.go`、`database_cache.go`、`databasex.go` 的矛盾/过时注释更新。
- **B1**：`ssadb/database.go` 注释中的 `COALESCE(variable_name, '')` 改为无引号描述，`gofmt -l` 干净。

### 已验证（无需改）

- **A14**：当前源码已经是先 `Add(1)` 再 `Done()`（`FinishPersist` 内 retry 计数在锁内先加），报告所述顺序问题已不存在。

### 仍为风险/未改（语义变更，保持假设状态）

（第七轮后：A6 全局遮蔽、A11 stale 判断需要 schema/语义决策；A10 大迁移、B9 默认并发语义需单独验证；B3 中文注释统一、B5 剩余魔数/截断为纯风格项。）

- A4（Value pool 契约）、A5（FunctionType.String 并发）、A6（全局 StaticMember 遮蔽/无界）、A7（native SQL 方言/魔数）、A9（yieldIrCodes 全量物化）、A10（启动全表查重/索引迁移）、A11（UpdatedAt 判断 stale）、A12（GOMEMLIMIT 全局副作用）、A16（GC/FreeOSMemory 有意保留；仅删除 hack/import）、B3（中英混杂注释）、B4（测试钩子进生产）、B5（魔数/截断）、B9（默认并发语义）。

### 验证记录（2026-08-19 17:20 CST）

- `go build -o /tmp/yak-gate-build ./common/yak/cmd/yak.go` 通过；本轮全部改动文件 `gofmt -l` 干净。
- `scripts/ssa-test.sh` 全量门禁：在 `common/yak/go2ssa/test` 因 30s 包超时中止（goroutine dump 显示仍在做 ANTLR 反序列化）。该包在本机 baseline（main worktree，干净代码）单独跑也需 30.4s+，属于门禁超时过紧/机器负载，非本批改动引入；用宽松超时单独跑该包通过（本 worktree 49.6s）。
- 目标包在隔离 `YAKIT_HOME` 下全部通过：
  - `common/utils/dbcache/...` ok（含 `TestMarkDirtyAsync_ResetsPersistLimitBypass`）
  - `common/yak/ssa/...` ok（含 `TestIndexStore_SaveOffsetDedupMatchesDBUniqueKey`）
  - `common/yak/ssa/ssadb` ok（含 native read 单行错误语义测试）
  - `common/yak/ssaapi` ok（含 `TestSFCheck_FastPath_SymbolTableGrowthIsFresh`）
  - `common/yak/ssaapi/test/ssatest` ok
- 说明：未使用共享 `.db/`（其 `default-yakssa.db` 有历史残留，会让 `TestProgram_NewProgram` 计数多 1），与 `scripts/ssa-test.sh` 相同用临时 `YAKIT_HOME` 验证。

## 更新记录（2026-08-19 第二轮，@ 254434248）

- **A4 已处理**：移除 `exclusive_z_top_defs.go` 两处 `releaseValue`（mask `shadow` 与 `normalizedKey`），只保留 `NewValue` 失败路径的池回收。按报告建议“去掉 release 只靠 GC 兜底（保留 acquire 池化）”，不再依赖注释契约保证可达性。
- **A7 已修复**：抽出 `constTypeOpcode` / `constTypeName` 常量，GORM fallback 与 native-SQL 共用，防止一侧改动另一侧分叉；native ConstType 非精确匹配按 dialect 选择操作符（postgres/postgresql 用 `~`，其余用 `REGEXP`），与 GORM fallback 的 switch 完全一致；新增 `TestConstTypeRegexpOperatorByDialect`。
- **B5（部分）**：`resolvePersistLimit` 的 `512` 下限与 `ssa_globalBulePrint.go` 的 `5000` memberPairs 上限提取为命名常量（`minPersistLimit`、`maxRestoredGlobalMemberPairs`）。

### 验证（第二轮）

- 隔离 `YAKIT_HOME` 下 `common/utils/dbcache/...`、`common/yak/ssa/...`（含 ssadb）、`common/yak/ssaapi` 全部通过。
- 改动文件 `gofmt -l` 干净；`go build -o /tmp/yak-gate-build ./common/yak/cmd/yak.go` 通过。

## 更新记录（2026-08-19 第三轮，@ a3f74ee3）

- **B4（部分）**：`NativeIrCodeBatchReads` / `NativeConstTypeIDQueries` 两个仅测试使用的导出访问器移入 `loader_regression_test.go`（同包），生产文件只保留未导出的计数器与自增；`MarkDirtyForTest`/`FlushKeysStats` 因外部测试包引用暂留（后续可加 build tag 或专用测试辅助包再处理）。

### 验证（第三轮）

- 隔离 `YAKIT_HOME` 下 `common/yak/ssa/ssadb` 通过；改动文件 `gofmt -l` 干净。

## 更新记录（2026-08-19 第四轮，@ 36837d6f）

- **A9 已修复**：`yieldIrCodes` 的 DB miss 集合先排序再去重，再按 `nativeIrCodeBatchChunk` 分块查询并逐块 `SafeFeed`，不再一次性物化全部行；块内/块间仍保持升序 code_id，顺序契约不变（新增 `TestYieldIrCodes_A9_StreamsChunksInAscendingOrder`，2500 行跨多块验证）。任意块 native 出错时，剩余部分回退 GORM `FastPagination`，不会把 DB 错误当成空结果。

### 验证（第四轮）

- 隔离 `YAKIT_HOME` 下 `common/yak/ssa/ssadb` 全量通过（含 A2/A3/A8/A9 相关测试）；`go build ./common/yak/cmd/yak.go` 通过；改动文件 `gofmt -l` 干净。

## 更新记录（2026-08-19 第五轮，@ 708f5efa3）

- **A10（部分）**：`ensureUniqueIrCodesProgramCodeIndex` / `ensureUniqueIrOffsetsIndex` 的目录查询改为按 dialect 走 `sqlite_master`（SQLite）或 `pg_indexes`（PostgreSQL），不再假设 `sqlite_master`；`Row().Scan` 错误不再静默吞掉（记 Warn）；新增 `TestUniqueIndexCatalogHelpers_DialectAware`。启动时“全表查重”仍只在索引不存在时执行一次，未做大迁移重构。
- **A12（部分）**：大型项目路径的 80% GOMEMLIMIT 现在尊重已显式配置的 `GOMEMLIMIT` 环境变量（`defaultLargeProjectMemLimit`），不再无条件覆盖进程设置；新增对应单元测试。`/proc/meminfo` 仅 Linux 的局限与 80% 默认值语义仍保留。
- **A16（部分）**：`ReloadProgramFromDatabase` 删除 `CleanBaseline` 之后冗余的 `runtime.GC()`（`debug.FreeOSMemory` 本身会先 GC 再归还内存），保留 Path A 的显式内存归还设计。

### 验证（第五轮）

- 隔离 `YAKIT_HOME` 下 `dbcache`/`ssa`（含 ssadb）/`ssaapi`/`ssatest` 全部通过；`go build ./common/yak/cmd/yak.go` 通过；改动文件 `gofmt -l` 干净。

## 更新记录（2026-08-19 第六轮，@ cfdc87702）

- **A5（部分，缓存失效修复）**：新增 `FunctionType.SetParameter/SetReturnType/SetParameterLen/SetIsVariadic`，把 `return.go`（Finish 推导参数/返回类型）、`extern_instance.go`（InsertSliceItem 前插参数）、`generic_type.go`（ApplyGenericType 改写）等 string-affecting 变更统一走 setter，`stringCache` 不再过期返回旧签名；新增 `Test_FunctionType_StringCacheInvalidatedByMutation`（含自引用终止）。
- **A5 剩余**：`String()` 的 `Name`/`stringCache` 并发写与重入保护仍需锁或原子指针，留作下一项（涉及并发语义与重入，单独做）。

### 验证（第六轮）

- 隔离 `YAKIT_HOME` 下 `dbcache`/`ssa`（含 ssadb）/`ssaapi`/`ssatest` 全部通过；`go build ./common/yak/cmd/yak.go` 通过；改动文件 `gofmt -l` 干净。

## 更新记录（2026-08-19 第七轮，@ 50b1f7ae）

- **A5 完成**：`FunctionType.String()` 不再把 `Name` 当重入标记写（消除并发踩踏），改用 `atomic.Pointer[string]` 做 stringCache、`atomic.Bool` 做计算中标记、`atomic.Int64` 版本号在变更时失效缓存；并发调用要么拿完整签名，要么短暂拿 `...` 占位符，不会死锁/无限递归/写坏 Name。`-race` 下通过新增的 `Test_FunctionType_StringConcurrentSafe`。
- **A11（文档部分）**：`UpdatedAt` 仍是唯一权威新鲜度字段；`createDeleteOnlyProgram` 里对 `irProg.ID==0` 的防御分支与 `UpdateProgramWithError`（按 id 更新）行为一致，属于死代码，已顺手清理。

### 验证（第七轮）

- `go test -race -run 'Test_FunctionType_String' ./common/yak/ssa` 通过；完整 `dbcache`/`ssa`（含 ssadb）/`ssaapi`/`ssatest` 通过；`go build` 通过；改动文件 gofmt 干净。

## 更新记录（2026-08-19 第八轮，@ 9155d75f）

- **B3 完成（本批文件）**：`lazy_builder.go` / `deferred_build.go` 的中英混杂注释统一为英文；项目其余历史中文注释保留（大规模统一需单独 PR，避免噪音 diff）。
- **B5 完成（本批）**：instruction-store Close 的 `maxPasses=16` 改为命名常量 `closeFlushMaxPasses`；`fullTypeName` 截断不再是静默丢失——`fullTypeNameAdd`/`fullTypeNameSet` 截断时增加原子计数，新增 `TestFullTypeNameTruncationObservable`。

### 验证（第八轮）

- 隔离 `YAKIT_HOME` 下 `dbcache`/`ssa`（含 ssadb）/`ssaapi`/`ssatest` 全部通过；`go build` 通过；改动文件 gofmt 干净。

## 更新记录（2026-08-19 第九轮，@ d2b6b1b8）

- **A6（遮蔽污染）已修复**：`AssignVariable` 的 StaticMember 同步更新只对非 local 变量生效（`!variable.GetLocal()`）。函数内局部变量遮蔽全局名时，不再把局部值写进 `GlobalVariablesBlueprint`，避免跨文件/跨函数读到错误全局值；全局变量的正常更新路径（`AddGlobalVariable` 声明 / `TryUpdateGlobalVariableByName` / `storeField`）不受影响。新增 `TestLocalShadowDoesNotOverwriteGlobalStaticMember`（Build 全局在前、局部赋值在后，修复前会失败）。
- **A6 剩余**：`appendBlueprintMember` 只对最后一个值去重导致交替赋值时 StaticMember slice 无界增长，需要限制历史长度/去重策略（涉及 `GetStaticMembers` 消费者语义，保留）。

### 验证（第九轮）

- 隔离 `YAKIT_HOME` 下 `dbcache`/`ssa`（含 ssadb）/`ssaapi`/`ssatest` 全部通过；改动文件 gofmt 干净。

## 更新记录（2026-08-19 第十轮，@ 26678bdf）

- **A6 完成**：`RegisterStaticMember` 增加每名历史上限 `maxStaticMemberHistory = 16`（保留最近值，`GetStaticMember` 始终读最新），交替赋值不再无界增长；新增 `TestStaticMemberHistoryBounded`。叠加上一轮的 local 遮蔽修复，A6 两个子问题（遮蔽污染、无界增长）均已处理。

### 验证（第十轮）

- 隔离 `YAKIT_HOME` 下 `dbcache`/`ssa`（含 ssadb）/`ssaapi`/`ssatest` 全部通过；`go build` 通过；改动文件 gofmt 干净。

## 更新记录（2026-08-19 第十一轮，@ cfa60e19f）

- **B4（dbcache）**：删除生产代码里的 `MarkDirtyForTest`，测试改走真实非阻塞 API `MarkDirtyAsync`（覆盖同样的 dedup 场景）；`FlushKeysStats`/`FlushStats` 保留为通用可观测 API（外部测试包引用）。
- **B5（剩余魔数）**：`databasex.go` 的 close 轮数 `8` 命名化为 `closeFlushMaxPasses`；`pprof_collector.go` 的 `5*time.Minute+2s` 改为基于 `pprofCPUDurationHigh` 的表达式。

### 验证（第十一轮）

- 隔离 `YAKIT_HOME` 下 `dbcache`/`ssa`（含 ssadb）/`ssaapi`/`ssatest` 全部通过；`go build` 通过；改动文件 gofmt 干净。

## 更新记录（2026-08-19 第十二轮，@ 234ccd834）

- **A11（部分）**：新增 `GetProgramUpdatedAt` 轻量查询（只取 `updated_at`，不加载 FileList/ExtraFile 大列）。`FromDatabase` 缓存命中时：
  - `irProgram` 已存在（长期缓存）：走轻量查询；程序行不存在 → 判定 stale 并移除缓存（原实现会漏掉“程序被删除”的场景）；时间比较从 `!Equal` 改为 `After`，避免时间精度噪声误判；命中且未过期时顺带刷新 `UpdatedAt`。
  - `irProgram` 为 nil（编译产生的缓存项）：保留原完整 `GetProgram` 行为，一次补齐 Recompile 所需 ConfigInput/overlay 元数据（否则 TestJarRecompile 会因“引擎版本过旧”失败）。
- 新增 `TestGetProgramUpdatedAtLightQuery`（存在/不存在两种路径）。

### 验证（第十二轮）

- `TestJarRecompile` 单独与全量 `dbcache`/`ssa`（含 ssadb）/`ssaapi`/`ssatest` 均通过；`go build` 通过；改动文件 gofmt 干净。

## 更新记录（2026-08-19 第十三轮，@ d3dfae305）

- **A3（剩余）完成**：`offsetSaved` 去重 map 改为按 indexStore 生命周期释放——`Close()` 时清空（编译结束、offset saver 关闭后不再有新的 offset），大项目上不再随唯一 offset 数跨 program 线性增长；`Flush`（中途批量）不清空，避免重复入队触发 DB UNIQUE 冲突。新增 `TestIndexStore_CloseClearsOffsetDedup`。
- **B2 补充**：`flush_observability_test.go` 里“FlushStats 尚不存在/RED”的过时注释更新为当前实现描述。

### 验证（第十三轮）

- 隔离 `YAKIT_HOME` 下 `dbcache`/`ssa`（含 ssadb）/`ssaapi`/`ssatest` 全部通过；`go build` 通过；改动文件 gofmt 干净。

## 更新记录（2026-08-19 第十四轮，@ d08fdb57e）

- **A12（剩余）完成**：`systemMemoryTotalBytes` 从共享文件拆为 build-tag 实现：
  - `system_memory_linux.go`：`/proc/meminfo`（原实现）
  - `system_memory_darwin.go`：`unix.SysctlUint64("hw.memsize")`（新增支持）
  - `system_memory_other.go`：返回 0，非 Linux/macOS 不再“静默失效”，而是显式跳过 legacy 80% 限制
  - `defaultLargeProjectMemLimit` 同时尊重 `YAK_SSA_COMPILE_MEM_LIMIT`（opt-in adaptive GC 策略），避免两条路径互相覆盖；测试补充该分支

### 验证（第十四轮）

- Linux 全量 `dbcache`/`ssa`（含 ssadb）/`ssaapi`/`ssatest` 通过；`go build` 通过。
- Darwin/Windows 交叉编译被仓库既有 cgo 依赖（go-pcre2-lite、yaklang/pcap 无对应原生绑定）阻塞，与本改动无关；Darwin 分支 API 与 `golang.org/x/sys v0.30.0` 的 `unix.SysctlUint64` 匹配。

## 更新记录（2026-08-19 第十五轮，@ d883858b3）

- **B3（memedit/editor.go）**：评审文档点名的 `memedit/editor.go` 全部 106 处中英混杂注释统一为英文（含并发安全说明、路径拼接语义、编辑 API 描述），无行为变更。

### 验证（第十五轮）

- `memedit`/`ssa`（含 ssadb）/`ssaapi` 通过；改动文件 gofmt 干净。

## 更新记录（2026-08-19 第十六轮，@ 09aff33f9）

- **A11 完成**：`IrProgram` 新增 `CompileGeneration` 业务字段（gorm AutoMigrate 自动加列，旧行默认 0）；`UpdateProgramWithError` 每次元数据/编译保存自增。`FromDatabase` 缓存命中以 generation 为主判据（`loaded > cached` → 失效），仅当双方 generation 均为 0（legacy 行）才回退 `updated_at.After` 判断；程序行不存在仍判 stale。编译产生的 nil `irProgram` 缓存项保留完整加载路径。
- 测试：
  - `TestGetProgramFreshnessLightQuery`（updated_at + generation 轻量查询）
  - `TestFromDatabaseStaleOnCompileGeneration`（跨进程模拟：删 IR + 直接 bump generation → 缓存替换）
  - 原 `TestFromDatabaseReloadsWhenIrUpdatedAtChanges` 改为 `TestFromDatabaseReloadsWhenCompileGenerationChanges`：元数据-only 的 updated_at 变化不再丢 IR 缓存，generation bump 才丢。

### 验证（第十六轮）

- 全量 `dbcache`/`ssa`（含 ssadb）/`ssaapi`/`ssatest` 通过；`go build` 通过；改动文件 gofmt 干净。

## 更新记录（2026-08-19 第十七轮，@ c0a864e8a）

- **A10 完成**：唯一索引修复/创建抽成独立的 `migrateUniqueIrCodeIndexes(db)`，与常规性能索引路径分离，代码结构上即为“一次性迁移”：两个 helper 都先做 dialect-aware 目录检查，索引已存在直接返回，只有缺少索引的 legacy 库才触发重复扫描/去重/建索引。原有全部唯一索引测试继续通过。

### 验证（第十七轮）

- 全量 `dbcache`/`ssa`（含 ssadb）/`ssaapi`/`ssatest` 通过；`go build` 通过；改动文件 gofmt 干净。

## 终态（2026-08-19，@ ca234f8b9）

- 评审 A1–A17、B1–B8 的全部机械安全项已实现并验证（见各轮记录）；A10/A11/A12/A16 按“部分修复 + 明确理由”或完整修复收尾。
- 唯一剩余决策项：**B9 默认并发**（scan/compile 默认 `GOMAXPROCS-1`）。当前实现为统一的 `DefaultCPUConcurrency()`，并保留 `WithScanConcurrency` / `WithCompileConcurrency` 显式覆盖；维持现状或回退到 5 需要作者确认，属语义/回归决策，不在无授权下改动。
- 可选风格项：B3 全仓中文注释统一（约 170 文件、2400 行），建议单独 PR。
