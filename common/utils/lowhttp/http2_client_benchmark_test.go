package lowhttp

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func BenchmarkH2ClientRoundTrip(b *testing.B) {
	for _, parallel := range []bool{false, true} {
		b.Run(fmt.Sprintf("parallel=%t", parallel), func(b *testing.B) {
			server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = io.Copy(io.Discard, r.Body)
				w.Header().Set("X-Benchmark", "shared-hpack-header")
				_, _ = io.WriteString(w, "ok")
			}))
			server.EnableHTTP2 = true
			server.StartTLS()
			defer server.Close()
			pool := h2PoolFor(context.Background(), time.Minute)
			defer pool.Clear()
			addr := server.Listener.Addr().(*net.TCPAddr)
			packet := []byte(fmt.Sprintf("GET /bench HTTP/2\r\nHost: %s\r\nAccept: application/json\r\nUser-Agent: lowhttp-h2-benchmark\r\n\r\n", addr))
			send := func() {
				_, err := HTTPWithoutRedirect(WithPacketBytes(packet), WithHttps(true), WithHttp2(true), WithHost("127.0.0.1"), WithPort(addr.Port), WithConnPool(true), ConnPool(pool), WithTimeout(5*time.Second))
				if err != nil {
					b.Error(err)
				}
			}
			send()
			b.ReportAllocs()
			b.ResetTimer()
			if parallel {
				b.RunParallel(func(pb *testing.PB) {
					for pb.Next() {
						send()
					}
				})
			} else {
				for i := 0; i < b.N; i++ {
					send()
				}
			}
			b.StopTimer()
		})
	}
}
