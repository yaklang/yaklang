package yakgrpc

import (
	"context"
	"fmt"
	"net/http"
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
		// Incremental updates are expected; final response is also returned.
		PerRequestTimeoutSeconds: 1.8,
		DialTimeoutSeconds:       1.0,
		ForceFuzz:                true,
	})
	require.NoError(t, err)

	var gotSSE int
	var last *ypb.FuzzerResponse
	var firstUUID string
	for {
		rsp, err := stream.Recv()
		if err != nil {
			break
		}
		if rsp == nil || len(rsp.ResponseRaw) == 0 {
			continue
		}
		if utils.MatchAnyOfSubString(string(rsp.ResponseRaw), "data: msg0", "data: msg1", "data: msg2", "data: msg3") {
			gotSSE++
			last = rsp
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
	for {
		rsp, err := stream.Recv()
		if err != nil {
			break
		}
		if rsp == nil || len(rsp.ResponseRaw) == 0 {
			continue
		}
		if utils.MatchAnyOfSubString(string(rsp.ResponseRaw), "data: msg0", "data: msg1", "data: msg2", "data: msg3") {
			gotSSE++
			if firstUUID == "" {
				firstUUID = rsp.UUID
			} else {
				require.Equal(t, firstUUID, rsp.UUID, "sse updates should share the same UUID")
			}
		}
	}
	require.GreaterOrEqual(t, gotSSE, 2, "should receive incremental SSE updates without Accept header")
}
