# Timeline 时间桶分组渲染（GroupByMinutes）

## 1. 这是什么

`GroupByMinutes` 是 `Timeline` 的一个**纯增量**渲染入口，用于把活跃条目按
**绝对时间对齐的 N 分钟时间桶**切分为若干 `TimelineIntervalBlock`，每个 block
独立用 [aitag](aitag/README.md) 兼容标签包裹，最终拼接成一个 LLM 可消费的字符串。

它与 `Dump` / `DumpBefore` / 各种压缩路径完全无交集，只读 timeline 内部状态。

```go
groups := timeline.GroupByMinutes(3)        // N=3 分钟为一桶
blocks := groups.GetBlocks()                // []*TimelineIntervalBlock
prompt := blocks.Render("TIMELINE_INTERVAL_GROUP")
```

---

## 2. 为什么需要这个

LLM 调用按 token 计费，KV cache 也按 token 命中。把 timeline 渲染成
**字节级稳定的前缀** 可以同时降低成本与延迟：

- **绝对时间对齐**：`(minute / N) * N` 把同一个时间点永远落到同一个桶里，
  桶的边界与 timeline 的"插入顺序"无关。
- **稳定 nonce**：每个 block 的 aitag nonce 由 `BucketStart.Unix() + IntervalMinutes`
  推出（`b{N}t{unixSec}`），同一桶在不同次调用中产生**完全相同的 nonce**。
- **status 不入标签**：`Open / Frozen` 状态只暴露在 `block.Open` 字段上，
  **不写入** 标签或内容字符串，避免一个桶在被冻结时改变其字节流。
- **bucket 元信息写在内容首行**：`# bucket=YYYY/MM/DD HH:MM:SS-HH:MM:SS interval=Nm`，
  这一行对同一个桶恒定，依然是缓存友好的稳定前缀。

净效果：**只要前面的桶不再有新条目落入，它的渲染输出永远字节级不变**，
LLM provider 的前缀缓存（Anthropic prompt caching、OpenAI prefix caching 等）
就可以一直命中前 N-1 个 block，只有最后一个"open" 桶需要重新计费。

---

## 3. API 速查

```go
// 主入口
func (m *Timeline) GroupByMinutes(minutes int) *TimelineGroups

// 容器
type TimelineGroups struct {/* ... */}
func (g *TimelineGroups) GetBlocks() TimelineIntervalBlocks
func (g *TimelineGroups) IntervalMinutes() int

// 切片
type TimelineIntervalBlocks []*TimelineIntervalBlock
func (bs TimelineIntervalBlocks) Render(aitagName string) string

// 单个桶
type TimelineIntervalBlock struct {
    BucketStart     time.Time   // 桶起点（已对齐到 N 分钟边界）
    BucketEnd       time.Time   // 桶终点（exclusive）
    IntervalMinutes int
    Items           []*TimelineItem  // 已按 id 升序、已剔除 deleted
    Open            bool             // 仅最末一个有 item 的桶为 true
}

func (b *TimelineIntervalBlock) Render() string       // 仅 body，不含 aitag 包裹
func (b *TimelineIntervalBlock) StableNonce() string  // 用于 aitag 标签的 nonce
func (b *TimelineIntervalBlock) StableKey() string    // 16 字符 sha256 摘要
```

---

## 4. 输出格式

`blocks.Render("TIMELINE_INTERVAL_GROUP")` 输出形如：

```
<|TIMELINE_INTERVAL_GROUP_b3t1746180000|>
# bucket=2026/05/02 10:00:00-10:03:00 interval=3m
10:00:30 [tool/scan ok]
result-line-1
result-line-2
10:01:45 [user/review]
user-answer
<|TIMELINE_INTERVAL_GROUP_END_b3t1746180000|>
<|TIMELINE_INTERVAL_GROUP_b3t1746180180|>
# bucket=2026/05/02 10:03:00-10:06:00 interval=3m
10:04:00 [text/note]
noted content
<|TIMELINE_INTERVAL_GROUP_END_b3t1746180180|>
```

> 注意：content 行**不加任何前导缩进**。LLM 凭 `HH:MM:SS [type/verbose]` 行头模式识别 entry 边界，
> 缩进只是 token 浪费。同样也避免使用空行分隔 entry。

每个 block 的格式都符合 [aitag](aitag/README.md) 的 `<|TAGNAME_NONCE|>...<|TAGNAME_END_NONCE|>`
规范，所以可以直接喂给 `aitag.Parse` 或 `aitag.SplitViaTAG`：

```go
result, _ := aitag.SplitViaTAG(prompt, "TIMELINE_INTERVAL_GROUP")
for _, blk := range result.GetTaggedBlocks() {
    fmt.Println(blk.Nonce, blk.Content)
}
```

---

## 5. 缓存原理

> 假设 timeline 每经过一段时间被 dump 一次给 LLM。

| 时刻 | timeline | render 出的 nonce 序列 |
|---|---|---|
| T0 | bucket A | `[A]` |
| T1 | bucket A, B | `[A, B]` |
| T2 | bucket A, B, C | `[A, B, C]` |
| T3 | bucket A, B, C+1 | `[A, B, C]` |

- 在 T1 之后，桶 A 永远不会再有新条目（绝对时间已经过去），所以 `block(A).Render()`
  字节级永久固定，Tag 包裹也不变，从而**整个 `<|...A|>...<|...A_END|>` 段是 cache hit**。
- T3 相较 T2 只是给 C 加了条目，A、B 段完全不变，可继续命中。

**关键不变量**

1. `bucketStart` 由绝对时间决定，与插入顺序无关。
2. nonce 仅依赖 `bucketStart` 与 `intervalMinutes`。
3. body 首行 `# bucket=...` 也仅依赖这两个参数。
4. body 内 item 行用 `HH:MM:SS [type/verbose]` + 直接续写内容（不加缩进）；item 一旦写入
   就不会修改（除非被 `SoftDelete`，那是有意行为）。

> 测试 `TestGroupByMinutes_PrefixStabilityForCacheHit` 与
> `TestGroupByMinutes_CacheHitRatio_*` 给出了字节级与比例级双重保障。

---

## 6. 与 Dump 的差异

| 维度 | `Timeline.Dump()` | `GroupByMinutes(...).GetBlocks().Render(...)` |
|---|---|---|
| 包含 reducer / archive | 是 | 否（只渲染活跃条目） |
| 时间格式 | `YYYY/MM/DD HH:MM:SS` | block 内 item 用 `HH:MM:SS`；block 头用完整日期 |
| 条目内容 | `item.String()` 完整 | 优先 `GetShrinkResult()` |
| 输出大小 | 大（人类可读） | 小（token 友好） |
| aitag 包裹 | 否 | 是 |
| 缓存稳定性 | 无保证 | 强保证（按桶稳定） |

二者是互补的：`Dump` 用于人类调试与持久化，`GroupByMinutes` 用于 LLM prompt 内的高频引用。

---

## 7. 边界与不变量

- `minutes <= 0` → 返回空 groups，不 panic。
- 空 timeline → 0 blocks，`Render(...)` 返回空字符串。
- 整桶被 `SoftDelete` → 该桶不出现在 `GetBlocks()` 中。
- 跨午夜的两个桶分别属于不同日。
- 边界点（`t == bucketEnd`）落入下一个桶。
- 对同一 timeline 反复调用 `GroupByMinutes(N)` 必产出 byte-equal 的渲染串。

详细见 [`timeline_groups_render_test.go`](timeline_groups_render_test.go)，覆盖了 30+ 用例。

---

## 8. 选型说明（FAQ）

**Q: 为什么 nonce 不直接用 sha256？**
A: aitag 用最后一个 `_` 区分 tagName 与 nonce，nonce 必须**不含下划线**。
   `b{N}t{unixSec}` 既是字母数字、又对人类可读、还能反推桶起点，便于调试。

**Q: 为什么 status (frozen/open) 不写到内容里？**
A: 一旦写进内容，桶从 `open` → `frozen` 时字节流就会变，整段就不再命中缓存。
   状态由 `block.Open` 字段单独暴露，调用方自己决定要不要把 open 段切出来不缓存。

**Q: reducer / archive 为什么不参与？**
A: 保持职责单一。reducer/archive 是 AI 二次摘要，本身已经是缓存友好的（不会变）。
   `GroupByMinutes` 只负责"原始活跃条目按时间分桶"。需要全景视图请用 `Dump`。

**Q: 如果两个桶之间有空洞（中间一段时间没有任何条目）会怎样？**
A: 中间没有条目的桶不会出现在 `GetBlocks()` 里。这不影响缓存命中：每个 block
   仍然由其 nonce 唯一标识，缺失的桶直接被跳过。
