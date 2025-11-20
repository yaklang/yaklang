package ssaapi

import (
	"github.com/yaklang/yaklang/common/utils/diagnostics"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

var WithDiagnostics = ssaconfig.SetOption("ssa_compile/diagnostics", func(c *Config, enabled bool) {
	c.diagnosticsEnabled = enabled
})

func (c *Config) DiagnosticsEnabled() bool {
	return c != nil && c.diagnosticsEnabled
}

func (c *Config) DiagnosticsRecorder() *diagnostics.Recorder {
	if !c.DiagnosticsEnabled() {
		return nil
	}
	if c.diagnosticsRecorder == nil {
		c.diagnosticsRecorder = diagnostics.NewRecorder()
	}
	return c.diagnosticsRecorder
}

func (c *Config) SetDiagnosticsRecorder(rec *diagnostics.Recorder) {
	c.diagnosticsRecorder = rec
}

func (c *Config) DiagnosticsTrack(name string, steps ...func()) {
	if len(steps) == 0 {
		return
	}
	if !c.DiagnosticsEnabled() {
		for _, step := range steps {
			if step != nil {
				step()
			}
		}
		return
	}
	if rec := c.DiagnosticsRecorder(); rec != nil {
		rec.Track(true, name, steps...)
		return
	}
	for _, step := range steps {
		if step != nil {
			step()
		}
	}
}

func (c *Config) DiagnosticsTrackWithError(name string, steps ...diagnostics.StepFunc) (diagnostics.Measurement, error) {
	empty := diagnostics.Measurement{Name: name}
	if !c.DiagnosticsEnabled() {
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
	if rec := c.DiagnosticsRecorder(); rec != nil {
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

func (c *Config) LogDiagnostics(label string) {
	if !c.DiagnosticsEnabled() {
		return
	}
	recorder := c.DiagnosticsRecorder()
	if recorder == nil {
		return
	}
	diagnostics.LogRecorder(label, recorder)
}

func (c *saveValueCtx) DiagnosticsRecorder() *diagnostics.Recorder {
	return c.diagnosticsRecorder
}

func (c *saveValueCtx) DiagnosticsTrack(name string, steps ...func()) {
	if len(steps) == 0 {
		return
	}
	if rec := c.DiagnosticsRecorder(); rec != nil {
		rec.Track(true, name, steps...)
		return
	}
	for _, step := range steps {
		if step != nil {
			step()
		}
	}
}

func (c *saveValueCtx) DiagnosticsTrackWithError(name string, steps ...diagnostics.StepFunc) (diagnostics.Measurement, error) {
	empty := diagnostics.Measurement{Name: name}
	if rec := c.DiagnosticsRecorder(); rec != nil {
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

func OptionSaveValue_Diagnostics(rec *diagnostics.Recorder) SaveValueOption {
	return func(c *saveValueCtx) {
		c.diagnosticsRecorder = rec
	}
}
