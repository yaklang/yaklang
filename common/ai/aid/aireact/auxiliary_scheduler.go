package aireact

import (
	"context"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
)

// ScheduleAuxiliaryTask is the unified entry point for auxiliary AI tasks.
// It consults the single-model registry to decide whether to skip the call
// or run it. When single-model mode is disabled, all tasks run unchanged.
//
// The prompt is built lazily via spec.PromptBuilder — only after the skip
// check passes. All post-processing logic lives in spec.OnResult, so when
// the task is skipped, the entire function body (prompt construction + AI
// call + result handling) is suppressed.
func (r *ReAct) ScheduleAuxiliaryTask(ctx context.Context, spec aicommon.AuxiliaryTaskSpec) {
	// 1. Execution decision: skip means the entire task is suppressed.
	if r.config.IsSingleAIModelMode() {
		if action := aicommon.GetSingleModelAction(spec.Name); action == aicommon.SingleModelSkip {
			return
		}
	}

	// 2. Build prompt lazily (only when the task will actually run).
	if spec.PromptBuilder == nil {
		return
	}
	prompt := spec.PromptBuilder()
	if prompt == "" {
		return
	}

	// 3. Invoke LiteForge (no parameter degradation — caller decides later).
	cb := r.config.GetSpeedPriorityAICallback()
	result, err := r.invokeLiteForgeWithCallback(
		cb, ctx, spec.Name, prompt, spec.Outputs, spec.Opts...,
	)
	if err != nil {
		return // error handling centralized in scheduler
	}
	if result == nil {
		return
	}

	// 4. Result callback — all post-processing logic lives here.
	if spec.OnResult != nil {
		spec.OnResult(result)
	}
}
