package aicommon

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/aitag"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

// TestDumpBefore_ReducerTimeStable 验证 Dump 中 reducer block 使用稳定时间戳，多次调用字节级一致
// 关键词: Dump reducer 稳定时间戳, 缓存稳定, aitag 格式
// 历史背景: 旧实现 reducer 行用 time.Now() 渲染，导致 Dump 每次输出不同，破坏 LLM 前缀缓存。
// 新格式: Dump 走 GroupByMinutes，reducer block 渲染为 <|TIMELINE_r<id>t<unixSec>|> 包裹的 aitag 块，
// 内部首行 `# reducer key=<id> ts=<unixSec>`，时间字段使用 unixSec 而非 YYYY/MM/DD 字符串。
func TestDumpBefore_ReducerTimeStable(t *testing.T) {
	tl := NewTimeline(nil, nil)

	baseTs := time.Date(2024, 6, 1, 10, 30, 0, 0, time.UTC)
	for i := int64(1); i <= 3; i++ {
		injectTimelineItem(tl, i, baseTs.Add(time.Duration(i)*time.Second), makeToolResult(i, "tool", true, "data"))
	}

	// 模拟批量压缩后只剩 reducer + 部分活跃条目
	reducerKey := int64(2)
	reducerTs := baseTs.UnixMilli()
	tl.compressedHead = &TimelineCompressedHead{
		Text:             "compressed batch memory",
		CoveredEndItemID: reducerKey,
		CoveredEndAtMs:   reducerTs,
		Version:          7,
	}

	dump1 := tl.Dump()
	require.NotEmpty(t, dump1)

	// 短暂等待，确保 time.Now() 与第一次不同（如果实现仍依赖 Now，输出会变）
	time.Sleep(50 * time.Millisecond)

	dump2 := tl.Dump()
	require.Equal(t, dump1, dump2, "Dump reducer block MUST be byte-identical across consecutive calls")

	// 必须使用基于 reducerTs 派生的稳定 unix 秒戳作为 nonce 与首行 ts
	expectedNonce := fmt.Sprintf("h%dv7", reducerTs/1000)
	require.Contains(t, dump1, "<|TIMELINE_"+expectedNonce+"|>", "compressed head block aitag wrapper should use stable nonce")
	require.Contains(t, dump1, fmt.Sprintf("# compressed_head covered_end_item_id=%d covered_end_at_ms=%d version=%d", reducerKey, reducerTs, int64(7)))
	require.Contains(t, dump1, "[compressed/head]")
	require.Contains(t, dump1, "compressed batch memory")
}

// TestDumpBefore_ReducerNoLegacyNow 验证当 reducerTs 缺失时也使用稳定占位，不再用 time.Now()
// 关键词: Dump reducer fallback, 老数据稳定渲染, aitag 格式
// 新格式: 老数据 reducerTs 缺失 → unix 秒戳为 0、行头 00:00:00、aitag nonce r<id>t0
func TestDumpBefore_ReducerNoLegacyNow(t *testing.T) {
	tl := NewTimeline(nil, nil)

	baseTs := time.Date(2024, 6, 1, 10, 30, 0, 0, time.UTC)
	injectTimelineItem(tl, int64(1), baseTs, makeToolResult(1, "tool", true, "data"))

	// 仅设置 reducers，不设置 reducerTs（模拟老数据）
	reducerKey := int64(1)
	tl.compressedHead = &TimelineCompressedHead{
		Text:             "legacy memory",
		CoveredEndItemID: reducerKey,
		CoveredEndAtMs:   0,
		Version:          1,
	}

	dump1 := tl.Dump()
	time.Sleep(50 * time.Millisecond)
	dump2 := tl.Dump()

	require.Equal(t, dump1, dump2, "Dump must remain stable even when reducerTs is missing")
	// 老数据 fallback：使用 ts=0 占位，aitag nonce 为 r<id>t0，行头时间为 00:00:00
	require.Contains(t, dump1, "<|TIMELINE_h0v1|>")
	require.Contains(t, dump1, fmt.Sprintf("# compressed_head covered_end_item_id=%d covered_end_at_ms=0 version=1", reducerKey))
	require.Contains(t, dump1, "[compressed/head]")
	require.Contains(t, dump1, "legacy memory")
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

// 关键词: ToolResult 占位避免 import 警告（commonPrefixLen 在 timeline_groups_render_aitag_test.go 已定义，本文件复用）
var _ = aitool.ToolResult{}
