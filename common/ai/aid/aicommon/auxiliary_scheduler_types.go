package aicommon

import (
	"context"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

// AuxiliaryTaskSpec holds the essential fields for an auxiliary AI task:
// a name (registry key), a lazy prompt builder, and a result callback.
// Optional fields (output schema, LiteForge opts) are set via AuxiliaryTaskOption.
type AuxiliaryTaskSpec struct {
	Name          string
	PromptBuilder func() string
	OnResult      func(*Action)

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

// AuxiliaryScheduler is the interface for scheduling auxiliary AI tasks.
// It decides whether to skip or run based on the single-model registry.
type AuxiliaryScheduler interface {
	ScheduleAuxiliaryTask(ctx context.Context, name string, promptBuilder func() string, onResult func(*Action), opts ...AuxiliaryTaskOption)
}
