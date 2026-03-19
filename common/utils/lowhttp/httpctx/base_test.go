package httpctx

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGetRequestHTTPSWithFallback(t *testing.T) {
	t.Run("nil request returns false", func(t *testing.T) {
		require.False(t, GetRequestHTTPSWithFallback(nil))
	})

	t.Run("plain request returns false", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "http://example.com/", nil)
		require.False(t, GetRequestHTTPSWithFallback(req))
	})

	t.Run("request with TLS returns true", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "https://example.com/", nil)
		req.TLS = &tls.ConnectionState{}
		require.True(t, GetRequestHTTPSWithFallback(req))
	})

	t.Run("SetRequestHTTPS returns true", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "http://example.com/", nil)
		SetRequestHTTPS(req, true)
		require.True(t, GetRequestHTTPSWithFallback(req))
	})

	t.Run("ConnectToHTTPS context returns true", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "http://example.com/", nil)
		SetContextValueInfoFromRequest(req, REQUEST_CONTEXT_ConnectToHTTPS, true)
		require.True(t, GetRequestHTTPSWithFallback(req))
	})

	t.Run("SetRequestHTTPS false without other signals returns false", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "http://example.com/", nil)
		SetRequestHTTPS(req, false)
		require.False(t, GetRequestHTTPSWithFallback(req))
	})

	t.Run("ConnectToHTTPS false without other signals returns false", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "http://example.com/", nil)
		SetContextValueInfoFromRequest(req, REQUEST_CONTEXT_ConnectToHTTPS, false)
		require.False(t, GetRequestHTTPSWithFallback(req))
	})

	t.Run("TLS takes priority even if httpctx says false", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "http://example.com/", nil)
		req.TLS = &tls.ConnectionState{}
		SetRequestHTTPS(req, false)
		require.True(t, GetRequestHTTPSWithFallback(req))
	})
}
