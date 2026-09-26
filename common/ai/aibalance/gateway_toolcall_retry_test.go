package aibalance

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/aibalanceclient"
)

// A tool-call-only response has no assistant content. For memfit models that
// empty content must not trigger the TOTP retry and submit the tool call twice.
func TestMemfitToolCallOnlyResponseDoesNotRetry(t *testing.T) {
	oldSecret := aibalanceclient.GetCachedSecret()
	aibalanceclient.SetCachedSecret("JBSWY3DPEHPK3PXP")
	t.Cleanup(func() { aibalanceclient.SetCachedSecret(oldSecret) })

	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, `data: {"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_once","type":"function","function":{"name":"submit_tool_params","arguments":"{\"params\":{\"value\":\"ok\"}}"}}]},"finish_reason":null}]}`+"\n\n")
		_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	var ids []string
	var arguments strings.Builder
	client := &GatewayClient{}
	client.LoadOption(
		aispec.WithType("aibalance"),
		aispec.WithAPIKey("test-key"),
		aispec.WithModel("memfit-standard-free"),
		aispec.WithBaseURL(server.URL+"/v1/chat/completions"),
		aispec.WithStreamHandler(func(r io.Reader) { _, _ = io.Copy(io.Discard, r) }),
		aispec.WithReasonStreamHandler(func(r io.Reader) { _, _ = io.Copy(io.Discard, r) }),
		aispec.WithToolCallCallback(func(calls []*aispec.ToolCall) {
			for _, call := range calls {
				if call.ID != "" {
					ids = append(ids, call.ID)
				}
				arguments.WriteString(call.Function.Arguments)
			}
		}),
	)

	content, err := client.Chat("submit parameters")
	require.NoError(t, err)
	require.Empty(t, content)
	require.Equal(t, int32(1), requests.Load())
	require.Equal(t, []string{"call_once"}, ids)
	require.JSONEq(t, `{"params":{"value":"ok"}}`, arguments.String())
}

// Reasoning can be the only output in a valid response. With a separate
// reason handler ChatBase returns empty content, but the gateway must not
// mistake that for a failed TOTP request and send it again.
func TestMemfitReasonOnlyResponseDoesNotRetry(t *testing.T) {
	oldSecret := aibalanceclient.GetCachedSecret()
	aibalanceclient.SetCachedSecret("JBSWY3DPEHPK3PXP")
	t.Cleanup(func() { aibalanceclient.SetCachedSecret(oldSecret) })

	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"reasoning_content\":\"thinking\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
	}))
	defer server.Close()

	var reason strings.Builder
	client := &GatewayClient{}
	client.LoadOption(
		aispec.WithType("aibalance"),
		aispec.WithAPIKey("test-key"),
		aispec.WithModel("memfit-standard-free"),
		aispec.WithBaseURL(server.URL+"/v1/chat/completions"),
		aispec.WithStreamHandler(func(r io.Reader) { _, _ = io.Copy(io.Discard, r) }),
		aispec.WithReasonStreamHandler(func(r io.Reader) { _, _ = io.Copy(&reason, r) }),
	)

	content, err := client.Chat("think")
	require.NoError(t, err)
	require.Empty(t, content)
	require.Equal(t, "thinking", reason.String())
	require.Equal(t, int32(1), requests.Load())
}
