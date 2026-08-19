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
//   - An initial snapshot is collected 30 seconds after start
//   - A final snapshot is collected on shutdown
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
}

const (
	// pprof listens on a random free localhost port (not a fixed 18080) so
	// concurrent debug compiles / leftover servers do not collide.
	memoryThresholdHigh = 10 * 1024 * 1024 * 1024 // 10 GB
	// pprofInterval is slightly longer than the high-memory CPU duration so a
	// periodic profile never starts while the previous 5-minute profile is still
	// finishing (observed as pprof HTTP 500 on Hadoop run4/run5).
	pprofInterval          = pprofCPUDurationHigh + 2*time.Second
	pprofCPUDurationNormal = 60 * time.Second
	pprofCPUDurationHigh   = 5 * time.Minute
	pprofInitialDelay      = 30 * time.Second
	pprofHTTPTimeout       = 10 * time.Minute
	pprofListenAttempts    = 8
)

// StartPprofCollector creates the output directories, starts the pprof HTTP server,
// and launches a background goroutine that periodically collects pprof snapshots.
// The returned cleanup function stops the collector, waits for in-progress
// collections to finish, and collects a final snapshot.
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
		// Final snapshot: collect synchronously so CPU profile completes before return.
		// Use a short CPU duration (10s) so cleanup doesn't block too long.
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

	select {
	case <-ctx.Done():
		return
	case <-time.After(pprofInitialDelay):
	}

	c.collectSnapshot("initial", false)

	ticker := time.NewTicker(pprofInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.collectSnapshot(time.Now().Format("150405"), false)
		}
	}
}

// collectSnapshot collects CPU, memory, and goroutine profiles.
// When syncCPU is true, the CPU profile fetch blocks until completion (used for
// the final snapshot). When false, it runs in a tracked goroutine (used for
// periodic snapshots so the ticker is not blocked).
func (c *pprofCollector) collectSnapshot(tag string, syncCPU bool) {
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

	if syncCPU {
		// For final snapshot: wait for CPU profile to complete
		c.fetchCPU(label, cpuDuration)
	} else {
		// For periodic snapshots: run CPU profile in background
		c.wg.Add(1)
		go func() {
			defer c.wg.Done()
			defer func() {
				if r := recover(); r != nil {
					log.Errorf("[pprof] periodic CPU snapshot panicked: %v", r)
				}
			}()
			c.fetchCPU(label, cpuDuration)
		}()
	}
}

// collectSnapshotFinal collects a final snapshot with a short CPU profile
// duration (10 seconds) so cleanup doesn't block too long. CPU profile is
// collected synchronously.
func (c *pprofCollector) collectSnapshotFinal(tag string) {
	ts := time.Now().Format("20060102-150405")
	label := fmt.Sprintf("%s-%s", ts, tag)

	cpuDuration := 10 * time.Second // short final CPU profile

	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	memGB := float64(m.Alloc) / (1024 * 1024 * 1024)
	if m.Alloc >= memoryThresholdHigh {
		cpuDuration = 30 * time.Second // still shorter than periodic 5min
		log.Infof("[pprof] memory %.1fGB >= 10GB, final CPU profile: %v", memGB, cpuDuration)
	} else {
		log.Infof("[pprof] memory %.1fGB < 10GB, final CPU profile: %v", memGB, cpuDuration)
	}

	c.fetchHeap(label)
	c.fetchGoroutine(label)
	c.fetchDBStats(label)
	c.fetchRuntimeStats(label)
	c.fetchCPU(label, cpuDuration) // synchronous
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

func (c *pprofCollector) fetchCPU(label string, duration time.Duration) {
	url := fmt.Sprintf("http://%s/debug/pprof/profile?seconds=%d", c.httpAddr, int(duration.Seconds()))
	target := filepath.Join(c.cpuDir, label+".cpu.prof")
	if err := fetchPprof(url, target); err != nil {
		log.Errorf("[pprof] CPU profile failed: %v", err)
		return
	}
	log.Infof("[pprof] CPU profile saved: %s (%v)", target, duration)
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
