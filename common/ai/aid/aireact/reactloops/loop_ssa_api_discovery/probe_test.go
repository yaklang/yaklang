package loop_ssa_api_discovery

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestProbeTarget_UnreachableAfterHTTPNormalize(t *testing.T) {
	ctx := context.Background()
	// host:port 会被规范为 http://，探活走 HTTP 而非纯 TCP
	res := ProbeTarget(ctx, "127.0.0.1:1")
	require.False(t, res.Reachable)
	require.Contains(t, []string{"http_head", "http_get"}, res.ProbeMethod)
}

func TestProbeTarget_HTTP(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	res := ProbeTarget(ctx, srv.URL)
	require.True(t, res.Reachable)
}
