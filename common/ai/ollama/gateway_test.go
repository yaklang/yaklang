package ollama

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aispec"
)

func TestOllamaLoadOption_LocalDefaultHTTP(t *testing.T) {
	g := &GatewayClient{}
	g.LoadOption(aispec.WithModel("qwen"))

	assert.Contains(t, g.targetUrl, "http://")
	assert.Contains(t, g.targetUrl, "127.0.0.1:11434")
	assert.Contains(t, g.targetUrl, "/v1/chat/completions")
}

func TestOllamaLoadOption_CloudURL(t *testing.T) {
	g := &GatewayClient{}
	g.LoadOption(
		aispec.WithModel("kimi-k2.7-code:cloud"),
		aispec.WithDomain("ollama.com"),
		aispec.WithAPIKey("test-key"),
	)

	assert.True(t, strings.HasPrefix(g.targetUrl, "https://"), "cloud URL should use HTTPS, got: %s", g.targetUrl)
	assert.Contains(t, g.targetUrl, "ollama.com")
	assert.Contains(t, g.targetUrl, "/v1/chat/completions")
}

func TestOllamaLoadOption_ExplicitBaseURL(t *testing.T) {
	g := &GatewayClient{}
	g.LoadOption(
		aispec.WithModel("kimi-k2.7-code:cloud"),
		aispec.WithBaseURL("https://ollama.com/v1/chat/completions"),
		aispec.WithAPIKey("test-key"),
	)

	assert.Equal(t, "https://ollama.com/v1/chat/completions", g.targetUrl)
}

func TestOllamaCheckValid_CloudWithAPIKey(t *testing.T) {
	g := &GatewayClient{}
	g.LoadOption(
		aispec.WithModel("kimi-k2.7-code:cloud"),
		aispec.WithDomain("ollama.com"),
		aispec.WithAPIKey("test-api-key"),
	)

	err := g.CheckValid()
	assert.NoError(t, err)
}

func TestOllamaCheckValid_CloudWithoutAPIKey(t *testing.T) {
	g := &GatewayClient{}
	g.LoadOption(
		aispec.WithModel("kimi-k2.7-code:cloud"),
		aispec.WithDomain("ollama.com"),
	)

	err := g.CheckValid()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "API key")
}

func TestOllamaBuildHTTPOptions_WithAPIKey(t *testing.T) {
	g := &GatewayClient{}
	g.LoadOption(
		aispec.WithModel("kimi-k2.7-code:cloud"),
		aispec.WithDomain("ollama.com"),
		aispec.WithAPIKey("my-secret-key"),
	)

	assert.Equal(t, "my-secret-key", g.config.APIKey)

	opts, err := g.BuildHTTPOptions()
	require.NoError(t, err)
	assert.NotEmpty(t, opts)
}

func TestOllamaBuildHTTPOptions_WithoutAPIKey(t *testing.T) {
	g := &GatewayClient{}
	g.LoadOption(aispec.WithModel("qwen"))

	assert.Empty(t, g.config.APIKey)

	opts, err := g.BuildHTTPOptions()
	require.NoError(t, err)
	assert.NotEmpty(t, opts)
}

func TestOllamaIsCloudTarget(t *testing.T) {
	tests := []struct {
		name     string
		url      string
		expected bool
	}{
		{"cloud domain", "https://ollama.com/v1/chat/completions", true},
		{"local default", "http://127.0.0.1:11434/v1/chat/completions", false},
		{"local custom port", "http://localhost:11434/v1/chat/completions", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := &GatewayClient{targetUrl: tt.url}
			assert.Equal(t, tt.expected, g.isCloudTarget())
		})
	}
}

func TestOllamaLoadOption_NativeAPI(t *testing.T) {
	g := &GatewayClient{}
	g.LoadOption(aispec.WithModel("qwen_native_api"))

	assert.Contains(t, g.targetUrl, "/api/chat")
	assert.NotContains(t, g.targetUrl, "/v1/chat/completions")
	assert.Equal(t, "qwen", g.config.Model)
	assert.False(t, g.useOpenAIFormat)
}
