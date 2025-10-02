package aicommon

import (
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/utils/linktable"
)

// TestTimelineIntegration_CompressSerializeRestore 测试完整的压缩-序列化-恢复-重分配流程
func TestTimelineIntegration_CompressSerializeRestore(t *testing.T) {
	// 第一步：创建原始 timeline
	originalTimeline := NewTimeline(nil, nil)

	// 添加100个条目
	largeData := make([]byte, 512)
	for i := range largeData {
		largeData[i] = byte('A' + (i % 26))
	}

	for i := 1; i <= 100; i++ {
		originalTimeline.PushToolResult(&aitool.ToolResult{
			ID:          int64(i * 100), // 非连续 ID
			Name:        "tool",
			Description: "test",
			Success:     true,
			Data:        string(largeData),
		})
	}

	require.Equal(t, 100, originalTimeline.idToTimelineItem.Len())

	// 第二步：模拟压缩前60个条目
	var idsToCompress []int64
	for i := 1; i <= 60; i++ {
		idsToCompress = append(idsToCompress, int64(i*100))
	}

	compressedSummary := "Compressed summary of 60 items"
	lastCompressedID := idsToCompress[len(idsToCompress)-1]
	originalTimeline.reducers.Set(lastCompressedID, linktable.NewUnlimitedStringLinkTable(compressedSummary))

	// 删除被压缩的条目
	for _, id := range idsToCompress {
		if ts, ok := originalTimeline.idToTs.Get(id); ok {
			originalTimeline.tsToTimelineItem.Delete(ts)
		}
		originalTimeline.idToTs.Delete(id)
		originalTimeline.idToTimelineItem.Delete(id)
	}

	require.Equal(t, 40, originalTimeline.idToTimelineItem.Len())
	require.Equal(t, 1, originalTimeline.reducers.Len())

	// 第三步：序列化
	serialized, err := MarshalTimeline(originalTimeline)
	require.NoError(t, err)
	require.NotEmpty(t, serialized)

	t.Logf("Serialized size: %d bytes", len(serialized))

	// 第四步：反序列化（模拟从数据库恢复）
	restoredTimeline, err := UnmarshalTimeline(serialized)
	require.NoError(t, err)
	require.NotNil(t, restoredTimeline)

	// 验证恢复的数据
	require.Equal(t, 40, restoredTimeline.idToTimelineItem.Len(), "Should have 40 items")
	require.Equal(t, 1, restoredTimeline.reducers.Len(), "Should have 1 reducer")

	// 验证被压缩的条目确实不在恢复的 timeline 中
	for _, id := range idsToCompress {
		_, exists := restoredTimeline.idToTimelineItem.Get(id)
		require.False(t, exists, "Compressed ID %d should not exist", id)
	}

	// 验证 reducer 被正确恢复
	reducer, exists := restoredTimeline.reducers.Get(lastCompressedID)
	require.True(t, exists)
	require.Equal(t, compressedSummary, reducer.Value())

	// 第五步：重新分配 ID（模拟 persistent session 恢复）
	var idCounter int64 = 1000
	generator := func() int64 {
		return atomic.AddInt64(&idCounter, 1)
	}

	lastID := restoredTimeline.ReassignIDs(generator)

	// 验证重新分配的结果
	require.Equal(t, int64(1040), lastID, "Should assign 40 IDs from 1001 to 1040")
	require.Equal(t, 40, restoredTimeline.idToTimelineItem.Len())

	// 验证 ID 是连续的
	ids := restoredTimeline.GetTimelineItemIDs()
	for i := 0; i < len(ids); i++ {
		require.Equal(t, int64(1001+i), ids[i], "IDs should be sequential from 1001")
	}

	// 验证数据完整性
	for _, id := range ids {
		item, exists := restoredTimeline.idToTimelineItem.Get(id)
		require.True(t, exists)

		tr, ok := item.GetValue().(*aitool.ToolResult)
		require.True(t, ok)
		require.Equal(t, "tool", tr.Name)
		require.Equal(t, id, tr.ID, "ID should be updated")
	}

	t.Log("Complete integration test passed: compress -> serialize -> restore -> reassign")
}

// TestTimelineIntegration_MultipleSessionRestores 测试多次会话恢复
func TestTimelineIntegration_MultipleSessionRestores(t *testing.T) {
	// 模拟第一个会话
	session1 := NewTimeline(nil, nil)
	for i := 1; i <= 50; i++ {
		session1.PushToolResult(&aitool.ToolResult{
			ID:   int64(i),
			Name: "tool",
			Data: i,
		})
	}

	// 序列化第一个会话
	serialized1, err := MarshalTimeline(session1)
	require.NoError(t, err)

	// 恢复并重新分配 ID
	restored1, err := UnmarshalTimeline(serialized1)
	require.NoError(t, err)

	var counter1 int64 = 0
	generator1 := func() int64 {
		return atomic.AddInt64(&counter1, 1)
	}
	lastID1 := restored1.ReassignIDs(generator1)
	require.Equal(t, int64(50), lastID1)

	// 继续添加数据到恢复的 timeline
	for i := 51; i <= 100; i++ {
		restored1.PushToolResult(&aitool.ToolResult{
			ID:   atomic.AddInt64(&counter1, 1),
			Name: "tool",
			Data: i,
		})
	}

	require.Equal(t, 100, restored1.idToTimelineItem.Len())

	// 压缩前30个
	var idsToCompress []int64
	for i := int64(1); i <= 30; i++ {
		idsToCompress = append(idsToCompress, i)
	}

	restored1.reducers.Set(30, linktable.NewUnlimitedStringLinkTable("Compressed"))
	for _, id := range idsToCompress {
		if ts, ok := restored1.idToTs.Get(id); ok {
			restored1.tsToTimelineItem.Delete(ts)
		}
		restored1.idToTs.Delete(id)
		restored1.idToTimelineItem.Delete(id)
	}

	require.Equal(t, 70, restored1.idToTimelineItem.Len())

	// 第二次序列化
	serialized2, err := MarshalTimeline(restored1)
	require.NoError(t, err)

	// 第二次恢复
	restored2, err := UnmarshalTimeline(serialized2)
	require.NoError(t, err)

	var counter2 int64 = 2000
	generator2 := func() int64 {
		return atomic.AddInt64(&counter2, 1)
	}
	lastID2 := restored2.ReassignIDs(generator2)

	require.Equal(t, int64(2070), lastID2)
	require.Equal(t, 70, restored2.idToTimelineItem.Len())

	// 验证 ID 的连续性
	ids := restored2.GetTimelineItemIDs()
	for i := 0; i < len(ids); i++ {
		require.Equal(t, int64(2001+i), ids[i])
	}

	t.Log("Multiple session restore test passed")
}

// TestTimelineIntegration_LargeScaleCompression 测试大规模压缩场景
func TestTimelineIntegration_LargeScaleCompression(t *testing.T) {
	timeline := NewTimeline(nil, nil)

	// 添加1000个条目
	for i := 1; i <= 1000; i++ {
		timeline.PushToolResult(&aitool.ToolResult{
			ID:          int64(i),
			Name:        "tool",
			Description: "some description",
			Success:     true,
			Data:        "data",
		})
	}

	require.Equal(t, 1000, timeline.idToTimelineItem.Len())

	// 模拟批量压缩：压缩前900个，保留100个
	var idsToCompress []int64
	for i := int64(1); i <= 900; i++ {
		idsToCompress = append(idsToCompress, i)
	}

	// 每100个创建一个 reducer
	for i := 0; i < 9; i++ {
		endID := int64((i + 1) * 100)
		timeline.reducers.Set(endID, linktable.NewUnlimitedStringLinkTable("Compressed batch"))
	}

	// 删除被压缩的条目
	for _, id := range idsToCompress {
		if ts, ok := timeline.idToTs.Get(id); ok {
			timeline.tsToTimelineItem.Delete(ts)
		}
		timeline.idToTs.Delete(id)
		timeline.idToTimelineItem.Delete(id)
	}

	require.Equal(t, 100, timeline.idToTimelineItem.Len())
	require.Equal(t, 9, timeline.reducers.Len())

	// 序列化
	serialized, err := MarshalTimeline(timeline)
	require.NoError(t, err)

	t.Logf("1000 items compressed to 100, serialized size: %d bytes", len(serialized))

	// 反序列化
	restored, err := UnmarshalTimeline(serialized)
	require.NoError(t, err)

	require.Equal(t, 100, restored.idToTimelineItem.Len())
	require.Equal(t, 9, restored.reducers.Len())

	// 验证被压缩的条目不存在
	for i := int64(1); i <= 900; i++ {
		_, exists := restored.idToTimelineItem.Get(i)
		require.False(t, exists)
	}

	// 验证保留的条目存在
	for i := int64(901); i <= 1000; i++ {
		_, exists := restored.idToTimelineItem.Get(i)
		require.True(t, exists)
	}

	// 重新分配 ID
	var idCounter int64 = 0
	generator := func() int64 {
		return atomic.AddInt64(&idCounter, 1)
	}

	lastID := restored.ReassignIDs(generator)
	require.Equal(t, int64(100), lastID)

	t.Log("Large scale compression test passed")
}

// TestTimelineIntegration_EmptyToCompressed 测试从空 timeline 到压缩的完整流程
func TestTimelineIntegration_EmptyToCompressed(t *testing.T) {
	timeline := NewTimeline(nil, nil)

	// 初始为空
	require.Equal(t, 0, timeline.idToTimelineItem.Len())

	// 序列化空 timeline
	serialized1, err := MarshalTimeline(timeline)
	require.NoError(t, err)

	// 恢复空 timeline
	restored1, err := UnmarshalTimeline(serialized1)
	require.NoError(t, err)
	require.Equal(t, 0, restored1.idToTimelineItem.Len())

	// 添加数据
	var idCounter int64 = 0
	generator := func() int64 {
		return atomic.AddInt64(&idCounter, 1)
	}

	for i := 0; i < 200; i++ {
		restored1.PushToolResult(&aitool.ToolResult{
			ID:   generator(),
			Name: "tool",
			Data: i,
		})
	}

	require.Equal(t, 200, restored1.idToTimelineItem.Len())

	// 压缩前150个
	var idsToCompress []int64
	for i := int64(1); i <= 150; i++ {
		idsToCompress = append(idsToCompress, i)
	}

	restored1.reducers.Set(150, linktable.NewUnlimitedStringLinkTable("Large compression"))
	for _, id := range idsToCompress {
		if ts, ok := restored1.idToTs.Get(id); ok {
			restored1.tsToTimelineItem.Delete(ts)
		}
		restored1.idToTs.Delete(id)
		restored1.idToTimelineItem.Delete(id)
	}

	require.Equal(t, 50, restored1.idToTimelineItem.Len())

	// 第二次序列化（带压缩）
	serialized2, err := MarshalTimeline(restored1)
	require.NoError(t, err)

	t.Logf("After compression, size: %d bytes", len(serialized2))

	// 第二次恢复
	restored2, err := UnmarshalTimeline(serialized2)
	require.NoError(t, err)

	require.Equal(t, 50, restored2.idToTimelineItem.Len())
	require.Equal(t, 1, restored2.reducers.Len())

	// 重新分配 ID
	idCounter = 5000
	lastID := restored2.ReassignIDs(generator)

	require.Equal(t, int64(5050), lastID)
	require.Equal(t, 50, restored2.idToTimelineItem.Len())

	t.Log("Empty to compressed integration test passed")
}
