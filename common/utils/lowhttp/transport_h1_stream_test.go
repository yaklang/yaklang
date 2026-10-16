package lowhttp

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/lowhttp/httpctx"
)

func TestDirectH1NoBodyBufferClosingResponse(t *testing.T) {
	const body = "data: hello\n\ndata: [DONE]\n\n"
	// Many header lines keep the stream goroutine busy while the transport
	// finishes parsing. Run with -race to catch accesses before handler cleanup.
	header := "HTTP/1.1 200 OK\r\nConnection: close\r\nContent-Type: text/event-stream\r\n" +
		strings.Repeat("X-Padding: abcdefghijklmnopqrstuvwxyz\r\n", 512) +
		fmt.Sprintf("Content-Length: %d\r\n\r\n", len(body))
	for i := 0; i < 10; i++ {
		client, server := net.Pipe()
		t.Cleanup(func() { client.Close(); server.Close() })
		go func() {
			defer server.Close()
			if _, err := http.ReadRequest(bufio.NewReader(server)); err == nil {
				_, _ = io.WriteString(server, header+body)
			}
		}()
		req, err := http.NewRequest(http.MethodGet, "http://example.test/", nil)
		require.NoError(t, err)
		httpctx.SetNoBodyBuffer(req, true)
		captured := make(chan string, 1)
		tr := &transportRequest{
			option: &LowhttpExecConfig{
				NoBodyBuffer: true, DiscardIntermediateResponseBody: true,
				DefaultBufferSize: len(header) + len(body),
				BodyStreamReaderHandler: func(_ []byte, reader io.ReadCloser) {
					defer reader.Close()
					data, _ := io.ReadAll(reader)
					captured <- string(data)
				},
			},
			reqIns: req, timeout: time.Second, traceInfo: &LowhttpTraceInfo{},
		}
		result, err := (&h1Transport{}).roundTripDirect(context.Background(), tr, client,
			[]byte("GET / HTTP/1.1\r\nHost: example.test\r\n\r\n"))
		require.NoError(t, err)
		require.NotNil(t, result.firstResponse)
		require.Equal(t, header, string(result.rawBytes))
		select {
		case got := <-captured:
			require.Equal(t, body, got)
		default:
			t.Fatal("stream handler did not finish")
		}
	}
}

func TestLegacyProxyFallbackNoBodyBufferSSE(t *testing.T) {
	const body = "data: hello\n\ndata: [DONE]\n\n"
	var connects, forwarded atomic.Int32
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodConnect {
			connects.Add(1)
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if r.RequestURI != "http://target.invalid/events" || !r.Close {
			t.Errorf("unexpected forward request: uri=%q close=%v", r.RequestURI, r.Close)
		}
		forwarded.Add(1)
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Connection", "close")
		_, _ = io.WriteString(w, body)
	}))
	defer proxy.Close()
	for i := 0; i < 10; i++ {
		captured := make(chan string, 1)
		rsp, err := HTTPWithoutRedirect(
			WithPacketBytes([]byte("GET /events HTTP/1.1\r\nHost: target.invalid\r\n\r\n")),
			WithProxy(proxy.URL), WithConnPool(false), WithNoBodyBuffer(true),
			WithEnableSystemProxyFromEnv(false), WithSaveHTTPFlow(false), WithDisableSession(true),
			WithTimeout(2*time.Second), WithConnectTimeout(time.Second),
			WithBodyStreamReaderHandler(func(_ []byte, reader io.ReadCloser) {
				defer reader.Close()
				data, _ := io.ReadAll(reader)
				captured <- string(data)
			}),
		)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, GetStatusCodeFromResponse(rsp.RawPacket))
		require.Empty(t, GetHTTPPacketBody(rsp.RawPacket))
		select {
		case got := <-captured:
			require.Equal(t, body, got)
		default:
			t.Fatal("stream handler did not finish")
		}
	}
	require.Equal(t, int32(10), connects.Load())
	require.Equal(t, int32(10), forwarded.Load())
}

func TestDirectH1NoBodyBufferPartialResponses(t *testing.T) {
	const partialHeader = "HTTP/1.1 200 OK\r\nX-Incomplete: value"
	const header = "HTTP/1.1 200 OK\r\nContent-Length: 100\r\n\r\n"
	for _, tc := range []struct {
		name       string
		wire       string
		wantRaw    string
		cancel     bool
		timeout    bool
		wantError  bool
		wantStream bool
	}{
		{name: "empty_eof", wantError: true},
		{name: "tls_alert", wire: "\x15\x03\x01\x00\x02\x02", wantRaw: "\x15\x03\x01\x00\x02\x02", wantError: true},
		{name: "malformed", wire: "invalid status\r\nX-Test: value\r\n\r\nbody", wantRaw: "invalid status\r\nX-Test: value\r\n\r\n", wantStream: true},
		{name: "header_eof", wire: partialHeader, wantRaw: partialHeader},
		{name: "header_cancel", wire: partialHeader, wantRaw: partialHeader, cancel: true},
		{name: "header_timeout", wire: partialHeader, wantRaw: partialHeader, timeout: true},
		{name: "body_cancel", wire: header + "partial body", wantRaw: header, cancel: true, wantStream: true},
		{name: "body_timeout", wire: header + "partial body", wantRaw: header, timeout: true, wantStream: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			client, server := net.Pipe()
			defer client.Close()
			defer server.Close()
			release := make(chan struct{})
			defer close(release)
			go func() {
				defer server.Close()
				if _, err := http.ReadRequest(bufio.NewReader(server)); err != nil {
					return
				}
				if tc.wire != "" {
					_, _ = io.WriteString(server, tc.wire)
				}
				if tc.cancel {
					cancel()
				}
				if tc.cancel || tc.timeout {
					<-release
				}
			}()
			req, err := http.NewRequest(http.MethodGet, "http://example.test/", nil)
			require.NoError(t, err)
			httpctx.SetNoBodyBuffer(req, true)
			captured := make(chan string, 1)
			tr := &transportRequest{
				option: &LowhttpExecConfig{
					NoBodyBuffer: true, DiscardIntermediateResponseBody: true,
					BodyStreamReaderHandler: func(_ []byte, reader io.ReadCloser) {
						defer reader.Close()
						data, _ := io.ReadAll(reader)
						captured <- string(data)
					},
				},
				reqIns: req, timeout: 100 * time.Millisecond, traceInfo: &LowhttpTraceInfo{},
			}
			result, err := (&h1Transport{}).roundTripDirect(ctx, tr, client,
				[]byte("GET / HTTP/1.1\r\nHost: example.test\r\n\r\n"))
			if tc.wantError {
				require.Error(t, err)
			}
			require.NotNil(t, result)
			require.True(t, result.portIsOpen)
			require.Equal(t, "pipe", result.remoteAddr)
			require.Equal(t, tc.wantRaw, string(result.rawBytes))
			select {
			case got := <-captured:
				require.True(t, tc.wantStream)
				require.Equal(t, strings.TrimPrefix(tc.wire, tc.wantRaw), got)
			default:
				require.False(t, tc.wantStream, "stream callback did not finish")
			}
		})
	}
}

func TestDirectH1NoBodyBufferAuthRetry(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()
	const finalHeader = "HTTP/1.1 200 OK\r\nConnection: close\r\nContent-Length: 2\r\n\r\n"
	go func() {
		defer server.Close()
		reader := bufio.NewReader(server)
		if _, err := http.ReadRequest(reader); err != nil {
			return
		}
		_, _ = io.WriteString(server, "HTTP/1.1 401 Unauthorized\r\nWWW-Authenticate: Basic realm=\"test\"\r\nContent-Length: 0\r\n\r\n")
		retry, err := http.ReadRequest(reader)
		if err != nil {
			return
		}
		if retry.Header.Get("Authorization") != "Basic dXNlcjpwYXNz" {
			t.Errorf("unexpected authorization: %q", retry.Header.Get("Authorization"))
		}
		_, _ = io.WriteString(server, finalHeader+"ok")
	}()
	packet := []byte("GET / HTTP/1.1\r\nHost: example.test\r\n\r\n")
	req, err := http.NewRequest(http.MethodGet, "http://example.test/", nil)
	require.NoError(t, err)
	httpctx.SetNoBodyBuffer(req, true)
	tr := &transportRequest{
		option: &LowhttpExecConfig{
			Packet: packet, Username: "user", Password: "pass", NoBodyBuffer: true,
			BodyStreamReaderHandler: func(_ []byte, reader io.ReadCloser) {
				defer reader.Close()
				_, _ = io.Copy(io.Discard, reader)
			},
		},
		reqIns: req, timeout: time.Second, traceInfo: &LowhttpTraceInfo{},
	}
	result, err := (&h1Transport{}).roundTripDirect(context.Background(), tr, client, packet)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotNil(t, result.firstResponse)
	require.Equal(t, http.StatusOK, result.firstResponse.StatusCode)
	require.Equal(t, finalHeader, string(result.rawBytes), "auth retry must reset header capture along with the buffer")
}
