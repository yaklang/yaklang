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
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/yaklang/yaklang/common/log"
)

const envHeapDumpDir = "YAK_DIAGNOSTICS_HEAP_DUMP_DIR"

type heapDumpConfig struct {
	memThreshold uint64
	name         string
	fileName     string
	dumpDir      string
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
		if !strings.HasSuffix(name, ".pb.gz") {
			name += ".pb.gz"
		}
		cfg.fileName = name
	}
}

func WithDumpDir(dir string) HeapDumpOption {
	return func(cfg *heapDumpConfig) {
		cfg.dumpDir = strings.TrimSpace(dir)
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

func StartPprofServer(addr string) {
	startHTTPServer(strings.TrimSpace(addr))
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
	save := func(phase string) {
		var stats runtime.MemStats
		runtime.ReadMemStats(&stats)
		if cfg.memThreshold > 0 && stats.Alloc < cfg.memThreshold {
			return
		}
		logHeapStats(withPhaseName(cfg.name, phase), stats)
		if cfg.disable || !saveFile {
			saved = true
			return
		}
		if err := writeHeapProfile(cfg, stats.Alloc, phase); err != nil {
			log.Errorf("could not write heap profile: %v", err)
			return
		}
		saved = true
	}

	save("before_gc")
	if cfg.runtimeGC {
		runtime.GC()
		save("after_gc")
	}
	return saved
}

func writeHeapProfile(cfg heapDumpConfig, alloc uint64, phase string) error {
	target := resolveHeapProfileTarget(cfg, alloc, phase)
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
	log.Infof("[heap] snapshot profile saved: %s", target)
	return nil
}

func bytesToMB(b uint64) uint64 {
	return b / 1024 / 1024
}

func normalizeHeapName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "heap"
	}
	var b strings.Builder
	for _, r := range name {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-' {
			b.WriteRune(r)
			continue
		}
		b.WriteByte('_')
	}
	return b.String()
}

func withPhaseName(name string, phase string) string {
	base := strings.TrimSpace(name)
	if base == "" {
		base = "heap"
	}
	if phase == "" {
		return base
	}
	return base + " " + phase
}

func logHeapStats(name string, stats runtime.MemStats) {
	log.Infof(
		"[heap] %s alloc=%dMB heap_alloc=%dMB heap_inuse=%dMB heap_idle=%dMB heap_objects=%d num_gc=%d",
		name,
		bytesToMB(stats.Alloc),
		bytesToMB(stats.HeapAlloc),
		bytesToMB(stats.HeapInuse),
		bytesToMB(stats.HeapIdle),
		stats.HeapObjects,
		stats.NumGC,
	)
}

func resolveHeapProfileTarget(cfg heapDumpConfig, alloc uint64, phase string) string {
	target := strings.TrimSpace(cfg.fileName)
	if target == "" {
		name := fmt.Sprintf(
			"heap_profile_%s_%d_%s",
			time.Now().Format("2006-01-02-15:04:05.000000000"),
			bytesToMB(alloc),
			normalizeHeapName(cfg.name),
		)
		if phase != "" {
			name += "_" + phase
		}
		name += ".pb.gz"
		if dir := strings.TrimSpace(cfg.dumpDir); dir != "" {
			return filepath.Join(dir, name)
		}
		if dir := strings.TrimSpace(os.Getenv(envHeapDumpDir)); dir != "" {
			return filepath.Join(dir, name)
		}
		return name
	}

	trimmed := strings.TrimSuffix(target, ".pb.gz")
	if phase != "" {
		trimmed += "_" + phase
	}
	return trimmed + ".pb.gz"
}

// LogHeapSnapshot logs heap memory usage, and optionally logs the GC-before/after delta.
func LogHeapSnapshot(name string, withRuntimeGC bool) {
	DumpHeap(
		WithName(name),
		WithRuntimeGC(withRuntimeGC),
	)
}
