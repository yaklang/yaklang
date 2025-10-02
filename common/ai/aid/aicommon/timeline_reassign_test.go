package aicommon

import (
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

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

// TestTimelineReassignIDs_WithSummary 测试包含 summary 的 timeline
func TestTimelineReassignIDs_WithSummary(t *testing.T) {
	timeline := NewTimeline(nil, nil)

	// 添加数据
	for i := 1; i <= 5; i++ {
		timeline.PushToolResult(&aitool.ToolResult{
			ID:          int64(1000 + i),
			Name:        "tool",
			Description: "test",
			Success:     true,
			Data:        i,
		})
	}

	// 模拟添加 summary
	item := &TimelineItem{
		value: &aitool.ToolResult{
			ID:           1003,
			ShrinkResult: "summarized content",
		},
	}
	timeline.summary.Set(1003, nil) // 简化测试，不需要完整的 LinkTable

	require.Equal(t, 5, timeline.idToTimelineItem.Len())
	require.Equal(t, 1, timeline.summary.Len())

	var idCounter int64 = 100
	generator := func() int64 {
		return atomic.AddInt64(&idCounter, 1)
	}

	lastID := timeline.ReassignIDs(generator)

	require.Equal(t, int64(105), lastID)
	require.Equal(t, 5, timeline.idToTimelineItem.Len())

	// 验证 summary 被正确更新
	require.Equal(t, 1, timeline.summary.Len())
	// 旧的 ID 1003 应该不存在了
	_, exists := timeline.summary.Get(1003)
	require.False(t, exists, "Old summary ID should not exist")

	_ = item // 避免未使用变量警告
}

// TestTimelineReassignIDs_WithReducers 测试包含 reducers 的 timeline
func TestTimelineReassignIDs_WithReducers(t *testing.T) {
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

	// 模拟添加 reducer
	timeline.reducers.Set(2002, nil) // 简化测试

	require.Equal(t, 3, timeline.idToTimelineItem.Len())
	require.Equal(t, 1, timeline.reducers.Len())

	var idCounter int64 = 200
	generator := func() int64 {
		return atomic.AddInt64(&idCounter, 1)
	}

	lastID := timeline.ReassignIDs(generator)

	require.Equal(t, int64(203), lastID)
	require.Equal(t, 3, timeline.idToTimelineItem.Len())

	// 验证 reducers 被正确更新
	require.Equal(t, 1, timeline.reducers.Len())
	// 旧的 ID 2002 应该不存在了
	_, exists := timeline.reducers.Get(2002)
	require.False(t, exists, "Old reducer ID should not exist")
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
