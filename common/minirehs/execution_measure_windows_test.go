//go:build windows && cgo

package minirehs

import (
	"fmt"
	"os"
	"runtime"
	"sort"
	"strconv"
	"sync"
	"syscall"
	"testing"
	"unsafe"
)

var executionQPC = syscall.NewLazyDLL("kernel32.dll").NewProc("QueryPerformanceCounter")
var executionQPF = syscall.NewLazyDLL("kernel32.dll").NewProc("QueryPerformanceFrequency")

func executionCounter(proc *syscall.LazyProc, dst *int64) {
	if ok, _, err := proc.Call(uintptr(unsafe.Pointer(dst))); ok == 0 {
		panic(err)
	}
	runtime.KeepAlive(dst)
}

func executionProcessCPU(t *testing.T) uint64 {
	t.Helper()
	var created, exited, kernel, user syscall.Filetime
	if err := syscall.GetProcessTimes(syscall.Handle(^uintptr(0)), &created, &exited, &kernel, &user); err != nil {
		t.Fatal(err)
	}
	ticks := func(f syscall.Filetime) uint64 { return uint64(f.HighDateTime)<<32 | uint64(f.LowDateTime) }
	return (ticks(kernel) + ticks(user)) * 100
}

// Finite closed-loop clients: no extra application queue. The first phase has
// no per-request timers; the second measures service latency including Go/OS
// scheduling with QPC. Go1.22/amd64's shared interrupt-time clock can quantize
// submillisecond requests to zero. These are never production percentiles.
func TestProgramExecutionMeasure(t *testing.T) {
	dir := programEvidenceDir(t)
	readInt := func(name string, fallback int) int {
		if v := os.Getenv(name); v != "" {
			n, err := strconv.Atoi(v)
			if err != nil || n < 1 {
				t.Fatal(name, v, err)
			}
			return n
		}
		return fallback
	}
	procs := readInt("MINIREHS_EXEC_P", 2)
	outer := readInt("MINIREHS_EXEC_Q", 1)
	prior := runtime.GOMAXPROCS(procs)
	defer runtime.GOMAXPROCS(prior)
	set := os.Getenv("MINIREHS_EXEC_CASE")
	var patterns []Pattern
	var records [][]byte
	count := 4096
	switch set {
	case "GetRequestHTTP", "GetRequestDense":
		patterns = getRequestImagePatterns(t)
		name := "HTTP912"
		if set == "GetRequestDense" {
			name, count = "DenseHTTP16K", 1024
		}
		records = [][]byte{programRuntimeInputs()[name]}
	case "MITMCorpus", "MITMShortCorpus", "GetRequestShortCorpus":
		patterns, _ = compilableMITMPatterns(t)
		if set == "GetRequestShortCorpus" {
			patterns = getRequestImagePatterns(t)
		}
		records, _ = loadCorpus(t)
		if len(records) != 1332 {
			t.Fatal("complete MITM corpus required, set MINIREHS_FULL_CORPUS=1")
		}
		if set != "MITMCorpus" {
			var short [][]byte
			for _, record := range records {
				if len(record) < 512 {
					short = append(short, record)
				}
			}
			records = short
			if len(records) != 193 {
				t.Fatal("actual complete short-record stratum changed", len(records))
			}
		}
		count = 2664
	default:
		t.Fatal("set MINIREHS_EXEC_CASE GetRequestHTTP/GetRequestDense/MITMCorpus/MITMShortCorpus/GetRequestShortCorpus")
	}
	count = readInt("MINIREHS_EXEC_N", count)
	if count < outer {
		t.Fatal("need at least one request per client")
	}
	var frequency int64
	executionCounter(executionQPF, &frequency)
	if frequency < 1 {
		t.Fatal("invalid QPC frequency", frequency)
	}
	nanos := func(ticks int64) int64 { return ticks * 1e9 / frequency }
	clockPairs := make([]int64, 10001)
	var c0, c1 int64
	for i := range clockPairs {
		executionCounter(executionQPC, &c0)
		executionCounter(executionQPC, &c1)
		clockPairs[i] = nanos(c1 - c0)
	}
	sort.Slice(clockPairs, func(i, j int) bool { return clockPairs[i] < clockPairs[j] })
	db, err := Compile(patterns, WithBackend(BackendMVS), WithReportLocation(false), WithLogger(silentLogger{}))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	modeName := programSmallWorkerMode(t, db)
	// Warm all source records/confirmers before timing; immutable subjects are
	// reused, and every request still scans its complete original record.
	warm, err := db.NewScratch()
	if err != nil {
		t.Fatal(err)
	}
	for _, record := range records {
		if err := db.Scan(record, warm, nil); err != nil {
			t.Fatal(err)
		}
	}
	warm.Close()
	type phase struct {
		Latencies                            bool
		Requests, Bytes                      int
		WallNS                               int64
		ProcessCPUNS, GoBytes, GoAllocations uint64
		P50ServiceNS, P99ServiceNS           *int64
	}
	var phases []phase
	for _, latency := range []bool{false, true} {
		scratches := make([]Scratch, outer)
		for i := range scratches {
			scratches[i], err = db.NewScratch()
			if err != nil {
				t.Fatal(err)
			}
		}
		var samples []int64
		if latency {
			samples = make([]int64, count)
		}
		var ready, done sync.WaitGroup
		ready.Add(outer)
		done.Add(outer)
		start := make(chan struct{})
		errs := make(chan error, outer)
		for w := 0; w < outer; w++ {
			go func(worker int) {
				defer done.Done()
				if err := db.Scan(records[worker%len(records)], scratches[worker], nil); err != nil {
					errs <- err
					ready.Done()
					return
				}
				ready.Done()
				<-start
				var requestStart, requestEnd int64
				for i := worker; i < count; i += outer {
					if latency {
						executionCounter(executionQPC, &requestStart)
					}
					if err := db.Scan(records[i%len(records)], scratches[worker], nil); err != nil {
						errs <- err
						return
					}
					if latency {
						executionCounter(executionQPC, &requestEnd)
						samples[i] = nanos(requestEnd - requestStart)
					}
				}
			}(w)
		}
		ready.Wait()
		var before, after runtime.MemStats
		runtime.ReadMemStats(&before)
		cpu := executionProcessCPU(t)
		executionCounter(executionQPC, &c0)
		close(start)
		done.Wait()
		executionCounter(executionQPC, &c1)
		cpu = executionProcessCPU(t) - cpu
		runtime.ReadMemStats(&after)
		close(errs)
		for err := range errs {
			t.Fatal(err)
		}
		bytes := 0
		for i := 0; i < count; i++ {
			bytes += len(records[i%len(records)])
		}
		p := phase{Latencies: latency, Requests: count, Bytes: bytes, WallNS: nanos(c1 - c0), ProcessCPUNS: cpu, GoBytes: after.TotalAlloc - before.TotalAlloc, GoAllocations: after.Mallocs - before.Mallocs}
		if latency {
			sort.Slice(samples, func(i, j int) bool { return samples[i] < samples[j] })
			p.P50ServiceNS, p.P99ServiceNS = &samples[(count-1)/2], &samples[(count-1)*99/100]
		}
		phases = append(phases, p)
		for _, sc := range scratches {
			sc.Close()
		}
	}
	writeProgramJSON(t, dir, "execution-measure.json", map[string]interface{}{"case": set, "mode": modeName, "P": procs, "Q": outer, "patterns": len(patterns), "records": len(records), "phases": phases, "go": runtime.Version(), "logical_cpus": runtime.NumCPU(), "qpc_frequency": frequency, "qpc_pair_p50_ns": clockPairs[5000], "qpc_pair_p99_ns": clockPairs[9900], "notes": fmt.Sprintf("Closed-loop Q=%d clients; no extra app queue. Uninstrumented request loop and separate QPC-sampled service-latency phase (clock pair overhead reported, not subtracted). Go1.22 shared interrupt-time clock excluded from percentiles after smoke produced zeros. GetProcessTimes captures whole process CPU incl C;100ns API units, coarser accounting resolution. ConfiguredP is GOMAXPROCS, not proof of OS quota or C-thread hardcap. ShortCorpus is the complete original-order <512B stratum, not truncated records. Affinity/background load uncontrolled; no production weights/PMU/productionP99.", outer)})
}
