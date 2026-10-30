package embedding

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/aibalanceclient"
)

func TestClientUserIDEmbeddingScope(t *testing.T) {
	id := aibalanceclient.GetOrCreateClientUserID()
	require.NotEmpty(t, id)
	headers := make(chan []string, 3)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		headers <- r.Header.Values(aibalanceclient.ClientUserIDHeader)
		fmt.Fprint(w, `{"data":[{"embedding":[1,0]}]}`)
	}))
	defer server.Close()
	for _, provider := range []string{"aibalance", "openai"} {
		opts := []aispec.AIConfigOption{aispec.WithBaseURL(server.URL), aispec.WithType(provider)}
		if provider == "aibalance" {
			opts = append(opts, aispec.WithExtraHeaderString("x-user-id", "custom-override"))
		}
		_, err := NewOpenaiEmbeddingClient(opts...).Embedding("fixture")
		require.NoError(t, err)
		if provider == "aibalance" {
			require.Equal(t, []string{id}, <-headers)
		} else {
			require.Empty(t, <-headers)
		}
	}
	// The free embedder uses the production URL with the default provider type.
	c := NewOpenaiEmbeddingClient()
	c.httpClient.Transport = clientUserIDRoundTripper(func(r *http.Request) (*http.Response, error) {
		require.Equal(t, id, r.Header.Get(aibalanceclient.ClientUserIDHeader))
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("")), Header: make(http.Header)}, nil
	})
	rsp, err := c.doEmbeddingRequest("https://aibalance.yaklang.com/v1/embeddings", nil)
	require.NoError(t, err)
	rsp.Body.Close()
}

type clientUserIDRoundTripper func(*http.Request) (*http.Response, error)

func (f clientUserIDRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
