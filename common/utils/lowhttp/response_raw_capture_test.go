package lowhttp

import (
	"context"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils"
)

func TestIsSSEContentTypeHeader(t *testing.T) {
	tests := []struct {
		name   string
		header string
		want   bool
	}{
		{name: "exact", header: "Content-Type: text/event-stream", want: true},
		{name: "case_and_parameters", header: "content-type: Text/Event-Stream; charset=utf-8", want: true},
		{name: "json", header: "Content-Type: application/json"},
		{name: "json_profile_mentions_sse", header: `Content-Type: application/json; profile="text/event-stream"`},
		{name: "other_header_mentions_sse", header: "Content-Type: application/json\r\nX-Stream-Hint: text/event-stream"},
		{name: "lookalike_header", header: "X-Content-Type: text/event-stream"},
		{name: "missing", header: "Cache-Control: no-cache"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw := []byte("HTTP/1.1 200 OK\r\n" + tt.header + "\r\n\r\n")
			require.Equal(t, tt.want, IsSSEContentTypeHeader(raw))
		})
	}
}

func TestAutoDetectSSE_StreamableHTTPResponseSwitching(t *testing.T) {
	const (
		firstJSON = `{"jsonrpc":"2.0","id":1,"result":{"mode":"json-first"}}`
		sseBody   = "event: message\ndata: {\"jsonrpc\":\"2.0\",\"result\":{\"mode\":\"sse\"}}\n\n"
		lastJSON  = `{"jsonrpc":"2.0","id":2,"result":{"mode":"json-last"}}`
	)

	serve := func(w http.ResponseWriter, r *http.Request) {
		// Echo the peer address so the test can prove that response-mode changes
		// happen on the same persistent connection, rather than by reconnecting.
		w.Header().Set("X-Mock-Remote", r.RemoteAddr)
		switch r.URL.Path {
		case "/json-first":
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("X-Stream-Hint", "text/event-stream")
			_, _ = io.WriteString(w, firstJSON)
		case "/events":
			w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
			_, _ = io.WriteString(w, sseBody)
		case "/json-last":
			w.Header().Set("Content-Type", `application/json; profile="text/event-stream"`)
			_, _ = io.WriteString(w, lastJSON)
		default:
			http.NotFound(w, r)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	h1Host, h1Port := utils.DebugMockHTTPHandlerFunc(serve)
	h2Host, h2Port := utils.DebugMockHTTP2HandlerFuncContext(ctx, serve)

	transports := []struct {
		name        string
		host        string
		port        int
		httpVersion string
		https       bool
		http2       bool
	}{
		{name: "http1_pool", host: h1Host, port: h1Port, httpVersion: "HTTP/1.1"},
		{name: "http2_pool", host: h2Host, port: h2Port, httpVersion: "HTTP/2.0", https: true, http2: true},
	}

	type streamCapture struct {
		header []byte
		body   []byte
		err    error
	}
	for _, transport := range transports {
		transport := transport
		t.Run(transport.name, func(t *testing.T) {
			pool := NewHttpConnPool(ctx, 10, 2)
			t.Cleanup(pool.Clear)

			replay := func(path string) (*LowhttpResponse, streamCapture) {
				t.Helper()
				captureCh := make(chan streamCapture, 2)
				request := []byte("GET " + path + " " + transport.httpVersion + "\r\nHost: " + utils.HostPort(transport.host, transport.port) + "\r\nAccept: application/json, text/event-stream\r\n\r\n")
				rsp, err := HTTP(
					WithPacketBytes(request),
					WithHttps(transport.https),
					WithHttp2(transport.http2),
					WithConnPool(true),
					ConnPool(pool),
					WithTimeout(5*time.Second),
					WithAutoDetectSSE(true),
					WithBodyStreamReaderHandler(func(header []byte, body io.ReadCloser) {
						defer body.Close()
						bodyBytes, readErr := io.ReadAll(body)
						captureCh <- streamCapture{header: append([]byte(nil), header...), body: bodyBytes, err: readErr}
					}),
				)
				require.NoError(t, err)
				select {
				case captured := <-captureCh:
					require.NoError(t, captured.err)
					select {
					case duplicate := <-captureCh:
						t.Fatalf("stream handler called more than once: duplicate header=%q body=%q err=%v", duplicate.header, duplicate.body, duplicate.err)
					default:
					}
					return rsp, captured
				case <-time.After(2 * time.Second):
					t.Fatal("stream handler did not finish")
					return nil, streamCapture{}
				}
			}

			firstRsp, firstCapture := replay("/json-first")
			require.False(t, IsSSEContentTypeHeader(firstCapture.header))
			require.Equal(t, firstJSON, string(GetHTTPPacketBody(firstRsp.RawPacket)))
			require.Equal(t, firstJSON, string(firstCapture.body))
			remoteAddr := GetHTTPPacketHeader(firstCapture.header, "X-Mock-Remote")
			require.NotEmpty(t, remoteAddr)

			eventRsp, eventCapture := replay("/events")
			require.True(t, IsSSEContentTypeHeader(eventCapture.header))
			require.Empty(t, GetHTTPPacketBody(eventRsp.RawPacket), "SSE body must stay out of the raw response buffer")
			require.Equal(t, sseBody, string(eventCapture.body))
			require.Equal(t, remoteAddr, GetHTTPPacketHeader(eventCapture.header, "X-Mock-Remote"), "SSE must use the same persistent connection")

			lastRsp, lastCapture := replay("/json-last")
			require.False(t, IsSSEContentTypeHeader(lastCapture.header))
			require.Equal(t, lastJSON, string(GetHTTPPacketBody(lastRsp.RawPacket)))
			require.Equal(t, lastJSON, string(lastCapture.body))
			require.Equal(t, remoteAddr, GetHTTPPacketHeader(lastCapture.header, "X-Mock-Remote"), "SSE mode must not leak into the next JSON response")
		})
	}
}
