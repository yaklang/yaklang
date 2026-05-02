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

// TestGroupByMinutes_RenderShorterThanDump 设置 ShrinkResult 后 Render(tag) 应明显短于 Dump
// 关键词: GroupByMinutes 体积优化, vs Dump
func TestGroupByMinutes_RenderShorterThanDump(t *testing.T) {
	tl := NewTimeline(nil, nil)
	base := time.Date(2026, 5, 2, 10, 0, 0, 0, time.UTC)
	for i := 1; i <= 5; i++ {
		tr := makeToolResult(int64(i), "scan", true, strings.Repeat("noisy_data_chunk_", 200))
		tr.SetShrinkResult("S")
		injectTimelineItem(tl, int64(i), base.Add(time.Duration(i*10)*time.Second), tr)
	}
	groupRender := tl.GroupByMinutes(3).GetBlocks().Render("TIMELINE_INTERVAL_GROUP")
	dumpRender := tl.Dump()
	require.Greater(t, len(dumpRender), len(groupRender), "group-render should be shorter than dump when shrunk")
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
