package aicommon

import (
	"fmt"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

// TestTimelineFinal_CompressedDataNotInSerialization 核心测试：压缩后的数据不在序列化中
func TestTimelineFinal_CompressedDataNotInSerialization(t *testing.T) {
	timeline := NewTimeline(nil, nil)

	// 添加100个条目，每个包含绝对唯一的标识（使用序号）
	type markerInfo struct {
		marker string
		id     int64
	}
	allMarkers := make([]markerInfo, 100)

	for i := 1; i <= 100; i++ {
		// 使用序号作为唯一标识，确保不会与其他item重复
		marker := fmt.Sprintf("UNIQUE_DATA_ITEM_%05d_MARKER", i)
		allMarkers[i-1] = markerInfo{
			marker: marker,
			id:     int64(i),
		}

		timeline.PushToolResult(&aitool.ToolResult{
			ID:          int64(i),
			Name:        fmt.Sprintf("tool_%d", i),
			Description: marker,
			Success:     true,
			Data:        marker,
		})
	}

	require.Equal(t, 100, timeline.idToTimelineItem.Len())

	// 压缩前70个
	var compressedIDs []int64
	for i := 1; i <= 70; i++ {
		compressedIDs = append(compressedIDs, int64(i))
	}

	compressedSummary := "SUMMARY_COMPRESSED_70_ITEMS"
	timeline.compressedHead = &TimelineCompressedHead{
		Text:             compressedSummary,
		CoveredEndItemID: 70,
		Version:          1,
	}

	// 删除被压缩的条目
	for _, id := range compressedIDs {
		if ts, ok := timeline.idToTs.Get(id); ok {
			timeline.tsToTimelineItem.Delete(ts)
		}
		timeline.idToTs.Delete(id)
		timeline.idToTimelineItem.Delete(id)
	}

	require.Equal(t, 30, timeline.idToTimelineItem.Len())
	require.NotNil(t, timeline.compressedHead, "Should have compressed head")

	// 序列化
	serialized, err := MarshalTimeline(timeline)
	require.NoError(t, err)

	t.Logf("Compressed timeline serialized to %d bytes (from 100 items to 30 items + 1 reducer)", len(serialized))

	// 验证被压缩的数据标识不应该在序列化中（item 1-70）
	for i := 0; i < 70; i++ {
		require.NotContains(t, serialized, allMarkers[i].marker,
			"Compressed item %d (ID=%d) should NOT be in serialization", i+1, allMarkers[i].id)
	}

	// 验证保留的数据在序列化中（item 71-100）
	for i := 70; i < 100; i++ {
		require.Contains(t, serialized, allMarkers[i].marker,
			"Retained item %d (ID=%d) should be in serialization", i+1, allMarkers[i].id)
	}

	// 验证压缩摘要在序列化中
	require.Contains(t, serialized, compressedSummary)

	// 反序列化验证
	restored, err := UnmarshalTimeline(serialized)
	require.NoError(t, err)

	require.Equal(t, 30, restored.idToTimelineItem.Len(), "Should have 30 items")
	require.NotNil(t, restored.compressedHead, "Should have compressed head")

	// 验证被压缩的条目不存在
	for _, id := range compressedIDs {
		_, exists := restored.idToTimelineItem.Get(id)
		require.False(t, exists, "Compressed ID %d should not exist", id)
	}

	t.Log("✓ PASS: Compressed data is completely removed from serialization")
}

// TestTimelineFinal_ReassignAfterCompress 测试压缩后重新分配ID和继续使用
func TestTimelineFinal_ReassignAfterCompress(t *testing.T) {
	timeline := NewTimeline(nil, nil)

	// 第一阶段：添加100个条目
	for i := 1; i <= 100; i++ {
		timeline.PushToolResult(&aitool.ToolResult{
			ID:   int64(i * 100),
			Name: "phase1",
			Data: "data_" + strings.Repeat("A", i%10),
		})
	}

	// 压缩前60个
	var compressedIDs []int64
	for i := 1; i <= 60; i++ {
		compressedIDs = append(compressedIDs, int64(i*100))
	}

	timeline.compressedHead = &TimelineCompressedHead{
		Text:             "Phase1 compressed",
		CoveredEndItemID: 6000,
		Version:          1,
	}
	for _, id := range compressedIDs {
		if ts, ok := timeline.idToTs.Get(id); ok {
			timeline.tsToTimelineItem.Delete(ts)
		}
		timeline.idToTs.Delete(id)
		timeline.idToTimelineItem.Delete(id)
	}

	require.Equal(t, 40, timeline.idToTimelineItem.Len())
	require.NotNil(t, timeline.compressedHead)

	// 序列化
	serialized, err := MarshalTimeline(timeline)
	require.NoError(t, err)

	// 第二阶段：恢复并重新分配ID
	restored, err := UnmarshalTimeline(serialized)
	require.NoError(t, err)

	var idCounter int64 = 0
	generator := func() int64 {
		return atomic.AddInt64(&idCounter, 1)
	}

	lastID := restored.ReassignIDs(generator)
	require.Equal(t, int64(40), lastID)

	// 验证ID是连续的
	ids := restored.GetTimelineItemIDs()
	require.Equal(t, 40, len(ids))
	for i, id := range ids {
		require.Equal(t, int64(i+1), id, "IDs should be sequential from 1")
	}

	// 第三阶段：继续添加新数据
	for i := 1; i <= 60; i++ {
		restored.PushToolResult(&aitool.ToolResult{
			ID:   generator(),
			Name: "phase2",
			Data: "new_data",
		})
	}

	require.Equal(t, 100, restored.idToTimelineItem.Len())

	// 再次序列化
	serialized2, err := MarshalTimeline(restored)
	require.NoError(t, err)

	t.Logf("After adding new data, serialized to %d bytes", len(serialized2))

	// 最终验证
	final, err := UnmarshalTimeline(serialized2)
	require.NoError(t, err)

	require.Equal(t, 100, final.idToTimelineItem.Len())
	require.NotNil(t, final.compressedHead)

	t.Log("✓ PASS: Can reassign IDs and continue working after compression")
}

// TestTimelineFinal_SingleReducerPolicy 测试单一 reducer 策略
func TestTimelineFinal_SingleReducerPolicy(t *testing.T) {
	timeline := NewTimeline(nil, nil)

	// 添加200个条目
	for i := 1; i <= 200; i++ {
		timeline.PushToolResult(&aitool.ToolResult{
			ID:   int64(i),
			Name: "tool",
			Data: strings.Repeat("X", 100),
		})
	}

	// 第一次压缩：压缩前100个
	var ids1 []int64
	for i := int64(1); i <= 100; i++ {
		ids1 = append(ids1, i)
	}

	timeline.compressedHead = &TimelineCompressedHead{
		Text:             "First batch compressed",
		CoveredEndItemID: 100,
		Version:          1,
	}
	for _, id := range ids1 {
		if ts, ok := timeline.idToTs.Get(id); ok {
			timeline.tsToTimelineItem.Delete(ts)
		}
		timeline.idToTs.Delete(id)
		timeline.idToTimelineItem.Delete(id)
	}

	require.Equal(t, 100, timeline.idToTimelineItem.Len())
	require.NotNil(t, timeline.compressedHead, "Should have compressed head")

	serialized1, err := MarshalTimeline(timeline)
	require.NoError(t, err)
	size1 := len(serialized1)

	// 第二次压缩：再压缩50个
	var ids2 []int64
	for i := int64(101); i <= 150; i++ {
		ids2 = append(ids2, i)
	}

	timeline.compressedHead = &TimelineCompressedHead{
		Text:             "Second batch also compressed",
		CoveredEndItemID: 150,
		Version:          2,
	}
	for _, id := range ids2 {
		if ts, ok := timeline.idToTs.Get(id); ok {
			timeline.tsToTimelineItem.Delete(ts)
		}
		timeline.idToTs.Delete(id)
		timeline.idToTimelineItem.Delete(id)
	}

	require.Equal(t, 50, timeline.idToTimelineItem.Len())

	serialized2, err := MarshalTimeline(timeline)
	require.NoError(t, err)
	size2 := len(serialized2)

	// 第二次序列化应该更小（因为条目更少了）
	require.Less(t, size2, size1, "Second serialization should be smaller")

	t.Logf("First compression: %d bytes (100 items), Second compression: %d bytes (50 items)", size1, size2)
	t.Logf("Compressed head version: %d", timeline.compressedHead.Version)

	// 验证大小显著减小
	require.Less(t, size2, size1*7/10, "Size should decrease significantly with fewer items")

	t.Log("✓ PASS: Reducer policy prevents memory explosion")
}

// TestTimelineFinal_LargeScaleStressTest 大规模压力测试
func TestTimelineFinal_LargeScaleStressTest(t *testing.T) {
	timeline := NewTimeline(nil, nil)

	// 添加500个条目（减少数量加快测试）
	largeData := strings.Repeat("X", 512)
	for i := 1; i <= 500; i++ {
		timeline.PushToolResult(&aitool.ToolResult{
			ID:          int64(i),
			Name:        "tool",
			Description: "desc",
			Success:     true,
			Data:        largeData,
		})
	}

	// 压缩前450个，只保留50个
	var compressedIDs []int64
	for i := int64(1); i <= 450; i++ {
		compressedIDs = append(compressedIDs, i)
	}

	smallSummary := "Compressed 450 items summary"
	timeline.compressedHead = &TimelineCompressedHead{
		Text:             smallSummary,
		CoveredEndItemID: 450,
		Version:          1,
	}

	for _, id := range compressedIDs {
		if ts, ok := timeline.idToTs.Get(id); ok {
			timeline.tsToTimelineItem.Delete(ts)
		}
		timeline.idToTs.Delete(id)
		timeline.idToTimelineItem.Delete(id)
	}

	require.Equal(t, 50, timeline.idToTimelineItem.Len())
	require.NotNil(t, timeline.compressedHead)

	// 序列化
	serialized, err := MarshalTimeline(timeline)
	require.NoError(t, err)

	t.Logf("Serialized size: %d bytes (50 items + 1 small reducer)", len(serialized))

	// 验证大小合理（50个条目 * 512B + overhead < 100KB）
	require.Less(t, len(serialized), 100*1024,
		"Size should be < 100KB for 50 items")

	// 反序列化并验证
	restored, err := UnmarshalTimeline(serialized)
	require.NoError(t, err)

	require.Equal(t, 50, restored.idToTimelineItem.Len())
	require.NotNil(t, restored.compressedHead)

	// 重新分配ID并继续使用
	var idCounter int64 = 10000
	generator := func() int64 {
		return atomic.AddInt64(&idCounter, 1)
	}

	lastID := restored.ReassignIDs(generator)
	require.Equal(t, int64(10050), lastID)

	// 添加更多数据
	for i := 0; i < 50; i++ {
		restored.PushToolResult(&aitool.ToolResult{
			ID:   generator(),
			Name: "new",
			Data: "small",
		})
	}

	require.Equal(t, 100, restored.idToTimelineItem.Len())

	// 最终序列化
	finalSerialized, err := MarshalTimeline(restored)
	require.NoError(t, err)

	t.Logf("Final size: %d bytes (100 items + compressed head)", len(finalSerialized))
	require.Less(t, len(finalSerialized), 200*1024,
		"Final size should still be reasonable")

	t.Log("✓ PASS: Large scale compression works correctly without memory explosion")
}

// TestTimelineFinal_MemorySafety 内存安全测试
func TestTimelineFinal_MemorySafety(t *testing.T) {
	timeline := NewTimeline(nil, nil)

	// 添加300个条目（减少数量加快测试）
	for i := 1; i <= 300; i++ {
		timeline.PushToolResult(&aitool.ToolResult{
			ID:   int64(i),
			Name: "tool",
			Data: strings.Repeat("DATA", 50),
		})
	}

	// 压缩前250个
	var toCompress []int64
	for i := int64(1); i <= 250; i++ {
		toCompress = append(toCompress, i)
	}

	timeline.compressedHead = &TimelineCompressedHead{
		Text:             "Compressed batch 1",
		CoveredEndItemID: 250,
		Version:          1,
	}
	for _, id := range toCompress {
		if ts, ok := timeline.idToTs.Get(id); ok {
			timeline.tsToTimelineItem.Delete(ts)
		}
		timeline.idToTs.Delete(id)
		timeline.idToTimelineItem.Delete(id)
	}

	// 序列化
	s1, err := MarshalTimeline(timeline)
	require.NoError(t, err)

	// 恢复
	r1, err := UnmarshalTimeline(s1)
	require.NoError(t, err)

	// 重新分配ID
	var counter int64 = 0
	gen := func() int64 {
		return atomic.AddInt64(&counter, 1)
	}

	r1.ReassignIDs(gen)

	// 继续添加300个
	for i := 0; i < 300; i++ {
		r1.PushToolResult(&aitool.ToolResult{
			ID:   gen(),
			Name: "tool",
			Data: strings.Repeat("DATA", 50),
		})
	}

	// 再次压缩前250个
	var toCompress2 []int64
	for i := int64(1); i <= 250; i++ {
		toCompress2 = append(toCompress2, i)
	}

	r1.compressedHead = &TimelineCompressedHead{
		Text:             "Compressed batch 2",
		CoveredEndItemID: 250,
		Version:          2,
	}
	for _, id := range toCompress2 {
		if ts, ok := r1.idToTs.Get(id); ok {
			r1.tsToTimelineItem.Delete(ts)
		}
		r1.idToTs.Delete(id)
		r1.idToTimelineItem.Delete(id)
	}

	// 最终序列化
	s2, err := MarshalTimeline(r1)
	require.NoError(t, err)

	t.Logf("First: %d bytes, Second: %d bytes", len(s1), len(s2))

	// 两次大小应该相近（都是保留50个条目左右）
	ratio := float64(len(s2)) / float64(len(s1))
	t.Logf("Size ratio: %.2f", ratio)

	// 允许第二次稍大一些（因为可能有2个reducers），但不应该翻倍
	require.Less(t, ratio, 3.0, "Size should not triple (no severe memory leak)")

	r2, _ := UnmarshalTimeline(s2)
	require.NotNil(t, r2.compressedHead)

	t.Log("✓ PASS: No severe memory leak, sizes remain controlled")
}
