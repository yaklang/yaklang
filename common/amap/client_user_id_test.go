package amap

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/aibalanceclient"
	"github.com/yaklang/yaklang/common/utils/lowhttp/poc"
)

func TestClientUserIDAmapProxyAndDirect(t *testing.T) {
	id := aibalanceclient.GetOrCreateClientUserID()
	require.NotEmpty(t, id)
	type received struct{ path, id, trace string }
	requests := make(chan received, 5)
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
			fmt.Fprint(w, `{"error":"memfit_totp_auth_failed"}`)
			return
		}
		fmt.Fprint(w, `{"status":"1","info":"OK"}`)
	}))
	defer server.Close()
	c := &Client{config: &Config{ApiKey: "fixture", BaseURL: server.URL + "/amap",
		isAIBalanceProxy: true, proxyServerBase: server.URL,
		lowhttpOptions: []poc.PocConfigOption{poc.WithSave(false),
			poc.WithReplaceHttpPacketHeader("x-user-id", "custom-override")},
	}}
	var result map[string]any
	require.NoError(t, c.doRequest(context.Background(), "/fixture", url.Values{}, &result))
	// A direct provider uses the same transport without automatic identity.
	c = &Client{config: &Config{ApiKey: "fixture", BaseURL: server.URL,
		lowhttpOptions: []poc.PocConfigOption{poc.WithSave(false)}}}
	require.NoError(t, c.doRequest(context.Background(), "/direct", url.Values{}, &result))
	close(requests)
	var paths []string
	for request := range requests {
		if request.path == "/direct" {
			require.Empty(t, request.id)
			require.Empty(t, request.trace)
		} else {
			require.Equal(t, id, request.id)
			if request.path != "/v1/memfit-totp-uuid" {
				require.Equal(t, id, request.trace)
			}
		}
		paths = append(paths, request.path)
	}
	require.Equal(t, []string{"/amap/fixture", "/v1/memfit-totp-uuid", "/amap/fixture", "/direct"}, paths)
}
