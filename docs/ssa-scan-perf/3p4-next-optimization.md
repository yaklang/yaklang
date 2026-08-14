# 3+4 组合后续优化计划（基于 Debug+pprof 热点分析）

日期：2026-08-14
分析对象：build/3.1-closure/3p4-hadoop-compile-20260813-123653/（9 组 CPU/heap/goroutine pprof）
对照：build/hadoop-run19/（main run19，完整 compile+scan）

## 0. 执行约束（沿用既有纪律）

- 不修改 4 号 worktree（enhance-ssa-scan_perf2），不修改 PR，不 push。
- 每个优化独立 commit；先写最小 deterministic RED，再做最小修复；不加无验证证据的优化。
- 性能结论必须以同 target/rules/命令/口径的 Debug+pprof 实测为准；Risk 与规则结果不得回退。
- 涉及 BitVector/SSA 语义的所有改动保留 alias 回归测试。

## 1. 数据基础

3+4 组合完整 compile+scan（Hadoop）：

| 指标 | 数值 |
|---|---:|
| 编译窗口（compile stage→f4_finish） | 29m23s（main run19 27m49s，+5.6%） |
| IR flush（final barrier enter→done） | 4m54s（main run19 4m20s，+34s） |
| 扫描窗口（规则开始→Risk） | 约 7m42s（DB-only 对照 355s） |
| 累计 alloc_space（final） | 1,047.9GB（main run19 1,003.6GB） |
| live heap（final） | 15.7GB（GOMEMLIMIT 24.97GB） |
| RSS 峰值 | 26.8GB |
| 峰值 goroutine | 1002（编译 809；main 编译 381） |
| Risk | 9026（DB-only 8764，均不视为回退） |

阶段划分：编译 12:37:23→13:06:46；flush 13:06:46→13:11:40；扫描 13:11:42→13:19:24。

## 2. 候选优化清单（按绝对成本 × 可优化性排序）

### P0-1 BitVector.Or / ensure 所有权重构（扫描收尾第一大 alloc 热点）

- 证据：扫描收尾段（13:17:44→13:19:25，约 100s）新增 alloc 30.1GB；DB-only 对照后半段 +22.6GB；累计约 33GB（DB-only final alloc 的 17.6%）。纯扫描中段仅 +1.8GB，说明分配集中在规则 finalize/重 anchor 合并。
- 代码位置：common/utils/bitvector.go:104（Or）、:57（ensure）、:41（Clone COW）。
- 机制：COW 后 Or 对 shared 目标先 detach 复制整个 []uint64 再逐 word 合并；anchor-bits 合并热路径（mergeAnchorBits/applyScopedAnchorBits）每次合并触发复制。O1 原地 Or 因 alias 语义风险已回退（d0d2424b7），alias 回归测试已纳入。
- 预期收益：可靠所有权方案（SetAnchorBitVector 全路径 ShareWords + GetFields 直接 alias 修复 + 引用计数）后 Or 分配可降约 30GB；GC CPU 同步下降，扫描 wall 预估降 5-10%。
- 风险：高（O1 曾因 alias 回退）；必须保留 o1_alias_red_test.go。
- 最小验证：单开分支，先跑 RED alias 测试；DB-only hadoop 对比 alloc/GC/扫描 wall；Risk=8764 不变。

### P0-2 GORM 批量写链 native INSERT（flush 段热点）

- 证据：flush 段（13:02→13:07）新增 alloc 71.5GB：prepareBatchCreateRow cum +22.1GB、Scope.Fields cum +10.8GB、AddToVars +9.9GB、sqlite exec +10.5GB、driverArgsConnLocked +5.8GB。编译末期 60s CPU 中 GormTransaction cum 18.9%、CreateInBatches 16.7%、database/sql.withLock 15.1%。对象数：prepareBatchCreateRow cum 25.7 亿、reflect.unsafe_New 13.9 亿、commonDialect.Quote 6.0 亿。
- 代码位置：common/utils/dbcache/ async saver → common/yak/ssa SaveIrIndexBatch（GORM CreateInBatches）；gorm pin（df1fe835b）已把 search.clone 从 59GB 降到 5.1GB。
- 机制：GORM 批量 INSERT 每次经 reflect 构造参数、quote 每列；SQLite 写事务与 lock 开销大。
- 预期收益：写路径 native SQL（预编译 INSERT + 直接构造 args）可显著降对象数与事务 CPU，flush wall（294s）有望减 20-40%。
- 风险：中（事务边界、软删除语义、迁移兼容）。
- 最小验证：SaveIRIndexBatch native INSERT benchmark；small/medium 回归；hadoop flush 段 alloc/CPU 对比。

### P0-3 SQLite/native SQL 读链优化（扫描段约 10-16GB）

- 证据：纯扫描段（13:12→13:17）新增 alloc：Rows.nextLocked cum 10.3GB、nativeGetIrCodesByIds cum 6.3GB、GetExs cum 4.9GB、GoStringN/GoString 约 3GB；live heap 中 nativeGetIrCodesByIds 1.47GB + nativeGetIrCodeItemById 0.73GB。扫描 CPU 中 Value.getInstruction cum 4.6%（340s）。
- 代码位置：common/yak/ssa/ssadb/native_read.go:141（IN chunk=1000，每行 Scan 59 列）。
- 机制：getValue/getInstruction 逐值触发 DB 读，每行构造 IrCode + 大量字符串列；chunk 与列投影未裁剪。
- 预期收益：列投影裁剪 + chunk 调大 + 行对象池可降 5-10GB alloc 与 1-3% CPU。
- 风险：中（需保持 code_id 排序与软删除语义）。
- 最小验证：nativeGetIrCodesByIds benchmark（chunk/列集）；small 回归。

### P1-4 ANTLR 编译期对象（编译期最大 alloc 族，约 320GB）

- 证据：NewATNConfig 142.1GB（19.2%，15.7 亿对象）、NewJPCMap2 98.4GB、JStore.Put 31.9GB、JMap.Put 25.5GB、NewBaseSingletonPredictionContext 23.5GB；与 main run19 对照（142.1/97.9GB）几乎一致，非回归。
- 代码位置：ANTLR v4 运行时；现有 YAK_ANTLR_CACHE_RESET_FILES=1 / BYTES=2MiB（ssa_compile_utils.go）复用窗口极小。
- 机制：编译期每文件解析重建 ATN 状态；cache reset 阈值过小。
- 预期收益：放宽 reset 阈值（如 25 文件/8MiB）可在编译 alloc 省 10-30%；live heap 15.7GB，GOMEMLIMIT 24.97GB 有空间。
- 风险：低-中（峰值内存上升；编译正确性不受影响）。
- 最小验证：仅设环境变量重跑 small/medium，对比 alloc/峰值 RSS（零代码）。

### P1-5 异步 persist/flush 等待（final barrier +34s）

- 证据：3p4 final barrier enter 13:08:00→done 13:11:40（4m53.7s）；main run19 23:30:45→23:33:55（4m20.0s，+33.7s）。两边 request=638、enqueued≈512 万、remaining_dirty≈180 万几乎相同；ssa-instruction-save cost 总和相同（142.7s vs 142.5s）——差异全部在等待/backpressure，不在保存吞吐。
- 代码位置：common/utils/dbcache/save.go:330-360（Flush: wg.Wait + saveWG.Wait）、databasex.go:322（Cache.Flush）、:107（Barrier/AsyncDrainAndShrink）。
- 机制：#4 结算修复（f2ca24127/e0f62eb6d/fcad1921a）保证 barrier 前 flush 所有 pending（正确性提升），代价是更严格同步等待；3p4 goroutine 更宽（809 vs 381）但写消费者未增加。
- 预期收益：flush 293.7s→260s 可省约 34s（总 wall 1.3%）；与 P0-2 合并收益更大。
- 风险：中高（结算语义是 #4 修复核心，勿重引入 hang/错误掩盖）。
- 最小验证：dbcache deterministic 测试（Barrier/Flush/failure）+ -race 全包；再做并发消费者/分片 saver benchmark。

### P1-6 TakeSymbolSnapshot omap version（已 -78%，剩余约 9.3GB）

- 证据：纯扫描段 +7.0GB、收尾 +2.3GB；相对 main 42.3GB 已降 78%（O3 生效）。
- 代码位置：common/syntaxflow/sfvm/symbol_snapshot.go:45。
- 机制：named 符号表每次仍新建 keys+dedupKeys 两个 map；空/纯 magic 表已共享 emptySymbolSnapshot。
- 预期收益：omap mutation/version 复用快照可再降约 5GB。
- 风险：高（跨 path 语义）。
- 最小验证：BenchmarkTakeSymbolSnapshot + small 语义回归。

### P1-7 valuePool（已评估拒绝，不重复投入）

- 证据：21.6GB（纯扫描段 +15.7GB、收尾 +5.9GB）；live 1.02GB。
- 代码位置：common/yak/ssaapi/values.go:32-60。
- 机制：sync.Pool.New（pprof 中 ssaapi.init.func23）——Value 多逃逸、releaseValue 只回收工厂壳，pool 冷；warmup 扩大 retained heap 不降 alloc。

### P2-8 GC 扫描 CPU（编译 48-54% cum、扫描 56-58% cum）

- 不是独立热点，是上述 alloc 的果（spanClass/scanObjectsSmall/scanSpan/gcDrain）。
- GOGC=100、GOMEMLIMIT=24.97GB 已按大项目配置；不建议单独调参，应通过 P0-1/P0-2/P0-3 降 alloc。

### P2-9 sync.Once.doSlow（扫描 CPU cum 954s，12.9%，待定位）

- 与 Value.getValue（897s）/getBottomUses（864s）/Values.Recursive（1348s）链重叠，但 getValue/getBottomUses 源码无 sync.Once。
- 下一步：go tool pprof -list 定位 doSlow 调用者（候选：规则/scanPolicy 全局 Once、instruction 懒加载）。

### P2-10 goroutine 结构（381→809-1002）

- async persist + scan concurrency 更宽，调度/GC 压力上升；风险高、收益不确定，保留 profile 作证据，暂不列为优化项。

## 3. 推荐执行顺序

1. P1-4：ANTLR cache reset 环境变量验证（零代码，先确认收益量级）。
2. P0-2：GORM 写链 native INSERT（收益大、风险中）。
3. P0-1：BitVector 可靠所有权（收益最大、风险高，RED 先行，单开分支）。
4. P0-3：native 读列投影/chunk 调参。
5. P1-5：flush 等待结构实验（依赖 P0-2 结果）。
6. P2-9：doSlow 定位（纯调查）。

## 4. 验收口径

- DB-only Hadoop：Risk=8764、扫描窗口、累计 alloc、GC CPU cum、RSS 峰值；与 main debug-scan2（679s / 313.3GB）及 3p4 DB-only（355s / 188.1GB）对比。
- 完整 compile+scan：编译窗口、flush wall、扫描窗口、alloc、live heap、goroutine；与 run19 同口径对比。
- 每个优化独立 commit；gofmt + diff --check + 相关包测试（scripts/ssa-test.sh）；不 push。

---

# 附录 A：2026-08-14 main vs branch 实测补充（compare-main-3p4-20260814）

## A.1 运行背景与条件

- main binary：main worktree f0dd37ee5（SHA 79358da9…；落后 origin/main 10 commits）。
- branch binary：test/scan/large_projects 28a7b3c53（源码=enhance/ssa/scan_perf2 1409fc552；SHA 8ef5d41a…）。
- 命令两侧完全一致：code-scan -t /home/wlz/Target/apache/hadoop -l java --db <fresh>/ssa.db --debug <run>/debug --format sarif --log-level info --rule-timeout 4h --file-perf-log --rule-perf-log；fresh YAKIT_HOME；RSS 每秒采样；全程 Debug+pprof（127.0.0.1:18080）。
- main 于 12:17:30 达到 1 小时上限被终止（未进入 scan，无 Risk）；branch 完整跑完（wall 2792s，Risk=9042，SARIF=9042）。

## A.2 时间线对比

| 阶段 | main | branch | 差值 |
|---|---:|---:|---:|
| compile stage→batch121 | 19m45s | 21m15s | main 快 1m30s |
| batch121→f4_finish（f2/f3） | 25m54s | 7m01s | branch 快 18m53s |
| 编译窗口合计 | 46m16s | 28m54s | branch 快 17m22s（-37.5%） |
| flush（final barrier） | 12:03:51 开始被终止 | 4m48s | — |
| 总 wall | 终止于 3600s | 2792s | — |

- 关键差异：main 编译产物 instruction 总数 20,183,803 vs branch 6,170,341（-69.4%，约 3.3 倍），直接解释 f2/f3 与 flush 的差距。
- 编译期累计 alloc（f2/f3 同期快照 12:02 vs 12:43）：main 1,178GB vs branch 746GB（-36.7%）。

## A.3 新 pprof 数值（branch run 20260814-121735）

### A.3.1 扫描期新增 alloc（125324→125827，约 5m）

总新增 90.3GB：

| 热点 | 新增 alloc | 说明 |
|---|---:|---|
| ssaapi.init.func23（valuePool） | 15.1GB | 同旧 run 结论，pool 冷 |
| BitVector.Or | 10.4GB | COW detach |
| TakeSymbolSnapshot.func1 | 7.6GB | named 表每轮建 map |
| gorm.DB.clone + search.clone | 10.5GB（6.8+3.7GB） | 扫描期 DB 读仍走 GORM clone |
| nativeGetIrCodesByIds | cum 5.7GB | native 读链 |
| SafeMapWithKey.Set | 2.7GB | Value EffectOn/DependOn |
| BitVector.ensure | 4.4GB | 扩容复制 |

### A.3.2 扫描收尾新增 alloc（125827→final，约 4.5m）

总新增 32.9GB：BitVector.Or **20.1GB**、valuePool 5.4GB、TakeSymbolSnapshot 1.7GB、GetExs cum 2.4GB、BitVector.ensure 1.1GB。

### A.3.3 live heap（final，15.6GB）

- nativeGetIrCodesByIds 1.03GB flat / 1.34GB cum、nativeGetIrCodeItemById 0.56/0.72GB → DB 读链约 2.1GB 常驻。
- valuePool（init.func23）0.96GB、SafeMapWithKey.Set 0.93GB、NewValue 0.86GB、codec.AnyToBytes 0.72GB、memedit.NewRuneOffsetMap 0.62GB。
- NewValue/NewInstruction/omap/NewVersioned 合计约 2GB（Value 图结构常驻）。

### A.3.4 alloc_objects（累计 106.7 亿对象）

- ANTLR NewATNConfig 15.7 亿（14.8%）、reflect.unsafe_New 13.8 亿（13.0%）、gorm.commonDialect.Quote 5.9 亿（5.6%）、prepareBatchCreateRow cum 25.5 亿（23.9%）、sqlite bind 3.5 亿、Scope.Fields cum 5.5 亿、getModelStruct 2.1 亿。
- 对象数与 alloc 字节基本同步；GORM 反射/quote 是对象数第一可优化来源。

### A.3.5 flush 段 CPU（124822，300s，836% 并行）

- GC 主导：spanClass 26.9%、scanObjectsSmall cum 27.8%、tryDeferToSpanScan cum 35.3%、scanSpan cum 52.9%。
- cgocall flat 14.5%；业务热点：Value.getInstruction cum 3.2%（81s）。
- flush 段 alloc：prepareBatchCreateRow cum 132.9GB、AddToVars 37.2GB、Fields cum 44.3GB、sqlite exec/bind 链约 40GB。

### A.3.6 扫描段 CPU（125324，300s，2457% 并行）

- 与旧 3p4 run 同构：spanClass 26.9%、scanObjectsSmall cum 27.8%、cgocall 9.0%、scanSpan cum 56.4%、gcDrain cum 58.4%。
- 业务热点：QuerySyntaxflow cum 20.5%、SFFrame.exec cum 19.8%、Values.Recursive cum 18.3%、Value.getValue cum 12.2%、getBottomUses cum 11.7%、Value.getInstruction cum 4.6%（340s）、sync.Once.doSlow cum 12.9%（954s，仍未定位调用者）。
- goroutine 峰值 1002（扫描 125827 时刻 814）。

## A.4 新增/更新结论

- BitVector.Or 在扫描收尾段仍是单点最大 alloc（20.1GB），且与旧 run 的 30.1GB 同源 → P0-1 优先级不变，收益确认。
- GORM 写链（prepareBatchCreateRow/Fields/AddToVars/quote）仍是 flush 段对象数与 alloc 第一来源 → P0-2 优先级不变。
- native 读链在 live heap 中常驻约 2.1GB，且扫描期每轮 Rows.nextLocked/GetExs 持续分配 → P0-3 增加“行对象复用/列投影”证据。
- valuePool 与 TakeSymbolSnapshot 的扫描期新增 alloc 均再次复现（15.1/7.6GB），维持原结论（valuePool 已拒绝、TakeSymbolSnapshot 归 C 档）。
- main 实测证明：未合并分支优化时 f2/f3 为 25m54s 且 instruction 总量 3.3 倍；后续优化验收可直接用 instruction 总数作为编译期持久化量的代理指标。

---

# 附录 B：早期 3.1 后续优化任务清单（归档自 .codex-next-optimizations.md）

> 以下任务来自 3.1 阶段的 `.codex-next-optimizations.md`，为历史来源；状态已在当前计划中更新。

- O1（BitVector.Or/ensure 分配，~26.1GB + 5.8GB）：已做 COW 与 O1 原地 Or 实验，因 alias 风险回退（d0d2424b7），作为 P0-1 遗留：需可靠所有权方案 + RED alias 测试 + AllocsPerRun 基准。
- O2（批量 IR 读取去 GORM）：已完成——D（单行 native）、O2（PreloadIrCodesByIdsFast native）、A2/A3（yieldIrCodes/SearchVariable native）；对应当前 P0-3 读链剩余（列投影/chunk/行对象池）。
- O3（TakeSymbolSnapshot 同 check 复用，~26.5GB）：已实现懒构建+magic-only 短路（O3，8c06e0951），当前扫描期仍约 7.6GB；per-check 复用归入 P1-6（omap version，C 档）。
- O4（Value pool 容量/预热）：已评估拒绝（pool 冷；warmup 扩 retained heap 不降 alloc），当前扫描期仍 15.1GB，维持拒绝，仅在 P0 项完成后重审。
