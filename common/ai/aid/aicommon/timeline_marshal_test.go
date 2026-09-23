package aicommon

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

func TestTimelineMarshalUnmarshal(t *testing.T) {
	// 创建原始 Timeline
	originalTimeline := NewTimeline(nil, nil)

	// 添加一些数据
	for i := 1; i <= 3; i++ {
		originalTimeline.PushToolResult(&aitool.ToolResult{
			ID:                   int64(100 + i),
			Name:                 "test_tool",
			Description:          "test description",
			Param:                map[string]any{"param": i},
			Success:              true,
			Data:                 i,
			Error:                "",
			OmitParamsInTimeline: i == 1,
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

func TestTimelineMarshalPreservesPromptOnlyTextWithoutChangingDisplay(t *testing.T) {
	timeline := NewTimeline(nil, nil)
	timeline.PushTextWithPromptProjection(
		301,
		"[model_thinking]:\ndisplay-only reasoning",
		"[model_thinking]:\n<|TIMELINE_MODEL_THINKING_n1|>payload<|TIMELINE_MODEL_THINKING_END_n1|>",
	)

	serialized, err := MarshalTimeline(timeline)
	require.NoError(t, err)
	restored, err := UnmarshalTimeline(serialized)
	require.NoError(t, err)

	item, ok := restored.idToTimelineItem.Get(301)
	require.True(t, ok)
	require.Equal(t, "[model_thinking]:\ndisplay-only reasoning", item.String())
	textItem, ok := item.GetValue().(*TextTimelineItem)
	require.True(t, ok)
	require.Contains(t, textItem.PromptText, "TIMELINE_MODEL_THINKING_n1")
	require.NotContains(t, item.String(), "TIMELINE_MODEL_THINKING_n1")
}

// TestTimelineMarshalWithCompressedHead 测试 single compressed head 的往返一致性
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
	const legacy = `{"id_to_ts":{},"ts_to_timeline_item":{},"id_to_timeline_item":{},"reducers":{"11":"first memory","22":"second memory"},"reducer_ts":{"11":1700000000000,"22":1700000001000},"archive_refs":{},"per_dump_content_limit":100,"total_dump_content_limit":500}`

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
	require.Nil(t, restoredTimeline.compressedHead)

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

func TestTimelineUnmarshalLegacySummaryType(t *testing.T) {
	for _, test := range []struct {
		name    string
		input   string
		wantErr bool
	}{
		{name: "object", input: `{"summary":{"old":null}}`},
		{name: "null", input: `{"summary":null}`},
		{name: "array", input: `{"summary":[]}`, wantErr: true},
		{name: "number", input: `{"summary":1}`, wantErr: true},
		{name: "invalid item", input: `{"summary":{"old":1}}`, wantErr: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			timeline, err := UnmarshalTimeline(test.input)
			if test.wantErr {
				require.Error(t, err)
				require.Nil(t, timeline)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, timeline)
			serialized, err := MarshalTimeline(timeline)
			require.NoError(t, err)
			require.NotContains(t, serialized, `"summary"`)
		})
	}
}
