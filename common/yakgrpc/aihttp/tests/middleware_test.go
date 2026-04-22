package aihttp_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yakgrpc/aihttp"
)

func TestJWTAuthMiddleware(t *testing.T) {
	secret := "my-super-secret"
	gw := newTestGateway(t, aihttp.WithJWTAuth(secret))

	t.Run("no token", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/agent/setting", nil)
		w := performRequest(gw, req)
		require.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("valid token", func(t *testing.T) {
		token, err := aihttp.GenerateJWTToken(secret, 1)
		require.NoError(t, err)

		req := httptest.NewRequest("GET", "/agent/setting", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		w := performRequest(gw, req)
		require.Equal(t, http.StatusOK, w.Code)
	})
}

func TestCORSHeaders(t *testing.T) {
	gw := newTestGateway(t)

	t.Run("no origin keeps wildcard", func(t *testing.T) {
		req := httptest.NewRequest("OPTIONS", "/agent/setting", nil)
		w := performRequest(gw, req)

		require.Equal(t, "*", w.Header().Get("Access-Control-Allow-Origin"))
		require.Empty(t, w.Header().Get("Access-Control-Allow-Credentials"))
	})

	t.Run("origin supports credentialed browser requests", func(t *testing.T) {
		req := httptest.NewRequest("OPTIONS", "/agent/setting", nil)
		req.Header.Set("Origin", "http://localhost:4173")
		req.Header.Set("Access-Control-Request-Headers", "authorization, cache-control")
		w := performRequest(gw, req)

		require.Equal(t, "http://localhost:4173", w.Header().Get("Access-Control-Allow-Origin"))
		require.Equal(t, "true", w.Header().Get("Access-Control-Allow-Credentials"))
		require.Equal(t, "authorization, cache-control", w.Header().Get("Access-Control-Allow-Headers"))
		require.Contains(t, w.Header().Values("Vary"), "Origin")
	})
}
