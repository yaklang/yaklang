package aicommon

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

func TestMemoryTimelineOrdinary(t *testing.T) {
	memoryTimeline := NewTimeline(10, nil, nil)
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

func TestMemoryTimelineWithBatchCompression(t *testing.T) {
	memoryTimeline := NewTimeline(200, &mockedAI{}, nil)
	memoryTimeline.BindConfig(NewMockedAIConfig(context.Background()), &mockedAI{})
	memoryTimeline.SetTimelineLimit(1000) // Set very high limit to avoid any reducer triggering
	// Push exactly 105 items to trigger batch compression (100 items threshold)
	for i := 1; i <= 105; i++ {
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
	require.True(t, strings.Contains(result, "test"))
	require.True(t, strings.Contains(result, "--["))
	// With batch compression, we should have compressed items and remaining items
	require.True(t, strings.Contains(result, "reducer-memory"))
	// Should have roughly half the original items after compression (originally 105, should be around 50-60 after compression)
	totalItems := strings.Count(result, "--[")
	require.True(t, totalItems < 70, "Should have compressed items, total items: %d", totalItems)
	require.True(t, totalItems > 30, "Should have remaining items after compression, total items: %d", totalItems)
}

type mockedAI2 struct {
	hCompressTime *int64
}

func (m *mockedAI2) CallAI(req *AIRequest) (*AIResponse, error) {
	rsp := NewUnboundAIResponse()
	defer rsp.Close()

	prompt := req.GetPrompt()
	if strings.Contains(prompt, "批量精炼与浓缩") || strings.Contains(prompt, "batch compress") {
		rsp.EmitOutputStream(strings.NewReader(`
{"@action": "timeline-reducer", "reducer_memory": "batch compressed content ` + fmt.Sprint(atomic.AddInt64(m.hCompressTime, 1)) + `"}
`))
	} else if utils.MatchAllOfRegexp(prompt, `const"\s*:\s*"timeline-reducer"`) || strings.Contains(prompt, "timeline-reducer") {
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

func TestMemoryTimelineWithReachLimitBatchCompression(t *testing.T) {
	memoryTimeline := NewTimeline(2, &mockedAI2{
		hCompressTime: new(int64),
	}, nil)
	memoryTimeline.BindConfig(NewMockedAIConfig(context.Background()), &mockedAI2{})
	memoryTimeline.SetTimelineLimit(2)
	// Push enough items to trigger batch compression (100 items threshold)
	for i := 1; i <= 105; i++ {
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
	require.True(t, strings.Contains(result, "--["))
	require.True(t, strings.Contains(result, "高度压缩的内容"))
	// Should have at least one batch compression result
	require.True(t, strings.Count(result, `高度压缩的内容`) >= 1)
	// Should have remaining timeline items
	totalItems := strings.Count(result, "--[")
	require.True(t, totalItems > 0, "Should have remaining timeline items")
}

// MockedAIConfig 实现 AICallerConfigIf 接口，用于测试
type MockedAIConfig struct {
	*BaseInteractiveHandler
	*BaseCheckpointableStorage

	ctx context.Context

	idSequence int64
	runtimeId  string

	emitter *Emitter

	timelineRecordLimit      int64
	timelineContentSizeLimit int64
}

func NewMockedAIConfig(ctx context.Context) AICallerConfigIf {
	emitter := &Emitter{
		streamWG:            &sync.WaitGroup{},
		id:                  "mock-emitter",
		baseEmitter:         func(e *schema.AiOutputEvent) error { return nil },
		eventProcesserStack: utils.NewStack[EventProcesser](),
	}

	config := &MockedAIConfig{
		BaseInteractiveHandler:    &BaseInteractiveHandler{},
		BaseCheckpointableStorage: NewBaseCheckpointableStorage(),
		ctx:                       ctx,
		runtimeId:                 "mock-runtime-id",
		emitter:                   emitter,
		timelineRecordLimit:       10,
		timelineContentSizeLimit:  1000,
	}

	config.BaseInteractiveHandler.endpointManager = NewEndpointManager()

	return config
}

func (m *MockedAIConfig) AcquireId() int64 {
	return atomic.AddInt64(&m.idSequence, 1)
}

func (m *MockedAIConfig) GetRuntimeId() string {
	return m.runtimeId
}

func (m *MockedAIConfig) IsCtxDone() bool {
	select {
	case <-m.ctx.Done():
		return true
	default:
		return false
	}
}

func (m *MockedAIConfig) GetContext() context.Context {
	return m.ctx
}

func (m *MockedAIConfig) CallAIResponseConsumptionCallback(current int) {
	// Mock implementation - do nothing
}

func (m *MockedAIConfig) GetAITransactionAutoRetryCount() int64 {
	return 3
}

func (m *MockedAIConfig) RetryPromptBuilder(originalPrompt string, err error) string {
	if err == nil {
		return originalPrompt
	}
	return fmt.Sprintf("Retry prompt for error: %v\nOriginal: %s", err, originalPrompt)
}

func (m *MockedAIConfig) GetEmitter() *Emitter {
	return m.emitter
}

func (m *MockedAIConfig) NewAIResponse() *AIResponse {
	return NewAIResponse(m)
}

func (m *MockedAIConfig) CallAIResponseOutputFinishedCallback(s string) {
	// Mock implementation - do nothing
}

func (m *MockedAIConfig) GetTimelineRecordLimit() int64 {
	return m.timelineRecordLimit
}

func (m *MockedAIConfig) GetTimelineContentSizeLimit() int64 {
	return m.timelineContentSizeLimit
}
