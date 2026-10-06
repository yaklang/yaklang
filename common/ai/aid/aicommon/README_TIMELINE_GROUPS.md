# Timeline 时间桶分组渲染（GroupByMinutes）

## 1. 这是什么

`GroupByMinutes` 是 `Timeline` 的一个**纯增量**渲染入口，它把 timeline 的两类内容
统一切分成一组**可被 [aitag](aitag/README.md) 包裹的 RenderableBlock**：

| Block 类型                | 来源                                 | 是否冻结        |
| ------------------------- | ------------------------------------ | --------------- |
| `TimelineIntervalBlock`   | 活跃条目先按 N 分钟绝对时间桶分组，再在同一日历桶内按字节预算（默认 64KB）切「子桶」 | 整条 timeline 仅最末 interval 子桶 Open，其余 Frozen |
| `TimelineCompressedHeadBlock` | `Timeline.compressedHead`（唯一有效压缩段） | 永远 Frozen     |

调用方通过 `groups.GetAllRenderable().Render(tagName)` 一次拿到 compressed-head + interval
混合的 aitag 拼接串，前缀部分（compressed-head + 已冻结桶）可被 LLM provider 的 prompt
cache 直接命中。

```go
groups := timeline.GroupByMinutes(3)              // N=3 分钟为一桶
prompt := groups.GetAllRenderable().Render("TL")  // compressed head 在前 + interval 在后
```

> 历史背景：原本 `Timeline.summary` 字段曾被设计为"单条 shrink 结果记录表"，
> 但代码搜索（详见 `timeline.go`）确认它**没有任何写入路径**，是 dead code。
> 在本次改动中已经删除该字段及其全部读写路径，仅在反序列化层保留对老数据
> JSON 中 `summary` 字段的**静默忽略**，保证向后兼容。

---

## 2. 为什么需要这个

LLM 调用按 token 计费，KV cache 也按 token 命中。把 timeline 渲染成
**字节级稳定的前缀**可以同时降低成本与延迟：

- **绝对时间对齐**：`(minute / N) * N` 把同一个时间点永远落到同一个桶里，
  桶的边界与 timeline 的"插入顺序"无关。
- **稳定 nonce**：每个 block 的 aitag nonce 都由"内容来源"决定，
  - interval block: `b{N}t{unixSec}`；若同一时间日历桶内因字节预算切出多个子桶，则为 `b{N}t{unixSec}s{seq}`（seq 从 0 递增，仍满足 hijacker 对 `b` 前缀的解析）
  - compressed head block: `h{coveredEndSec}v{version}`
  
  同一子桶/同一 reducer 在不同次调用中产生**完全相同的 nonce**。子桶切分仅依赖条目时间与渲染字节估计，**幂等**。
- **字节子桶**：`Timeline.SetTimelineBucketByteSize` / `GroupByMinutesAndBytes` 控制；默认 `TimelineDumpDefaultBucketByteSize`（**64KB**，2026-05 由 16KB 上调，见 [TIMELINE_BUCKET_TUNING.md](TIMELINE_BUCKET_TUNING.md)）。设为负数可关闭字节切分（仅时间桶，nonce 与旧版完全一致）。单条 item 超过预算时独占一个子桶，不在 entry 中间切开。
- **动态桶大小**：`Timeline.SetTimelineBucketSizer(BucketSizer)` 启用动态算法。`DefaultBucketSizer()` 返回项目推荐的 `EntryAdaptiveBucketSizer(8, 32K, 256K)`：让一个桶大致容纳 8 条最近 entry 的平均字节，自动适配小 / 大 entry。sizer 非 nil 时优先于固定 byteSize；`GroupByMinutesAndBytes(N, X)` 显式调用始终走固定值（向后兼容）。
- **status 不入标签**：`Open / Frozen` 状态只暴露在 `block.IsOpen()` 上，
  **不写入** 标签或内容字符串，避免一个桶在被冻结时改变其字节流。
- **bucket 元信息写在内容首行**：`# bucket=YYYY/MM/DD HH:MM:SS-HH:MM:SS interval=Nm`
  或 `# compressed_head covered_end_item_id=<id> covered_end_at_ms=<ms> version=<v>`，这一行对同一个 block 恒定，
  依然是缓存友好的稳定前缀。

净效果：**只要前面的 block 不再变，它的渲染输出永远字节级不变**，
LLM provider 的前缀缓存（Anthropic prompt caching、OpenAI prefix caching 等）
就可以一直命中前 N-1 个 block，只有最后一个 open 桶需要重新计费。

---

## 3. API 速查

### 主入口

```go
func (m *Timeline) GroupByMinutes(minutes int) *TimelineGroups
// 等价于 GroupByMinutesAndBytes(minutes, getEffectiveBucketByteSize())，默认叠 64KB 子桶预算。
// 若设置了 BucketSizer (SetTimelineBucketSizer), 走动态算法路径。

func (m *Timeline) GroupByMinutesAndBytes(minutes int, bytesPerBucket int64) *TimelineGroups
// bytesPerBucket < 0：关闭字节子桶；== 0：使用 TimelineDumpDefaultBucketByteSize（64KB）；> 0：使用该字节上限。
// 显式调用此函数不被 BucketSizer 拦截，保持向后兼容。

func (m *Timeline) SetTimelineBucketByteSize(n int64)
// n>0 自定义预算；n==0 恢复默认；n<0 关闭字节子桶（仅时间桶）。

func (m *Timeline) SetTimelineBucketSizer(sizer BucketSizer)
// 设置动态桶大小决策器。nil 表示禁用动态算法。
// 推荐: tl.SetTimelineBucketSizer(aicommon.DefaultBucketSizer())
//       (EntryAdaptive 8x, 32K-256K)
```

### 容器

```go
type TimelineGroups struct{ /* private */ }

func (g *TimelineGroups) IntervalMinutes() int
func (g *TimelineGroups) GetBlocks() TimelineIntervalBlocks       // 仅 interval block
func (g *TimelineGroups) GetAllRenderable() TimelineRenderableBlocks // compressed head 在前 + interval 在后
```

### Renderable 抽象

```go
type TimelineRenderableBlock interface {
    Render() string
    StableNonce() string
    IsOpen() bool
}

type TimelineRenderableBlocks []TimelineRenderableBlock

// 裸渲染: 把所有 block 顺序拼接, 每个 block 用 aitagName_<nonce> 包裹.
func (bs TimelineRenderableBlocks) Render(aitagName string) string

// 带 frozen 边界渲染: 在裸渲染基础上, 把所有 IsOpen()==false 的连续前缀 block
// 包在一对 <|frozenTagName_frozenNonce|>...<|frozenTagName_END_frozenNonce|>
// 边界标签内, 让下游 aicache hijacker 能用简单字符串 IndexOf 精准切到
// frozen 与 open 的边界 (§7.7.7 双 cc 命中前提). 全 frozen / 全 open 时
// 不加边界, 与裸 Render 字节一致.
func (bs TimelineRenderableBlocks) RenderWithFrozenBoundary(
    aitagName, frozenTagName, frozenNonce string,
) string

// 包级默认: aicache hijacker 与 Timeline.Dump 一致使用此默认值.
const (
    TimelineFrozenBoundaryTagName = "AI_CACHE_FROZEN"
    TimelineFrozenBoundaryNonce   = "semi-dynamic"
)
```

### 单个 block

```go
type TimelineIntervalBlock struct {
    BucketStart     time.Time   // 桶起点（已对齐到 N 分钟边界）
    BucketEnd       time.Time   // 桶终点（exclusive）
    IntervalMinutes int
    Items           []*TimelineItem
    Open            bool        // 仅整条 timeline 最末 interval 子桶为 true
    SeqInBucket     int         // 同一时间日历桶内子桶序号（从 0 起）
    TotalInBucket   int         // 该日历桶内子桶总数；为 1 时 nonce 不带 s 后缀
}
func (b *TimelineIntervalBlock) Render() string
func (b *TimelineIntervalBlock) StableNonce() string  // "b{N}t{unixSec}" 或 "b{N}t{unixSec}s{seq}"
func (b *TimelineIntervalBlock) StableKey() string    // 16 字符 sha256 摘要
func (b *TimelineIntervalBlock) IsOpen() bool

type TimelineCompressedHeadBlock struct {
    CoveredEndItemID int64
    CoveredEndAtMs   int64
    Version          int64
    Text             string
}
func (h *TimelineCompressedHeadBlock) Render() string
func (h *TimelineCompressedHeadBlock) StableNonce() string  // "h{coveredEndSec}v{version}"
func (h *TimelineCompressedHeadBlock) IsOpen() bool         // 恒为 false
```

---

## 4. 输出格式

### Interval block 内容（不含 aitag 包裹）

```
# bucket=2026/05/02 10:00:00-10:03:00 interval=3m
10:00:30 [tool/scan]
result-line-1
result-line-2
10:01:45 [user/review]
user-answer
```

### Compressed head 内容（不含 aitag 包裹）

```
# compressed_head covered_end_item_id=42 covered_end_at_ms=1746180000000 version=1
[compressed/head]
batch-compress summary line 1
batch-compress summary line 2
```

### 整体 Render 输出（`groups.GetAllRenderable().Render("TL")` 裸渲染）

```
<|TL_h1746179400v1|>
# compressed_head covered_end_item_id=42 covered_end_at_ms=1746179400000 version=1
[compressed/head]
compressed-batch-summary
<|TL_END_h1746179400v1|>
<|TL_b3t1746180000|>
# bucket=2026/05/02 10:00:00-10:03:00 interval=3m
10:00:30 [tool/scan]
result-line-1
10:01:45 [user/review]
user-answer
<|TL_END_b3t1746180000|>
<|TL_b3t1746180180|>
# bucket=2026/05/02 10:03:00-10:06:00 interval=3m
10:04:00 [text/note]
noted-content
<|TL_END_b3t1746180180|>
```

### 整体 Render 输出（`groups.GetAllRenderable().RenderWithFrozenBoundary("TL", "", "")` 带 frozen 边界）

```
<|AI_CACHE_FROZEN_semi-dynamic|>
<|TL_h1746179400v1|>
# compressed_head covered_end_item_id=42 covered_end_at_ms=1746179400000 version=1
[compressed/head]
compressed-batch-summary
<|TL_END_h1746179400v1|>
<|TL_b3t1746180000|>
# bucket=2026/05/02 10:00:00-10:03:00 interval=3m
10:00:30 [tool/scan]
result-line-1
10:01:45 [user/review]
user-answer
<|TL_END_b3t1746180000|>
<|AI_CACHE_FROZEN_END_semi-dynamic|>
<|TL_b3t1746180180|>
# bucket=2026/05/02 10:03:00-10:06:00 interval=3m
10:04:00 [text/note]
noted-content
<|TL_END_b3t1746180180|>
```

> compressed head + 第一个时间桶被框入 `<|AI_CACHE_FROZEN_semi-dynamic|>...<|AI_CACHE_FROZEN_END_semi-dynamic|>`,
> 因为它们 `IsOpen()==false` (frozen 段); 末尾时间桶 `b3t1746180180` 是 `Open=true`,
> 留在边界外。下游 aicache hijacker 会按这对边界把 prompt 切成 `[high-static, frozen-prefix, open-tail]`
> 三段, 给 system + frozen-prefix 各自打一个 `cache_control:{"type":"ephemeral"}`,
> 实现 §7.7.7 双 cc 命中。

> 退化场景:
>   - 全 open (单时间桶 + 无 reducer): 不输出 boundary, 与裸 Render 字节一致
>   - 全 frozen (无 interval, 仅 reducer): 不输出 boundary (整段 frozen 不需要切)
>   - 这两种场景下 hijacker 走 2 段拼接, 由 aibalance 的"baseline 单 cc"路径兜底

> 注意：content 行**不加任何前导缩进**。LLM 凭 `HH:MM:SS [type/verbose]` 行头模式识别 entry 边界，
> 缩进只是 token 浪费。

每个 block 都符合 [aitag](aitag/README.md) 的 `<|TAGNAME_NONCE|>...<|TAGNAME_END_NONCE|>`
规范，因此可以直接喂给 `aitag.Parse` / `aitag.SplitViaTAG`：

```go
result, _ := aitag.SplitViaTAG(prompt, "TL")
for _, blk := range result.GetTaggedBlocks() {
    fmt.Println(blk.Nonce, blk.Content)
}
```

---

## 5. 压缩、冻结与记忆

渲染不会调用 AI。执行前的阈值检查由 `CompressBeforePrompt` 负责；显式压缩入口是 `CompressOnce`。压缩快照包含已有摘要、投影后的 Frozen/Open 条目、用户信息、Evidence 和本会话已有记忆候选。

当前没有“保留最新 1/4”的固定比例。模型输出摘要和原文 ID 范围，事务校验来源指纹后一起提交摘要、保留条目与冻结状态；请求期间新增的条目仍属于 Open 段。来源被修改、解析失败或提交失败时，不丢弃原始 Timeline。

压缩产生的记忆候选通过独立会话通知交付保存；摘要发布不等于记忆已经保存成功。完整用户任务的正常收尾由记忆层处理，渲染、普通 freeze、审核等待和阶段切换不自行触发抽取。

## 6. 缓存边界

- interval nonce 来自时间桶和子桶序号；冻结后的内容不变时，渲染字节保持稳定。
- compressed head nonce 为 `h{coveredEndSec}v{version}`；同一摘要稳定，新一轮压缩生成新版本。
- `Render` 仅拼接块；`RenderWithFrozenBoundary` 另外标注 Frozen/Open 边界。
- `Dump`/`String` 使用默认分组及冻结边界；`DumpForPrompt` 在同一桶布局上应用提示词投影，移除 bookkeeping 噪声。
- `DumpBefore(beforeId)` 只渲染对应 ID 上界的子时间线，不能当作完整 `Dump` 的别名。
- 字节稳定是缓存命中的必要条件，实际命中还取决于 provider 和完整请求前缀。

## 7. 回归测试

分组和 AITAG：[`timeline_groups_render_test.go`](timeline_groups_render_test.go)、[`timeline_groups_render_aitag_test.go`](timeline_groups_render_aitag_test.go)。

摘要时间戳、旧数据和缓存前缀：[`timeline_compression_render_test.go`](timeline_compression_render_test.go)，包括 `TestDumpCompressedHeadStable` 的正常时间戳与缺失时间戳两个案例。

事务、来源冲突与原文保留：[`timeline_compression_transaction_test.go`](timeline_compression_transaction_test.go)、[`timeline_compression_output_test.go`](timeline_compression_output_test.go)。会话记忆和恢复：[`timeline_session_memory_test.go`](timeline_session_memory_test.go)、[`timeline_memory_lifecycle_test.go`](timeline_memory_lifecycle_test.go)。
