package mock

import (
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"strings"
	"testing"
)

type mockedAI struct{}

func (*mockedAI) CallAI(*aicommon.AIRequest) (*aicommon.AIResponse, error) {
	response := aicommon.NewUnboundAIResponse()
	response.EmitOutputStream(strings.NewReader(`{"@action":"timeline-summary","summary":"Verified history; next step pending."}`))
	response.Close()
	return response, nil
}
func (m *mockedAI) CallSpeedPriorityAI(req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
	return m.CallAI(req)
}
func (m *mockedAI) CallQualityPriorityAI(req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
	return m.CallAI(req)
}

// TestTimelineBasicMethods 测试Timeline基础方法
func TestTimelineBasicMethods(t *testing.T) {
	memoryTimeline := aicommon.NewTimeline(&mockedAI{}, nil)

	// Test SetAICaller and GetAICaller
	newAI := &mockedAI{}
	memoryTimeline.SetAICaller(newAI)
	require.Equal(t, newAI, memoryTimeline.GetAICaller())

	// Test GetIdToTimelineItem and GetTimelineItemIDs
	// Add some items first
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

	// Test GetIdToTimelineItem
	idToItem := memoryTimeline.GetIdToTimelineItem()
	require.Equal(t, 3, idToItem.Len())

	// Test GetTimelineItemIDs
	ids := memoryTimeline.GetTimelineItemIDs()
	require.Len(t, ids, 3)
	require.Contains(t, ids, int64(101))
	require.Contains(t, ids, int64(102))
	require.Contains(t, ids, int64(103))

	// Test ClearRuntimeConfig
	memoryTimeline.ClearRuntimeConfig()
	require.Nil(t, memoryTimeline.GetAICaller())
}

// TestTimelineAdvancedOperations 测试Timeline高级操作
func TestTimelineAdvancedOperations(t *testing.T) {
	memoryTimeline := aicommon.NewTimeline(&mockedAI{}, nil)

	// Test PushUserInteraction
	memoryTimeline.PushUserInteraction(aicommon.UserInteractionStage_FreeInput, 200, "test system prompt", "test user prompt")

	result := memoryTimeline.Dump()
	require.Contains(t, result, "test system prompt")
	require.Contains(t, result, "test user prompt")

	// Test PushText
	memoryTimeline.PushText(300, "test text content")
	result = memoryTimeline.Dump()
	require.Contains(t, result, "test text content")

	// Test SoftDelete
	// Add an item to delete
	memoryTimeline.PushToolResult(&aitool.ToolResult{
		ID:          400,
		Name:        "test",
		Description: "test",
		Param:       map[string]any{"test": "test"},
		Success:     true,
		Data:        "test",
		Error:       "test",
	})

	beforeDelete := memoryTimeline.Dump()
	require.Contains(t, beforeDelete, "id: 400")

	memoryTimeline.SoftDelete(400)
	afterDelete := memoryTimeline.Dump()
	require.NotContains(t, afterDelete, "id: 400")

	// Test CreateSubTimeline
	subTimeline := memoryTimeline.CreateSubTimeline(200)
	require.NotNil(t, subTimeline)
	require.IsType(t, &aicommon.Timeline{}, subTimeline)
}

// TestTimelineUtilityMethods 测试Timeline工具方法
func TestTimelineUtilityMethods(t *testing.T) {
	memoryTimeline := aicommon.NewTimeline(&mockedAI{}, nil)

	// Add some items
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

	// Test GetTimelineOutput
	output := memoryTimeline.GetTimelineOutput()
	require.NotNil(t, output)
	require.True(t, len(output) > 0)

	// Test ToTimelineItemOutputLastN
	lastN := memoryTimeline.ToTimelineItemOutputLastN(2)
	require.Len(t, lastN, 2)

	// Test PromptForToolCallResultsForLastN
	prompt := memoryTimeline.PromptForToolCallResultsForLastN(2)
	require.NotEmpty(t, prompt)
	require.Contains(t, prompt, "test")
}
