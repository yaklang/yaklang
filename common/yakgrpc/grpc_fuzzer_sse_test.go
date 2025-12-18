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

func TestGRPCMUSTPASS_HTTPFuzzer_SSE_StreamUpdates(t *testing.T) {
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

	start := time.Now()
	stream, err := c.HTTPFuzzer(context.Background(), &ypb.FuzzerRequest{
		Request: fmt.Sprintf("GET / HTTP/1.1\r\nHost: %s\r\nAccept: text/event-stream\r\n\r\n", utils.HostPort(host, port)),
		// Give enough time to receive multiple SSE window flushes before timeout ends the request.
		PerRequestTimeoutSeconds: 1.8,
		DialTimeoutSeconds:       1.0,
		ForceFuzz:               true,
	})
	require.NoError(t, err)

	var gotUpdates int
	var firstUpdateAt time.Duration
	for {
		rsp, err := stream.Recv()
		if err != nil {
			break
		}
		if rsp == nil || len(rsp.ResponseRaw) == 0 {
			continue
		}
		if utils.MatchAnyOfSubString(string(rsp.ResponseRaw), "data: msg0", "data: msg1", "data: msg2", "data: msg3") {
			gotUpdates++
			if gotUpdates == 1 {
				firstUpdateAt = time.Since(start)
			}
		}
	}

	require.GreaterOrEqual(t, gotUpdates, 2, "should receive multiple SSE updates before request ends")
	require.Less(t, firstUpdateAt, 900*time.Millisecond, "first SSE update should arrive before request timeout")
}
