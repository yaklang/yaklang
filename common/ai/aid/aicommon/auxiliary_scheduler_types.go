package aicommon

import (
	"context"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

// AuxiliaryTaskSpec holds the optional execution fields for an auxiliary AI
// task. The required fields are positional arguments of ScheduleAuxiliaryTask.
type AuxiliaryTaskSpec struct {
	Outputs []aitool.ToolOption
	Opts    []GeneralKVConfigOption
}

// AuxiliaryTaskOption modifies an AuxiliaryTaskSpec.
type AuxiliaryTaskOption func(*AuxiliaryTaskSpec)

// WithAuxiliaryOutputs sets the output schema for the auxiliary task.
func WithAuxiliaryOutputs(outputs ...aitool.ToolOption) AuxiliaryTaskOption {
	return func(s *AuxiliaryTaskSpec) {
		s.Outputs = outputs
	}
}

// WithAuxiliaryOpts sets GeneralKVConfigOption values forwarded to LiteForge
// (stream callbacks, static instructions, etc.).
func WithAuxiliaryOpts(opts ...GeneralKVConfigOption) AuxiliaryTaskOption {
	return func(s *AuxiliaryTaskSpec) {
		s.Opts = opts
	}
}

// AuxiliaryScheduler is implemented by Config. Keeping scheduling on the
// configuration makes the single-model policy available to ReAct and non-ReAct
// callers alike, without coupling the policy to a particular runtime.
type AuxiliaryScheduler interface {
	ScheduleAuxiliaryTask(ctx context.Context, name string, promptBuilder func() string, onResult func(*Action), opts ...AuxiliaryTaskOption)
}
