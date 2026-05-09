package aibalance

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/yaklang/yaklang/common/ai/aispec"
)

func TestEnableThinkingConfig_Aibalance(t *testing.T) {
	t.Run("thinking true sets pointer", func(t *testing.T) {
		client := &GatewayClient{}
		client.LoadOption(
			aispec.WithType("aibalance"),
			aispec.WithAPIKey("test-key"),
			aispec.WithModel("memfit-qwen3.5-plus-free"),
			aispec.WithEnableThinking(true),
		)
		require.NotNil(t, client.config.EnableThinking)
		if !*client.config.EnableThinking {
			t.Fatalf("expected EnableThinking=true")
		}
	})

	t.Run("thinking false sets pointer", func(t *testing.T) {
		client := &GatewayClient{}
		client.LoadOption(
			aispec.WithType("aibalance"),
			aispec.WithAPIKey("test-key"),
			aispec.WithModel("memfit-qwen3.5-plus-free"),
			aispec.WithEnableThinking(false),
		)
		require.NotNil(t, client.config.EnableThinking)
		if *client.config.EnableThinking {
			t.Fatalf("expected EnableThinking=false")
		}
	})

	t.Run("no thinking option leaves nil", func(t *testing.T) {
		client := &GatewayClient{}
		client.LoadOption(
			aispec.WithType("aibalance"),
			aispec.WithAPIKey("test-key"),
			aispec.WithModel("memfit-qwen3.5-plus-free"),
		)
		if client.config.EnableThinking != nil {
			t.Fatalf("expected EnableThinking=nil, got %v", client.config.EnableThinking)
		}
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
		require.NotNil(t, config.EnableThinking)
		if !*config.EnableThinking {
			t.Fatalf("expected EnableThinking=true")
		}
	})

	t.Run("tongyi no thinking option leaves nil", func(t *testing.T) {
		config := aispec.NewDefaultAIConfig(
			aispec.WithType("tongyi"),
			aispec.WithAPIKey("test-key"),
			aispec.WithModel("qwen3.5-plus"),
		)
		if config.EnableThinking != nil {
			t.Fatalf("expected EnableThinking=nil")
		}
	})
}

func TestEnableThinkingConfig_GenericDefault(t *testing.T) {
	t.Run("no type still sets pointer", func(t *testing.T) {
		config := aispec.NewDefaultAIConfig(
			aispec.WithEnableThinking(true),
		)
		require.NotNil(t, config.EnableThinking)
		if !*config.EnableThinking {
			t.Fatalf("expected EnableThinking=true")
		}
	})

	t.Run("openai type sets pointer", func(t *testing.T) {
		config := aispec.NewDefaultAIConfig(
			aispec.WithType("openai"),
			aispec.WithAPIKey("test-key"),
			aispec.WithEnableThinking(true),
		)
		require.NotNil(t, config.EnableThinking)
		if !*config.EnableThinking {
			t.Fatalf("expected EnableThinking=true")
		}
	})
}

func TestEnableThinkingConfig_Volcengine(t *testing.T) {
	t.Run("volcengine thinking true", func(t *testing.T) {
		config := aispec.NewDefaultAIConfig(
			aispec.WithType("volcengine"),
			aispec.WithAPIKey("test-key"),
			aispec.WithEnableThinking(true),
		)
		require.NotNil(t, config.EnableThinking)
		if !*config.EnableThinking {
			t.Fatalf("expected EnableThinking=true")
		}
		m := aispec.ThinkingExtraBodyForProvider(config.Type, config.Model, config.BaseURL, config.Domain, *config.EnableThinking)
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
		require.NotNil(t, config.EnableThinking)
		if !*config.EnableThinking {
			t.Fatalf("expected EnableThinking=true")
		}
	})
}
