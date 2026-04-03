package aispec

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestBuildOptionsFromConfig_AppliesAPIType(t *testing.T) {
	config := &ypb.AIModelConfig{
		ModelName: "gpt-4.1-mini",
		Provider: &ypb.ThirdPartyApplicationConfig{
			Type:    "openai",
			APIKey:  "test-key",
			Domain:  "api.openai.com",
			APIType: "responses",
			Headers: []*ypb.KVPair{
				{Key: "X-Test-Header", Value: "test-value"},
			},
		},
	}

	resolved := NewDefaultAIConfig(BuildOptionsFromConfig(config)...)
	assert.Equal(t, "responses", resolved.APIType)
	assert.Equal(t, "gpt-4.1-mini", resolved.Model)
	assert.Equal(t, "openai", resolved.Type)
	assert.Equal(t, "api.openai.com", resolved.Domain)
	assert.Len(t, resolved.Headers, 1)
	assert.Equal(t, "X-Test-Header", resolved.Headers[0].GetKey())
	assert.Equal(t, "test-value", resolved.Headers[0].GetValue())
}

func TestGetBaseURLFromConfig_UsesResponsesAPIType(t *testing.T) {
	config := NewDefaultAIConfig(
		WithType("openai"),
		WithAPIType("responses"),
	)

	assert.Equal(t,
		"https://api.openai.com/v1/responses",
		GetBaseURLFromConfig(config, "https://api.openai.com", "/v1/chat/completions"),
	)

	config = NewDefaultAIConfig(
		WithType("openai"),
		WithAPIType("responses"),
		WithBaseURL("https://proxy.example.com/v1"),
	)

	assert.Equal(t,
		"https://proxy.example.com/v1/responses",
		GetBaseURLFromConfig(config, "https://api.openai.com", "/v1/chat/completions"),
	)
}

func TestGetBaseURLFromConfig_UsesExplicitEndpointWhenEnabled(t *testing.T) {
	config := NewDefaultAIConfig(
		WithType("openai"),
		WithBaseURL("https://proxy.example.com/v1"),
		WithEndpoint("https://proxy.example.com/custom/chat/completions"),
		WithEnableEndpoint(true),
	)

	assert.Equal(t,
		"https://proxy.example.com/custom/chat/completions",
		GetBaseURLFromConfig(config, "https://api.openai.com", "/v1/chat/completions"),
	)
}

func TestGetBaseURLFromConfig_DoesNotRewriteOllamaAPIType(t *testing.T) {
	config := NewDefaultAIConfig(
		WithType("ollama"),
		WithAPIType("responses"),
		WithNoHttps(true),
	)

	assert.Equal(t,
		"http://127.0.0.1:11434/v1/chat/completions",
		GetBaseURLFromConfig(config, "http://127.0.0.1:11434", "/v1/chat/completions"),
	)
}

func TestGetBaseURLFromConfig_PreservesExplicitChatCompletionsBaseURL(t *testing.T) {
	t.Run("responses api type keeps explicit chat completions endpoint", func(t *testing.T) {
		config := NewDefaultAIConfig(
			WithType("openai"),
			WithAPIType("responses"),
			WithBaseURL("https://proxy.example.com/v1/chat/completions"),
		)

		assert.Equal(t,
			"https://proxy.example.com/v1/chat/completions",
			GetBaseURLFromConfig(config, "https://api.openai.com", "/v1/chat/completions"),
		)
	})

	t.Run("openai mode does not append default suffix to explicit chat completions endpoint", func(t *testing.T) {
		config := NewDefaultAIConfig(
			WithBaseURL("https://proxy.example.com/custom/chat/completions"),
		)

		assert.Equal(t,
			"https://proxy.example.com/custom/chat/completions",
			GetBaseURLFromConfigEx(config, "https://api.openai.com", "/v1/responses", true),
		)
	})
}
