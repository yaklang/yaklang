package ssaprofile

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"time"

	"github.com/yaklang/yaklang/common/log"
)

type dumpHeapConfig struct {
	memThreshold uint64
	name         string
	fileName     string
	disable      bool
	count        int
	runtimeGC    bool
}

type dumpHeapOption func(*dumpHeapConfig)

func NewHeapConfig(opts ...dumpHeapOption) dumpHeapConfig {
	cfg := dumpHeapConfig{
		memThreshold: 0, // 默认阈值 0
	}
	for _, opt := range opts {
		opt(&cfg)
	}
	return cfg
}

func WithHeapLimit(memThreshold uint64) dumpHeapOption {
	return func(cfg *dumpHeapConfig) {
		cfg.memThreshold = memThreshold
	}
}

func WithDumpCount(count int) dumpHeapOption {
	return func(cfg *dumpHeapConfig) {
		cfg.count = count
	}
}

func WithFileName(name string) dumpHeapOption {
	return func(dhc *dumpHeapConfig) {
		// check file name suffix is .pb.gz
		if filepath.Ext(name) != ".pb.gz" {
			name += ".pb.gz"
		}
		dhc.fileName = name
	}
}

func WithName(name string) dumpHeapOption {
	return func(cfg *dumpHeapConfig) {
		cfg.name = name
	}
}
func WithDisable(disable ...bool) dumpHeapOption {
	return func(cfg *dumpHeapConfig) {
		if len(disable) == 0 {
			cfg.disable = true
		} else {
			cfg.disable = disable[0]
		}
	}
}

func WithRuntimeGC(enable ...bool) dumpHeapOption {
	return func(cfg *dumpHeapConfig) {
		if len(enable) == 0 {
			cfg.runtimeGC = true
		}
		cfg.runtimeGC = enable[0]
	}
}

func DumpHeapProfile(opts ...dumpHeapOption) {
	cfg := NewHeapConfig(opts...)
	count := 0
	save := func() {
		var m runtime.MemStats
		runtime.ReadMemStats(&m)
		log.Printf("Memory usage exceeds threshold (%d MB) | %s\n", bToMb(m.Alloc), cfg.name)
		if cfg.disable {
			return
		}
		if m.Alloc > cfg.memThreshold {
			tmpFile := fmt.Sprintf(
				"heap_profile_%s_%d_%s_%d.pb.gz",
				time.Now().Format("2006-01-02-15:04:05"), bToMb(m.Alloc), cfg.name, count,
			)
			count++
			// log.Printf("Memory dumping heap profile to %s\n", tmpFile)

			// Create temporary file
			f, err := os.Create(tmpFile)
			if err != nil {
				log.Fatalf("Could not create temporary heap profile file: %v", err)
			}
			// Write to temporary file
			if err := pprof.WriteHeapProfile(f); err != nil {
				f.Close()
				os.Remove(tmpFile) // Clean up temporary file on error
				log.Fatalf("Could not write heap profile: %v", err)
			}
			f.Close()

			if filename := cfg.fileName; filename != "" {
				// Atomically move temporary file to target file
				if err := os.Rename(tmpFile, filename); err != nil {
					log.Fatalf("Could not move heap profile to final location: %v", err)
				}
				os.Remove(tmpFile) // Clean up temporary file
			}
		} else {
			log.Printf("Current memory usage: %d MB, below threshold (%d MB)\n", bToMb(m.Alloc), bToMb(cfg.memThreshold))
		}
	}
	save()
	if cfg.runtimeGC {
		runtime.GC()
		save()
	}
}

func DumpHeapProfileWithInterval(dumpInterval time.Duration, opts ...dumpHeapOption) {
	log.Printf("Memory usage exceeds threshold, dumping heap profile every %v\n", dumpInterval)
	// 定期dump heap profile
	go func() {
		ticker := time.NewTicker(dumpInterval)
		defer ticker.Stop()
		for range ticker.C {
			DumpHeapProfile(opts...)
		}
	}()
}
