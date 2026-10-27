package lowhttp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/lowhttp/httptrace"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/hpack"
)

func TestNetworkTraceH2RefusedStreamHasSeparateRecord(t *testing.T) {
	ctx, dir := traceTestContext(t)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer listener.Close()
	done := make(chan struct{})
	go func() {
		defer close(done)
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		conn.SetDeadline(time.Now().Add(5 * time.Second))
		preface := make([]byte, len(http2.ClientPreface))
		if _, err := io.ReadFull(conn, preface); err != nil {
			return
		}
		framer := http2.NewFramer(conn, conn)
		if framer.WriteSettings() != nil {
			return
		}
		requests := 0
		for {
			frame, err := framer.ReadFrame()
			if err != nil {
				return
			}
			switch f := frame.(type) {
			case *http2.SettingsFrame:
				if !f.IsAck() {
					framer.WriteSettingsAck()
				}
			case *http2.HeadersFrame:
				requests++
				if requests == 1 {
					framer.WriteRSTStream(f.StreamID, http2.ErrCodeRefusedStream)
					continue
				}
				var header bytes.Buffer
				encoder := hpack.NewEncoder(&header)
				encoder.WriteField(hpack.HeaderField{Name: ":status", Value: "200"})
				encoder.WriteField(hpack.HeaderField{Name: "content-length", Value: "2"})
				framer.WriteHeaders(http2.HeadersFrameParam{StreamID: f.StreamID, EndHeaders: true, BlockFragment: header.Bytes()})
				framer.WriteData(f.StreamID, true, []byte("ok"))
			}
		}
	}()
	pool := h2PoolFor(context.Background(), time.Minute)
	defer func() { pool.Clear(); <-done }()
	address := listener.Addr().(*net.TCPAddr)
	_, err = HTTP(WithPacketBytes([]byte(fmt.Sprintf("POST / HTTP/2\r\nHost: %s\r\n\r\n", address))), WithHost("127.0.0.1"), WithPort(address.Port), WithHttp2(true), WithConnPool(true), ConnPool(pool), WithContext(ctx), WithSaveHTTPFlow(false), WithTimeout(3*time.Second))
	require.NoError(t, err)
	records, _ := traceTestRecords(t, dir)
	require.Len(t, records, 2)
	failed, complete := 0, 0
	for _, record := range records {
		if record.TransportComplete {
			complete++
			require.Equal(t, 200, record.Status)
		} else {
			failed++
			require.Contains(t, record.Error, "REFUSED_STREAM")
		}
	}
	require.Equal(t, 1, failed)
	require.Equal(t, 1, complete)
}

func traceTestContext(t *testing.T) (context.Context, string) {
	t.Helper()
	dir := t.TempDir()
	ctx := httptrace.WithDirectory(context.Background(), dir)
	ctx = httptrace.WithScope(ctx, httptrace.Scope{SessionID: t.Name(), CallID: "call-fixture"})
	return httptrace.WithEnabled(ctx, true), dir
}
func traceTestRecords(t *testing.T, dir string) ([]httptrace.Metadata, []string) {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(dir, "*", "*", "metadata.json"))
	require.NoError(t, err)
	var records []httptrace.Metadata
	for _, file := range files {
		b, err := os.ReadFile(file)
		require.NoError(t, err)
		var record httptrace.Metadata
		require.NoError(t, json.Unmarshal(b, &record))
		records = append(records, record)
	}
	return records, files
}

func TestNetworkTraceCompleteStreaming(t *testing.T) {
	body := "data: " + strings.Repeat("0123456789", 3000) + "\n\ndata: [DONE]\n\n"
	for _, mode := range []string{"h1-direct", "h1-pool", "h2"} {
		t.Run(mode, func(t *testing.T) {
			ctx, dir := traceTestContext(t)
			server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				if mode == "h2" {
					w.Header().Set("Trailer", "X-Test-End")
					w.WriteHeader(103)
				}
				w.WriteHeader(200)
				w.(http.Flusher).Flush()
				for _, part := range []string{body[:1000], body[1000:]} {
					io.WriteString(w, part)
					w.(http.Flusher).Flush()
				}
				if mode == "h2" {
					w.Header().Set("X-Test-End", "complete")
				}
			}))
			if mode == "h2" {
				server.EnableHTTP2 = true
				server.StartTLS()
			} else {
				server.Start()
			}
			defer server.Close()
			addr := server.Listener.Addr().(*net.TCPAddr)
			proto := "HTTP/1.1"
			if mode == "h2" {
				proto = "HTTP/2"
			}
			request := []byte(fmt.Sprintf("POST /chat %s\r\nHost: %s\r\nAuthorization: Bearer fixture-secret\r\nContent-Length: 5\r\n\r\nhello", proto, addr))
			var consumed []byte
			_, err := HTTP(WithPacketBytes(request), WithHost("127.0.0.1"), WithPort(addr.Port), WithHttps(mode == "h2"), WithHttp2(mode == "h2"), WithConnPool(mode != "h1-direct"), WithContext(ctx), WithNoBodyBuffer(true), WithSaveHTTPFlow(false), WithTimeout(3*time.Second), WithBodyStreamReaderHandler(func(_ []byte, reader io.ReadCloser) { consumed, _ = io.ReadAll(reader) }))
			require.NoError(t, err)
			require.Contains(t, string(consumed), "[DONE]")
			records, files := traceTestRecords(t, dir)
			require.Len(t, records, 1)
			meta := records[0]
			if mode == "h2" {
				require.Len(t, meta.AdditionalHeaders, 2)
				require.Equal(t, 103, meta.AdditionalHeaders[0].Status)
				require.Equal(t, "complete", meta.AdditionalHeaders[1].Headers.Get("X-Test-End"))
			}
			require.True(t, meta.TransportComplete)
			require.Equal(t, 200, meta.Status)
			require.Greater(t, meta.ResponseBytes, int64(30000))
			require.NotNil(t, meta.FirstByteMS)
			require.GreaterOrEqual(t, meta.DurationMS, *meta.FirstByteMS)
			packet, err := os.ReadFile(filepath.Join(filepath.Dir(files[0]), "response.http"))
			require.NoError(t, err)
			if mode != "h2" {
				packet, err = FixHTTPResponsePacket(packet)
				require.NoError(t, err)
			}
			_, captured := SplitHTTPHeadersAndBodyFromPacket(packet)
			require.Equal(t, body, string(captured))
			requestBytes, err := os.ReadFile(filepath.Join(filepath.Dir(files[0]), "request.http"))
			require.NoError(t, err)
			require.Contains(t, string(requestBytes), "Bearer fixture-secret")
			require.Contains(t, string(requestBytes), "hello")
		})
	}
}

func TestNetworkTraceBrokenResponseAndDisabled(t *testing.T) {
	ctx, dir := traceTestContext(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, _, _ := w.(http.Hijacker).Hijack()
		defer conn.Close()
		io.WriteString(conn, "HTTP/1.1 200 OK\r\nContent-Type: text/event-stream\r\nContent-Length: 10000\r\n\r\ndata: half")
	}))
	defer server.Close()
	packet := UrlToRequestPacket("POST", server.URL, nil, false)
	_, err := HTTP(WithPacketBytes(packet), WithContext(ctx), WithRetryTimes(0), WithConnPool(false), WithSaveHTTPFlow(false))
	_ = err // lowhttp may intentionally tolerate a short Content-Length body
	meta, files := traceTestRecords(t, dir)
	require.Len(t, meta, 1)
	require.False(t, meta[0].TransportComplete)
	require.NotEmpty(t, meta[0].FramingError)
	raw, err := os.ReadFile(filepath.Join(filepath.Dir(files[0]), "response.http"))
	require.NoError(t, err)
	require.Contains(t, string(raw), "data: half")
	_, _ = HTTP(WithPacketBytes(packet), WithContext(httptrace.WithEnabled(ctx, false)), WithSaveHTTPFlow(false))
	meta, _ = traceTestRecords(t, dir)
	require.Len(t, meta, 1)
}

func TestNetworkTraceRetriesAndCancellation(t *testing.T) {
	ctx, dir := traceTestContext(t)
	n := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n++
		w.Header().Set("Retry-After", "17")
		if n < 3 {
			w.WriteHeader(429)
		}
		io.WriteString(w, "response")
	}))
	defer server.Close()
	_, err := HTTP(WithPacketBytes(UrlToRequestPacket("POST", server.URL, nil, false)), WithContext(ctx), WithRetryTimes(2), WithRetryInStatusCode([]int{429}), WithSaveHTTPFlow(false), WithRetryWaitTime(time.Millisecond))
	require.NoError(t, err)
	records, _ := traceTestRecords(t, dir)
	require.Len(t, records, 3)
	limited := 0
	for _, r := range records {
		if r.Status == 429 {
			limited++
			require.Equal(t, "17", r.RetryAfter)
		}
	}
	require.Equal(t, 2, limited)
	ctx, dir = traceTestContext(t)
	ctx, cancel := context.WithCancel(ctx)
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, "data: partial\n\n")
		w.(http.Flusher).Flush()
		cancel()
	}))
	defer slow.Close()
	_, _ = HTTP(WithPacketBytes(UrlToRequestPacket("POST", slow.URL, nil, false)), WithContext(ctx), WithSaveHTTPFlow(false))
	records, _ = traceTestRecords(t, dir)
	require.Len(t, records, 1)
	require.False(t, records[0].TransportComplete)
}

func TestNetworkTraceNeverSavesHTTPFlow(t *testing.T) {
	ctx, dir := traceTestContext(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "diagnostic body") }))
	defer server.Close()
	old := saveHTTPFlowFunc
	saves := 0
	saveHTTPFlowFunc = func(*LowhttpResponse, bool) { saves++ }
	defer func() { saveHTTPFlowFunc = old }()
	handlers := 0
	for _, enabled := range []bool{true, false} {
		_, err := HTTP(WithPacketBytes([]byte("GET / HTTP/1.1\r\nHost: "+strings.TrimPrefix(server.URL, "http://")+"\r\n\r\n")), WithContext(httptrace.WithEnabled(ctx, enabled)), WithSaveHTTPFlow(true), WithSaveHTTPFlowSync(true), WithSaveHTTPFlowHandler(func(*LowhttpResponse) { handlers++ }))
		require.NoError(t, err)
		if enabled {
			require.Zero(t, saves)
			require.Zero(t, handlers)
		}
	}
	require.Equal(t, 1, saves, "ordinary tool flows retain persistence")
	require.Equal(t, 1, handlers)
	records, _ := traceTestRecords(t, dir)
	require.Len(t, records, 1)
}
