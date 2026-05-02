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
type TimelineGroups struct {
	intervalMinutes int
	blocks          TimelineIntervalBlocks
}

// GetBlocks 返回当前分组的 block 切片，多次调用返回同一切片引用，避免复制开销
func (g *TimelineGroups) GetBlocks() TimelineIntervalBlocks {
	if g == nil {
		return nil
	}
	return g.blocks
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

	return &TimelineGroups{
		intervalMinutes: minutes,
		blocks:          TimelineIntervalBlocks(orderedBuckets),
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
