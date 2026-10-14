package scannode

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/ai/openai"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

func TestAITrafficReusesSavePolicyAndKeepsSessionsIsolated(t *testing.T) {
	lowhttp.RegisterSaveHTTPFlowHandler(saveAITrafficHTTPFlow)
	t.Cleanup(yakit.RegisterLowHTTPSaveCallback)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"provider response"}}]}`)
			return
		}
		_, _ = io.WriteString(w, "application response")
	}))
	defer server.Close()
	first := trafficTestCollector(t, "http://unused.invalid")
	second := trafficTestCollector(t, "http://unused.invalid")
	second.binding.Ref.SessionID = "second"
	ctx := withAITrafficContext(context.Background(), first, "turn-a")
	host := strings.TrimPrefix(server.URL, "http://")
	runtimeID := "saved-flow-" + t.Name()
	send := func(ctx context.Context, save bool) {
		_, err := lowhttp.HTTP(lowhttp.WithContext(ctx), lowhttp.WithSaveHTTPFlow(save), lowhttp.WithSaveHTTPFlowSync(true), lowhttp.WithRuntimeId(runtimeID), lowhttp.WithPacketBytes([]byte("GET /application HTTP/1.1\r\nHost: "+host+"\r\nConnection: close\r\n\r\n")))
		require.NoError(t, err)
	}
	send(ctx, true)
	paths, err := first.pending()
	require.NoError(t, err)
	require.Len(t, paths, 1)
	require.Positive(t, yakit.CountHTTPFlowByRuntimeID(consts.GetGormProjectDatabase(), runtimeID))
	send(ctx, false)
	send(context.Background(), true)
	send(withAITrafficContext(context.Background(), second, "turn-b"), true)
	client := new(openai.GatewayClient)
	client.LoadOption(aispec.WithContext(ctx), aispec.WithBaseURL(server.URL), aispec.WithModel("test-model"))
	result, err := client.Chat("test prompt")
	require.NoError(t, err)
	require.Equal(t, "provider response", result)
	paths, err = first.pending()
	require.NoError(t, err)
	require.Len(t, paths, 1, "unsaved, unrelated and provider traffic must be excluded")
	paths, err = second.pending()
	require.NoError(t, err)
	require.Len(t, paths, 1)
	first.recordHTTPFlow(&lowhttp.LowhttpResponse{RawRequest: []byte("GET / HTTP/1.1\r\n\r\n")}, "turn-a")
	paths, err = first.pending()
	require.NoError(t, err)
	require.Len(t, paths, 1, "no-response flows follow the existing save policy")
}
