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
// It starts an HTTP pprof server and collects CPU, memory, and goroutine
// profiles at regular intervals.
//
// Collection strategy:
//   - Every 5 minutes, collect CPU + heap + goroutine profiles
//   - CPU profile duration: 1 minute when memory < 10GB, 5 minutes when memory >= 10GB
//   - An initial snapshot is collected at start, including short scans
//   - Shutdown finishes active CPU sampling and collects final non-CPU snapshots
type pprofCollector struct {
	dir             string
	cpuDir          string
	memDir          string
	goroutineDir    string
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
	memoryThresholdHigh = 10 * 1024 * 1024 * 1024 // 10 GB
	// pprofInterval is slightly longer than the high-memory CPU duration so a
	// periodic profile never starts while the previous 5-minute profile is still
	// finishing (observed as pprof HTTP 500 on Hadoop run4/run5).
	pprofInterval          = 5*time.Minute + 2*time.Second
	pprofCPUDurationNormal = 60 * time.Second
	pprofCPUDurationHigh   = 5 * time.Minute
	pprofHTTPTimeout       = 10 * time.Minute
	pprofListenAttempts    = 8

	// OOM investigations need early evidence. Large source scans have been
	// killed by the kernel around 3-5 minutes, before the first periodic
	// snapshot at 5m30s; capture intermediate runtime/heap/goroutine state at
	// one minute, then every minute while the task stays below the normal
	// periodic cadence.
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
	dbStatsDir := filepath.Join(debugDir, "db-stats")
	runtimeStatsDir := filepath.Join(debugDir, "runtime-stats")

	for _, dir := range []string{cpuDir, memDir, goroutineDir, dbStatsDir, runtimeStatsDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("create pprof dir %s: %w", dir, err)
		}
	}

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
	c.collectSnapshot(ctx, "initial")

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
			// Capture the first four one-minute samples, then fall back to
			// the normal 5-minute cadence.
			if earlySamples >= 4 {
				earlyTicker.Stop()
				continue
			}
			earlySamples++
			c.collectSnapshot(ctx, fmt.Sprintf("early%02d", earlySamples))
		case <-ticker.C:
			c.collectSnapshot(ctx, time.Now().Format("150405"))
		}
	}
}

// collectSnapshot captures state and starts cancellable CPU sampling in a
// tracked goroutine so the ticker continues to capture memory during sampling.
func (c *pprofCollector) collectSnapshot(ctx context.Context, tag string) {
	ts := time.Now().Format("20060102-150405")
	label := fmt.Sprintf("%s-%s", ts, tag)

	cpuDuration := pprofCPUDurationNormal
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	memGB := float64(m.Alloc) / (1024 * 1024 * 1024)
	if m.Alloc >= memoryThresholdHigh {
		cpuDuration = pprofCPUDurationHigh
		log.Infof("[pprof] memory %.1fGB >= 10GB threshold, using %v CPU profile", memGB, cpuDuration)
	} else {
		log.Infof("[pprof] memory %.1fGB < 10GB threshold, using %v CPU profile", memGB, cpuDuration)
	}

	// Memory and goroutine snapshots are fast (non-blocking)
	c.fetchHeap(label)
	c.fetchGoroutine(label)
	c.fetchDBStats(label)
	c.fetchRuntimeStats(label)

	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		c.fetchCPU(ctx, label, cpuDuration)
	}()
}

// collectSnapshotFinal captures final retained state. CPU sampling has already
// stopped with the workload; do not sample the idle shutdown period.
func (c *pprofCollector) collectSnapshotFinal(tag string) {
	ts := time.Now().Format("20060102-150405")
	label := fmt.Sprintf("%s-%s", ts, tag)

	c.fetchHeap(label)
	c.fetchGoroutine(label)
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
	// DB counters accumulate across the full interval between snapshots
	// (~5 minutes), unlike CPU profiles which only sample 1 minute in the
	// normal-memory path. Record the actual wall window for UI share %.
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
		return // The active sample already covers this interval.
	}
	defer c.cpuMu.Unlock()
	target := filepath.Join(c.cpuDir, label+".cpu.prof")
	started := time.Now()
	if err := collectCPUProfile(ctx, target, duration); err != nil {
		log.Errorf("[pprof] CPU profile failed: %v", err)
		return
	}
	log.Infof("[pprof] CPU profile saved: %s (%v)", target, time.Since(started))
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
	defer f.Close()

	_, err = io.Copy(f, resp.Body)
	return err
}
