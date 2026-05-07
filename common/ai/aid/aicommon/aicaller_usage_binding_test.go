package aicommon

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aispec"
)

func TestAIChatToAICallbackType_BindsUsageIntoAIResponse(t *testing.T) {
	cfg := NewTestConfig(context.Background())

	expectedUsage := &aispec.ChatUsage{
		PromptTokens:     10,
		CompletionTokens: 6,
		TotalTokens:      16,
		PromptTokensDetails: &aispec.PromptTokensDetails{
			CachedTokens: 4,
		},
	}

	chatFn := func(prompt string, opts ...aispec.AIConfigOption) (string, error) {
		ac := aispec.NewDefaultAIConfig()
		for _, opt := range opts {
			opt(ac)
		}
		require.NotNil(t, ac.RawHTTPRequestResponseCallback)
		ac.RawHTTPRequestResponseCallback(
			[]byte("POST /chat HTTP/1.1\r\n\r\n"),
			[]byte("HTTP/1.1 200 OK\r\nContent-Type: application/json\r\n\r\n"),
			[]byte(`{"choices":[{"message":{"content":"ok"}}]}`),
			expectedUsage,
		)
		return "ok", nil
	}

	cb := AIChatToAICallbackType(chatFn)
	rsp, err := cb(cfg, NewAIRequest("ping"))
	require.NoError(t, err)
	require.NotNil(t, rsp)
	drainAIResponse(t, rsp)

	gotUsage := rsp.GetUsageInfo()
	require.NotNil(t, gotUsage)
	require.Equal(t, expectedUsage.PromptTokens, gotUsage.PromptTokens)
	require.Equal(t, expectedUsage.CompletionTokens, gotUsage.CompletionTokens)
	require.Equal(t, expectedUsage.TotalTokens, gotUsage.TotalTokens)
	require.NotNil(t, gotUsage.PromptTokensDetails)
	require.Equal(t, expectedUsage.PromptTokensDetails.CachedTokens, gotUsage.PromptTokensDetails.CachedTokens)
}
