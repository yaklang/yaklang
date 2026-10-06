package aicommon

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/aitag"
)

// Compressed-head timestamps and nonces must remain stable, including old
// dumps with no covered timestamp. Exact expectations catch use of time.Now
// without introducing wall-clock sleeps into the cache regression.
func TestDumpCompressedHeadStable(t *testing.T) {
	base := time.Date(2024, 6, 1, 10, 30, 0, 0, time.UTC)
	for _, test := range []struct {
		name      string
		timestamp int64
		version   int64
		nonce     string
	}{
		{"timestamp", base.UnixMilli(), 7, "h1717237800v7"},
		{"legacy_missing_timestamp", 0, 1, "h0v1"},
	} {
		t.Run(test.name, func(t *testing.T) {
			tl := NewTimeline(nil, nil)
			injectTimelineItem(tl, 1, base, makeToolResult(1, "tool", true, "data"))
			tl.compressedHead = &TimelineCompressedHead{
				Text: "compressed history", CoveredEndItemID: 2,
				CoveredEndAtMs: test.timestamp, Version: test.version,
			}
			dump := tl.Dump()
			require.NotEmpty(t, dump)
			require.Equal(t, dump, tl.Dump())
			require.Contains(t, dump, "<|TIMELINE_"+test.nonce+"|>")
			require.Contains(t, dump, fmt.Sprintf("# compressed_head covered_end_item_id=2 covered_end_at_ms=%d version=%d", test.timestamp, test.version))
			require.Contains(t, dump, "[compressed/head]")
			require.Contains(t, dump, "compressed history")
		})
	}
}

// TestGroupByMinutes_ReducerBlock_Basic 验证 GroupByMinutes 输出 reducer block
// 关键词: GroupByMinutes reducerBlocks 基础
func TestGroupByMinutes_ReducerBlock_Basic(t *testing.T) {
	tl := NewTimeline(nil, nil)
	baseTs := time.Date(2024, 6, 1, 10, 30, 0, 0, time.UTC)
	injectTimelineItem(tl, int64(1), baseTs.Add(time.Second), makeToolResult(1, "ls", true, "ok"))

	tl.compressedHead = &TimelineCompressedHead{
		Text:             "reducer text alpha",
		CoveredEndItemID: 101,
		CoveredEndAtMs:   baseTs.UnixMilli(),
		Version:          2,
	}

	g := tl.GroupByMinutes(3)
	require.NotNil(t, g)

	all := g.GetAllRenderable()
	require.GreaterOrEqual(t, len(all), 2)
	hb, ok := all[0].(*TimelineCompressedHeadBlock)
	require.True(t, ok)
	require.Equal(t, int64(101), hb.CoveredEndItemID)
	require.Equal(t, int64(2), hb.Version)
	body0 := hb.Render()
	require.Contains(t, body0, "reducer text alpha")
	require.Contains(t, body0, "[compressed/head]")
	require.Contains(t, body0, "# compressed_head covered_end_item_id=101")
}

// TestGroupByMinutes_ReducerBlock_NonceStable 验证 reducer block 的 nonce 稳定且 aitag 兼容
// 关键词: TimelineReducerBlock.StableNonce, aitag 兼容
func TestGroupByMinutes_ReducerBlock_NonceStable(t *testing.T) {
	ts := time.Date(2024, 6, 1, 10, 30, 0, 0, time.UTC)
	rb := &TimelineCompressedHeadBlock{
		CoveredEndItemID: 42,
		CoveredEndAtMs:   ts.UnixMilli(),
		Version:          9,
		Text:             "memory body",
	}
	n1 := rb.StableNonce()
	n2 := rb.StableNonce()
	require.Equal(t, n1, n2)
	require.NotContains(t, n1, "_", "nonce must not contain '_' to keep aitag tagName boundary correct")
	require.Equal(t, "h1717237800v9", n1)

	// IsOpen 恒为 false
	require.False(t, rb.IsOpen())

	// 老数据：Ts 为零时仍稳定
	rb2 := &TimelineCompressedHeadBlock{CoveredEndItemID: 42, Text: "memory body", Version: 1}
	require.Equal(t, "h0v1", rb2.StableNonce())
	require.False(t, rb2.IsOpen())
}

// TestGroupByMinutes_ReducerBlock_AITagSplit 验证 reducer + interval block 通过 aitag.SplitViaTAG 可被正确切分
// 关键词: TimelineRenderableBlocks.Render, aitag.SplitViaTAG, reducer + interval 混合
func TestGroupByMinutes_ReducerBlock_AITagSplit(t *testing.T) {
	tl := NewTimeline(nil, nil)
	baseTs := time.Date(2024, 6, 1, 10, 30, 0, 0, time.UTC)
	injectTimelineItem(tl, int64(1), baseTs.Add(1*time.Second), makeToolResult(1, "ls", true, "out-1"))
	injectTimelineItem(tl, int64(2), baseTs.Add(4*time.Minute), makeToolResult(2, "cat", true, "out-2"))

	tl.compressedHead = &TimelineCompressedHead{
		Text:             "reducer alpha",
		CoveredEndItemID: 50,
		CoveredEndAtMs:   baseTs.Add(-5 * time.Minute).UnixMilli(),
		Version:          3,
	}

	g := tl.GroupByMinutes(3)
	all := g.GetAllRenderable()
	require.GreaterOrEqual(t, len(all), 2)

	prompt := all.Render("TG")
	require.NotEmpty(t, prompt)

	res, err := aitag.SplitViaTAG(prompt, "TG")
	require.NoError(t, err)

	tagged := res.GetTaggedBlocks()
	// reducer block + interval blocks 应都成为独立 tagged block
	require.Equal(t, len(all), len(tagged), "every renderable block must be parsed as a tagged aitag block")
}

// TestGroupByMinutes_ReducerBlock_PrefixStability 验证连续调用 GroupByMinutes + Render 字节一致
// 关键词: TimelineRenderableBlocks.Render 缓存稳定, 前缀字节一致
func TestGroupByMinutes_ReducerBlock_PrefixStability(t *testing.T) {
	tl := NewTimeline(nil, nil)
	baseTs := time.Date(2024, 6, 1, 10, 30, 0, 0, time.UTC)
	injectTimelineItem(tl, int64(1), baseTs.Add(1*time.Second), makeToolResult(1, "ls", true, "out-1"))
	injectTimelineItem(tl, int64(2), baseTs.Add(4*time.Minute), makeToolResult(2, "cat", true, "out-2"))
	tl.compressedHead = &TimelineCompressedHead{
		Text:             "reducer alpha",
		CoveredEndItemID: 50,
		CoveredEndAtMs:   baseTs.Add(-5 * time.Minute).UnixMilli(),
		Version:          3,
	}

	g1 := tl.GroupByMinutes(3)
	g2 := tl.GroupByMinutes(3)

	r1 := g1.GetAllRenderable().Render("PREFIX")
	r2 := g2.GetAllRenderable().Render("PREFIX")
	require.Equal(t, r1, r2, "GroupByMinutes + Render output must be byte-identical for unchanged timeline")

	// 即使插入新条目，reducer block 与前面已冻结 interval block 的输出应仍是新输出的前缀
	// 关键词: 增量插入, 前缀缓存命中
	injectTimelineItem(tl, int64(3), baseTs.Add(8*time.Minute), makeToolResult(3, "echo", true, "out-3"))
	g3 := tl.GroupByMinutes(3)
	r3 := g3.GetAllRenderable().Render("PREFIX")

	common := commonPrefixLen(r1, r3)
	require.Greater(t, common, len(r1)/2,
		"after adding a new bucket, the previous render should remain a long stable prefix; got common=%d/%d",
		common, len(r1))
}

// TestSummaryFieldRemoved_BackwardCompat 验证序列化不再写出 Summary，反序列化老数据时静默忽略
// 关键词: summary 字段移除, Marshal omit summary, Unmarshal 兼容老数据
func TestSummaryFieldRemoved_BackwardCompat(t *testing.T) {
	tl := NewTimeline(nil, nil)
	for i := int64(1); i <= 2; i++ {
		tl.PushToolResult(makeToolResult(i, "tool", true, "data"))
	}
	tl.compressedHead = &TimelineCompressedHead{
		Text:             "memory",
		CoveredEndItemID: 101,
		CoveredEndAtMs:   int64(1700000000000),
		Version:          2,
	}

	out, err := MarshalTimeline(tl)
	require.NoError(t, err)
	// 新数据中不应有 summary 字段
	require.NotContains(t, out, "\"summary\":", "MarshalTimeline must not emit summary field anymore")
	require.Contains(t, out, "\"compressed_head\":")

	// 模拟老数据 JSON：包含 summary 字段
	legacy := strings.Replace(out, "\"compressed_head\":", "\"summary\":{\"999\":{\"id\":999}},\"compressed_head\":", 1)
	tl2, err := UnmarshalTimeline(legacy)
	require.NoError(t, err, "Unmarshal must tolerate legacy summary field")
	require.NotNil(t, tl2)
	require.NotNil(t, tl2.compressedHead)
	require.Equal(t, int64(101), tl2.compressedHead.CoveredEndItemID)
	require.Equal(t, int64(1700000000000), tl2.compressedHead.CoveredEndAtMs)
}
