package scannode

import (
	"os"
	"path/filepath"
	"runtime/pprof"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// createTestDebugDir creates a realistic debug directory fixture for testing.
func createTestDebugDir(t *testing.T) string {
	dir := t.TempDir()

	// Create subdirectories
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "cpu-pprof"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "memory-pprof"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "goroutine-pprof"), 0o755))

	// Write cmd.txt
	require.NoError(t, os.WriteFile(filepath.Join(dir, "cmd.txt"), []byte("test command"), 0o644))

	// Write a fake ssadb.db
	require.NoError(t, os.WriteFile(filepath.Join(dir, "ssadb.db"), []byte("fake db"), 0o644))

	// Write log with compile/scan phases
	logContent := `2026/08/05 12:00:00 [INFO] ============= start to scan code =============
2026/08/05 12:00:00 [INFO] [code-scan] mode: compile + scan via syntaxflow_scan.StartScan (batch/CI report path)
2026/08/05 12:00:00 [INFO] sync rule from embed to database success, cost 2.3s
2026/08/05 12:00:01 [INFO] [pprof] collector started, HTTP server on 127.0.0.1:18080
2026/08/05 12:00:30 [INFO] SSA 任务阶段: compile
2026/08/05 12:05:00 [INFO] SSA 任务阶段: scan
2026/08/05 12:05:00 [INFO] [pprof] CPU profile saved: cpu-pprof/20260805-120030-initial.cpu.prof
2026/08/05 12:10:00 [INFO] [pprof] CPU profile saved: cpu-pprof/20260805-121000-121000.cpu.prof
2026/08/05 12:15:00 [INFO] [pprof] CPU profile saved: cpu-pprof/20260805-121500-121500.cpu.prof
2026/08/05 12:30:00 [INFO] code scan done
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "log"), []byte(logContent), 0o644))

	// Create fake pprof files
	writeFakePprof(t, filepath.Join(dir, "cpu-pprof", "20260805-120030-initial.cpu.prof"))
	writeFakePprof(t, filepath.Join(dir, "cpu-pprof", "20260805-121000-121000.cpu.prof"))
	writeFakePprof(t, filepath.Join(dir, "memory-pprof", "20260805-120030-initial.mem.prof"))
	writeFakePprof(t, filepath.Join(dir, "goroutine-pprof", "20260805-120030-initial.goroutine.prof"))

	return dir
}

func writeFakePprof(t *testing.T, path string) {
	f, err := os.Create(path)
	require.NoError(t, err)
	defer f.Close()
	// Write a valid heap profile
	require.NoError(t, pprof.WriteHeapProfile(f))
}

func TestAnalyzeDebugRun_Complete(t *testing.T) {
	dir := createTestDebugDir(t)
	result := AnalyzeDebugRun(dir)

	assert.Equal(t, "completed", result.Status)
	require.NotNil(t, result.StartedAt)
	require.NotNil(t, result.FinishedAt)
	assert.NotEmpty(t, result.Duration)
	assert.NotEmpty(t, result.Phases)
	assert.NotEmpty(t, result.Samples)
	require.NotNil(t, result.Summary)
	assert.Equal(t, 2, result.Summary.CPUProfileFiles)
	assert.Equal(t, 1, result.Summary.HeapProfileFiles)
	assert.True(t, result.Summary.CompilePhaseFound)
	assert.True(t, result.Summary.ScanPhaseFound)
}

func TestAnalyzeDebugRun_MissingDir(t *testing.T) {
	result := AnalyzeDebugRun("/nonexistent/path")
	assert.Equal(t, "error", result.Status)
	assert.NotEmpty(t, result.Errors)
}

func TestAnalyzeDebugRun_PartialFiles(t *testing.T) {
	dir := t.TempDir()
	// Only create a log file, no pprof dirs
	require.NoError(t, os.WriteFile(filepath.Join(dir, "log"), []byte("2026/08/05 12:00:00 [INFO] start\n"), 0o644))

	result := AnalyzeDebugRun(dir)

	// Should not crash
	assert.Empty(t, result.Samples)
	// Status should be running (no completion marker)
	assert.Equal(t, "running", result.Status)
}

func TestAnalyzeDebugRun_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	result := AnalyzeDebugRun(dir)

	assert.NotEqual(t, "completed", result.Status)
	assert.True(t, result.Partial)
}

func TestAnalyzeDebugRun_Running(t *testing.T) {
	dir := t.TempDir()
	// Log without completion marker
	logContent := "2026/08/05 12:00:00 [INFO] start to scan code\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "log"), []byte(logContent), 0o644))

	result := AnalyzeDebugRun(dir)
	assert.Equal(t, "running", result.Status)
}

func TestAnalyzeDebugRun_Failed(t *testing.T) {
	dir := t.TempDir()
	logContent := "2026/08/05 12:00:00 [INFO] start\n2026/08/05 12:01:00 [ERROR] code scan failed: something went wrong\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "log"), []byte(logContent), 0o644))

	result := AnalyzeDebugRun(dir)
	assert.Equal(t, "failed", result.Status)
}

func TestAnalyzeSample_WithPprof(t *testing.T) {
	dir := createTestDebugDir(t)
	detail, err := AnalyzeSample(dir, "20260805-120030-initial")
	require.NoError(t, err)
	assert.True(t, detail.HasCPU)
	assert.True(t, detail.HasHeap)
	assert.True(t, detail.HasGoroutine)
	// CPU profile should parse successfully (it's a heap profile written as cpu, but still valid pprof)
	if detail.CPUProfile != nil {
		assert.NotEmpty(t, detail.CPUProfile.TopFunctions)
	}
}

func TestAnalyzeSample_MissingLabel(t *testing.T) {
	dir := t.TempDir()
	detail, err := AnalyzeSample(dir, "nonexistent")
	require.NoError(t, err)
	assert.False(t, detail.HasCPU)
	assert.False(t, detail.HasHeap)
}

func TestGenerateDebugZip(t *testing.T) {
	dir := createTestDebugDir(t)
	zipPath, err := GenerateDebugZip(dir)
	require.NoError(t, err)
	assert.NotEmpty(t, zipPath)

	info, err := os.Stat(zipPath)
	require.NoError(t, err)
	assert.True(t, info.Size() > 0, "zip should not be empty")
}

func TestGenerateDebugZip_PathTraversal(t *testing.T) {
	_, err := GenerateDebugZip("../../../etc/passwd")
	assert.Error(t, err)
}

func TestParseLabelTimestamp(t *testing.T) {
	ts := parseLabelTimestamp("20260805-120030-initial")
	require.NotNil(t, ts)
	assert.Equal(t, 2026, ts.Year())
	assert.Equal(t, 12, ts.Hour())
}

func TestParseLabelTimestamp_Invalid(t *testing.T) {
	assert.Nil(t, parseLabelTimestamp("invalid"))
}

func TestParseLogTimestamp(t *testing.T) {
	ts := parseLogTimestamp("2026/08/05 12:00:00 [INFO] test")
	require.NotNil(t, ts)
	assert.Equal(t, 2026, ts.Year())
}

func TestAssignPhase(t *testing.T) {
	start := time.Date(2026, 8, 5, 12, 0, 0, 0, time.Local)
	scanStart := time.Date(2026, 8, 5, 12, 5, 0, 0, time.Local)
	finish := time.Date(2026, 8, 5, 12, 30, 0, 0, time.Local)
	phases := []PhaseAnalysis{
		{Phase: "compile", Source: "log_inferred", StartedAt: &start, FinishedAt: &scanStart},
		{Phase: "scan", Source: "log_inferred", StartedAt: &scanStart, FinishedAt: &finish},
	}

	phase, source := assignPhase(&start, phases)
	assert.Equal(t, "compile", phase)
	assert.NotEmpty(t, source)

	midScan := time.Date(2026, 8, 5, 12, 10, 0, 0, time.Local)
	phase, _ = assignPhase(&midScan, phases)
	assert.Equal(t, "scan", phase)
}
