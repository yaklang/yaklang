package bin_parser

import (
	"fmt"
	"os"
	"runtime"
	"sync"
	"testing"

	"github.com/shirou/gopsutil/v4/process"
	"github.com/yaklang/yaklang/common/bin-parser/parser"
)

// Keep the application corpus, warmup, and per-batch scheduler identical to
// BenchmarkCurrentCorpusApplicationJSON. Only serialization is omitted: each
// message still produces independently owned fields and the original metadata.
func BenchmarkCurrentCorpusApplicationStructured(b *testing.B) {
	benchmarkCurrentCorpusStructured(b, false, 0)
}

// Same messages, complete output and per-batch scheduling as the Node path.
// Only the public parsing entry point changes; no decoded result is cached.
func BenchmarkCurrentCorpusApplicationStructuredDirect(b *testing.B) {
	benchmarkCurrentCorpusStructured(b, true, 0)
}

// Synthetic plan-table cardinality only: every alias dispatches the same
// existing decoder. This is not protocol detection or 600 real decoders. The
// table is exercised across all names, not only a permanently hot entry.
func BenchmarkCurrentCorpusStructuredRegistry(b *testing.B) {
	for _, size := range []int{10, 300, 600} {
		b.Run(fmt.Sprintf("entries=%d", size), func(b *testing.B) {
			benchmarkCurrentCorpusStructured(b, true, size)
		})
	}
}

func benchmarkCurrentCorpusStructured(b *testing.B, direct bool, registrySize int) {
	works := currentCorpusWorks(b, true)
	parse := func(w currentCorpusWork) (map[string]any, error) {
		if direct {
			value, err := ParseStructured(w.wire, w.rule, w.entry)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", w.id, err)
			}
			return value, nil
		}
		reader := newProtocolCorpusBoundedReader(w.wire)
		n, err := parser.ParseBinary(reader, w.rule, w.entry)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", w.id, err)
		}
		if reader.Len() != 0 {
			return nil, fmt.Errorf("%s: unread bytes", w.id)
		}
		return map[string]any{"fields": NodeToMap(n), "metadata": n.Cfg.GetItem("additionInfo")}, nil
	}
	var registry map[string]func(currentCorpusWork) (map[string]any, error)
	var registryKeys []string
	if registrySize > 0 {
		registry = make(map[string]func(currentCorpusWork) (map[string]any, error), registrySize)
		for i := 0; i < registrySize; i++ {
			key := fmt.Sprintf("synthetic-plan-%04d", i)
			registryKeys = append(registryKeys, key)
			registry[key] = parse
		}
	}
	var size int64
	warmed := map[string]bool{}
	for _, w := range works {
		size += int64(len(w.wire))
		key := w.rule + "/" + w.entry
		if !warmed[key] {
			if _, err := parse(w); err != nil {
				b.Fatal(err)
			}
			warmed[key] = true
		}
	}
	workers := runtime.GOMAXPROCS(0)
	if workers > len(works) {
		workers = len(works)
	}
	// Keep a result per worker alive across batches. Avoid a shared per-message
	// sink and ensure the exported object really escapes the parsing function.
	results := make([]map[string]any, workers)
	b.ReportAllocs()
	b.SetBytes(size)
	proc, err := process.NewProcess(int32(os.Getpid()))
	if err != nil {
		b.Fatal(err)
	}
	var heapStart runtime.MemStats
	runtime.ReadMemStats(&heapStart)
	cpuStart, err := proc.Times()
	if err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for round := 0; round < b.N; round++ {
		var wg sync.WaitGroup
		errs := make(chan error, workers)
		for worker := 0; worker < workers; worker++ {
			wg.Add(1)
			go func(worker int) {
				defer wg.Done()
				var result map[string]any
				for i := worker; i < len(works); i += workers {
					var err error
					decode := parse
					if registrySize > 0 {
						decode = registry[registryKeys[(round*len(works)+i)%registrySize]]
					}
					result, err = decode(works[i])
					if err != nil {
						errs <- err
						return
					}
				}
				results[worker] = result
			}(worker)
		}
		wg.Wait()
		close(errs)
		for err := range errs {
			b.Fatal(err)
		}
	}
	b.StopTimer()
	cpuEnd, err := proc.Times()
	if err != nil {
		b.Fatal(err)
	}
	// Sum user and kernel CPU time over all process threads, including GC.
	// Unlike wall time, this exposes extra CPU spent on parallel execution.
	b.ReportMetric((cpuEnd.User+cpuEnd.System-cpuStart.User-cpuStart.System)*1e9/float64(b.N), "cpu-ns/op")
	var heapEnd runtime.MemStats
	runtime.ReadMemStats(&heapEnd)
	b.ReportMetric(float64(heapEnd.NumGC-heapStart.NumGC)/b.Elapsed().Seconds(), "gc-cycles/s")
	b.ReportMetric(float64(heapEnd.HeapSys), "heap-reserved-B")
	if memory, err := proc.MemoryInfo(); err == nil {
		b.ReportMetric(float64(memory.RSS), "end-RSS-B")
	}
	runtime.KeepAlive(results)
	b.ReportMetric(float64(b.N*len(works))/b.Elapsed().Seconds(), "messages/s")
	b.ReportMetric(float64(len(works)), "messages/op")
	b.ReportMetric(float64(size), "input-B/op")
	b.ReportMetric(float64(workers), "workers")
	if registrySize > 0 {
		b.ReportMetric(float64(registrySize), "synthetic-entries")
	}
}
