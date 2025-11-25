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

// trackWithError executes a function with diagnostics tracking if enabled
func (s *SFFrame) trackWithError(name string, fn func() error) error {
	if recorder := s.GetDiagnosticsRecorder(); recorder != nil {
		_, err := recorder.TrackWithError(true, name, fn)
		return err
	}
	return fn()
}

func (s *SFFrame) logScanPerformance(totalDuration time.Duration, enableRulePerf bool) {
	ruleRecorder := s.GetDiagnosticsRecorder()
	log.Infof("=== Scan Total ===")
	log.Infof("SyntaxFlow VM Run Finish Time: %v", totalDuration)
	log.Infof("==================")

	if enableRulePerf && ruleRecorder != nil {
		ruleRecorder.Log("Rule Performance (scan)")
	}
}
