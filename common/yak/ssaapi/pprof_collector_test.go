package ssaapi

import (
	"net"
	"net/http"
	"testing"

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
