# SSA 扫描性能优化计划

> 目标仓库：`test-scan-large_projects`（worktree）
> 分支：`test/scan/large_projects`
> 状态：PLANNING（等待 goal 模式逐项实现）
> 最近更新时间：2026-08-11

## 背景与现状

用 `--debug` 对三个大项目做了全量测试（log + pprof 均保留）：

| 项目 | 编译→Save | SaveToDatabase | 扫描 | Risk / Success / Failed |
|---|---|---|---|---|
| grav（PHP，50M） | 5m36 | 8s | 18.4s | 69 / 36 / 0 |
| core（Java，1.9G） | 16m08 | 3m19 | 10m15.8s | 9193 / 128 / 0 |
| hadoop（Java，334M） | 28m59 | 4m20 | 11m19.8s | 8764 / 128 / 0 |

产物目录（保留）：
- `build/grav-run/debug/`、`build/core-run/debug/`、`build/hadoop-run19/debug-scan2/`

### pprof 结论（扫描期）

**CPU**：三个项目 65–70% 花在 GC（`gcDrain`、`scanObjectSmall`、`extractHeapBitsSmall`）。业务侧热点：
- hadoop：`getInstruction` 5.2%、`LazyInstruction.Self` 10.4%、`AppendPredecessor` 2.0%
- core：`GetResident` 7.1%（dbcache 锁竞争）、`lockSlow` 3.1%

**分配（hadoop 扫描全程 306GB）**：
| 热点 | 分配 | 占比 |
|---|---|---|
| `Program.NewValue`（cum） | ~100GB | 32.7% |
| `TakeSymbolSnapshot` | 41.3GB | 13.5% |
| `BitVector.Clone` | 32.3GB | 10.6% |
| GORM 查询链（Scope.Fields/DB.clone/search.clone/buildScanPlan） | ~45GB | ~15% |
| `SafeString.String` | 23.2GB | 7.6% |
| `BitVector.Or` | 9.2GB | 3.0% |

`Program.NewValue` 的 cum 100GB 子树（alloc_space）里，真正的浪费主要是：
- `codec.AnyToBytes`（源/位置解码）592MB
- `recalculateLineMappings`（MemEditor 行映射）275MB
- `ssa.NewValue` / `NewLazyInstructionFromIrCode`（懒加载）230MB / 176MB
- sqlite `GoStringN` / `GetIrCodeItemById` 203MB / 86MB
- `omap.Set` / `NewVersioned` / `NewRange` / json.unquote 各 100–120MB
- `reflect.unsafe_New`（Value 结构体本身，flat）1.65GB

**墙钟**：core 有 23 条规则撞 `work-limit=1,000,000` 后 bail（部分结果）；hadoop 有 1 条（检测Java 日志伪造攻击 680.3s，几乎等于整个扫描时长）。

---

## 设计原则

1. **`ssaapi.Value` 不能直接按 inst-id 全局复用**：`Value` 携带分析路径状态（`EffectOn`/`DependOn`/`Predecessors`/`anchorBits`/`cfgSiteInstID`/`DescInfo`/`runtimeCtx`）。`exclusive_z_top_defs.go:80-88` 故意为同一 inst 建多个 shell（`shadow`）以隔离 effecton 边。全局去重会破坏隔离语义。
2. 每个优化独立 commit；先小项目（engineercms / qor / grav）验证 Risk 不回归，再上 core/hadoop。
3. 每次运行都用 `--debug` 留 pprof，前后对比。

---

## 优化步骤（goal 模式，按序实现）

### 步骤 1：A1 — `Value` 结构体用 `sync.Pool` 回收（安全，先做）

**目标**：省掉每次 `NewValue` 的 `reflect.unsafe_New` 分配（~1.65GB flat / 306GB 全程），降低 GC。

**要点**：
- 在 `Program` 或包级加 `sync.Pool[*Value]`。
- `NewValue` 从池 `Get`，但**逻辑上每次仍是全新独立对象**：`Get` 后清空全部分析状态字段（`EffectOn`/`DependOn`/`Predecessors`/`anchorBits`/`cfgSiteInstID`/`DescInfo`/`runtimeCtx`/`users`/`operands`），`uid` 仍走 `p.id.Inc()` 生成新值。
- 不改变身份语义，不共享任何分析状态，只复用底层内存。
- 使用完放回池（注意：`Value` 被 `Values` 持有，放回时机需谨慎——只在确定不再被引用的短生命周期壳上放回；若风险高，可先只做池化 + 依赖 GC 兜底）。

**测试方法**：
- 单测：`NewValue` 连续创建大量 Value，验证 uid 单调不重复、状态字段均为空。
- 回归：engineercms / qor 扫描 Risk 数与改动前一致。
- pprof：`Program.NewValue` 的 `reflect.unsafe_New` flat 显著下降。

---

### 步骤 2：A2 — per-descent 指令解析去重

**目标**：同一 inst 在一条 dataflow descent 内只解析一次（懒加载 + AnyToBytes + RuneOffsetMap + NewRange），消除 `Program.NewValue` cum 100GB 里重复解析的主体。

**要点**：
- 在 `AnalyzeContext` 上按 inst-id 缓存「已解析的底层指令 + 位置/源范围」。
- 缓存作用域**限定在一条 descent**（一次 `GetTopDefs`/`GetBottomUses` 的 `AnalyzeContext` 生命周期），descent 结束即清。
- 跨 descent / 跨规则不共享 → 不污染跨规则 / 跨路径状态（满足设计原则 1）。
- 缓存命中时跳过 `codec.AnyToBytes` / `recalculateLineMappings` / `NewLazyInstructionFromIrCode` / `NewRange` 等重复工作。

**测试方法**：
- 单测：同一 descent 内同一 inst 只触发一次解析；跨 descent 不串。
- 回归：engineercms / qor 扫描 Risk 数与改动前一致。
- pprof：`Program.NewValue` cum 子树里 AnyToBytes / recalcLineMappings / NewLazyInstructionFromIrCode / NewRange 的分配显著下降。

---

### 步骤 3（候选）：A3 — 分析路径状态移出 `Value`

> 仅在 A1+A2 收益不足时启动。改动面大（`saveDataflowPath` / `AppendDependOn` / `AppendPredecessor` / `sf_value.go` 等），回归风险高。

**目标**：把 `EffectOn`/`DependOn`/`Predecessors`/`anchorBits` 从 `Value` 移到 per-descent side-table，让 `Value` 只作身份。

---

### 步骤 4：B — SFVM `TakeSymbolSnapshot` 懒构建 + 复用

**目标**：消除 `TakeSymbolSnapshot` 41.3GB 分配（每个 sfCheck 对父 SymbolTable 全表建两个 map）。

**要点**：
- 快照懒构建 + 复用：同一个 check 只建一次快照。
- 父表未变时直接短路返回空快照。
- `HasNewNamedValue` 改为遍历 result 时按需查父表，而非预建整套 map。
- 语义（判断 child 是否有"新"命名输出）不得改变，否则合并结果会丢。

**测试方法**：
- 单测：`TestTakeSymbolSnapshot_HasNewNamedValue` 及新增「父表不变短路」「复用不重建」用例。
- 回归：engineercms / qor 扫描 Risk 数与改动前一致。
- pprof：`TakeSymbolSnapshot` 分配显著下降。

---

### 步骤 5：C — `BitVector.Clone` / `Or` 写时复制（COW）

> 注：benchmark 文档记录过 Fix 2 已做过一次 COW（mergeAnchorBits 355GB→2.4GB），本次是剩余的 `applyScopedAnchorBits` / `sf_cfg*` 里的 `Clone`（当前 32.3GB）。

**目标**：把 `BitVector.Clone` 32.3GB + `Or` 9.2GB 进一步压低。

**要点**：
- `Clone` 改 COW：只保留共享 words + 引用计数，写（`Set`/`Or`）时才复制。
- 或 `applyScopedAnchorBits` 里对共享 identity 对象也改为按需复制。
- 严格保证「锚点永不原地改」约定，否则共享 words 被写坏会静默丢结果。

**测试方法**：
- 单测 + `-race` 并发回归（多个 goroutine 共享同一 BitVector，验证无写竞争）。
- 回归：engineercms / qor 扫描 Risk 数与改动前一致。
- pprof：`BitVector.Clone` / `Or` 分配下降。

---

### 步骤 6：D — GORM 读路径瘦身

**目标**：消除 GORM 查询链 ~45GB 分配（`Scope.Fields` / `DB.clone` / `search.clone` / `buildScanPlan`）。

**要点**：
- 热点单行查询（`GetIrCodeItemById` / `GetIrTypeById` 等）改原生 SQL + 预编译 statement + row 复用。
- 或对 GORM DB 用 `PrepareStmt` 缓存 + 复用同一 `*gorm.DB`。
- 保持多读单写锁语义不变。

**测试方法**：
- 单测：`GetIrCodeItemById` / `GetIrTypeById` 结果与改动前一致。
- 回归：core / hadoop 扫描 Risk 数与改动前一致。
- pprof：GORM `Scope.Fields` / `DB.clone` / `buildScanPlan` 分配显著下降。

---

### 步骤 7：E — 规则剪枝（砍墙钟）

**目标**：把撞 work-limit 的慢规则从 680s（log-forging）/ 577s（路径穿越）等降到分钟级，扫描墙钟直接减半。

**要点**：
- 对 top 慢规则（log-forging、路径穿越、MyBatis `${}`、xlsx-streamer 等）加文件/调用点预过滤 + early-stop。
- 或对超大图提高 work-limit 但配合规则内剪枝。
- 改的是规则本身，不碰 VM 语义；注意对比改动前后 Risk 命中率。

**测试方法**：
- 回归：engineercms / qor / hadoop / core 扫描 Risk 数与改动前对比（允许合理的命中率解释）。
- 时间：规则 duration 显著下降，撞 work-limit 的规则减少。

---

### 步骤 8（小项，最后做）：F

- `SafeString.String` 23.2GB 字符串拷贝去重。
- core 扫描期 `GetResident` 锁竞争（分片锁）。
- 日志降噪（`skip any type for Function` 刷屏污染 20MB log）。

---

## Todo 清单

- [x] **步骤 1** A1：`Value` sync.Pool 回收（目标：`unsafe_New` flat 1.65GB → 大幅下降）
- [x] **步骤 2** A2：per-descent 指令解析去重（目标：NewValue cum 100GB → 显著下降）
- [ ] **步骤 3** A3：分析路径状态移出 Value（候选，A1+A2 收益不足才启动）
- [x] **步骤 4** B：`TakeSymbolSnapshot` 懒构建 + 复用（目标：41.3GB → 接近 0）
- [x] **步骤 5** C：`BitVector` COW（目标：Clone 32.3GB + Or 9.2GB → 下降）
- [x] **步骤 6** D：GORM 读路径瘦身（目标：~45GB → 显著下降）
- [ ] **步骤 7** E：规则剪枝（目标：扫描墙钟减半，Risk 命中率可解释）
- [x] **步骤 8** F：小项（SafeString String 惰性缓存已实现；GetResident 锁 / 日志降噪待评估）

每个步骤完成后：
- 独立 commit（git add 该文件 + 对应代码 + 测试）。
- 更新本文件对应 checkbox 为 `[x]` 并记录实测收益（Risk 数 / 分配 / 时间）。
- 用 `--debug` 重跑 core 或 hadoop，把新 pprof 与 `build/core-run/debug/`、`build/hadoop-run19/debug-scan2/` 对比。
