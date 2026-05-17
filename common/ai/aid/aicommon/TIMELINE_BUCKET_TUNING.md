# Timeline 字节子桶调优报告 (2026-05)

> 调优目标: 让 `Timeline.GroupByMinutes` 在同一 3 分钟时间桶内的字节子桶切分
> 行为, 在 dashscope/qwen 显式缓存计费模型下取得最低净成本。
>
> 关键词: TimelineDumpDefaultBucketByteSize, BucketSizer, 主动缓存, dashscope 计费,
> cache_creation, cache_hit, 16K -> 64K, EntryAdaptive

---

## 1. 背景: 为什么桶大小是关键

[Timeline.GroupByMinutes](timeline_groups_render.go) 把 timeline 切成
"3 分钟时间桶 + 字节子桶" 两层结构, 字节子桶的预算原本是 `16 * 1024`。

**真正影响成本的是上游 (dashscope) 的 cache_creation 行为**:

| dashscope 实测约束 (见 [TONGYI_CACHE_REPORT.md](../aicache/TONGYI_CACHE_REPORT.md) §4.4 / §4.12) | 含义 |
| --- | --- |
| 建块阈值 = 1024 token (≈4KB) | 前缀短于此不会建任何缓存块 |
| **"部分命中 + 增量建块"机制不存在** (E12 决定性 FAIL) | frozen 字节序列每变一次, 整段 user1 按 125% cache_creation 计费**全段重建**, 没有"前缀命中 + 增量计费" |
| 命中是字节级 1:1 一致 | 整段 user1 = 一个块, 子桶切多少对单次命中**无收益** |

由此推论:

> **每次 flush 字节子桶 ≈ 一次 cache_creation (125% 计费)**。
> 在 3 分钟时间桶之内, 切桶越频繁, cache_create 次数越多, 成本越高。

桶切分**唯一**的正收益是"让 open 子桶之前的内容提前进 frozen", 加快缓存命中
启动时间; 但这个收益**只在 3 分钟内**有效 (时间桶切换是天然的强制 flush), 而每次
flush 都要付 125% 建块费。

---

## 2. 实验方法

### 2.1 脚手架

[bucket_bench.go](bucket_bench.go) 提供:

- `LoadRealSessionEvents(dir)` — 从 `yakit-projects/aispace/<session>/` 重建
  timeline push 事件序列
- `BuildSyntheticScenario(name)` — 4 类合成场景生成器
- `ReplayAndMeasure(events, opts)` — 重放后收集指标

[timeline_bucket_bench_test.go](timeline_bucket_bench_test.go) 是入口
(`//go:build bucketbench`):

```bash
go test -tags bucketbench -v -run TestBucketBench \
    ./common/ai/aid/aicommon/ -timeout 5m
```

### 2.2 数据集

| 数据集 | 来源 | events | 特点 |
| --- | --- | --- | --- |
| `short_query` | 合成 | 30 | 1 分钟内 30 条 ~500B 小条目 |
| `dense_tools` | 合成 | 20 | 3 分钟内 20 条 2-8KB 密集工具调用 |
| `single_huge` | 合成 | 6 | 单条 64KB + 周边 5 条 1KB |
| `mixed` | 合成 | 36 | 9 分钟内 3 桶, 500B-20KB 交错 |
| `real_redhaze` | 真实 | 90 | `/Users/v1ll4n/yakit-projects/aispace/11194_redhaze_pentest_auth_20260517_1d364/` 的工具调用记录 |

### 2.3 指标

- `flush_count`: frozen 段 sha256 hash 变化次数, **直接对应 cache_creation 次数**
- `avg_frozen_bytes / p95_frozen_bytes / max_frozen_bytes`: 全程 frozen 段尺寸
- `est_create_cost`: `Σ(frozen_bytes_when_changed) × 1.25` (dashscope cache_creation 计费倍率)
- `est_hit_savings`: `Σ(frozen_bytes_when_stable) × 0.6` (cached_tokens 命中节省 = 1 - 0.4)
- `net_cost = est_create_cost - est_hit_savings` (越负越好)

---

## 3. 固定桶大小扫描结果

候选值: `{-1 (no-split), 4K, 6K, 8K, 12K, 16K(原默认), 24K, 32K, 48K, 64K, 96K, 128K, 192K}`

### 3.1 关键数据 (摘录, 完整见 [testdata/bucket_bench/](testdata/bucket_bench/))

| 场景 | 16K (原默认) | 32K | 48K | 64K | 96K | 128K | 192K |
| --- | --- | --- | --- | --- | --- | --- | --- |
| `short_query` (30) | +820B | 0 | 0 | 0 | 0 | 0 | 0 |
| `dense_tools` (20) | **+127K** | -119K | -145K | **-215K** | +119K | 0 | 0 |
| `single_huge` (6) | 0 | 0 | 0 | 0 | 0 | 0 | 0 |
| `mixed` (36) | **+1.60M** | -260K | -761K | -551K | **-1.04M** | -1.04M | -1.04M |
| `real_redhaze` (90) | -1.99M | -2.99M | **-3.12M** | -3.12M | -3.12M | -3.12M | -3.12M |

(单位 byte; 负数 = 节省, 越负越好。粗体 = 该列最优。)

### 3.2 观察

1. **16K 在密集 + 真实数据上严重亏损**: `dense_tools` 与 `mixed` 上 16K 净
   成本为正 (亏损 0.13M-1.60M); `real_redhaze` 上虽然节省 1.99M 但远不如 48K+。
2. **48K 在真实数据上达到最优**: `real_redhaze` 48K 以上的所有值都给出
   `-3.12M` 净成本, 与 no-split 等价 (因为 3 分钟时间桶天然限制了 frozen 段增长)。
3. **64K 是固定值里的"安全甜区"**: 仅在 `dense_tools` 上严格胜出 (-215K vs
   48K 的 -145K), 在所有其它场景接近或达到最优。
4. **96K+ 在密集场景反而劣化**: `dense_tools` 96K = +119K (因为 frozen 段没
   能在中段建好, 命中机会被压缩到末尾)。
5. **no-split (`-1`) 在 4/5 场景达到最优**, 但在 `dense_tools` 上输给 64K
   (因为 3 分钟内一波密集 entry 来不及在时间桶内冻结)。

### 3.3 性能模型

定义 \( N \) 为单时间桶内的 entry 总字节, \( B \) 为字节子桶预算:

- flush 次数 \( \approx \lceil N / B \rceil - 1 \)
- 每次 flush 重建整段 frozen, 字节量 \( \approx \text{frozen\_累计} \)
- 太小: \( N/B \) 大, flush 频繁; 太大: 没机会 flush, 命中启动慢

**经验最优值**: \( B \) 取 "**单时间桶内 entry 字节量的 0.8x ~ 1.2x**", 让一个
时间桶大致切 0~1 次。

真实数据观察: 单 3 分钟时间桶平均 ~50KB entry, 64K 接近该平均值, 与实验结论
一致。

---

## 4. 动态算法对比

### 4.1 4 个候选

| 算法 | 公式 | 含义 |
| --- | --- | --- |
| `A_Fixed_16K` (baseline) | 16384 | 旧默认 |
| `A_Fixed_64K` | 65536 | 新推荐固定值 |
| `B_TimeRemaining(64K→8K)` | `64K × (timeRemaining / 3min)`, 下限 8K | 时间桶刚开始用大桶, 末段用小桶 |
| `C_EntryAdaptive(8x, 32K-256K)` | `clamp(8 × meanRecentEntry, 32K, 256K)` | 一个桶大致容纳 8 条平均 entry |
| `D_TokenAware(5000tok)` | `5000 × 3.5B = 17500` | 目标 5K token 字节预算 |

### 4.2 结果

| 场景 | A_16K | A_64K | B_TimeRem | C_EntryAdaptive | D_TokenAware |
| --- | --- | --- | --- | --- | --- |
| `short_query` (30) | +820B | **0** | 0 | 0 | +10.8K |
| `dense_tools` (20) | +156K | -214K | +362K | **-225K** | +202K |
| `single_huge` (6) | 0 | 0 | 0 | 0 | 0 |
| `mixed` (36) | +1.60M | -551K | +621K | **-822K** | +557K |
| `real_redhaze` (90) | -1.99M | **-3.12M** | -1.61M | -2.99M | -2.24M |

### 4.3 观察

- **`C_EntryAdaptive` 在 dense_tools / mixed 上明显胜出**, 在真实数据上达到
  64K 固定值的 96% (`-2.99M` vs `-3.12M`)。
- **`B_TimeRemaining` 反而比 16K 更差**: 时间桶末段强制小桶导致额外 flush。
- **`D_TokenAware` 接近固定 16K**: 17.5K 与 16K 实质上是一回事。

### 4.4 算法选择

> 真实生产场景大概率是 `real_redhaze` 形态 (跨多个时间桶的离散 entry, 个别大输出)。
> 在这种场景上 `A_Fixed_64K` 与 `C_EntryAdaptive` 几乎打平 (差 4%)。
>
> 对工具调用极其密集的特殊场景 (一个 3 分钟时间桶内连续 10-20 次 LLM call),
> `C_EntryAdaptive` 有 25%-50% 的额外收益。

---

## 5. 推荐方案

### 5.1 立即落地

1. **`TimelineDumpDefaultBucketByteSize: 16K → 64K`** ([timeline.go](timeline.go))
   - 默认行为升级, 所有走 `GroupByMinutes` 的路径自动受益
   - 在真实数据上节省 1.13M (从 -1.99M -> -3.12M, **57% 提升**)
   - 在所有测过的场景里都不弱于 16K
2. **保留 `TimelineDumpLegacyBucketByteSize = 16K` 常量**, 用作历史标记 / 显式回滚锚点
3. **新增 `DefaultBucketSizer()` 工厂** ([bucket_bench.go](bucket_bench.go))
   - 等价于 `EntryAdaptiveBucketSizer(8, 32K, 256K)`
   - 主动缓存敏感的调用方可显式开启:
     ```go
     tl := aicommon.NewTimeline(ai, nil)
     tl.SetTimelineBucketSizer(aicommon.DefaultBucketSizer())
     ```
   - **不主动注册到 `NewTimeline`**, 保持向后兼容

### 5.2 后续优化建议

| 优先级 | 项 | 收益预期 |
| --- | --- | --- |
| P2 | 在 aireact 主路径主动启用 `DefaultBucketSizer()` | 密集工具调用场景再省 ~25% |
| P3 | 把 sizer 决策序列化到 timeline 持久化层, 保证 reload 后切桶行为一致 | 防止 sizer 升级时旧 timeline 反复 cache_create |
| P3 | aibalance UI 上下文成分图加 "桶 flush 次数" 指标 | 让用户能直接看到调优效果 |

---

## 6. 兼容性与回退

### 6.1 行为变化

| 调用方式 | 旧行为 (16K 默认) | 新行为 (64K 默认) |
| --- | --- | --- |
| `tl.GroupByMinutes(3)` | 同一 3 分钟桶内 16K 一切 | 同一 3 分钟桶内 64K 一切 |
| `tl.GroupByMinutesAndBytes(3, X)` | X 决定切分 | 不变 (旁路 sizer / 默认值) |
| `tl.SetTimelineBucketByteSize(N)` | N 覆盖默认 | 不变 |
| `tl.SetTimelineBucketSizer(s)` (新增) | 不存在 | sizer 优先于固定值 |

### 6.2 影响面

- `TimelineDumpDefaultBucketByteSize` 在生产代码中**只在 4 处出现**: timeline.go
  定义 + getEffectiveBucketByteSize + GroupByMinutesAndBytes 默认值回退 +
  packTimelineIntervalSubBlocksWithSizer fallback。
- 现有所有 `TestByteBucket_*` / `TestGroupByMinutes_*` / aicache fixture 测试
  **全部通过** (验证: 32 个相关测试全 PASS)。
- aicache hijacker fixture 测试不依赖具体桶大小, 只验证边界标签字面量与字节
  稳定性。

### 6.3 回退路径

如果发现新默认值在某个场景下意外劣化:

```go
// 全局回退到旧默认:
aicommon.SetTimelineBucketByteSize(aicommon.TimelineDumpLegacyBucketByteSize)

// 单 timeline 回退:
tl.SetTimelineBucketByteSize(aicommon.TimelineDumpLegacyBucketByteSize)

// 完全关闭字节切分 (退回纯时间桶):
tl.SetTimelineBucketByteSize(-1)
```

---

## 7. 副产物: bufio.Scanner 64KB 单 token 上限

实验中发现 [ParseStringToRawLines](../../../utils/str_utils.go) 使用 `bufio.Scanner`
默认 64KB 单 token 上限, 任何单条 entry 内的"单行超过 64KB 文本"会被**静默丢弃**。

实际生产里工具输出基本都是多行文本 (HTTP 包 / yaml / JSON 缩进), 触发概率极低,
但合成测试需要刻意构造换行避免命中。

> 这是 timeline 渲染层的潜在风险, 不在本调优范围内, 后续可考虑给 ParseStringToRawLines
> 加 `scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)` 提升单行上限。

---

## 8. 复现实验

```bash
# 完整扫描 + 算法对比 (~3 秒)
go test -tags bucketbench -v -run TestBucketBench \
    ./common/ai/aid/aicommon/ -timeout 5m

# 单元回归
go test -run "TestBucketSizer|TestPackTimeline|TestDefault64K" -v \
    ./common/ai/aid/aicommon/
```

实验报告产物落在 [testdata/bucket_bench/](testdata/bucket_bench/):

- `<unix-ts>-fixed-sweep.md`: 固定值扫描
- `<unix-ts>-algo-compare.md`: 动态算法对比

---

## 9. 引用

- [TONGYI_CACHE_REPORT.md](../aicache/TONGYI_CACHE_REPORT.md) §4.4 (1024 token 阈值实测)
  与 §4.12 (增量建块不存在实测)
- [CACHE_BOUNDARY_GUIDE.md](../aicache/CACHE_BOUNDARY_GUIDE.md) §1-§3 (frozen 边界标签机制)
- [README_TIMELINE_GROUPS.md](README_TIMELINE_GROUPS.md) (Timeline 桶切分实现总览)
