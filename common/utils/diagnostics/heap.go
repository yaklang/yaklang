package diagnostics

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	_ "net/http/pprof"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/log"
)

type heapDumpConfig struct {
	memThreshold uint64
	name         string
	fileName     string
	disable      bool
	maxDumps     int
	runtimeGC    bool
	httpAddr     string
}

type HeapDumpOption func(*heapDumpConfig)

func newHeapDumpConfig(opts ...HeapDumpOption) heapDumpConfig {
	cfg := heapDumpConfig{
		memThreshold: 0,
		httpAddr:     ":18080",
	}
	for _, opt := range opts {
		opt(&cfg)
	}
	return cfg
}

func WithHeapLimit(memThreshold uint64) HeapDumpOption {
	return func(cfg *heapDumpConfig) {
		cfg.memThreshold = memThreshold
	}
}

func WithDumpCount(count int) HeapDumpOption {
	return func(cfg *heapDumpConfig) {
		cfg.maxDumps = count
	}
}

func WithHeapFile(name string) HeapDumpOption {
	return func(cfg *heapDumpConfig) {
		if filepath.Ext(name) != ".pb.gz" {
			name += ".pb.gz"
		}
		cfg.fileName = name
	}
}

func WithFileName(name string) HeapDumpOption {
	return WithHeapFile(name)
}

func WithName(name string) HeapDumpOption {
	return func(cfg *heapDumpConfig) {
		cfg.name = name
	}
}

func WithDisable(disable ...bool) HeapDumpOption {
	return func(cfg *heapDumpConfig) {
		if len(disable) == 0 {
			cfg.disable = true
			return
		}
		cfg.disable = disable[0]
	}
}

func WithRuntimeGC(enable ...bool) HeapDumpOption {
	return func(cfg *heapDumpConfig) {
		if len(enable) == 0 {
			cfg.runtimeGC = true
			return
		}
		cfg.runtimeGC = enable[0]
	}
}

func WithHTTPServer(addr string) HeapDumpOption {
	return func(cfg *heapDumpConfig) {
		cfg.httpAddr = addr
	}
}

var pprofServerOnce sync.Map

func startHTTPServer(addr string) {
	if addr == "" {
		return
	}
	if _, loaded := pprofServerOnce.LoadOrStore(addr, struct{}{}); loaded {
		return
	}
	go func() {
		log.Infof("starting pprof HTTP server on %s", addr)
		if err := http.ListenAndServe(addr, nil); err != nil {
			if !errors.Is(err, http.ErrServerClosed) {
				log.Errorf("pprof HTTP server error on %s: %v", addr, err)
			}
		}
	}()
}

func DumpHeap(opts ...HeapDumpOption) bool {
	cfg := newHeapDumpConfig(opts...)
	return performHeapDump(cfg, true)
}

func StartHeapMonitor(interval time.Duration, opts ...HeapDumpOption) context.CancelFunc {
	if interval <= 0 {
		interval = 30 * time.Second
	}
	cfg := newHeapDumpConfig(opts...)
	startHTTPServer(cfg.httpAddr)

	ctx, cancel := context.WithCancel(context.Background())
	go runHeapMonitor(ctx, interval, cfg)
	log.Infof("Memory usage monitor started (interval=%v, threshold=%d MB)", interval, bytesToMB(cfg.memThreshold))
	log.Infof("Use 'go tool pprof --seconds 30 http://127.0.0.1%v/debug/pprof/profile' to collect CPU profile", cfg.httpAddr)
	return cancel
}

func runHeapMonitor(ctx context.Context, interval time.Duration, cfg heapDumpConfig) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	dumps := 0
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if cfg.maxDumps > 0 && dumps >= cfg.maxDumps {
				continue
			}
			if performHeapDump(cfg, true) {
				dumps++
			}
		}
	}
}

func performHeapDump(cfg heapDumpConfig, saveFile bool) bool {
	saved := false
	save := func() {
		var stats runtime.MemStats
		runtime.ReadMemStats(&stats)
		if cfg.memThreshold > 0 && stats.Alloc < cfg.memThreshold {
			return
		}
		log.Infof("Memory usage exceeds threshold (%d MB) | %s", bytesToMB(stats.Alloc), cfg.name)
		if cfg.disable || !saveFile {
			saved = true
			return
		}
		if err := writeHeapProfile(cfg, stats.Alloc); err != nil {
			log.Errorf("could not write heap profile: %v", err)
			return
		}
		saved = true
	}

	save()
	if cfg.runtimeGC {
		runtime.GC()
		save()
	}
	return saved
}

func writeHeapProfile(cfg heapDumpConfig, alloc uint64) error {
	target := cfg.fileName
	if target == "" {
		target = fmt.Sprintf("heap_profile_%s_%d_%s.pb.gz", time.Now().Format("2006-01-02-15:04:05"), bytesToMB(alloc), cfg.name)
	}
	if filepath.Ext(target) != ".pb.gz" {
		target += ".pb.gz"
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil && !errors.Is(err, os.ErrExist) {
		return err
	}

	tmp := target + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	defer func() {
		f.Close()
		os.Remove(tmp)
	}()

	if err := pprof.WriteHeapProfile(f); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp, target); err != nil {
		return err
	}
	return nil
}

func bytesToMB(b uint64) uint64 {
	return b / 1024 / 1024
}
