package pcaputil

import (
	"testing"
	"time"
)

func BenchmarkTop30AdmittedRedis(b *testing.B) {
	s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("tcp"), WithSessionClientDirection(0))
	if err != nil {
		b.Fatal(err)
	}
	request, response := []byte("*1\r\n$4\r\nPING\r\n"), []byte("+PONG\r\n")
	ts := time.Unix(1, 0)
	s.Feed(0, ts, request)
	s.Feed(1, ts, response)
	probes := s.Stats().ProbeCalls
	b.SetBytes(int64(len(request) + len(response)))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if r := s.Feed(0, ts, request); r.Err != nil {
			b.Fatal(r.Err)
		}
		if r := s.Feed(1, ts, response); r.Err != nil {
			b.Fatal(r.Err)
		}
	}
	b.StopTimer()
	b.ReportMetric(float64(s.Stats().ProbeCalls-probes)/float64(b.N), "probes/op")
	s.Close("benchmark end")
	if s.Stats().BufferedBytes != 0 {
		b.Fatal("retained session state")
	}
}
