package ssaapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	_ "net/http/pprof"
	"os"
	"path/filepath"
	"runtime"
	runtimepprof "runtime/pprof"
	"sync"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/load"
	"github.com/shirou/gopsutil/v4/mem"
	"github.com/shirou/gopsutil/v4/process"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
)

// pprofCollector manages periodic pprof collection during a scan.
// It starts an HTTP pprof server and collects CPU, memory, goroutine, mutex,
// and block profiles at regular intervals.
//
// Collection strategy:
//   - Every minute, collect a 30-second CPU profile
//   - Every 5 minutes, collect heap, goroutine (including debug=2 stacks), mutex,
//     block, DB stats, and runtime stats; early snapshots also capture these during
//     the first 4 minutes to preserve evidence for memory-guarded scans
//   - Collect allocs at startup and every 15 minutes
//   - Shutdown flushes any active CPU sample and collects final non-CPU snapshots
type pprofCollector struct {
	dir             string
	cpuDir          string
	memDir          string
	goroutineDir    string
	mutexDir        string
	blockDir        string
	allocsDir       string
	dbStatsDir      string
	runtimeStatsDir string
	httpAddr        string
	wg              sync.WaitGroup
	lastDBStats     ssadb.DBOpStats
	lastDBStatsAt   time.Time
	dbStatsMu       sync.Mutex
	cpuMu           sync.Mutex
}

const (
	// pprof listens on a random free localhost port (not a fixed 18080) so
	// concurrent debug compiles / leftover servers do not collide.
	pprofCPUInterval          = time.Minute
	pprofCPUDuration          = 30 * time.Second
	pprofInterval             = 5 * time.Minute
	pprofHTTPTimeout          = 10 * time.Minute
	pprofListenAttempts       = 8
	pprofAllocInterval        = 15 * time.Minute
	pprofMutexProfileFraction = 1
	pprofBlockProfileRate     = 1

	// OOM investigations need early evidence. Large source scans have been
	// killed by the kernel around 3-5 minutes, before the first periodic
	// snapshot at 5 minutes; capture intermediate runtime/heap/goroutine state
	// each minute for the first four minutes.
	pprofEarlyInterval = 1 * time.Minute
)

// StartPprofCollector creates the output directories, starts the pprof HTTP server,
// and launches a background goroutine that periodically collects pprof snapshots.
// The returned cleanup function stops the collector, waits for in-progress
// collections to flush on cancellation, and collects a final non-CPU snapshot.
func StartPprofCollector(debugDir string) (func(), error) {
	cpuDir := filepath.Join(debugDir, "cpu-pprof")
	memDir := filepath.Join(debugDir, "memory-pprof")
	goroutineDir := filepath.Join(debugDir, "goroutine-pprof")
	mutexDir := filepath.Join(debugDir, "mutex-pprof")
	blockDir := filepath.Join(debugDir, "block-pprof")
	allocsDir := filepath.Join(debugDir, "allocs-pprof")
	dbStatsDir := filepath.Join(debugDir, "db-stats")
	runtimeStatsDir := filepath.Join(debugDir, "runtime-stats")

	for _, dir := range []string{cpuDir, memDir, goroutineDir, mutexDir, blockDir, allocsDir, dbStatsDir, runtimeStatsDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("create pprof dir %s: %w", dir, err)
		}
	}

	previousMutexFraction := runtime.SetMutexProfileFraction(pprofMutexProfileFraction)
	actualMutexFraction := runtime.SetMutexProfileFraction(-1)
	runtime.SetBlockProfileRate(pprofBlockProfileRate)
	log.Infof("[pprof] runtime profile rates active: MutexProfileFraction=%d BlockProfileRate=%d (previous mutex fraction=%d)",
		actualMutexFraction, pprofBlockProfileRate, previousMutexFraction)

	addr, err := startPprofHTTPServer()
	if err != nil {
		return nil, err
	}

	ssadb.EnsureDBOpCallbacks(ssadb.GetDB())

	ctx, cancel := context.WithCancel(context.Background())
	collector := &pprofCollector{
		dir:             debugDir,
		cpuDir:          cpuDir,
		memDir:          memDir,
		goroutineDir:    goroutineDir,
		mutexDir:        mutexDir,
		blockDir:        blockDir,
		allocsDir:       allocsDir,
		dbStatsDir:      dbStatsDir,
		runtimeStatsDir: runtimeStatsDir,
		httpAddr:        addr,
		lastDBStats:     ssadb.SnapshotDBOpStats(),
		lastDBStatsAt:   time.Now(),
	}

	collector.wg.Add(1)
	go collector.collectLoop(ctx)

	cleanup := func() {
		cancel()
		collector.wg.Wait()
		// Active CPU samples have been flushed. Sampling an idle process here
		// only adds latency and obscures the task CPU utilization.
		collector.collectSnapshotFinal("final")
		log.Infof("[pprof] collector stopped, snapshots saved in %s", debugDir)
	}

	log.Infof("[pprof] collector started, HTTP server on %s, saving to %s", addr, debugDir)
	return cleanup, nil
}

var (
	pprofServerMu      sync.Mutex
	pprofServerStarted bool
	pprofServerAddr    string
)

// startPprofHTTPServer binds net/http/pprof on a random free localhost port.
// One process shares a single server; later callers reuse the bound address.
func startPprofHTTPServer() (string, error) {
	pprofServerMu.Lock()
	defer pprofServerMu.Unlock()
	if pprofServerStarted && pprofServerAddr != "" {
		return pprofServerAddr, nil
	}

	var lastErr error
	for attempt := 0; attempt < pprofListenAttempts; attempt++ {
		port := utils.GetRandomAvailableTCPPort()
		addr := utils.HostPort("127.0.0.1", port)
		ln, err := net.Listen("tcp", addr)
		if err != nil {
			lastErr = err
			continue
		}
		bound := ln.Addr().String()
		pprofServerAddr = bound
		pprofServerStarted = true
		go func(listener net.Listener, listenAddr string) {
			log.Infof("[pprof] starting HTTP server on %s", listenAddr)
			if err := http.Serve(listener, nil); err != nil {
				log.Errorf("[pprof] HTTP server error: %v", err)
			}
		}(ln, bound)
		// Brief wait so Accept is ready for the first profile fetch.
		time.Sleep(50 * time.Millisecond)
		return bound, nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("no free localhost port after %d attempts", pprofListenAttempts)
	}
	return "", fmt.Errorf("start pprof HTTP server: %w", lastErr)
}

func (c *pprofCollector) collectLoop(ctx context.Context) {
	defer c.wg.Done()
	defer func() {
		if r := recover(); r != nil {
			log.Errorf("[pprof] collector loop panicked: %v", r)
		}
	}()

	if ctx.Err() != nil {
		return
	}
	allocsTicker := time.NewTicker(pprofAllocInterval)
	defer allocsTicker.Stop()
	cpuTicker := time.NewTicker(pprofCPUInterval)
	defer cpuTicker.Stop()
	c.startCPUProfile(ctx, "initial")
	c.collectSnapshot("initial")

	earlyTicker := time.NewTicker(pprofEarlyInterval)
	defer earlyTicker.Stop()
	earlySamples := 0
	ticker := time.NewTicker(pprofInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-earlyTicker.C:
			// Capture the first four one-minute non-CPU samples, then fall back to
			// the normal 5-minute cadence.
			if earlySamples >= 4 {
				earlyTicker.Stop()
				continue
			}
			earlySamples++
			c.collectSnapshot(fmt.Sprintf("early%02d", earlySamples))
		case <-ticker.C:
			c.collectSnapshot(time.Now().Format("150405"))
		case <-cpuTicker.C:
			c.startCPUProfile(ctx, time.Now().Format("150405"))
		case <-allocsTicker.C:
			c.fetchAllocs(time.Now().Format("20060102-150405-allocs"))
		}
	}
}

// collectSnapshot captures the non-CPU state at startup, early OOM-evidence
// intervals, and the regular 5-minute cadence.
func (c *pprofCollector) collectSnapshot(tag string) {
	ts := time.Now().Format("20060102-150405")
	label := fmt.Sprintf("%s-%s", ts, tag)

	// Memory, goroutine, mutex, and block snapshots are fast (non-blocking).
	c.fetchHeap(label)
	c.fetchGoroutine(label)
	c.fetchGoroutineDebug2(label)
	c.fetchMutex(label)
	c.fetchBlock(label)
	if tag == "initial" {
		c.fetchAllocs(label)
	}
	c.fetchDBStats(label)
	c.fetchRuntimeStats(label)
}

// startCPUProfile collects a bounded CPU sample on its own cadence, independent
// of the slower heap, goroutine, mutex, and block snapshots.
func (c *pprofCollector) startCPUProfile(ctx context.Context, tag string) {
	ts := time.Now().Format("20060102-150405")
	label := fmt.Sprintf("%s-%s", ts, tag)
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		c.fetchCPU(ctx, label, pprofCPUDuration)
	}()
}

// collectSnapshotFinal captures final retained state. CPU sampling has already
// stopped with the workload; do not sample the idle shutdown period.
func (c *pprofCollector) collectSnapshotFinal(tag string) {
	ts := time.Now().Format("20060102-150405")
	label := fmt.Sprintf("%s-%s", ts, tag)

	c.fetchHeap(label)
	c.fetchGoroutine(label)
	c.fetchGoroutineDebug2(label)
	c.fetchMutex(label)
	c.fetchBlock(label)
	c.fetchAllocs(label)
	c.fetchDBStats(label)
	c.fetchRuntimeStats(label)
}

func (c *pprofCollector) fetchDBStats(label string) {
	if c == nil || c.dbStatsDir == "" {
		return
	}
	now := time.Now()
	current := ssadb.SnapshotDBOpStats()
	c.dbStatsMu.Lock()
	delta := ssadb.DeltaDBOpStats(c.lastDBStats, current)
	// DB counters accumulate across the full interval between snapshots, unlike
	// CPU profiles, which sample for 30 seconds every minute. Record the actual
	// wall window for UI share %.
	if !c.lastDBStatsAt.IsZero() {
		windowMs := now.Sub(c.lastDBStatsAt).Milliseconds()
		if windowMs < 0 {
			windowMs = 0
		}
		delta.WindowMs = windowMs
	}
	c.lastDBStats = current
	c.lastDBStatsAt = now
	c.dbStatsMu.Unlock()

	raw, err := json.MarshalIndent(delta, "", "  ")
	if err != nil {
		log.Errorf("[pprof] marshal db stats failed: %v", err)
		return
	}
	target := filepath.Join(c.dbStatsDir, label+".db.json")
	if err := os.WriteFile(target, raw, 0o644); err != nil {
		log.Errorf("[pprof] write db stats failed: %v", err)
		return
	}
	log.Infof("[pprof] db stats saved: %s window=%dms total_ms=%d total=%d query=%d create=%d update=%d delete=%d",
		target,
		delta.WindowMs,
		delta.TotalMs,
		delta.TotalCount,
		delta.Ops[ssadb.DBOpQuery].Count,
		delta.Ops[ssadb.DBOpCreate].Count,
		delta.Ops[ssadb.DBOpUpdate].Count,
		delta.Ops[ssadb.DBOpDelete].Count,
	)
}

// RuntimeStatsSnapshot captures host vs scan-task CPU/memory at one sample.
type RuntimeStatsSnapshot struct {
	Timestamp             string  `json:"timestamp"`
	NumCPU                int     `json:"num_cpu"`
	Load1                 float64 `json:"load1,omitempty"`
	HostCPUPercent        float64 `json:"host_cpu_percent"`
	ProcessCPUPercent     float64 `json:"process_cpu_percent"`
	HostMemTotalBytes     uint64  `json:"host_mem_total_bytes"`
	HostMemUsedBytes      uint64  `json:"host_mem_used_bytes"`
	HostMemAvailableBytes uint64  `json:"host_mem_available_bytes"`
	ProcessRSSBytes       uint64  `json:"process_rss_bytes"`
	ProcessHeapAllocBytes uint64  `json:"process_heap_alloc_bytes"`
	ProcessHeapSysBytes   uint64  `json:"process_heap_sys_bytes"`
	Goroutines            int     `json:"goroutines"`
}

const runtimeStatsSampleWindow = 200 * time.Millisecond

func (c *pprofCollector) fetchRuntimeStats(label string) {
	if c == nil || c.runtimeStatsDir == "" {
		return
	}
	snapshot, err := captureRuntimeStats()
	if err != nil {
		log.Warnf("[pprof] capture runtime stats failed: %v", err)
		return
	}
	raw, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		log.Errorf("[pprof] marshal runtime stats failed: %v", err)
		return
	}
	target := filepath.Join(c.runtimeStatsDir, label+".runtime.json")
	if err := os.WriteFile(target, raw, 0o644); err != nil {
		log.Errorf("[pprof] write runtime stats failed: %v", err)
		return
	}
	log.Infof(
		"[pprof] runtime stats saved: %s host_cpu=%.1f%% task_cpu=%.1f%% host_mem=%dMiB task_rss=%dMiB",
		target,
		snapshot.HostCPUPercent,
		snapshot.ProcessCPUPercent,
		snapshot.HostMemUsedBytes/(1024*1024),
		snapshot.ProcessRSSBytes/(1024*1024),
	)
}

func captureRuntimeStats() (*RuntimeStatsSnapshot, error) {
	snapshot := &RuntimeStatsSnapshot{
		Timestamp:  time.Now().UTC().Format(time.RFC3339),
		NumCPU:     runtime.NumCPU(),
		Goroutines: runtime.NumGoroutine(),
	}

	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)
	snapshot.ProcessHeapAllocBytes = memStats.HeapAlloc
	snapshot.ProcessHeapSysBytes = memStats.HeapSys

	if vm, err := mem.VirtualMemory(); err == nil && vm != nil {
		snapshot.HostMemTotalBytes = vm.Total
		snapshot.HostMemUsedBytes = vm.Used
		snapshot.HostMemAvailableBytes = vm.Available
	} else if err != nil {
		return snapshot, err
	}

	if avg, err := load.Avg(); err == nil && avg != nil {
		snapshot.Load1 = avg.Load1
	}

	hostPercents, err := cpu.Percent(runtimeStatsSampleWindow, false)
	if err != nil {
		return snapshot, err
	}
	if len(hostPercents) > 0 {
		snapshot.HostCPUPercent = hostPercents[0]
	}

	proc, err := process.NewProcess(int32(os.Getpid()))
	if err != nil {
		return snapshot, err
	}
	if rss, err := proc.MemoryInfo(); err == nil && rss != nil {
		snapshot.ProcessRSSBytes = rss.RSS
	}
	if pct, err := proc.Percent(runtimeStatsSampleWindow); err == nil {
		snapshot.ProcessCPUPercent = pct
	}

	return snapshot, nil
}

func (c *pprofCollector) fetchCPU(ctx context.Context, label string, duration time.Duration) {
	if !c.cpuMu.TryLock() {
		log.Warnf("[pprof] CPU profile skipped for %s: previous sample is still active", label)
		return // The active sample already covers this interval.
	}
	defer c.cpuMu.Unlock()
	target := filepath.Join(c.cpuDir, label+".cpu.prof")
	started := time.Now()
	if err := collectCPUProfile(ctx, target, duration); err != nil {
		log.Errorf("[pprof] CPU profile failed: %v", err)
		return
	}
	if info, err := os.Stat(target); err == nil {
		log.Infof("[pprof] CPU profile saved: %s (%v, %d bytes)", target, time.Since(started), info.Size())
	}
}

// Capture directly so cancellation stops sampling AND flushes the valid partial
// profile. Cancelling an HTTP request would discard its response body instead.
func collectCPUProfile(ctx context.Context, target string, duration time.Duration) error {
	f, err := os.Create(target)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := runtimepprof.StartCPUProfile(f); err != nil {
		return err
	}
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
	case <-timer.C:
	}
	runtimepprof.StopCPUProfile()
	return f.Close()
}

func (c *pprofCollector) fetchHeap(label string) {
	url := fmt.Sprintf("http://%s/debug/pprof/heap", c.httpAddr)
	target := filepath.Join(c.memDir, label+".mem.prof")
	if err := fetchPprof(url, target); err != nil {
		log.Errorf("[pprof] heap profile failed: %v", err)
		return
	}
	log.Infof("[pprof] heap profile saved: %s", target)
}

func (c *pprofCollector) fetchGoroutine(label string) {
	url := fmt.Sprintf("http://%s/debug/pprof/goroutine", c.httpAddr)
	target := filepath.Join(c.goroutineDir, label+".goroutine.prof")
	if err := fetchPprof(url, target); err != nil {
		log.Errorf("[pprof] goroutine profile failed: %v", err)
		return
	}
	log.Infof("[pprof] goroutine profile saved: %s", target)
}

func (c *pprofCollector) fetchGoroutineDebug2(label string) {
	url := fmt.Sprintf("http://%s/debug/pprof/goroutine?debug=2", c.httpAddr)
	target := filepath.Join(c.goroutineDir, label+".goroutine.debug2.txt")
	if err := fetchPprof(url, target); err != nil {
		log.Errorf("[pprof] goroutine debug=2 profile failed: %v", err)
		return
	}
	log.Infof("[pprof] goroutine debug=2 profile saved: %s", target)
}

func (c *pprofCollector) fetchMutex(label string) {
	url := fmt.Sprintf("http://%s/debug/pprof/mutex", c.httpAddr)
	target := filepath.Join(c.mutexDir, label+".mutex.prof")
	if err := fetchPprof(url, target); err != nil {
		log.Errorf("[pprof] mutex profile failed: %v", err)
		return
	}
	log.Infof("[pprof] mutex profile saved: %s", target)
}

func (c *pprofCollector) fetchBlock(label string) {
	url := fmt.Sprintf("http://%s/debug/pprof/block", c.httpAddr)
	target := filepath.Join(c.blockDir, label+".block.prof")
	if err := fetchPprof(url, target); err != nil {
		log.Errorf("[pprof] block profile failed: %v", err)
		return
	}
	log.Infof("[pprof] block profile saved: %s", target)
}

func (c *pprofCollector) fetchAllocs(label string) {
	url := fmt.Sprintf("http://%s/debug/pprof/allocs", c.httpAddr)
	target := filepath.Join(c.allocsDir, label+".allocs.prof")
	if err := fetchPprof(url, target); err != nil {
		log.Errorf("[pprof] allocs profile failed: %v", err)
		return
	}
	log.Infof("[pprof] allocs profile saved: %s", target)
}

func fetchPprof(url, target string) error {
	client := &http.Client{Timeout: pprofHTTPTimeout}
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("pprof HTTP %d for %s", resp.StatusCode, url)
	}

	f, err := os.Create(target)
	if err != nil {
		return err
	}
	written, copyErr := io.Copy(f, resp.Body)
	closeErr := f.Close()
	if copyErr != nil {
		return copyErr
	}
	if closeErr != nil {
		return closeErr
	}
	log.Infof("[pprof] profile downloaded: %s size=%d bytes", target, written)
	return nil
}
