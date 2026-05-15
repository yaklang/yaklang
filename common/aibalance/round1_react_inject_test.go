package aibalance

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/ai/aispec"
)

// round1_react_inject_test.go 验证 InjectToolsAsReactPrompt 的纯函数契约 + 边界 case.
//
// 关键词: aibalance round1 react inject 单测, ReAct system prompt, 纯函数零副作用

func sampleWeatherTool() aispec.Tool {
	return aispec.Tool{
		Type: "function",
		Function: aispec.ToolFunction{
			Name:        "get_current_weather",
			Description: "Get current weather of a city.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"city": map[string]any{"type": "string"},
				},
				"required": []string{"city"},
			},
		},
	}
}

func sampleNewsTool() aispec.Tool {
	return aispec.Tool{
		Type: "function",
		Function: aispec.ToolFunction{
			Name:        "fetch_news",
			Description: "Fetch latest news of a topic.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"topic": map[string]any{"type": "string"},
				},
			},
		},
	}
}

// 关键词: inject empty tools => no-op
func TestInjectToolsAsReactPrompt_NoTools(t *testing.T) {
	msgs := []aispec.ChatDetail{{Role: "user", Content: "hi"}}
	out := InjectToolsAsReactPrompt(msgs, nil)
	assert.Equal(t, msgs, out)
}

// 关键词: inject append to existing last system message
func TestInjectToolsAsReactPrompt_AppendToLastSystem(t *testing.T) {
	msgs := []aispec.ChatDetail{
		{Role: "system", Content: "Existing rules."},
		{Role: "user", Content: "What's the weather in Beijing?"},
	}
	out := InjectToolsAsReactPrompt(msgs, []aispec.Tool{sampleWeatherTool()})
	if assert.Equal(t, 2, len(out)) {
		assert.Equal(t, "system", out[0].Role)
		content := chatContentToPlainText(out[0].Content)
		assert.True(t, strings.HasPrefix(content, "Existing rules."), "system prefix preserved")
		assert.Contains(t, content, "get_current_weather", "tool name injected")
		assert.Contains(t, content, "[tool_call name=TOOL_NAME]JSON_ARGUMENTS[/tool_call]", "ReAct format header")
	}
	// 原 msgs 不被修改
	assert.Equal(t, "Existing rules.", msgs[0].Content)
}

// 关键词: inject prepend a new system when no system exists
func TestInjectToolsAsReactPrompt_PrependSystemIfMissing(t *testing.T) {
	msgs := []aispec.ChatDetail{
		{Role: "user", Content: "ask"},
	}
	out := InjectToolsAsReactPrompt(msgs, []aispec.Tool{sampleNewsTool()})
	if assert.Equal(t, 2, len(out)) {
		assert.Equal(t, "system", out[0].Role)
		content := chatContentToPlainText(out[0].Content)
		assert.Contains(t, content, "fetch_news")
		assert.Equal(t, "user", out[1].Role)
	}
	// 原 msgs 不被修改
	assert.Equal(t, 1, len(msgs))
}

// 关键词: inject multiple system messages, append only to the last one
func TestInjectToolsAsReactPrompt_AppendToLastWhenMultipleSystem(t *testing.T) {
	msgs := []aispec.ChatDetail{
		{Role: "system", Content: "Rule A"},
		{Role: "user", Content: "q1"},
		{Role: "system", Content: "Rule B"},
		{Role: "user", Content: "q2"},
	}
	out := InjectToolsAsReactPrompt(msgs, []aispec.Tool{sampleWeatherTool()})
	if assert.Equal(t, 4, len(out)) {
		assert.Equal(t, "Rule A", out[0].Content, "first system untouched")
		assert.Contains(t, chatContentToPlainText(out[2].Content), "Rule B", "last system base preserved")
		assert.Contains(t, chatContentToPlainText(out[2].Content), "get_current_weather", "tool injected at last system")
	}
}

// 关键词: inject parallel tools => 两个工具都出现在 prompt
func TestInjectToolsAsReactPrompt_MultipleTools(t *testing.T) {
	msgs := []aispec.ChatDetail{{Role: "user", Content: "go"}}
	out := InjectToolsAsReactPrompt(msgs, []aispec.Tool{sampleWeatherTool(), sampleNewsTool()})
	assert.Equal(t, 2, len(out))
	sys := chatContentToPlainText(out[0].Content)
	assert.Contains(t, sys, "get_current_weather")
	assert.Contains(t, sys, "fetch_news")
}

// 关键词: inject does not mutate input slice (defensive)
func TestInjectToolsAsReactPrompt_NoMutation(t *testing.T) {
	msgs := []aispec.ChatDetail{
		{Role: "system", Content: "Rule"},
		{Role: "user", Content: "ask"},
	}
	before := msgs[0].Content
	_ = InjectToolsAsReactPrompt(msgs, []aispec.Tool{sampleWeatherTool()})
	assert.Equal(t, before, msgs[0].Content, "must not mutate input")
}

// 关键词: format helpers, 与 round2 flatten 文本格式一致
func TestFormatReactTools(t *testing.T) {
	tcText := FormatReactToolCallText("get_weather", `{"city":"BJ"}`)
	assert.Equal(t, `[tool_call name=get_weather]{"city":"BJ"}[/tool_call]`, tcText)

	tcTextEmpty := FormatReactToolCallText("ping", "")
	assert.Equal(t, `[tool_call name=ping]{}[/tool_call]`, tcTextEmpty)

	trText := FormatReactToolResultText("call_1", `{"ok":true}`)
	assert.Equal(t, `[tool_result tool_call_id=call_1]{"ok":true}[/tool_result]`, trText)
}
