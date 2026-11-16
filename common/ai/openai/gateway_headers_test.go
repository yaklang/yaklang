package openai

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestDownloadedProviderHeadersReachOpenAI(t *testing.T) {
	requests := make(chan http.Header, 4)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests <- r.Header.Clone()
		if r.URL.Path == "/v1/models" {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"data":[{"id":"enterprise-model","object":"model"}]}`)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"ready\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
	}))
	defer server.Close()
	for _, user := range []string{"alice.zh", "bob.zh"} {
		provider := &ypb.ThirdPartyApplicationConfig{
			Type: "openai", BaseURL: server.URL + "/v1", APIKey: "fixture-key",
			Headers: []*ypb.KVPair{{Key: "agile-user-id", Value: user}, {Key: "X-Static", Value: "keep"}},
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		client := new(GatewayClient)
		client.LoadOption(append(aispec.BuildOptionsFromConfig(&ypb.AIModelConfig{ModelName: "enterprise-model", Provider: provider, IsOnline: true}), aispec.WithContext(ctx))...)
		_, err := client.Chat("check downloaded headers")
		require.NoError(t, err)
		models, err := client.GetModelList()
		require.NoError(t, err)
		require.NotEmpty(t, models)
		for range 2 {
			select {
			case header := <-requests:
				require.Equal(t, []string{user}, header.Values("agile-user-id"))
				require.Equal(t, "keep", header.Get("X-Static"))
				require.Equal(t, "Bearer fixture-key", header.Get("Authorization"))
			case <-ctx.Done():
				t.Fatal("model request was not received")
			}
		}
		cancel()
		require.Equal(t, user, provider.Headers[0].Value)
	}
}
