package pcaputil

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync/atomic"
	"testing"
)

func reviewSession(n int) map[string]any {
	headers := make([]map[string]any, n)
	for i := range headers {
		headers[i] = map[string]any{"Name": fmt.Sprintf("x-header-%d", i), "Value": "some session header value", "Sensitive": false}
	}
	return map[string]any{"Headers": headers, "Protocol": "http2", "Stream": uint32(1)}
}

func BenchmarkProtocolReview(b *testing.B) {
	for _, budget := range []int{1 << 20, 128} {
		b.Run(fmt.Sprintf("history/budget=%d", budget), func(b *testing.B) {
			view, _ := NewBinParserInspector(128, budget)
			event := &BinParserEvent{Raw: bytes.Repeat([]byte{'x'}, 1024), Session: reviewSession(64)}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				view.OnEvent(event)
			}
		})
	}
	for _, chunked := range []bool{false, true} {
		b.Run(fmt.Sprintf("http-fragmented/chunked=%v", chunked), func(b *testing.B) {
			wire := []byte("POST / HTTP/1.1\r\nHost: example.test\r\nX-Long: " + string(bytes.Repeat([]byte{'x'}, 16000)) + "\r\nContent-Length: 0\r\n\r\n")
			if chunked {
				wire = []byte("POST / HTTP/1.1\r\nHost: example.test\r\nTransfer-Encoding: chunked\r\n\r\n0\r\nX-Long: " + string(bytes.Repeat([]byte{'x'}, 16000)) + "\r\n\r\n")
			}
			b.ReportAllocs()
			b.SetBytes(int64(len(wire)))
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				f := binFlow{protocol: "http", a: &binParser{config: BinParserConfig{MaxMessageBytes: 1 << 20}}}
				for end := 1; end <= len(wire); end++ {
					n, _, err := f.frameDirection(0, wire[:end])
					if err != nil {
						b.Fatal(err)
					}
					if n > 0 && n != len(wire) {
						b.Fatal(n)
					}
				}
			}
		})
	}
}

// Use -cpu=1,2,4,8 to vary scheduler capacity independently from TCP workers.
// These are finite replay batches including worker setup/drain, not live pps.
func BenchmarkProtocolReviewScheduler(b *testing.B) {
	single, _, singleCount := binBenchmarkCaptureFlows(b, false, 1)
	dns := binCorpusBytes(b, "ndpi/ndpi-dns.pcap")
	for _, tc := range []struct {
		name string
		wire []byte
		want uint64
	}{{"single-tcp", single, singleCount}, {"dns-udp", dns, 0}} {
		for _, workers := range []int{1, 2, 4} {
			b.Run(fmt.Sprintf("%s/workers=%d", tc.name, workers), func(b *testing.B) {
				var expected uint64
				run := func() uint64 {
					var count atomic.Uint64
					err := ReplayPcap(bytes.NewReader(tc.wire), WithTCPReassemblyWorkers(workers), WithBinParser(func(e *BinParserEvent) {
						if e.Status != "decoded" || e.Structured == nil {
							panic(fmt.Sprintf("%s: %s", e.Status, e.Error))
						}
						count.Add(1)
					}))
					if err != nil {
						b.Fatal(err)
					}
					return count.Load()
				}
				expected = run()
				if expected == 0 || tc.want > 0 && expected != tc.want {
					b.Fatalf("messages=%d want=%d", expected, tc.want)
				}
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					if got := run(); got != expected {
						b.Fatal(got)
					}
				}
				b.StopTimer()
				b.ReportMetric(float64(runtime.GOMAXPROCS(0)), "gomaxprocs")
				b.ReportMetric(float64(expected), "messages/batch")
				b.ReportMetric(float64(b.Elapsed().Nanoseconds())/float64(b.N)/float64(expected), "ns/message")
			})
		}
	}
}

// Full fields plus a newly created, flushed, closed pcap on each finite replay.
// File removal is included; no fsync or physical-device throughput is implied.
func BenchmarkProtocolReviewRecording(b *testing.B) {
	wire, payload, want := binBenchmarkCapture(b, true)
	file := filepath.Join(b.TempDir(), "recording.pcap")
	b.ReportAllocs()
	b.SetBytes(payload)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var count atomic.Uint64
		err := ReplayPcap(bytes.NewReader(wire), WithTCPReassemblyWorkers(1), WithOutputFile(file), WithBinParser(func(e *BinParserEvent) {
			if e.Status != "decoded" || e.Structured == nil {
				panic(e.Status)
			}
			count.Add(1)
		}))
		if err != nil {
			b.Fatal(err)
		}
		if count.Load() != want {
			b.Fatal(count.Load())
		}
		if err := os.Remove(file); err != nil {
			b.Fatal(err)
		}
	}
}
