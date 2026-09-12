package parser

import (
	"bytes"
	"fmt"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
	"testing"
)

func captureLegacy(t testing.TB, w []byte, rule string, cfg map[string]any, keys ...string) (map[string]any, error) {
	t.Helper()
	r := &structuredReader{Reader: bytes.NewReader(w), bits: uint64(len(w)) * 8}
	n, err := ParseBinaryWithConfig(r, rule, cfg, keys...)
	if err != nil {
		return nil, err
	}
	if r.Len() != 0 {
		return nil, fmt.Errorf("unconsumed bytes")
	}
	return map[string]any{"fields": stream_parser.NodeToMap(n), "metadata": n.Cfg.GetItem("additionInfo")}, nil
}

func TestCaptureStructuredHTTPParity(t *testing.T) {
	require.True(t, captureStructuredRules()["http"], "rule changed: audit adapter before updating fingerprint")
	cases := []struct {
		wire, method string
		close        bool
	}{
		{"GET / HTTP/1.1\r\nHost: example.test\r\n\r\n", "", false},
		{"POST / HTTP/1.1\r\nContent-Length: 3\r\ncontent-length: 003\r\n\r\nabc", "", false},
		{"HTTP/1.1 200 OK\r\nTransfer-Encoding: chunked\r\n\r\n3; ext=1\r\nabc\r\n0\r\nFoo: bar\r\n\r\n", "GET", false},
		{"HTTP/1.0 200 OK\r\n\r\nbody", "GET", true},
		{"HTTP/1.1 200 OK\r\nContent-Length: 999\r\n\r\n", "HEAD", false},
		{"HTTP/1.1 200 OK\r\nContent-Length: 999\r\n\r\n", "CONNECT", false},
		{"HTTP/1.1 204 No Content\r\n\r\n", "GET", false},
		{"POST / HTTP/1.1\r\nTransfer-Encoding: chunked\r\n\r\n 3 \r\nabc\r\n0\r\n\r\n", "", false},
	}
	for _, tc := range cases {
		cfg := map[string]any{"httpResponseToMethod": tc.method, "httpCloseDelimited": tc.close}
		w := []byte(tc.wire)
		want, err := captureLegacy(t, w, "application-layer.http", cfg, "HTTPExact")
		require.NoError(t, err)
		got, ok, err := captureHTTPStructured(w, cfg)
		require.NoError(t, err)
		require.True(t, ok)
		require.Equal(t, want, got)
		for i := range w {
			w[i] = '!'
		}
		require.Equal(t, want, got, "result owns input")
		got, err = ParseStructuredWithConfig([]byte(tc.wire), "application-layer.http", cfg, "HTTPExact")
		require.NoError(t, err)
		require.Equal(t, want, got)
	}
	for _, w := range []string{
		"GET / HTTP/1.1\r\nBad Header: x\r\n\r\n",
		"POST / HTTP/1.1\r\nContent-Length: 3\r\nContent-Length: 4\r\n\r\nabc",
		"POST / HTTP/1.1\r\nContent-Length: 0\r\nTransfer-Encoding: chunked\r\n\r\n0\r\n\r\n",
		"POST / HTTP/1.1\r\nTransfer-Encoding: chunked\r\n\r\n3\r\nabcXX0\r\n\r\n",
		"POST / HTTP/1.1\r\nTransfer-Encoding: chunked\r\n\r\n0\r\nContent-Length: 5\r\n\r\n",
		"GET / HTTP/2.0\r\n\r\n", "GET / HTTP/1.1\r\n\r\nextra",
	} {
		_, _, err := captureHTTPStructured([]byte(w), nil)
		require.Error(t, err, w)
		_, err = captureLegacy(t, []byte(w), "application-layer.http", nil, "HTTPExact")
		require.Error(t, err, w)
	}
}

func TestCaptureStructuredTLSParity(t *testing.T) {
	require.True(t, captureStructuredRules()["tls"])
	for _, w := range [][]byte{{23, 3, 3, 0, 3, 1, 2, 3}, {21, 3, 3, 0, 0}, {22, 3, 3, 0, 3, 2, 0, 0}, {20, 3, 1, 0, 1, 1, 23, 3, 3, 0, 2, 4, 5}} {
		want, err := captureLegacy(t, w, "application-layer.tls", nil)
		require.NoError(t, err)
		got, ok := captureTLSStructured(w)
		require.True(t, ok)
		require.Equal(t, want, got)
		actual, err := ParseStructured(w, "application-layer.tls")
		require.NoError(t, err)
		require.Equal(t, want, actual)
		for i := range w {
			w[i] = 0
		}
		require.Equal(t, want, got)
	}
	for _, w := range [][]byte{{22, 3, 3, 0, 1, 1}, {23, 3, 3, 0, 3, 1}, {23}, {}} {
		_, ok := captureTLSStructured(w)
		require.False(t, ok)
	}
}

func TestCaptureStructuredRespectsRegistrationAndConfig(t *testing.T) {
	original := base.ParserRegistration("default")
	defer base.RegisterParser("default", original)
	custom := &structuredCountingParser{}
	base.RegisterParser("default", custom)
	_, err := ParseStructured([]byte{23, 3, 3, 0, 0}, "application-layer.tls")
	require.NoError(t, err)
	require.Positive(t, custom.calls)
	custom.calls = 0
	_, err = ParseStructuredWithConfig([]byte("GET / HTTP/1.1\r\n\r\n"), "application-layer.http", nil, "HTTPExact")
	require.NoError(t, err)
	require.Positive(t, custom.calls)
	_, ok, _ := captureHTTPStructured(nil, map[string]any{"unknown": true})
	require.False(t, ok)
}

func FuzzCaptureHTTPStructured(f *testing.F) {
	for _, w := range []string{"GET / HTTP/1.1\r\nHost: x\r\n\r\n", "POST / HTTP/1.1\r\nTransfer-Encoding: chunked\r\n\r\n3\r\nabc\r\n0\r\n\r\n"} {
		f.Add(w)
	}
	f.Fuzz(func(t *testing.T, w string) {
		if len(w) > 65536 {
			t.Skip()
		}
		got, ok, err := captureHTTPStructured([]byte(w), nil)
		if ok && err == nil {
			want, err := captureLegacy(t, []byte(w), "application-layer.http", nil, "HTTPExact")
			require.NoError(t, err)
			require.Equal(t, want, got)
		}
	})
}
