package scannode

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewDebugCollector_CreatesDir(t *testing.T) {
	dc, err := NewDebugCollector("task-1", "attempt-1", "sub-1")
	require.NoError(t, err)
	require.NotNil(t, dc)
	defer dc.Cleanup()

	_, err = os.Stat(dc.dir)
	assert.NoError(t, err, "debug dir should exist")
}

func TestDebugCollector_StartStopProfiling(t *testing.T) {
	dc, err := NewDebugCollector("task-1", "attempt-1", "sub-1")
	require.NoError(t, err)
	defer dc.Cleanup()

	require.NoError(t, dc.Start())
	assert.NotNil(t, dc.LogWriter())

	require.NoError(t, dc.StopProfiling())

	// Check artifacts exist
	artifacts := dc.Artifacts()
	require.Len(t, artifacts, 3, "should have cpu, heap, and log artifacts")

	// Verify cpu.prof exists
	cpuPath := filepath.Join(dc.dir, "cpu.prof")
	_, err = os.Stat(cpuPath)
	assert.NoError(t, err)

	// Verify heap.prof exists
	heapPath := filepath.Join(dc.dir, "heap.prof")
	_, err = os.Stat(heapPath)
	assert.NoError(t, err)

	// Verify task.log exists
	logPath := filepath.Join(dc.dir, "task.log")
	_, err = os.Stat(logPath)
	assert.NoError(t, err)
}

func TestDebugCollector_WriteTimingSummary(t *testing.T) {
	dc, err := NewDebugCollector("task-1", "attempt-1", "sub-1")
	require.NoError(t, err)
	defer dc.Cleanup()

	require.NoError(t, dc.Start())
	require.NoError(t, dc.StopProfiling())
	require.NoError(t, dc.WriteTimingSummary("scan"))

	// Verify timing.json exists
	timingPath := filepath.Join(dc.dir, "timing.json")
	_, err = os.Stat(timingPath)
	assert.NoError(t, err)

	// After writing timing, artifacts should include timing
	artifacts := dc.Artifacts()
	assert.Len(t, artifacts, 4, "should have cpu, heap, log, and timing artifacts")
}

func TestIsDebugEnabled(t *testing.T) {
	assert.False(t, isDebugEnabled(nil))
	assert.False(t, isDebugEnabled(map[string]string{}))
	assert.False(t, isDebugEnabled(map[string]string{"debug_enabled": "false"}))
	assert.True(t, isDebugEnabled(map[string]string{"debug_enabled": "true"}))
	assert.True(t, isDebugEnabled(map[string]string{"debug_enabled": "True"}))
}

func TestDebugObjectKey(t *testing.T) {
	key := debugObjectKey("task-1", "attempt-1", "debug_pprof_cpu", "cpu.prof")
	assert.Equal(t, "debug/task-1/attempt-1/debug_pprof_cpu/cpu.prof", key)
}
