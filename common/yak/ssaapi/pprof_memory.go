package ssaapi

import (
	"encoding/json"
	"net/http"
	"runtime"
	"time"
)

// memorySnapshot is a lightweight read for the 5-second external sampler.
// The 5-minute runtime-stats file cannot explain a short available-memory
// drop on its own, and a full heap profile is too expensive to take that
// often. Every byte here is a MemStats counter or an exact runtime.MemStats
// field, so the sampler can tell live heap, idle spans retained by the
// runtime, and memory already returned to the OS apart from VmRSS.
type memorySnapshot struct {
	Timestamp         string `json:"timestamp"`
	HeapAllocBytes    uint64 `json:"process_heap_alloc_bytes"`
	HeapSysBytes      uint64 `json:"process_heap_sys_bytes"`
	HeapIdleBytes     uint64 `json:"process_heap_idle_bytes"`
	HeapInuseBytes    uint64 `json:"process_heap_inuse_bytes"`
	HeapReleasedBytes uint64 `json:"process_heap_released_bytes"`
	HeapObjects       uint64 `json:"process_heap_objects"`
	StackInuseBytes   uint64 `json:"process_stack_inuse_bytes"`
	MCacheInuseBytes  uint64 `json:"process_mcache_inuse_bytes"`
	NextGCBytes       uint64 `json:"process_next_gc_bytes"`
	Goroutines        int    `json:"goroutines"`
}

func captureMemorySnapshot() memorySnapshot {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return memorySnapshot{
		Timestamp:         time.Now().Format(time.RFC3339Nano),
		HeapAllocBytes:    m.HeapAlloc,
		HeapSysBytes:      m.HeapSys,
		HeapIdleBytes:     m.HeapIdle,
		HeapInuseBytes:    m.HeapInuse,
		HeapReleasedBytes: m.HeapReleased,
		HeapObjects:       m.HeapObjects,
		StackInuseBytes:   m.StackInuse,
		MCacheInuseBytes:  m.MCacheInuse,
		NextGCBytes:       m.NextGC,
		Goroutines:        runtime.NumGoroutine(),
	}
}

const pprofMemoryPath = "/debug/ssa/memory"

// sharedPprofHandler serves the standard net/http/pprof endpoints and
// /debug/ssa/memory, which the external sampler polls without taking a heap
// profile. Registering it on the package DefaultServeMux would leak the route
// into every process that imports ssaapi, so the SSA listener owns it.
func sharedPprofHandler() http.Handler {
	mux := http.NewServeMux()
	mux.Handle(pprofMemoryPath, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(captureMemorySnapshot())
	}))
	mux.Handle("/debug/pprof/", http.DefaultServeMux)
	return mux
}
