package aicommon

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestModelContextIdentitySurvivesGatewayAndResponseTee(t *testing.T) {
	for _, overflow := range []bool{false, true} {
		t.Run(fmt.Sprint(overflow), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if overflow {
					w.WriteHeader(http.StatusBadRequest)
					fmt.Fprint(w, `{"error":{"code":"context_length_exceeded","message":"maximum context length is 128000 tokens"}}`)
					return
				}
				w.Header().Set("Content-Type", "text/event-stream")
				fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"accepted\"}}]}\n\ndata: [DONE]\n\n")
			}))
			defer server.Close()
			model := &ypb.AIModelConfig{ModelName: "test-model", Provider: &ypb.ThirdPartyApplicationConfig{Type: "openai", APIKey: "test-only", BaseURL: server.URL + "/v1"}}
			key := aispec.ModelContextIdentity(model)
			callback, err := CreateCallbackFromConfig(model)
			require.NoError(t, err)
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			cfg := newTransactionTestConfig(ctx)
			cfg.retryMax = 1
			parsed := false
			err = CallAITransaction(cfg, "local test", func(req *AIRequest) (*AIResponse, error) { return callback(cfg, req) }, func(resp *AIResponse) error {
				parsed = true
				require.Equal(t, key, resp.GetModelContextKey())
				data, readErr := io.ReadAll(resp.GetOutputStreamReader("test", false, cfg.GetEmitter()))
				require.Contains(t, string(data), "accepted")
				return readErr
			})
			if overflow {
				require.True(t, IsPromptTooLong(err), "%v", err)
				ev, ok := PromptOverflowEvidenceFrom(err)
				require.True(t, ok)
				require.Equal(t, key, ev.ContextKey)
				require.False(t, parsed)
			} else {
				require.NoError(t, err)
				require.True(t, parsed)
			}
		})
	}
}
