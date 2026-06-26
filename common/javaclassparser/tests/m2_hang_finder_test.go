package tests

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/javaclassparser"
)

// TestHangFinder decompiles each .class in HANG_DIR (default /tmp/jdsc-final) in a goroutine with a
// per-class timeout (HANG_MS, default 4000ms) and reports any class that exceeds it (hang / pathological
// slowdown). Skipped unless HANG_FINDER is set.
func TestHangFinder(t *testing.T) {
	if os.Getenv("HANG_FINDER") == "" {
		t.Skip("set HANG_FINDER=1 to scan for hanging classes")
	}
	dir := os.Getenv("HANG_DIR")
	if dir == "" {
		dir = "/tmp/jdsc-final"
	}
	limitMs := envInt("HANG_MS", 4000)
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	var names []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".class") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	t.Logf("scanning %d classes, per-class limit %dms", len(names), limitMs)
	var slow []string
	for _, name := range names {
		raw, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			continue
		}
		done := make(chan struct{})
		start := time.Now()
		go func() {
			defer func() { recover(); close(done) }()
			_, _ = javaclassparser.Decompile(raw)
		}()
		select {
		case <-done:
			if el := time.Since(start); el > time.Duration(limitMs)*time.Millisecond {
				t.Logf("SLOW %s took %s", name, el)
				slow = append(slow, name)
			}
		case <-time.After(time.Duration(limitMs) * time.Millisecond):
			t.Logf("HANG %s exceeded %dms (still running)", name, limitMs)
			slow = append(slow, name+" (HANG)")
			// give it a moment; if truly hung the goroutine leaks but the test continues
		}
	}
	t.Logf("==== done. slow/hang count=%d ====", len(slow))
	for _, s := range slow {
		t.Logf("  %s", s)
	}
}
