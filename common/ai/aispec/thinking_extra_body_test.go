package aispec

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestThinkingExtraBodyForProvider_QwenHost(t *testing.T) {
	m := ThinkingExtraBodyForProvider("", "foo", "https://dashscope.aliyuncs.com/compatible-mode/v1", "", true)
	require.Contains(t, m, "enable_thinking")
	assert.Equal(t, true, m["enable_thinking"])
	m2 := ThinkingExtraBodyForProvider("", "foo", "", "dashscope-intl.aliyuncs.com", false)
	assert.Equal(t, false, m2["enable_thinking"])
}

func TestThinkingExtraBodyForProvider_QwenModel(t *testing.T) {
	m := ThinkingExtraBodyForProvider("", "my-qwen-max", "https://example.com", "", true)
	assert.Equal(t, true, m["enable_thinking"])
}

func TestThinkingExtraBodyForProvider_DeepseekHost(t *testing.T) {
	m := ThinkingExtraBodyForProvider("", "x", "https://api.deepseek.com/v1", "", true)
	inner, ok := m["thinking"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "enabled", inner["type"])
}

func TestThinkingExtraBodyForProvider_DeepseekModel(t *testing.T) {
	m := ThinkingExtraBodyForProvider("", "deepseek-chat", "https://proxy.local", "", false)
	inner, ok := m["thinking"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "disabled", inner["type"])
}

func TestThinkingExtraBodyForProvider_OpenAIHost(t *testing.T) {
	m := ThinkingExtraBodyForProvider("", "custom", "https://api.openai.com/v1/chat/completions", "", true)
	inner, ok := m["reasoning"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "medium", inner["effort"])
	m2 := ThinkingExtraBodyForProvider("", "x", "", "generativelanguage.googleapis.com", false)
	inner2, ok := m2["reasoning"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "none", inner2["effort"])
}

func TestThinkingExtraBodyForProvider_OpenAIModel(t *testing.T) {
	m := ThinkingExtraBodyForProvider("", "gpt-4.1-mini", "https://unknown.example", "", false)
	inner, ok := m["reasoning"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "none", inner["effort"])
}

func TestThinkingExtraBodyForProvider_Default(t *testing.T) {
	m := ThinkingExtraBodyForProvider("", "unknown-model-xyz", "https://unknown.example", "", true)
	inner, ok := m["thinking"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "enabled", inner["type"])
}

func TestThinkingExtraBodyForProvider_TypeBeforeHost(t *testing.T) {
	// tongyi 厂商名优先：无 dashscope 域名也应走 Qwen 的 enable_thinking
	m := ThinkingExtraBodyForProvider("tongyi", "foo", "https://proxy.example/v1", "", true)
	assert.Equal(t, true, m["enable_thinking"])
}

func TestThinkingExtraBodyForProvider_VolcengineTypeUsesThinkingMap(t *testing.T) {
	m := ThinkingExtraBodyForProvider("volcengine", "x", "", "", true)
	inner, ok := m["thinking"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "enabled", inner["type"])
}

func TestThinkingExtraBodyForProvider_OpenAITypeBeforeModel(t *testing.T) {
	m := ThinkingExtraBodyForProvider("openai", "non-gpt-id", "https://other.example", "", true)
	inner, ok := m["reasoning"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "medium", inner["effort"])
}
