package minimartian

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/utils/lowhttp/httpctx"
)

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) {
	return 0, fmt.Errorf("capture unavailable")
}

func TestBestEffortStreamRecorderDoesNotBreakForwarding(t *testing.T) {
	payload := []byte("data: still-forwarded\n\n")
	n, err := (&bestEffortStreamRecorder{writer: failingWriter{}}).Write(payload)
	require.NoError(t, err)
	require.Equal(t, len(payload), n)
}

func TestIsServerSentEventContentType(t *testing.T) {
	require.True(t, isServerSentEventContentType("text/event-stream"))
	require.True(t, isServerSentEventContentType("text/event-stream; charset=utf-8"))
	require.True(t, isServerSentEventContentType("text/event-stream;charset=utf-8"))
	require.False(t, isServerSentEventContentType("text/html"))
	require.False(t, isServerSentEventContentType("application/json"))
	require.False(t, isServerSentEventContentType(""))
}

type lockedReadWriter struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (w *lockedReadWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.b.Write(p)
}

func (w *lockedReadWriter) Read(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.b.Read(p)
}

func (w *lockedReadWriter) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.b.String()
}

type testStreamRecorder struct {
	lockedReadWriter
	closed chan struct{}
	once   sync.Once
}

func (r *testStreamRecorder) Close() error {
	r.once.Do(func() { close(r.closed) })
	return nil
}

// TestExecLowHTTPStreamsAndRecordsSSEBeforeEOF verifies that the SSE
// ResponseHeaderCallback path forwards body chunks to the downstream client
// and to the best-effort recorder in real time, before the upstream sends
// the terminating chunk.
func TestExecLowHTTPStreamsAndRecordsSSEBeforeEOF(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = listener.Close() })

	releaseUpstream := make(chan struct{})
	serverDone := make(chan struct{})
	go func() {
		defer close(serverDone)
		conn, acceptErr := listener.Accept()
		if acceptErr != nil {
			return
		}
		defer conn.Close()
		reader := bufio.NewReader(conn)
		for {
			line, readErr := reader.ReadString('\n')
			if readErr != nil || line == "\r\n" {
				break
			}
		}
		// Put Transfer-Encoding before Content-Type to verify that the SSE
		// callback takes precedence over the generic chunked callback.
		_, _ = io.WriteString(conn, "HTTP/1.1 200 OK\r\nTransfer-Encoding: chunked\r\nContent-Type: text/event-stream\r\nConnection: close\r\n\r\n")
		_, _ = io.WriteString(conn, "d\r\ndata: ready\n\n\r\n")
		<-releaseUpstream
		_, _ = io.WriteString(conn, "0\r\n\r\n")
	}()

	rawRequest := lowhttp.FixHTTPRequest([]byte(fmt.Sprintf("GET /events HTTP/1.1\r\nHost: %s\r\nConnection: close\r\n\r\n", listener.Addr().String())))
	req, err := lowhttp.ParseBytesToHttpRequest(rawRequest)
	require.NoError(t, err)
	httpctx.SetBareRequestBytes(req, rawRequest)
	httpctx.SetPlainRequestBytes(req, rawRequest)
	frontend := &lockedReadWriter{}
	httpctx.SetMITMFrontendReadWriter(req, frontend)

	proxy := NewProxy()
	poolCtx, cancelPool := context.WithCancel(context.Background())
	t.Cleanup(cancelPool)
	proxy.SetConnPool(lowhttp.NewHttpConnPool(poolCtx, 100, 2))
	recorderCreated := make(chan *testStreamRecorder, 1)
	proxy.SetHTTPStreamRecorderFactory(func(_ bool, _ *http.Request, _ *http.Response, _ []byte) (io.WriteCloser, error) {
		recorder := &testStreamRecorder{closed: make(chan struct{})}
		recorderCreated <- recorder
		return recorder, nil
	})

	requestDone := make(chan error, 1)
	go func() {
		_, execErr := proxy.execLowhttp(nil, req)
		requestDone <- execErr
	}()

	var recorder *testStreamRecorder
	select {
	case recorder = <-recorderCreated:
	case <-time.After(2 * time.Second):
		t.Fatal("stream recorder was not created after SSE response headers")
	}
	// Both the downstream client and the recorder should have received the
	// body chunk before the upstream sends the terminating 0\r\n\r\n.
	require.Eventually(t, func() bool {
		return strings.Contains(frontend.String(), "data: ready") && strings.Contains(recorder.String(), "data: ready")
	}, 2*time.Second, 20*time.Millisecond)
	// The response header must be written exactly once — no double-header race.
	require.Equal(t, 1, strings.Count(frontend.String(), "HTTP/1.1 200 OK"))

	// The request must still be in flight while the upstream is streaming.
	select {
	case err := <-requestDone:
		t.Fatalf("stream returned before upstream EOF: %v", err)
	default:
	}

	close(releaseUpstream)
	select {
	case err := <-requestDone:
		require.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("stream did not finish after upstream EOF")
	}
	select {
	case <-recorder.closed:
	case <-time.After(time.Second):
		t.Fatal("stream recorder was not closed")
	}
	<-serverDone
}

// TestExecLowHTTPCancelsUpstreamAfterClientDisconnect verifies that when the
// downstream client connection breaks after the header has been forwarded,
// the upstream context is cancelled so the blocking body reader can return.
func TestExecLowHTTPCancelsUpstreamAfterClientDisconnect(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = listener.Close() })

	serverDone := make(chan struct{})
	go func() {
		defer close(serverDone)
		conn, acceptErr := listener.Accept()
		if acceptErr != nil {
			return
		}
		defer conn.Close()
		reader := bufio.NewReader(conn)
		for {
			line, readErr := reader.ReadString('\n')
			if readErr != nil || line == "\r\n" {
				break
			}
		}
		_, _ = io.WriteString(conn, "HTTP/1.1 200 OK\r\nContent-Type: text/event-stream\r\nTransfer-Encoding: chunked\r\nConnection: close\r\n\r\n")
		// Send one chunk then keep the connection open.
		_, _ = io.WriteString(conn, "d\r\ndata: ready\n\n\r\n")
		// Block until the connection is closed by the client-side cancel.
		buf := make([]byte, 1)
		for {
			if _, e := conn.Read(buf); e != nil {
				break
			}
		}
	}()

	rawRequest := lowhttp.FixHTTPRequest([]byte(fmt.Sprintf("GET /events HTTP/1.1\r\nHost: %s\r\nConnection: close\r\n\r\n", listener.Addr().String())))
	req, err := lowhttp.ParseBytesToHttpRequest(rawRequest)
	require.NoError(t, err)
	httpctx.SetBareRequestBytes(req, rawRequest)
	httpctx.SetPlainRequestBytes(req, rawRequest)

	// Use a writer that fails after the header is written, simulating a
	// downstream client disconnect.
	frontend := &failAfterHeaderWriter{}
	httpctx.SetMITMFrontendReadWriter(req, frontend)

	proxy := NewProxy()
	poolCtx, cancelPool := context.WithCancel(context.Background())
	t.Cleanup(cancelPool)
	proxy.SetConnPool(lowhttp.NewHttpConnPool(poolCtx, 100, 2))

	requestDone := make(chan error, 1)
	go func() {
		_, execErr := proxy.execLowhttp(nil, req)
		requestDone <- execErr
	}()

	select {
	case <-requestDone:
		// The request should return (with an error) because the downstream
		// write failure cancelled the upstream context.
	case <-time.After(5 * time.Second):
		t.Fatal("stream did not return after downstream client disconnect")
	}
	<-serverDone
}

type failAfterHeaderWriter struct {
	bytes.Buffer
	headerWritten bool
}

func (w *failAfterHeaderWriter) Write(p []byte) (int, error) {
	if w.headerWritten {
		return 0, io.ErrClosedPipe
	}
	w.headerWritten = true
	return w.Buffer.Write(p)
}

func (w *failAfterHeaderWriter) Read(p []byte) (int, error) {
	return 0, io.EOF
}

// TestClassifyStreamResponse verifies the single decision point that picks the
// streaming forwarding kind, covering every branch of classifyStreamResponse:
//
//   - SSE (text/event-stream, with/without charset)               -> streamKindSSE
//   - content-type filtered (matcher hit / miss)                  -> streamKindFiltered / None
//   - chunked Transfer-Encoding (case-insensitive)               -> streamKindChunkedLarge
//   - Content-Length >= MaxContentLength (boundary >= vs <)      -> streamKindChunkedLarge / None
//   - nil response                                                -> streamKindNone
//
// The immediate-vs-buffered trigger and cancel-on-write-fail choices in
// makeStreamResponseCallback are derived from this kind, so this guards the
// invariants the rest of the forwarding code depends on.
func TestClassifyStreamResponse(t *testing.T) {
	const maxCL = 1024 * 1024 // 1 MiB

	mkReq := func() *http.Request {
		req, err := http.NewRequest("GET", "http://example.com/events", nil)
		require.NoError(t, err)
		return req
	}
	mkRsp := func(ct string, transferEncoding []string, contentLength int64) *http.Response {
		rsp := &http.Response{
			StatusCode: 200,
			Header:     http.Header{},
			Request:    mkReq(),
		}
		if ct != "" {
			rsp.Header.Set("Content-Type", ct)
		}
		rsp.TransferEncoding = transferEncoding
		rsp.ContentLength = contentLength
		return rsp
	}

	cases := []struct {
		name            string
		rsp             *http.Response
		filterMatcher   func(string) bool
		want            streamKind
	}{
		{"sse plain", mkRsp("text/event-stream", nil, -1), nil, streamKindSSE},
		{"sse with charset", mkRsp("text/event-stream; charset=utf-8", nil, -1), nil, streamKindSSE},
		{"sse with params", mkRsp("text/event-stream;charset=utf-8", nil, -1), nil, streamKindSSE},
		{"filtered hit", mkRsp("application/x-ndjson", nil, -1), func(ct string) bool {
			return strings.EqualFold(ct, "application/x-ndjson")
		}, streamKindFiltered},
		{"filtered miss (no matcher set)", mkRsp("application/json", nil, -1), nil, streamKindNone},
		{"filtered miss (matcher rejects)", mkRsp("application/json", nil, -1), func(ct string) bool {
			return false
		}, streamKindNone},
		{"chunked lowercase", mkRsp("text/plain", []string{"chunked"}, -1), nil, streamKindChunkedLarge},
		{"chunked mixedcase", mkRsp("text/plain", []string{"ChUnKeD"}, -1), nil, streamKindChunkedLarge},
		{"chunked + sse -> sse wins", mkRsp("text/event-stream", []string{"chunked"}, -1), nil, streamKindSSE},
		{"content-length over threshold", mkRsp("text/plain", nil, int64(maxCL) + 1), nil, streamKindChunkedLarge},
		{"content-length at threshold (>=)", mkRsp("text/plain", nil, int64(maxCL)), nil, streamKindChunkedLarge},
		{"content-length under threshold", mkRsp("text/plain", nil, int64(maxCL) - 1), nil, streamKindNone},
		{"content-length unknown (-1) small body", mkRsp("text/plain", nil, -1), nil, streamKindNone},
		{"plain html no stream", mkRsp("text/html", nil, 100), nil, streamKindNone},
		{"empty content-type", mkRsp("", nil, 100), nil, streamKindNone},
		{"nil response", nil, nil, streamKindNone},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := NewProxy()
			p.SetMaxContentLength(maxCL)

			req := mkReq()
			if tc.filterMatcher != nil {
				httpctx.SetResponseContentTypeFiltered(req, tc.filterMatcher)
			}

			got := p.classifyStreamResponse(req, tc.rsp)
			require.Equal(t, tc.want, got, "classifyStreamResponse kind mismatch")
		})
	}
}
