package ssaapi

import (
	"context"
	"fmt"
	"io"
	"net/http"
	_ "net/http/pprof"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"
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
	dir          string
	cpuDir       string
	memDir       string
	goroutineDir string
	httpAddr     string
	wg           sync.WaitGroup
}

const (
	defaultPprofHTTPAddr = "127.0.0.1:18080"
	memoryThresholdHigh  = 10 * 1024 * 1024 * 1024 // 10 GB
	// pprofInterval is slightly longer than the high-memory CPU duration so a
	// periodic profile never starts while the previous 5-minute profile is still
	// finishing (observed as pprof HTTP 500 on Hadoop run4/run5).
	pprofInterval          = 5*time.Minute + 2*time.Second
	pprofCPUDurationNormal = 60 * time.Second
	pprofCPUDurationHigh   = 5 * time.Minute
	pprofInitialDelay      = 30 * time.Second
	pprofHTTPTimeout       = 10 * time.Minute
)

// StartPprofCollector creates the output directories, starts the pprof HTTP server,
// and launches a background goroutine that periodically collects pprof snapshots.
// The returned cleanup function stops the collector, waits for in-progress
// collections to finish, and collects a final snapshot.
func StartPprofCollector(debugDir string) (func(), error) {
	cpuDir := filepath.Join(debugDir, "cpu-pprof")
	memDir := filepath.Join(debugDir, "memory-pprof")
	goroutineDir := filepath.Join(debugDir, "goroutine-pprof")

	for _, dir := range []string{cpuDir, memDir, goroutineDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("create pprof dir %s: %w", dir, err)
		}
	}

	addr := defaultPprofHTTPAddr
	startPprofHTTPServer(addr)

	ctx, cancel := context.WithCancel(context.Background())
	collector := &pprofCollector{
		dir:          debugDir,
		cpuDir:       cpuDir,
		memDir:       memDir,
		goroutineDir: goroutineDir,
		httpAddr:     addr,
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
)

func startPprofHTTPServer(addr string) {
	pprofServerMu.Lock()
	defer pprofServerMu.Unlock()
	if pprofServerStarted {
		return
	}
	pprofServerStarted = true
	go func() {
		log.Infof("[pprof] starting HTTP server on %s", addr)
		if err := http.ListenAndServe(addr, nil); err != nil {
			log.Errorf("[pprof] HTTP server error: %v", err)
		}
	}()
	time.Sleep(100 * time.Millisecond)
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
	c.fetchCPU(label, cpuDuration) // synchronous
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
