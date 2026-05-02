package aicommon

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/utils/linktable"
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
	// Dump 现在走 GroupByMinutes(3).GetAllRenderable().Render("TIMELINE")，输出 aitag 包裹格式
	// 关键词: TestMemoryTimelineOrdinary, aitag 包裹断言
	require.True(t, strings.Contains(result, "<|TIMELINE_"))
	require.True(t, strings.Contains(result, "<|TIMELINE_END_"))
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

func (m *mockedAI) CallSpeedPriorityAI(req *AIRequest) (*AIResponse, error) {
	return m.CallAI(req)
}

func (m *mockedAI) CallQualityPriorityAI(req *AIRequest) (*AIResponse, error) {
	return m.CallAI(req)
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

// TestTimelineReducerTsInitialized 测试 NewTimeline 后 reducerTs 已初始化
// 关键词: reducerTs 初始化校验
// 历史说明：原 TestTimelineRenderSummaryPrompt 仅断言 memoryTimeline.summary 非 nil，
// summary 字段已被识别为 dead code 并移除；这里改为校验新增的 reducerTs 字段
func TestTimelineReducerTsInitialized(t *testing.T) {
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

	require.NotNil(t, memoryTimeline.reducerTs)
	require.Equal(t, 0, memoryTimeline.reducerTs.Len())
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

// TestDump_ByteStability 验证 Dump 在不变 timeline 上字节级稳定
// 这是 prompt cache 命中的核心保证：同一个未变化的 timeline 反复调用 Dump 必须产出相同字节流
// 关键词: TestDump_ByteStability, prompt cache 命中保护, Dump 字节稳定
func TestDump_ByteStability(t *testing.T) {
	tl := NewTimeline(nil, nil)
	baseTs := time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC)

	// 跨多个 3 分钟桶注入活跃条目
	injectTimelineItem(tl, int64(101), baseTs.Add(30*time.Second), makeToolResult(101, "scan", true, "scan-result-A"))
	injectTimelineItem(tl, int64(102), baseTs.Add(2*time.Minute+10*time.Second), makeToolResult(102, "ls", true, "ls-output"))
	injectTimelineItem(tl, int64(103), baseTs.Add(5*time.Minute), makeToolResult(103, "cat", true, "cat-output"))
	injectTimelineItem(tl, int64(104), baseTs.Add(8*time.Minute+45*time.Second), makeToolResult(104, "echo", true, "echo-output"))

	// 注入 reducer 模拟之前已批量压缩
	tl.reducers.Set(int64(50), linktable.NewUnlimitedStringLinkTable("compressed batch memory alpha"))
	tl.reducerTs.Set(int64(50), baseTs.Add(-5*time.Minute).UnixMilli())
	tl.reducers.Set(int64(60), linktable.NewUnlimitedStringLinkTable("compressed batch memory beta"))
	tl.reducerTs.Set(int64(60), baseTs.Add(-2*time.Minute).UnixMilli())

	dump1 := tl.Dump()
	require.NotEmpty(t, dump1, "Dump should not be empty for non-empty timeline")
	require.Contains(t, dump1, "<|TIMELINE_", "Dump must use aitag-wrapped format")
	require.Contains(t, dump1, "<|TIMELINE_END_", "Dump must include aitag end markers")

	// 多次重复调用应字节级一致
	for i := 0; i < 5; i++ {
		time.Sleep(20 * time.Millisecond)
		dump := tl.Dump()
		require.Equal(t, dump1, dump, "Dump must be byte-identical across consecutive calls (iteration %d)", i)
	}

	// 同样断言 String() 与 Dump() 一致
	require.Equal(t, dump1, tl.String(), "String() must equal Dump()")
}
