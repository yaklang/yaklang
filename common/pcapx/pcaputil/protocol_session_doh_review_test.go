package pcaputil

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestDoHHTTP1MixedPipeline(t *testing.T) {
	for _, deferred := range []bool{false, true} {
		for _, chunk := range []int{0, 1, 7} {
			t.Run(fmt.Sprintf("deferred=%v/chunk=%d", deferred, chunk), func(t *testing.T) {
				steps := []sessionStep{
					{0, dohGET("example.test", "example.com", 42)},
					{0, []byte("HEAD /index.html HTTP/1.1\r\nHost: example.test\r\n\r\n")},
					{0, []byte("GET /index.html HTTP/1.1\r\nHost: example.test\r\n\r\n")},
					{1, []byte("HTTP/1.1 103 Early Hints\r\nLink: </style.css>; rel=preload\r\n\r\n")},
					{1, dohHTTPResp(200, dnsWire(dnsAResponse(42, "example.com", [4]byte{1, 2, 3, 4})))},
					{1, []byte("HTTP/1.1 200 OK\r\nContent-Type: text/html\r\nContent-Length: 100\r\n\r\n")},
					{1, []byte("HTTP/1.1 200 OK\r\nContent-Type: text/plain\r\nContent-Length: 2\r\n\r\nok")},
				}
				events, _ := sessionTestFlow(t, "http", steps, chunk, deferred)
				require.Len(t, events, 7)
				want := []string{"doh", "http", "http", "http", "doh", "http", "http"}
				for i, e := range events {
					require.Empty(t, e.Error, "event %d", i)
					require.Equal(t, want[i], e.Protocol, "event %d", i)
				}
			})
		}
	}
}

func BenchmarkHTTP1OrdinaryDeferred(b *testing.B) {
	c := NewDefaultConfig()
	require.NoError(b, WithBinParserConfig(BinParserConfig{Deferred: true, OnEvent: func(*ProtocolEvent) {}})(c))
	require.NoError(b, c.prepareBinParser())
	req := []byte("GET /index.html HTTP/1.1\r\nHost: example.test\r\n\r\n")
	resp := []byte("HTTP/1.1 200 OK\r\nContent-Length: 2\r\n\r\nok")
	ts := time.Unix(1, 0)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		f := &binFlow{a: c.binParser, id: uint64(i + 1)}
		f.feed(0, req, ts)
		f.feed(1, resp, ts)
		f.close(TrafficFlowCloseReason_FIN)
	}
}
