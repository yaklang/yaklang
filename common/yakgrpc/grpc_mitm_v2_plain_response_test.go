//go:build !yakit_exclude

package yakgrpc

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/utils/lowhttp/httpctx"
)

func makeGzipMITMResponsePacket(tb testing.TB, body []byte) []byte {
	tb.Helper()
	var compressed bytes.Buffer
	w := gzip.NewWriter(&compressed)
	_, err := w.Write(body)
	require.NoError(tb, err)
	require.NoError(tb, w.Close())
	header := []byte(fmt.Sprintf("HTTP/1.1 200 OK\r\nContent-Encoding: gzip\r\nContent-Length: %d\r\n\r\n", compressed.Len()))
	return append(header, compressed.Bytes()...)
}

func legacyDecodeAndCachePlainResponseBytes(req *http.Request, wire []byte) []byte {
	decoded := lowhttp.DeletePacketEncoding(wire)
	httpctx.SetPlainResponseBytes(req, decoded)
	return decoded
}

func TestCacheModifiedPlainResponseBytes(t *testing.T) {
	t.Run("unmodified response keeps decoded cache", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "http://example.com/", nil)
		httpctx.SetPlainResponseBytes(req, []byte("cached-response"))
		cached := httpctx.GetPlainResponseBytes(req)

		cacheModifiedPlainResponseBytes(req, cached)

		after := httpctx.GetPlainResponseBytes(req)
		require.Equal(t, cached, after)
		require.Same(t, &cached[0], &after[0], "unmodified response was cloned again")
	})

	t.Run("modified response keeps plain and hijacked storage independent", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "http://example.com/", nil)
		httpctx.SetResponseModified(req, "test")
		modified := []byte("modified-response")

		cacheModifiedPlainResponseBytes(req, modified)
		modified[0] = 'X'

		require.Equal(t, []byte("modified-response"), httpctx.GetPlainResponseBytes(req))
	})
}

func TestDecodeAndCachePlainResponseBytesPreservesOwnership(t *testing.T) {
	t.Run("unencoded wire remains independent from cache", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "http://example.com/", nil)
		wire := []byte("HTTP/1.1 200 OK\r\nContent-Length: 7\r\n\r\npayload")
		expected := bytes.Clone(wire)

		decoded := decodeAndCachePlainResponseBytes(req, wire)
		require.Same(t, &wire[0], &decoded[0])
		cached := httpctx.GetPlainResponseBytes(req)
		require.NotSame(t, &wire[0], &cached[0])
		wire[len(wire)-1] = 'X'
		require.Equal(t, expected, cached)
	})

	t.Run("decoded packet transfers directly to cache", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "http://example.com/", nil)
		body := bytes.Repeat([]byte("decoded-body"), 128)
		wire := makeGzipMITMResponsePacket(t, body)

		decoded := decodeAndCachePlainResponseBytes(req, wire)
		cached := httpctx.GetPlainResponseBytes(req)
		require.NotEmpty(t, decoded)
		require.Same(t, &decoded[0], &cached[0])
		_, decodedBody := lowhttp.SplitHTTPHeadersAndBodyFromPacketView(decoded)
		require.Equal(t, body, decodedBody)
	})
}

func TestMITMPlainResponseLoaderIsLazy(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "http://example.com/", nil)
	body := bytes.Repeat([]byte("lazy-response"), 64)
	wire := makeGzipMITMResponsePacket(t, body)
	httpctx.SetBareResponseBytesForce(req, wire)

	load := newMITMPlainResponseLoader(req)
	require.Empty(t, httpctx.GetPlainResponseBytes(req))

	plain := load()
	require.NotEmpty(t, plain)
	require.Same(t, &plain[0], &load()[0])
	_, plainBody := lowhttp.SplitHTTPHeadersAndBodyFromPacketView(plain)
	require.Equal(t, body, plainBody)
}

func TestMITMMirrorPlainResponseOwnership(t *testing.T) {
	body := bytes.Repeat([]byte("mirror-response"), 64)
	wire := makeGzipMITMResponsePacket(t, body)
	fixed, _, err := lowhttp.FixHTTPResponse(wire)
	require.NoError(t, err)
	require.NotEmpty(t, fixed)

	t.Run("synchronous path borrows fixed response", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "http://example.com/", nil)
		httpctx.SetBareResponseBytesForce(req, wire)
		httpctx.SetFixedResponseBytesOwned(req, bytes.Clone(fixed))
		storedFixed := httpctx.GetFixedResponseBytes(req)

		plain := getMITMMirrorPlainResponseBytes(req, false)
		require.Same(t, &storedFixed[0], &plain[0])
		require.Empty(t, httpctx.GetPlainResponseBytes(req))
	})

	t.Run("asynchronous hook path owns decoded response", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "http://example.com/", nil)
		httpctx.SetBareResponseBytesForce(req, wire)
		httpctx.SetFixedResponseBytesOwned(req, bytes.Clone(fixed))
		storedFixed := httpctx.GetFixedResponseBytes(req)

		plain := getMITMMirrorPlainResponseBytes(req, true)
		require.NotSame(t, &storedFixed[0], &plain[0])
		require.Same(t, &plain[0], &httpctx.GetPlainResponseBytes(req)[0])
		_, plainBody := lowhttp.SplitHTTPHeadersAndBodyFromPacketView(plain)
		require.Equal(t, body, plainBody)
	})

	t.Run("modified response never borrows fixed response", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "http://example.com/", nil)
		modified := []byte("HTTP/1.1 200 OK\r\nContent-Length: 8\r\n\r\nmodified")
		httpctx.SetFixedResponseBytesOwned(req, bytes.Clone(fixed))
		storedFixed := httpctx.GetFixedResponseBytes(req)
		httpctx.SetResponseModified(req, "test")
		httpctx.SetHijackedResponseBytes(req, modified)

		plain := getMITMMirrorPlainResponseBytes(req, false)
		require.NotSame(t, &storedFixed[0], &plain[0])
		require.Equal(t, modified, plain)
	})
}

func BenchmarkDecodeAndCachePlainResponseBytes256KGzip(b *testing.B) {
	body := bytes.Repeat([]byte("decoded-response-body"), 256*1024/len("decoded-response-body"))
	wire := makeGzipMITMResponsePacket(b, body)
	for _, tc := range []struct {
		name string
		fn   func(*http.Request, []byte) []byte
	}{
		{name: "legacy-decoded-clone", fn: legacyDecodeAndCachePlainResponseBytes},
		{name: "decoded-owned-transfer", fn: decodeAndCachePlainResponseBytes},
	} {
		b.Run(tc.name, func(b *testing.B) {
			req := httptest.NewRequest(http.MethodGet, "http://example.com/", nil)
			b.SetBytes(int64(len(body)))
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				decoded := tc.fn(req, wire)
				if len(decoded) < len(body) {
					b.Fatal("response body was not decoded")
				}
			}
		})
	}
}

func BenchmarkCacheModifiedPlainResponseBytes256K(b *testing.B) {
	response := bytes.Repeat([]byte("r"), 256*1024)

	b.Run("legacy-unconditional-clone", func(b *testing.B) {
		req := httptest.NewRequest(http.MethodGet, "http://example.com/", nil)
		b.SetBytes(int64(len(response)))
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			httpctx.SetPlainResponseBytes(req, response)
		}
	})

	b.Run("unmodified-cached", func(b *testing.B) {
		req := httptest.NewRequest(http.MethodGet, "http://example.com/", nil)
		httpctx.SetPlainResponseBytes(req, response)
		cached := httpctx.GetPlainResponseBytes(req)
		b.SetBytes(int64(len(response)))
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			cacheModifiedPlainResponseBytes(req, cached)
		}
	})

	b.Run("modified-independent-clone", func(b *testing.B) {
		req := httptest.NewRequest(http.MethodGet, "http://example.com/", nil)
		httpctx.SetResponseModified(req, "benchmark")
		b.SetBytes(int64(len(response)))
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			cacheModifiedPlainResponseBytes(req, response)
		}
	})
}
