package aicommon

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestDumpBefore_PreservesReducerBlocks 回归测试：DumpBefore 必须保留 reducer block
// 关键词: DumpBefore reducer 保留, CreateSubTimeline reducer 复制 bug 修复, 回归
//
// 历史背景:
//
//	旧 Dump 实现里 reducer 在 buf 顶层独立渲染；新 Dump 走 GroupByMinutes，DumpBefore 通过
//	CreateSubTimeline 限定上界后再调用 sub.Dump()。但 CreateSubTimeline 用 item.ID 去 m.reducers
//	里查（reducers 的 key 是 reducerKeyID，对应已被压缩并从活跃区移除的旧 item id），永远 miss，
//	导致 sub.reducers 为空，DumpBefore 整体丢失全部 reducer block，相对旧实现是语义回退。
//
// 期望:
//
//	DumpBefore 输出必须包含 reducer block（与全量 Dump 行为一致）。
func TestDumpBefore_PreservesReducerBlocks(t *testing.T) {
	tl := NewTimeline(nil, nil)
	baseTs := time.Date(2024, 6, 1, 10, 30, 0, 0, time.UTC)

	for i := int64(1); i <= 5; i++ {
		injectTimelineItem(tl, i, baseTs.Add(time.Duration(i)*time.Second), makeToolResult(i, "tool", true, fmt.Sprintf("data-%d", i)))
	}

	// 模拟批量压缩留下的 reducer，key 是某个已经从活跃区移除的旧 id（远小于活跃 id 范围更安全）
	reducerKey := int64(5)
	reducerTsMs := baseTs.UnixMilli()
	tl.compressedHead = &TimelineCompressedHead{
		Text:             "compressed batch alpha",
		CoveredEndItemID: reducerKey,
		CoveredEndAtMs:   reducerTsMs,
		Version:          3,
	}

	// DumpBefore 上界覆盖所有活跃 item
	out := tl.DumpBefore(5)
	require.NotEmpty(t, out, "DumpBefore should not be empty when reducer + active items both exist")

	// 必须包含 reducer block 的 aitag 包裹与首行
	expectedNonce := fmt.Sprintf("h%dv3", reducerTsMs/1000)
	require.Contains(t, out, "<|TIMELINE_"+expectedNonce+"|>",
		"DumpBefore MUST preserve compressed-head block aitag wrapper after sub-timeline materialization")
	require.Contains(t, out, fmt.Sprintf("# compressed_head covered_end_item_id=%d covered_end_at_ms=%d version=%d", reducerKey, reducerTsMs, int64(3)),
		"DumpBefore compressed-head block first line missing or unstable")
	require.Contains(t, out, "compressed batch alpha", "compressed head body lost in DumpBefore")

	// 也要包含至少一个 interval block（活跃 item 渲染），interval block nonce 形如 b<minutes>t<unixSec>
	require.Contains(t, out, "<|TIMELINE_b", "DumpBefore should still emit interval block(s) for active items")
}

// TestDumpBefore_NoActiveItems_SilentLikeDump 边界对齐：若上界过滤掉所有活跃 item，
// DumpBefore 应与 Dump 在 idToTimelineItem 为空时的行为对称——返回空。
// 关键词: DumpBefore 边界, 与 Dump 行为对齐, GroupByMinutes 空活跃时不渲染 reducer
func TestDumpBefore_NoActiveItems_SilentLikeDump(t *testing.T) {
	tl := NewTimeline(nil, nil)
	baseTs := time.Date(2024, 6, 1, 10, 30, 0, 0, time.UTC)

	injectTimelineItem(tl, int64(10), baseTs, makeToolResult(10, "tool", true, "active"))

	tl.compressedHead = &TimelineCompressedHead{
		Text:             "legacy compressed memo",
		CoveredEndItemID: 7,
		CoveredEndAtMs:   baseTs.Add(-time.Hour).UnixMilli(),
		Version:          1,
	}

	// 上界 5：活跃 item id (=10) 全部被过滤
	out := tl.DumpBefore(5)
	require.Empty(t, out,
		"DumpBefore hides compressed head when beforeId is before covered end")
}

// TestDumpBefore_MultipleReducers_AllPreserved 多个 reducer 全部保留
// 关键词: DumpBefore 多 reducer 保留, 顺序稳定
func TestDumpBefore_MultipleReducers_AllPreserved(t *testing.T) {
	tl := NewTimeline(nil, nil)
	baseTs := time.Date(2024, 6, 1, 10, 30, 0, 0, time.UTC)

	for i := int64(1); i <= 3; i++ {
		injectTimelineItem(tl, i, baseTs.Add(time.Duration(i)*time.Second), makeToolResult(i, "tool", true, "x"))
	}

	tl.compressedHead = &TimelineCompressedHead{
		Text:             "memory-200",
		CoveredEndItemID: 3,
		CoveredEndAtMs:   baseTs.UnixMilli(),
		Version:          4,
	}

	out := tl.DumpBefore(3)
	require.NotEmpty(t, out)

	require.Contains(t, out, "memory-200")
	require.Contains(t, out, "# compressed_head covered_end_item_id=3")
}

// TestDumpBefore_EquivalentToDumpWhenAllItemsCovered 上界覆盖所有 item 时，
// DumpBefore 输出应与 Dump 完全一致（含 reducer block）。
// 关键词: DumpBefore 与 Dump 等价, reducer 行为对齐
func TestDumpBefore_EquivalentToDumpWhenAllItemsCovered(t *testing.T) {
	tl := NewTimeline(nil, nil)
	baseTs := time.Date(2024, 6, 1, 10, 30, 0, 0, time.UTC)

	for i := int64(1); i <= 4; i++ {
		injectTimelineItem(tl, i, baseTs.Add(time.Duration(i)*time.Second), makeToolResult(i, "tool", true, "data"))
	}

	tl.compressedHead = &TimelineCompressedHead{
		Text:             "beta-mem",
		CoveredEndItemID: 4,
		CoveredEndAtMs:   baseTs.Add(-2 * time.Minute).UnixMilli(),
		Version:          2,
	}

	full := tl.Dump()
	bounded := tl.DumpBefore(4)
	require.Equal(t, full, bounded,
		"DumpBefore with upper bound covering all active items must be byte-identical to Dump")
}

// TestCreateSubTimeline_InheritsAllReducers 源头回归：CreateSubTimeline 必须全量继承 reducers
// 关键词: CreateSubTimeline reducer 继承, 死代码修复, 源头回归
//
// 历史 bug: 原实现 m.reducers.Get(id) 用入参 id（活跃 item id）去查 reducers 表，但
// reducers 的 key 是 reducerKeyID（被批量压缩后从活跃区移除的旧 item id），二者属于
// 不同命名空间，必然 miss。结果任何派生 sub-timeline 的 reducer 全部丢失。
func TestCreateSubTimeline_InheritsAllReducers(t *testing.T) {
	tl := NewTimeline(nil, nil)
	baseTs := time.Date(2024, 6, 1, 10, 30, 0, 0, time.UTC)

	// 主 timeline 中的活跃 item id: 1,2,3
	for i := int64(1); i <= 3; i++ {
		injectTimelineItem(tl, i, baseTs.Add(time.Duration(i)*time.Second), makeToolResult(i, "tool", true, "x"))
	}

	// 模拟两次批量压缩留下的 reducer，key 是已经从活跃区移除的旧 id（与 ids 不重叠）
	tl.compressedHead = &TimelineCompressedHead{
		Text:             "hist-mem-901",
		CoveredEndItemID: 901,
		CoveredEndAtMs:   baseTs.UnixMilli(),
		Version:          2,
	}

	sub := tl.CreateSubTimeline(1, 2)
	require.NotNil(t, sub)

	require.NotNil(t, sub.compressedHead)
	require.Equal(t, tl.compressedHead.Text, sub.compressedHead.Text)
	require.Equal(t, tl.compressedHead.CoveredEndItemID, sub.compressedHead.CoveredEndItemID)

	// 副语义：sub.Dump() 中应能渲染出所有 reducer block
	out := sub.Dump()
	require.Contains(t, out, "hist-mem-901")
	require.Contains(t, out, "# compressed_head covered_end_item_id=901")
}

// TestCreateSubTimeline_ReducerInheritIndependentOfIds 验证 reducer 继承与 ids 是否包含
// reducer key 完全无关（ids 故意不包含 reducer key 也必须继承）
// 关键词: CreateSubTimeline reducer ids 解耦
func TestCreateSubTimeline_ReducerInheritIndependentOfIds(t *testing.T) {
	tl := NewTimeline(nil, nil)
	baseTs := time.Date(2024, 6, 1, 10, 30, 0, 0, time.UTC)

	for i := int64(10); i <= 12; i++ {
		injectTimelineItem(tl, i, baseTs.Add(time.Duration(i)*time.Second), makeToolResult(i, "tool", true, "x"))
	}

	tl.compressedHead = &TimelineCompressedHead{
		Text:             "orphan-reducer",
		CoveredEndItemID: 5,
		CoveredEndAtMs:   baseTs.UnixMilli(),
		Version:          1,
	}

	// ids 中不包含 5（reducer key），但 reducer 仍应被继承
	sub := tl.CreateSubTimeline(10, 11)
	require.NotNil(t, sub)
	require.NotNil(t, sub.compressedHead)
	require.Equal(t, "orphan-reducer", sub.compressedHead.Text)
}
