package aicommon

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/aitag"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/utils/linktable"
)

// TestDumpBefore_ReducerTimeStable 验证 DumpBefore 中 reducer 行使用稳定时间戳，多次调用字节级一致
// 关键词: DumpBefore reducer 稳定时间戳, 缓存稳定
// 历史背景: 旧实现 reducer 行用 time.Now() 渲染，导致 Dump 每次输出不同，破坏 LLM 前缀缓存
func TestDumpBefore_ReducerTimeStable(t *testing.T) {
	tl := NewTimeline(nil, nil)

	baseTs := time.Date(2024, 6, 1, 10, 30, 0, 0, time.UTC)
	for i := int64(1); i <= 3; i++ {
		injectTimelineItem(tl, i, baseTs.Add(time.Duration(i)*time.Second), makeToolResult(i, "tool", true, "data"))
	}

	// 模拟批量压缩后只剩 reducer + 部分活跃条目
	reducerKey := int64(2)
	reducerTs := baseTs.UnixMilli()
	tl.reducers.Set(reducerKey, linktable.NewUnlimitedStringLinkTable("compressed batch memory"))
	tl.reducerTs.Set(reducerKey, reducerTs)

	dump1 := tl.Dump()
	require.NotEmpty(t, dump1)

	// 短暂等待，确保 time.Now() 与第一次不同（如果实现仍依赖 Now，输出会变）
	time.Sleep(50 * time.Millisecond)

	dump2 := tl.Dump()
	require.Equal(t, dump1, dump2, "DumpBefore reducer line MUST be byte-identical across consecutive calls")

	// 必须包含基于 reducerTs 渲染的稳定时间字段
	expectedTimeStr := time.Unix(0, reducerTs*int64(time.Millisecond)).Format("2006/01/02 15:04:05")
	require.Contains(t, dump1, expectedTimeStr, "reducer line should use stable timestamp from reducerTs")
	require.Contains(t, dump1, "reducer-memory: compressed batch memory")
}

// TestDumpBefore_ReducerNoLegacyNow 验证当 reducerTs 缺失时也使用稳定占位，不再用 time.Now()
// 关键词: DumpBefore reducer fallback, 老数据稳定渲染
func TestDumpBefore_ReducerNoLegacyNow(t *testing.T) {
	tl := NewTimeline(nil, nil)

	baseTs := time.Date(2024, 6, 1, 10, 30, 0, 0, time.UTC)
	injectTimelineItem(tl, int64(1), baseTs, makeToolResult(1, "tool", true, "data"))

	// 仅设置 reducers，不设置 reducerTs（模拟老数据）
	tl.reducers.Set(int64(1), linktable.NewUnlimitedStringLinkTable("legacy memory"))

	dump1 := tl.Dump()
	time.Sleep(50 * time.Millisecond)
	dump2 := tl.Dump()

	require.Equal(t, dump1, dump2, "DumpBefore must remain stable even when reducerTs is missing")
	// 老数据 fallback：使用 1970 epoch 占位，确保稳定
	require.Contains(t, dump1, "1970/01/01 00:00:00")
}

// TestGroupByMinutes_ReducerBlock_Basic 验证 GroupByMinutes 输出 reducer block
// 关键词: GroupByMinutes reducerBlocks 基础
func TestGroupByMinutes_ReducerBlock_Basic(t *testing.T) {
	tl := NewTimeline(nil, nil)
	baseTs := time.Date(2024, 6, 1, 10, 30, 0, 0, time.UTC)
	injectTimelineItem(tl, int64(1), baseTs.Add(time.Second), makeToolResult(1, "ls", true, "ok"))

	tl.reducers.Set(int64(101), linktable.NewUnlimitedStringLinkTable("reducer text alpha"))
	tl.reducerTs.Set(int64(101), baseTs.UnixMilli())
	tl.reducers.Set(int64(102), linktable.NewUnlimitedStringLinkTable("reducer text beta"))
	tl.reducerTs.Set(int64(102), baseTs.Add(2*time.Minute).UnixMilli())

	g := tl.GroupByMinutes(3)
	require.NotNil(t, g)

	rbs := g.GetReducerBlocks()
	require.Len(t, rbs, 2)
	// 按 ReducerKeyID 升序排列
	require.Equal(t, int64(101), rbs[0].ReducerKeyID)
	require.Equal(t, int64(102), rbs[1].ReducerKeyID)

	body0 := rbs[0].Render()
	require.Contains(t, body0, "reducer text alpha")
	require.Contains(t, body0, "[reducer/memory]")
	require.Contains(t, body0, "# reducer key=101 ts=")
}

// TestGroupByMinutes_ReducerBlock_NonceStable 验证 reducer block 的 nonce 稳定且 aitag 兼容
// 关键词: TimelineReducerBlock.StableNonce, aitag 兼容
func TestGroupByMinutes_ReducerBlock_NonceStable(t *testing.T) {
	ts := time.Date(2024, 6, 1, 10, 30, 0, 0, time.UTC)
	rb := &TimelineReducerBlock{
		ReducerKeyID: 42,
		Ts:           ts,
		Text:         "memory body",
	}
	n1 := rb.StableNonce()
	n2 := rb.StableNonce()
	require.Equal(t, n1, n2)
	require.NotContains(t, n1, "_", "nonce must not contain '_' to keep aitag tagName boundary correct")
	require.Equal(t, "r42t1717237800", n1)

	// IsOpen 恒为 false
	require.False(t, rb.IsOpen())

	// 老数据：Ts 为零时仍稳定
	rb2 := &TimelineReducerBlock{ReducerKeyID: 42, Text: "memory body"}
	require.Equal(t, "r42t0", rb2.StableNonce())
	require.False(t, rb2.IsOpen())
}

// TestGroupByMinutes_ReducerBlock_AITagSplit 验证 reducer + interval block 通过 aitag.SplitViaTAG 可被正确切分
// 关键词: TimelineRenderableBlocks.Render, aitag.SplitViaTAG, reducer + interval 混合
func TestGroupByMinutes_ReducerBlock_AITagSplit(t *testing.T) {
	tl := NewTimeline(nil, nil)
	baseTs := time.Date(2024, 6, 1, 10, 30, 0, 0, time.UTC)
	injectTimelineItem(tl, int64(1), baseTs.Add(1*time.Second), makeToolResult(1, "ls", true, "out-1"))
	injectTimelineItem(tl, int64(2), baseTs.Add(4*time.Minute), makeToolResult(2, "cat", true, "out-2"))

	tl.reducers.Set(int64(50), linktable.NewUnlimitedStringLinkTable("reducer alpha"))
	tl.reducerTs.Set(int64(50), baseTs.Add(-5*time.Minute).UnixMilli())

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
	tl.reducers.Set(int64(50), linktable.NewUnlimitedStringLinkTable("reducer alpha"))
	tl.reducerTs.Set(int64(50), baseTs.Add(-5*time.Minute).UnixMilli())

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
	tl.reducers.Set(int64(101), linktable.NewUnlimitedStringLinkTable("memory"))
	tl.reducerTs.Set(int64(101), int64(1700000000000))

	out, err := MarshalTimeline(tl)
	require.NoError(t, err)
	// 新数据中不应有 summary 字段
	require.NotContains(t, out, "\"summary\":", "MarshalTimeline must not emit summary field anymore")
	require.Contains(t, out, "\"reducer_ts\":")

	// 模拟老数据 JSON：包含 summary 字段
	legacy := strings.Replace(out, "\"reducer_ts\":", "\"summary\":{\"999\":{\"id\":999}},\"reducer_ts\":", 1)
	tl2, err := UnmarshalTimeline(legacy)
	require.NoError(t, err, "Unmarshal must tolerate legacy summary field")
	require.NotNil(t, tl2)
	// 反序列化后 reducers 与 reducerTs 仍应正常恢复
	require.Equal(t, 1, tl2.reducers.Len())
	gotTs, ok := tl2.reducerTs.Get(int64(101))
	require.True(t, ok)
	require.Equal(t, int64(1700000000000), gotTs)
}

// 关键词: ToolResult 占位避免 import 警告（commonPrefixLen 在 timeline_groups_render_aitag_test.go 已定义，本文件复用）
var _ = aitool.ToolResult{}
