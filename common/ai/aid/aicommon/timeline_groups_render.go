package aicommon

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/linktable"
)

// TimelineIntervalBlock 表示一个绝对时间对齐的固定时间桶
// 桶起点对齐到 N 分钟边界，例如 N=3 时桶为 [10:00,10:03)、[10:03,10:06)
// 这种对齐方式让相同时间永远落入相同桶，从而保证渲染前缀字节级稳定，便于 LLM 前缀缓存命中
type TimelineIntervalBlock struct {
	BucketStart     time.Time
	BucketEnd       time.Time // exclusive
	IntervalMinutes int
	Items           []*TimelineItem // 按 id 升序，已剔除 deleted
	Open            bool            // 仅最末一个产生 item 的桶为 true
}

// TimelineIntervalBlocks 是按时间顺序排列的 block 切片
type TimelineIntervalBlocks []*TimelineIntervalBlock

// TimelineGroups 是 GroupByMinutes 的结果，包含若干 block
// 持有 intervalMinutes 元信息以便外层校验/调试
// reducerBlocks 持有由 Timeline.reducers 派生出的稳定可渲染 reducer 块，按 ReducerKeyID 升序
// 关键词: TimelineGroups, reducerBlocks
type TimelineGroups struct {
	intervalMinutes int
	blocks          TimelineIntervalBlocks
	reducerBlocks   []*TimelineReducerBlock
}

// GetBlocks 返回当前分组的 block 切片，多次调用返回同一切片引用，避免复制开销
func (g *TimelineGroups) GetBlocks() TimelineIntervalBlocks {
	if g == nil {
		return nil
	}
	return g.blocks
}

// GetReducerBlocks 返回当前分组中由 Timeline.reducers 派生出的 reducer block
// 不复制底层切片，调用方不应修改返回值
// 关键词: TimelineGroups.GetReducerBlocks
func (g *TimelineGroups) GetReducerBlocks() []*TimelineReducerBlock {
	if g == nil {
		return nil
	}
	return g.reducerBlocks
}

// GetAllRenderable 返回 reducer blocks 在前、interval blocks 在后的统一可渲染列表
// 该顺序与 DumpBefore 一致：先输出 reducer，再输出活跃 timeline item 的时间桶
// 关键词: TimelineGroups.GetAllRenderable, reducer 优先, 与 Dump 一致
func (g *TimelineGroups) GetAllRenderable() TimelineRenderableBlocks {
	if g == nil {
		return nil
	}
	out := make(TimelineRenderableBlocks, 0, len(g.reducerBlocks)+len(g.blocks))
	for _, rb := range g.reducerBlocks {
		if rb == nil {
			continue
		}
		out = append(out, rb)
	}
	for _, blk := range g.blocks {
		if blk == nil {
			continue
		}
		out = append(out, blk)
	}
	return out
}

// IntervalMinutes 返回分桶时使用的分钟数
func (g *TimelineGroups) IntervalMinutes() int {
	if g == nil {
		return 0
	}
	return g.intervalMinutes
}

// GroupByMinutes 按 N 分钟时间桶对当前 timeline 中的活跃条目分组
// 桶按绝对时间边界对齐（例如 N=3 时起点是 :00、:03、:06...）
// minutes <= 0 时返回空 *TimelineGroups（GetBlocks 为 nil）
// 该方法不修改 timeline 任何字段，纯读取
// 关键词: GroupByMinutes, 时间桶分组, 缓存友好渲染
func (m *Timeline) GroupByMinutes(minutes int) *TimelineGroups {
	if m == nil || minutes <= 0 {
		return &TimelineGroups{intervalMinutes: 0}
	}
	if m.idToTimelineItem == nil || m.idToTimelineItem.Len() == 0 {
		return &TimelineGroups{intervalMinutes: minutes}
	}

	type bucketKey struct {
		startUnix int64
	}
	bucketIndex := make(map[bucketKey]*TimelineIntervalBlock)
	var orderedBuckets []*TimelineIntervalBlock

	intervalDur := time.Duration(minutes) * time.Minute

	m.idToTimelineItem.ForEach(func(id int64, item *TimelineItem) bool {
		if item == nil {
			return true
		}
		if item.deleted {
			return true
		}
		// 取时间戳：优先 createdAt（保留时区信息），回退 idToTs 毫秒戳
		// 不优先用 idToTs 是因为 time.Unix 会丢失原 location，导致跨时区分桶不一致
		var t time.Time
		if !item.createdAt.IsZero() {
			t = item.createdAt
		} else if ts, ok := m.idToTs.Get(id); ok && ts > 0 {
			t = time.Unix(0, ts*int64(time.Millisecond))
		} else {
			return true
		}

		// 对齐到 N 分钟绝对边界
		bucketStart := alignToBucket(t, minutes)
		key := bucketKey{startUnix: bucketStart.UnixNano()}
		blk, ok := bucketIndex[key]
		if !ok {
			blk = &TimelineIntervalBlock{
				BucketStart:     bucketStart,
				BucketEnd:       bucketStart.Add(intervalDur),
				IntervalMinutes: minutes,
				Items:           nil,
				Open:            false,
			}
			bucketIndex[key] = blk
			orderedBuckets = append(orderedBuckets, blk)
		}
		blk.Items = append(blk.Items, item)
		return true
	})

	// 按 BucketStart 升序排序
	sort.SliceStable(orderedBuckets, func(i, j int) bool {
		return orderedBuckets[i].BucketStart.Before(orderedBuckets[j].BucketStart)
	})

	// 每个桶内按 id 升序
	for _, blk := range orderedBuckets {
		sort.SliceStable(blk.Items, func(i, j int) bool {
			return blk.Items[i].GetID() < blk.Items[j].GetID()
		})
	}

	// 标记最末一个桶为 Open
	if len(orderedBuckets) > 0 {
		orderedBuckets[len(orderedBuckets)-1].Open = true
	}

	// 收集 reducer blocks（已压缩条目）
	// 关键词: GroupByMinutes, reducerBlocks 填充, reducer 渲染
	var reducerBlocks []*TimelineReducerBlock
	if m.reducers != nil && m.reducers.Len() > 0 {
		m.reducers.ForEach(func(reducerKeyID int64, lt *linktable.LinkTable[string]) bool {
			if lt == nil {
				return true
			}
			text := lt.Value()
			if strings.TrimSpace(text) == "" {
				return true
			}
			var ts time.Time
			if m.reducerTs != nil {
				if msTs, ok := m.reducerTs.Get(reducerKeyID); ok && msTs > 0 {
					ts = time.Unix(0, msTs*int64(time.Millisecond))
				}
			}
			reducerBlocks = append(reducerBlocks, &TimelineReducerBlock{
				ReducerKeyID: reducerKeyID,
				Ts:           ts,
				Text:         text,
			})
			return true
		})
		// 按 ReducerKeyID 升序排序，保证渲染顺序稳定
		sort.SliceStable(reducerBlocks, func(i, j int) bool {
			return reducerBlocks[i].ReducerKeyID < reducerBlocks[j].ReducerKeyID
		})
	}

	return &TimelineGroups{
		intervalMinutes: minutes,
		blocks:          TimelineIntervalBlocks(orderedBuckets),
		reducerBlocks:   reducerBlocks,
	}
}

// alignToBucket 将 t 对齐到 N 分钟绝对边界
// 保留 t 的时区信息，归零秒与纳秒，并把分钟向下取整到 N 的倍数
func alignToBucket(t time.Time, minutes int) time.Time {
	if minutes <= 0 {
		return t
	}
	loc := t.Location()
	year, month, day := t.Date()
	hour, minute, _ := t.Clock()
	alignedMin := (minute / minutes) * minutes
	return time.Date(year, month, day, hour, alignedMin, 0, 0, loc)
}

// Render 渲染单个 block 的内部内容（不包含 aitag 包裹）
// 输出格式（无任何前导缩进，最大化 token 节省）：
//
//	# bucket=YYYY/MM/DD HH:MM:SS-HH:MM:SS interval=Nm
//	HH:MM:SS [type/verbose]
//	${shrunk content line 1}
//	${shrunk content line 2}
//	HH:MM:SS [type/verbose]
//	${...}
//
// 首行 metadata 对同一桶恒定，是缓存友好的前缀；
// 不写 frozen/open status 到内容里，保证冻结后字节级稳定。
// LLM 凭 HH:MM:SS 行头识别新 entry，无需缩进区分。
// 优先使用 GetShrinkResult()；折叠连续空行；剔除前后空白
// 关键词: TimelineIntervalBlock.Render, 紧凑渲染, token 节省, 缓存稳定, 无缩进
func (b *TimelineIntervalBlock) Render() string {
	if b == nil || len(b.Items) == 0 {
		return ""
	}
	var buf bytes.Buffer
	// 首行 metadata：bucket 时间范围 + interval。同一桶永远不变，可作稳定前缀
	buf.WriteString(fmt.Sprintf("# bucket=%s-%s interval=%dm\n",
		b.BucketStart.Format(utils.DefaultTimeFormat3),
		b.BucketEnd.Format("15:04:05"),
		b.IntervalMinutes,
	))

	first := true
	for _, item := range b.Items {
		if item == nil || item.deleted {
			continue
		}
		var ts time.Time
		if !item.createdAt.IsZero() {
			ts = item.createdAt
		} else {
			ts = b.BucketStart
		}
		hh, mm, ss := ts.Clock()
		typeVerbose := renderItemTypeVerbose(item)
		if !first {
			buf.WriteByte('\n')
		}
		// 行头：HH:MM:SS [type/verbose]
		buf.WriteString(fmt.Sprintf("%02d:%02d:%02d [%s]", hh, mm, ss, typeVerbose))
		first = false

		content := selectShrunkContent(item)
		if content == "" {
			continue
		}
		// 折叠多个连续空行为单空行；不加任何缩进
		var prevBlank bool
		for _, line := range utils.ParseStringToRawLines(content) {
			line = strings.TrimRight(line, " \t\r")
			if strings.TrimSpace(line) == "" {
				if prevBlank {
					continue
				}
				prevBlank = true
				buf.WriteByte('\n')
				continue
			}
			prevBlank = false
			buf.WriteByte('\n')
			buf.WriteString(line)
		}
	}
	return strings.TrimRight(buf.String(), "\n")
}

// StableNonce 基于桶绝对起点与 interval 派生的稳定 nonce
// 同一桶（相同 BucketStart + IntervalMinutes）永远产生相同 nonce
// 不含下划线，符合 aitag 标签规范（aitag 以最后一个 _ 区分 tagName 与 nonce）
// 关键词: TimelineIntervalBlock.StableNonce, aitag 兼容 nonce, 缓存稳定
func (b *TimelineIntervalBlock) StableNonce() string {
	if b == nil {
		return ""
	}
	// 用秒级 unix 时间足够区分（桶最小粒度 1 分钟），加 interval 避免不同 interval 重合
	return fmt.Sprintf("b%dt%d", b.IntervalMinutes, b.BucketStart.Unix())
}

// StableKey 返回当前 block 的稳定哈希（基于桶范围 + 渲染内容）
// 不包含 Open 字段，所以桶从 open 转 frozen 时只要内容未变 key 就不变；
// 当且仅当桶完全冻结后追加条目影响该桶时 key 才会变化
// 用于调试与单测断言：frozen block 渲染的字节级稳定性
// 关键词: TimelineIntervalBlock.StableKey, 缓存稳定性校验
func (b *TimelineIntervalBlock) StableKey() string {
	if b == nil {
		return ""
	}
	h := sha256.New()
	h.Write([]byte(fmt.Sprintf("interval=%d|start=%d|end=%d|content=",
		b.IntervalMinutes,
		b.BucketStart.UnixNano(),
		b.BucketEnd.UnixNano(),
	)))
	h.Write([]byte(b.Render()))
	return hex.EncodeToString(h.Sum(nil))[:16]
}

// Render 将所有 block 按 aitag 兼容格式拼接：
//
//	<|TAGNAME_b{N}t{unixSec}|>
//	# bucket=YYYY/MM/DD HH:MM:SS-HH:MM:SS interval=Nm
//	${block body lines}
//	<|TAGNAME_END_b{N}t{unixSec}|>
//
// 每个 block 各用一个稳定派生的 nonce 包裹，可被 aitag.SplitViaTAG / aitag.Parse
// 解析为独立的 tagged block。frozen block 的标签与内容均字节级稳定，前缀缓存命中
// 标签内不写 status，frozen/open 信息通过 TimelineIntervalBlock.Open 字段暴露
// 关键词: TimelineIntervalBlocks.Render, aitag 兼容, 稳定 nonce, 前缀缓存
func (bs TimelineIntervalBlocks) Render(aitagName string) string {
	if len(bs) == 0 {
		return ""
	}
	tag := normalizeAITagName(aitagName)
	var buf bytes.Buffer
	for i, blk := range bs {
		if blk == nil {
			continue
		}
		nonce := blk.StableNonce()
		if i > 0 {
			buf.WriteByte('\n')
		}
		buf.WriteString(fmt.Sprintf("<|%s_%s|>\n", tag, nonce))
		body := blk.Render()
		if body != "" {
			buf.WriteString(body)
			buf.WriteByte('\n')
		}
		buf.WriteString(fmt.Sprintf("<|%s_END_%s|>", tag, nonce))
	}
	return buf.String()
}

// normalizeAITagName 规范化 tagName：剔除不合法字符（aitag 仅接受字母数字下划线）
// 空字符串回退为 TIMELINE_INTERVAL_GROUP
// 关键词: normalizeAITagName, aitag 兼容
func normalizeAITagName(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "TIMELINE_INTERVAL_GROUP"
	}
	var b strings.Builder
	for _, ch := range s {
		if (ch >= 'A' && ch <= 'Z') || (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9') || ch == '_' {
			b.WriteRune(ch)
		}
	}
	if b.Len() == 0 {
		return "TIMELINE_INTERVAL_GROUP"
	}
	return b.String()
}

// renderItemTypeVerbose 返回 [type/verbose] 中的 type/verbose 部分
// 关键词: 类型 verbose, ToolResult/UserInteraction/Text 区分
func renderItemTypeVerbose(item *TimelineItem) string {
	if item == nil || item.value == nil {
		return "raw/unknown"
	}
	switch v := item.value.(type) {
	case *aitool.ToolResult:
		status := "ok"
		if !v.Success {
			status = "fail"
		}
		name := strings.TrimSpace(v.Name)
		if name == "" {
			name = "unknown"
		}
		return fmt.Sprintf("tool/%s %s", name, status)
	case *UserInteraction:
		stage := string(v.Stage)
		if stage == "" {
			stage = string(UserInteractionStage_FreeInput)
		}
		return fmt.Sprintf("user/%s", stage)
	case *TextTimelineItem:
		entry := extractTextEntryType(v.Text)
		if entry == "" {
			entry = "raw"
		}
		return fmt.Sprintf("text/%s", entry)
	default:
		return "raw/unknown"
	}
}

// extractTextEntryType 从 TextTimelineItem.Text 提取 [entryType] 头部
// 复用 timeline_item_human_readable.go 中已有的正则风格
// 关键词: TextTimelineItem entryType 提取
func extractTextEntryType(text string) string {
	if text == "" {
		return ""
	}
	m := withTaskRegex.FindStringSubmatch(text)
	if len(m) > 1 {
		return strings.TrimSpace(m[1])
	}
	m = withoutTaskRegex.FindStringSubmatch(text)
	if len(m) > 1 {
		return strings.TrimSpace(m[1])
	}
	return ""
}

// selectShrunkContent 优先返回 GetShrinkResult，回退到 GetShrinkSimilarResult，最后回退到 String
// 用于 token 节省：尽量使用已存在的精简表示
// 关键词: 优先 ShrinkResult, token 优化
func selectShrunkContent(item *TimelineItem) string {
	if item == nil || item.value == nil {
		return ""
	}
	if s := strings.TrimSpace(item.value.GetShrinkResult()); s != "" {
		return s
	}
	if s := strings.TrimSpace(item.value.GetShrinkSimilarResult()); s != "" {
		return s
	}
	return strings.TrimSpace(item.value.String())
}

// TimelineRenderableBlock 是 timeline 中"可被 aitag 包裹渲染"的统一抽象
// 任何实现该接口的类型都可以被 TimelineRenderableBlocks 拼装为 aitag 包裹的连续段
// IsOpen 用于上层缓存策略：true 表示当前仍可能变化、不建议缓存；false 表示已冻结
// 关键词: TimelineRenderableBlock, aitag 包裹, frozen/open
type TimelineRenderableBlock interface {
	Render() string
	StableNonce() string
	IsOpen() bool
}

// IsOpen 实现 TimelineRenderableBlock 接口
// 仅时间桶最末一个产生 item 的桶为 Open，其他全部为 false（已冻结）
// 关键词: TimelineIntervalBlock.IsOpen
func (b *TimelineIntervalBlock) IsOpen() bool {
	if b == nil {
		return false
	}
	return b.Open
}

// TimelineReducerBlock 表示一个由 Timeline.reducers 中已压缩条目派生出的可渲染块
// 始终为 frozen（IsOpen 恒为 false），渲染内容稳定可缓存
// 关键词: TimelineReducerBlock, reducer 渲染, 缓存稳定
type TimelineReducerBlock struct {
	ReducerKeyID int64
	Ts           time.Time // 来自 Timeline.reducerTs；为零时表示老数据无稳定时间戳
	Text         string
}

// Render 渲染单个 reducer block 的内部内容（不含 aitag 包裹）
// 输出格式（与 TimelineIntervalBlock.Render 风格对齐，无前导缩进）：
//
//	# reducer key=<id> ts=<seconds since epoch or 0>
//	HH:MM:SS [reducer/memory]
//	${reducer text line 1}
//	${reducer text line 2}
//
// 关键词: TimelineReducerBlock.Render, 缓存稳定, 无缩进
func (r *TimelineReducerBlock) Render() string {
	if r == nil {
		return ""
	}
	var sec int64
	hh, mm, ss := 0, 0, 0
	if !r.Ts.IsZero() {
		sec = r.Ts.Unix()
		hh, mm, ss = r.Ts.Clock()
	}
	var buf bytes.Buffer
	buf.WriteString(fmt.Sprintf("# reducer key=%d ts=%d\n", r.ReducerKeyID, sec))
	buf.WriteString(fmt.Sprintf("%02d:%02d:%02d [reducer/memory]", hh, mm, ss))

	text := strings.TrimSpace(r.Text)
	if text != "" {
		var prevBlank bool
		for _, line := range utils.ParseStringToRawLines(text) {
			line = strings.TrimRight(line, " \t\r")
			if strings.TrimSpace(line) == "" {
				if prevBlank {
					continue
				}
				prevBlank = true
				buf.WriteByte('\n')
				continue
			}
			prevBlank = false
			buf.WriteByte('\n')
			buf.WriteString(line)
		}
	}
	return strings.TrimRight(buf.String(), "\n")
}

// StableNonce 基于 ReducerKeyID 与稳定时间戳派生的 aitag-兼容 nonce
// 形如 "r{ReducerKeyID}t{unixSec}"，无下划线、字母数字组合
// 同一 reducer key + ts 永远产生相同 nonce，可被前缀缓存复用
// 关键词: TimelineReducerBlock.StableNonce, aitag nonce
func (r *TimelineReducerBlock) StableNonce() string {
	if r == nil {
		return ""
	}
	var sec int64
	if !r.Ts.IsZero() {
		sec = r.Ts.Unix()
	}
	return fmt.Sprintf("r%dt%d", r.ReducerKeyID, sec)
}

// IsOpen 恒为 false，reducer 一旦写入即视为冻结
// 关键词: TimelineReducerBlock.IsOpen
func (r *TimelineReducerBlock) IsOpen() bool {
	return false
}

// TimelineRenderableBlocks 是任意可渲染块的有序集合（可混合 IntervalBlock + ReducerBlock）
// 关键词: TimelineRenderableBlocks
type TimelineRenderableBlocks []TimelineRenderableBlock

// TimelineFrozenBoundaryTagName 是 RenderWithFrozenBoundary 默认使用的
// "frozen 段外包"标签名。配合 TimelineFrozenBoundaryNonce 一起组成
// <|AI_CACHE_FROZEN_semi-dynamic|>...<|AI_CACHE_FROZEN_END_semi-dynamic|>,
// 让下游 aicache hijacker 通过简单字符串 IndexOf 就能精准切到 frozen
// 与 open 的边界, 无需再深入解析 timeline 内部嵌套 TIMELINE 子标签结构。
//
// 命名说明:
//   - "AI_CACHE_FROZEN" 表明这是 aicache 体系约定的"该段内容已字节冻结、
//     适合作为 prefix cache 命中"的边界标签
//   - "semi-dynamic" 是边界 nonce, 表达"这一段的稳定性介于 high-static 与
//     完全 open 之间", 与 PROMPT_SECTION_semi-dynamic 是不同 tagName, 互不冲突
//
// 关键词: TimelineFrozenBoundaryTagName, AI_CACHE_FROZEN, frozen 边界标签
const (
	TimelineFrozenBoundaryTagName = "AI_CACHE_FROZEN"
	TimelineFrozenBoundaryNonce   = "semi-dynamic"
)

// Render 将所有 renderable block 按 aitag 兼容格式拼接：
//
//	<|TAGNAME_<nonce>|>
//	${block body}
//	<|TAGNAME_END_<nonce>|>
//
// 同一个 aitagName 下不同 block 通过各自稳定 nonce 区分；
// frozen block 的标签与内容均字节级稳定，可被 LLM 前缀缓存命中
//
// 注意: 这是不带 frozen boundary 的"裸"渲染, 主要用于不需要切缓存边界的
// 场景 (例如调试 dump、测试断言)。生产 prompt 路径建议走
// RenderWithFrozenBoundary, 它在外层加上 AI_CACHE_FROZEN 边界让 hijacker
// 能精准识别可缓存前缀。
//
// 关键词: TimelineRenderableBlocks.Render, aitag 兼容, 前缀缓存, 裸渲染
func (bs TimelineRenderableBlocks) Render(aitagName string) string {
	if len(bs) == 0 {
		return ""
	}
	tag := normalizeAITagName(aitagName)
	var buf bytes.Buffer
	emitted := 0
	for _, blk := range bs {
		if blk == nil {
			continue
		}
		nonce := blk.StableNonce()
		if emitted > 0 {
			buf.WriteByte('\n')
		}
		buf.WriteString(fmt.Sprintf("<|%s_%s|>\n", tag, nonce))
		body := blk.Render()
		if body != "" {
			buf.WriteString(body)
			buf.WriteByte('\n')
		}
		buf.WriteString(fmt.Sprintf("<|%s_END_%s|>", tag, nonce))
		emitted++
	}
	return buf.String()
}

// RenderWithFrozenBoundary 在 Render 的基础上, 把"已冻结"前缀段外面再
// 包一层 <|frozenTagName_frozenNonce|>...<|frozenTagName_END_frozenNonce|>
// 边界标签, 让下游缓存切割逻辑 (aicache hijacker) 通过简单字符串 IndexOf
// 就能精准定位到 frozen 与 open 的边界, 无需再深入解析每个 TIMELINE 子块。
//
// 输出形态 (frozenTagName=AI_CACHE_FROZEN, frozenNonce=semi-dynamic 时):
//
//	<|AI_CACHE_FROZEN_semi-dynamic|>
//	<|TAGNAME_<nonce-of-reducer-1>|>...<|TAGNAME_END_<nonce-of-reducer-1>|>
//	<|TAGNAME_<nonce-of-frozen-interval-1>|>...<|TAGNAME_END_<nonce-of-frozen-interval-1>|>
//	...
//	<|AI_CACHE_FROZEN_END_semi-dynamic|>
//	<|TAGNAME_<nonce-of-open-interval>|>...<|TAGNAME_END_<nonce-of-open-interval>|>
//
// 边界判定: 走列表 + IsOpen() 看尾部 — frozen 段 = 所有 IsOpen()==false 的连续前缀,
// open 段 = 第一个 IsOpen()==true 之后的所有 block。这个判定与
// TimelineGroups.GetAllRenderable() 输出顺序 (reducer 在前 + interval 按时间升序)
// 完全一致, reducer 恒 frozen, 仅最末时间桶 IsOpen()=true。
//
// 边界**只在确实存在 frozen 段时**才包裹:
//   - 全 open (没有任何 frozen block) -> 不包裹, 直接 Render(aitagName)
//   - 全 frozen (没有 open block) -> 不包裹, 直接 Render(aitagName)
//     (整段都是 frozen, 不需要切边界, 可整体作为 prefix cache)
//   - 一个 frozen + 一个 open -> 包裹 frozen, open 留在边界外
//
// 这避免了"边界标签存在但实际无 open 段"的歧义场景, 让 hijacker 看到边界
// 标签时可以确信"边界后必有易变内容, 需要切到独立 user 消息"。
//
// 字节稳定性保证 (与 prefix cache 命中前提对齐):
//   - frozen 段内部的 block.Render() 已字节稳定 (reducer / 非末时间桶)
//   - 边界标签字面量恒定 (不含动态值, 仅常量 frozenTagName_frozenNonce)
//   - 因此只要 frozen 段 block 列表内容不变, 整段输出 (含边界) 字节级一致
//   - open 段内容变化不影响 frozen 段输出 (它们位于边界 END 之后)
//
// frozenTagName / frozenNonce 留空时使用包级默认 TimelineFrozenBoundaryTagName /
// TimelineFrozenBoundaryNonce。
//
// 关键词: TimelineRenderableBlocks.RenderWithFrozenBoundary, AI_CACHE_FROZEN,
//        frozen open 边界标签, hijacker 切割锚点, 前缀缓存
func (bs TimelineRenderableBlocks) RenderWithFrozenBoundary(aitagName, frozenTagName, frozenNonce string) string {
	if len(bs) == 0 {
		return ""
	}

	// 过滤 nil 并区分 frozen / open
	frozen := make(TimelineRenderableBlocks, 0, len(bs))
	open := make(TimelineRenderableBlocks, 0, len(bs))
	for _, blk := range bs {
		if blk == nil {
			continue
		}
		if blk.IsOpen() {
			open = append(open, blk)
		} else {
			frozen = append(frozen, blk)
		}
	}

	// 全 open 或全 frozen -> 不包边界, 直接走原 Render
	if len(frozen) == 0 || len(open) == 0 {
		return bs.Render(aitagName)
	}

	bTag := normalizeAITagName(frozenTagName)
	if bTag == "TIMELINE_INTERVAL_GROUP" && strings.TrimSpace(frozenTagName) == "" {
		// 调用方未指定时回退到包级默认
		bTag = normalizeAITagName(TimelineFrozenBoundaryTagName)
	}
	bNonce := strings.TrimSpace(frozenNonce)
	if bNonce == "" {
		bNonce = TimelineFrozenBoundaryNonce
	}

	frozenBody := frozen.Render(aitagName)
	openBody := open.Render(aitagName)

	var buf bytes.Buffer
	buf.WriteString(fmt.Sprintf("<|%s_%s|>\n", bTag, bNonce))
	buf.WriteString(frozenBody)
	buf.WriteByte('\n')
	buf.WriteString(fmt.Sprintf("<|%s_END_%s|>\n", bTag, bNonce))
	buf.WriteString(openBody)
	return buf.String()
}

// RenderFrozenOnly 只渲染所有 IsOpen()==false 的 block (reducer + 非末 interval),
// 不带任何 frozen 边界标签外壳。这是 Render 在"按稳定性分层"路径下的 frozen 半区
// 等价物: 调用方自行决定边界 wrap 时机 (例如 prompt builder 把多种来源的 frozen
// 内容合并到单一 AI_CACHE_FROZEN 块时, 不希望 timeline 自带一对 wrap 标签)。
//
// 全 open / 无 frozen 时返回空串; 仅有 1 个 frozen 时也照常输出 (没有"必须配 open"约束)。
//
// 关键词: TimelineRenderableBlocks.RenderFrozenOnly, frozen-only 渲染, 分层 prompt
func (bs TimelineRenderableBlocks) RenderFrozenOnly(aitagName string) string {
	if len(bs) == 0 {
		return ""
	}
	frozen := make(TimelineRenderableBlocks, 0, len(bs))
	for _, blk := range bs {
		if blk == nil {
			continue
		}
		if !blk.IsOpen() {
			frozen = append(frozen, blk)
		}
	}
	if len(frozen) == 0 {
		return ""
	}
	return frozen.Render(aitagName)
}

// RenderOpenOnly 只渲染所有 IsOpen()==true 的 block (即最末 interval 桶),
// 不带任何边界标签外壳。配合 RenderFrozenOnly 完成"按稳定性分层"渲染:
// frozen 段塞进 AI_CACHE_FROZEN 块, open 段单独放到 cc 点之后。
//
// 全 frozen / 无 open 时返回空串。
//
// 关键词: TimelineRenderableBlocks.RenderOpenOnly, open-only 渲染, 分层 prompt
func (bs TimelineRenderableBlocks) RenderOpenOnly(aitagName string) string {
	if len(bs) == 0 {
		return ""
	}
	open := make(TimelineRenderableBlocks, 0, len(bs))
	for _, blk := range bs {
		if blk == nil {
			continue
		}
		if blk.IsOpen() {
			open = append(open, blk)
		}
	}
	if len(open) == 0 {
		return ""
	}
	return open.Render(aitagName)
}
