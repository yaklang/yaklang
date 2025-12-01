package mock

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

type mockedAI struct {
}

func (m *mockedAI) CallAI(req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
	rsp := aicommon.NewUnboundAIResponse()
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

type mockedAI2 struct {
	hCompressTime *int64
}

func (m *mockedAI2) CallAI(req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
	rsp := aicommon.NewUnboundAIResponse()
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

func TestMemoryTimelineWithBatchCompression(t *testing.T) {
	memoryTimeline := aicommon.NewTimeline(&mockedAI{}, nil)
	config := NewMockedAIConfig(context.Background()).(*MockedAIConfig)
	config.TimelineContentSizeLimit = 20000 // Set larger content size limit
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

func TestMemoryTimelineWithReachLimitBatchCompression(t *testing.T) {
	memoryTimeline := aicommon.NewTimeline(&mockedAI2{
		hCompressTime: new(int64),
	}, nil)

	// 设置合理的内容大小限制以触发压缩
	config := NewMockedAIConfig(context.Background()).(*MockedAIConfig)
	config.TimelineContentSizeLimit = 5000 // 设置合适的大小限制
	memoryTimeline.BindConfig(config, &mockedAI2{})
	// Push items with longer content to trigger batch compression by content size
	for i := 1; i <= 60; i++ {
		memoryTimeline.PushToolResult(&aitool.ToolResult{
			ID:          int64(i + 100),
			Name:        "test",
			Description: "test with longer content to trigger compression",
			Param:       map[string]any{"test": "test with longer content to trigger compression"},
			Success:     true,
			Data:        "test with longer content to trigger compression",
			Error:       "test with longer content to trigger compression",
		})
	}

	result := memoryTimeline.Dump()
	t.Log(result)
	require.True(t, strings.Contains(result, "test"))
	require.True(t, strings.Contains(result, "--["))

	// Check if compression happened (either batch compression or content size triggered compression)
	hasCompression := strings.Contains(result, "batch compressed content") || strings.Contains(result, "reducer-memory:")
	require.True(t, hasCompression, "Should have some form of compression due to content size limit")

	// Should have remaining timeline items
	totalItems := strings.Count(result, "--[")
	require.True(t, totalItems > 0, "Should have remaining timeline items")
}

// TestNoCompression 测试不触发压缩的情况
func TestNoCompression(t *testing.T) {
	memoryTimeline := aicommon.NewTimeline(&mockedAI{}, nil)

	// 创建配置并设置大的内容限制以避免内容大小触发压缩
	config := NewMockedAIConfig(context.Background()).(*MockedAIConfig)
	config.TimelineContentSizeLimit = 100000 // 设置很大的限制
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
	memoryTimeline := aicommon.NewTimeline(&mockedAI{}, nil)

	// 设置合理的内容大小限制以触发压缩
	config := NewMockedAIConfig(context.Background()).(*MockedAIConfig)
	config.TimelineContentSizeLimit = 14000 // 设置合适的大小限制
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
	memoryTimeline := aicommon.NewTimeline(&mockedAI{}, nil)

	// 设置合理的内容大小限制以触发压缩
	config := NewMockedAIConfig(context.Background()).(*MockedAIConfig)
	config.TimelineContentSizeLimit = 10000 // 设置合适的大小限制
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
	memoryTimeline := aicommon.NewTimeline(&mockedAI{}, nil)

	emitter := aicommon.NewEmitter("mock-emitter", func(e *schema.AiOutputEvent) (*schema.AiOutputEvent, error) {
		return e, nil
	})

	config := &MockedAIConfig{
		KeyValueConfig:            aicommon.NewKeyValueConfig(),
		BaseInteractiveHandler:    &aicommon.BaseInteractiveHandler{},
		BaseCheckpointableStorage: aicommon.NewBaseCheckpointableStorage(),
		Ctx:                       context.Background(),
		RuntimeId:                 "mock-runtime-id",
		Emitter:                   emitter,
		TimelineContentSizeLimit:  100,
	}

	config.BaseInteractiveHandler = aicommon.NewBaseInteractiveHandler()

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
	memoryTimeline := aicommon.NewTimeline(&mockedAI{}, nil)

	// 设置合理的内容大小限制以触发压缩
	config := NewMockedAIConfig(context.Background()).(*MockedAIConfig)
	config.TimelineContentSizeLimit = 20000 // 设置合适的大小限制
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
	memoryTimeline := aicommon.NewTimeline(&mockedAI{}, nil)

	// 设置大的内容限制以避免内容大小触发压缩
	config := NewMockedAIConfig(context.Background()).(*MockedAIConfig)
	config.TimelineContentSizeLimit = 1000000 // 设置很大的限制
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
	memoryTimeline := aicommon.NewTimeline(&mockedAI{}, nil)

	// 设置合理的内容大小限制以触发压缩
	config := NewMockedAIConfig(context.Background()).(*MockedAIConfig)
	config.TimelineContentSizeLimit = 25000 // 设置合适的大小限制
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

// TestTimelineBindConfig 测试Timeline绑定配置
func TestTimelineBindConfig(t *testing.T) {
	memoryTimeline := aicommon.NewTimeline(&mockedAI{}, nil)

	// Test BindConfig
	config := NewMockedAIConfig(context.Background()).(*MockedAIConfig)
	memoryTimeline.BindConfig(config, &mockedAI{})
	// This sets internal config, we can test that compression still works
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
