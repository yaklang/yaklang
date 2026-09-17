package bin_parser

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/bin-parser/parser"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
)

type currentCorpusWork struct {
	id, rule, entry string
	wire            []byte
	rejection       string
}

func currentCorpusWorks(tb testing.TB, application bool) []currentCorpusWork {
	tb.Helper()
	raw, e := os.ReadFile("testdata/protocol-corpus/manifest.json")
	if e != nil {
		tb.Fatal(e)
	}
	var manifest protocolCorpusManifest
	if e = json.Unmarshal(raw, &manifest); e != nil {
		tb.Fatal(e)
	}
	var works []currentCorpusWork
	for _, capture := range manifest.Captures {
		if application && capture.ID != "ndpi-memcached" && capture.ID != "gen-memcache-bin" && capture.ID != "pr5023-gen-memcache-bin" && capture.ID != "ndpi-cassandra" {
			continue
		}
		raw, e := os.ReadFile(filepath.Join("testdata/protocol-corpus", capture.CaptureFile))
		if e != nil {
			tb.Fatal(e)
		}
		if fmt.Sprintf("%x", sha256.Sum256(raw)) != capture.SHA256 {
			tb.Fatalf("digest mismatch %s", capture.ID)
		}
		reader, e := protocolCorpusEnvelopePacketReader(raw)
		if e != nil {
			tb.Fatal(e)
		}
		var container []byte
		if capture.ID == "tcpdump-unsupported-linktype" {
			container, e = protocolCorpusClassicPcapSingleRecord(raw)
			if e != nil {
				tb.Fatal(e)
			}
		}
		frame := 0
		for {
			wire, ci, e := reader.ReadPacketData()
			if e == io.EOF {
				break
			}
			if e != nil {
				tb.Fatal(e)
			}
			frame++
			id := fmt.Sprintf("%s/%d", capture.ID, frame)
			if application {
				entry, rule, offset := "", "application-layer.memcached_fields", 66
				switch capture.ID {
				case "ndpi-cassandra":
					entry = cassandraFieldsTestEntries[frame]
					rule = cassandraFieldsTestRule
				case "ndpi-memcached":
					if frame == 4 {
						entry = "MemcachedStatsRequestFields"
					}
					if frame == 6 {
						entry = "MemcachedStatsResponseFields"
					}
				default:
					offset = 54
					if frame == 4 {
						entry = "MemcachedBinaryGetRequestFields"
					}
				}
				if entry != "" {
					works = append(works, currentCorpusWork{id: id, rule: rule, entry: entry, wire: bytes.Clone(wire[offset:])})
				}
			} else {
				spec, e := protocolCorpusPacketEnvelopeSpec(capture, frame, wire, ci.CaptureLength, ci.Length, container)
				if e != nil {
					tb.Fatal(e)
				}
				works = append(works, currentCorpusWork{id: id, rule: "packet_envelope", entry: spec.entry, wire: bytes.Clone(spec.input), rejection: protocolCorpusEnvelopeRejectionContracts[capture.ID].errorContains})
			}
		}
		if frame != capture.PacketCount {
			tb.Fatalf("count mismatch %s", capture.ID)
		}
	}
	expected := 58533
	if application {
		expected = 11
	}
	if len(works) != expected {
		tb.Fatalf("workload changed: %d != %d; review benchmark manifest", len(works), expected)
	}
	return works
}

func currentCorpusParse(w currentCorpusWork, export bool) (int, error) {
	return currentCorpusParseWithProjection(w, export, NodeToMap)
}

func currentCorpusParseWithProjection(w currentCorpusWork, export bool, project func(*base.Node) any) (int, error) {
	reader := newProtocolCorpusBoundedReader(w.wire)
	n, e := parser.ParseBinary(reader, w.rule, w.entry)
	if w.rejection != "" {
		if e == nil || !strings.Contains(e.Error(), w.rejection) {
			return 0, fmt.Errorf("expected rejection %s: %v", w.id, e)
		}
		return 0, nil
	}
	if e != nil {
		return 0, fmt.Errorf("%s: %w", w.id, e)
	}
	if reader.Len() != 0 {
		return 0, fmt.Errorf("%s: unread bytes", w.id)
	}
	if !export {
		return 0, nil
	}
	// Cost of in-process structured evidence plus JSON serialization only.
	// No disk, RPC, database, model request or UI rendering is timed here.
	data, e := json.Marshal(map[string]any{"fields": project(n), "metadata": n.Cfg.GetItem("additionInfo")})
	return len(data), e
}

func currentCorpusJSON(n *base.Node) ([]byte, error) {
	return json.Marshal(map[string]any{"fields": NodeToMap(n), "metadata": n.Cfg.GetItem("additionInfo")})
}

func benchmarkCurrentCorpus(b *testing.B, application, export bool) {
	benchmarkCurrentCorpusWithProjection(b, application, export, NodeToMap)
}

func benchmarkCurrentCorpusWithProjection(b *testing.B, application, export bool, project func(*base.Node) any) {
	works := currentCorpusWorks(b, application)
	var size int64
	warmed := map[string]bool{}
	for _, w := range works {
		size += int64(len(w.wire))
		key := w.rule + "/" + w.entry
		if !warmed[key] {
			if _, e := currentCorpusParseWithProjection(w, export, project); e != nil {
				b.Fatal(e)
			}
			warmed[key] = true
		}
	}
	workers := runtime.GOMAXPROCS(0)
	if workers > len(works) {
		workers = len(works)
	}
	b.ReportAllocs()
	b.SetBytes(size)
	b.ResetTimer()
	for round := 0; round < b.N; round++ {
		var wg sync.WaitGroup
		errs := make(chan error, workers)
		for worker := 0; worker < workers; worker++ {
			wg.Add(1)
			go func(worker int) {
				defer wg.Done()
				for i := worker; i < len(works); i += workers {
					if _, e := currentCorpusParseWithProjection(works[i], export, project); e != nil {
						errs <- e
						return
					}
				}
			}(worker)
		}
		wg.Wait()
		close(errs)
		for e := range errs {
			b.Fatal(e)
		}
	}
	b.StopTimer()
	b.ReportMetric(float64(b.N*len(works))/b.Elapsed().Seconds(), "messages/s")
	b.ReportMetric(float64(len(works)), "messages/op")
	b.ReportMetric(float64(size), "input-B/op")
	b.ReportMetric(float64(workers), "workers")
	// B/op and allocs/op describe a complete workload round, not one message.
}
func BenchmarkCurrentCorpusEnvelopes(b *testing.B) { benchmarkCurrentCorpus(b, false, false) }
func BenchmarkCurrentCorpusApplicationFields(b *testing.B) {
	benchmarkCurrentCorpus(b, true, false)
}
func BenchmarkCurrentCorpusApplicationJSON(b *testing.B) {
	benchmarkCurrentCorpus(b, true, true)
}

// Deliberate opt-in assessment, separate from correctness tests/full regressions.
// Sequential observations include runtime/GC jitter; nearest-rank percentiles.
// Heap samples every 32 messages are not an exact peak RSS measurement.
func TestCurrentCorpusApplicationLatency(t *testing.T) {
	if os.Getenv("BINPARSER_EVALUATE") != "1" {
		t.Skip("manual current-corpus assessment")
	}
	works := currentCorpusWorks(t, true)
	for _, export := range []bool{false, true} {
		for _, w := range works {
			if _, e := currentCorpusParse(w, export); e != nil {
				t.Fatal(e)
			}
		}
		runtime.GC()
		var before, after runtime.MemStats
		runtime.ReadMemStats(&before)
		heapSampleMax := before.HeapAlloc
		var elapsed []time.Duration
		byEntry := map[string][]time.Duration{}
		var inputBytes, outputBytes int64
		started := time.Now()
		for round := 0; round < 200; round++ {
			for _, w := range works {
				start := time.Now()
				n, e := currentCorpusParse(w, export)
				duration := time.Since(start)
				if e != nil {
					t.Fatal(e)
				}
				elapsed = append(elapsed, duration)
				byEntry[w.entry] = append(byEntry[w.entry], duration)
				inputBytes += int64(len(w.wire))
				outputBytes += int64(n)
				if len(elapsed)%32 == 0 {
					var m runtime.MemStats
					runtime.ReadMemStats(&m)
					if m.HeapAlloc > heapSampleMax {
						heapSampleMax = m.HeapAlloc
					}
				}
			}
		}
		wall := time.Since(started)
		runtime.ReadMemStats(&after)
		percentiles := func(v []time.Duration) map[string]float64 {
			sort.Slice(v, func(i, j int) bool { return v[i] < v[j] })
			result := map[string]float64{}
			for _, p := range []int{50, 95, 99, 100} {
				ix := (len(v)*p+99)/100 - 1
				result[fmt.Sprintf("p%d_us", p)] = float64(v[ix]) / 1000
			}
			return result
		}
		entries := map[string]any{}
		for entry, v := range byEntry {
			entries[entry] = percentiles(v)
		}
		result := map[string]any{"json_export": export, "messages": len(elapsed), "input_bytes": inputBytes, "output_bytes": outputBytes, "seconds": wall.Seconds(), "messages_per_second": float64(len(elapsed)) / wall.Seconds(), "input_mbps": float64(inputBytes) * 8 / wall.Seconds() / 1e6, "latency": percentiles(elapsed), "by_entry": entries, "allocated_bytes": after.TotalAlloc - before.TotalAlloc, "allocations": after.Mallocs - before.Mallocs, "gc_count": after.NumGC - before.NumGC, "gc_pause_ns": after.PauseTotalNs - before.PauseTotalNs, "sampled_heap_max_bytes": heapSampleMax}
		line, e := json.Marshal(result)
		if e != nil {
			t.Fatal(e)
		}
		t.Logf("CURRENT_CORPUS_METRICS %s", line)
	}
}
