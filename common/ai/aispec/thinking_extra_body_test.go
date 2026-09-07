package aispec

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ===========================================================================
// Qwen / 百炼
// ===========================================================================

func TestThinkingExtraBodyForProvider_QwenHost(t *testing.T) {
	m := ThinkingExtraBodyForProvider("", "foo", "https://dashscope.aliyuncs.com/compatible-mode/v1", "", "", "high")
	require.Contains(t, m, "enable_thinking")
	assert.Equal(t, true, m["enable_thinking"])
	assert.Equal(t, "high", m["reasoning_effort"])
	m2 := ThinkingExtraBodyForProvider("", "foo", "", "dashscope-intl.aliyuncs.com", "", "none")
	assert.Equal(t, false, m2["enable_thinking"])
}

func TestThinkingExtraBodyForProvider_QwenModel(t *testing.T) {
	m := ThinkingExtraBodyForProvider("", "my-qwen-max", "https://example.com", "", "", "medium")
	assert.Equal(t, true, m["enable_thinking"])
	assert.Equal(t, "medium", m["reasoning_effort"])
}

func TestThinkingExtraBodyForProvider_QwenResponses(t *testing.T) {
	// Qwen Responses 模式：使用 reasoning.effort 嵌套对象
	m := ThinkingExtraBodyForProvider("tongyi", "qwen3.7-max", "", "", "responses", "high")
	inner, ok := m["reasoning"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "high", inner["effort"])
	_, hasEnableThinking := m["enable_thinking"]
	assert.False(t, hasEnableThinking, "Responses 模式不应注入 enable_thinking")

	// Qwen Responses none
	m = ThinkingExtraBodyForProvider("tongyi", "qwen3.7-max", "", "", "responses", "none")
	inner, ok = m["reasoning"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "none", inner["effort"])
}

// ===========================================================================
// DeepSeek
// ===========================================================================

func TestThinkingExtraBodyForProvider_DeepseekHost(t *testing.T) {
	m := ThinkingExtraBodyForProvider("", "x", "https://api.deepseek.com/v1", "", "", "high")
	inner, ok := m["thinking"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "enabled", inner["type"])
	assert.Equal(t, "high", m["reasoning_effort"])
}

func TestThinkingExtraBodyForProvider_DeepseekModel(t *testing.T) {
	m := ThinkingExtraBodyForProvider("", "deepseek-chat", "https://proxy.local", "", "", "none")
	inner, ok := m["thinking"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "disabled", inner["type"])
}

func TestThinkingExtraBodyForProvider_DeepSeekChatCompletions(t *testing.T) {
	// Chat Completions: thinking 开关 + reasoning_effort
	m := ThinkingExtraBodyForProvider("deepseek", "deepseek-v4-pro", "", "", "", "medium")
	assert.Equal(t, "enabled", m["thinking"].(map[string]any)["type"])
	assert.Equal(t, "medium", m["reasoning_effort"])

	m = ThinkingExtraBodyForProvider("deepseek", "deepseek-v4-pro", "", "", "", "max")
	assert.Equal(t, "enabled", m["thinking"].(map[string]any)["type"])
	assert.Equal(t, "max", m["reasoning_effort"])

	m = ThinkingExtraBodyForProvider("deepseek", "deepseek-v4-pro", "", "", "", "xhigh")
	assert.Equal(t, "xhigh", m["reasoning_effort"])
}

func TestThinkingExtraBodyForProvider_DeepSeekResponses(t *testing.T) {
	// DeepSeek Responses 模式：只需 reasoning.effort，无需 thinking 开关
	m := ThinkingExtraBodyForProvider("deepseek", "deepseek-v4-pro", "", "", "responses", "high")
	inner, ok := m["reasoning"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "high", inner["effort"])
	_, hasThinking := m["thinking"]
	assert.False(t, hasThinking, "Responses 模式不应注入 thinking 开关")
	_, hasEffort := m["reasoning_effort"]
	assert.False(t, hasEffort, "Responses 模式不应注入顶层 reasoning_effort")

	// DeepSeek Responses none
	m = ThinkingExtraBodyForProvider("deepseek", "deepseek-v4-pro", "", "", "responses", "none")
	inner, ok = m["reasoning"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "none", inner["effort"])
	_, hasThinking = m["thinking"]
	assert.False(t, hasThinking)
}

// ===========================================================================
// OpenAI / Gemini / OpenRouter
// ===========================================================================

func TestThinkingExtraBodyForProvider_OpenAIHost(t *testing.T) {
	// Responses API
	m := ThinkingExtraBodyForProvider("", "custom", "https://api.openai.com/v1/chat/completions", "", "responses", "medium")
	inner, ok := m["reasoning"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "medium", inner["effort"])
	// Chat Completions API
	m2 := ThinkingExtraBodyForProvider("", "x", "", "generativelanguage.googleapis.com", "chat_completions", "none")
	effort, ok := m2["reasoning_effort"]
	require.True(t, ok)
	assert.Equal(t, "none", effort)
}

func TestThinkingExtraBodyForProvider_OpenAIModel(t *testing.T) {
	m := ThinkingExtraBodyForProvider("", "gpt-4.1-mini", "https://unknown.example", "", "chat_completions", "none")
	effort, ok := m["reasoning_effort"]
	require.True(t, ok)
	assert.Equal(t, "none", effort)
}

func TestThinkingExtraBodyForProvider_OpenAIEffortLevels(t *testing.T) {
	// low
	m := ThinkingExtraBodyForProvider("openai", "o3-mini", "", "", "responses", "low")
	inner, ok := m["reasoning"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "low", inner["effort"])

	// high
	m = ThinkingExtraBodyForProvider("openai", "o3-mini", "", "", "responses", "high")
	inner, ok = m["reasoning"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "high", inner["effort"])

	// xhigh → passthrough（降级由上层处理）
	m = ThinkingExtraBodyForProvider("openai", "o3-mini", "", "", "responses", "xhigh")
	inner, ok = m["reasoning"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "xhigh", inner["effort"])

	// max → passthrough
	m = ThinkingExtraBodyForProvider("openai", "o3-mini", "", "", "responses", "max")
	inner, ok = m["reasoning"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "max", inner["effort"])
}

func TestThinkingExtraBodyForProvider_OpenAIChatCompletions(t *testing.T) {
	// Chat Completions API 应该用顶层 reasoning_effort
	m := ThinkingExtraBodyForProvider("openai", "o3-mini", "", "", "chat_completions", "high")
	effort, ok := m["reasoning_effort"]
	require.True(t, ok)
	assert.Equal(t, "high", effort)
	_, hasNested := m["reasoning"]
	assert.False(t, hasNested, "Chat Completions should not use nested reasoning object")
}

func TestThinkingExtraBodyForProvider_OpenAIAutoNoInject(t *testing.T) {
	// level 为空时不应注入任何参数
	m := ThinkingExtraBodyForProvider("openai", "gpt-5", "", "", "responses", "")
	assert.Nil(t, m)
	m = ThinkingExtraBodyForProvider("openai", "gpt-5", "", "", "chat_completions", "")
	assert.Nil(t, m)
}

// ===========================================================================
// xAI / Grok
// ===========================================================================

func TestThinkingExtraBodyForProvider_xAI(t *testing.T) {
	// Grok Responses: passthrough（降级由上层处理）
	m := ThinkingExtraBodyForProvider("xai", "grok-4.6", "", "", "responses", "max")
	inner, ok := m["reasoning"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "max", inner["effort"])

	// Grok Responses none → passthrough
	m = ThinkingExtraBodyForProvider("xai", "grok-4.6", "", "", "responses", "none")
	inner, ok = m["reasoning"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "none", inner["effort"])

	// Grok 4.3 none → passthrough
	m = ThinkingExtraBodyForProvider("xai", "grok-4.3", "", "", "responses", "none")
	inner, ok = m["reasoning"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "none", inner["effort"])

	// Grok Chat Completions
	m = ThinkingExtraBodyForProvider("xai", "grok-4.6", "", "", "chat_completions", "high")
	effort, ok := m["reasoning_effort"]
	require.True(t, ok)
	assert.Equal(t, "high", effort)
}

func TestThinkingExtraBodyForProvider_xAIAutoNoInject(t *testing.T) {
	m := ThinkingExtraBodyForProvider("xai", "grok-4.6", "", "", "responses", "")
	assert.Nil(t, m)
	m = ThinkingExtraBodyForProvider("xai", "grok-4.6", "", "", "chat_completions", "")
	assert.Nil(t, m)
}

// ===========================================================================
// Anthropic / Claude
// ===========================================================================

func TestThinkingExtraBodyForProvider_Anthropic(t *testing.T) {
	// Claude ≤4.6: budget_tokens
	m := ThinkingExtraBodyForProvider("anthropic", "claude-sonnet-4-6", "", "", "", "medium")
	inner, ok := m["thinking"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "enabled", inner["type"])
	assert.Equal(t, int64(8192), inner["budget_tokens"])

	// Claude 4.7+: adaptive only
	m = ThinkingExtraBodyForProvider("anthropic", "claude-4.7-sonnet", "", "", "", "high")
	inner, ok = m["thinking"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "adaptive", inner["type"])

	// Claude 5.x: adaptive only
	m = ThinkingExtraBodyForProvider("anthropic", "claude-opus-5", "", "", "", "max")
	inner, ok = m["thinking"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "adaptive", inner["type"])

	// Claude none: 不注入
	m = ThinkingExtraBodyForProvider("anthropic", "claude-sonnet-4-6", "", "", "", "none")
	assert.Nil(t, m)
}

func TestThinkingExtraBodyForProvider_AnthropicDisplay(t *testing.T) {
	// Claude 4.7+ adaptive 模式应包含 display 字段
	m := ThinkingExtraBodyForProvider("anthropic", "claude-4.7-sonnet", "", "", "", "high")
	inner, ok := m["thinking"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "adaptive", inner["type"])
	assert.Equal(t, "summarized", inner["display"])

	// Claude ≤4.6 budget_tokens 模式应包含 display 字段
	m = ThinkingExtraBodyForProvider("anthropic", "claude-sonnet-4-6", "", "", "", "medium")
	inner, ok = m["thinking"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "enabled", inner["type"])
	assert.Equal(t, "detailed", inner["display"])
	assert.Equal(t, int64(8192), inner["budget_tokens"])
}

// ===========================================================================
// Kimi / Moonshot
// ===========================================================================

func TestThinkingExtraBodyForProvider_KimiK3(t *testing.T) {
	// K3: reasoning_effort 控制（passthrough，降级由上层处理）
	m := ThinkingExtraBodyForProvider("moonshot", "kimi-k3", "", "", "", "high")
	assert.Equal(t, "high", m["reasoning_effort"])

	m = ThinkingExtraBodyForProvider("moonshot", "kimi-k3", "", "", "", "none")
	assert.Equal(t, "none", m["reasoning_effort"])

	m = ThinkingExtraBodyForProvider("moonshot", "kimi-k3", "", "", "", "medium")
	assert.Equal(t, "medium", m["reasoning_effort"])

	m = ThinkingExtraBodyForProvider("moonshot", "kimi-k3", "", "", "", "max")
	assert.Equal(t, "max", m["reasoning_effort"])

	m = ThinkingExtraBodyForProvider("moonshot", "kimi-k3", "", "", "", "xhigh")
	assert.Equal(t, "xhigh", m["reasoning_effort"])
}

func TestThinkingExtraBodyForProvider_KimiK2(t *testing.T) {
	// K2.x: thinking.type 开关
	m := ThinkingExtraBodyForProvider("moonshot", "kimi-k2.6", "", "", "", "high")
	inner, ok := m["thinking"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "enabled", inner["type"])

	m = ThinkingExtraBodyForProvider("moonshot", "kimi-k2.6", "", "", "", "none")
	inner, ok = m["thinking"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "disabled", inner["type"])
}

func TestThinkingExtraBodyForProvider_KimiK2Keep(t *testing.T) {
	// K2.x enabled 时应包含 keep: "all"
	m := ThinkingExtraBodyForProvider("moonshot", "kimi-k2.6", "", "", "", "high")
	inner, ok := m["thinking"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "enabled", inner["type"])
	assert.Equal(t, "all", inner["keep"])
}

func TestThinkingExtraBodyForProvider_KimiResponses(t *testing.T) {
	// K3 Responses: reasoning.effort
	m := ThinkingExtraBodyForProvider("moonshot", "kimi-k3", "", "", "responses", "high")
	inner, ok := m["reasoning"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "high", inner["effort"])
	_, hasThinking := m["thinking"]
	assert.False(t, hasThinking, "K3 Responses 不应注入 thinking 开关")
	_, hasEffort := m["reasoning_effort"]
	assert.False(t, hasEffort, "Responses 模式不应注入顶层 reasoning_effort")

	// K2.x Responses: reasoning.effort（不使用 thinking.type 开关）
	m = ThinkingExtraBodyForProvider("moonshot", "kimi-k2.6", "", "", "responses", "medium")
	inner, ok = m["reasoning"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "medium", inner["effort"])
	_, hasThinking = m["thinking"]
	assert.False(t, hasThinking, "K2 Responses 不应注入 thinking 开关")

	// Kimi Responses none
	m = ThinkingExtraBodyForProvider("moonshot", "kimi-k3", "", "", "responses", "none")
	inner, ok = m["reasoning"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "none", inner["effort"])

	// Kimi Responses auto → 不注入
	m = ThinkingExtraBodyForProvider("moonshot", "kimi-k3", "", "", "responses", "")
	assert.Nil(t, m)
}

// ===========================================================================
// MiniMax
// ===========================================================================

func TestThinkingExtraBodyForProvider_MiniMaxUnderTongyi(t *testing.T) {
	// MiniMax 经百炼(tongyi)代理时，必须用 thinking.type，而非 enable_thinking。
	mOff := ThinkingExtraBodyForProvider("tongyi", "MiniMax/MiniMax-M3", "https://dashscope.aliyuncs.com/compatible-mode/v1/chat/completions", "", "", "none")
	_, hasEnableThinking := mOff["enable_thinking"]
	assert.False(t, hasEnableThinking, "MiniMax 不应注入 enable_thinking")
	innerOff, ok := mOff["thinking"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "disabled", innerOff["type"])

	// 开启思考使用 adaptive（MiniMax 不接受 enabled）。
	mOn := ThinkingExtraBodyForProvider("tongyi", "MiniMax/MiniMax-M3", "", "", "", "high")
	innerOn, ok := mOn["thinking"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "adaptive", innerOn["type"])
}

func TestThinkingExtraBodyForProvider_MiniMaxResponsesStaysUnique(t *testing.T) {
	// MiniMax 在 Responses 模式下仍使用独特的 thinking.type，而非 reasoning.effort
	m := ThinkingExtraBodyForProvider("tongyi", "MiniMax/MiniMax-M3", "", "", "responses", "high")
	inner, ok := m["thinking"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "adaptive", inner["type"])
	_, hasReasoning := m["reasoning"]
	assert.False(t, hasReasoning, "MiniMax Responses 不应注入 reasoning.effort")

	m = ThinkingExtraBodyForProvider("tongyi", "MiniMax/MiniMax-M3", "", "", "responses", "none")
	inner, ok = m["thinking"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "disabled", inner["type"])
	_, hasReasoning = m["reasoning"]
	assert.False(t, hasReasoning, "MiniMax Responses none 不应注入 reasoning.effort")
}

// ===========================================================================
// 通用 / fallback
// ===========================================================================

func TestThinkingExtraBodyForProvider_Default(t *testing.T) {
	m := ThinkingExtraBodyForProvider("", "unknown-model-xyz", "https://unknown.example", "", "", "high")
	inner, ok := m["thinking"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "enabled", inner["type"])

	m2 := ThinkingExtraBodyForProvider("", "unknown-model-xyz", "https://unknown.example", "", "", "none")
	inner2, ok := m2["thinking"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "disabled", inner2["type"])
}

func TestThinkingExtraBodyForProvider_DefaultResponses(t *testing.T) {
	// Default Responses: reasoning.effort
	m := ThinkingExtraBodyForProvider("", "unknown-model-xyz", "https://unknown.example", "", "responses", "high")
	inner, ok := m["reasoning"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "high", inner["effort"])
	_, hasThinking := m["thinking"]
	assert.False(t, hasThinking, "Default Responses 不应注入 thinking 开关")

	// Default Responses none → 不注入
	m = ThinkingExtraBodyForProvider("", "unknown-model-xyz", "https://unknown.example", "", "responses", "none")
	assert.Nil(t, m, "Default Responses none 不应注入任何参数")

	// Default Responses auto → 不注入
	m = ThinkingExtraBodyForProvider("", "unknown-model-xyz", "https://unknown.example", "", "responses", "")
	assert.Nil(t, m)

	// Default Chat Completions 仍使用 thinking.type + reasoning_effort
	m = ThinkingExtraBodyForProvider("", "unknown-model-xyz", "https://unknown.example", "", "chat_completions", "high")
	inner, ok = m["thinking"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "enabled", inner["type"])
	assert.Equal(t, "high", m["reasoning_effort"])
}

func TestThinkingExtraBodyForProvider_TypeBeforeHost(t *testing.T) {
	// tongyi 厂商名优先：无 dashscope 域名也应走 Qwen 的 enable_thinking
	m := ThinkingExtraBodyForProvider("tongyi", "foo", "https://proxy.example/v1", "", "", "low")
	assert.Equal(t, true, m["enable_thinking"])
	assert.Equal(t, "low", m["reasoning_effort"])
}

func TestThinkingExtraBodyForProvider_VolcengineTypeUsesThinkingMap(t *testing.T) {
	m := ThinkingExtraBodyForProvider("volcengine", "x", "", "", "", "high")
	inner, ok := m["thinking"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "enabled", inner["type"])
}

func TestThinkingExtraBodyForProvider_FamilyResponses(t *testing.T) {
	// volcengine Responses: reasoning.effort
	m := ThinkingExtraBodyForProvider("volcengine", "doubao-v2", "", "", "responses", "high")
	inner, ok := m["reasoning"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "high", inner["effort"])
	_, hasThinking := m["thinking"]
	assert.False(t, hasThinking, "Family Responses 不应注入 thinking 开关")
	_, hasEffort := m["reasoning_effort"]
	assert.False(t, hasEffort, "Responses 模式不应注入顶层 reasoning_effort")

	// chatglm Responses
	m = ThinkingExtraBodyForProvider("chatglm", "glm-4", "", "", "responses", "medium")
	inner, ok = m["reasoning"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "medium", inner["effort"])

	// siliconflow Responses none
	m = ThinkingExtraBodyForProvider("siliconflow", "glm-4", "", "", "responses", "none")
	assert.Nil(t, m, "Family Responses none 不应注入任何参数")

	// Family Responses auto → 不注入
	m = ThinkingExtraBodyForProvider("volcengine", "doubao-v2", "", "", "responses", "")
	assert.Nil(t, m)

	// Family Chat Completions 仍使用 thinking.type + reasoning_effort
	m = ThinkingExtraBodyForProvider("volcengine", "doubao-v2", "", "", "chat_completions", "high")
	inner, ok = m["thinking"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "enabled", inner["type"])
	assert.Equal(t, "high", m["reasoning_effort"])
}

func TestThinkingExtraBodyForProvider_OpenAITypeBeforeModel(t *testing.T) {
	m := ThinkingExtraBodyForProvider("openai", "non-gpt-id", "https://other.example", "", "responses", "medium")
	inner, ok := m["reasoning"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "medium", inner["effort"])
}

// ===========================================================================
// legacy 兼容
// ===========================================================================

func TestMapReasoningEffortToThinkingConfig(t *testing.T) {
	tests := []struct {
		effort string
		want   string
	}{
		{"", ""},
		{"auto", ""},
		{"default", ""},
		{"off", "none"},
		{"none", "none"},
		{"disabled", "none"},
		{"low", "low"},
		{"medium", "medium"},
		{"high", "high"},
		{"HIGH", "high"},
		{"xhigh", "xhigh"},
		{"max", "max"},
		{"MAX", "max"},
		{"turbo", "turbo"},
	}
	for _, tt := range tests {
		got := MapReasoningEffortToThinkingConfig(tt.effort)
		assert.Equal(t, tt.want, got, "effort=%q", tt.effort)
	}
}