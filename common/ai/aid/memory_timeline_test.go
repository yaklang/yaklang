package aid

import (
	"fmt"
	"strings"
	"testing"

	"github.com/segmentio/ksuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

func TestMemoryTimelineOrdinary(t *testing.T) {
	memoryTimeline := newMemoryTimeline(10, nil)
	for i := 1; i <= 5; i++ {
		memoryTimeline.PushToolResult(&aitool.ToolResult{
			ID:          fmt.Sprintf("test-%d", i),
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
	require.True(t, strings.Contains(result, "├─["))
}

type mockedAI struct {
}

func (m *mockedAI) callAI(req *AIRequest) (*AIResponse, error) {
	rsp := newUnboundAIResponse()
	defer rsp.Close()
	rsp.EmitOutputStream(strings.NewReader("summary via ai"))
	return rsp, nil
}

func TestMemoryTimelineWithSummary(t *testing.T) {
	memoryTimeline := newMemoryTimeline(3, &mockedAI{})
	for i := 1; i <= 10; i++ {
		memoryTimeline.PushToolResult(&aitool.ToolResult{
			ID:          ksuid.New().String(),
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
	require.True(t, strings.Contains(result, "├─["))
	require.True(t, strings.Contains(result, "summary via ai"))
}
