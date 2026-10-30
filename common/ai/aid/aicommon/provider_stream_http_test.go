package aicommon

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/lowhttp/poc"
)

func TestProviderStreamHTTPTruncationBounded(t *testing.T) {
	for _, recoverStream := range []bool{false, true} {
		t.Run(fmt.Sprint(recoverStream), func(t *testing.T) {
			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				n := requests.Add(1)
				if recoverStream && n > 1 {
					w.Header().Set("Content-Type", "text/event-stream")
					fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"COMPLETE\"}}]}\n\ndata: [DONE]\n\n")
					return
				}
				conn, _, err := w.(http.Hijacker).Hijack()
				if err != nil {
					return
				}
				defer conn.Close()
				// Advertise a longer response, then close mid-body as a disconnected provider does.
				fmt.Fprint(conn, "HTTP/1.1 200 OK\r\nContent-Type: text/event-stream\r\nTransfer-Encoding: chunked\r\nConnection: close\r\n\r\n1000\r\ndata: {\"choices\":[{\"delta\":{\"content\":\"PARTIAL\"}}]}\n\n")
			}))
			defer server.Close()
			host, portString, err := net.SplitHostPort(server.Listener.Addr().String())
			require.NoError(t, err)
			port, err := strconv.Atoi(portString)
			require.NoError(t, err)
			callback := newChatBaseHTTPCallAI(host, port, true, func() []poc.PocConfigOption {
				return []poc.PocConfigOption{poc.WithTimeout(2), poc.WithConnPool(false), poc.WithNoBodyBuffer(true)}
			})
			cfg, captured := newHTTPFailureEmitHarness(t, callback)
			cfg.retryMax = 3
			cfg.retryWait = func(context.Context, time.Duration) error { return nil }
			err = CallAITransaction(cfg, "stream truncation", cfg.CallAI, func(rsp *AIResponse) error {
				data, readErr := io.ReadAll(rsp.GetOutputStreamReader("test", false, cfg.GetEmitter()))
				if readErr != nil {
					return readErr
				}
				if !strings.Contains(string(data), "COMPLETE") {
					return fmt.Errorf("incomplete action")
				}
				return nil
			})
			if recoverStream {
				require.NoError(t, err)
				require.EqualValues(t, 2, requests.Load())
			} else {
				require.ErrorContains(t, err, "unexpected EOF")
				require.EqualValues(t, 3, requests.Load())
				payload := assertSingleStructuredFailure(t, *captured)
				require.Contains(t, strings.ToLower(failureText(payload)), "unexpected eof")
			}
		})
	}
}

func TestProviderStreamHTTPChunked429Cause(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(429)
		w.(http.Flusher).Flush()
		fmt.Fprint(w, `{"error":{"message":"daily quota exhausted"}}`)
	}))
	defer server.Close()
	host, portString, err := net.SplitHostPort(server.Listener.Addr().String())
	require.NoError(t, err)
	port, err := strconv.Atoi(portString)
	require.NoError(t, err)
	for _, stream := range []bool{false, true} {
		t.Run(fmt.Sprint(stream), func(t *testing.T) {
			cb := newChatBaseHTTPCallAI(host, port, stream, func() []poc.PocConfigOption {
				return []poc.PocConfigOption{poc.WithTimeout(2), poc.WithConnPool(false), poc.WithNoBodyBuffer(true)}
			})
			cfg, _ := newHTTPFailureEmitHarness(t, cb)
			cfg.retryMax = 1
			err := CallAITransaction(cfg, "quota", cfg.CallAI, func(*AIResponse) error { t.Fatal("must not parse quota"); return nil })
			require.ErrorContains(t, err, "daily quota exhausted")
		})
	}
}
