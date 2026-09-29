package aicommon

import (
	"encoding/json"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

func TestTimelineCompressionRestoreIgnoresRemovedMetadata(t *testing.T) {
	tl := NewTimeline(nil, nil)
	tl.PushText(1, "ordinary history")
	tl.PushPromotable(2, TimelinePromotedKindRecentTool, TimelinePromotedTargetSemiDynamic1, "probe", TimelinePromotedOperationUpsert, "exact schema")
	tl.compressedHead = &TimelineCompressedHead{Text: "prior summary", Version: 1}
	before, err := MarshalTimeline(tl)
	require.NoError(t, err)
	var legacy map[string]json.RawMessage
	require.NoError(t, json.Unmarshal([]byte(before), &legacy))
	// Retired metadata is an unknown field, even if its old format is malformed.
	legacy["archive_refs"] = json.RawMessage(`"unused historical metadata"`)
	raw, err := json.Marshal(legacy)
	require.NoError(t, err)
	restored, err := UnmarshalTimeline(string(raw))
	require.NoError(t, err)
	after, err := MarshalTimeline(restored)
	require.NoError(t, err)
	require.Equal(t, before, after, "ordinary history, summary and exact state survive; removed metadata does not")
	require.NotContains(t, after, "archive_refs")
}

func TestTimelineCompressionInvalidRemapPreservesState(t *testing.T) {
	tl := NewTimeline(nil, nil)
	tl.PushText(10, "first entry")
	tl.PushText(20, "second entry")
	before, err := MarshalTimeline(tl)
	require.NoError(t, err)
	require.Zero(t, tl.ReassignIDs(func() int64 { return 1 }))
	after, err := MarshalTimeline(tl)
	require.NoError(t, err)
	require.Equal(t, before, after, "duplicate generated IDs must not partially rewrite indexes")
}

// TestTimelineReassignIDs_Basic 测试基本的 ID 重新分配功能
func TestTimelineReassignIDs_Basic(t *testing.T) {
	timeline := NewTimeline(nil, nil)

	// 添加一些测试数据，使用非连续的 ID
	timeline.PushToolResult(&aitool.ToolResult{
		ID:          1001,
		Name:        "tool1",
		Description: "test1",
		Success:     true,
		Data:        "data1",
	})

	timeline.PushToolResult(&aitool.ToolResult{
		ID:          2005,
		Name:        "tool2",
		Description: "test2",
		Success:     true,
		Data:        "data2",
	})

	timeline.PushUserInteraction(UserInteractionStage_Review, 3010, "prompt", "input")
	timeline.PushText(4020, "text content")

	require.Equal(t, 4, timeline.idToTimelineItem.Len())

	// 创建 ID 生成器
	var idCounter int64 = 100
	generator := func() int64 {
		return atomic.AddInt64(&idCounter, 1)
	}

	// 重新分配 ID
	lastID := timeline.ReassignIDs(generator)

	// 验证结果
	require.Equal(t, int64(104), lastID)
	require.Equal(t, 4, timeline.idToTimelineItem.Len())

	// 验证 ID 是连续的
	ids := timeline.GetTimelineItemIDs()
	for i := 1; i < len(ids); i++ {
		require.Equal(t, ids[i-1]+1, ids[i], "IDs should be sequential")
	}

	// 验证数据完整性
	var foundTool1, foundTool2, foundInteraction, foundText bool
	for _, id := range ids {
		item, ok := timeline.idToTimelineItem.Get(id)
		require.True(t, ok)

		switch v := item.GetValue().(type) {
		case *aitool.ToolResult:
			if v.Name == "tool1" {
				foundTool1 = true
				require.Equal(t, "data1", v.Data)
				require.Equal(t, id, v.ID)
			} else if v.Name == "tool2" {
				foundTool2 = true
				require.Equal(t, "data2", v.Data)
				require.Equal(t, id, v.ID)
			}
		case *UserInteraction:
			foundInteraction = true
			require.Equal(t, UserInteractionStage_Review, v.Stage)
			require.Equal(t, id, v.ID)
		case *TextTimelineItem:
			foundText = true
			require.Equal(t, "text content", v.Text)
			require.Equal(t, id, v.ID)
		}
	}

	require.True(t, foundTool1)
	require.True(t, foundTool2)
	require.True(t, foundInteraction)
	require.True(t, foundText)
}

// TestTimelineReassignIDs_Empty 测试空 timeline 的情况
func TestTimelineReassignIDs_Empty(t *testing.T) {
	timeline := NewTimeline(nil, nil)

	var idCounter int64 = 100
	generator := func() int64 {
		return atomic.AddInt64(&idCounter, 1)
	}

	lastID := timeline.ReassignIDs(generator)
	require.Equal(t, int64(0), lastID)
	require.Equal(t, 0, timeline.idToTimelineItem.Len())
}

// TestTimelineReassignIDs_WithCompressedHead 测试 ReassignIDs 同步重映射 compressedHead.CoveredEndItemID
// 关键词: ReassignIDs, compressedHead 重映射
func TestTimelineReassignIDs_WithCompressedHead(t *testing.T) {
	timeline := NewTimeline(nil, nil)

	for i := 1; i <= 5; i++ {
		timeline.PushToolResult(&aitool.ToolResult{
			ID:          int64(1000 + i),
			Name:        "tool",
			Description: "test",
			Success:     true,
			Data:        i,
		})
	}

	// 模拟批量压缩后产生的 compressedHead
	timeline.compressedHead = &TimelineCompressedHead{
		Text:             "reducer-memory-1003",
		CoveredEndItemID: 1003,
		CoveredEndAtMs:   int64(1700000000000),
		Version:          1,
	}

	require.Equal(t, 5, timeline.idToTimelineItem.Len())

	var idCounter int64 = 100
	generator := func() int64 {
		return atomic.AddInt64(&idCounter, 1)
	}

	lastID := timeline.ReassignIDs(generator)

	require.Equal(t, int64(105), lastID)
	require.Equal(t, 5, timeline.idToTimelineItem.Len())

	// compressedHead 的 CoveredEndItemID 应被重映射到新 ID
	require.NotNil(t, timeline.compressedHead)
	// 旧 ID 1003 是第 3 个 item（1001→101, 1002→102, 1003→103）
	require.Equal(t, int64(103), timeline.compressedHead.CoveredEndItemID,
		"CoveredEndItemID should be remapped to new id 103")
	require.Equal(t, int64(1700000000000), timeline.compressedHead.CoveredEndAtMs,
		"CoveredEndAtMs must remain unchanged")
}

// TestTimelineReassignIDs_WithCompressedHead_NoActiveItemMapping 测试当 compressedHead.CoveredEndItemID
// 指向的旧 ID 不在 idToTimelineItem 中时（即已被软删除），ReassignIDs 同时重映射活跃条目与摘要边界
func TestTimelineReassignIDs_WithCompressedHead_NoActiveItemMapping(t *testing.T) {
	timeline := NewTimeline(nil, nil)

	// 添加数据
	for i := 1; i <= 3; i++ {
		timeline.PushToolResult(&aitool.ToolResult{
			ID:          int64(2000 + i),
			Name:        "tool",
			Description: "test",
			Success:     true,
			Data:        i,
		})
	}

	// 模拟 compressedHead 指向一个不在活跃 item 中的 ID（2002 已被软删除）
	timeline.SoftDelete(2002)
	timeline.compressedHead = &TimelineCompressedHead{
		Text:             "compressed",
		CoveredEndItemID: 2002,
		Version:          1,
	}

	require.Equal(t, 3, timeline.idToTimelineItem.Len())

	var idCounter int64 = 200
	generator := func() int64 {
		return atomic.AddInt64(&idCounter, 1)
	}

	// ReassignIDs 保留摘要边界占位
	lastID := timeline.ReassignIDs(generator)

	// 活跃 items 为 201、203，摘要覆盖边界为 202
	require.Equal(t, int64(203), lastID)
	require.EqualValues(t, 202, timeline.compressedHead.CoveredEndItemID)
	// 缺少原文也不能保留旧覆盖 ID
	require.NotNil(t, timeline.compressedHead)
}

// TestTimelineReassignIDs_PreserveOrder 测试是否保持时间顺序
func TestTimelineReassignIDs_PreserveOrder(t *testing.T) {
	timeline := NewTimeline(nil, nil)

	// 添加数据（ID 乱序，但时间顺序应该按照添加顺序）
	timeline.PushToolResult(&aitool.ToolResult{
		ID:   5000,
		Name: "first",
		Data: 1,
	})

	timeline.PushToolResult(&aitool.ToolResult{
		ID:   1000,
		Name: "second",
		Data: 2,
	})

	timeline.PushToolResult(&aitool.ToolResult{
		ID:   3000,
		Name: "third",
		Data: 3,
	})

	var idCounter int64 = 0
	generator := func() int64 {
		return atomic.AddInt64(&idCounter, 1)
	}

	timeline.ReassignIDs(generator)

	// 验证顺序保持不变（按照时间戳顺序）
	ids := timeline.GetTimelineItemIDs()
	require.Equal(t, 3, len(ids))

	// 通过时间戳顺序获取
	var names []string
	timeline.tsToTimelineItem.ForEach(func(ts int64, item *TimelineItem) bool {
		if tr, ok := item.GetValue().(*aitool.ToolResult); ok {
			names = append(names, tr.Name)
		}
		return true
	})

	require.Equal(t, []string{"first", "second", "third"}, names)
}

// TestTimelineReassignIDs_NilGenerator 测试 nil generator 的情况
func TestTimelineReassignIDs_NilGenerator(t *testing.T) {
	timeline := NewTimeline(nil, nil)
	timeline.PushText(100, "test")

	lastID := timeline.ReassignIDs(nil)
	require.Equal(t, int64(0), lastID)
	// timeline 应该保持不变
	require.Equal(t, 1, timeline.idToTimelineItem.Len())
}

// TestTimelineReassignIDs_LargeDataset 测试大数据集
func TestTimelineReassignIDs_LargeDataset(t *testing.T) {
	timeline := NewTimeline(nil, nil)

	// 添加100个条目
	for i := 1; i <= 100; i++ {
		timeline.PushToolResult(&aitool.ToolResult{
			ID:   int64(i * 1000), // 非连续 ID
			Name: "tool",
			Data: i,
		})
	}

	require.Equal(t, 100, timeline.idToTimelineItem.Len())

	var idCounter int64 = 0
	generator := func() int64 {
		return atomic.AddInt64(&idCounter, 1)
	}

	lastID := timeline.ReassignIDs(generator)

	require.Equal(t, int64(100), lastID)
	require.Equal(t, 100, timeline.idToTimelineItem.Len())

	// 验证所有 ID 都是连续的
	ids := timeline.GetTimelineItemIDs()
	for i := 0; i < len(ids); i++ {
		require.Equal(t, int64(i+1), ids[i])
	}
}

// TestTimelineReassignIDs_ConcurrentSafety 测试并发安全性
func TestTimelineReassignIDs_ConcurrentSafety(t *testing.T) {
	timeline := NewTimeline(nil, nil)

	// 添加测试数据
	for i := 1; i <= 10; i++ {
		timeline.PushToolResult(&aitool.ToolResult{
			ID:   int64(i * 100),
			Name: "tool",
			Data: i,
		})
	}

	var idCounter int64 = 0
	generator := func() int64 {
		return atomic.AddInt64(&idCounter, 1)
	}

	// 多次重新分配应该是安全的
	for i := 0; i < 5; i++ {
		idCounter = int64(i * 100)
		lastID := timeline.ReassignIDs(generator)
		require.Equal(t, int64(i*100+10), lastID)
		require.Equal(t, 10, timeline.idToTimelineItem.Len())
	}
}

func TestTimelineMarshalWithCompressedHead(t *testing.T) {
	originalTimeline := NewTimeline(nil, nil)

	// 添加一些工具结果
	for i := 1; i <= 5; i++ {
		originalTimeline.PushToolResult(&aitool.ToolResult{
			ID:          int64(100 + i),
			Name:        "test_tool",
			Description: "test description",
			Param:       map[string]any{"param": i},
			Success:     true,
			Data:        i,
			Error:       "",
		})
	}

	originalTimeline.compressedHead = &TimelineCompressedHead{
		Text:             "compressed memory",
		CoveredEndItemID: 200,
		CoveredEndAtMs:   1700000000000,
		Version:          3,
	}

	jsonStr, err := MarshalTimeline(originalTimeline)
	require.NoError(t, err)
	require.NotEmpty(t, jsonStr)

	restoredTimeline, err := UnmarshalTimeline(jsonStr)
	require.NoError(t, err)
	require.NotNil(t, restoredTimeline)

	require.NotNil(t, restoredTimeline.compressedHead)
	require.Equal(t, originalTimeline.compressedHead.Text, restoredTimeline.compressedHead.Text)
	require.Equal(t, originalTimeline.compressedHead.CoveredEndItemID, restoredTimeline.compressedHead.CoveredEndItemID)
	require.Equal(t, originalTimeline.compressedHead.CoveredEndAtMs, restoredTimeline.compressedHead.CoveredEndAtMs)
	require.Equal(t, originalTimeline.compressedHead.Version, restoredTimeline.compressedHead.Version)

	t.Log("Timeline marshal with compressed head test passed")
}

func TestUnmarshalTimelineLegacyReducers(t *testing.T) {
	// This shape was written before compressed_head replaced reducers.
	const legacy = `{"id_to_ts":{},"ts_to_timeline_item":{},"id_to_timeline_item":{},"reducers":{"11":"first memory","22":"second memory"},"reducer_ts":{"11":1700000000000,"22":1700000001000},"per_dump_content_limit":100,"total_dump_content_limit":500}`

	timeline, err := UnmarshalTimeline(legacy)
	require.NoError(t, err)
	require.Equal(t, &TimelineCompressedHead{
		Text:             "second memory",
		CoveredEndItemID: 22,
		CoveredEndAtMs:   1700000001000,
		Version:          2,
	}, timeline.compressedHead)
	require.Equal(t, []*TimelineCompressedHistoryNode{{
		Version:          1,
		PrevVersion:      0,
		Text:             "first memory",
		CoveredEndItemID: 11,
		CoveredEndAtMs:   1700000000000,
		CreatedAtMs:      1700000000000,
	}}, timeline.compressedHistory)

	serialized, err := MarshalTimeline(timeline)
	require.NoError(t, err)
	require.NotContains(t, serialized, `"reducers"`)
	require.NotContains(t, serialized, `"reducer_ts"`)
	require.Contains(t, serialized, `"compressed_head"`)
	require.Contains(t, serialized, `"compressed_history"`)
}
