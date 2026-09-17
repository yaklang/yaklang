package bin_parser

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/shirou/gopsutil/v4/process"
	"github.com/yaklang/yaklang/common/bin-parser/parser"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
)

// These workloads enter the existing Ethernet -> IP -> TCP/UDP candidate
// dispatch with no application name, entry, frame offset or flow hint. They
// are deliberately separate from envelope-only and known-PDU benchmarks.
// Non-Ethernet captures are explicitly excluded, not credited as decoded.
func corpusDispatchWorks(tb testing.TB, all bool) []currentCorpusWork {
	tb.Helper()
	var manifest protocolCorpusManifest
	data, err := os.ReadFile("testdata/protocol-corpus/manifest.json")
	if err != nil {
		tb.Fatal(err)
	}
	if err = json.Unmarshal(data, &manifest); err != nil {
		tb.Fatal(err)
	}
	selected := make(map[string]bool)
	for _, capture := range manifest.Captures {
		if capture.LinkType == "Ethernet" && capture.RepresentativeFrame != nil {
			selected[fmt.Sprintf("%s/%d", capture.ID, capture.RepresentativeFrame.Number)] = true
		}
	}
	var works []currentCorpusWork
	for _, work := range currentCorpusWorks(tb, false) {
		if work.entry != "Ethernet Envelope" && work.entry != "Truncated Ethernet Record Envelope" {
			continue
		}
		if !all && !selected[work.id] {
			continue
		}
		work.rule, work.entry, work.rejection = "ethernet", "Ethernet", ""
		works = append(works, work)
	}
	if len(works) == 0 {
		tb.Fatal("empty automatic-dispatch corpus")
	}
	return works
}

func dispatchParse(work currentCorpusWork) (result map[string]any, node *base.Node, remaining int, err error) {
	reader := newProtocolCorpusBoundedReader(work.wire)
	node, err = parser.ParseBinary(reader, "ethernet", "Ethernet")
	remaining = reader.Len()
	if err == nil {
		result = map[string]any{"fields": NodeToMap(node), "metadata": node.Cfg.GetItem("additionInfo")}
	}
	return
}

// Opt-in inventory audit. Successful parser return is NOT counted as proof of
// correct protocol identification or complete application semantics. Report
// errors, unread bytes and explicitly named fallback spans independently.
func TestCorpusAutomaticDispatchAudit(t *testing.T) {
	if os.Getenv("BIN_PARSER_DISPATCH_AUDIT") != "1" {
		t.Skip("opt-in automatic-dispatch audit")
	}
	works := corpusDispatchWorks(t, os.Getenv("BIN_PARSER_DISPATCH_ALL") == "1")
	type observation struct {
		ID            string `json:"id"`
		Bytes         int    `json:"bytes"`
		Nanoseconds   int64  `json:"nanoseconds"`
		Remaining     int    `json:"remaining"`
		FallbackBytes int    `json:"explicit_fallback_bytes"`
		Error         string `json:"error,omitempty"`
	}
	rows := make([]observation, 0, len(works))
	for i, work := range works {
		start := time.Now()
		result, node, remaining, err := dispatchParse(work)
		row := observation{ID: work.id, Bytes: len(work.wire), Nanoseconds: time.Since(start).Nanoseconds(), Remaining: remaining}
		if err != nil {
			row.Error = err.Error()
		} else {
			var inspect func(*base.Node)
			inspect = func(n *base.Node) {
				if stream_parser.NodeHasResult(n) && stream_parser.NodeIsTerminal(n) && (n.Name == "Remaining Payload" || n.Name == "Next Protocol Data" || strings.HasPrefix(n.Name, "Unparsed ")) {
					if value, ok := stream_parser.GetResultByNode(n).([]byte); ok {
						row.FallbackBytes += len(value)
					}
				}
				for _, child := range n.Children {
					inspect(child)
				}
			}
			inspect(node)
		}
		runtime.KeepAlive(result)
		rows = append(rows, row)
		if (i+1)%100 == 0 {
			t.Logf("audited %d/%d Ethernet packets", i+1, len(works))
		}
	}
	if path := os.Getenv("BIN_PARSER_DISPATCH_REPORT"); path != "" {
		data, err := json.MarshalIndent(rows, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	var total, fallback, rejected, unread int
	var elapsed int64
	for _, row := range rows {
		total += row.Bytes
		fallback += row.FallbackBytes
		elapsed += row.Nanoseconds
		if row.Error != "" {
			rejected++
		} else if row.Remaining > 0 {
			unread++
		}
	}
	t.Logf("packets=%d bytes=%d parse_errors=%d success_with_unread=%d explicit_fallback_bytes=%d parse_export_seconds=%.6f attempted_mbps=%.3f; includes cold setup, errors and opaque fallback, NOT full decoding throughput", len(rows), total, rejected, unread, fallback, float64(elapsed)/1e9, float64(total)*8000/float64(elapsed))
}

func BenchmarkCorpusAutomaticDispatch(b *testing.B) {
	for _, scenario := range []string{"representatives", "unknown_tcp_128", "unknown_udp_128", "unknown_tcp_1200", "unknown_udp_1200"} {
		b.Run(scenario, func(b *testing.B) {
			var works []currentCorpusWork
			if scenario == "representatives" {
				works = corpusDispatchWorks(b, false)
			} else {
				size := 128
				if strings.HasSuffix(scenario, "1200") {
					size = 1200
				}
				works = []currentCorpusWork{{id: scenario, wire: dispatchUnknownFrame(strings.Contains(scenario, "tcp"), size)}}
			}
			benchmarkDispatchWorks(b, works)
		})
	}
}

func TestCorpusAutomaticDispatchUnknownAudit(t *testing.T) {
	if os.Getenv("BIN_PARSER_DISPATCH_AUDIT") != "1" {
		t.Skip("opt-in automatic-dispatch audit")
	}
	for _, tcp := range []bool{true, false} {
		for _, size := range []int{128, 1200} {
			work := currentCorpusWork{wire: dispatchUnknownFrame(tcp, size)}
			result, node, remaining, err := dispatchParse(work)
			if err != nil {
				t.Logf("tcp=%v payload=%d error=%v", tcp, size, err)
				continue
			}
			var candidates []string
			var inspect func(*base.Node)
			inspect = func(n *base.Node) {
				if n.Name == "TCP" || n.Name == "UDP" {
					for _, child := range n.Children {
						if child.Name == "Payload" {
							for _, candidate := range child.Children {
								candidates = append(candidates, candidate.Name)
							}
						}
					}
				}
				for _, child := range n.Children {
					inspect(child)
				}
			}
			inspect(node)
			t.Logf("tcp=%v payload=%d unread=%d accepted_nodes=%v; arbitrary bytes are not ground-truth protocol messages", tcp, size, remaining, candidates)
			runtime.KeepAlive(result)
		}
	}
}

func benchmarkDispatchWorks(b *testing.B, works []currentCorpusWork) {
	var size int64
	for _, work := range works {
		size += int64(len(work.wire))
		dispatchParse(work)
	}
	workers := min(runtime.GOMAXPROCS(0), len(works))
	results := make([]map[string]any, workers)
	rejected, unread := make([]int, workers), make([]int, workers)
	proc, err := process.NewProcess(int32(os.Getpid()))
	if err != nil {
		b.Fatal(err)
	}
	cpuStart, err := proc.Times()
	if err != nil {
		b.Fatal(err)
	}
	b.SetBytes(size)
	b.ReportAllocs()
	b.ResetTimer()
	for round := 0; round < b.N; round++ {
		var wg sync.WaitGroup
		for worker := 0; worker < workers; worker++ {
			wg.Add(1)
			go func(worker int) {
				defer wg.Done()
				for i := worker; i < len(works); i += workers {
					value, _, left, err := dispatchParse(works[i])
					if err != nil {
						rejected[worker]++
					} else if left != 0 {
						unread[worker]++
					}
					results[worker] = value
				}
			}(worker)
		}
		wg.Wait()
	}
	b.StopTimer()
	cpuEnd, err := proc.Times()
	if err != nil {
		b.Fatal(err)
	}
	var errors, leftovers int
	for i := range rejected {
		errors += rejected[i]
		leftovers += unread[i]
	}
	b.ReportMetric((cpuEnd.User+cpuEnd.System-cpuStart.User-cpuStart.System)*1e9/float64(b.N), "cpu-ns/op")
	b.ReportMetric(float64(errors)/float64(b.N), "parse-errors/op")
	b.ReportMetric(float64(leftovers)/float64(b.N), "unread-packets/op")
	b.ReportMetric(float64(size), "input-B/op")
	b.ReportMetric(float64(len(works)), "packets/op")
	b.ReportMetric(float64(b.N*len(works))/b.Elapsed().Seconds(), "packets/s")
	b.ReportMetric(float64(workers), "workers")
	runtime.KeepAlive(results)
}

// Deterministic unrecognized payload on nonstandard ports. Framing is valid;
// no application identifier or successful expected decode is supplied.
func dispatchUnknownFrame(tcp bool, size int) []byte {
	header := 8
	if tcp {
		header = 20
	}
	frame := make([]byte, 14+20+header+size)
	copy(frame[:12], bytes.Repeat([]byte{0x02}, 12))
	binary.BigEndian.PutUint16(frame[12:14], 0x0800)
	ip := frame[14:34]
	ip[0], ip[8], ip[9] = 0x45, 64, 17
	if tcp {
		ip[9] = 6
	}
	binary.BigEndian.PutUint16(ip[2:4], uint16(len(frame)-14))
	copy(ip[12:], []byte{192, 0, 2, 1, 192, 0, 2, 2})
	transport := frame[34:]
	binary.BigEndian.PutUint16(transport[:2], 49152)
	binary.BigEndian.PutUint16(transport[2:4], 49153)
	if tcp {
		transport[12], transport[13] = 0x50, 0x18
	} else {
		binary.BigEndian.PutUint16(transport[4:6], uint16(len(transport)))
	}
	for i := header; i < len(transport); i++ {
		transport[i] = 0xa5 ^ byte(i*73)
	}
	binary.BigEndian.PutUint16(ip[10:12], dispatchChecksum(ip))
	pseudo := make([]byte, 12+len(transport))
	copy(pseudo, ip[12:20])
	pseudo[9] = ip[9]
	binary.BigEndian.PutUint16(pseudo[10:12], uint16(len(transport)))
	copy(pseudo[12:], transport)
	checksum := dispatchChecksum(pseudo)
	if tcp {
		binary.BigEndian.PutUint16(transport[16:18], checksum)
	} else {
		if checksum == 0 {
			checksum = 0xffff
		}
		binary.BigEndian.PutUint16(transport[6:8], checksum)
	}
	return frame
}

func dispatchChecksum(data []byte) uint16 {
	var sum uint32
	for len(data) >= 2 {
		sum += uint32(binary.BigEndian.Uint16(data))
		data = data[2:]
	}
	if len(data) != 0 {
		sum += uint32(data[0]) << 8
	}
	for sum>>16 != 0 {
		sum = sum&0xffff + sum>>16
	}
	return ^uint16(sum)
}
