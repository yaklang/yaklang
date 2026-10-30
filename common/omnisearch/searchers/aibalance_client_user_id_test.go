package searchers

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/aibalanceclient"
)

func TestClientUserIDSearchRetry(t *testing.T) {
	previous := aibalanceclient.GetCachedSecret()
	aibalanceclient.SetCachedSecret("test-secret")
	t.Cleanup(func() { aibalanceclient.SetCachedSecret(previous) })
	id := aibalanceclient.GetOrCreateClientUserID()
	require.NotEmpty(t, id)
	type received struct{ path, id, trace string }
	requests := make(chan received, 4)
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests <- received{r.URL.Path, r.Header.Get(aibalanceclient.ClientUserIDHeader), r.Header.Get("Trace-ID")}
		if r.URL.Path == "/v1/memfit-totp-uuid" {
			fmt.Fprint(w, `{"uuid":"MEMFIT-AItest-secretMEMFIT-AI"}`)
			return
		}
		attempts++
		if attempts == 1 {
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprint(w, `{"error":{"type":"memfit_totp_auth_failed","message":"Memfit TOTP authentication failed"}}`)
			return
		}
		fmt.Fprint(w, `{"results":[],"total":0,"searcher_type":"fixture"}`)
	}))
	defer server.Close()
	c := NewAiBalanceSearchClient(&AiBalanceSearchConfig{BaseURL: server.URL, Timeout: 5})
	_, err := c.Search("fixture", 1, 10)
	require.NoError(t, err)
	close(requests)
	var paths []string
	for request := range requests {
		require.Equal(t, id, request.id)
		if request.path == "/v1/web-search" {
			require.Equal(t, id, request.trace)
		}
		paths = append(paths, request.path)
	}
	require.Equal(t, []string{"/v1/web-search", "/v1/memfit-totp-uuid", "/v1/web-search"}, paths)
}
