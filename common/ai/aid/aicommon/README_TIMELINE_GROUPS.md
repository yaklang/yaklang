# Timeline 时间桶分组渲染（GroupByMinutes）

## 1. 这是什么

`GroupByMinutes` 是 `Timeline` 的一个**纯增量**渲染入口，它把 timeline 的两类内容
统一切分成一组**可被 [aitag](aitag/README.md) 包裹的 RenderableBlock**：

| Block 类型                | 来源                                 | 是否冻结        |
| ------------------------- | ------------------------------------ | --------------- |
| `TimelineIntervalBlock`   | 活跃条目按 N 分钟绝对时间桶分组      | 末桶 Open，其余 Frozen |
| `TimelineReducerBlock`    | `Timeline.reducers` 已压缩条目摘要   | 永远 Frozen     |

调用方通过 `groups.GetAllRenderable().Render(tagName)` 一次拿到 reducer + interval
混合的 aitag 拼接串，前缀部分（reducer + 已冻结桶）可被 LLM provider 的 prompt
cache 直接命中。

```go
groups := timeline.GroupByMinutes(3)              // N=3 分钟为一桶
prompt := groups.GetAllRenderable().Render("TL")  // reducer 在前 + interval 在后
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
  - interval block: `b{N}t{unixSec}`（N=interval，unixSec=桶起点）
  - reducer block: `r{ReducerKeyID}t{unixSec}`（unixSec=`Timeline.reducerTs` 中存的稳定时间戳）
  
  同一桶/同一 reducer 在不同次调用中产生**完全相同的 nonce**。
- **status 不入标签**：`Open / Frozen` 状态只暴露在 `block.IsOpen()` 上，
  **不写入** 标签或内容字符串，避免一个桶在被冻结时改变其字节流。
- **bucket 元信息写在内容首行**：`# bucket=YYYY/MM/DD HH:MM:SS-HH:MM:SS interval=Nm`
  或 `# reducer key=<id> ts=<unixSec>`，这一行对同一个 block 恒定，
  依然是缓存友好的稳定前缀。

净效果：**只要前面的 block 不再变，它的渲染输出永远字节级不变**，
LLM provider 的前缀缓存（Anthropic prompt caching、OpenAI prefix caching 等）
就可以一直命中前 N-1 个 block，只有最后一个 open 桶需要重新计费。

---

## 3. API 速查

### 主入口

```go
func (m *Timeline) GroupByMinutes(minutes int) *TimelineGroups
```

### 容器

```go
type TimelineGroups struct{ /* private */ }

func (g *TimelineGroups) IntervalMinutes() int
func (g *TimelineGroups) GetBlocks() TimelineIntervalBlocks       // 仅 interval block
func (g *TimelineGroups) GetReducerBlocks() []*TimelineReducerBlock  // 仅 reducer block
func (g *TimelineGroups) GetAllRenderable() TimelineRenderableBlocks // reducer 在前 + interval 在后
```

### Renderable 抽象

```go
type TimelineRenderableBlock interface {
    Render() string
    StableNonce() string
    IsOpen() bool
}

type TimelineRenderableBlocks []TimelineRenderableBlock
func (bs TimelineRenderableBlocks) Render(aitagName string) string
```

### 单个 block

```go
type TimelineIntervalBlock struct {
    BucketStart     time.Time   // 桶起点（已对齐到 N 分钟边界）
    BucketEnd       time.Time   // 桶终点（exclusive）
    IntervalMinutes int
    Items           []*TimelineItem
    Open            bool        // 仅最末有 item 的桶为 true
}
func (b *TimelineIntervalBlock) Render() string
func (b *TimelineIntervalBlock) StableNonce() string  // "b{N}t{unixSec}"
func (b *TimelineIntervalBlock) StableKey() string    // 16 字符 sha256 摘要
func (b *TimelineIntervalBlock) IsOpen() bool

type TimelineReducerBlock struct {
    ReducerKeyID int64        // 来自 reducers 的 key（最末被压缩 item 的原 ID）
    Ts           time.Time    // 来自 Timeline.reducerTs，老数据可能为零
    Text         string       // reducer 摘要文本（来自 AI batchCompress 或 emergencyCompress）
}
func (r *TimelineReducerBlock) Render() string
func (r *TimelineReducerBlock) StableNonce() string  // "r{key}t{unixSec}"
func (r *TimelineReducerBlock) IsOpen() bool         // 恒为 false
```

---

## 4. 输出格式

### Interval block 内容（不含 aitag 包裹）

```
# bucket=2026/05/02 10:00:00-10:03:00 interval=3m
10:00:30 [tool/scan ok]
result-line-1
result-line-2
10:01:45 [user/review]
user-answer
```

### Reducer block 内容（不含 aitag 包裹）

```
# reducer key=42 ts=1746180000
10:00:00 [reducer/memory]
batch-compress summary line 1
batch-compress summary line 2
```

### 整体 Render 输出（`groups.GetAllRenderable().Render("TL")`）

```
<|TL_r42t1746179400|>
# reducer key=42 ts=1746179400
09:50:00 [reducer/memory]
compressed-batch-summary
<|TL_END_r42t1746179400|>
<|TL_b3t1746180000|>
# bucket=2026/05/02 10:00:00-10:03:00 interval=3m
10:00:30 [tool/scan ok]
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

## 5. DEMO：一个 timeline 在压缩前后到底变成什么样子

下面给出**完整的对照例子**，让你直观看到压缩对 token / 字节数和 prompt cache 命中的影响。

### 5.1 压缩前

假设 timeline 中有 5 条相邻几分钟内产生的 ToolResult，每条原始数据都是一段 ~1KB 的扫描结果：

```text
id=101 09:50:01 [tool/scan ok]   data="<1024 字节扫描原文>"
id=102 09:51:30 [tool/scan ok]   data="<1024 字节扫描原文>"
id=103 09:53:11 [tool/scan ok]   data="<1024 字节扫描原文>"
id=104 09:54:42 [tool/scan ok]   data="<1024 字节扫描原文>"
id=105 09:55:22 [tool/scan ok]   data="<1024 字节扫描原文>"
... 后续还有几条仍在活跃中的 entry：
id=106 09:58:00 [tool/cat ok]    data="cat-result-A"
id=107 10:01:30 [user/review]    "looks good"
id=108 10:04:10 [text/note]      "[normal] noted"
```

调用 `groups.GetAllRenderable().Render("TL")`，输出大致是（**无 reducer**）：

```
<|TL_b3t1746179400|>
# bucket=2026/05/02 09:48:00-09:51:00 interval=3m
09:50:01 [tool/scan ok]
<1024 字节扫描原文>
<|TL_END_b3t1746179400|>
<|TL_b3t1746179580|>
# bucket=2026/05/02 09:51:00-09:54:00 interval=3m
09:51:30 [tool/scan ok]
<1024 字节扫描原文>
09:53:11 [tool/scan ok]
<1024 字节扫描原文>
<|TL_END_b3t1746179580|>
<|TL_b3t1746179760|>
# bucket=2026/05/02 09:54:00-09:57:00 interval=3m
09:54:42 [tool/scan ok]
<1024 字节扫描原文>
09:55:22 [tool/scan ok]
<1024 字节扫描原文>
<|TL_END_b3t1746179760|>
<|TL_b3t1746179940|>
# bucket=2026/05/02 09:57:00-10:00:00 interval=3m
09:58:00 [tool/cat ok]
cat-result-A
<|TL_END_b3t1746179940|>
<|TL_b3t1746180060|>
# bucket=2026/05/02 10:00:00-10:03:00 interval=3m
10:01:30 [user/review]
looks good
<|TL_END_b3t1746180060|>
<|TL_b3t1746180240|>
# bucket=2026/05/02 10:03:00-10:06:00 interval=3m
10:04:10 [text/note]
[normal] noted
<|TL_END_b3t1746180240|>
```

总长度 ≈ 5KB（5 段原文）+ 桶头与 aitag 包裹开销。

### 5.2 触发批量压缩

`Timeline.batchCompressByTargetSize` 把前 5 条 ToolResult（ID 101..105）作为一批
喂给 AI，要求生成一条 `reducer_memory`，比如：

```text
"5 scan invocations between 09:50 and 09:55 against host X, all returned port 80 open, no other anomaly"
```

随后：

- `m.idToTimelineItem.Delete(101..105)`，把这 5 条原始条目移出活跃区
- `m.reducers.Set(105, "5 scan invocations ... no other anomaly")`
- **新行为**：`m.reducerTs.Set(105, <ts(105)>)` 同步记录最末被压缩 item 的原始毫秒时间戳

### 5.3 压缩后的 Render 输出

再次调用 `groups.GetAllRenderable().Render("TL")`：

```
<|TL_r105t1746179722|>
# reducer key=105 ts=1746179722
09:55:22 [reducer/memory]
5 scan invocations between 09:50 and 09:55 against host X, all returned port 80 open, no other anomaly
<|TL_END_r105t1746179722|>
<|TL_b3t1746179940|>
# bucket=2026/05/02 09:57:00-10:00:00 interval=3m
09:58:00 [tool/cat ok]
cat-result-A
<|TL_END_b3t1746179940|>
<|TL_b3t1746180060|>
# bucket=2026/05/02 10:00:00-10:03:00 interval=3m
10:01:30 [user/review]
looks good
<|TL_END_b3t1746180060|>
<|TL_b3t1746180240|>
# bucket=2026/05/02 10:03:00-10:06:00 interval=3m
10:04:10 [text/note]
[normal] noted
<|TL_END_b3t1746180240|>
```

观察要点：

1. **5 条 1KB 扫描原文 → 1 行 reducer 摘要**。token 数从 ~5KB 直接降到几百字节，
   节省比 ≈ 90%。
2. **reducer 行使用稳定时间戳**（来自 `reducerTs[105] = 1746179722_000` ms），
   `09:55:22 [reducer/memory]` 在多次 Dump / Render 间字节级一致。
3. **后续多次调用同一个 timeline 的 Render**：
   - 即使活跃区又新增条目，reducer block 的输出**永远不变**；
   - 此前的 frozen interval block（如 `TL_b3t1746179940`）也不变；
   - 只有最末一个 open 桶可能换 nonce 或新增 entry，需要重新计费。
4. **传入 LLM**：在 `aitag.SplitViaTAG(..., "TL")` 之后，每个 block 仍然是独立的
   tagged block，可以按需选择"全保留"或"丢掉 reducer 之外的早期 block"。

> 在 prompt cache 视角下，这意味着只要 timeline 不再有 ID ≤ 105 的"重生"，
> `<|TL_r105t1746179722|>...<|TL_END_r105t1746179722|>` 这一段就**永久 cache hit**。

---

## 6. 缓存原理

> 假设 timeline 每经过一段时间被 dump 一次给 LLM。

| 时刻 | timeline 状态                          | render 出的 nonce 序列        |
| ---- | -------------------------------------- | ----------------------------- |
| T0   | bucket A                               | `[A]`                         |
| T1   | bucket A, B                            | `[A, B]`                      |
| T2   | bucket A, B, C                         | `[A, B, C]`                   |
| T3   | bucket A, B, C+1                       | `[A, B, C]`（C 内容延长）     |
| T4   | reducer R(覆盖 A)，bucket B, C         | `[R, B, C]`                   |

- 在 T1 之后，桶 A 永远不会再有新条目，`block(A)` 字节级永久固定，
  整段 `<|...A|>...<|...A_END|>` cache hit。
- T3 相较 T2 只是给 C 加了条目，A、B 段完全不变，可继续命中。
- T4 把 A 压缩成了 reducer R：R 的 nonce 由 `r{ReducerKeyID}t{unixSec(reducerTs)}`
  决定，从此**任何时候只要再次 Dump 这同一个 timeline，R 的字节流都不变**。

**关键不变量**

1. `bucketStart` 由绝对时间决定，与插入顺序无关。
2. interval nonce 仅依赖 `bucketStart` 与 `intervalMinutes`。
3. reducer nonce 仅依赖 `ReducerKeyID` 与 `reducerTs[ReducerKeyID]`。
4. body 首行（`# bucket=...` 或 `# reducer key=... ts=...`）也仅依赖上述参数。
5. body 内 entry 行用 `HH:MM:SS [type/verbose]` + 直接续写内容（不加缩进）；
   item 一旦写入就不会修改（除非被 `SoftDelete`，那是有意行为）。

> 测试 `TestGroupByMinutes_PrefixStabilityForCacheHit` /
> `TestGroupByMinutes_CacheHitRatio_*` /
> `TestDumpBefore_ReducerTimeStable` /
> `TestGroupByMinutes_ReducerBlock_PrefixStability` 给出了字节级与比例级双重保障。

---

## 7. 与 Dump 的差异

| 维度                  | `Timeline.Dump()`                | `GroupByMinutes(...).GetAllRenderable().Render(...)` |
| --------------------- | -------------------------------- | ---------------------------------------------------- |
| 包含 reducer          | 是                               | 是（独立 reducer block）                             |
| 包含 archive          | 是                               | 否（archive 是更上层概念，本接口只关心 timeline 内部）|
| 时间格式              | `YYYY/MM/DD HH:MM:SS`            | block 内 entry 用 `HH:MM:SS`；首行包含完整桶时间     |
| 条目内容              | `item.String()` 完整              | 优先 `GetShrinkResult()` / `GetShrinkSimilarResult()` |
| 输出大小              | 大（人类可读）                    | 小（token 友好）                                     |
| aitag 包裹            | 否                                | 是                                                   |
| 缓存稳定性            | 无保证（旧版 reducer 行用 `time.Now()`） | 强保证（reducer 用 `reducerTs`，interval 用绝对桶时间） |

> 本次改动也修复了 `DumpBefore` 中 reducer 行原本使用 `time.Now()` 渲染的问题。
> 现在 `DumpBefore` 与 `GroupByMinutes` 共享同一份 `Timeline.reducerTs`，二者
> 输出在重复调用之间都字节级一致。

---

## 8. 边界与不变量

- `minutes <= 0` → 返回空 groups，不 panic。
- 空 timeline → 0 interval blocks；若仍有 reducer，会产出 reducer block。
- 整桶被 `SoftDelete` → 该桶不出现在 `GetBlocks()` 中。
- 跨午夜的两个桶分别属于不同日。
- 边界点（`t == bucketEnd`）落入下一个桶。
- 对同一 timeline 反复调用 `GroupByMinutes(N)` 必产出 byte-equal 的渲染串。
- 反序列化老数据（含 `summary` 字段）：`summary` 内容被忽略，其余字段照常恢复。

详细见 [`timeline_groups_render_test.go`](timeline_groups_render_test.go) /
[`timeline_groups_render_aitag_test.go`](timeline_groups_render_aitag_test.go) /
[`timeline_reducer_block_test.go`](timeline_reducer_block_test.go)，覆盖了 40+ 用例。

---

## 9. 选型说明（FAQ）

**Q: 为什么 nonce 不直接用 sha256？**
A: aitag 用最后一个 `_` 区分 tagName 与 nonce，nonce 必须**不含下划线**。
   `b{N}t{unixSec}` / `r{key}t{unixSec}` 既是字母数字、又对人类可读、还能反推出处，便于调试。

**Q: 为什么 status (frozen/open) 不写到内容里？**
A: 一旦写进内容，桶从 `open` → `frozen` 时字节流就会变，整段就不再命中缓存。
   状态由 `block.IsOpen()` 单独暴露，调用方自己决定要不要把 open 段切出来不缓存。

**Q: 为什么 reducer block 总是 frozen？**
A: reducer 是 batch/emergency compress 的产物，一旦写入就不会就地修改。
   后续如果有新一轮压缩，会形成新的 `reducerKeyID` 与新的 reducer block，
   不会回写原 block。

**Q: 老数据中只有 reducer 没有 reducerTs（ts 为 0）会怎样？**
A: 渲染时使用稳定占位（`# reducer key=<id> ts=0`、行头 `00:00:00`），
   依然字节级稳定，不会破坏缓存。`DumpBefore` 也使用相同的 `1970/01/01 00:00:00`
   占位字符串。

**Q: 为什么删除 `Timeline.summary` 字段？**
A: 代码搜索确认 `summary` 字段在生产路径里**没有任何写入**——它的语义
   "单条 shrink 结果记录"已被 `TimelineItemValue.GetShrinkResult()` 取代。
   保留这个字段只会在 marshal/reassign/softdelete 等多处带来无效分支与潜在 bug。
   因此本次改动直接移除字段与所有读写路径，仅在反序列化时容忍老数据中残存的
   `summary` JSON 字段并静默忽略。
