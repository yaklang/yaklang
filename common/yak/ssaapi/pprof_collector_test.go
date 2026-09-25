package ssaapi

import (
	"context"
	"github.com/google/pprof/profile"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

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
