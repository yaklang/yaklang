package aireact

import (
	"context"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
)

// ScheduleAuxiliaryTask is the unified entry point for auxiliary AI tasks.
// It consults the single-model registry to decide whether to skip the call
// or run it. When single-model mode is disabled, all tasks run unchanged.
//
// The prompt is built lazily via promptBuilder — only after the skip
// check passes. All post-processing logic lives in onResult, so when
// the task is skipped, the entire function body is suppressed.
func (r *ReAct) ScheduleAuxiliaryTask(
	ctx context.Context,
	name string,
	promptBuilder func() string,
	onResult func(*aicommon.Action),
	opts ...aicommon.AuxiliaryTaskOption,
) {
	// 1. Execution decision: skip means the entire task is suppressed.
	if r.config.IsSingleAIModelMode() {
		if action := aicommon.GetSingleModelAction(name); action == aicommon.SingleModelSkip {
			return
		}
	}

	// 2. Build prompt lazily (only when the task will actually run).
	if promptBuilder == nil {
		return
	}
	prompt := promptBuilder()
	if prompt == "" {
		return
	}

	// 3. Apply optional fields (outputs, LiteForge opts).
	spec := &aicommon.AuxiliaryTaskSpec{
		Outputs: nil, // default: no output schema
		Opts:    nil,
	}
	for _, opt := range opts {
		opt(spec)
	}

	// 4. Invoke LiteForge (no parameter degradation — caller decides later).
	cb := r.config.GetSpeedPriorityAICallback()
	result, err := r.invokeLiteForgeWithCallback(
		cb, ctx, name, prompt, spec.Outputs, spec.Opts...,
	)
	if err != nil {
		return // error handling centralized in scheduler
	}
	if result == nil {
		return
	}

	// 5. Result callback — all post-processing logic lives here.
	if onResult != nil {
		onResult(result)
	}
}
