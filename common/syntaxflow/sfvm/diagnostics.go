package sfvm

import (
	"time"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/diagnostics"
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
		return recorder.TrackTrace(name, fn)
	}
	return fn()
}

func (s *SFFrame) logScanPerformance(totalDuration time.Duration, enableRulePerf bool) {
	ruleRecorder := s.GetDiagnosticsRecorder()
	log.Infof("=== Scan Total ===")
	log.Infof("SyntaxFlow VM Run Finish Time: %v", totalDuration)
	log.Infof("==================")

	if enableRulePerf && ruleRecorder != nil {
		for _, item := range ruleRecorder.Snapshot() {
			if item.Total > totalDuration/5 {
				log.Infof("Rule Performance: %s", item.String())
			}
		}
	}
}
