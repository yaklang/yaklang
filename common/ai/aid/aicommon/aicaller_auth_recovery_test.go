package aicommon

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aibalance"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/aibalanceclient"
)

// Exercise the production async adapter and transaction boundary with a real
// gateway retry. Raw callbacks must reach AIResponse, including the first 401.
func TestAIChatTransactionTOTPRecovery(t *testing.T) {
	for _, succeeds := range []bool{true, false} {
		t.Run(fmt.Sprintf("success=%t", succeeds), func(t *testing.T) {
			previous := aibalanceclient.GetCachedSecret()
			aibalanceclient.SetCachedSecret("test-secret")
			t.Cleanup(func() { aibalanceclient.SetCachedSecret(previous) })
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/v1/memfit-totp-uuid" {
					fmt.Fprint(w, `{"uuid":"MEMFIT-AItest-secretMEMFIT-AI"}`)
					return
				}
				attempt := calls.Add(1)
				if attempt == 1 || !succeeds {
					w.WriteHeader(http.StatusUnauthorized)
					fmt.Fprint(w, `{"error":{"type":"memfit_totp_auth_failed","message":"Memfit TOTP authentication failed"}}`)
					return
				}
				w.Header().Set("Content-Type", "text/event-stream")
				fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"recovered\"}}]}\n\ndata: [DONE]\n\n")
			}))
			defer server.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			cfg := newTransactionTestConfig(ctx)
			cfg.retryMax = 1
			callback := AIChatToAICallbackType(func(prompt string, opts ...aispec.AIConfigOption) (string, error) {
				gateway := &aibalance.GatewayClient{}
				opts = append(opts, aispec.WithBaseURL(server.URL+"/v1/chat/completions"), aispec.WithModel("memfit-test"), aispec.WithAPIKey("test-key"))
				gateway.LoadOption(opts...)
				return gateway.Chat(prompt)
			})
			parsed := false
			err := CallAITransaction(cfg, "local fixture", func(req *AIRequest) (*AIResponse, error) {
				return callback(cfg, req)
			}, func(resp *AIResponse) error {
				parsed = true
				require.Equal(t, http.StatusOK, resp.GetHTTPStatusCode())
				data, readErr := io.ReadAll(resp.GetOutputStreamReader("test", false, cfg.GetEmitter()))
				require.NoError(t, readErr)
				require.Contains(t, string(data), "recovered")
				return nil
			})
			if succeeds {
				require.NoError(t, err)
				require.True(t, parsed)
			} else {
				require.Error(t, err)
				require.False(t, parsed)
			}
			require.EqualValues(t, 2, calls.Load())
		})
	}
}
