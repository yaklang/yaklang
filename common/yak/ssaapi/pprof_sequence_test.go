package ssaapi

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/pprof/profile"
	"github.com/stretchr/testify/require"
)

// Encoding heap/allocs/stack profiles must not run inside the CPU sample:
// otherwise the sample measures the collector instead of the scan workload.
func TestPprofCollectorFinishesCPUBeforeSnapshot(t *testing.T) {
	root := t.TempDir()
	collector := &pprofCollector{
		dir:          root,
		cpuDir:       filepath.Join(root, "cpu-pprof"),
		memDir:       filepath.Join(root, "memory-pprof"),
		goroutineDir: filepath.Join(root, "goroutine-pprof"),
	}
	for _, dir := range []string{collector.cpuDir, collector.memDir, collector.goroutineDir} {
		require.NoError(t, os.MkdirAll(dir, 0o755))
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cpuAtSnapshot := make(chan error, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/debug/pprof/heap" {
			// The first heap request must see a complete, flushed CPU profile.
			files, err := filepath.Glob(filepath.Join(collector.cpuDir, "*.cpu.prof"))
			if err == nil && len(files) != 1 {
				err = fmt.Errorf("CPU profile count before heap snapshot = %d, want 1", len(files))
			}
			if err == nil {
				var data []byte
				data, err = os.ReadFile(files[0])
				if err == nil {
					var parsed *profile.Profile
					parsed, err = profile.ParseData(data)
					if err == nil {
						err = parsed.CheckValid()
					}
				}
			}
			cpuAtSnapshot <- err
			cancel()
		}
		_, _ = w.Write([]byte("snapshot"))
	}))
	defer server.Close()
	collector.httpAddr = strings.TrimPrefix(server.URL, "http://")
	collector.wg.Add(1)
	go collector.collectLoop(ctx)
	collector.wg.Wait()
	require.NoError(t, <-cpuAtSnapshot, "non-CPU snapshot overlapped an unfinished CPU sample")
	for _, pattern := range []string{
		"memory-pprof/*.mem.prof", "goroutine-pprof/*.goroutine.prof",
		"goroutine-pprof/*.goroutine.txt", "allocs-pprof/*.allocs.prof",
		"mutex-pprof/*.mutex.prof", "block-pprof/*.block.prof",
	} {
		files, err := filepath.Glob(filepath.Join(root, pattern))
		require.NoError(t, err)
		require.NotEmpty(t, files, pattern)
	}
}
