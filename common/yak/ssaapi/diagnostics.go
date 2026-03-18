package ssaapi

import (
	"strings"

	"github.com/yaklang/yaklang/common/utils/diagnostics"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

// FilterCompilePerFile 只保留 buildKind 中 per-file 的测量（compile /path、Build[path]），排除 LazyBuild
func FilterCompilePerFile(snap []diagnostics.Measurement, buildKind diagnostics.TrackKind) []diagnostics.Measurement {
	out := make([]diagnostics.Measurement, 0, len(snap))
	for _, m := range snap {
		if m.Kind != buildKind {
			continue
		}
		if m.Name == "LazyBuild" || strings.HasPrefix(m.Name, "LazyBuild[") {
			continue
		}
		out = append(out, m)
	}
	return out
}

// DiagnosticsEnabled 由环境变量 YAK_DIAGNOSTICS_LOG_LEVEL 控制，LevelNormal 及以上时为 true（格式化打印可见）
func (c *Config) DiagnosticsEnabled() bool {
	return c != nil && diagnostics.Enabled(diagnostics.LevelNormal)
}

// DiagnosticsRecorder 返回统一 Recorder；显式传入优先，否则按 LevelNormal 及以上懒创建
func (c *Config) DiagnosticsRecorder() *diagnostics.Recorder {
	if c == nil {
		return diagnostics.DefaultRecorder()
	}
	if c.perfRecorder != nil {
		return c.perfRecorder
	}
	if !diagnostics.Enabled(diagnostics.LevelNormal) {
		return diagnostics.DefaultRecorder()
	}
	c.perfRecorder = diagnostics.NewRecorder()
	return c.perfRecorder
}

func (c *Config) SetDiagnosticsRecorder(rec *diagnostics.Recorder) {
	c.perfRecorder = rec
}

// WithDiagnosticsRecorder 显式传入 recorder，用于性能捕获（如 ParseWithPerformanceCapture）
var WithDiagnosticsRecorder = ssaconfig.SetOption("ssa_compile/diagnostics_recorder", func(c *Config, rec *diagnostics.Recorder) {
	c.perfRecorder = rec
})

// LogDiagnostics 按 Kind 分开展示：AST Performance、Build Performance、Database Performance、Others
func (c *Config) LogDiagnostics(label string) {
	if c == nil {
		return
	}
	recorder := c.DiagnosticsRecorder()
	if recorder == nil {
		return
	}
	LogCompileByKind(recorder, label)
}

// LogCompileByKind 按 SSA Kind 分开展示：AST、Build、Database、Others
func LogCompileByKind(rec *diagnostics.Recorder, label string) {
	if rec == nil {
		return
	}
	snap := rec.Snapshot()
	if len(snap) == 0 {
		return
	}
	suffix := ""
	if label != "" {
		suffix = " [" + label + "]"
	}

	astMeas := diagnostics.FilterByTrackKind(snap, ssa.TrackKindAST)
	buildMeas := diagnostics.FilterByTrackKind(snap, ssa.TrackKindBuild)
	dbMeas := diagnostics.FilterByTrackKind(snap, ssa.TrackKindDatabase)
	others := filterOthersExclude(snap, ssa.TrackKindAST, ssa.TrackKindBuild, ssa.TrackKindDatabase)

	if len(astMeas) > 0 {
		payload := diagnostics.TablePayloadFromMeasurements("AST Performance"+suffix, astMeas)
		diagnostics.LogTable(ssa.TrackKindAST, payload, true)
	}
	if len(buildMeas) > 0 {
		diagnostics.SortMeasurementsByDepthAndTotal(buildMeas, nil)
		payload := diagnostics.TablePayloadFromMeasurements("Build Performance"+suffix, buildMeas,
			diagnostics.TableIndentByDepth(true), diagnostics.TableBuildStyle(true))
		diagnostics.LogTable(ssa.TrackKindBuild, payload, true)
	}
	if len(dbMeas) > 0 {
		payload := diagnostics.TablePayloadFromMeasurements("Database Performance Summary"+suffix, dbMeas, diagnostics.TableIncludeCount(true))
		diagnostics.LogTable(ssa.TrackKindDatabase, payload, true)
	}
	if len(others) > 0 {
		payload := diagnostics.TablePayloadFromMeasurements("Other"+suffix, others)
		diagnostics.LogTable(diagnostics.TrackKindGeneral, payload, true)
	}
}

func filterOthersExclude(snap []diagnostics.Measurement, exclude ...diagnostics.TrackKind) []diagnostics.Measurement {
	excl := make(map[diagnostics.TrackKind]bool)
	for _, k := range exclude {
		excl[k] = true
	}
	out := make([]diagnostics.Measurement, 0, len(snap))
	for _, m := range snap {
		if !excl[m.Kind] {
			out = append(out, m)
		}
	}
	return out
}

func (c *Config) DiagnosticsTrack(name string, steps ...func() error) error {
	return diagnostics.RunStepsWithTrack(c.DiagnosticsRecorder(), name, steps...)
}

func (c *saveValueCtx) DiagnosticsTrack(name string, steps ...func() error) error {
	return diagnostics.RunStepsWithTrack(c.diagnosticsRecorder, name, steps...)
}

func OptionSaveValue_Diagnostics(rec *diagnostics.Recorder) SaveValueOption {
	return func(c *saveValueCtx) {
		c.diagnosticsRecorder = rec
	}
}

