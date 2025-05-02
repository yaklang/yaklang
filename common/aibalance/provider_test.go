package aibalance

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestProvider_GetAIClient(t *testing.T) {
	tests := []struct {
		name        string
		provider    *Provider
		expectError bool
	}{
		{
			name: "OpenAI provider",
			provider: &Provider{
				ModelName:   "gpt-3.5-turbo",
				TypeName:    "openai",
				DomainOrURL: "https://api.openai.com",
				APIKey:      "test-key",
			},
			expectError: false,
		},
		{
			name: "ChatGLM provider",
			provider: &Provider{
				ModelName:   "glm-4-flash",
				TypeName:    "chatglm",
				DomainOrURL: "https://open.bigmodel.cn/api/paas/v4/chat/completions",
				APIKey:      "test-key",
			},
			expectError: false,
		},
		{
			name: "Moonshot provider",
			provider: &Provider{
				ModelName:   "moonshot-v1-8k",
				TypeName:    "moonshot",
				DomainOrURL: "https://api.moonshot.cn",
				APIKey:      "test-key",
			},
			expectError: false,
		},
		{
			name: "Tongyi provider",
			provider: &Provider{
				ModelName:   "qwen-turbo",
				TypeName:    "tongyi",
				DomainOrURL: "https://dashscope.aliyuncs.com",
				APIKey:      "test-key",
			},
			expectError: false,
		},
		{
			name: "Invalid provider type",
			provider: &Provider{
				ModelName:   "test-model",
				TypeName:    "invalid-type",
				DomainOrURL: "https://test.com",
				APIKey:      "test-key",
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := tt.provider.GetAIClient(nil, nil)
			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, client)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, client)
			}
		})
	}
}

func TestProvider_GetAIClient_Performance(t *testing.T) {
	providers := []*Provider{
		{
			ModelName:   "gpt-3.5-turbo",
			TypeName:    "openai",
			DomainOrURL: "https://api.openai.com",
			APIKey:      "test-key",
		},
		{
			ModelName:   "glm-4-flash",
			TypeName:    "chatglm",
			DomainOrURL: "https://open.bigmodel.cn/api/paas/v4/chat/completions",
			APIKey:      "test-key",
		},
		{
			ModelName:   "moonshot-v1-8k",
			TypeName:    "moonshot",
			DomainOrURL: "https://api.moonshot.cn",
			APIKey:      "test-key",
		},
	}

	for _, provider := range providers {
		t.Run(provider.TypeName, func(t *testing.T) {
			start := time.Now()
			_, err := provider.GetAIClient(nil, nil)
			assert.NoError(t, err)
			duration := time.Since(start)

			// 确保性能在2秒内
			assert.Less(t, duration, 2*time.Second, "GetAIClient took too long for %s", provider.TypeName)
		})
	}
}
