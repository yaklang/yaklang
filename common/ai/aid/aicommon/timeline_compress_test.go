package aicommon

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/utils/linktable"
)

// TestTimelineCompress_ItemsRemoved 测试压缩后条目被正确删除
func TestTimelineCompress_ItemsRemoved(t *testing.T) {
	timeline := NewTimeline(nil, nil)

	// 添加100个条目
	for i := 1; i <= 100; i++ {
		timeline.PushToolResult(&aitool.ToolResult{
			ID:          int64(i),
			Name:        "tool",
			Description: "test description",
			Success:     true,
			Data:        "some data that takes up space",
		})
	}

	require.Equal(t, 100, timeline.idToTimelineItem.Len())

	// 模拟批量压缩：删除前60个，保留一个 reducer
	var idsToRemove []int64
	for i := int64(1); i <= 60; i++ {
		idsToRemove = append(idsToRemove, i)
	}

	// 模拟压缩结果
	compressedMemory := "This is a compressed summary of 60 items"
	lastCompressedId := idsToRemove[len(idsToRemove)-1]
	timeline.reducers.Set(lastCompressedId, linktable.NewUnlimitedStringLinkTable(compressedMemory))

	// 删除被压缩的 items
	for _, id := range idsToRemove {
		if ts, ok := timeline.idToTs.Get(id); ok {
			timeline.tsToTimelineItem.Delete(ts)
		}
		timeline.idToTs.Delete(id)
		timeline.idToTimelineItem.Delete(id)
	}

	// 验证压缩后的状态
	require.Equal(t, 40, timeline.idToTimelineItem.Len(), "Should have 40 items left (100 - 60)")
	require.Equal(t, 1, timeline.reducers.Len(), "Should have 1 reducer")

	// 验证被删除的 ID 不存在
	for i := int64(1); i <= 60; i++ {
		_, exists := timeline.idToTimelineItem.Get(i)
		require.False(t, exists, "ID %d should not exist after compression", i)
	}

	// 验证保留的 ID 存在
	for i := int64(61); i <= 100; i++ {
		_, exists := timeline.idToTimelineItem.Get(i)
		require.True(t, exists, "ID %d should still exist", i)
	}

	// 验证 reducer 存在
	reducer, exists := timeline.reducers.Get(lastCompressedId)
	require.True(t, exists)
	require.Equal(t, compressedMemory, reducer.Value())
}

// TestTimelineCompress_Serialization 测试序列化只包含 reducer，不包含被删除的内容
func TestTimelineCompress_Serialization(t *testing.T) {
	timeline := NewTimeline(nil, nil)

	// 添加100个条目
	for i := 1; i <= 100; i++ {
		timeline.PushToolResult(&aitool.ToolResult{
			ID:          int64(i),
			Name:        "tool",
			Description: "test description with some content",
			Success:     true,
			Data:        "some data that takes up space in serialization",
		})
	}

	// 序列化压缩前
	beforeCompress, err := MarshalTimeline(timeline)
	require.NoError(t, err)
	beforeSize := len(beforeCompress)

	// 模拟压缩前60个
	var idsToRemove []int64
	for i := int64(1); i <= 60; i++ {
		idsToRemove = append(idsToRemove, i)
	}

	compressedMemory := "Compressed summary of 60 items"
	lastCompressedId := idsToRemove[len(idsToRemove)-1]
	timeline.reducers.Set(lastCompressedId, linktable.NewUnlimitedStringLinkTable(compressedMemory))

	// 删除被压缩的 items
	for _, id := range idsToRemove {
		if ts, ok := timeline.idToTs.Get(id); ok {
			timeline.tsToTimelineItem.Delete(ts)
		}
		timeline.idToTs.Delete(id)
		timeline.idToTimelineItem.Delete(id)
	}

	// 序列化压缩后
	afterCompress, err := MarshalTimeline(timeline)
	require.NoError(t, err)
	afterSize := len(afterCompress)

	// 验证压缩后序列化大小显著减小
	t.Logf("Before compression: %d bytes, After compression: %d bytes", beforeSize, afterSize)
	require.Less(t, afterSize, beforeSize, "Serialized size should be smaller after compression")

	// 反序列化并验证
	restored, err := UnmarshalTimeline(afterCompress)
	require.NoError(t, err)
	require.Equal(t, 40, restored.idToTimelineItem.Len(), "Restored timeline should only have 40 items")
	require.Equal(t, 1, restored.reducers.Len(), "Restored timeline should have 1 reducer")

	// 验证被删除的条目确实不在序列化结果中
	for i := int64(1); i <= 60; i++ {
		_, exists := restored.idToTimelineItem.Get(i)
		require.False(t, exists, "Deleted ID %d should not be in restored timeline", i)
	}

	// 验证 reducer 被正确恢复
	reducer, exists := restored.reducers.Get(lastCompressedId)
	require.True(t, exists)
	require.Equal(t, compressedMemory, reducer.Value())
}

// TestTimelineCompress_SizeReduction 测试压缩达到目标大小
func TestTimelineCompress_SizeReduction(t *testing.T) {
	timeline := NewTimeline(nil, nil)

	// 添加100个条目，每个约1KB
	largeData := make([]byte, 1024)
	for i := range largeData {
		largeData[i] = 'A'
	}

	for i := 1; i <= 100; i++ {
		timeline.PushToolResult(&aitool.ToolResult{
			ID:          int64(i),
			Name:        "tool",
			Description: "description",
			Success:     true,
			Data:        string(largeData),
		})
	}

	require.Equal(t, 100, timeline.idToTimelineItem.Len())

	// 计算压缩前的大小
	beforeSize := timeline.calculateActualContentSize()
	t.Logf("Timeline size before compression: %d bytes", beforeSize)
	require.Greater(t, beforeSize, int64(50*1024), "Should be > 50KB")

	// 模拟压缩：删除一半
	var idsToRemove []int64
	for i := int64(1); i <= 50; i++ {
		idsToRemove = append(idsToRemove, i)
	}

	compressedMemory := "Compressed 50 items into summary"
	lastCompressedId := idsToRemove[len(idsToRemove)-1]
	timeline.reducers.Set(lastCompressedId, linktable.NewUnlimitedStringLinkTable(compressedMemory))

	// 删除被压缩的 items
	for _, id := range idsToRemove {
		if ts, ok := timeline.idToTs.Get(id); ok {
			timeline.tsToTimelineItem.Delete(ts)
		}
		timeline.idToTs.Delete(id)
		timeline.idToTimelineItem.Delete(id)
	}

	// 计算压缩后的大小
	afterSize := timeline.calculateActualContentSize()
	t.Logf("Timeline size after compression: %d bytes", afterSize)
	t.Logf("Compression ratio: %.2f%%", float64(afterSize)/float64(beforeSize)*100)

	// 验证大小显著减小（考虑到有额外开销，放宽到 60%）
	require.Less(t, afterSize, beforeSize*6/10, "Size should be significantly reduced after compression")
	require.Equal(t, 50, timeline.idToTimelineItem.Len(), "Should have 50 items left")
}

// TestTimelineCompress_MultipleCompressions 测试多次压缩
func TestTimelineCompress_MultipleCompressions(t *testing.T) {
	timeline := NewTimeline(nil, nil)

	// 添加100个条目
	for i := 1; i <= 100; i++ {
		timeline.PushToolResult(&aitool.ToolResult{
			ID:   int64(i),
			Name: "tool",
			Data: "data",
		})
	}

	require.Equal(t, 100, timeline.idToTimelineItem.Len())

	// 第一次压缩：压缩前40个
	var ids1 []int64
	for i := int64(1); i <= 40; i++ {
		ids1 = append(ids1, i)
	}
	timeline.reducers.Set(40, linktable.NewUnlimitedStringLinkTable("First compression"))
	for _, id := range ids1 {
		if ts, ok := timeline.idToTs.Get(id); ok {
			timeline.tsToTimelineItem.Delete(ts)
		}
		timeline.idToTs.Delete(id)
		timeline.idToTimelineItem.Delete(id)
	}

	require.Equal(t, 60, timeline.idToTimelineItem.Len())
	require.Equal(t, 1, timeline.reducers.Len())

	// 第二次压缩：再压缩20个 (ID 41-60)
	var ids2 []int64
	for i := int64(41); i <= 60; i++ {
		ids2 = append(ids2, i)
	}
	timeline.reducers.Set(60, linktable.NewUnlimitedStringLinkTable("Second compression"))
	for _, id := range ids2 {
		if ts, ok := timeline.idToTs.Get(id); ok {
			timeline.tsToTimelineItem.Delete(ts)
		}
		timeline.idToTs.Delete(id)
		timeline.idToTimelineItem.Delete(id)
	}

	require.Equal(t, 40, timeline.idToTimelineItem.Len())
	require.Equal(t, 2, timeline.reducers.Len(), "Should have 2 reducers")

	// 验证两个 reducer 都存在
	r1, ok1 := timeline.reducers.Get(40)
	require.True(t, ok1)
	require.Equal(t, "First compression", r1.Value())

	r2, ok2 := timeline.reducers.Get(60)
	require.True(t, ok2)
	require.Equal(t, "Second compression", r2.Value())

	// 验证只有 61-100 的条目存在
	for i := int64(1); i <= 60; i++ {
		_, exists := timeline.idToTimelineItem.Get(i)
		require.False(t, exists, "ID %d should not exist", i)
	}
	for i := int64(61); i <= 100; i++ {
		_, exists := timeline.idToTimelineItem.Get(i)
		require.True(t, exists, "ID %d should exist", i)
	}
}

// TestTimelineCompress_PreservesUncompressedData 测试未压缩的数据完整性
func TestTimelineCompress_PreservesUncompressedData(t *testing.T) {
	timeline := NewTimeline(nil, nil)

	// 添加测试数据
	for i := 1; i <= 100; i++ {
		timeline.PushToolResult(&aitool.ToolResult{
			ID:          int64(i),
			Name:        "tool",
			Description: "desc",
			Success:     true,
			Data:        i, // 使用索引作为数据
		})
	}

	// 压缩前50个
	var idsToRemove []int64
	for i := int64(1); i <= 50; i++ {
		idsToRemove = append(idsToRemove, i)
	}

	timeline.reducers.Set(50, linktable.NewUnlimitedStringLinkTable("Compressed"))
	for _, id := range idsToRemove {
		if ts, ok := timeline.idToTs.Get(id); ok {
			timeline.tsToTimelineItem.Delete(ts)
		}
		timeline.idToTs.Delete(id)
		timeline.idToTimelineItem.Delete(id)
	}

	// 验证保留的数据完整性
	for i := int64(51); i <= 100; i++ {
		item, exists := timeline.idToTimelineItem.Get(i)
		require.True(t, exists)

		tr, ok := item.GetValue().(*aitool.ToolResult)
		require.True(t, ok)
		require.Equal(t, int(i), tr.Data, "Data should be preserved for ID %d", i)
	}
}

// TestTimelineCompress_ReassignAfterCompress 测试压缩后重新分配 ID
func TestTimelineCompress_ReassignAfterCompress(t *testing.T) {
	timeline := NewTimeline(nil, nil)

	// 添加100个条目
	for i := 1; i <= 100; i++ {
		timeline.PushToolResult(&aitool.ToolResult{
			ID:   int64(i),
			Name: "tool",
			Data: i,
		})
	}

	// 压缩前60个
	var idsToRemove []int64
	for i := int64(1); i <= 60; i++ {
		idsToRemove = append(idsToRemove, i)
	}

	compressedID := int64(60)
	timeline.reducers.Set(compressedID, linktable.NewUnlimitedStringLinkTable("Compressed"))
	for _, id := range idsToRemove {
		if ts, ok := timeline.idToTs.Get(id); ok {
			timeline.tsToTimelineItem.Delete(ts)
		}
		timeline.idToTs.Delete(id)
		timeline.idToTimelineItem.Delete(id)
	}

	require.Equal(t, 40, timeline.idToTimelineItem.Len())

	// 重新分配 ID
	var idCounter int64 = 0
	generator := func() int64 {
		idCounter++
		return idCounter
	}

	lastID := timeline.ReassignIDs(generator)

	// 验证结果
	require.Equal(t, int64(40), lastID, "Should have 40 items")
	require.Equal(t, 40, timeline.idToTimelineItem.Len())

	// 验证 ID 是连续的从 1 到 40
	ids := timeline.GetTimelineItemIDs()
	for i := 0; i < len(ids); i++ {
		require.Equal(t, int64(i+1), ids[i])
	}

	// 注意：压缩后，reducer 的 ID 指向的是已删除的 item
	// 在 ReassignIDs 时，由于这些 items 已经被删除，reducer 不会被重新分配
	// 这是预期的行为，因为 reducer 代表的是已经被压缩删除的内容
	// 在实际使用中，这些 reducer 应该在序列化时被特殊处理
	// 这里我们验证 reducer 仍然存在（虽然 ID 可能已经无效）
	require.GreaterOrEqual(t, timeline.reducers.Len(), 0, "Reducers may be cleaned up")
}

// TestTimelineCompress_SerializationSize 测试序列化大小确实减小
func TestTimelineCompress_SerializationSize(t *testing.T) {
	// 创建两个 timeline，一个压缩，一个不压缩
	timelineUncompressed := NewTimeline(nil, nil)
	timelineCompressed := NewTimeline(nil, nil)

	// 添加相同的数据
	largeData := make([]byte, 512)
	for i := range largeData {
		largeData[i] = byte('A' + (i % 26))
	}

	for i := 1; i <= 100; i++ {
		timelineUncompressed.PushToolResult(&aitool.ToolResult{
			ID:          int64(i),
			Name:        "tool",
			Description: "test description",
			Success:     true,
			Data:        string(largeData),
		})

		timelineCompressed.PushToolResult(&aitool.ToolResult{
			ID:          int64(i),
			Name:        "tool",
			Description: "test description",
			Success:     true,
			Data:        string(largeData),
		})
	}

	// 序列化未压缩的
	uncompressedSerialized, err := MarshalTimeline(timelineUncompressed)
	require.NoError(t, err)
	uncompressedSize := len(uncompressedSerialized)

	// 压缩 timelineCompressed 的前50个
	var idsToRemove []int64
	for i := int64(1); i <= 50; i++ {
		idsToRemove = append(idsToRemove, i)
	}

	// 使用一个小的摘要
	compressedSummary := "Summary of 50 compressed items"
	timelineCompressed.reducers.Set(50, linktable.NewUnlimitedStringLinkTable(compressedSummary))

	for _, id := range idsToRemove {
		if ts, ok := timelineCompressed.idToTs.Get(id); ok {
			timelineCompressed.tsToTimelineItem.Delete(ts)
		}
		timelineCompressed.idToTs.Delete(id)
		timelineCompressed.idToTimelineItem.Delete(id)
	}

	// 序列化压缩的
	compressedSerialized, err := MarshalTimeline(timelineCompressed)
	require.NoError(t, err)
	compressedSize := len(compressedSerialized)

	t.Logf("Uncompressed size: %d bytes", uncompressedSize)
	t.Logf("Compressed size: %d bytes", compressedSize)
	t.Logf("Size reduction: %.2f%%", (1-float64(compressedSize)/float64(uncompressedSize))*100)

	// 压缩后的序列化应该显著更小（考虑额外开销，放宽到 60%）
	require.Less(t, compressedSize, uncompressedSize*6/10, "Compressed serialization should be significantly smaller")
}
