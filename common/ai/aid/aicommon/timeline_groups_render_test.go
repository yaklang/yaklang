package aicommon

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

// injectTimelineItem 直接将 item 写入 timeline 内部映射，绕过 time.Now()
// 仅用于测试构造确定性时间戳，避免与并发时间冲突
// 关键词: timeline 测试构造, 确定性时间戳
func injectTimelineItem(tl *Timeline, id int64, ts time.Time, value TimelineItemValue) {
	tsMs := ts.UnixMilli()
	item := &TimelineItem{
		createdAt: ts,
		value:     value,
	}
	tl.idToTs.Set(id, tsMs)
	tl.OrderInsertId(id, item)
	tl.OrderInsertTs(tsMs, item)
}

// makeToolResult 构造一个简单的 ToolResult 条目
func makeToolResult(id int64, name string, success bool, data string) *aitool.ToolResult {
	return &aitool.ToolResult{
		ID:          id,
		Name:        name,
		Description: "test tool",
		Param:       map[string]any{"k": "v"},
		Success:     success,
		Data:        data,
	}
}

// TestGroupByMinutes_EmptyTimeline 空 timeline 不应产生任何 block
// 关键词: GroupByMinutes 空 timeline 边界
func TestGroupByMinutes_EmptyTimeline(t *testing.T) {
	tl := NewTimeline(nil, nil)
	groups := tl.GroupByMinutes(3)
	require.NotNil(t, groups)
	require.Equal(t, 3, groups.IntervalMinutes())
	require.Empty(t, groups.GetBlocks())
	require.Equal(t, "", groups.GetBlocks().Render("TIMELINE_INTERVAL_GROUP"))
}

// TestGroupByMinutes_SingleItem_OpenMarked 单条目应产生 1 个 block 且标记为 Open
// 关键词: GroupByMinutes 单条目, Open 标记
func TestGroupByMinutes_SingleItem_OpenMarked(t *testing.T) {
	tl := NewTimeline(nil, nil)
	ts := time.Date(2026, 5, 2, 10, 1, 30, 0, time.UTC)
	injectTimelineItem(tl, 100, ts, makeToolResult(100, "scan", true, "ok"))

	blocks := tl.GroupByMinutes(3).GetBlocks()
	require.Len(t, blocks, 1)
	require.True(t, blocks[0].Open, "single block should be marked Open")
	require.Len(t, blocks[0].Items, 1)
	require.Equal(t, int64(100), blocks[0].Items[0].GetID())
}

// TestGroupByMinutes_SameBucket_MultipleItems 同一桶内多条目应聚合且按 id 升序
// 关键词: GroupByMinutes 同桶聚合, id 排序
func TestGroupByMinutes_SameBucket_MultipleItems(t *testing.T) {
	tl := NewTimeline(nil, nil)
	base := time.Date(2026, 5, 2, 10, 0, 0, 0, time.UTC)
	injectTimelineItem(tl, 3, base.Add(2*time.Minute+30*time.Second), makeToolResult(3, "c", true, "data-c"))
	injectTimelineItem(tl, 1, base.Add(10*time.Second), makeToolResult(1, "a", true, "data-a"))
	injectTimelineItem(tl, 2, base.Add(1*time.Minute), makeToolResult(2, "b", true, "data-b"))

	blocks := tl.GroupByMinutes(3).GetBlocks()
	require.Len(t, blocks, 1)
	require.Len(t, blocks[0].Items, 3)
	require.Equal(t, int64(1), blocks[0].Items[0].GetID())
	require.Equal(t, int64(2), blocks[0].Items[1].GetID())
	require.Equal(t, int64(3), blocks[0].Items[2].GetID())
	require.True(t, blocks[0].Open)
}

// TestGroupByMinutes_MultipleBuckets_OnlyLastOpen 跨多个桶时只有最末桶为 Open
// 关键词: GroupByMinutes 多桶, frozen/open
func TestGroupByMinutes_MultipleBuckets_OnlyLastOpen(t *testing.T) {
	tl := NewTimeline(nil, nil)
	base := time.Date(2026, 5, 2, 10, 0, 0, 0, time.UTC)
	injectTimelineItem(tl, 1, base.Add(30*time.Second), makeToolResult(1, "a", true, "x")) // [10:00,10:03)
	injectTimelineItem(tl, 2, base.Add(4*time.Minute), makeToolResult(2, "b", true, "x"))  // [10:03,10:06)
	injectTimelineItem(tl, 3, base.Add(7*time.Minute), makeToolResult(3, "c", true, "x"))  // [10:06,10:09)

	blocks := tl.GroupByMinutes(3).GetBlocks()
	require.Len(t, blocks, 3)
	require.False(t, blocks[0].Open)
	require.False(t, blocks[1].Open)
	require.True(t, blocks[2].Open)

	// 时间顺序
	require.True(t, blocks[0].BucketStart.Before(blocks[1].BucketStart))
	require.True(t, blocks[1].BucketStart.Before(blocks[2].BucketStart))
}

// TestGroupByMinutes_InvalidMinutes minutes<=0 应返回空 blocks 不 panic
// 关键词: GroupByMinutes 无效参数
func TestGroupByMinutes_InvalidMinutes(t *testing.T) {
	tl := NewTimeline(nil, nil)
	injectTimelineItem(tl, 1, time.Now(), makeToolResult(1, "a", true, "x"))

	require.NotPanics(t, func() {
		g := tl.GroupByMinutes(0)
		require.NotNil(t, g)
		require.Empty(t, g.GetBlocks())
	})
	require.NotPanics(t, func() {
		g := tl.GroupByMinutes(-5)
		require.NotNil(t, g)
		require.Empty(t, g.GetBlocks())
	})
}

// TestGroupByMinutes_DeletedItemsFiltered SoftDelete 后的条目不应出现在 block 中
// 关键词: GroupByMinutes 删除过滤, SoftDelete
func TestGroupByMinutes_DeletedItemsFiltered(t *testing.T) {
	tl := NewTimeline(nil, nil)
	base := time.Date(2026, 5, 2, 10, 0, 0, 0, time.UTC)
	injectTimelineItem(tl, 1, base.Add(10*time.Second), makeToolResult(1, "a", true, "data-a"))
	injectTimelineItem(tl, 2, base.Add(20*time.Second), makeToolResult(2, "b", true, "data-b"))
	injectTimelineItem(tl, 3, base.Add(4*time.Minute), makeToolResult(3, "c", true, "data-c")) // 第二个桶

	tl.SoftDelete(1)

	blocks := tl.GroupByMinutes(3).GetBlocks()
	require.Len(t, blocks, 2)
	// 第一个桶只剩 id=2
	require.Len(t, blocks[0].Items, 1)
	require.Equal(t, int64(2), blocks[0].Items[0].GetID())
	// 第二个桶 id=3
	require.Len(t, blocks[1].Items, 1)
	require.Equal(t, int64(3), blocks[1].Items[0].GetID())

	rendered := blocks.Render("TIMELINE_INTERVAL_GROUP")
	require.NotContains(t, rendered, "data-a", "deleted item content should not appear")
	require.Contains(t, rendered, "data-b")
	require.Contains(t, rendered, "data-c")

	// 整桶全删除则该桶不存在
	tl.SoftDelete(2)
	blocks2 := tl.GroupByMinutes(3).GetBlocks()
	require.Len(t, blocks2, 1, "fully deleted bucket should disappear")
	require.Equal(t, int64(3), blocks2[0].Items[0].GetID())
}

// TestGroupByMinutes_BucketAlignment 桶严格对齐到 N 分钟绝对边界
// 关键词: GroupByMinutes 桶对齐, 边界点归下一桶
func TestGroupByMinutes_BucketAlignment(t *testing.T) {
	tl := NewTimeline(nil, nil)
	loc := time.UTC

	// N=3：10:01:30 -> [10:00,10:03)
	t1 := time.Date(2026, 5, 2, 10, 1, 30, 0, loc)
	// 10:04:00 -> [10:03,10:06)
	t2 := time.Date(2026, 5, 2, 10, 4, 0, 0, loc)
	// 边界点 10:03:00 应进入 [10:03,10:06)
	t3 := time.Date(2026, 5, 2, 10, 3, 0, 0, loc)

	injectTimelineItem(tl, 1, t1, makeToolResult(1, "a", true, "x"))
	injectTimelineItem(tl, 2, t2, makeToolResult(2, "b", true, "x"))
	injectTimelineItem(tl, 3, t3, makeToolResult(3, "c", true, "x"))

	blocks := tl.GroupByMinutes(3).GetBlocks()
	require.Len(t, blocks, 2)

	// 第一个桶
	require.Equal(t, time.Date(2026, 5, 2, 10, 0, 0, 0, loc), blocks[0].BucketStart)
	require.Equal(t, time.Date(2026, 5, 2, 10, 3, 0, 0, loc), blocks[0].BucketEnd)
	require.Len(t, blocks[0].Items, 1)
	require.Equal(t, int64(1), blocks[0].Items[0].GetID())

	// 第二个桶包含边界点 t3 与 t2
	require.Equal(t, time.Date(2026, 5, 2, 10, 3, 0, 0, loc), blocks[1].BucketStart)
	require.Equal(t, time.Date(2026, 5, 2, 10, 6, 0, 0, loc), blocks[1].BucketEnd)
	require.Len(t, blocks[1].Items, 2)
	ids := []int64{blocks[1].Items[0].GetID(), blocks[1].Items[1].GetID()}
	require.Contains(t, ids, int64(2))
	require.Contains(t, ids, int64(3))
}

// TestGroupByMinutes_DaySpan 跨午夜的桶应分别属于不同日
// 关键词: GroupByMinutes 跨日, 桶日期
func TestGroupByMinutes_DaySpan(t *testing.T) {
	tl := NewTimeline(nil, nil)
	loc := time.UTC

	t1 := time.Date(2026, 5, 2, 23, 58, 30, 0, loc)
	t2 := time.Date(2026, 5, 3, 0, 1, 0, 0, loc)
	injectTimelineItem(tl, 1, t1, makeToolResult(1, "a", true, "x"))
	injectTimelineItem(tl, 2, t2, makeToolResult(2, "b", true, "x"))

	blocks := tl.GroupByMinutes(3).GetBlocks()
	require.Len(t, blocks, 2)

	require.Equal(t, 2, blocks[0].BucketStart.Day())
	require.Equal(t, 3, blocks[1].BucketStart.Day())
}

// TestGroupByMinutes_TypeVerbose 三种类型应渲染出对应 [type/verbose]
// 关键词: GroupByMinutes 类型 verbose, ToolResult/UserInteraction/Text
func TestGroupByMinutes_TypeVerbose(t *testing.T) {
	tl := NewTimeline(nil, nil)
	base := time.Date(2026, 5, 2, 10, 0, 30, 0, time.UTC)

	injectTimelineItem(tl, 1, base.Add(10*time.Second), makeToolResult(1, "scanX", true, "tool-data"))
	injectTimelineItem(tl, 2, base.Add(20*time.Second), &UserInteraction{
		ID:              2,
		SystemPrompt:    "sys-q",
		UserExtraPrompt: "user-a",
		Stage:           UserInteractionStage_Review,
	})
	injectTimelineItem(tl, 3, base.Add(30*time.Second), &TextTimelineItem{
		ID:   3,
		Text: "[note] [task:abc]:\n  hello world",
	})
	// 失败的 tool
	injectTimelineItem(tl, 4, base.Add(40*time.Second), makeToolResult(4, "scanY", false, "err-data"))

	blocks := tl.GroupByMinutes(3).GetBlocks()
	require.Len(t, blocks, 1)
	rendered := blocks[0].Render()
	require.Contains(t, rendered, "[tool/scanX ok]")
	require.Contains(t, rendered, "[user/review]")
	require.Contains(t, rendered, "[text/note]")
	require.Contains(t, rendered, "[tool/scanY fail]")
}

// TestGroupByMinutes_TagWrapping 验证 aitag 兼容包裹格式
// 关键词: GroupByMinutes aitag 包裹, TAG_END_<nonce>, 稳定 nonce
func TestGroupByMinutes_TagWrapping(t *testing.T) {
	tl := NewTimeline(nil, nil)
	base := time.Date(2026, 5, 2, 10, 0, 0, 0, time.UTC)
	injectTimelineItem(tl, 1, base.Add(30*time.Second), makeToolResult(1, "a", true, "x")) // 桶1
	injectTimelineItem(tl, 2, base.Add(4*time.Minute), makeToolResult(2, "b", true, "y"))  // 桶2

	blocks := tl.GroupByMinutes(3).GetBlocks()
	require.Len(t, blocks, 2)

	out := blocks.Render("MY_TAG")

	// 每个 block 都应有 <|MY_TAG_<nonce>|> 起始与 <|MY_TAG_END_<nonce>|> 结束
	for _, blk := range blocks {
		nonce := blk.StableNonce()
		require.Contains(t, out, "<|MY_TAG_"+nonce+"|>", "missing start tag for nonce %s", nonce)
		require.Contains(t, out, "<|MY_TAG_END_"+nonce+"|>", "missing end tag for nonce %s", nonce)
	}

	// 数量校验：起始标签数等于 block 数
	require.Equal(t, 2, strings.Count(out, "<|MY_TAG_END_"))

	// bucket 元信息行写在 block 内容中
	require.Equal(t, 2, strings.Count(out, "# bucket="))
	require.Equal(t, 2, strings.Count(out, "interval=3m"))

	// 不应在标签里出现 status / 空格（aitag 不支持）
	require.NotContains(t, out, "<|MY_TAG bucket=")
	require.NotContains(t, out, "status=")

	// 空 aitagName 时应回退为默认
	outDefault := blocks.Render("")
	require.Contains(t, outDefault, "<|TIMELINE_INTERVAL_GROUP_b3t")
	require.Contains(t, outDefault, "<|TIMELINE_INTERVAL_GROUP_END_b3t")
}

// TestGroupByMinutes_ShrinkResultPreferred 当 item 设置 ShrinkResult 后，Render 应优先使用 shrunk 内容
// 关键词: GroupByMinutes ShrinkResult, token 节省
func TestGroupByMinutes_ShrinkResultPreferred(t *testing.T) {
	tl := NewTimeline(nil, nil)
	base := time.Date(2026, 5, 2, 10, 0, 0, 0, time.UTC)
	tr := makeToolResult(1, "a", true, strings.Repeat("verylongtooldatacontent ", 50))
	tr.SetShrinkResult("SHRUNK_PERSIST_X")
	injectTimelineItem(tl, 1, base.Add(10*time.Second), tr)

	blocks := tl.GroupByMinutes(3).GetBlocks()
	require.Len(t, blocks, 1)
	rendered := blocks[0].Render()
	require.Contains(t, rendered, "SHRUNK_PERSIST_X", "should prefer shrink result")
	require.NotContains(t, rendered, "verylongtooldatacontent", "raw long content should not appear")
}

// TestGroupByMinutes_DumpEqualsAllRenderable 验证 Dump 现在就是 GroupByMinutes(3).GetAllRenderable().Render("TIMELINE") 的别名
// 关键词: Dump 等价于 GroupByMinutes, 行为一致, aitag 格式
// 历史背景: 旧 Dump 是行式文本（"--[time]"），与 GroupByMinutes 输出不一致；本次将 Dump 内部统一为 GroupByMinutes
func TestGroupByMinutes_DumpEqualsAllRenderable(t *testing.T) {
	tl := NewTimeline(nil, nil)
	base := time.Date(2026, 5, 2, 10, 0, 0, 0, time.UTC)
	for i := 1; i <= 5; i++ {
		tr := makeToolResult(int64(i), "scan", true, strings.Repeat("noisy_data_chunk_", 200))
		tr.SetShrinkResult("S")
		injectTimelineItem(tl, int64(i), base.Add(time.Duration(i*10)*time.Second), tr)
	}
	dumpRender := tl.Dump()
	groupRender := tl.GroupByMinutes(TimelineDumpDefaultIntervalMinutes).GetAllRenderable().Render(TimelineDumpDefaultAITagName)
	require.Equal(t, groupRender, dumpRender, "Dump must be byte-equal to GroupByMinutes(N).GetAllRenderable().Render(tag)")
	require.NotEmpty(t, dumpRender)
}

// TestGroupByMinutes_PrefixStabilityForCacheHit frozen block 在追加新条目后 Render 文本必须字节级不变
// 这是 GroupByMinutes 缓存友好的核心保证
// 关键词: GroupByMinutes 前缀稳定, 缓存命中
func TestGroupByMinutes_PrefixStabilityForCacheHit(t *testing.T) {
	tl := NewTimeline(nil, nil)
	base := time.Date(2026, 5, 2, 10, 0, 0, 0, time.UTC)
	injectTimelineItem(tl, 1, base.Add(30*time.Second), makeToolResult(1, "a", true, "data-a")) // bucket 1
	injectTimelineItem(tl, 2, base.Add(60*time.Second), makeToolResult(2, "b", true, "data-b")) // bucket 1
	injectTimelineItem(tl, 3, base.Add(4*time.Minute), makeToolResult(3, "c", true, "data-c"))  // bucket 2 (open)

	groups1 := tl.GroupByMinutes(3)
	blocks1 := groups1.GetBlocks()
	require.Len(t, blocks1, 2)

	frozenRender1 := blocks1[0].Render()
	frozenKey1 := blocks1[0].StableKey()

	// 追加更多条目到桶 2 与新桶 3
	injectTimelineItem(tl, 4, base.Add(5*time.Minute), makeToolResult(4, "d", true, "data-d"))
	injectTimelineItem(tl, 5, base.Add(7*time.Minute), makeToolResult(5, "e", true, "data-e"))

	groups2 := tl.GroupByMinutes(3)
	blocks2 := groups2.GetBlocks()
	require.Len(t, blocks2, 3)

	// blocks2[0] 必须是 blocks1[0] 对应的同一桶
	require.True(t, blocks2[0].BucketStart.Equal(blocks1[0].BucketStart))
	require.False(t, blocks2[0].Open, "first bucket should remain frozen")
	require.Equal(t, frozenRender1, blocks2[0].Render(), "frozen block render must be byte-equal across calls")
	require.Equal(t, frozenKey1, blocks2[0].StableKey(), "frozen block stable key must remain stable")

	// 桶 2 也是 frozen 了
	require.False(t, blocks2[1].Open, "second bucket should now be frozen")
	// 只有最末桶 Open
	require.True(t, blocks2[2].Open)

	// 整体渲染前缀稳定（取所有 frozen block 的 Render 拼接）
	prefix1 := blocks1[0].Render()
	prefix2 := blocks2[0].Render()
	require.True(t, strings.HasPrefix(prefix2, prefix1) || prefix2 == prefix1)
}

// TestGroupByMinutes_DeterministicRender 同一 timeline 多次调用产出 byte-equal
// 关键词: GroupByMinutes 确定性, 幂等
func TestGroupByMinutes_DeterministicRender(t *testing.T) {
	tl := NewTimeline(nil, nil)
	base := time.Date(2026, 5, 2, 10, 0, 0, 0, time.UTC)
	for i := 1; i <= 6; i++ {
		injectTimelineItem(tl, int64(i), base.Add(time.Duration(i*40)*time.Second),
			makeToolResult(int64(i), "x", true, "d"))
	}

	r1 := tl.GroupByMinutes(3).GetBlocks().Render("TAG")
	r2 := tl.GroupByMinutes(3).GetBlocks().Render("TAG")
	r3 := tl.GroupByMinutes(3).GetBlocks().Render("TAG")
	require.Equal(t, r1, r2)
	require.Equal(t, r2, r3)
}

// TestGroupByMinutes_GetBlocksReturnsSameSlice 同一 groups 多次 GetBlocks 引用稳定
// 关键词: GroupByMinutes GetBlocks 引用稳定
func TestGroupByMinutes_GetBlocksReturnsSameSlice(t *testing.T) {
	tl := NewTimeline(nil, nil)
	base := time.Date(2026, 5, 2, 10, 0, 0, 0, time.UTC)
	injectTimelineItem(tl, 1, base.Add(30*time.Second), makeToolResult(1, "a", true, "x"))
	injectTimelineItem(tl, 2, base.Add(4*time.Minute), makeToolResult(2, "b", true, "y"))

	groups := tl.GroupByMinutes(3)
	b1 := groups.GetBlocks()
	b2 := groups.GetBlocks()
	require.Equal(t, len(b1), len(b2))
	for i := range b1 {
		require.Same(t, b1[i], b2[i], "block pointer should be reused")
	}
}

// TestGroupByMinutes_OutputFormat 输出格式中应包含 HH:MM:SS 与 [type/verbose]，且 content 行无缩进
// 关键词: GroupByMinutes 输出格式, 紧凑时间, 无缩进
func TestGroupByMinutes_OutputFormat(t *testing.T) {
	tl := NewTimeline(nil, nil)
	ts := time.Date(2026, 5, 2, 10, 1, 30, 0, time.UTC)
	tr := makeToolResult(1, "scan", true, "data-x")
	tr.SetShrinkResult("compact-result")
	injectTimelineItem(tl, 1, ts, tr)

	rendered := tl.GroupByMinutes(3).GetBlocks()[0].Render()
	require.Contains(t, rendered, "10:01:30 [tool/scan ok]")
	// content 行不应带前导空格（节省 token）
	require.Contains(t, rendered, "\ncompact-result")
	require.NotContains(t, rendered, "  compact-result")
}

// TestGroupByMinutes_NoLeadingIndentInContent 验证渲染输出中 content 不带前导缩进
// 关键词: GroupByMinutes 无缩进, token 节省
func TestGroupByMinutes_NoLeadingIndentInContent(t *testing.T) {
	tl := NewTimeline(nil, nil)
	base := time.Date(2026, 5, 2, 10, 0, 0, 0, time.UTC)
	for i := 1; i <= 3; i++ {
		tr := makeToolResult(int64(i), "scan", true, fmt.Sprintf("multi\nline\ndata-%d", i))
		tr.SetShrinkResult(fmt.Sprintf("RES-%d", i))
		injectTimelineItem(tl, int64(i), base.Add(time.Duration(i)*30*time.Second), tr)
	}
	body := tl.GroupByMinutes(3).GetBlocks()[0].Render()

	for _, line := range strings.Split(body, "\n") {
		if line == "" {
			continue
		}
		require.False(t, strings.HasPrefix(line, " "),
			"no rendered line should start with a space (got %q)", line)
		require.False(t, strings.HasPrefix(line, "\t"),
			"no rendered line should start with a tab (got %q)", line)
	}
}

// ---------------------------------------------------------------------------
// frozen boundary 专项测试 (RenderWithFrozenBoundary + Timeline.Dump 集成)
// 验证 §7.7.7 hijacker 切割锚点所依赖的 <|AI_CACHE_FROZEN_semi-dynamic|>
// 边界标签按规则正确插入。
// ---------------------------------------------------------------------------

// frozenStartTag / frozenEndTag 是测试断言用的边界字面量
const (
	frozenStartTag = "<|" + TimelineFrozenBoundaryTagName + "_" + TimelineFrozenBoundaryNonce + "|>"
	frozenEndTag   = "<|" + TimelineFrozenBoundaryTagName + "_END_" + TimelineFrozenBoundaryNonce + "|>"
)

// TestRenderWithFrozenBoundary_AllOpen_NoBoundary 全 open (单个时间桶, 无 reducer)
// 不应当输出 frozen 边界标签, 退化为原 Render 输出。
// 关键词: RenderWithFrozenBoundary, 全 open 退化, 无边界
func TestRenderWithFrozenBoundary_AllOpen_NoBoundary(t *testing.T) {
	tl := NewTimeline(nil, nil)
	base := time.Date(2026, 5, 2, 10, 0, 0, 0, time.UTC)
	injectTimelineItem(tl, 1, base.Add(30*time.Second), makeToolResult(1, "a", true, "data-a"))
	injectTimelineItem(tl, 2, base.Add(60*time.Second), makeToolResult(2, "b", true, "data-b"))

	bs := tl.GroupByMinutes(3).GetAllRenderable()
	out := bs.RenderWithFrozenBoundary("TIMELINE", "", "")
	plain := bs.Render("TIMELINE")

	require.NotContains(t, out, frozenStartTag, "all-open should NOT carry frozen boundary")
	require.NotContains(t, out, frozenEndTag, "all-open should NOT carry frozen boundary END")
	require.Equal(t, plain, out, "all-open: RenderWithFrozenBoundary must equal plain Render")
}

// TestRenderWithFrozenBoundary_AllFrozenReducerOnly 全 frozen (仅 reducer 没有
// interval) 也不加边界 — 整段都是 frozen, 不需要切, hijacker 走退化路径。
// 关键词: RenderWithFrozenBoundary, 全 reducer 退化, 无边界
func TestRenderWithFrozenBoundary_AllFrozenReducerOnly(t *testing.T) {
	bs := TimelineRenderableBlocks{
		&TimelineReducerBlock{ReducerKeyID: 1, Text: "reducer-A"},
		&TimelineReducerBlock{ReducerKeyID: 2, Text: "reducer-B"},
	}
	out := bs.RenderWithFrozenBoundary("TL", "", "")
	plain := bs.Render("TL")

	require.NotContains(t, out, frozenStartTag, "all-frozen should NOT carry boundary")
	require.Equal(t, plain, out, "all-frozen: RenderWithFrozenBoundary must equal plain Render")
}

// TestRenderWithFrozenBoundary_MixFrozenOpen_HasBoundary frozen + open 混合时
// 必须把所有 frozen block 包在 boundary 内, open block 保持在 boundary 外。
// 关键词: RenderWithFrozenBoundary, 混合段, frozen 包裹 + open 在外
func TestRenderWithFrozenBoundary_MixFrozenOpen_HasBoundary(t *testing.T) {
	bs := TimelineRenderableBlocks{
		&TimelineReducerBlock{ReducerKeyID: 1, Text: "reducer-frozen"},
		&fakeFrozenInterval{nonce: "b3t100", body: "frozen-A"},
		&fakeFrozenInterval{nonce: "b3t200", body: "frozen-B"},
		&fakeOpenInterval{nonce: "b3t300", body: "open-tail"},
	}
	out := bs.RenderWithFrozenBoundary("TL", "", "")

	startIdx := strings.Index(out, frozenStartTag)
	endIdx := strings.Index(out, frozenEndTag)
	require.True(t, startIdx >= 0, "must contain frozen boundary START")
	require.True(t, endIdx > startIdx, "frozen boundary END must follow START")

	// frozen 段内必须含 reducer-frozen / frozen-A / frozen-B
	frozenPart := out[startIdx:endIdx]
	require.Contains(t, frozenPart, "reducer-frozen")
	require.Contains(t, frozenPart, "frozen-A")
	require.Contains(t, frozenPart, "frozen-B")
	require.NotContains(t, frozenPart, "open-tail", "open must NOT be inside frozen boundary")

	// boundary END 之后必须含 open-tail
	tailPart := out[endIdx:]
	require.Contains(t, tailPart, "open-tail")
}

// TestRenderWithFrozenBoundary_ByteStableOnFrozenChange frozen 段内容不变时,
// open 段变化不影响 boundary 内字节序列 (这是 prefix cache 命中的核心前置)。
// 关键词: RenderWithFrozenBoundary, 字节稳定, frozen 不受 open 变化影响
func TestRenderWithFrozenBoundary_ByteStableOnFrozenChange(t *testing.T) {
	frozen := []TimelineRenderableBlock{
		&TimelineReducerBlock{ReducerKeyID: 1, Text: "stable-reducer"},
		&fakeFrozenInterval{nonce: "b3t100", body: "stable-frozen"},
	}
	r1 := append(append(TimelineRenderableBlocks{}, frozen...),
		&fakeOpenInterval{nonce: "b3t999", body: "open-r1"}).
		RenderWithFrozenBoundary("TL", "", "")
	r2 := append(append(TimelineRenderableBlocks{}, frozen...),
		&fakeOpenInterval{nonce: "b3t999", body: "open-r2-different"}).
		RenderWithFrozenBoundary("TL", "", "")

	r1Frozen := r1[strings.Index(r1, frozenStartTag):strings.Index(r1, frozenEndTag)+len(frozenEndTag)]
	r2Frozen := r2[strings.Index(r2, frozenStartTag):strings.Index(r2, frozenEndTag)+len(frozenEndTag)]
	require.Equal(t, r1Frozen, r2Frozen,
		"frozen boundary content must be byte-stable when frozen blocks unchanged")
	require.NotEqual(t, r1, r2, "overall output should differ on open change")
}

// TestRenderWithFrozenBoundary_CustomBoundaryName 自定义 boundary tag/nonce
// 应当被尊重, 不强制使用包级默认。
// 关键词: RenderWithFrozenBoundary, 自定义 boundary 标签
func TestRenderWithFrozenBoundary_CustomBoundaryName(t *testing.T) {
	bs := TimelineRenderableBlocks{
		&TimelineReducerBlock{ReducerKeyID: 1, Text: "R"},
		&fakeOpenInterval{nonce: "b3t1", body: "O"},
	}
	out := bs.RenderWithFrozenBoundary("TL", "MY_FROZEN", "test-segment")
	require.Contains(t, out, "<|MY_FROZEN_test-segment|>")
	require.Contains(t, out, "<|MY_FROZEN_END_test-segment|>")
	require.NotContains(t, out, frozenStartTag, "must not use default boundary tag when custom provided")
}

// TestTimelineDump_HasFrozenBoundary_OnMixedTimeline 用 Timeline.Dump 端到端
// 验证: timeline 含多个时间桶 (frozen + open) 时, Dump 输出必须自带 frozen
// 边界标签, 这是 hijacker 3 段拆分的核心前提。
// 关键词: Timeline.Dump, frozen boundary 端到端, 多时间桶
func TestTimelineDump_HasFrozenBoundary_OnMixedTimeline(t *testing.T) {
	tl := NewTimeline(nil, nil)
	base := time.Date(2026, 5, 2, 10, 0, 0, 0, time.UTC)
	// bucket 1: 10:00-10:03 (frozen 后续)
	injectTimelineItem(tl, 1, base.Add(30*time.Second), makeToolResult(1, "a", true, "data-a"))
	injectTimelineItem(tl, 2, base.Add(60*time.Second), makeToolResult(2, "b", true, "data-b"))
	// bucket 2: 10:03-10:06 (frozen)
	injectTimelineItem(tl, 3, base.Add(4*time.Minute), makeToolResult(3, "c", true, "data-c"))
	// bucket 3: 10:06-10:09 (open, 末桶)
	injectTimelineItem(tl, 4, base.Add(7*time.Minute), makeToolResult(4, "d", true, "data-d"))

	dump := tl.Dump()
	require.Contains(t, dump, frozenStartTag, "Dump with multi-bucket timeline must inject frozen boundary")
	require.Contains(t, dump, frozenEndTag, "Dump must inject frozen boundary END")

	startIdx := strings.Index(dump, frozenStartTag)
	endIdx := strings.Index(dump, frozenEndTag)
	frozenPart := dump[startIdx:endIdx]
	require.Contains(t, frozenPart, "data-a")
	require.Contains(t, frozenPart, "data-b")
	require.Contains(t, frozenPart, "data-c")
	require.NotContains(t, frozenPart, "data-d", "data-d (open bucket) must be outside frozen boundary")

	tailPart := dump[endIdx+len(frozenEndTag):]
	require.Contains(t, tailPart, "data-d")
}

// TestTimelineDump_NoFrozenBoundary_OnSingleBucket 单时间桶 (全 open) 场景
// Timeline.Dump 应当退化为原 Render 输出, 不带 frozen 边界。
// 关键词: Timeline.Dump, 全 open 退化, 无边界
func TestTimelineDump_NoFrozenBoundary_OnSingleBucket(t *testing.T) {
	tl := NewTimeline(nil, nil)
	base := time.Date(2026, 5, 2, 10, 0, 0, 0, time.UTC)
	injectTimelineItem(tl, 1, base.Add(30*time.Second), makeToolResult(1, "a", true, "data-a"))
	injectTimelineItem(tl, 2, base.Add(60*time.Second), makeToolResult(2, "b", true, "data-b"))

	dump := tl.Dump()
	require.NotContains(t, dump, frozenStartTag,
		"single-bucket (all-open) Dump should NOT inject frozen boundary")
}

// fakeFrozenInterval / fakeOpenInterval 是测试用的最小 RenderableBlock 实现,
// 用来在不依赖完整 Timeline 构造的情况下精确控制 IsOpen 与 body 字面量。
type fakeFrozenInterval struct {
	nonce, body string
}

func (f *fakeFrozenInterval) Render() string      { return f.body }
func (f *fakeFrozenInterval) StableNonce() string { return f.nonce }
func (f *fakeFrozenInterval) IsOpen() bool        { return false }

type fakeOpenInterval struct {
	nonce, body string
}

func (f *fakeOpenInterval) Render() string      { return f.body }
func (f *fakeOpenInterval) StableNonce() string { return f.nonce }
func (f *fakeOpenInterval) IsOpen() bool        { return true }
