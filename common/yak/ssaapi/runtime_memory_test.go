package ssaapi

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// The existing 5-minute runtime snapshot cannot distinguish live heap from
// idle spans or memory already returned to the OS. These fields are also
// required by the lightweight endpoint used by the 5-second external sampler.
func TestRuntimeStatsExposeRetainedHeap(t *testing.T) {
	snapshot, err := captureRuntimeStats()
	require.NoError(t, err)
	data, err := json.Marshal(snapshot)
	require.NoError(t, err)
	var fields map[string]any
	require.NoError(t, json.Unmarshal(data, &fields))
	for _, name := range []string{"process_heap_alloc_bytes", "process_heap_sys_bytes", "process_heap_idle_bytes", "process_heap_inuse_bytes", "process_heap_released_bytes"} {
		require.Contains(t, fields, name)
	}
}

func TestPprofMemoryEndpoint(t *testing.T) {
	addr, err := startPprofHTTPServer()
	require.NoError(t, err)
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get("http://" + addr + "/debug/ssa/memory")
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var fields map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&fields))
	for _, name := range []string{"timestamp", "process_heap_alloc_bytes", "process_heap_sys_bytes", "process_heap_idle_bytes", "process_heap_inuse_bytes", "process_heap_released_bytes"} {
		require.Contains(t, fields, name)
	}
}
