package aicommon

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/aitag"
)

// =============================================================================
// 第一组：aitag.SplitViaTAG 集成
// 验证 GroupByMinutes 输出可被 aitag splitter 准确切分，每个 tagged block 与
// TimelineIntervalBlock 一一对应；nonce / Content 行为与缓存假设一致
// =============================================================================

// TestGroupByMinutes_AITag_SplitOneToOne 拆出的 tagged block 数量与 GetBlocks 一致
// 关键词: GroupByMinutes aitag SplitViaTAG, 一对一
func TestGroupByMinutes_AITag_SplitOneToOne(t *testing.T) {
	tl := NewTimeline(nil, nil)
	base := time.Date(2026, 5, 2, 10, 0, 0, 0, time.UTC)
	for i := 1; i <= 7; i++ {
		injectTimelineItem(tl, int64(i), base.Add(time.Duration(i*40)*time.Second),
			makeToolResult(int64(i), "scan", true, fmt.Sprintf("d-%d", i)))
	}

	blocks := tl.GroupByMinutes(3).GetBlocks()
	require.Greater(t, len(blocks), 0)

	prompt := blocks.Render("TIMELINE_INTERVAL_GROUP")
	result, err := aitag.SplitViaTAG(prompt, "TIMELINE_INTERVAL_GROUP")
	require.NoError(t, err)

	tagged := result.GetTaggedBlocks()
	require.Equal(t, len(blocks), len(tagged), "tagged block count must match block count")

	// 每个 tagged block 的 TagName 都正确，nonce 来自 StableNonce()
	for i, tb := range tagged {
		require.Equal(t, "TIMELINE_INTERVAL_GROUP", tb.TagName)
		require.Equal(t, blocks[i].StableNonce(), tb.Nonce, "nonce mismatch at block %d", i)
	}
}

// TestGroupByMinutes_AITag_ContentMatchesBlockRender splitter 拆出的 Content 字节级等于 block.Render()
// 关键词: GroupByMinutes aitag SplitViaTAG, Content 等价
func TestGroupByMinutes_AITag_ContentMatchesBlockRender(t *testing.T) {
	tl := NewTimeline(nil, nil)
	base := time.Date(2026, 5, 2, 10, 0, 0, 0, time.UTC)
	injectTimelineItem(tl, 1, base.Add(20*time.Second), makeToolResult(1, "a", true, "x"))
	injectTimelineItem(tl, 2, base.Add(70*time.Second), makeToolResult(2, "b", false, "y"))
	injectTimelineItem(tl, 3, base.Add(4*time.Minute), makeToolResult(3, "c", true, "z"))

	blocks := tl.GroupByMinutes(3).GetBlocks()
	prompt := blocks.Render("MY_TIMELINE_TAG")
	result, err := aitag.SplitViaTAG(prompt, "MY_TIMELINE_TAG")
	require.NoError(t, err)

	tagged := result.GetTaggedBlocks()
	require.Len(t, tagged, len(blocks))
	for i := range blocks {
		require.Equal(t, blocks[i].Render(), tagged[i].Content,
			"splitter content must equal block.Render() at index %d", i)
	}
}

// TestGroupByMinutes_AITag_RoundTrip splitter 还原原文与 Render 结果一致
// 关键词: GroupByMinutes aitag SplitViaTAG, round-trip
func TestGroupByMinutes_AITag_RoundTrip(t *testing.T) {
	tl := NewTimeline(nil, nil)
	base := time.Date(2026, 5, 2, 10, 0, 0, 0, time.UTC)
	for i := 1; i <= 5; i++ {
		injectTimelineItem(tl, int64(i), base.Add(time.Duration(i)*70*time.Second),
			makeToolResult(int64(i), "scan", true, "d"))
	}
	prompt := tl.GroupByMinutes(3).GetBlocks().Render("TG")

	result, err := aitag.SplitViaTAG(prompt, "TG")
	require.NoError(t, err)
	require.Equal(t, prompt, result.String(), "round-trip via splitter must reproduce input")
}

// TestGroupByMinutes_AITag_StreamingParse aitag.Parse 流式解析能命中所有 block
// 关键词: GroupByMinutes aitag.Parse, 流式回调
func TestGroupByMinutes_AITag_StreamingParse(t *testing.T) {
	tl := NewTimeline(nil, nil)
	base := time.Date(2026, 5, 2, 10, 0, 0, 0, time.UTC)
	for i := 1; i <= 6; i++ {
		injectTimelineItem(tl, int64(i), base.Add(time.Duration(i)*45*time.Second),
			makeToolResult(int64(i), "scan", true, fmt.Sprintf("payload-%d", i)))
	}
	blocks := tl.GroupByMinutes(3).GetBlocks()
	prompt := blocks.Render("TIMELINE_INTERVAL_GROUP")

	var hitCount int32
	options := []aitag.ParseOption{}
	for _, blk := range blocks {
		nonce := blk.StableNonce()
		expected := blk.Render()
		options = append(options, aitag.WithCallback("TIMELINE_INTERVAL_GROUP", nonce,
			func(reader io.Reader) {
				data, _ := io.ReadAll(reader)
				if string(data) == expected {
					atomic.AddInt32(&hitCount, 1)
				}
			}))
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	doneCh := make(chan error, 1)
	go func() {
		doneCh <- aitag.Parse(strings.NewReader(prompt), options...)
	}()
	select {
	case err := <-doneCh:
		require.NoError(t, err)
	case <-ctx.Done():
		t.Fatal("aitag.Parse timed out")
	}

	require.Equal(t, int32(len(blocks)), atomic.LoadInt32(&hitCount),
		"every block should be hit by streaming parse")
}

// TestGroupByMinutes_AITag_NonceIsStable 同一桶在不同次 GroupByMinutes 中产生相同 nonce
// 关键词: GroupByMinutes StableNonce, 跨调用稳定
func TestGroupByMinutes_AITag_NonceIsStable(t *testing.T) {
	tl := NewTimeline(nil, nil)
	base := time.Date(2026, 5, 2, 10, 0, 0, 0, time.UTC)
	injectTimelineItem(tl, 1, base.Add(30*time.Second), makeToolResult(1, "a", true, "x"))
	injectTimelineItem(tl, 2, base.Add(4*time.Minute), makeToolResult(2, "b", true, "y"))

	a := tl.GroupByMinutes(3).GetBlocks()
	b := tl.GroupByMinutes(3).GetBlocks()
	require.Equal(t, len(a), len(b))
	for i := range a {
		require.Equal(t, a[i].StableNonce(), b[i].StableNonce())
	}

	// 不同 interval 必定产生不同 nonce
	c := tl.GroupByMinutes(5).GetBlocks()
	require.NotEmpty(t, c)
	require.NotEqual(t, a[0].StableNonce(), c[0].StableNonce(),
		"nonce must include interval to avoid collision")
}

// =============================================================================
// 第二组：缓存命中率定量分析
// 模拟真实使用：timeline 增量增长，比较两次相邻 Render 的公共前缀
// =============================================================================

// commonPrefixLen 计算两个字符串的最长公共前缀字节数
func commonPrefixLen(a, b string) int {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	for i := 0; i < n; i++ {
		if a[i] != b[i] {
			return i
		}
	}
	return n
}

// TestGroupByMinutes_CacheHitRatio_AppendNewBucket 追加新桶后旧桶完全 cache hit
// 关键词: GroupByMinutes 缓存命中率, 新桶追加
func TestGroupByMinutes_CacheHitRatio_AppendNewBucket(t *testing.T) {
	tl := NewTimeline(nil, nil)
	base := time.Date(2026, 5, 2, 10, 0, 0, 0, time.UTC)
	// 4 个完整桶
	for i := 1; i <= 8; i++ {
		injectTimelineItem(tl, int64(i), base.Add(time.Duration(i)*45*time.Second),
			makeToolResult(int64(i), "scan", true, fmt.Sprintf("d%d", i)))
	}
	r1 := tl.GroupByMinutes(3).GetBlocks().Render("TG")

	// 在一个全新桶里追加 1 条
	injectTimelineItem(tl, 9, base.Add(13*time.Minute), makeToolResult(9, "newbucket", true, "z"))
	r2 := tl.GroupByMinutes(3).GetBlocks().Render("TG")

	cp := commonPrefixLen(r1, r2)
	ratio := float64(cp) / float64(len(r1))
	t.Logf("append-new-bucket: prefix=%d / first=%d ratio=%.3f", cp, len(r1), ratio)
	require.Equal(t, len(r1), cp, "appending into a brand-new bucket must not invalidate any prior bytes")
	require.Greater(t, len(r2), len(r1), "second render must be longer (added data)")
	require.Equal(t, 1.0, ratio, "cache hit upper bound should be 100%% when only new bucket appended")
}

// TestGroupByMinutes_CacheHitRatio_AppendIntoOpenBucket 在 open 桶追加只让最后段失效
// 关键词: GroupByMinutes 缓存命中率, open 桶追加
func TestGroupByMinutes_CacheHitRatio_AppendIntoOpenBucket(t *testing.T) {
	tl := NewTimeline(nil, nil)
	base := time.Date(2026, 5, 2, 10, 0, 0, 0, time.UTC)
	// 8 条目跨 ~4 个桶
	for i := 1; i <= 8; i++ {
		injectTimelineItem(tl, int64(i), base.Add(time.Duration(i)*45*time.Second),
			makeToolResult(int64(i), "scan", true, fmt.Sprintf("data-content-%d", i)))
	}
	blocks1 := tl.GroupByMinutes(3).GetBlocks()
	r1 := blocks1.Render("TG")

	// 找到最后一个桶（open）的起始位置；之前的字节应该全都命中
	lastNonce := blocks1[len(blocks1)-1].StableNonce()
	lastTagPos := strings.Index(r1, "<|TG_"+lastNonce+"|>")
	require.GreaterOrEqual(t, lastTagPos, 0)

	// 向最后一个桶（已经是 open）追加一条
	lastBucket := blocks1[len(blocks1)-1]
	tInOpen := lastBucket.BucketStart.Add(30 * time.Second)
	injectTimelineItem(tl, 100, tInOpen, makeToolResult(100, "extra", true, "extra-data"))

	r2 := tl.GroupByMinutes(3).GetBlocks().Render("TG")
	cp := commonPrefixLen(r1, r2)
	ratio := float64(cp) / float64(len(r1))
	t.Logf("append-open-bucket: prefix=%d / first=%d ratio=%.3f", cp, len(r1), ratio)

	// 至少前面所有 frozen block 必须 byte-equal
	require.GreaterOrEqual(t, cp, lastTagPos,
		"frozen prefix bytes must be preserved when appending into open bucket")
	require.Greater(t, ratio, 0.5, "cache hit upper bound should exceed 50%%")
}

// TestGroupByMinutes_CacheHitRatio_VariousIntervals 多种 interval 下缓存收益均显著
// 关键词: GroupByMinutes 缓存命中率, interval 1/3/5/10/60
func TestGroupByMinutes_CacheHitRatio_VariousIntervals(t *testing.T) {
	intervals := []int{1, 3, 5, 10, 60}
	for _, interval := range intervals {
		t.Run(fmt.Sprintf("interval=%dm", interval), func(t *testing.T) {
			tl := NewTimeline(nil, nil)
			base := time.Date(2026, 5, 2, 10, 0, 0, 0, time.UTC)
			// 跨 ~6 个 interval 桶
			for i := 1; i <= 12; i++ {
				ts := base.Add(time.Duration(i*interval/2) * time.Minute).Add(time.Duration(i*7) * time.Second)
				injectTimelineItem(tl, int64(i), ts,
					makeToolResult(int64(i), "scan", true, fmt.Sprintf("d-%d", i)))
			}
			r1 := tl.GroupByMinutes(interval).GetBlocks().Render("TG")
			require.NotEmpty(t, r1)

			// 在最末桶之后追加一条新桶
			extraTs := base.Add(time.Duration(20*interval) * time.Minute)
			injectTimelineItem(tl, 999, extraTs, makeToolResult(999, "extra", true, "x"))
			r2 := tl.GroupByMinutes(interval).GetBlocks().Render("TG")

			cp := commonPrefixLen(r1, r2)
			ratio := float64(cp) / float64(len(r1))
			t.Logf("interval=%dm cache-hit ratio = %.3f (cp=%d / r1=%d)", interval, ratio, cp, len(r1))
			require.Equal(t, len(r1), cp,
				"appending entirely new bucket must keep prior bytes")
			require.Equal(t, 1.0, ratio)
		})
	}
}

// TestGroupByMinutes_CacheHitRatio_GrowingTimeline 长期增长场景累积缓存命中率
// 关键词: GroupByMinutes 缓存命中率, 长期增长
func TestGroupByMinutes_CacheHitRatio_GrowingTimeline(t *testing.T) {
	tl := NewTimeline(nil, nil)
	base := time.Date(2026, 5, 2, 10, 0, 0, 0, time.UTC)

	var prevRender string
	var totalSaved, totalRendered int64

	for round := 1; round <= 20; round++ {
		injectTimelineItem(tl, int64(round),
			base.Add(time.Duration(round)*30*time.Second),
			makeToolResult(int64(round), "scan", true, fmt.Sprintf("payload-%d", round)))

		current := tl.GroupByMinutes(3).GetBlocks().Render("TG")
		if prevRender != "" {
			cp := commonPrefixLen(prevRender, current)
			totalSaved += int64(cp)
			totalRendered += int64(len(current))
		}
		prevRender = current
	}

	// 20 轮后累积命中字节数与累积渲染字节数比例
	overall := float64(totalSaved) / float64(totalRendered)
	t.Logf("growing-timeline overall cache-hit ratio = %.3f (saved=%d / total=%d)",
		overall, totalSaved, totalRendered)
	require.Greater(t, overall, 0.5,
		"average cache-hit ratio across rounds should exceed 50%%")
}

// =============================================================================
// 第三组：复杂边界
// =============================================================================

// TestGroupByMinutes_Boundary_SameSecondMultipleItems 同一秒多条目应保留全部并按 id 排序
// 关键词: GroupByMinutes 同秒多条目
func TestGroupByMinutes_Boundary_SameSecondMultipleItems(t *testing.T) {
	tl := NewTimeline(nil, nil)
	ts := time.Date(2026, 5, 2, 10, 0, 30, 0, time.UTC)
	for i := 1; i <= 5; i++ {
		// 微小毫秒偏移，避免 UnixMilli 完全相同
		injectTimelineItem(tl, int64(i), ts.Add(time.Duration(i)*time.Millisecond),
			makeToolResult(int64(i), "x", true, fmt.Sprintf("d%d", i)))
	}
	blocks := tl.GroupByMinutes(3).GetBlocks()
	require.Len(t, blocks, 1)
	require.Len(t, blocks[0].Items, 5)
	for i := 1; i <= 5; i++ {
		require.Equal(t, int64(i), blocks[0].Items[i-1].GetID())
	}
}

// TestGroupByMinutes_Boundary_HourSpan 跨小时边界正确切分
// 关键词: GroupByMinutes 跨小时
func TestGroupByMinutes_Boundary_HourSpan(t *testing.T) {
	tl := NewTimeline(nil, nil)
	loc := time.UTC
	t1 := time.Date(2026, 5, 2, 10, 58, 30, 0, loc)
	t2 := time.Date(2026, 5, 2, 11, 1, 0, 0, loc)
	injectTimelineItem(tl, 1, t1, makeToolResult(1, "a", true, "x"))
	injectTimelineItem(tl, 2, t2, makeToolResult(2, "b", true, "y"))

	blocks := tl.GroupByMinutes(3).GetBlocks()
	require.Len(t, blocks, 2)
	require.Equal(t, 10, blocks[0].BucketStart.Hour())
	require.Equal(t, 11, blocks[1].BucketStart.Hour())
}

// TestGroupByMinutes_Boundary_LargeTimeline 100+ 条目下结构与确定性
// 关键词: GroupByMinutes 大量条目, 性能与稳定
func TestGroupByMinutes_Boundary_LargeTimeline(t *testing.T) {
	tl := NewTimeline(nil, nil)
	base := time.Date(2026, 5, 2, 10, 0, 0, 0, time.UTC)
	for i := 1; i <= 150; i++ {
		injectTimelineItem(tl, int64(i),
			base.Add(time.Duration(i)*15*time.Second),
			makeToolResult(int64(i), "scan", true, fmt.Sprintf("d-%d", i)))
	}
	blocks := tl.GroupByMinutes(3).GetBlocks()
	require.Greater(t, len(blocks), 5)

	totalItems := 0
	for _, blk := range blocks {
		totalItems += len(blk.Items)
	}
	require.Equal(t, 150, totalItems)

	// 渲染两次必须 byte-equal（决定性）
	r1 := blocks.Render("TG")
	r2 := tl.GroupByMinutes(3).GetBlocks().Render("TG")
	require.Equal(t, r1, r2)
}

// TestGroupByMinutes_Boundary_LargeInterval interval=60 分钟下行为正确
// 关键词: GroupByMinutes 大 interval
func TestGroupByMinutes_Boundary_LargeInterval(t *testing.T) {
	tl := NewTimeline(nil, nil)
	loc := time.UTC
	t1 := time.Date(2026, 5, 2, 10, 35, 0, 0, loc)
	t2 := time.Date(2026, 5, 2, 11, 5, 0, 0, loc)
	t3 := time.Date(2026, 5, 2, 11, 55, 0, 0, loc)
	injectTimelineItem(tl, 1, t1, makeToolResult(1, "a", true, "x"))
	injectTimelineItem(tl, 2, t2, makeToolResult(2, "b", true, "y"))
	injectTimelineItem(tl, 3, t3, makeToolResult(3, "c", true, "z"))

	blocks := tl.GroupByMinutes(60).GetBlocks()
	require.Len(t, blocks, 2)
	require.Equal(t, time.Date(2026, 5, 2, 10, 0, 0, 0, loc), blocks[0].BucketStart)
	require.Equal(t, time.Date(2026, 5, 2, 11, 0, 0, 0, loc), blocks[1].BucketStart)
	require.Len(t, blocks[1].Items, 2)
}

// TestGroupByMinutes_Boundary_RepeatedContent 设置 ShrinkResult 后 Render 不应含原始大数据
// 关键词: GroupByMinutes 重复内容, ShrinkResult 节省
func TestGroupByMinutes_Boundary_RepeatedContent(t *testing.T) {
	tl := NewTimeline(nil, nil)
	base := time.Date(2026, 5, 2, 10, 0, 0, 0, time.UTC)
	bigData := strings.Repeat("LARGE_RAW_DATA_CHUNK_", 200)
	for i := 1; i <= 6; i++ {
		tr := makeToolResult(int64(i), "scan", true, bigData)
		tr.SetShrinkResult("SHRUNK_X")
		injectTimelineItem(tl, int64(i), base.Add(time.Duration(i)*30*time.Second), tr)
	}

	groupRender := tl.GroupByMinutes(3).GetBlocks().Render("TG")
	require.NotContains(t, groupRender, "LARGE_RAW_DATA_CHUNK_",
		"shrunk content must replace raw content in render")
	require.Contains(t, groupRender, "SHRUNK_X")

	// 设置 shrink 后 Render 应明显小于 6 条目原始数据总和
	require.Less(t, len(groupRender), len(bigData),
		"render must be smaller than even one raw data chunk")
}

// TestGroupByMinutes_Boundary_RepeatedContent_NoShrinkVsShrink 对比有/无 shrink 时 render 体积
// 关键词: GroupByMinutes ShrinkResult 量化收益
func TestGroupByMinutes_Boundary_RepeatedContent_NoShrinkVsShrink(t *testing.T) {
	build := func(useShrink bool) *Timeline {
		tl := NewTimeline(nil, nil)
		base := time.Date(2026, 5, 2, 10, 0, 0, 0, time.UTC)
		bigData := strings.Repeat("RAW_X_", 100)
		for i := 1; i <= 6; i++ {
			tr := makeToolResult(int64(i), "scan", true, bigData)
			if useShrink {
				tr.SetShrinkResult("S")
			}
			injectTimelineItem(tl, int64(i), base.Add(time.Duration(i)*30*time.Second), tr)
		}
		return tl
	}
	noShrink := build(false).GroupByMinutes(3).GetBlocks().Render("TG")
	withShrink := build(true).GroupByMinutes(3).GetBlocks().Render("TG")
	t.Logf("noShrink=%d withShrink=%d savings=%.2fx",
		len(noShrink), len(withShrink),
		float64(len(noShrink))/float64(len(withShrink)))
	require.Greater(t, len(noShrink), len(withShrink)*3,
		"shrink result should reduce render size by >3x for repeated raw payloads")
}

// TestGroupByMinutes_Boundary_GapBuckets 桶之间的空洞不影响后续桶 nonce
// 关键词: GroupByMinutes 空洞桶
func TestGroupByMinutes_Boundary_GapBuckets(t *testing.T) {
	tl := NewTimeline(nil, nil)
	base := time.Date(2026, 5, 2, 10, 0, 0, 0, time.UTC)
	// 桶 [10:00,10:03)
	injectTimelineItem(tl, 1, base.Add(30*time.Second), makeToolResult(1, "a", true, "x"))
	// 跳过 [10:03,10:06) 与 [10:06,10:09)
	// 桶 [10:09,10:12)
	injectTimelineItem(tl, 2, base.Add(10*time.Minute), makeToolResult(2, "b", true, "y"))

	blocks := tl.GroupByMinutes(3).GetBlocks()
	require.Len(t, blocks, 2, "gap buckets must not appear")

	expectedFirstNonce := fmt.Sprintf("b3t%d", base.Unix())
	require.Equal(t, expectedFirstNonce, blocks[0].StableNonce())

	expectedSecondStart := time.Date(2026, 5, 2, 10, 9, 0, 0, time.UTC)
	require.Equal(t, expectedSecondStart, blocks[1].BucketStart)
}

// TestGroupByMinutes_Boundary_AllItemsInOpenBucket 所有 item 都在同一桶 → 1 block + Open
// 关键词: GroupByMinutes 全部 open
func TestGroupByMinutes_Boundary_AllItemsInOpenBucket(t *testing.T) {
	tl := NewTimeline(nil, nil)
	base := time.Date(2026, 5, 2, 10, 0, 0, 0, time.UTC)
	for i := 1; i <= 10; i++ {
		injectTimelineItem(tl, int64(i), base.Add(time.Duration(i)*5*time.Second),
			makeToolResult(int64(i), "x", true, "d"))
	}
	blocks := tl.GroupByMinutes(3).GetBlocks()
	require.Len(t, blocks, 1)
	require.True(t, blocks[0].Open)
	require.Len(t, blocks[0].Items, 10)
}

// TestGroupByMinutes_Boundary_MixedTypesInBucket 同一桶内混合类型条目
// 关键词: GroupByMinutes 混合类型
func TestGroupByMinutes_Boundary_MixedTypesInBucket(t *testing.T) {
	tl := NewTimeline(nil, nil)
	base := time.Date(2026, 5, 2, 10, 0, 0, 0, time.UTC)
	injectTimelineItem(tl, 1, base.Add(10*time.Second), makeToolResult(1, "a", true, "tool-data"))
	injectTimelineItem(tl, 2, base.Add(20*time.Second), &UserInteraction{
		ID: 2, Stage: UserInteractionStage_FreeInput,
		SystemPrompt: "q", UserExtraPrompt: "user-ans",
	})
	injectTimelineItem(tl, 3, base.Add(30*time.Second), &TextTimelineItem{ID: 3, Text: "[note]:\n  text-content"})

	blocks := tl.GroupByMinutes(3).GetBlocks()
	require.Len(t, blocks, 1)
	body := blocks[0].Render()
	require.Contains(t, body, "[tool/a ok]")
	require.Contains(t, body, "[user/free_input]")
	require.Contains(t, body, "[text/note]")

	// aitag 切分仍正确
	prompt := blocks.Render("TG")
	res, err := aitag.SplitViaTAG(prompt, "TG")
	require.NoError(t, err)
	require.Len(t, res.GetTaggedBlocks(), 1)
	require.Equal(t, body, res.GetTaggedBlocks()[0].Content)
}

// TestGroupByMinutes_Boundary_BucketReorderProof timeline 内插入顺序与时间顺序不一致时仍按时间分桶
// 关键词: GroupByMinutes 乱序插入
func TestGroupByMinutes_Boundary_BucketReorderProof(t *testing.T) {
	tl := NewTimeline(nil, nil)
	base := time.Date(2026, 5, 2, 10, 0, 0, 0, time.UTC)
	// 故意按 id 倒序、时间正序插入（由调用方决定 ts，所以是合法的）
	injectTimelineItem(tl, 30, base.Add(7*time.Minute), makeToolResult(30, "c", true, "z"))
	injectTimelineItem(tl, 20, base.Add(4*time.Minute), makeToolResult(20, "b", true, "y"))
	injectTimelineItem(tl, 10, base.Add(30*time.Second), makeToolResult(10, "a", true, "x"))

	blocks := tl.GroupByMinutes(3).GetBlocks()
	require.Len(t, blocks, 3)
	// 时间从小到大
	require.True(t, blocks[0].BucketStart.Before(blocks[1].BucketStart))
	require.True(t, blocks[1].BucketStart.Before(blocks[2].BucketStart))
}

// TestGroupByMinutes_AITag_SplitWithLeadingTrailingText splitter 能处理 prompt 前后有非标签文本
// 关键词: GroupByMinutes aitag SplitViaTAG, 前后带文本
func TestGroupByMinutes_AITag_SplitWithLeadingTrailingText(t *testing.T) {
	tl := NewTimeline(nil, nil)
	base := time.Date(2026, 5, 2, 10, 0, 0, 0, time.UTC)
	for i := 1; i <= 4; i++ {
		injectTimelineItem(tl, int64(i), base.Add(time.Duration(i)*45*time.Second),
			makeToolResult(int64(i), "scan", true, "d"))
	}
	body := tl.GroupByMinutes(3).GetBlocks().Render("TG")

	prompt := "PREFIX-LINE\n" + body + "\nSUFFIX-LINE"
	res, err := aitag.SplitViaTAG(prompt, "TG")
	require.NoError(t, err)

	// 至少一个 text 块（前缀） + N 个 tagged + 至少一个 text 块（后缀）
	tagged := res.GetTaggedBlocks()
	textBlocks := res.GetTextBlocks()
	require.GreaterOrEqual(t, len(tagged), 2)
	require.GreaterOrEqual(t, len(textBlocks), 1)

	// round-trip
	var sb bytes.Buffer
	for _, b := range res.GetOrderedBlocks() {
		sb.WriteString(b.Raw)
	}
	require.Equal(t, prompt, sb.String())
}
