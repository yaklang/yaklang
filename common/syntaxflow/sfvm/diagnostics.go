package sfvm

import (
	"errors"

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

// trackStatementWithError executes a statement with diagnostics tracking and handles common errors
func (s *SFFrame) trackStatementWithError(name string, fn func() error) error {
	return s.trackWithError(name, func() error {
		err := fn()
		if err != nil {
			s.debugSubLog("execStatement error: %v", err)
			if errors.Is(err, AbortError) {
				return nil
			}
			if errors.Is(err, CriticalError) {
				return err
			}
			// go to expression end
			if result := s.errorSkipStack.Peek(); result != nil {
				s.idx = result.end
				return nil
			}
			return err
		}
		return nil
	})
}
