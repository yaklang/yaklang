package sfvm

import (
	"fmt"
	"time"

	"github.com/yaklang/yaklang/common/utils/diagnostics"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

// GetDiagnosticsRecorder returns the diagnostics recorder if diagnostics are enabled
func (s *SFFrame) GetDiagnosticsRecorder() *diagnostics.Recorder {
	if s.config != nil && s.config.diagnosticsEnabled && s.config.diagnosticsRecorder != nil {
		return s.config.diagnosticsRecorder
	}
	return nil
}

// track executes a function with diagnostics tracking if enabled
func (s *SFFrame) track(name string, fn func() error) error {
	if recorder := s.GetDiagnosticsRecorder(); recorder != nil {
		_, err := recorder.ForKind(ssa.TrackKindScan).Track(name, fn)
		return err
	}
	return fn()
}

// logScanPerformance 使用统一 diagnostics API：总时间用 LogLow，慢规则表格用 LogTable（LevelHigh）
func (s *SFFrame) logScanPerformance(totalDuration time.Duration, enableRulePerf bool) {
	if totalDuration < 1*time.Second {
		return
	}
	// 总时间：LogLow（LevelLow 及以上输出）
	diagnostics.LogLow(ssa.TrackKindScan, "", fmt.Sprintf("SyntaxFlow VM total elapsed %v", totalDuration))

	ruleRecorder := s.GetDiagnosticsRecorder()
	if !enableRulePerf || ruleRecorder == nil {
		return
	}
	snap := ruleRecorder.Snapshot()
	if len(snap) == 0 {
		return
	}
	// 过滤耗时超过总时长 1/5 的慢规则
	var slowMeas []diagnostics.Measurement
	for _, item := range snap {
		if item.Total > totalDuration/5 {
			slowMeas = append(slowMeas, item)
		}
	}
	if len(slowMeas) == 0 {
		return
	}
	headers, rows := diagnostics.MeasurementsToRows(slowMeas)
	if len(rows) > 0 {
		diagnostics.LogTable(ssa.TrackKindScan, &diagnostics.TablePayload{Title: "SyntaxFlow VM Slow Rules", Headers: headers, Rows: rows}, false)
	}
}
