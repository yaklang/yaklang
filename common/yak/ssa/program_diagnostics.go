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

func (prog *Program) DiagnosticsTrack(name string, steps ...func() error) {
	_ = prog.DiagnosticsTrackErr(name, steps...)
}

func (prog *Program) DiagnosticsTrackErr(name string, steps ...func() error) error {
	if len(steps) == 0 {
		return nil
	}
	if prog == nil {
		for _, step := range steps {
			if step != nil {
				if err := step(); err != nil {
					return err
				}
			}
		}
		return nil
	}
	if rec := prog.DiagnosticsRecorder(); rec != nil {
		return rec.Track(name, steps...)
	}
	for _, step := range steps {
		if step != nil {
			if err := step(); err != nil {
				return err
			}
		}
	}
	return nil
}

func (c *ProgramCache) diagnosticsTrack(name string, steps ...func() error) {
	_ = c.diagnosticsTrackErr(name, steps...)
}

func (c *ProgramCache) diagnosticsTrackErr(name string, steps ...func() error) error {
	if c == nil || len(steps) == 0 {
		return nil
	}
	if prog := c.program; prog != nil {
		return prog.DiagnosticsTrackErr(name, steps...)
	}
	for _, step := range steps {
		if step != nil {
			if err := step(); err != nil {
				return err
			}
		}
	}
	return nil
}
