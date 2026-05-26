package aicommon

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

type mockTimelineArchiveStore struct {
	batches []*TimelineArchiveBatch
}

func (m *mockTimelineArchiveStore) ArchiveCompressedBatch(ctx context.Context, batch *TimelineArchiveBatch) (*TimelineArchiveRef, error) {
	_ = ctx
	m.batches = append(m.batches, batch)
	return &TimelineArchiveRef{
		ArchiveID:      batch.ArchiveID,
		Reason:         batch.Reason,
		SummaryPreview: batch.Summary,
		ReducerKeyID:   batch.ReducerKeyID,
		SourceStartID:  batch.SourceStartID,
		SourceEndID:    batch.SourceEndID,
		ItemCount:      batch.ItemCount,
	}, nil
}

func (m *mockTimelineArchiveStore) SearchArchivedBatches(context.Context, *TimelineArchiveSearchQuery) (*TimelineArchiveSearchResult, error) {
	return &TimelineArchiveSearchResult{}, nil
}

// TestTimelineCompress_ItemsRemoved 测试压缩后条目被正确标记为非活跃（inactive）
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

	// 模拟批量压缩：把前60个标记为 inactive，设置 compressedHead
	compressedMemory := "This is a compressed summary of 60 items"
	lastCompressedId := int64(60)
	timeline.compressedHead = &TimelineCompressedHead{
		Text:             compressedMemory,
		CoveredEndItemID: lastCompressedId,
		Version:          1,
	}
	for i := int64(1); i <= 60; i++ {
		timeline.SoftDelete(i)
	}

	// 验证活跃 item 数量（底层 map 仍有 100 条，但活跃只有 40）
	activeIDs := timeline.getActiveTimelineItemIDs()
	require.Equal(t, 40, len(activeIDs), "Should have 40 active items (100 - 60)")

	// 验证被压缩的 ID 确实已 inactive
	for i := int64(1); i <= 60; i++ {
		item, exists := timeline.idToTimelineItem.Get(i)
		require.True(t, exists, "ID %d should still exist in map", i)
		require.True(t, item.deleted, "ID %d should be marked inactive", i)
	}

	// 验证保留的 ID 仍活跃
	for i := int64(61); i <= 100; i++ {
		item, exists := timeline.idToTimelineItem.Get(i)
		require.True(t, exists, "ID %d should still exist", i)
		require.False(t, item.deleted, "ID %d should be active", i)
	}

	// 验证 compressedHead 存在
	require.NotNil(t, timeline.compressedHead)
	require.Equal(t, compressedMemory, timeline.compressedHead.Text)
}

// TestTimelineCompress_Serialization 测试序列化只包含 compressedHead，不包含被压缩的内容
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
	timeline.compressedHead = &TimelineCompressedHead{
		Text:             compressedMemory,
		CoveredEndItemID: lastCompressedId,
		Version:          1,
	}

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
	require.NotNil(t, restored.compressedHead, "Restored timeline should have compressed head")

	// 验证被删除的条目确实不在序列化结果中
	for i := int64(1); i <= 60; i++ {
		_, exists := restored.idToTimelineItem.Get(i)
		require.False(t, exists, "Deleted ID %d should not be in restored timeline", i)
	}

	require.Equal(t, compressedMemory, restored.compressedHead.Text)
	require.Equal(t, lastCompressedId, restored.compressedHead.CoveredEndItemID)
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

	beforeSize := timeline.calculateActualContentSize()
	t.Logf("Timeline size before compression: %d tokens", beforeSize)
	require.Greater(t, beforeSize, int64(10*1024), "Should be > 10K tokens")

	// 模拟压缩：删除一半
	var idsToRemove []int64
	for i := int64(1); i <= 50; i++ {
		idsToRemove = append(idsToRemove, i)
	}

	compressedMemory := "Compressed 50 items into summary"
	lastCompressedId := int64(50)
	timeline.compressedHead = &TimelineCompressedHead{
		Text:             compressedMemory,
		CoveredEndItemID: lastCompressedId,
		Version:          1,
	}

	// 标记被压缩的 items 为 inactive
	for _, id := range idsToRemove {
		timeline.SoftDelete(id)
	}

	// 计算压缩后的大小
	afterSize := timeline.calculateActualContentSize()
	t.Logf("Timeline size after compression: %d bytes", afterSize)
	t.Logf("Compression ratio: %.2f%%", float64(afterSize)/float64(beforeSize)*100)

	// 验证大小显著减小（考虑到有额外开销，放宽到 60%）
	require.Less(t, afterSize, beforeSize*6/10, "Size should be significantly reduced after compression")
	activeAfter := timeline.getActiveTimelineItemIDs()
	require.Equal(t, 50, len(activeAfter), "Should have 50 active items left")
}

func TestTimelineArchiveMergedContent_IncludesTimelineDetails(t *testing.T) {
	timeline := NewTimeline(nil, nil)
	timeline.PushText(1, "first timeline note")
	timeline.PushUserInteraction(UserInteractionStage_Review, 2, "system prompt", "user answer")

	items := timeline.idToTimelineItem.Values()
	merged := timelineArchiveMergedContent(items)

	require.Contains(t, merged, "id=1")
	require.Contains(t, merged, "first timeline note")
	require.Contains(t, merged, "id=2")
	require.Contains(t, merged, "system prompt")
	require.Contains(t, merged, "user answer")
}

// TestTimelineCompress_MultipleCompressions 测试多次压缩（单有效压缩段不变式）
// 新模型：每次压缩产生新 head，旧 head 入 history，运行态只有一个有效段
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
	for i := int64(1); i <= 40; i++ {
		timeline.SoftDelete(i)
	}
	timeline.updateCompressedHead(&TimelineCompressedHead{
		Text:             "First compression",
		CoveredEndItemID: 40,
		CoveredEndAtMs:   0,
	})

	activeAfter1 := timeline.getActiveTimelineItemIDs()
	require.Equal(t, 60, len(activeAfter1), "Should have 60 active items after first compression")
	require.NotNil(t, timeline.compressedHead, "Should have 1 compressed head")
	require.Equal(t, "First compression", timeline.compressedHead.Text)
	require.Equal(t, int64(40), timeline.compressedHead.CoveredEndItemID)
	require.Equal(t, int64(1), timeline.compressedHead.Version)

	// 第二次压缩：再压缩20个 (ID 41-60)
	for i := int64(41); i <= 60; i++ {
		timeline.SoftDelete(i)
	}
	timeline.updateCompressedHead(&TimelineCompressedHead{
		Text:             "Second compression",
		CoveredEndItemID: 60,
		CoveredEndAtMs:   0,
	})

	activeAfter2 := timeline.getActiveTimelineItemIDs()
	require.Equal(t, 40, len(activeAfter2), "Should have 40 active items after second compression")

	// 单有效段不变式：只有一个 compressedHead
	require.NotNil(t, timeline.compressedHead)
	require.Equal(t, "Second compression", timeline.compressedHead.Text)
	require.Equal(t, int64(60), timeline.compressedHead.CoveredEndItemID)
	require.Equal(t, int64(2), timeline.compressedHead.Version)

	// 旧 head 应进入 history
	require.Equal(t, 1, len(timeline.compressedHistory))
	require.Equal(t, "First compression", timeline.compressedHistory[0].Text)

	// 验证只有 61-100 的条目处于活跃
	activeIDs := timeline.getActiveTimelineItemIDs()
	for _, id := range activeIDs {
		require.True(t, id >= 61 && id <= 100, "Active item ID %d should be in range 61-100", id)
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

	timeline.compressedHead = &TimelineCompressedHead{
		Text:             "Compressed",
		CoveredEndItemID: 50,
		Version:          1,
	}
	for _, id := range idsToRemove {
		timeline.SoftDelete(id)
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
	timeline.compressedHead = &TimelineCompressedHead{
		Text:             "Compressed",
		CoveredEndItemID: compressedID,
		Version:          1,
	}
	for _, id := range idsToRemove {
		timeline.SoftDelete(id)
	}

	activeAfter := timeline.getActiveTimelineItemIDs()
	require.Equal(t, 40, len(activeAfter))

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

	// 注意：compressedHead 的 CoveredEndItemID 指向的是已被软删除的 item ID
	// ReassignIDs 会尝试重映射 compressedHead.CoveredEndItemID，但由于这些 items
	// 已标记为 deleted（软删除），不会参与重新分配
	require.NotNil(t, timeline.compressedHead, "compressedHead should still exist after ReassignIDs")
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
	timelineCompressed.compressedHead = &TimelineCompressedHead{
		Text:             compressedSummary,
		CoveredEndItemID: 50,
		Version:          1,
	}

	for _, id := range idsToRemove {
		timelineCompressed.SoftDelete(id)
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

func TestTimelineCompress_EmergencyArchiveWritesMidtermStore(t *testing.T) {
	timeline := NewTimeline(nil, nil)
	store := &mockTimelineArchiveStore{}
	cfg := &Config{
		PersistentSessionId:  "persistent-session-1",
		TimelineArchiveStore: store,
	}
	timeline.SoftBindConfig(cfg, nil)

	largeData := make([]byte, 1024)
	for i := range largeData {
		largeData[i] = 'B'
	}

	for i := 1; i <= 12; i++ {
		timeline.PushToolResult(&aitool.ToolResult{
			ID:          int64(i),
			Name:        "tool",
			Description: "test tool",
			Success:     true,
			Data:        string(largeData),
		})
	}

	timeline.emergencyCompress(2 * 1024)

	require.Len(t, store.batches, 1)
	batch := store.batches[0]
	require.Equal(t, TimelineArchiveReasonEmergencyCompress, batch.Reason)
	require.Equal(t, "persistent-session-1", batch.PersistentSessionID)
	require.Greater(t, batch.ItemCount, 0)
	require.NotEmpty(t, batch.Summary)
	require.Len(t, timeline.archiveRefs.Values(), 1)
}

func TestTimelineCompress_SerializationPreservesArchiveRefs(t *testing.T) {
	timeline := NewTimeline(nil, nil)
	ref := &TimelineArchiveRef{
		ArchiveID:      "timeline-archive-1",
		Reason:         TimelineArchiveReasonBatchCompress,
		SummaryPreview: "compressed old tool outputs",
		ReducerKeyID:   42,
		SourceStartID:  1,
		SourceEndID:    42,
		ItemCount:      42,
	}
	timeline.attachArchiveRef(42, ref)

	serialized, err := MarshalTimeline(timeline)
	require.NoError(t, err)

	restored, err := UnmarshalTimeline(serialized)
	require.NoError(t, err)

	got, ok := restored.archiveRefs.Get(42)
	require.True(t, ok)
	require.Equal(t, ref.ArchiveID, got.ArchiveID)
	require.Equal(t, ref.SummaryPreview, got.SummaryPreview)
	require.Equal(t, ref.ItemCount, got.ItemCount)
}
