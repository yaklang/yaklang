package aicommon

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/linktable"
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
	reducerKey := int64(100)
	reducerTsMs := baseTs.UnixMilli()
	tl.reducers.Set(reducerKey, linktable.NewUnlimitedStringLinkTable("compressed batch alpha"))
	tl.reducerTs.Set(reducerKey, reducerTsMs)

	// DumpBefore 上界覆盖所有活跃 item
	out := tl.DumpBefore(5)
	require.NotEmpty(t, out, "DumpBefore should not be empty when reducer + active items both exist")

	// 必须包含 reducer block 的 aitag 包裹与首行
	expectedNonce := fmt.Sprintf("r%dt%d", reducerKey, reducerTsMs/1000)
	require.Contains(t, out, "<|TIMELINE_"+expectedNonce+"|>",
		"DumpBefore MUST preserve reducer block aitag wrapper after sub-timeline materialization")
	require.Contains(t, out, fmt.Sprintf("# reducer key=%d ts=%d", reducerKey, reducerTsMs/1000),
		"DumpBefore reducer block first line missing or unstable")
	require.Contains(t, out, "compressed batch alpha", "reducer body lost in DumpBefore")

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

	tl.reducers.Set(int64(7), linktable.NewUnlimitedStringLinkTable("legacy compressed memo"))
	tl.reducerTs.Set(int64(7), baseTs.Add(-time.Hour).UnixMilli())

	// 上界 5：活跃 item id (=10) 全部被过滤
	out := tl.DumpBefore(5)
	// 与 Dump() 对称：若 sub-timeline 没有活跃 item，则不渲染 reducer block，输出为空
	require.Empty(t, out,
		"DumpBefore must return empty when no active items survive the upper bound, mirroring Dump on empty active set")
}

// TestDumpBefore_MultipleReducers_AllPreserved 多个 reducer 全部保留
// 关键词: DumpBefore 多 reducer 保留, 顺序稳定
func TestDumpBefore_MultipleReducers_AllPreserved(t *testing.T) {
	tl := NewTimeline(nil, nil)
	baseTs := time.Date(2024, 6, 1, 10, 30, 0, 0, time.UTC)

	for i := int64(1); i <= 3; i++ {
		injectTimelineItem(tl, i, baseTs.Add(time.Duration(i)*time.Second), makeToolResult(i, "tool", true, "x"))
	}

	keys := []int64{200, 201, 202}
	for idx, k := range keys {
		tl.reducers.Set(k, linktable.NewUnlimitedStringLinkTable(fmt.Sprintf("memory-%d", k)))
		tl.reducerTs.Set(k, baseTs.Add(time.Duration(idx)*time.Minute).UnixMilli())
	}

	out := tl.DumpBefore(3)
	require.NotEmpty(t, out)

	for _, k := range keys {
		require.Contains(t, out, fmt.Sprintf("memory-%d", k), "reducer body for key=%d missing", k)
		require.Contains(t, out, fmt.Sprintf("# reducer key=%d ts=", k), "reducer header for key=%d missing", k)
	}

	// reducer block 顺序应按 ReducerKeyID 升序（与 GroupByMinutes 内部一致）
	idx0 := strings.Index(out, "memory-200")
	idx1 := strings.Index(out, "memory-201")
	idx2 := strings.Index(out, "memory-202")
	require.Greater(t, idx1, idx0, "reducer 200 should appear before 201")
	require.Greater(t, idx2, idx1, "reducer 201 should appear before 202")
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

	tl.reducers.Set(int64(50), linktable.NewUnlimitedStringLinkTable("alpha-mem"))
	tl.reducerTs.Set(int64(50), baseTs.Add(-5*time.Minute).UnixMilli())
	tl.reducers.Set(int64(51), linktable.NewUnlimitedStringLinkTable("beta-mem"))
	tl.reducerTs.Set(int64(51), baseTs.Add(-2*time.Minute).UnixMilli())

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
	reducerKeys := []int64{900, 901}
	for idx, k := range reducerKeys {
		tl.reducers.Set(k, linktable.NewUnlimitedStringLinkTable(fmt.Sprintf("hist-mem-%d", k)))
		tl.reducerTs.Set(k, baseTs.Add(time.Duration(idx)*time.Minute).UnixMilli())
	}

	sub := tl.CreateSubTimeline(1, 2)
	require.NotNil(t, sub)

	// 关键回归断言：sub 必须全量继承 reducers / reducerTs，与 ids 解耦
	require.Equal(t, len(reducerKeys), sub.reducers.Len(),
		"CreateSubTimeline must inherit ALL reducers regardless of input ids")
	for _, k := range reducerKeys {
		got, ok := sub.reducers.Get(k)
		require.True(t, ok, "reducer key=%d missing from sub-timeline", k)
		require.Equal(t, fmt.Sprintf("hist-mem-%d", k), got.Value())

		gotTs, tsOk := sub.reducerTs.Get(k)
		require.True(t, tsOk, "reducerTs for key=%d missing from sub-timeline", k)
		require.Greater(t, gotTs, int64(0))
	}

	// 副语义：sub.Dump() 中应能渲染出所有 reducer block
	out := sub.Dump()
	for _, k := range reducerKeys {
		require.Contains(t, out, fmt.Sprintf("# reducer key=%d ts=", k),
			"sub.Dump() must render reducer block for key=%d", k)
		require.Contains(t, out, fmt.Sprintf("hist-mem-%d", k))
	}
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

	tl.reducers.Set(int64(5), linktable.NewUnlimitedStringLinkTable("orphan-reducer"))
	tl.reducerTs.Set(int64(5), baseTs.UnixMilli())

	// ids 中不包含 5（reducer key），但 reducer 仍应被继承
	sub := tl.CreateSubTimeline(10, 11)
	require.NotNil(t, sub)
	require.Equal(t, 1, sub.reducers.Len())
	v, ok := sub.reducers.Get(int64(5))
	require.True(t, ok)
	require.Equal(t, "orphan-reducer", v.Value())
}
