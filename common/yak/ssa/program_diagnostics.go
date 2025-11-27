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

func (c *ProgramCache) diagnosticsTrack(name string, steps ...func() error) {
	if len(steps) == 0 {
		return
	}
	if c == nil {
		return
	}
	if prog := c.program; prog != nil {
		prog.DiagnosticsTrack(name, steps...)
		return
	}
	for _, step := range steps {
		if step != nil {
			step()
		}
	}
}
