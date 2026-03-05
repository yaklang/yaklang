package aihttp_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yakgrpc/aihttp"
)

func newTestGateway(t *testing.T, opts ...aihttp.GatewayOption) *aihttp.AIAgentHTTPGateway {
	t.Helper()

	defaultOpts := []aihttp.GatewayOption{
		aihttp.WithRoutePrefix("/agent"),
		aihttp.WithHost("127.0.0.1"),
		aihttp.WithPort(0),
	}
	defaultOpts = append(defaultOpts, opts...)

	gw, err := aihttp.NewAIAgentHTTPGateway(defaultOpts...)
	require.NoError(t, err, "create gateway")
	t.Cleanup(func() {
		gw.Shutdown()
	})
	return gw
}

func performRequest(gw *aihttp.AIAgentHTTPGateway, req *http.Request) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	gw.ServeHTTP(w, req)
	return w
}
