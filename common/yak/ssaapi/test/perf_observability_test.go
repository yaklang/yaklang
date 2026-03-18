package ssaapi

import (
	"bytes"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/diagnostics"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

// captureStdout runs fn and returns everything printed to stdout.
func captureStdout(fn func()) string {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	defer func() { os.Stdout = old }()
	fn()
	w.Close()
	var buf bytes.Buffer
	buf.ReadFrom(r)
	return buf.String()
}

// TestPerfObservability_RecorderSnapshot 特征测试：filePerformanceRecorder snapshot 结构
// 必须包含 AST[path]、Build[path]，且 Kind 正确（AST/Build），防止重构破坏契约
func TestPerfObservability_RecorderSnapshot(t *testing.T) {
	origLevel := diagnostics.GetLevel()
	defer diagnostics.SetLevel(origLevel)
	diagnostics.SetLevel(diagnostics.LevelHigh) // file perf 需要 LevelHigh

	fs := filesys.NewVirtualFs()
	fs.AddFile("main.yak", `a = 1`)
	progs, err := ssaapi.ParseProjectWithFS(fs,
		ssaapi.WithLanguage(ssaconfig.Yak),
		ssaapi.WithMemory(),
		ssaapi.WithProgramName("perf-snapshot-test"),
	)
	require.NoError(t, err)
	require.NotEmpty(t, progs)

	cfg := progs[0].GetConfig()
	fileRec := cfg.DiagnosticsRecorder()
	require.NotNil(t, fileRec, "file perf recorder should exist when LevelHigh")
	fileSnap := fileRec.Snapshot()

	var hasAST, hasBuild bool
	for _, m := range fileSnap {
		if strings.HasPrefix(m.Name, "AST[") {
			hasAST = true
			require.Equal(t, ssa.TrackKindAST, m.Kind, "AST entry must have Kind AST")
		}
		if strings.HasPrefix(m.Name, "Build[") {
			hasBuild = true
			require.Equal(t, ssa.TrackKindBuild, m.Kind, "Build entry must have Kind Build")
		}
	}
	require.True(t, hasAST, "file snapshot should contain AST[path]; got: %v", namesFromSnapshot(fileSnap))
	require.True(t, hasBuild, "file snapshot should contain Build[path]; got: %v", namesFromSnapshot(fileSnap))
}

func namesFromSnapshot(snap []diagnostics.Measurement) []string {
	names := make([]string, len(snap))
	for i, m := range snap {
		names[i] = m.Name
	}
	return names
}

// TestPerfObservability_PerformanceTableOutput 特征测试：项目编译输出必须包含性能表格、表头、数据行
func TestPerfObservability_PerformanceTableOutput(t *testing.T) {
	origLevel := diagnostics.GetLevel()
	defer diagnostics.SetLevel(origLevel)
	diagnostics.SetLevel(diagnostics.LevelHigh)

	fs := filesys.NewVirtualFs()
	fs.AddFile("main.yak", `a = 1`)
	out := captureStdout(func() {
		_, err := ssaapi.ParseProjectWithFS(fs,
			ssaapi.WithLanguage(ssaconfig.Yak),
			ssaapi.WithMemory(),
			ssaapi.WithProgramName("perf-table-test"),
		)
		require.NoError(t, err)
	})
	// 表格标题：AST/Build 分开输出
	require.True(t, strings.Contains(out, "AST Performance") || strings.Contains(out, "Build Performance"), "output should have AST or Build table")
	require.Contains(t, out, "Name")
	require.Contains(t, out, "Duration")
	require.Contains(t, out, "=", "table should have separator lines")
	// 至少一行数据（AST[path] 或 | 分隔的数据行）
	lines := strings.Split(out, "\n")
	var dataRows int
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" && !strings.HasPrefix(trimmed, "=") &&
			trimmed != "AST Performance" && trimmed != "Build Performance" &&
			trimmed != "Name" && !strings.HasPrefix(trimmed, "+-") {
			if strings.Contains(line, "AST[") || strings.Contains(line, "|") {
				dataRows++
			}
		}
	}
	require.Greater(t, dataRows, 0, "output should have at least one data row")
}

// TestPerfObservability_E2EProjectCompileOutput 端到端特征测试：完整项目编译输出契约
// 防止重构后行为静默变化（表格、树）
func TestPerfObservability_E2EProjectCompileOutput(t *testing.T) {
	origLevel := diagnostics.GetLevel()
	defer diagnostics.SetLevel(origLevel)
	diagnostics.SetLevel(diagnostics.LevelHigh)

	fs := filesys.NewVirtualFs()
	fs.AddFile("main.yak", `a = 1`)
	out := captureStdout(func() {
		_, err := ssaapi.ParseProjectWithFS(fs,
			ssaapi.WithLanguage(ssaconfig.Yak),
			ssaapi.WithMemory(),
			ssaapi.WithProgramName("e2e-perf-test"),
		)
		require.NoError(t, err)
	})
	// 1. 性能表格
	require.Contains(t, out, "Compile Performance", "must have Compile Performance table")
	require.Contains(t, out, "Name")
	require.Contains(t, out, "Duration")
	// 2. AST/Build 分开输出
	require.True(t, strings.Contains(out, "AST Performance") || strings.Contains(out, "Build Performance"), "must have AST or Build table")
}

// TestPerfObservability_PresenterWithBuildTree 特征测试：Presenter 输出 AST/Build 表格
func TestPerfObservability_PresenterWithBuildTree(t *testing.T) {
	rec := diagnostics.NewRecorder()
	rec.Track(ssa.TrackKindAST, "AST[x.yak]", func() error { return nil })
	rec.Track(ssa.TrackKindBuild, "LazyBuild", func() error {
		time.Sleep(150 * time.Microsecond)
		return nil
	})

	var payloads []diagnostics.DisplayPayload
	snap := rec.Snapshot()
	astMeas := diagnostics.FilterByTrackKind(snap, ssa.TrackKindAST)
	buildMeas := diagnostics.FilterByTrackKind(snap, ssa.TrackKindBuild)
	if len(astMeas) > 0 {
		payloads = append(payloads, diagnostics.TablePayloadFromMeasurements("AST Performance", astMeas))
	}
	if len(buildMeas) > 0 {
		payloads = append(payloads, diagnostics.TablePayloadFromMeasurements("Build Performance", buildMeas,
			diagnostics.TableIndentByDepth(true), diagnostics.TableBuildStyle(true)))
	}
	out := captureStdout(func() {
		diagnostics.SetLevel(diagnostics.LevelHigh)
		for _, p := range payloads {
			if p != nil {
				diagnostics.LogTable(ssa.TrackKindBuild, p, true)
			}
		}
	})
	require.Contains(t, out, "AST Performance")
	require.Contains(t, out, "Build Performance")
	require.Contains(t, out, "LazyBuild")
}

// TestPerfObservability_FileSummaryMsKB 特征测试：MergeFilePerfForDisplay 排序 + 表格格式
// ms/KB 降序，表格必须含 Size、ms/KB 列
func TestPerfObservability_FileSummaryMsKB(t *testing.T) {
	measurements := []diagnostics.Measurement{
		{Name: "AST[a.yak]", Total: 5 * time.Millisecond, Count: 1, Size: 1024},
		{Name: "AST[b.yak]", Total: 10 * time.Millisecond, Count: 1, Size: 2048},
		{Name: "Build[c.yak]", Total: 20 * time.Millisecond, Count: 1, Size: 512},
	}
	merged := make([]diagnostics.Measurement, len(measurements))
	copy(merged, measurements)
	diagnostics.SortMeasurementsByMsPerKB(merged)
	require.Len(t, merged, 3)
	// Higher ms/KB first: c.yak 20ms/0.5KB=40, a.yak 5ms/1KB=5, b.yak 10ms/2KB=5
	// So c.yak first, then a and b (same ratio, a has higher Total)
	require.Equal(t, "Build[c.yak]", merged[0].Name)
	headers, rows := diagnostics.MeasurementsToRows(merged)
	table := diagnostics.FormatTable("File Summary", headers, rows)
	require.Contains(t, table, "Name")
	require.Contains(t, table, "Duration")
	require.Contains(t, table, "Size")
	require.Contains(t, table, "ms/KB")
	require.Contains(t, table, "a.yak")
	require.Contains(t, table, "b.yak")
	require.Contains(t, table, "c.yak")
}

// TestPerfObservability_BuildTreeOutput 特征测试：TrackBuild 使用 TrackKindBuild，LazyBuild 进入 Build Performance 表格
func TestPerfObservability_BuildTreeOutput(t *testing.T) {
	origLevel := diagnostics.GetLevel()
	defer diagnostics.SetLevel(origLevel)
	diagnostics.SetLevel(diagnostics.LevelNormal)

	rec := diagnostics.NewRecorder()
	_ = ssa.TrackBuildWithOptions(rec, "LazyBuild", func() error {
		time.Sleep(200 * time.Microsecond)
		return nil
	}, ssa.WithTrackDepthEnabled(true))

	snap := rec.Snapshot()
	buildMeas := diagnostics.FilterByTrackKind(snap, ssa.TrackKindBuild)
	require.NotEmpty(t, buildMeas, "LazyBuild should appear in Build Performance")
	require.Equal(t, "LazyBuild", buildMeas[0].Name)

	compileMeas := ssaapi.FilterCompilePerFile(snap, ssa.TrackKindBuild)
	require.Empty(t, compileMeas, "no per-file compile in this test")

	var payloads []diagnostics.DisplayPayload
	payloads = append(payloads, diagnostics.TablePayloadFromMeasurements("Build Performance", buildMeas,
		diagnostics.TableIndentByDepth(true), diagnostics.TableBuildStyle(true)))

	out := captureStdout(func() {
		diagnostics.SetLevel(diagnostics.LevelHigh)
		for _, p := range payloads {
			if p != nil {
				diagnostics.LogTable(ssa.TrackKindBuild, p, true)
			}
		}
	})
	require.Contains(t, out, "Build Performance")
	require.Contains(t, out, "LazyBuild")
	require.Contains(t, out, "µs", "duration format")
	require.Contains(t, out, "=", "border lines")
}
