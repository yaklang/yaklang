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

func (prog *Program) DiagnosticsTrack(name string, steps ...func()) {
	if len(steps) == 0 {
		return
	}
	if prog == nil {
		for _, step := range steps {
			if step != nil {
				step()
			}
		}
		return
	}
	rec := prog.DiagnosticsRecorder()
	if rec != nil {
		rec.Track(true, name, steps...)
		return
	}
	for _, step := range steps {
		if step != nil {
			step()
		}
	}
}

func (prog *Program) DiagnosticsTrackWithError(name string, steps ...diagnostics.StepFunc) (diagnostics.Measurement, error) {
	empty := diagnostics.Measurement{Name: name}
	rec := prog.DiagnosticsRecorder()
	if rec != nil {
		return rec.TrackWithError(true, name, steps...)
	}
	for _, step := range steps {
		if step == nil {
			continue
		}
		if err := step(); err != nil {
			return empty, err
		}
	}
	return empty, nil
}
