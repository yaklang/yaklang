package aicommon

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/linktable"
)

// 关键词: byte bucket, 字节子桶测试

func TestByteBucket_EstimateMatchesRender(t *testing.T) {
	tl := NewTimeline(nil, nil)
	base := time.Date(2026, 5, 2, 10, 0, 30, 0, time.UTC)
	injectTimelineItem(tl, 1, base, makeToolResult(1, "a", true, "line1\nline2"))
	injectTimelineItem(tl, 2, base.Add(5*time.Second), makeToolResult(2, "b", true, "x"))

	blocks := tl.GroupByMinutesAndBytes(3, 20000).GetBlocks()
	require.GreaterOrEqual(t, len(blocks), 1)
	for _, b := range blocks {
		est := intervalBlockHeaderByteLen(b.BucketStart, b.BucketEnd, b.IntervalMinutes)
		first := true
		for _, it := range b.Items {
			est += timelineEntryAppendByteLen(it, b.BucketStart, first)
			first = false
		}
		rendered := b.Render()
		require.Equal(t, len(rendered), est, "estimate must match Render byte length for block")
	}
}

func TestByteBucket_NoSplitWhenUnderK(t *testing.T) {
	tl := NewTimeline(nil, nil)
	base := time.Date(2026, 5, 2, 10, 0, 0, 0, time.UTC)
	injectTimelineItem(tl, 1, base.Add(10*time.Second), makeToolResult(1, "a", true, "short"))
	injectTimelineItem(tl, 2, base.Add(20*time.Second), makeToolResult(2, "b", true, "short2"))

	blocks := tl.GroupByMinutesAndBytes(3, 64*1024).GetBlocks()
	require.Len(t, blocks, 1)
	require.Equal(t, 1, blocks[0].TotalInBucket)
	require.Equal(t, 0, blocks[0].SeqInBucket)
	require.NotContains(t, blocks[0].StableNonce(), "s0", "single sub-bucket should use legacy nonce without s suffix")
}

func TestByteBucket_SplitOnSizeOverflow(t *testing.T) {
	tl := NewTimeline(nil, nil)
	base := time.Date(2026, 5, 2, 10, 0, 0, 0, time.UTC)
	// two items, each large, same 3m bucket; small K forces two sub-buckets
	pad := strings.Repeat("X", 4000)
	injectTimelineItem(tl, 1, base.Add(10*time.Second), makeToolResult(1, "a", true, pad))
	injectTimelineItem(tl, 2, base.Add(20*time.Second), makeToolResult(2, "b", true, pad))

	blocks := tl.GroupByMinutesAndBytes(3, 5000).GetBlocks()
	require.GreaterOrEqual(t, len(blocks), 2, "expected byte split within same calendar bucket")
	require.Equal(t, 2, blocks[0].TotalInBucket)
	require.Equal(t, 0, blocks[0].SeqInBucket)
	require.Equal(t, 1, blocks[1].SeqInBucket)
	require.False(t, blocks[0].Open)
	require.True(t, blocks[1].Open)
	require.Contains(t, blocks[0].StableNonce(), "s0")
	require.Contains(t, blocks[1].StableNonce(), "s1")
}

func TestByteBucket_OversizedSingleItem(t *testing.T) {
	tl := NewTimeline(nil, nil)
	base := time.Date(2026, 5, 2, 10, 0, 0, 0, time.UTC)
	huge := strings.Repeat("Z", 20000)
	injectTimelineItem(tl, 1, base.Add(10*time.Second), makeToolResult(1, "a", true, huge))

	blocks := tl.GroupByMinutesAndBytes(3, 1000).GetBlocks()
	require.Len(t, blocks, 1)
	require.Equal(t, 1, blocks[0].TotalInBucket)
	require.True(t, blocks[0].Open)
}

func TestByteBucket_TimeAndSizeBothFire(t *testing.T) {
	tl := NewTimeline(nil, nil)
	base := time.Date(2026, 5, 2, 10, 0, 0, 0, time.UTC)
	pad := strings.Repeat("Y", 3000)
	injectTimelineItem(tl, 1, base.Add(30*time.Second), makeToolResult(1, "a", true, pad))
	injectTimelineItem(tl, 2, base.Add(4*time.Minute), makeToolResult(2, "b", true, pad))

	blocks := tl.GroupByMinutesAndBytes(3, 4000).GetBlocks()
	require.GreaterOrEqual(t, len(blocks), 2)
	require.False(t, blocks[0].Open)
	require.True(t, blocks[len(blocks)-1].Open)
}

func TestByteBucket_Idempotent_MultipleCalls(t *testing.T) {
	tl := NewTimeline(nil, nil)
	base := time.Date(2026, 5, 2, 10, 0, 0, 0, time.UTC)
	for i := 1; i <= 4; i++ {
		injectTimelineItem(tl, int64(i), base.Add(time.Duration(i)*time.Second),
			makeToolResult(int64(i), "t", true, strings.Repeat("p", 800)))
	}
	g := tl.GroupByMinutesAndBytes(3, 2500)
	r1 := g.GetAllRenderable().Render("TIMELINE")
	r2 := tl.GroupByMinutesAndBytes(3, 2500).GetAllRenderable().Render("TIMELINE")
	require.Equal(t, r1, r2)
}

func TestByteBucket_StableUnderManualReducer(t *testing.T) {
	tl := NewTimeline(nil, nil)
	base := time.Date(2026, 5, 2, 10, 0, 0, 0, time.UTC)
	injectTimelineItem(tl, 10, base.Add(30*time.Second), makeToolResult(10, "keep", true, "stable-a"))
	injectTimelineItem(tl, 11, base.Add(4*time.Minute), makeToolResult(11, "tail", true, "tail-b"))

	tl.reducers.Set(5, linktable.NewUnlimitedStringLinkTable("reducer-summary-line"))
	tl.reducerTs.Set(5, base.Add(1*time.Minute).UnixMilli())

	r1 := tl.GroupByMinutesAndBytes(3, 8000).GetBlocks()[1].Render()

	injectTimelineItem(tl, 12, base.Add(7*time.Minute), makeToolResult(12, "new", true, "new-c"))

	r1Again := tl.GroupByMinutesAndBytes(3, 8000).GetBlocks()[1].Render()
	require.Equal(t, r1, r1Again, "unaffected middle calendar bucket render must stay stable when appending later bucket")
}

func TestByteBucket_FrozenBoundaryWrapsMultiSubBucket(t *testing.T) {
	tl := NewTimeline(nil, nil)
	base := time.Date(2026, 5, 2, 10, 0, 0, 0, time.UTC)
	pad := strings.Repeat("W", 3500)
	injectTimelineItem(tl, 1, base.Add(10*time.Second), makeToolResult(1, "a", true, pad))
	injectTimelineItem(tl, 2, base.Add(20*time.Second), makeToolResult(2, "b", true, pad))

	dump := tl.GroupByMinutesAndBytes(3, 4000).GetAllRenderable().RenderWithFrozenBoundary(
		TimelineDumpDefaultAITagName,
		TimelineFrozenBoundaryTagName,
		TimelineFrozenBoundaryNonce,
	)
	start := "<|" + TimelineFrozenBoundaryTagName + "_" + TimelineFrozenBoundaryNonce + "|>"
	end := "<|" + TimelineFrozenBoundaryTagName + "_END_" + TimelineFrozenBoundaryNonce + "|>"
	i0 := strings.Index(dump, start)
	i1 := strings.Index(dump, end)
	require.True(t, i0 >= 0 && i1 > i0)
	frozen := dump[i0 : i1+len(end)]
	require.Contains(t, frozen, "s0")
	require.NotContains(t, frozen, "s1")
}

func TestByteBucket_DisableByteSplitMatchesLegacyNonce(t *testing.T) {
	tl := NewTimeline(nil, nil)
	base := time.Date(2026, 5, 2, 10, 0, 0, 0, time.UTC)
	injectTimelineItem(tl, 1, base.Add(10*time.Second), makeToolResult(1, "a", true, strings.Repeat("q", 9000)))
	injectTimelineItem(tl, 2, base.Add(20*time.Second), makeToolResult(2, "b", true, strings.Repeat("q", 9000)))

	tl.SetTimelineBucketByteSize(-1)
	blocks := tl.GroupByMinutes(3).GetBlocks()
	require.Len(t, blocks, 1)
	n := blocks[0].StableNonce()
	require.NotContains(t, n, "s0")
	require.Equal(t, fmt.Sprintf("b3t%d", blocks[0].BucketStart.Unix()), n)
}
