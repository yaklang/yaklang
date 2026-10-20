package ssaapi

import (
	"context"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/pprof/profile"
	"github.com/stretchr/testify/require"
)

func TestPprofEndpointsSmoke(t *testing.T) {
	if os.Getenv("SSA_PPROF_ENDPOINT_SMOKE") != "1" {
		t.Skip("set SSA_PPROF_ENDPOINT_SMOKE=1 to run the 30-second pprof endpoint smoke")
	}
	debugDir := os.Getenv("SSA_PPROF_SMOKE_DIR")
	require.NotEmpty(t, debugDir)
	absDir, err := filepath.Abs(debugDir)
	require.NoError(t, err)
	rel, err := filepath.Rel("/mnt/data/scan-outputs", absDir)
	require.NoError(t, err)
	require.False(t, rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)),
		"pprof smoke output must stay under /mnt/data/scan-outputs")

	cleanup, err := StartPprofCollector(absDir)
	require.NoError(t, err)
	cleaned := false
	t.Cleanup(func() {
		if !cleaned {
			cleanup()
		}
	})

	pprofServerMu.Lock()
	addr := pprofServerAddr
	pprofServerMu.Unlock()
	require.NotEmpty(t, addr)

	stop := make(chan struct{})
	var workers sync.WaitGroup
	var work atomic.Uint64
	workers.Add(3)
	go func() {
		defer workers.Done()
		for {
			select {
			case <-stop:
				return
			default:
				work.Add(1)
			}
		}
	}()
	var contended sync.Mutex
	go func() {
		defer workers.Done()
		for {
			select {
			case <-stop:
				return
			default:
				contended.Lock()
				time.Sleep(time.Millisecond)
				contended.Unlock()
			}
		}
	}()
	go func() {
		defer workers.Done()
		for {
			select {
			case <-stop:
				return
			default:
				contended.Lock()
				contended.Unlock()
			}
		}
	}()

	// The collector's initial 30-second direct CPU sample must finish before
	// asking net/http/pprof for its CPU endpoint (only one CPU sampler can run).
	time.Sleep(31 * time.Second)
	endpointDir := filepath.Join(absDir, "endpoint-smoke")
	require.NoError(t, os.MkdirAll(endpointDir, 0o755))
	client := &http.Client{Timeout: 15 * time.Second}
	fetch := func(name, path string) []byte {
		resp, err := client.Get("http://" + addr + path)
		require.NoError(t, err)
		defer resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode, path)
		data, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		require.NotEmpty(t, data, path)
		require.NoError(t, os.WriteFile(filepath.Join(endpointDir, name), data, 0o644))
		return data
	}

	cpuData := fetch("cpu.prof", "/debug/pprof/profile?seconds=2")
	for _, endpoint := range []struct{ name, path string }{
		{"heap.prof", "/debug/pprof/heap"},
		{"goroutine.prof", "/debug/pprof/goroutine"},
		{"goroutine.debug2.txt", "/debug/pprof/goroutine?debug=2"},
		{"mutex.prof", "/debug/pprof/mutex"},
		{"block.prof", "/debug/pprof/block"},
		{"allocs.prof", "/debug/pprof/allocs"},
	} {
		fetch(endpoint.name, endpoint.path)
	}
	close(stop)
	workers.Wait()
	cleanup()
	cleaned = true
	runtime.KeepAlive(work.Load())

	parsed, err := profile.ParseData(cpuData)
	require.NoError(t, err)
	require.NoError(t, parsed.CheckValid())
	require.NotEmpty(t, parsed.Function, "CPU profile must contain symbolized functions")
}

func TestPprofCollectorWritesRequiredProfiles(t *testing.T) {
	debugDir := t.TempDir()
	cleanup, err := StartPprofCollector(debugDir)
	require.NoError(t, err)
	cleaned := false
	t.Cleanup(func() {
		if !cleaned {
			cleanup()
		}
	})

	// Generate enough CPU activity for a partial profile to include this test's
	// function, so `go tool pprof -top` can resolve symbols in the smoke run.
	var work uint64
	workUntil := time.Now().Add(time.Second)
	for time.Now().Before(workUntil) {
		work ^= uint64(time.Now().UnixNano())
	}
	runtime.KeepAlive(work)
	cleanup()
	cleaned = true

	for _, pattern := range []string{
		"cpu-pprof/*.cpu.prof",
		"memory-pprof/*.mem.prof",
		"goroutine-pprof/*.goroutine.prof",
		"goroutine-pprof/*.goroutine.debug2.txt",
		"mutex-pprof/*.mutex.prof",
		"block-pprof/*.block.prof",
		"allocs-pprof/*.allocs.prof",
		"db-stats/*.db.json",
		"runtime-stats/*.runtime.json",
	} {
		matches, err := filepath.Glob(filepath.Join(debugDir, pattern))
		require.NoError(t, err)
		require.NotEmptyf(t, matches, "missing profile artifact matching %q", pattern)
		info, err := os.Stat(matches[0])
		require.NoError(t, err)
		require.Greater(t, info.Size(), int64(0), "empty profile artifact %s", matches[0])
	}

	cpuProfiles, err := filepath.Glob(filepath.Join(debugDir, "cpu-pprof", "*.cpu.prof"))
	require.NoError(t, err)
	cpuData, err := os.ReadFile(cpuProfiles[0])
	require.NoError(t, err)
	parsed, err := profile.ParseData(cpuData)
	require.NoError(t, err)
	require.NoError(t, parsed.CheckValid())
	foundTestFunction := false
	for _, fn := range parsed.Function {
		if strings.Contains(fn.Name, "TestPprofCollectorWritesRequiredProfiles") {
			foundTestFunction = true
			break
		}
	}
	require.True(t, foundTestFunction, "CPU profile does not resolve the smoke test function")

	stackFiles, err := filepath.Glob(filepath.Join(debugDir, "goroutine-pprof", "*.goroutine.debug2.txt"))
	require.NoError(t, err)
	stackData, err := os.ReadFile(stackFiles[0])
	require.NoError(t, err)
	require.Contains(t, string(stackData), "goroutine ")
}

func TestStartPprofHTTPServerUsesRandomFreePort(t *testing.T) {
	// Isolate process-global pprof listener state for this test.
	pprofServerMu.Lock()
	pprofServerStarted = false
	pprofServerAddr = ""
	pprofServerMu.Unlock()

	addr, err := startPprofHTTPServer()
	require.NoError(t, err)
	require.NotEmpty(t, addr)
	require.NotContains(t, addr, ":18080")

	// Server must accept connections on the advertised address.
	conn, err := net.Dial("tcp", addr)
	require.NoError(t, err)
	_ = conn.Close()

	resp, err := http.Get("http://" + addr + "/debug/pprof/")
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	// Second start reuses the same bound address instead of fighting for a port.
	again, err := startPprofHTTPServer()
	require.NoError(t, err)
	require.Equal(t, addr, again)
}

func TestCaptureRuntimeStats(t *testing.T) {
	snapshot, err := captureRuntimeStats()
	require.NoError(t, err)
	require.NotNil(t, snapshot)
	require.Greater(t, snapshot.NumCPU, 0)
	require.Greater(t, snapshot.HostMemTotalBytes, uint64(0))
	require.Greater(t, snapshot.ProcessRSSBytes, uint64(0))
	require.GreaterOrEqual(t, snapshot.HostCPUPercent, 0.0)
	require.GreaterOrEqual(t, snapshot.ProcessCPUPercent, 0.0)
}

// A finished scan must not wait for the remainder of a 5-minute sample.
// Its partial profile must remain readable rather than being discarded.
func TestCPUProfileCancellationFlushesPartialProfile(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	target := filepath.Join(t.TempDir(), "partial.cpu.prof")
	done := make(chan error, 1)
	go func() { done <- collectCPUProfile(ctx, target, 5*time.Minute) }()
	time.Sleep(100 * time.Millisecond)
	cancel()
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("profile did not stop with the workload")
	}
	data, err := os.ReadFile(target)
	require.NoError(t, err)
	parsed, err := profile.ParseData(data)
	require.NoError(t, err)
	require.NoError(t, parsed.CheckValid())
	require.Greater(t, parsed.DurationNanos, int64(0))
	// Stopping must release the process-wide profiler for the next scan.
	require.NoError(t, collectCPUProfile(context.Background(), filepath.Join(t.TempDir(), "next.cpu.prof"), time.Millisecond))
}
