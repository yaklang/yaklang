package aid

import (
	"context"
	"fmt"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/utils"
	"strings"
	"sync/atomic"
	"testing"
)

func TestMemoryTimelineOrdinary(t *testing.T) {
	memoryTimeline := newMemoryTimeline(10, nil)
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
	require.True(t, strings.Contains(result, "├─["))
}

type mockedAI struct {
}

func (m *mockedAI) callAI(req *AIRequest) (*AIResponse, error) {
	rsp := newUnboundAIResponse()
	defer rsp.Close()
	rsp.EmitOutputStream(strings.NewReader(`
{"@action": "timeline-shrink", "persistent": "summary via ai"}
`))
	return rsp, nil
}

func TestMemoryTimelineWithSummary(t *testing.T) {
	memoryTimeline := newMemoryTimeline(3, &mockedAI{})
	memoryTimeline.BindConfig(newConfig(context.Background()))
	memoryTimeline.setTimelineLimit(3)
	for i := 1; i <= 10; i++ {
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

	result := memoryTimeline.Dump()
	t.Log(result)
	require.True(t, strings.Contains(result, "test"))
	require.True(t, strings.Contains(result, "├─["))
	require.True(t, strings.Contains(result, "summary via ai"))
	require.Equal(t, strings.Count(result, `summary via ai`), 7)
}

type mockedAI2 struct {
	hCompressTime *int64
}

func (m *mockedAI2) callAI(req *AIRequest) (*AIResponse, error) {
	rsp := newUnboundAIResponse()
	defer rsp.Close()

	if utils.MatchAllOfRegexp(req.GetPrompt(), `const"\s*:\s*"timeline-reducer"`) {
		rsp.EmitOutputStream(strings.NewReader(`
{"@action": "timeline-reducer", "reducer_memory": "高度压缩的内容` + fmt.Sprint(atomic.AddInt64(m.hCompressTime, 1)) + `"}
`))
	} else {
		rsp.EmitOutputStream(strings.NewReader(`
{"@action": "timeline-shrink", "persistent": "summary via ai"}
`))
	}

	return rsp, nil
}

func TestMemoryTimelineWithReachLimitSummary(t *testing.T) {
	memoryTimeline := newMemoryTimeline(2, &mockedAI2{
		hCompressTime: new(int64),
	})
	memoryTimeline.BindConfig(newConfig(context.Background()))
	memoryTimeline.setTimelineLimit(2)
	for i := 1; i <= 20; i++ {
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

	result := memoryTimeline.Dump()
	t.Log(result)
	require.True(t, strings.Contains(result, "test"))
	require.True(t, strings.Contains(result, "├─["))
	require.True(t, strings.Contains(result, "summary via ai"))
	require.Equal(t, strings.Count(result, `summary via ai`), 4)
	require.True(t, strings.Contains(result, "高度压缩的内容"))
	require.Equal(t, strings.Count(result, `高度压缩的内容`), 1)
	require.True(t, strings.Contains(result, "高度压缩的内容14"))
}
