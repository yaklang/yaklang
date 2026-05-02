package aicommon

import (
	"testing"

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

// TestTimelineMarshalWithReducers 测试 Timeline 序列化时 reducers 与 reducerTs 的往返一致性
// 关键词: MarshalTimeline, reducerTs 序列化往返
// 历史说明：原 TestTimelineMarshalWithSummaryAndReducers 同时验证 summary 与 reducers，
// 由于 summary 已被识别为 dead code 并移除，此处仅保留并扩展 reducers 验证 + 新增 reducerTs 校验
func TestTimelineMarshalWithReducers(t *testing.T) {
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

	// 手动添加一些 reducers 数据并配对写入 reducerTs
	// 关键词: reducers + reducerTs 同步写入
	originalTimeline.reducers.Set(int64(200), linktable.NewUnlimitedStringLinkTable("compressed memory"))
	originalTimeline.reducerTs.Set(int64(200), int64(1700000000000))

	jsonStr, err := MarshalTimeline(originalTimeline)
	require.NoError(t, err)
	require.NotEmpty(t, jsonStr)

	restoredTimeline, err := UnmarshalTimeline(jsonStr)
	require.NoError(t, err)
	require.NotNil(t, restoredTimeline)

	// 验证 reducers
	originalTimeline.reducers.ForEach(func(id int64, originalLt *linktable.LinkTable[string]) bool {
		restoredLt, ok := restoredTimeline.reducers.Get(id)
		require.True(t, ok, "Reducer for id %d not found", id)
		require.Equal(t, originalLt.Value(), restoredLt.Value())
		return true
	})

	// 验证 reducerTs（关键词: reducerTs 反序列化）
	require.Equal(t, originalTimeline.reducerTs.Len(), restoredTimeline.reducerTs.Len())
	originalTimeline.reducerTs.ForEach(func(id int64, ts int64) bool {
		got, ok := restoredTimeline.reducerTs.Get(id)
		require.True(t, ok, "ReducerTs for id %d not found", id)
		require.Equal(t, ts, got)
		return true
	})

	t.Log("Timeline marshal with reducers/reducerTs test passed")
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
	require.Equal(t, 0, restoredTimeline.reducers.Len())
	require.Equal(t, 0, restoredTimeline.reducerTs.Len())

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
