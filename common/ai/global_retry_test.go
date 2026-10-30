package ai

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestGlobalRetryStandardModelRoutes(t *testing.T) {
	old := consts.GetTieredAIConfig()
	t.Cleanup(func() { consts.SetTieredAIConfig(old) })
	for _, route := range []string{"explicit", "chatter", "main", "auxiliary"} {
		t.Run(route, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if calls.Add(1) == 1 {
					w.Header().Set("Retry-After", "0")
					w.WriteHeader(429)
					fmt.Fprint(w, `{"error":"customer-specific limit"}`)
					return
				}
				w.Header().Set("Content-Type", "text/event-stream")
				fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"reasoning_content\":\"checking\"}}]}\n\n")
				fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"ready\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
			}))
			defer server.Close()
			model := &ypb.AIModelConfig{ModelName: "fixture", Provider: &ypb.ThirdPartyApplicationConfig{Type: "openai", BaseURL: server.URL, APIKey: "fixture"}}
			consts.SetTieredAIConfig(&consts.TieredAIConfig{RetryForever: true, IntelligentConfigs: []*ypb.AIModelConfig{model}, LightweightConfigs: []*ypb.AIModelConfig{model}})
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			var output, reason strings.Builder
			var headers []string
			opts := []aispec.AIConfigOption{aispec.WithContext(ctx), aispec.WithTimeout(5),
				aispec.WithStreamHandler(func(r io.Reader) { io.Copy(&output, r) }),
				aispec.WithReasonStreamHandler(func(r io.Reader) { io.Copy(&reason, r) }),
				aispec.WithRawHTTPResponseHeaderCallback(func(h []byte) { headers = append(headers, string(h)) }),
			}
			var err error
			switch route {
			case "explicit":
				_, err = Chat("fixture", append(aispec.BuildOptionsFromConfig(model), opts...)...)
			case "chatter":
				var chat aispec.GeneralChatter
				chat, err = LoadChater("openai", aispec.BuildOptionsFromConfig(model)...)
				require.NoError(t, err)
				_, err = chat("fixture", opts...)
			case "main":
				_, err = TieredChatWithTier(TierIntelligent, "fixture", opts...)
			case "auxiliary":
				_, err = TieredChatWithTier(TierLightweight, "fixture", opts...)
			}
			require.NoError(t, err)
			require.EqualValues(t, 2, calls.Load())
			require.Equal(t, "ready", output.String())
			require.Equal(t, "checking", reason.String())
			require.NotEmpty(t, headers)
			for _, header := range headers {
				require.NotContains(t, header, "429")
			}
		})
	}
}
