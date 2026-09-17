package aicommon

import (
	"context"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

// AuxiliaryTaskSpec is the declarative specification for an auxiliary AI task.
// Callers declare: what task this is, how to build the prompt (lazily), the
// output schema, and what to do with the result. The scheduler handles the
// decision (skip / call) and error handling.
//
// The prompt is provided as a builder function so it is only constructed
// after the scheduler decides the task will actually run. When the task is
// skipped, neither PromptBuilder nor OnResult is called.
type AuxiliaryTaskSpec struct {
	// Name is the task identifier, matching a CallerLabel constant in the
	// single-model registry.
	Name string
	// PromptBuilder constructs the business prompt. Called only when the
	// task will actually run (not skipped). Return "" to abort the call.
	PromptBuilder func() string
	// Outputs is the output schema (aitool.ToolOption list).
	Outputs []aitool.ToolOption
	// Opts are GeneralKVConfigOption values forwarded to LiteForge
	// (stream callbacks, static instructions, etc.).
	Opts []GeneralKVConfigOption

	// OnResult is called after the AI result is successfully parsed into an
	// Action. It is NOT called when the task is skipped. All post-processing
	// logic (logging, variable assignment, downstream side-effects) should
	// live here so that skip naturally suppresses the entire task body.
	OnResult func(*Action)
}

// AuxiliaryScheduler is the interface for scheduling auxiliary AI tasks.
// It decides whether to skip or run based on the single-model registry.
type AuxiliaryScheduler interface {
	ScheduleAuxiliaryTask(ctx context.Context, spec AuxiliaryTaskSpec)
}
