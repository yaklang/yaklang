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

func TestContextBytesStorage(t *testing.T) {
	t.Run("request and response bytes are stored as cloned byte slices", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "http://example.com/", nil)
		requestRaw := []byte("GET / HTTP/1.1\r\nHost: example.com\r\n\r\n")
		responseRaw := []byte("HTTP/1.1 200 OK\r\nContent-Length: 2\r\n\r\nok")

		SetBareRequestBytes(req, requestRaw)
		SetBareResponseBytesForce(req, responseRaw)

		requestRaw[0] = 'X'
		responseRaw[0] = 'X'

		require.Equal(t, "GET / HTTP/1.1\r\nHost: example.com\r\n\r\n", string(GetBareRequestBytes(req)))
		require.Equal(t, "HTTP/1.1 200 OK\r\nContent-Length: 2\r\n\r\nok", string(GetBareResponseBytes(req)))
		require.Equal(t, "GET / HTTP/1.1\r\nHost: example.com\r\n\r\n", GetContextStringInfoFromRequest(req, REQUEST_CONTEXT_KEY_RequestBareBytes))
		require.Equal(t, "HTTP/1.1 200 OK\r\nContent-Length: 2\r\n\r\nok", GetContextStringInfoFromRequest(req, REQUEST_CONTEXT_KEY_ResponseBareBytes))
	})

	t.Run("legacy string-backed values remain readable as bytes", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "http://example.com/", nil)
		SetContextValueInfoFromRequest(req, REQUEST_CONTEXT_KEY_RequestPlainBytes, "legacy-request")
		SetContextValueInfoFromRequest(req, REQUEST_CONTEXT_KEY_ResponsePlainBytes, "legacy-response")
		SetContextValueInfoFromRequest(req, REQUEST_CONTEXT_KEY_MockResponseBytes, "legacy-mock")

		require.Equal(t, []byte("legacy-request"), GetPlainRequestBytes(req))
		require.Equal(t, []byte("legacy-response"), GetPlainResponseBytes(req))
		require.Equal(t, []byte("legacy-mock"), GetMockResponseBytes(req))
	})
}
