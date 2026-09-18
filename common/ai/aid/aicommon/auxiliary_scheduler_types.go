package aicommon

import (
	"context"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

// AuxiliaryTaskSpec holds the optional execution fields for an auxiliary AI
// task. The required fields are positional arguments of ScheduleAuxiliaryTask.
type AuxiliaryTaskSpec struct {
	Outputs          []aitool.ToolOption
	Opts             []GeneralKVConfigOption
	OutputActionName string
	OutputSchema     string
	Emitter          *Emitter
	OnError          func(error)
}

// AuxiliaryTaskOption modifies an AuxiliaryTaskSpec.
type AuxiliaryTaskOption func(*AuxiliaryTaskSpec)

// WithAuxiliaryOutputSchema preserves an existing JSON output protocol while
// the task name remains the registry key and request label.
func WithAuxiliaryOutputSchema(actionName, schema string) AuxiliaryTaskOption {
	return func(s *AuxiliaryTaskSpec) {
		s.OutputActionName, s.OutputSchema = actionName, schema
	}
}

// WithAuxiliaryEmitter binds field events to the owner of this invocation.
// It does not swap the shared Config emitter, so concurrent tools stay isolated.
func WithAuxiliaryEmitter(emitter *Emitter) AuxiliaryTaskOption {
	return func(s *AuxiliaryTaskSpec) { s.Emitter = emitter }
}

// WithAuxiliaryOnError receives the final invocation/parse error, after retries.
// Skipped tasks do not invoke this callback or OnResult.
func WithAuxiliaryOnError(handler func(error)) AuxiliaryTaskOption {
	return func(s *AuxiliaryTaskSpec) { s.OnError = handler }
}

type AuxiliaryTaskDecision struct {
	Action      SingleModelAction
	RequestOpts []AIRequestOption
}

func (d AuxiliaryTaskDecision) ShouldRun() bool {
	return d.Action != SingleModelSkip
}

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
	ResolveAuxiliaryTask(name string) AuxiliaryTaskDecision
}
