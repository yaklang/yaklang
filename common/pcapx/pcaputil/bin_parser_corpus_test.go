package pcaputil

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/shirou/gopsutil/v4/process"
	"github.com/stretchr/testify/require"
)

var binReplayCorpus = []string{
	"ndpi/ndpi-http.pcapng", "ndpi/ndpi-tls.pcap", "ndpi/ndpi-mqtt.pcap",
	"ndpi/ndpi-dns.pcap", "ndpi/ndpi-cassandra.pcap", "ndpi/ndpi-http-connect.pcap",
	"wireshark-tests/wireshark-http.pcap", "google-samples/google-http-ascii.pcap",
}

func binCorpusBytes(t testing.TB, name string) []byte {
	t.Helper()
	w, err := os.ReadFile(filepath.Join("..", "..", "bin-parser", "testdata", "protocol-corpus", "captures", filepath.FromSlash(name)))
	require.NoError(t, err)
	return w
}

func TestBinParserCorpusReplay(t *testing.T) {
	var decoded, useful, delivered uint64
	for _, name := range binReplayCorpus {
		var mu sync.Mutex
		status := make(map[string]int)
		var samples []string
		var stats BinParserStats
		err := ReplayPcap(bytes.NewReader(binCorpusBytes(t, name)), WithTCPReassemblyWorkers(1), WithBinParser(func(e *BinParserEvent) {
			mu.Lock()
			defer mu.Unlock()
			status[e.Protocol+"/"+e.Status]++
			if e.Status != "decoded" && len(samples) < 3 {
				samples = append(samples, e.Summary+": "+e.Error)
			}
		}), WithBinParserStats(func(s BinParserStats) { stats = s }))
		require.Zero(t, stats.BufferedBytes, "%s", name)
		decoded += stats.Decoded
		useful += stats.MessageBytes
		delivered += stats.InputBytes
		report := map[string]any{"file": name, "stats": stats, "statuses": status, "examples": samples}
		if err != nil {
			report["capture_error"] = err.Error()
		}
		encoded, err := json.Marshal(report)
		require.NoError(t, err)
		t.Log(string(encoded))
	}
	require.EqualValues(t, 108, decoded, "full message coverage changed")
	require.EqualValues(t, 78033, useful, "decoded byte coverage changed")
	require.EqualValues(t, 81028, delivered, "delivered byte coverage changed")
}

func BenchmarkPcapBinParserCorpus(b *testing.B) {
	var inputs [][]byte
	for _, name := range binReplayCorpus {
		inputs = append(inputs, binCorpusBytes(b, name))
	}
	for _, workers := range []int{1, 2, 4} {
		b.Run(fmt.Sprintf("workers=%d", workers), func(b *testing.B) {
			old := runtime.GOMAXPROCS(workers)
			defer runtime.GOMAXPROCS(old)
			var expected uint64
			run := func() (uint64, uint64) {
				var useful atomic.Uint64
				var input uint64
				for i, wire := range inputs {
					err := ReplayPcap(bytes.NewReader(wire), WithTCPReassemblyWorkers(workers), WithBinParser(func(e *BinParserEvent) {
						if e.Status == "decoded" {
							useful.Add(uint64(e.Length))
						}
					}), WithBinParserStats(func(s BinParserStats) { input += s.InputBytes }))
					knownGap := binReplayCorpus[i] == "ndpi/ndpi-tls.pcap" || binReplayCorpus[i] == "ndpi/ndpi-http-connect.pcap"
					if err != nil && !(knownGap && (strings.Contains(err.Error(), "unfilled sequence gap") || strings.Contains(err.Error(), "TCP SYN changes an established initial sequence number"))) {
						b.Fatalf("%s: %v", binReplayCorpus[i], err)
					}
				}
				return useful.Load(), input
			}
			expected, input := run()
			require.EqualValues(b, 78033, expected, "must not gain throughput by dropping complete messages")
			require.EqualValues(b, 81028, input, "delivered corpus bytes changed")
			proc, err := process.NewProcess(int32(os.Getpid()))
			require.NoError(b, err)
			cpu, err := proc.Times()
			require.NoError(b, err)
			b.SetBytes(int64(expected))
			b.ReportAllocs()
			b.ResetTimer()
			for n := 0; n < b.N; n++ {
				got, _ := run()
				if got != expected {
					b.Fatalf("inconsistent decoded byte count: %d != %d", got, expected)
				}
			}
			b.StopTimer()
			end, err := proc.Times()
			require.NoError(b, err)
			b.ReportMetric((end.User+end.System-cpu.User-cpu.System)*1e3/float64(b.N), "cpu-ms/batch")
			b.ReportMetric(float64(input)*float64(b.N)*8/b.Elapsed().Seconds()/1e6, "input-Mbps")
			b.ReportMetric(float64(expected)/float64(input)*100, "decoded-pct")
			b.ReportMetric(2, "known-error-files/batch")
		})
	}
}
