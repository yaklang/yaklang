package aibalance

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaklang/yaklang/common/ai/aispec"
)

// TestIsRoundTripFlattenEligible_NoToolMarks 纯文本对话 round1 不应触发 flatten。
// 关键词: round2 flatten 触发条件 - 纯文本不触发
func TestIsRoundTripFlattenEligible_NoToolMarks(t *testing.T) {
	msgs := []aispec.ChatDetail{
		{Role: "user", Content: "hi"},
		{Role: "assistant", Content: "hello"},
		{Role: "user", Content: "next"},
	}
	assert.False(t, IsRoundTripFlattenEligible(msgs),
		"纯文本对话 round1 不应被识别为 round-trip 标记")
}

// TestIsRoundTripFlattenEligible_AssistantToolCalls assistant.tool_calls
// 出现时必须识别为 round-trip 触发。
// 关键词: round2 flatten 触发条件 - assistant.tool_calls
func TestIsRoundTripFlattenEligible_AssistantToolCalls(t *testing.T) {
	msgs := []aispec.ChatDetail{
		{Role: "user", Content: "what's the weather?"},
		{
			Role:    "assistant",
			Content: "",
			ToolCalls: []*aispec.ToolCall{{
				ID: "c1", Type: "function",
				Function: aispec.FuncReturn{Name: "get_weather", Arguments: `{"city":"BJ"}`},
			}},
		},
	}
	assert.True(t, IsRoundTripFlattenEligible(msgs),
		"assistant.tool_calls 必须被识别为 round-trip 触发")
}

// TestIsRoundTripFlattenEligible_RoleTool role=tool 单条消息也应触发。
// 关键词: round2 flatten 触发条件 - role=tool
func TestIsRoundTripFlattenEligible_RoleTool(t *testing.T) {
	msgs := []aispec.ChatDetail{
		{Role: "tool", ToolCallID: "c1", Name: "get_weather", Content: "{}"},
	}
	assert.True(t, IsRoundTripFlattenEligible(msgs),
		"role=tool 必须被识别为 round-trip 触发")
}

// TestFlattenToolCallsForRoundTrip_NoOpForPureText 纯文本对话 flatten 后必须
// 字段级一致 (顺序/role/content 全等), 让没 round-trip 字段的请求零副作用。
// 关键词: round2 flatten 零副作用, 纯文本不变形
func TestFlattenToolCallsForRoundTrip_NoOpForPureText(t *testing.T) {
	msgs := []aispec.ChatDetail{
		{Role: "system", Content: "sys"},
		{Role: "user", Content: "u1"},
		{Role: "assistant", Content: "a1"},
		{Role: "user", Content: "u2"},
	}
	out := FlattenToolCallsForRoundTrip(msgs)
	require.Len(t, out, len(msgs))
	for i := range msgs {
		assert.Equal(t, msgs[i].Role, out[i].Role, "role[%d]", i)
		assert.Equal(t, msgs[i].Content, out[i].Content, "content[%d]", i)
	}
}

// TestFlattenToolCallsForRoundTrip_AssistantToolCallsBecomeText
// assistant.tool_calls 必须被替换为 ReAct 文本风格, tool_calls 字段清空。
// 关键词: assistant.tool_calls flatten 输出验证
func TestFlattenToolCallsForRoundTrip_AssistantToolCallsBecomeText(t *testing.T) {
	msgs := []aispec.ChatDetail{
		{Role: "user", Content: "what's the weather in Beijing?"},
		{
			Role:    "assistant",
			Content: "",
			ToolCalls: []*aispec.ToolCall{{
				ID: "call_xyz", Type: "function",
				Function: aispec.FuncReturn{
					Name:      "get_current_weather",
					Arguments: `{"city":"Beijing"}`,
				},
			}},
		},
	}
	out := FlattenToolCallsForRoundTrip(msgs)
	require.Len(t, out, 2)

	// user 不变
	assert.Equal(t, "user", out[0].Role)
	assert.Equal(t, "what's the weather in Beijing?", out[0].Content)

	// assistant 被 flatten
	assert.Equal(t, "assistant", out[1].Role)
	assert.Empty(t, out[1].ToolCalls,
		"flatten 后 assistant.tool_calls 必须被清空, 否则上游 wrapper 仍会因不识别字段空回")
	contentStr, ok := out[1].Content.(string)
	require.True(t, ok, "flatten 后 content 必须是 string")
	assert.Contains(t, contentStr, `[tool_call`)
	assert.Contains(t, contentStr, `id="call_xyz"`)
	assert.Contains(t, contentStr, `name="get_current_weather"`)
	assert.Contains(t, contentStr, `{"city":"Beijing"}`)
	assert.Contains(t, contentStr, `[/tool_call]`)
}

// TestFlattenToolCallsForRoundTrip_RoleToolBecomesUser role=tool 必须改成
// role=user, content 必须包含 tool_call_id 与原 content。
// 关键词: role=tool flatten 输出验证
func TestFlattenToolCallsForRoundTrip_RoleToolBecomesUser(t *testing.T) {
	msgs := []aispec.ChatDetail{
		{Role: "tool", Name: "get_current_weather", ToolCallID: "call_xyz",
			Content: `{"temperature_c":21,"condition":"sunny"}`},
	}
	out := FlattenToolCallsForRoundTrip(msgs)
	require.Len(t, out, 1)
	assert.Equal(t, "user", out[0].Role,
		"role=tool 必须改成 user, 否则上游 wrapper 不识别 tool 角色")
	contentStr, ok := out[0].Content.(string)
	require.True(t, ok)
	assert.Contains(t, contentStr, `[tool_result`)
	assert.Contains(t, contentStr, `name="get_current_weather"`)
	assert.Contains(t, contentStr, `tool_call_id="call_xyz"`)
	assert.Contains(t, contentStr, `"temperature_c":21`)
	assert.Contains(t, contentStr, `[/tool_result]`)
}

// TestFlattenToolCallsForRoundTrip_FullRoundTrip 完整 round-trip 三轮消息
// (user -> assistant.tool_calls -> tool) flatten 后必须:
//   1. 长度不变
//   2. 三条都没有 OpenAI tool_calls round-trip 字段 (tool_calls / tool_call_id / role=tool)
//   3. 可识别原始工具名/参数/结果文本
//
// 关键词: 完整 round-trip flatten 端到端验证
func TestFlattenToolCallsForRoundTrip_FullRoundTrip(t *testing.T) {
	msgs := []aispec.ChatDetail{
		{Role: "user", Content: "weather in Beijing?"},
		{Role: "assistant", Content: "", ToolCalls: []*aispec.ToolCall{{
			ID: "c1", Type: "function",
			Function: aispec.FuncReturn{Name: "get_current_weather", Arguments: `{"city":"Beijing"}`},
		}}},
		{Role: "tool", Name: "get_current_weather", ToolCallID: "c1",
			Content: `{"temperature_c":21,"condition":"sunny"}`},
	}
	out := FlattenToolCallsForRoundTrip(msgs)
	require.Len(t, out, 3)

	for i, m := range out {
		assert.NotEqual(t, "tool", m.Role, "msg[%d] role 不应再有 tool", i)
		assert.Empty(t, m.ToolCalls, "msg[%d] 不应再带 tool_calls", i)
		assert.Empty(t, m.ToolCallID, "msg[%d] 不应再带 tool_call_id", i)
	}

	asStr := func(c any) string {
		s, _ := c.(string)
		return s
	}
	assert.Contains(t, asStr(out[1].Content), "get_current_weather",
		"flatten assistant content 必须保留工具名供模型还原对话上下文")
	assert.Contains(t, asStr(out[2].Content), `"temperature_c":21`,
		"flatten user (原 tool) content 必须保留工具结果文本供模型生成 NL 回答")
}

// TestResolveFlattenForModel_DefaultOff 默认无 env 时永远返回 false。
// 关键词: round2 flatten 默认关闭
func TestResolveFlattenForModel_DefaultOff(t *testing.T) {
	t.Setenv(envFlattenToolCallsForModels, "")
	t.Setenv(envFlattenToolCallsAll, "")
	resetFlattenEnvCacheForTest()
	t.Cleanup(resetFlattenEnvCacheForTest)
	assert.False(t, ResolveFlattenForModel("any-model", "any-wrapper"),
		"默认无 env 配置应该不启用 flatten")
}

// TestResolveFlattenForModel_EnvWhitelist env 白名单匹配 model/wrapper 名
// (大小写不敏感, 容忍 , ; 空白多种分隔)。
// 关键词: round2 flatten env 白名单匹配
func TestResolveFlattenForModel_EnvWhitelist(t *testing.T) {
	t.Setenv(envFlattenToolCallsForModels, "z-deepseek-v4-pro, Z-DEEPSEEK-V4-FLASH ; my-wrapper")
	t.Setenv(envFlattenToolCallsAll, "")
	resetFlattenEnvCacheForTest()
	t.Cleanup(resetFlattenEnvCacheForTest)

	assert.True(t, ResolveFlattenForModel("z-deepseek-v4-pro", ""))
	assert.True(t, ResolveFlattenForModel("Z-DEEPSEEK-V4-FLASH", ""))
	assert.True(t, ResolveFlattenForModel("anything", "MY-WRAPPER"))
	assert.False(t, ResolveFlattenForModel("openai-gpt-4", "openai-gpt-4"))
}

// TestResolveFlattenForModel_EnvAllSwitch envFlattenToolCallsAll=true 时
// 不需要再看白名单, 任何 model/wrapper 都启用 flatten。
// 关键词: round2 flatten 全局 kill switch
func TestResolveFlattenForModel_EnvAllSwitch(t *testing.T) {
	t.Setenv(envFlattenToolCallsForModels, "")
	t.Setenv(envFlattenToolCallsAll, "true")
	resetFlattenEnvCacheForTest()
	t.Cleanup(resetFlattenEnvCacheForTest)
	assert.True(t, ResolveFlattenForModel("any-model", ""))
	assert.True(t, ResolveFlattenForModel("", "any-wrapper"))
}

// TestFlattenToolCallsForRoundTrip_DoesNotMutateInput 验证 flatten 函数不会
// 改写入参 slice 中的 ChatDetail 字段, 调用方仍可拿原 messages 做其它用途。
// 关键词: round2 flatten 纯函数不改入参
func TestFlattenToolCallsForRoundTrip_DoesNotMutateInput(t *testing.T) {
	original := []aispec.ChatDetail{
		{Role: "assistant", Content: "", ToolCalls: []*aispec.ToolCall{{
			ID: "c1", Type: "function",
			Function: aispec.FuncReturn{Name: "n", Arguments: "{}"},
		}}},
		{Role: "tool", ToolCallID: "c1", Content: "result"},
	}
	_ = FlattenToolCallsForRoundTrip(original)
	assert.Equal(t, "assistant", original[0].Role)
	assert.NotEmpty(t, original[0].ToolCalls,
		"原 slice 的 tool_calls 不应被 flatten 影响")
	assert.Equal(t, "tool", original[1].Role,
		"原 slice 的 role 不应被 flatten 改写")
	assert.Equal(t, "c1", original[1].ToolCallID)
}

// TestFlattenToolCallsForRoundTrip_ChatContentArray 多模态 content 数组
// (含 image_url) 必须能 flatten 成 plain text, 让 ReAct 回灌时上游也能看到
// 图片 URL 文本表示。
// 关键词: round2 flatten 多模态 content 兼容
func TestFlattenToolCallsForRoundTrip_ChatContentArray(t *testing.T) {
	imgURL := "https://example.com/p.png"
	msgs := []aispec.ChatDetail{
		{
			Role: "assistant",
			Content: []*aispec.ChatContent{
				aispec.NewUserChatContentText("here is the chart:"),
				aispec.NewUserChatContentImageUrl(imgURL),
			},
			ToolCalls: []*aispec.ToolCall{{
				ID: "c1", Type: "function",
				Function: aispec.FuncReturn{Name: "draw_chart", Arguments: `{"x":1}`},
			}},
		},
	}
	out := FlattenToolCallsForRoundTrip(msgs)
	require.Len(t, out, 1)
	contentStr, ok := out[0].Content.(string)
	require.True(t, ok)
	assert.Contains(t, contentStr, "here is the chart:")
	assert.Contains(t, contentStr, imgURL,
		"多模态 content 数组中的 image_url 必须保留到 flatten 后文本")
	assert.Contains(t, contentStr, "draw_chart")
}

// TestSplitFlattenList 验证 env 配置多种分隔符都能解析。
// 关键词: env 分隔符兼容
func TestSplitFlattenList(t *testing.T) {
	out := splitFlattenList("a, b ;c\n d")
	got := map[string]bool{}
	for _, x := range out {
		x = strings.TrimSpace(x)
		if x != "" {
			got[x] = true
		}
	}
	for _, want := range []string{"a", "b", "c", "d"} {
		assert.True(t, got[want], "splitFlattenList missing %q (got=%v)", want, got)
	}
}
