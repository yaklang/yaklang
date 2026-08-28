package yakgrpc

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestGRPCMUSTPASS_HTTPFuzzer_SSE_IncrementalChunkUpdates(t *testing.T) {
	host, port := utils.DebugMockHTTPHandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.WriteHeader(http.StatusOK)

		f, ok := w.(http.Flusher)
		require.True(t, ok, "http.Flusher should be supported")

		for i := 0; i < 4; i++ {
			_, _ = fmt.Fprintf(w, "data: msg%d\n\n", i)
			f.Flush()
			time.Sleep(350 * time.Millisecond)
		}

		time.Sleep(2 * time.Second)
	})

	c, err := NewLocalClient()
	require.NoError(t, err)

	stream, err := c.HTTPFuzzer(context.Background(), &ypb.FuzzerRequest{
		Request: fmt.Sprintf("GET / HTTP/1.1\r\nHost: %s\r\nAccept: text/event-stream\r\n\r\n", utils.HostPort(host, port)),
		// Incremental updates are expected; final "full response" should not be required.
		PerRequestTimeoutSeconds: 1.8,
		DialTimeoutSeconds:       1.0,
		ForceFuzz:                true,
	})
	require.NoError(t, err)

	var gotSSE int
	var last *ypb.FuzzerResponse
	var firstUUID string
	var gotFinal bool
	for {
		rsp, err := stream.Recv()
		if err != nil {
			break
		}
		if rsp == nil {
			continue
		}
		var hit bool
		for _, c := range rsp.RandomChunkedData {
			if c == nil {
				continue
			}
			if c.IsFinal {
				gotFinal = true
			}
			if bytes.Contains(c.Data, []byte("data: msg")) {
				hit = true
			}
		}
		if hit {
			gotSSE++
			last = rsp
		}
		if hit || len(rsp.RandomChunkedData) > 0 {
			if firstUUID == "" {
				firstUUID = rsp.UUID
			} else {
				require.Equal(t, firstUUID, rsp.UUID, "sse updates should share the same UUID")
			}
		}
	}

	require.GreaterOrEqual(t, gotSSE, 2, "should receive incremental SSE updates")
	require.NotNil(t, last)
	require.GreaterOrEqual(t, len(last.RandomChunkedData), 1, "should include response chunks")
	require.True(t, gotFinal, "should receive a final marker chunk")
}

func TestGRPCMUSTPASS_HTTPFuzzer_SSE_AutoDetectWithoutAccept(t *testing.T) {
	host, port := utils.DebugMockHTTPHandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.WriteHeader(http.StatusOK)

		f, ok := w.(http.Flusher)
		require.True(t, ok, "http.Flusher should be supported")

		for i := 0; i < 4; i++ {
			_, _ = fmt.Fprintf(w, "data: msg%d\n\n", i)
			f.Flush()
			time.Sleep(350 * time.Millisecond)
		}

		time.Sleep(2 * time.Second)
	})

	c, err := NewLocalClient()
	require.NoError(t, err)

	stream, err := c.HTTPFuzzer(context.Background(), &ypb.FuzzerRequest{
		// No Accept: text/event-stream; should still auto-detect SSE by response Content-Type.
		Request:                  fmt.Sprintf("GET / HTTP/1.1\r\nHost: %s\r\n\r\n", utils.HostPort(host, port)),
		PerRequestTimeoutSeconds: 1.8,
		DialTimeoutSeconds:       1.0,
		ForceFuzz:                true,
	})
	require.NoError(t, err)

	var gotSSE int
	var firstUUID string
	var gotFinal bool
	for {
		rsp, err := stream.Recv()
		if err != nil {
			break
		}
		if rsp == nil {
			continue
		}
		var hit bool
		for _, c := range rsp.RandomChunkedData {
			if c == nil {
				continue
			}
			if c.IsFinal {
				gotFinal = true
			}
			if bytes.Contains(c.Data, []byte("data: msg")) {
				hit = true
			}
		}
		if hit {
			gotSSE++
		}
		if hit || len(rsp.RandomChunkedData) > 0 {
			if firstUUID == "" {
				firstUUID = rsp.UUID
			} else {
				require.Equal(t, firstUUID, rsp.UUID, "sse updates should share the same UUID")
			}
		}
	}
	require.GreaterOrEqual(t, gotSSE, 2, "should receive incremental SSE updates without Accept header")
	require.True(t, gotFinal, "should receive a final marker chunk")
}

func TestGRPCMUSTPASS_HTTPFuzzer_SSE_HTTP2_IncrementalChunkUpdates(t *testing.T) {
	ctx := utils.TimeoutContextSeconds(10)
	host, port := utils.DebugMockHTTP2HandlerFuncContext(ctx, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.WriteHeader(http.StatusOK)

		f, ok := w.(http.Flusher)
		require.True(t, ok, "http.Flusher should be supported")

		for i := 0; i < 4; i++ {
			_, _ = fmt.Fprintf(w, "data: msg%d\n\n", i)
			f.Flush()
			time.Sleep(350 * time.Millisecond)
		}

		time.Sleep(2 * time.Second)
	})

	c, err := NewLocalClient()
	require.NoError(t, err)

	stream, err := c.HTTPFuzzer(context.Background(), &ypb.FuzzerRequest{
		Request:                  fmt.Sprintf("GET / HTTP/2.0\r\nHost: %s\r\nAccept: text/event-stream\r\n\r\n", utils.HostPort(host, port)),
		PerRequestTimeoutSeconds: 1.8,
		DialTimeoutSeconds:       1.0,
		ForceFuzz:                true,
		IsHTTPS:                  true,
	})
	require.NoError(t, err)

	var gotSSE int
	var last *ypb.FuzzerResponse
	var firstUUID string
	var gotFinal bool
	for {
		rsp, err := stream.Recv()
		if err != nil {
			break
		}
		if rsp == nil {
			continue
		}
		var hit bool
		for _, c := range rsp.RandomChunkedData {
			if c == nil {
				continue
			}
			if c.IsFinal {
				gotFinal = true
			}
			if bytes.Contains(c.Data, []byte("data: msg")) {
				hit = true
			}
		}
		if hit {
			gotSSE++
			last = rsp
		}
		if hit || len(rsp.RandomChunkedData) > 0 {
			if firstUUID == "" {
				firstUUID = rsp.UUID
			} else {
				require.Equal(t, firstUUID, rsp.UUID, "sse updates should share the same UUID")
			}
		}
	}

	require.GreaterOrEqual(t, gotSSE, 2, "should receive incremental SSE updates over HTTP/2")
	require.NotNil(t, last)
	require.GreaterOrEqual(t, len(last.RandomChunkedData), 1, "should include response chunks")
	require.True(t, gotFinal, "should receive a final marker chunk")
}

func TestGRPCMUSTPASS_HTTPFuzzer_SSE_HTTP2_AutoDetectWithoutAccept(t *testing.T) {
	ctx := utils.TimeoutContextSeconds(10)
	host, port := utils.DebugMockHTTP2HandlerFuncContext(ctx, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.WriteHeader(http.StatusOK)

		f, ok := w.(http.Flusher)
		require.True(t, ok, "http.Flusher should be supported")

		for i := 0; i < 4; i++ {
			_, _ = fmt.Fprintf(w, "data: msg%d\n\n", i)
			f.Flush()
			time.Sleep(350 * time.Millisecond)
		}

		time.Sleep(2 * time.Second)
	})

	c, err := NewLocalClient()
	require.NoError(t, err)

	stream, err := c.HTTPFuzzer(context.Background(), &ypb.FuzzerRequest{
		Request:                  fmt.Sprintf("GET / HTTP/2.0\r\nHost: %s\r\n\r\n", utils.HostPort(host, port)),
		PerRequestTimeoutSeconds: 5.0,
		DialTimeoutSeconds:       1.0,
		ForceFuzz:                true,
		IsHTTPS:                  true,
	})
	require.NoError(t, err)

	var gotSSE int
	var firstUUID string
	var gotFinal bool
	for {
		rsp, err := stream.Recv()
		if err != nil {
			break
		}
		if rsp == nil {
			continue
		}
		var hit bool
		for _, c := range rsp.RandomChunkedData {
			if c == nil {
				continue
			}
			if c.IsFinal {
				gotFinal = true
			}
			if bytes.Contains(c.Data, []byte("data: msg")) {
				hit = true
			}
		}
		if hit {
			gotSSE++
		}
		if hit || len(rsp.RandomChunkedData) > 0 {
			if firstUUID == "" {
				firstUUID = rsp.UUID
			} else {
				require.Equal(t, firstUUID, rsp.UUID, "sse updates should share the same UUID")
			}
		}
	}

	require.GreaterOrEqual(t, gotSSE, 2, "should receive incremental SSE updates over HTTP/2 without Accept header")
	require.True(t, gotFinal, "should receive a final marker chunk")
}

func TestGRPCMUSTPASS_HTTPFuzzer_MCPMixedAcceptKeepsJSONBody(t *testing.T) {
	const responseBody = `{"jsonrpc":"2.0","id":1,"result":{"protocolVersion":"2025-03-26","serverInfo":{"name":"streamable-mcp-server","version":"1.0.0"},"instructions":"mock MCP service"}}`
	const requestBody = `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"poc","version":"1.0"}}}`

	ctx := utils.TimeoutContextSeconds(15)
	h1Host, h1Port := utils.DebugMockHTTPHandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "/mcp", r.URL.Path)
		require.Equal(t, "application/json", r.Header.Get("Content-Type"))
		require.Equal(t, "application/json, text/event-stream", r.Header.Get("Accept"))
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		require.Equal(t, requestBody, string(body))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		// Match the real gateway's HTTP/1.1 response framing: headers are flushed
		// before the body, so net/http uses Transfer-Encoding: chunked.
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
		_, _ = w.Write([]byte(responseBody))
	})
	h2Host, h2Port := utils.DebugMockHTTP2HandlerFuncContext(ctx, func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "/mcp", r.URL.Path)
		require.Equal(t, "application/json", r.Header.Get("Content-Type"))
		require.Equal(t, "application/json, text/event-stream", r.Header.Get("Accept"))
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		require.Equal(t, requestBody, string(body))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(responseBody))
	})

	tests := []struct {
		name        string
		host        string
		port        int
		httpVersion string
		isHTTPS     bool
		disablePool bool
	}{
		{name: "http1_pool", host: h1Host, port: h1Port, httpVersion: "HTTP/1.1"},
		{name: "http1_no_pool", host: h1Host, port: h1Port, httpVersion: "HTTP/1.1", disablePool: true},
		{name: "http2_pool", host: h2Host, port: h2Port, httpVersion: "HTTP/2.0", isHTTPS: true},
		{name: "http2_no_pool", host: h2Host, port: h2Port, httpVersion: "HTTP/2.0", isHTTPS: true, disablePool: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := NewLocalClient()
			require.NoError(t, err)

			request := fmt.Sprintf("POST /mcp %s\r\nHost: %s\r\nContent-Type: application/json\r\nAccept: application/json, text/event-stream\r\nContent-Length: %d\r\n\r\n%s", tt.httpVersion, utils.HostPort(tt.host, tt.port), len(requestBody), requestBody)
			stream, err := client.HTTPFuzzer(ctx, &ypb.FuzzerRequest{
				Request:                  request,
				IsHTTPS:                  tt.isHTTPS,
				DisableUseConnPool:       tt.disablePool,
				PerRequestTimeoutSeconds: 5,
				DialTimeoutSeconds:       2,
				ForceFuzz:                true,
			})
			require.NoError(t, err)

			var response *ypb.FuzzerResponse
			for {
				got, recvErr := stream.Recv()
				if recvErr != nil {
					break
				}
				if got != nil && got.StatusCode == http.StatusOK {
					response = got
				}
			}

			require.NotNil(t, response)
			require.Contains(t, string(response.ResponseRaw), responseBody)
			require.Equal(t, int64(len(responseBody)), response.BodyLength)
			require.Empty(t, response.RandomChunkedData, "JSON MCP response must not be emitted as SSE chunks")
		})
	}
}

func TestGRPCMUSTPASS_HTTPFuzzer_MCPJSONContentTypeFalsePositives(t *testing.T) {
	const responseBody = `{"jsonrpc":"2.0","id":1,"result":{"protocolVersion":"2025-03-26","serverInfo":{"name":"streamable-mcp-server","version":"1.0.0"}}}`
	const requestBody = `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"poc","version":"1.0"}}}`

	type responseVariant struct {
		path        string
		contentType string
		streamHint  bool
	}
	variants := []responseVariant{
		{
			path:        "/mcp-json-profile",
			contentType: `application/json; profile="text/event-stream"`,
		},
		{
			path:        "/mcp-json-hint",
			contentType: "application/json",
			streamHint:  true,
		},
	}

	serve := func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "application/json, text/event-stream", r.Header.Get("Accept"))
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		require.Equal(t, requestBody, string(body))

		var matched *responseVariant
		for i := range variants {
			if variants[i].path == r.URL.Path {
				matched = &variants[i]
				break
			}
		}
		require.NotNil(t, matched)
		w.Header().Set("Content-Type", matched.contentType)
		if matched.streamHint {
			// A non-Content-Type header mentioning SSE must not change body ownership.
			w.Header().Set("X-Stream-Hint", "text/event-stream")
		}
		w.WriteHeader(http.StatusOK)
		if flusher, ok := w.(http.Flusher); ok {
			// Exercise HTTP/1.1 chunked framing, matching the real MCP gateway.
			flusher.Flush()
		}
		_, _ = w.Write([]byte(responseBody))
	}

	ctx := utils.TimeoutContextSeconds(20)
	h1Host, h1Port := utils.DebugMockHTTPHandlerFunc(serve)
	h2Host, h2Port := utils.DebugMockHTTP2HandlerFuncContext(ctx, serve)

	transports := []struct {
		name        string
		host        string
		port        int
		httpVersion string
		isHTTPS     bool
		disablePool bool
	}{
		{name: "http1_pool", host: h1Host, port: h1Port, httpVersion: "HTTP/1.1"},
		{name: "http1_no_pool", host: h1Host, port: h1Port, httpVersion: "HTTP/1.1", disablePool: true},
		{name: "http2_pool", host: h2Host, port: h2Port, httpVersion: "HTTP/2.0", isHTTPS: true},
		{name: "http2_no_pool", host: h2Host, port: h2Port, httpVersion: "HTTP/2.0", isHTTPS: true, disablePool: true},
	}

	for _, variant := range variants {
		for _, transport := range transports {
			variant := variant
			transport := transport
			t.Run(strings.TrimPrefix(variant.path, "/")+"/"+transport.name, func(t *testing.T) {
				client, err := NewLocalClient()
				require.NoError(t, err)

				request := fmt.Sprintf("POST %s %s\r\nHost: %s\r\nContent-Type: application/json\r\nAccept: application/json, text/event-stream\r\nContent-Length: %d\r\n\r\n%s", variant.path, transport.httpVersion, utils.HostPort(transport.host, transport.port), len(requestBody), requestBody)
				stream, err := client.HTTPFuzzer(ctx, &ypb.FuzzerRequest{
					Request:                  request,
					IsHTTPS:                  transport.isHTTPS,
					DisableUseConnPool:       transport.disablePool,
					PerRequestTimeoutSeconds: 5,
					DialTimeoutSeconds:       2,
					ForceFuzz:                true,
				})
				require.NoError(t, err)

				var completeResponse *ypb.FuzzerResponse
				var responseChunkCount int
				for {
					got, recvErr := stream.Recv()
					if recvErr != nil {
						break
					}
					if got == nil {
						continue
					}
					responseChunkCount += len(got.RandomChunkedData)
					if bytes.Contains(got.ResponseRaw, []byte(responseBody)) {
						completeResponse = got
					}
				}

				require.NotNil(t, completeResponse, "JSON response must remain in the normal replay path")
				require.Equal(t, int64(len(responseBody)), completeResponse.BodyLength)
				require.Zero(t, responseChunkCount, "JSON response must never be emitted as SSE chunks")
			})
		}
	}
}

func TestGRPCMUSTPASS_HTTPFuzzer_SSE_BurstReplayIsLossless(t *testing.T) {
	const eventCount = 1024
	var expected strings.Builder
	for i := 0; i < eventCount; i++ {
		_, _ = fmt.Fprintf(&expected, "data: event-%04d\n\n", i)
	}
	expectedBody := expected.String()

	serve := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		w.WriteHeader(http.StatusOK)
		flusher, ok := w.(http.Flusher)
		require.True(t, ok)
		for i := 0; i < eventCount; i++ {
			_, _ = fmt.Fprintf(w, "data: event-%04d\n\n", i)
			flusher.Flush()
		}
	}

	ctx := utils.TimeoutContextSeconds(30)
	h1Host, h1Port := utils.DebugMockHTTPHandlerFunc(serve)
	h2Host, h2Port := utils.DebugMockHTTP2HandlerFuncContext(ctx, serve)
	tests := []struct {
		name        string
		host        string
		port        int
		httpVersion string
		isHTTPS     bool
		disablePool bool
	}{
		{name: "http1_pool", host: h1Host, port: h1Port, httpVersion: "HTTP/1.1"},
		{name: "http1_no_pool", host: h1Host, port: h1Port, httpVersion: "HTTP/1.1", disablePool: true},
		{name: "http2_pool", host: h2Host, port: h2Port, httpVersion: "HTTP/2.0", isHTTPS: true},
		{name: "http2_no_pool", host: h2Host, port: h2Port, httpVersion: "HTTP/2.0", isHTTPS: true, disablePool: true},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			client, err := NewLocalClient()
			require.NoError(t, err)
			request := fmt.Sprintf("GET /events %s\r\nHost: %s\r\nAccept: application/json, text/event-stream\r\n\r\n", tt.httpVersion, utils.HostPort(tt.host, tt.port))
			stream, err := client.HTTPFuzzer(ctx, &ypb.FuzzerRequest{
				Request:                  request,
				IsHTTPS:                  tt.isHTTPS,
				DisableUseConnPool:       tt.disablePool,
				PerRequestTimeoutSeconds: 8,
				DialTimeoutSeconds:       2,
				ForceFuzz:                true,
			})
			require.NoError(t, err)

			var replayed bytes.Buffer
			var streamID string
			var finalMarkers int
			for {
				got, recvErr := stream.Recv()
				if recvErr != nil {
					break
				}
				if got == nil || len(got.RandomChunkedData) == 0 {
					continue
				}
				if streamID == "" {
					streamID = got.UUID
				} else {
					require.Equal(t, streamID, got.UUID, "all replay updates must share one stream UUID")
				}
				for _, chunk := range got.RandomChunkedData {
					if chunk == nil {
						continue
					}
					require.NotEqual(t, ypb.ChunkedDataDirection_CHUNKED_DATA_DIRECTION_REQUEST, chunk.Direction)
					if chunk.IsFinal {
						finalMarkers++
						continue
					}
					replayed.Write(chunk.Data)
				}
			}

			require.NotEmpty(t, streamID)
			require.Equal(t, 1, finalMarkers, "a stream must terminate exactly once")
			require.Equal(t, expectedBody, replayed.String(), "SSE replay must not drop or reorder body bytes")
		})
	}
}
