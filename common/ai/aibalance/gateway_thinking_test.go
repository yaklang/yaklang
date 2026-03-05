package aibalance

import (
	"testing"

	"github.com/yaklang/yaklang/common/ai/aispec"
)

func TestEnableThinkingConfig_Aibalance(t *testing.T) {
	t.Run("thinking true sets field and value", func(t *testing.T) {
		client := &GatewayClient{}
		client.LoadOption(
			aispec.WithType("aibalance"),
			aispec.WithAPIKey("test-key"),
			aispec.WithModel("memfit-qwen3.5-plus-free"),
			aispec.WithEnableThinking(true),
		)

		if client.config.EnableThinkingField != "enable_thinking" {
			t.Fatalf("expected EnableThinkingField='enable_thinking', got '%s'", client.config.EnableThinkingField)
		}
		val, ok := client.config.EnableThinkingValue.(bool)
		if !ok {
			t.Fatalf("expected EnableThinkingValue to be bool, got %T", client.config.EnableThinkingValue)
		}
		if !val {
			t.Fatalf("expected EnableThinkingValue=true, got false")
		}
	})

	t.Run("thinking false sets field and value", func(t *testing.T) {
		client := &GatewayClient{}
		client.LoadOption(
			aispec.WithType("aibalance"),
			aispec.WithAPIKey("test-key"),
			aispec.WithModel("memfit-qwen3.5-plus-free"),
			aispec.WithEnableThinking(false),
		)

		if client.config.EnableThinkingField != "enable_thinking" {
			t.Fatalf("expected EnableThinkingField='enable_thinking', got '%s'", client.config.EnableThinkingField)
		}
		val, ok := client.config.EnableThinkingValue.(bool)
		if !ok {
			t.Fatalf("expected EnableThinkingValue to be bool, got %T", client.config.EnableThinkingValue)
		}
		if val {
			t.Fatalf("expected EnableThinkingValue=false, got true")
		}
	})

	t.Run("no thinking option leaves field empty", func(t *testing.T) {
		client := &GatewayClient{}
		client.LoadOption(
			aispec.WithType("aibalance"),
			aispec.WithAPIKey("test-key"),
			aispec.WithModel("memfit-qwen3.5-plus-free"),
		)

		if client.config.EnableThinkingField != "" {
			t.Fatalf("expected EnableThinkingField='', got '%s'", client.config.EnableThinkingField)
		}
		if client.config.EnableThinkingValue != nil {
			t.Fatalf("expected EnableThinkingValue=nil, got %v", client.config.EnableThinkingValue)
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

		if config.EnableThinkingField != "enable_thinking" {
			t.Fatalf("expected EnableThinkingField='enable_thinking', got '%s'", config.EnableThinkingField)
		}
		val, ok := config.EnableThinkingValue.(bool)
		if !ok {
			t.Fatalf("expected EnableThinkingValue to be bool, got %T", config.EnableThinkingValue)
		}
		if !val {
			t.Fatalf("expected EnableThinkingValue=true, got false")
		}
	})

	t.Run("tongyi no thinking option leaves field empty", func(t *testing.T) {
		config := aispec.NewDefaultAIConfig(
			aispec.WithType("tongyi"),
			aispec.WithAPIKey("test-key"),
			aispec.WithModel("qwen3.5-plus"),
		)

		if config.EnableThinkingField != "" {
			t.Fatalf("expected EnableThinkingField='', got '%s'", config.EnableThinkingField)
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

		if config.EnableThinkingField != "thinking" {
			t.Fatalf("expected EnableThinkingField='thinking', got '%s'", config.EnableThinkingField)
		}
		valMap, ok := config.EnableThinkingValue.(map[string]any)
		if !ok {
			t.Fatalf("expected EnableThinkingValue to be map, got %T", config.EnableThinkingValue)
		}
		if valMap["type"] != "enabled" {
			t.Fatalf("expected thinking type='enabled', got '%v'", valMap["type"])
		}
	})
}
