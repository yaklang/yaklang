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
