package scannode

import (
	"os"
	"path/filepath"
	"runtime/pprof"
	"testing"
	"time"

	"github.com/google/pprof/profile"
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
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "db-stats"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "runtime-stats"), 0o755))

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

	require.NoError(t, os.WriteFile(filepath.Join(dir, "db-stats", "20260805-120030-initial.db.json"), []byte(`{
  "dialect": "postgres",
  "ops": {
    "query": {"count": 10, "total_ms": 100, "avg_ms": 10, "min_ms": 1, "max_ms": 40},
    "create": {"count": 2, "total_ms": 20, "avg_ms": 10}
  },
  "total_count": 12,
  "total_ms": 120
}`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "db-stats", "20260805-121000-121000.db.json"), []byte(`{
  "dialect": "postgres",
  "ops": {
    "query": {"count": 3, "total_ms": 30, "avg_ms": 10}
  },
  "total_count": 3,
  "total_ms": 30
}`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "runtime-stats", "20260805-120030-initial.runtime.json"), []byte(`{
  "timestamp": "2026-08-05T12:00:30Z",
  "num_cpu": 32,
  "load1": 2.5,
  "host_cpu_percent": 41.2,
  "process_cpu_percent": 9.6,
  "host_mem_total_bytes": 34359738368,
  "host_mem_used_bytes": 8589934592,
  "host_mem_available_bytes": 25769803776,
  "process_rss_bytes": 765460480,
  "process_heap_alloc_bytes": 512000000,
  "process_heap_sys_bytes": 600000000,
  "goroutines": 41
}`), 0o644))

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

	var initial *SampleSummary
	for i := range result.Samples {
		if result.Samples[i].Label == "20260805-120030-initial" {
			initial = &result.Samples[i]
			break
		}
	}
	require.NotNil(t, initial)
	require.NotNil(t, initial.DBStats)
	assert.Equal(t, "postgres", initial.DBStats.Dialect)
	assert.Equal(t, int64(12), initial.DBStats.TotalCount)
	require.NotNil(t, initial.Runtime)
	assert.Equal(t, 32, initial.Runtime.NumCPU)
	assert.InDelta(t, 41.2, initial.Runtime.HostCPUPercent, 0.01)
	assert.InDelta(t, 9.6, initial.Runtime.ProcessCPUPercent, 0.01)
	assert.Equal(t, uint64(765460480), initial.Runtime.ProcessRSSBytes)
	require.NotNil(t, result.Summary.DBStatsTotal)
	assert.Equal(t, int64(15), result.Summary.DBStatsTotal.TotalCount)
	assert.Equal(t, int64(150), result.Summary.DBStatsTotal.TotalMs)
	assert.Equal(t, int64(13), result.Summary.DBStatsTotal.Ops["query"].Count)
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
	require.NotNil(t, detail.DBStats)
	assert.Equal(t, int64(12), detail.DBStats.TotalCount)
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

func TestIsYaklangPprofFunc(t *testing.T) {
	assert.True(t, isYaklangPprofFunc("github.com/yaklang/yaklang/common/yak/ssa.Scan"))
	assert.False(t, isYaklangPprofFunc("runtime.mallocgc"))
}

func TestApplySampleWindow(t *testing.T) {
	start := time.Date(2026, 8, 26, 13, 0, 23, 0, time.UTC)
	s := &SampleSummary{Timestamp: &start}
	applySampleWindow(s, &PprofTopAnalysis{DurationNanos: int64(60 * time.Second)})
	require.NotNil(t, s.EndedAt)
	assert.Equal(t, int64(60_000), s.DurationMS)
	assert.Equal(t, start.Add(60*time.Second), *s.EndedAt)
}

func TestPprofTopFromStatsYaklang(t *testing.T) {
	// Caller must pass cum-desc sorted stats (same as buildPprofTopAnalysis).
	stats := []*pprofFuncStats{
		{name: "runtime.mallocgc", cum: 100, flat: 90},
		{name: "github.com/yaklang/yaklang/common/yak/ssa.A", cum: 50, flat: 25},
		{name: "github.com/yaklang/yaklang/common/yak/ssa.B", cum: 40, flat: 20},
	}
	top := pprofTopFromStats(stats, 190, 10, isYaklangPprofFunc)
	require.Len(t, top, 2)
	assert.Equal(t, "github.com/yaklang/yaklang/common/yak/ssa.A", top[0].Name)
	assert.Equal(t, "github.com/yaklang/yaklang/common/yak/ssa.B", top[1].Name)
}

func TestBuildPprofTopAnalysis_Stacks(t *testing.T) {
	fRoot := &profile.Function{ID: 1, Name: "main.main"}
	fMid := &profile.Function{ID: 2, Name: "github.com/yaklang/yaklang/common/yak/ssaapi.Compile"}
	fLeaf := &profile.Function{ID: 3, Name: "github.com/yaklang/gorm.(*DB).FirstOrCreate"}
	fOther := &profile.Function{ID: 4, Name: "runtime.systemstack"}

	loc := func(id uint64, fn *profile.Function) *profile.Location {
		return &profile.Location{ID: id, Line: []profile.Line{{Function: fn}}}
	}
	// Leaf at location[0], root at the end — Go pprof convention.
	p := &profile.Profile{
		SampleType: []*profile.ValueType{{Type: "cpu", Unit: "nanoseconds"}},
		Sample: []*profile.Sample{
			{
				Location: []*profile.Location{loc(1, fLeaf), loc(2, fMid), loc(3, fRoot)},
				Value:    []int64{80},
			},
			{
				Location: []*profile.Location{loc(4, fOther), loc(5, fRoot)},
				Value:    []int64{20},
			},
		},
	}

	result := buildPprofTopAnalysis(p, "cpu")
	require.Empty(t, result.ParseError)
	require.NotEmpty(t, result.TopStacks)
	assert.Equal(t, []string{
		"main.main",
		"github.com/yaklang/yaklang/common/yak/ssaapi.Compile",
		"github.com/yaklang/gorm.(*DB).FirstOrCreate",
	}, result.TopStacks[0].Frames)
	assert.Equal(t, int64(80), result.TopStacks[0].Value)
	assert.Equal(t, "80.00%", result.TopStacks[0].Pct)

	require.NotEmpty(t, result.YaklangStacks)
	assert.Contains(t, result.YaklangStacks[0].Frames, "github.com/yaklang/yaklang/common/yak/ssaapi.Compile")
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

func TestAnalyzeDebugRun_EnrichesPhasesFromTaskLog(t *testing.T) {
	nodeRoot := t.TempDir()
	debugDir := filepath.Join(nodeRoot, "debug-runs", "debug", "job-1_attempt-1")
	require.NoError(t, os.MkdirAll(filepath.Join(debugDir, "cpu-pprof"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(nodeRoot, "logs"), 0o755))

	// Collector log has compile errors but no SSA phase markers.
	require.NoError(t, os.WriteFile(
		filepath.Join(debugDir, "log"),
		[]byte("[ERRO] 2026-08-26 19:27:12 [ssaLog:ssa_compile_fs:360] parse failed\n"),
		0o644,
	))
	require.NoError(t, os.WriteFile(
		filepath.Join(nodeRoot, "logs", "job-1_subtask_attempt-1.log"),
		[]byte(`[INFO] 2026-08-26 19:26:16 {"id":"ssa-phase","data":"compile","tags":[]}
[INFO] 2026-08-26 19:26:16 SSA 任务阶段: compile
`),
		0o644,
	))
	writeFakePprof(t, filepath.Join(debugDir, "cpu-pprof", "20260826-192720-initial.cpu.prof"))

	result := AnalyzeDebugRunWithStatus(debugDir, "running")
	require.True(t, hasRecognizedDebugPhases(result.Phases), "phases=%+v", result.Phases)
	assert.Equal(t, "compile", result.Phases[0].Phase)
	require.NotEmpty(t, result.Samples)
	assert.Equal(t, "compile", result.Samples[0].Phase)
	assert.Equal(t, "log_inferred", result.Samples[0].PhaseSource)
}
