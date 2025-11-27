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

func (c *Config) DiagnosticsTrack(name string, steps ...func() error) error {
	return c.DiagnosticsRecorder().TrackTrace(name, steps...)
}

func (c *saveValueCtx) DiagnosticsTrack(name string, steps ...func() error) error {
	return c.diagnosticsRecorder.TrackTrace(name, steps...)
}

func OptionSaveValue_Diagnostics(rec *diagnostics.Recorder) SaveValueOption {
	return func(c *saveValueCtx) {
		c.diagnosticsRecorder = rec
	}
}
