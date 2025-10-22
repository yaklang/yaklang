package aicommon

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

func TestMemoryTimelineOrdinary(t *testing.T) {
	memoryTimeline := NewTimeline(nil, nil)
	for i := 1; i <= 5; i++ {
		memoryTimeline.PushToolResult(&aitool.ToolResult{
			ID:          int64(100 + i),
			Name:        "test",
			Description: "test",
			Param:       map[string]any{"test": "test"},
			Success:     true,
			Data:        "test",
			Error:       "test",
		})
	}
	result := memoryTimeline.Dump()
	t.Log(result)
	require.True(t, strings.Contains(result, "test"))
	require.True(t, strings.Contains(result, "--["))
}

type mockedAI struct {
}

func (m *mockedAI) CallAI(req *AIRequest) (*AIResponse, error) {
	rsp := NewUnboundAIResponse()
	defer rsp.Close()

	// Check if this is a batch compression request
	prompt := req.GetPrompt()
	if strings.Contains(prompt, "批量精炼与浓缩") || strings.Contains(prompt, "batch compress") {
		rsp.EmitOutputStream(strings.NewReader(`
{"@action": "timeline-reducer", "reducer_memory": "batch compressed summary via ai"}
`))
	} else if strings.Contains(prompt, "timeline-reducer") || strings.Contains(prompt, "timeline reducer") {
		rsp.EmitOutputStream(strings.NewReader(`
{"@action": "timeline-reducer", "reducer_memory": "reducer memory via ai"}
`))
	} else {
		rsp.EmitOutputStream(strings.NewReader(`
{"@action": "timeline-shrink", "persistent": "summary via ai"}
`))
	}
	return rsp, nil
}

type mockedToolResult struct {
	ID int64
}

func (m *mockedToolResult) String() string {
	return "mocked result"
}

func (m *mockedToolResult) GetID() int64 {
	return m.ID
}

func (m *mockedToolResult) GetShrinkResult() string {
	return "mocked shrink"
}

func (m *mockedToolResult) GetShrinkSimilarResult() string {
	return "mocked similar"
}

func (m *mockedToolResult) SetShrinkResult(s string) {
	// do nothing
}

// TestTimelineItemMethods 测试TimelineItem相关方法
func TestTimelineItemMethods(t *testing.T) {
	// Test TimelineItem methods
	item := &TimelineItem{
		value: &aitool.ToolResult{
			ID:          100,
			Name:        "test",
			Description: "test",
			Param:       map[string]any{"test": "test"},
			Success:     true,
			Data:        "test",
			Error:       "test",
		},
	}

	// Test GetValue
	val := item.GetValue()
	require.NotNil(t, val)
	require.Equal(t, int64(100), val.GetID())

	// Test IsDeleted
	require.False(t, item.IsDeleted())

	// Test GetShrinkResult and SetShrinkResult
	item.SetShrinkResult("shrink result")
	require.Equal(t, "shrink result", item.GetShrinkResult())

	// Test GetShrinkSimilarResult (for ToolResult, this should return ShrinkResult)
	require.Equal(t, "shrink result", item.GetShrinkSimilarResult())

	// Test ToTimelineItemOutput
	output := item.ToTimelineItemOutput()
	require.NotNil(t, output)
	require.Equal(t, "tool_result", output.Type)
	require.Contains(t, output.Content, "test")
}

// TestUserInteractionMethods 测试UserInteraction相关方法
func TestUserInteractionMethods(t *testing.T) {
	userInteraction := &UserInteraction{
		ID:              200,
		SystemPrompt:    "test system prompt",
		UserExtraPrompt: "test user prompt",
		Stage:           UserInteractionStage_Review,
	}

	// Test String
	str := userInteraction.String()
	require.Contains(t, str, "test system prompt")
	require.Contains(t, str, "test user prompt")
	require.Contains(t, str, "review")

	// Test GetID
	require.Equal(t, int64(200), userInteraction.GetID())

	// Test GetShrinkResult and SetShrinkResult
	userInteraction.SetShrinkResult("shrink result")
	require.Equal(t, "shrink result", userInteraction.GetShrinkResult())

	// Test GetShrinkSimilarResult
	require.Equal(t, "shrink result", userInteraction.GetShrinkSimilarResult())
}

// TestTextTimelineItemMethods 测试TextTimelineItem相关方法
func TestTextTimelineItemMethods(t *testing.T) {
	textItem := &TextTimelineItem{
		ID:   300,
		Text: "test text content",
	}

	// Test String
	str := textItem.String()
	require.Equal(t, "test text content", str)

	// Test GetID
	require.Equal(t, int64(300), textItem.GetID())

	// Test GetShrinkResult and SetShrinkResult
	textItem.SetShrinkResult("shrink result")
	require.Equal(t, "shrink result", textItem.GetShrinkResult())

	// Test GetShrinkSimilarResult
	textItem.ShrinkSimilarResult = "similar result"
	require.Equal(t, "similar result", textItem.GetShrinkSimilarResult())
}

// TestTimelineCompressionMethods 测试Timeline压缩相关方法
func TestTimelineCompressionMethods(t *testing.T) {
	memoryTimeline := NewTimeline(&mockedAI{}, nil)

	// Add an item
	memoryTimeline.PushToolResult(&aitool.ToolResult{
		ID:          100,
		Name:        "test",
		Description: "test",
		Param:       map[string]any{"test": "test"},
		Success:     true,
		Data:        "test",
		Error:       "test",
	})

	// Test CopyReducibleTimelineWithMemory
	copied := memoryTimeline.CopyReducibleTimelineWithMemory()
	require.NotNil(t, copied)
	require.IsType(t, &Timeline{}, copied)

	// Test renderSummaryPrompt (this is called internally by shrink)
	// We can test this indirectly by checking if shrink works
	items := []*TimelineItem{
		{
			value: &aitool.ToolResult{
				ID:          100,
				Name:        "test",
				Description: "test",
				Param:       map[string]any{"test": "test"},
				Success:     true,
				Data:        "test",
				Error:       "test",
			},
		},
	}

	// Test shrink method indirectly by setting perDumpContentLimit
	memoryTimeline.perDumpContentLimit = 10 // Very small limit to trigger shrink
	memoryTimeline.shrink(items[0])

	// The shrink result should be set
	require.NotEmpty(t, items[0].GetShrinkResult())
}

// TestTimelineConfigurationMethods 测试Timeline配置相关方法
func TestTimelineConfigurationMethods(t *testing.T) {
	memoryTimeline := NewTimeline(&mockedAI{}, nil)

	// Test SetTimelineContentLimit
	memoryTimeline.SetTimelineContentLimit(1000)
	// This is internal, we can't directly test it, but we can test that timeline works

	// Test ExtraMetaInfo
	metaFunc := func() string { return "test meta info" }
	timelineWithMeta := NewTimeline(&mockedAI{}, metaFunc)
	metaInfo := timelineWithMeta.ExtraMetaInfo()
	require.Equal(t, "test meta info", metaInfo)

	// Test with nil extraMetaInfo
	timelineNilMeta := NewTimeline(&mockedAI{}, nil)
	metaInfoNil := timelineNilMeta.ExtraMetaInfo()
	require.Equal(t, "", metaInfoNil)
}

// TestTimelineShrinkMethod 测试Timeline的shrink方法
func TestTimelineShrinkMethod(t *testing.T) {
	memoryTimeline := NewTimeline(&mockedAI{}, nil)

	// Add an item that will be shrunk
	memoryTimeline.PushToolResult(&aitool.ToolResult{
		ID:          100,
		Name:        "test",
		Description: "test",
		Param:       map[string]any{"test": "test"},
		Success:     true,
		Data:        "test",
		Error:       "test",
	})

	// Set perDumpContentLimit to trigger shrink
	memoryTimeline.perDumpContentLimit = 10 // Very small limit

	// Call shrink (this is normally called internally)
	items := []*TimelineItem{
		{
			value: &aitool.ToolResult{
				ID:          100,
				Name:        "test",
				Description: "test",
				Param:       map[string]any{"test": "test"},
				Success:     true,
				Data:        "test",
				Error:       "test",
			},
		},
	}

	memoryTimeline.shrink(items[0])
	require.NotEmpty(t, items[0].GetShrinkResult())
}

// TestTimelineRenderSummaryPrompt 测试renderSummaryPrompt方法
func TestTimelineRenderSummaryPrompt(t *testing.T) {
	memoryTimeline := NewTimeline(&mockedAI{}, nil)

	// Add items
	for i := 1; i <= 3; i++ {
		memoryTimeline.PushToolResult(&aitool.ToolResult{
			ID:          int64(i + 100),
			Name:        "test",
			Description: "test",
			Param:       map[string]any{"test": "test"},
			Success:     true,
			Data:        "test",
			Error:       "test",
		})
	}

	// Test renderSummaryPrompt indirectly by checking if summary works
	// This method is called internally by compression, but we can test the summary field
	require.NotNil(t, memoryTimeline.summary)
}

// TestTimelineItemOutputString 测试TimelineItemOutput的String方法
func TestTimelineItemOutputString(t *testing.T) {
	output := &TimelineItemOutput{
		Timestamp: time.Now(),
		Type:      "tool_result",
		Content:   "test content",
	}

	str := output.String()
	require.Contains(t, str, "tool_result")
	require.Contains(t, str, "test content")
	require.Contains(t, str, "[")
	require.Contains(t, str, "]")
}

// TestTimelineItemOutputTypeSwitch 测试ToTimelineItemOutput的类型切换
func TestTimelineItemOutputTypeSwitch(t *testing.T) {
	// Test with UserInteraction
	userItem := &TimelineItem{
		value: &UserInteraction{
			ID:              100,
			SystemPrompt:    "system",
			UserExtraPrompt: "user",
			Stage:           UserInteractionStage_Review,
		},
	}

	output := userItem.ToTimelineItemOutput()
	require.Equal(t, "user_interaction", output.Type)
	require.Contains(t, output.Content, "system")
	require.Contains(t, output.Content, "user")

	// Test with TextTimelineItem
	textItem := &TimelineItem{
		value: &TextTimelineItem{
			ID:   200,
			Text: "test text",
		},
	}

	output2 := textItem.ToTimelineItemOutput()
	require.Equal(t, "text", output2.Type)
	require.Equal(t, "test text", output2.Content)

	// Test with unknown type
	unknownItem := &TimelineItem{
		value: &mockedToolResult{ID: 300},
	}

	output3 := unknownItem.ToTimelineItemOutput()
	require.Equal(t, "raw", output3.Type)
}

// TestTimelineEdgeCases 测试Timeline边缘情况
func TestTimelineEdgeCases(t *testing.T) {
	// Test with nil AI
	memoryTimeline := NewTimeline(nil, nil)
	require.NotNil(t, memoryTimeline)

	// Test with empty timeline
	emptyTimeline := NewTimeline(&mockedAI{}, nil)
	result := emptyTimeline.Dump()
	require.Equal(t, "", result)

	// Test GetTimelineOutput with empty timeline
	output := emptyTimeline.GetTimelineOutput()
	require.Nil(t, output)

	// Test PromptForToolCallResultsForLastN with empty timeline
	prompt := emptyTimeline.PromptForToolCallResultsForLastN(5)
	require.Equal(t, "", prompt)

	// Test ToTimelineItemOutputLastN with empty timeline
	lastN := emptyTimeline.ToTimelineItemOutputLastN(5)
	require.Len(t, lastN, 0)
}
