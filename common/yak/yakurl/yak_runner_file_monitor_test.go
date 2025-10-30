package yakurl

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// TestHandlerFileMonitor_NewMonitor tests creating a new file monitor
func TestHandlerFileMonitor_NewMonitor(t *testing.T) {
	// Create a temporary directory for testing
	tmpDir, err := os.MkdirTemp("", "yak_monitor_test_*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	monitorID := utils.RandStringBytes(10)

	// Create request to start monitoring
	requestData := map[string]interface{}{
		"operate": OP_NEW_MONITOR,
		"id":      monitorID,
		"path":    tmpDir,
	}
	data, err := json.Marshal(requestData)
	require.NoError(t, err)

	request := &ypb.DuplexConnectionRequest{
		Data: data,
	}

	// Call the handler
	err = handlerFileMonitor(ctx, request)
	require.NoError(t, err)

	// Verify the monitor was created
	monitor, ok := YakRunnerMonitor.Get(monitorID)
	require.True(t, ok, "Monitor should be created")
	require.NotNil(t, monitor, "Monitor should not be nil")
	require.Equal(t, tmpDir, monitor.WatchPatch, "Monitor should watch the correct path")

	// Cleanup
	if monitor != nil {
		monitor.CancelFunc()
	}
	YakRunnerMonitor.Delete(monitorID)
}

// TestHandlerFileMonitor_StopMonitor tests stopping a file monitor
func TestHandlerFileMonitor_StopMonitor(t *testing.T) {
	// Create a temporary directory for testing
	tmpDir, err := os.MkdirTemp("", "yak_monitor_test_*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	monitorID := utils.RandStringBytes(10)

	// First, create a monitor
	requestData := map[string]interface{}{
		"operate": OP_NEW_MONITOR,
		"id":      monitorID,
		"path":    tmpDir,
	}
	data, err := json.Marshal(requestData)
	require.NoError(t, err)

	request := &ypb.DuplexConnectionRequest{
		Data: data,
	}

	err = handlerFileMonitor(ctx, request)
	require.NoError(t, err)

	// Verify monitor exists
	_, ok := YakRunnerMonitor.Get(monitorID)
	require.True(t, ok)

	// Now stop the monitor
	stopData := map[string]interface{}{
		"operate": OP_STOP_MONITOR,
		"id":      monitorID,
	}
	data, err = json.Marshal(stopData)
	require.NoError(t, err)

	request = &ypb.DuplexConnectionRequest{
		Data: data,
	}

	err = handlerFileMonitor(ctx, request)
	require.NoError(t, err)

	// Verify monitor was removed
	_, ok = YakRunnerMonitor.Get(monitorID)
	require.False(t, ok, "Monitor should be removed after stop")
}

// TestHandlerFileMonitor_ReplaceExistingMonitor tests that a new monitor replaces an old one with the same ID
func TestHandlerFileMonitor_ReplaceExistingMonitor(t *testing.T) {
	// Create two temporary directories
	tmpDir1, err := os.MkdirTemp("", "yak_monitor_test_1_*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir1)

	tmpDir2, err := os.MkdirTemp("", "yak_monitor_test_2_*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir2)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	monitorID := utils.RandStringBytes(10)

	// Create first monitor
	requestData := map[string]interface{}{
		"operate": OP_NEW_MONITOR,
		"id":      monitorID,
		"path":    tmpDir1,
	}
	data, err := json.Marshal(requestData)
	require.NoError(t, err)

	request := &ypb.DuplexConnectionRequest{
		Data: data,
	}

	err = handlerFileMonitor(ctx, request)
	require.NoError(t, err)

	monitor1, ok := YakRunnerMonitor.Get(monitorID)
	require.True(t, ok)
	require.Equal(t, tmpDir1, monitor1.WatchPatch)

	// Create second monitor with same ID but different path
	requestData = map[string]interface{}{
		"operate": OP_NEW_MONITOR,
		"id":      monitorID,
		"path":    tmpDir2,
	}
	data, err = json.Marshal(requestData)
	require.NoError(t, err)

	request = &ypb.DuplexConnectionRequest{
		Data: data,
	}

	err = handlerFileMonitor(ctx, request)
	require.NoError(t, err)

	// Verify the monitor was replaced
	monitor2, ok := YakRunnerMonitor.Get(monitorID)
	require.True(t, ok)
	require.Equal(t, tmpDir2, monitor2.WatchPatch, "Monitor should watch the new path")
	require.NotEqual(t, monitor1, monitor2, "Monitor should be replaced")

	// Cleanup
	monitor2.CancelFunc()
	YakRunnerMonitor.Delete(monitorID)
}

// TestHandlerFileMonitor_InvalidPath tests handling of invalid paths
func TestHandlerFileMonitor_InvalidPath(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	monitorID := utils.RandStringBytes(10)
	invalidPath := "/this/path/should/not/exist/at/all"

	// Create request with invalid path
	requestData := map[string]interface{}{
		"operate": OP_NEW_MONITOR,
		"id":      monitorID,
		"path":    invalidPath,
	}
	data, err := json.Marshal(requestData)
	require.NoError(t, err)

	request := &ypb.DuplexConnectionRequest{
		Data: data,
	}

	// Call the handler - should return an error
	err = handlerFileMonitor(ctx, request)
	require.Error(t, err, "Should return error for invalid path")

	// Verify no monitor was created
	_, ok := YakRunnerMonitor.Get(monitorID)
	require.False(t, ok, "Monitor should not be created for invalid path")
}

// TestHandlerFileMonitor_StopNonExistentMonitor tests stopping a monitor that doesn't exist
func TestHandlerFileMonitor_StopNonExistentMonitor(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	monitorID := utils.RandStringBytes(10)

	// Try to stop a monitor that doesn't exist
	stopData := map[string]interface{}{
		"operate": OP_STOP_MONITOR,
		"id":      monitorID,
	}
	data, err := json.Marshal(stopData)
	require.NoError(t, err)

	request := &ypb.DuplexConnectionRequest{
		Data: data,
	}

	// Should not return an error, just do nothing
	err = handlerFileMonitor(ctx, request)
	require.NoError(t, err)
}
