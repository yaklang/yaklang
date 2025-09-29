package aicommon

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/utils/linktable"
)

func TestTimelineMarshalUnmarshal(t *testing.T) {
	// 创建原始 Timeline
	originalTimeline := NewTimeline(nil, nil)

	// 添加一些数据
	for i := 1; i <= 3; i++ {
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

	// 添加用户交互
	originalTimeline.PushUserInteraction(UserInteractionStage_Review, 200, "system prompt", "user input")

	// 添加文本项目
	originalTimeline.PushText(300, "test text content")

	// 设置限制
	originalTimeline.perDumpContentLimit = 1000
	originalTimeline.totalDumpContentLimit = 5000

	// 序列化
	jsonStr, err := MarshalTimeline(originalTimeline)
	require.NoError(t, err)
	require.NotEmpty(t, jsonStr)

	t.Logf("Serialized JSON length: %d", len(jsonStr))

	// 反序列化
	restoredTimeline, err := UnmarshalTimeline(jsonStr)
	require.NoError(t, err)
	require.NotNil(t, restoredTimeline)

	// 验证数据完整性
	require.Equal(t, originalTimeline.idToTimelineItem.Len(), restoredTimeline.idToTimelineItem.Len())
	require.Equal(t, originalTimeline.perDumpContentLimit, restoredTimeline.perDumpContentLimit)
	require.Equal(t, originalTimeline.totalDumpContentLimit, restoredTimeline.totalDumpContentLimit)

	// 验证每个项目
	originalTimeline.idToTimelineItem.ForEach(func(id int64, originalItem *TimelineItem) bool {
		restoredItem, ok := restoredTimeline.idToTimelineItem.Get(id)
		require.True(t, ok, "Item %d not found in restored timeline", id)

		require.Equal(t, originalItem.deleted, restoredItem.deleted)
		// 时间戳可能在序列化过程中有精度损失，这里只比较内容是否相同
		require.Equal(t, originalItem.String(), restoredItem.String())

		return true
	})

	// 验证映射关系
	originalTimeline.idToTs.ForEach(func(id int64, originalTs int64) bool {
		restoredTs, ok := restoredTimeline.idToTs.Get(id)
		require.True(t, ok, "Timestamp mapping for id %d not found", id)
		require.Equal(t, originalTs, restoredTs)
		return true
	})

	// 验证 tsToTimelineItem
	originalTimeline.tsToTimelineItem.ForEach(func(ts int64, originalItem *TimelineItem) bool {
		restoredItem, ok := restoredTimeline.tsToTimelineItem.Get(ts)
		require.True(t, ok, "Timeline item for timestamp %d not found", ts)
		require.Equal(t, originalItem.String(), restoredItem.String())
		return true
	})

	t.Log("Timeline marshal/unmarshal test passed")
}

func TestTimelineMarshalWithSummaryAndReducers(t *testing.T) {
	// 创建 Timeline 并添加一些数据来生成 summary 和 reducers
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

	// 手动添加一些 summary 数据（模拟压缩后的数据）
	testItem := &TimelineItem{
		createdAt: time.Now(),
		value: &TextTimelineItem{
			ID:   150,
			Text: "compressed content",
		},
	}
	originalTimeline.summary.Set(int64(150), linktable.NewUnlimitedLinkTable(testItem))

	// 手动添加一些 reducers 数据
	originalTimeline.reducers.Set(int64(200), linktable.NewUnlimitedStringLinkTable("compressed memory"))

	// 序列化
	jsonStr, err := MarshalTimeline(originalTimeline)
	require.NoError(t, err)
	require.NotEmpty(t, jsonStr)

	// 反序列化
	restoredTimeline, err := UnmarshalTimeline(jsonStr)
	require.NoError(t, err)
	require.NotNil(t, restoredTimeline)

	// 验证 summary - 只应该有最后一个值
	originalTimeline.summary.ForEach(func(id int64, originalLt *linktable.LinkTable[*TimelineItem]) bool {
		restoredLt, ok := restoredTimeline.summary.Get(id)
		require.True(t, ok, "Summary for id %d not found", id)

		// 验证只保留了最后一个值
		require.Equal(t, originalLt.Value().String(), restoredLt.Value().String())
		return true
	})

	// 验证 reducers - 只应该有最后一个值
	originalTimeline.reducers.ForEach(func(id int64, originalLt *linktable.LinkTable[string]) bool {
		restoredLt, ok := restoredTimeline.reducers.Get(id)
		require.True(t, ok, "Reducer for id %d not found", id)

		// 验证只保留了最后一个值
		require.Equal(t, originalLt.Value(), restoredLt.Value())
		return true
	})

	t.Log("Timeline marshal with summary and reducers test passed")
}

func TestTimelineMarshalEmpty(t *testing.T) {
	// 测试空 Timeline
	originalTimeline := NewTimeline(nil, nil)

	// 序列化
	jsonStr, err := MarshalTimeline(originalTimeline)
	require.NoError(t, err)

	// 反序列化
	restoredTimeline, err := UnmarshalTimeline(jsonStr)
	require.NoError(t, err)
	require.NotNil(t, restoredTimeline)

	// 验证为空
	require.Equal(t, 0, restoredTimeline.idToTimelineItem.Len())
	require.Equal(t, 0, restoredTimeline.summary.Len())
	require.Equal(t, 0, restoredTimeline.reducers.Len())

	t.Log("Empty timeline marshal/unmarshal test passed")
}

func TestTimelineUnmarshalEmptyString(t *testing.T) {
	// 测试空字符串
	restoredTimeline, err := UnmarshalTimeline("")
	require.NoError(t, err)
	require.NotNil(t, restoredTimeline)

	// 应该返回一个空的 Timeline
	require.Equal(t, 0, restoredTimeline.idToTimelineItem.Len())

	t.Log("Empty string unmarshal test passed")
}
