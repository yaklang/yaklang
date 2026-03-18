package ssa

import "github.com/yaklang/yaklang/common/utils/diagnostics"

func (prog *Program) SetDiagnosticsRecorder(rec *diagnostics.Recorder) {
	if prog == nil {
		return
	}
	prog.diagnosticsRecorder = rec
}

func (prog *Program) DiagnosticsRecorder() *diagnostics.Recorder {
	if prog == nil {
		return nil
	}
	return prog.diagnosticsRecorder
}
