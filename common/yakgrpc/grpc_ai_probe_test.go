package yakgrpc

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// makeHTTPResponseHeader builds a raw HTTP response header bytes for the
// given status code, so tests can simulate upstream responses.
func makeHTTPResponseHeader(statusCode int) []byte {
	return []byte(fmt.Sprintf("HTTP/1.1 %d %s\r\n\r\n", statusCode, http.StatusText(statusCode)))
}

func TestIsLikelyErrorResponse(t *testing.T) {
	tests := []struct {
		name   string
		result string
		want   bool
	}{
		{"empty", "", true},
		{"normal text", "Hello, how can I help?", false},
		{"normal JSON", `{"choices":[{"message":{"content":"hi"}}]}`, false},
		{"error JSON", `{"error":{"message":"unsupported effort level"}}`, true},
		{"error JSON with type", `{"error":{"message":"invalid","type":"invalid_request_error"}}`, true},
		{"error without message key", `{"error":"bad request"}`, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, isLikelyErrorResponse(tt.result))
		})
	}
}

func TestProbeReasoningEffort_BothSupported(t *testing.T) {
	t.Run("xhigh and max both supported", func(t *testing.T) {
		chat := func(msg string, opts ...aispec.AIConfigOption) (string, error) {
			cfg := aispec.NewDefaultAIConfig(opts...)
			if cfg.RawHTTPRequestResponseCallback != nil {
				cfg.RawHTTPRequestResponseCallback(
					[]byte("POST /v1/chat/completions HTTP/1.1\r\n\r\n"),
					makeHTTPResponseHeader(200),
					[]byte(`{"choices":[{"message":{"content":"hi"}}]}`),
					nil,
				)
			}
			return "hi", nil
		}

		resp, err := probeReasoningEffort(context.Background(), &ypb.ProbeReasoningEffortRequest{
			Config: &ypb.ThirdPartyApplicationConfig{
				Type:   "openai",
				APIKey: "test-key",
				Domain: "api.openai.com",
			},
			Model: "o3-mini",
		}, chat)

		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.True(t, resp.GetXhighSupported())
		assert.True(t, resp.GetMaxSupported())
		assert.Empty(t, resp.GetXhighErrorMessage())
		assert.Empty(t, resp.GetMaxErrorMessage())
	})
}

func TestProbeReasoningEffort_NeitherSupported(t *testing.T) {
	t.Run("neither xhigh nor max supported", func(t *testing.T) {
		callCount := 0
		chat := func(msg string, opts ...aispec.AIConfigOption) (string, error) {
			callCount++
			cfg := aispec.NewDefaultAIConfig(opts...)
			if cfg.RawHTTPRequestResponseCallback != nil {
				cfg.RawHTTPRequestResponseCallback(
					[]byte("POST /v1/chat/completions HTTP/1.1\r\n\r\n"),
					makeHTTPResponseHeader(400),
					[]byte(`{"error":{"message":"unsupported reasoning effort"}}`),
					nil,
				)
			}
			return `{"error":{"message":"unsupported reasoning effort"}}`, nil
		}

		resp, err := probeReasoningEffort(context.Background(), &ypb.ProbeReasoningEffortRequest{
			Config: &ypb.ThirdPartyApplicationConfig{
				Type:   "openai",
				APIKey: "test-key",
				Domain: "api.openai.com",
			},
			Model: "gpt-4o",
		}, chat)

		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.False(t, resp.GetXhighSupported())
		assert.False(t, resp.GetMaxSupported())
		assert.Contains(t, resp.GetXhighErrorMessage(), "unsupported")
		assert.Contains(t, resp.GetMaxErrorMessage(), "unsupported")
		assert.Equal(t, 2, callCount)
	})
}

func TestProbeReasoningEffort_OnlyXhighSupported(t *testing.T) {
	t.Run("only xhigh supported, max returns 400", func(t *testing.T) {
		chat := func(msg string, opts ...aispec.AIConfigOption) (string, error) {
			cfg := aispec.NewDefaultAIConfig(opts...)
			effort := cfg.ThinkingLevel
			if cfg.RawHTTPRequestResponseCallback != nil {
				if effort == "xhigh" {
					cfg.RawHTTPRequestResponseCallback(
						nil,
						makeHTTPResponseHeader(200),
						[]byte(`{"choices":[{"message":{"content":"hi"}}]}`),
						nil,
					)
				} else {
					cfg.RawHTTPRequestResponseCallback(
						nil,
						makeHTTPResponseHeader(400),
						[]byte(`{"error":{"message":"max not supported"}}`),
						nil,
					)
				}
			}
			if effort == "xhigh" {
				return "hi", nil
			}
			return `{"error":{"message":"max not supported"}}`, nil
		}

		resp, err := probeReasoningEffort(context.Background(), &ypb.ProbeReasoningEffortRequest{
			Config: &ypb.ThirdPartyApplicationConfig{
				Type:   "openai",
				APIKey: "test-key",
				Domain: "api.openai.com",
			},
			Model: "o3-mini",
		}, chat)

		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.True(t, resp.GetXhighSupported())
		assert.False(t, resp.GetMaxSupported())
		assert.Empty(t, resp.GetXhighErrorMessage())
		assert.Contains(t, resp.GetMaxErrorMessage(), "max not supported")
	})
}

func TestProbeReasoningEffort_NetworkError(t *testing.T) {
	t.Run("network error returns unsupported", func(t *testing.T) {
		chat := func(msg string, opts ...aispec.AIConfigOption) (string, error) {
			return "", errors.New("connection refused")
		}

		resp, err := probeReasoningEffort(context.Background(), &ypb.ProbeReasoningEffortRequest{
			Config: &ypb.ThirdPartyApplicationConfig{
				Type:   "openai",
				APIKey: "test-key",
				Domain: "api.openai.com",
			},
			Model: "o3-mini",
		}, chat)

		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.False(t, resp.GetXhighSupported())
		assert.False(t, resp.GetMaxSupported())
	})
}

func TestProbeReasoningEffort_NilConfig(t *testing.T) {
	server := &Server{}

	_, err := server.ProbeReasoningEffort(context.Background(), &ypb.ProbeReasoningEffortRequest{
		Config: nil,
	})
	assert.Error(t, err)
}

func TestProbeReasoningEffort_EmptyType(t *testing.T) {
	server := &Server{}

	_, err := server.ProbeReasoningEffort(context.Background(), &ypb.ProbeReasoningEffortRequest{
		Config: &ypb.ThirdPartyApplicationConfig{
			APIKey: "test-key",
		},
		Model: "o3-mini",
	})
	assert.Error(t, err)
}
