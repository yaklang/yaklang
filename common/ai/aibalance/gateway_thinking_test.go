package aibalance

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/yaklang/yaklang/common/ai/aispec"
)

func TestEnableThinkingConfig_Aibalance(t *testing.T) {
	t.Run("thinking true sets ThinkingLevel=high", func(t *testing.T) {
		client := &GatewayClient{}
		client.LoadOption(
			aispec.WithType("aibalance"),
			aispec.WithAPIKey("test-key"),
			aispec.WithModel("memfit-qwen3.5-plus-free"),
			aispec.WithEnableThinking(true),
		)
		assert.Equal(t, "high", client.config.ThinkingLevel)
	})

	t.Run("thinking false sets ThinkingLevel=none", func(t *testing.T) {
		client := &GatewayClient{}
		client.LoadOption(
			aispec.WithType("aibalance"),
			aispec.WithAPIKey("test-key"),
			aispec.WithModel("memfit-qwen3.5-plus-free"),
			aispec.WithEnableThinking(false),
		)
		assert.Equal(t, "none", client.config.ThinkingLevel)
	})

	t.Run("no thinking option leaves empty", func(t *testing.T) {
		client := &GatewayClient{}
		client.LoadOption(
			aispec.WithType("aibalance"),
			aispec.WithAPIKey("test-key"),
			aispec.WithModel("memfit-qwen3.5-plus-free"),
		)
		assert.Empty(t, client.config.ThinkingLevel)
	})
}

func TestEnableThinkingConfig_Tongyi(t *testing.T) {
	t.Run("tongyi thinking true", func(t *testing.T) {
		config := aispec.NewDefaultAIConfig(
			aispec.WithType("tongyi"),
			aispec.WithAPIKey("test-key"),
			aispec.WithModel("qwen3.5-plus"),
			aispec.WithEnableThinking(true),
		)
		assert.Equal(t, "high", config.ThinkingLevel)
	})

	t.Run("tongyi no thinking option leaves empty", func(t *testing.T) {
		config := aispec.NewDefaultAIConfig(
			aispec.WithType("tongyi"),
			aispec.WithAPIKey("test-key"),
			aispec.WithModel("qwen3.5-plus"),
		)
		assert.Empty(t, config.ThinkingLevel)
	})
}

func TestEnableThinkingConfig_GenericDefault(t *testing.T) {
	t.Run("no type still sets ThinkingLevel=high", func(t *testing.T) {
		config := aispec.NewDefaultAIConfig(
			aispec.WithEnableThinking(true),
		)
		assert.Equal(t, "high", config.ThinkingLevel)
	})

	t.Run("openai type sets ThinkingLevel=high", func(t *testing.T) {
		config := aispec.NewDefaultAIConfig(
			aispec.WithType("openai"),
			aispec.WithAPIKey("test-key"),
			aispec.WithEnableThinking(true),
		)
		assert.Equal(t, "high", config.ThinkingLevel)
	})
}

func TestEnableThinkingConfig_Volcengine(t *testing.T) {
	t.Run("volcengine thinking true", func(t *testing.T) {
		config := aispec.NewDefaultAIConfig(
			aispec.WithType("volcengine"),
			aispec.WithAPIKey("test-key"),
			aispec.WithEnableThinking(true),
		)
		assert.Equal(t, "high", config.ThinkingLevel)
		m := aispec.ThinkingExtraBodyForProvider(config.Type, config.Model, config.BaseURL, config.Domain, "", config.ThinkingLevel)
		inner, ok := m["thinking"].(map[string]any)
		if !ok {
			t.Fatalf("expected thinking map, got %T", m["thinking"])
		}
		if inner["type"] != "enabled" {
			t.Fatalf("expected thinking type='enabled', got '%v'", inner["type"])
		}
	})

	t.Run("siliconflow thinking true", func(t *testing.T) {
		config := aispec.NewDefaultAIConfig(
			aispec.WithType("siliconflow"),
			aispec.WithAPIKey("test-key"),
			aispec.WithModel("deepseek-ai/DeepSeek-V4-Flash"),
			aispec.WithEnableThinking(true),
		)
		assert.Equal(t, "high", config.ThinkingLevel)
	})
}