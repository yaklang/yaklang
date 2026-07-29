//go:build !yakit_exclude

package yakgrpc

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/utils/lowhttp/httpctx"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

func makePlainRequestPacket(bodySize int) []byte {
	header := fmt.Sprintf("POST /upload HTTP/1.1\r\nHost: example.com\r\nContent-Length: %d\r\n\r\n", bodySize)
	packet := make([]byte, len(header)+bodySize)
	copy(packet, header)
	copy(packet[len(header):], bytes.Repeat([]byte("r"), bodySize))
	return packet
}

func legacyCachePlainRequestBytesIfStorable(req *http.Request, decoded []byte) {
	_, body := lowhttp.SplitHTTPHeadersAndBodyFromPacket(decoded)
	if len(body) <= yakit.MaxHTTPFlowRequestBodyInDBBytes {
		httpctx.SetPlainRequestBytes(req, decoded)
	}
}

func cloneDecodedPlainRequestBytesIfStorable(req *http.Request, wire []byte) []byte {
	decoded, independentlyOwned := lowhttp.DeletePacketEncodingWithOwnership(wire)
	_, body := lowhttp.SplitHTTPHeadersAndBodyFromPacketView(decoded)
	if len(body) <= yakit.MaxHTTPFlowRequestBodyInDBBytes {
		if independentlyOwned {
			httpctx.SetPlainRequestBytesOwned(req, decoded)
		} else {
			httpctx.SetPlainRequestBytes(req, decoded)
		}
	}
	return decoded
}

func TestCachePlainRequestBytesIfStorable(t *testing.T) {
	t.Run("exact limit is cached with independent ownership", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "http://example.com/upload", nil)
		packet := makePlainRequestPacket(yakit.MaxHTTPFlowRequestBodyInDBBytes)
		expected := bytes.Clone(packet)

		cachePlainRequestBytesIfStorable(req, packet)
		cached := httpctx.GetPlainRequestBytes(req)
		require.Equal(t, expected, cached)
		require.NotSame(t, &packet[0], &cached[0], "cache must not alias the decoded packet")

		packet[len(packet)-1] = 'X'
		require.Equal(t, expected, httpctx.GetPlainRequestBytes(req))
	})

	t.Run("body over limit is not cached", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "http://example.com/upload", nil)
		packet := makePlainRequestPacket(yakit.MaxHTTPFlowRequestBodyInDBBytes + 1)

		cachePlainRequestBytesIfStorable(req, packet)

		require.Empty(t, httpctx.GetPlainRequestBytes(req))
	})
}

func TestDecodeAndCachePlainRequestBytesIfStorablePreservesOwnership(t *testing.T) {
	t.Run("unencoded wire remains independent from cache", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "http://example.com/upload", nil)
		wire := makePlainRequestPacket(1024)
		expected := bytes.Clone(wire)

		decoded := decodeAndCachePlainRequestBytesIfStorable(req, wire)
		require.Same(t, &wire[0], &decoded[0])
		cached := httpctx.GetPlainRequestBytes(req)
		require.NotSame(t, &wire[0], &cached[0])
		wire[len(wire)-1] = 'X'
		require.Equal(t, expected, cached)
	})

	t.Run("unencoded context-owned bare packet is borrowed read-only", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "http://example.com/upload", nil)
		external := makePlainRequestPacket(1024)
		expected := bytes.Clone(external)
		httpctx.SetBareRequestBytes(req, external)
		bare := httpctx.GetBareRequestBytes(req)

		decoded := decodeAndCachePlainRequestBytesIfStorable(req, bare)
		cached := httpctx.GetPlainRequestBytes(req)
		require.Same(t, &bare[0], &decoded[0])
		require.Same(t, &bare[0], &cached[0])

		external[len(external)-1] = 'X'
		external = nil
		runtime.GC()
		require.Equal(t, expected, httpctx.GetBareRequestBytes(req))
		require.Equal(t, expected, httpctx.GetPlainRequestBytes(req))
	})
}

func BenchmarkCachePlainRequestBytesIfStorable(b *testing.B) {
	for _, size := range []int{128 * 1024, 256 * 1024} {
		packet := makePlainRequestPacket(size)
		for _, tc := range []struct {
			name string
			fn   func(*http.Request, []byte)
		}{
			{name: "legacy-body-clone", fn: legacyCachePlainRequestBytesIfStorable},
			{name: "readonly-body-view", fn: cachePlainRequestBytesIfStorable},
		} {
			b.Run(fmt.Sprintf("body-%dKiB/%s", size/1024, tc.name), func(b *testing.B) {
				req := httptest.NewRequest(http.MethodPost, "http://example.com/upload", nil)
				b.SetBytes(int64(len(packet)))
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					tc.fn(req, packet)
				}
			})
		}
	}
}

var benchmarkDecodedPlainRequest []byte

func BenchmarkDecodeAndCachePlainRequestBytesIfStorable(b *testing.B) {
	for _, size := range []int{64 * 1024, 128 * 1024} {
		packet := makePlainRequestPacket(size)
		for _, tc := range []struct {
			name string
			fn   func(*http.Request, []byte) []byte
		}{
			{name: "clone-unencoded-packet", fn: cloneDecodedPlainRequestBytesIfStorable},
			{name: "borrow-context-owned-bare", fn: decodeAndCachePlainRequestBytesIfStorable},
		} {
			b.Run(fmt.Sprintf("body-%dKiB/%s", size/1024, tc.name), func(b *testing.B) {
				req := httptest.NewRequest(http.MethodPost, "http://example.com/upload", nil)
				httpctx.SetBareRequestBytesOwned(req, bytes.Clone(packet))
				bare := httpctx.GetBareRequestBytes(req)
				b.SetBytes(int64(len(bare)))
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					benchmarkDecodedPlainRequest = tc.fn(req, bare)
				}
			})
		}
	}
}
