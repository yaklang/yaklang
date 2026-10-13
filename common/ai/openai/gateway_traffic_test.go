package openai

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
)

func TestGatewayExcludesSessionHTTPAttemptObserver(t *testing.T) {
	type contextKey struct{}
	ctx, cancel := context.WithTimeout(context.WithValue(context.Background(), contextKey{}, "session"), 5*time.Second)
	defer cancel()
	attempts := make(chan lowhttp.HTTPAttempt, 4)
	ctx = lowhttp.WithHTTPAttemptObserver(ctx, func(lowhttp.HTTPAttempt) func(lowhttp.HTTPAttempt) {
		return func(end lowhttp.HTTPAttempt) { attempts <- end }
	})
	providerCalls := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			providerCalls <- struct{}{}
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"provider response"}}]}`)
			return
		}
		_, _ = io.WriteString(w, "application response")
	}))
	defer server.Close()
	host := strings.TrimPrefix(server.URL, "http://")
	_, err := lowhttp.HTTP(lowhttp.WithContext(ctx), lowhttp.WithSaveHTTPFlow(false), lowhttp.WithPacketBytes([]byte("GET /application HTTP/1.1\r\nHost: "+host+"\r\nConnection: close\r\n\r\n")))
	require.NoError(t, err)
	require.Len(t, attempts, 1)

	client := new(GatewayClient)
	client.LoadOption(aispec.WithContext(ctx), aispec.WithBaseURL(server.URL), aispec.WithModel("test-model"))
	result, err := client.Chat("test prompt")
	require.NoError(t, err)
	require.Equal(t, "provider response", result)
	require.Len(t, providerCalls, 1)
	require.Len(t, attempts, 1, "model-provider requests must not enter session evidence")
	require.Equal(t, "session", client.GetConfig().Context.Value(contextKey{}))
	deadline, _ := ctx.Deadline()
	providerDeadline, ok := client.GetConfig().Context.Deadline()
	require.True(t, ok)
	require.Equal(t, deadline, providerDeadline)
	cancel()
	require.ErrorIs(t, client.GetConfig().Context.Err(), context.Canceled)
}
