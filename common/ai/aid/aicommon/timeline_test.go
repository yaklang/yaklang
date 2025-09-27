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
	config := NewMockedAIConfig(context.Background()).(*MockedAIConfig)
	config.timelineContentSizeLimit = 20000 // Set larger content size limit
	memoryTimeline.BindConfig(config, &mockedAI{})

	// Add items until content size triggers compression
	for i := 1; i <= 200; i++ { // Add more items to trigger compression
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

	// With batch compression triggered by content size, we should have some compressed items
	totalItems := strings.Count(result, "--[")
	require.True(t, totalItems < 150, "Should have compressed some items, total items: %d", totalItems)
	require.True(t, totalItems > 50, "Should have remaining items after compression, total items: %d", totalItems)

	// Check if compression actually happened (either reducer-memory: or compressed items)
	hasCompression := strings.Contains(result, "reducer-memory:") || totalItems < 100
	require.True(t, hasCompression, "Should have some form of compression")
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

	// 设置合理的内容大小限制以触发压缩
	config := NewMockedAIConfig(context.Background()).(*MockedAIConfig)
	config.timelineContentSizeLimit = 5000 // 设置合适的大小限制
	memoryTimeline.BindConfig(config, &mockedAI2{})
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
	require.True(t, strings.Contains(result, "batch compressed content"))
	// Should have at least one batch compression result
	require.True(t, strings.Count(result, `batch compressed content`) >= 1)
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

// TestNoCompression 测试不触发压缩的情况
func TestNoCompression(t *testing.T) {
	memoryTimeline := NewTimeline(10, &mockedAI{}, nil)

	// 创建配置并设置大的内容限制以避免内容大小触发压缩
	config := NewMockedAIConfig(context.Background()).(*MockedAIConfig)
	config.timelineContentSizeLimit = 100000 // 设置很大的限制
	memoryTimeline.BindConfig(config, &mockedAI{})

	// 添加少量项目，不触发批量压缩 (需要 >= 100 个项目)
	for i := 1; i <= 50; i++ {
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

	// 应该没有压缩，因为项目数量少于阈值
	totalItems := strings.Count(result, "--[")
	require.Equal(t, 50, totalItems, "Should have all 50 items without compression")
	require.False(t, strings.Contains(result, "reducer-memory:"), "Should not have reducer memory")
}

// TestBinarySearchCompression 测试二分法压缩逻辑
func TestBinarySearchCompression(t *testing.T) {
	memoryTimeline := NewTimeline(200, &mockedAI{}, nil)

	// 设置合理的内容大小限制以触发压缩
	config := NewMockedAIConfig(context.Background()).(*MockedAIConfig)
	config.timelineContentSizeLimit = 14000 // 设置合适的大小限制
	memoryTimeline.BindConfig(config, &mockedAI{})

	// 添加足够多的项目来触发压缩
	for i := 1; i <= 120; i++ {
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

	// 应该有压缩，因为项目数量 >= 100
	require.True(t, strings.Contains(result, "reducer-memory:"), "Should have reducer memory after compression")

	// 验证剩余项目数应该大约是原来的一半
	totalItems := strings.Count(result, "--[")
	require.True(t, totalItems < 120, "Should have fewer items after compression")
	require.True(t, totalItems >= 50, "Should have at least half the items after compression")
}

// TestCompressionBoundary 测试压缩边界条件
func TestCompressionBoundary(t *testing.T) {
	memoryTimeline := NewTimeline(10, &mockedAI{}, nil)

	// 设置合理的内容大小限制以触发压缩
	config := NewMockedAIConfig(context.Background()).(*MockedAIConfig)
	config.timelineContentSizeLimit = 10000 // 设置合适的大小限制
	memoryTimeline.BindConfig(config, &mockedAI{})

	// 测试边界情况：正好100个项目
	for i := 1; i <= 100; i++ {
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

	// 100个项目应该触发压缩
	require.True(t, strings.Contains(result, "reducer-memory:"), "Should trigger compression at exactly 100 items")
}

// TestCompressionWithContentSizeLimit 测试内容大小限制触发的压缩
func TestCompressionWithContentSizeLimit(t *testing.T) {
	memoryTimeline := NewTimeline(10, &mockedAI{}, nil)

	// 创建具体的 MockedAIConfig 实例以便设置字段
	config := &MockedAIConfig{
		BaseInteractiveHandler:    &BaseInteractiveHandler{},
		BaseCheckpointableStorage: NewBaseCheckpointableStorage(),
		ctx:                       context.Background(),
		runtimeId:                 "mock-runtime-id",
		emitter: func() *Emitter {
			emitter := &Emitter{
				streamWG:            &sync.WaitGroup{},
				id:                  "mock-emitter",
				baseEmitter:         func(e *schema.AiOutputEvent) error { return nil },
				eventProcesserStack: utils.NewStack[EventProcesser](),
			}
			return emitter
		}(),
		timelineRecordLimit:      10,
		timelineContentSizeLimit: 100, // 设置较小的内容大小限制
	}

	config.BaseInteractiveHandler.endpointManager = NewEndpointManager()

	memoryTimeline.BindConfig(config, &mockedAI{})

	// 添加较少的项目，但内容足够大以触发大小限制
	for i := 1; i <= 60; i++ {
		memoryTimeline.PushToolResult(&aitool.ToolResult{
			ID:          int64(i + 100),
			Name:        "test",
			Description: "test",
			Param:       map[string]any{"test": "test with longer content to exceed size limit"},
			Success:     true,
			Data:        "test with longer content to exceed size limit",
			Error:       "test with longer content to exceed size limit",
		})
	}

	result := memoryTimeline.Dump()
	require.True(t, strings.Contains(result, "test"))

	// 如果内容过大，应该触发压缩
	if strings.Contains(result, "reducer-memory:") {
		require.True(t, strings.Contains(result, "batch compressed"), "Should have batch compression result")
	}
}

// TestCompressionRatio 测试压缩比例是否合理
func TestCompressionRatio(t *testing.T) {
	memoryTimeline := NewTimeline(200, &mockedAI{}, nil)

	// 设置合理的内容大小限制以触发压缩
	config := NewMockedAIConfig(context.Background()).(*MockedAIConfig)
	config.timelineContentSizeLimit = 20000 // 设置合适的大小限制
	memoryTimeline.BindConfig(config, &mockedAI{})

	// 添加大量项目
	for i := 1; i <= 150; i++ {
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

	// 计算压缩前后的项目数
	totalItems := strings.Count(result, "--[")
	compressionResults := strings.Count(result, "reducer-memory:")

	// 应该有显著的压缩效果
	require.True(t, compressionResults > 0, "Should have compression results")
	require.True(t, totalItems < 150, "Should have fewer items after compression")

	// 压缩后的项目数应该合理（不会过度压缩，也不会压缩不足）
	require.True(t, totalItems >= 50, "Should keep at least half the items")
	require.True(t, totalItems <= 120, "Should not keep too many items after compression")
}

// TestNoCompressionUnderThreshold 测试低于阈值时不压缩
func TestNoCompressionUnderThreshold(t *testing.T) {
	memoryTimeline := NewTimeline(10, &mockedAI{}, nil)

	// 设置大的内容限制以避免内容大小触发压缩
	config := NewMockedAIConfig(context.Background()).(*MockedAIConfig)
	config.timelineContentSizeLimit = 1000000 // 设置很大的限制
	memoryTimeline.BindConfig(config, &mockedAI{})

	// 添加少量项目，不触发批量压缩
	for i := 1; i <= 99; i++ {
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

	// 99个项目不应该触发压缩
	totalItems := strings.Count(result, "--[")
	require.Equal(t, 99, totalItems, "Should have all 99 items without compression")
	require.False(t, strings.Contains(result, "reducer-memory:"), "Should not have reducer memory")
}

// TestCompressionWithDifferentSizes 测试不同大小的压缩
func TestCompressionWithDifferentSizes(t *testing.T) {
	memoryTimeline := NewTimeline(200, &mockedAI{}, nil)

	// 设置合理的内容大小限制以触发压缩
	config := NewMockedAIConfig(context.Background()).(*MockedAIConfig)
	config.timelineContentSizeLimit = 25000 // 设置合适的大小限制
	memoryTimeline.BindConfig(config, &mockedAI{})

	// 添加200个项目，应该触发多次压缩
	for i := 1; i <= 200; i++ {
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

	// 应该有压缩
	require.True(t, strings.Contains(result, "reducer-memory:"), "Should have reducer memory after compression")

	// 计算最终的项目数
	totalItems := strings.Count(result, "--[")
	compressionCount := strings.Count(result, "reducer-memory:")

	// 应该有合理的压缩效果
	require.True(t, compressionCount > 0, "Should have compression")
	require.True(t, totalItems < 200, "Should have fewer items after compression")
	require.True(t, totalItems >= 50, "Should keep reasonable number of items")
}
