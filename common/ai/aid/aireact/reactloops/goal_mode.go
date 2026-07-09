package reactloops

import "github.com/yaklang/yaklang/common/ai/aid/aicommon"

func (r *ReActLoop) IsGoalModeEnabled() bool {
	if r == nil || r.config == nil {
		return false
	}
	if cfg, ok := r.config.(interface{ GetEnableGoalMode() bool }); ok {
		return cfg.GetEnableGoalMode()
	}
	return false
}

func (r *ReActLoop) GetGoalMinIterations() int {
	if r == nil || r.config == nil {
		return int(aicommon.DefaultGoalMinIterations)
	}
	if cfg, ok := r.config.(interface{ GetGoalMinIterations() int64 }); ok {
		return int(cfg.GetGoalMinIterations())
	}
	return int(aicommon.DefaultGoalMinIterations)
}

// ShouldBlockFinishAtIteration reports whether the finish action should be
// blocked at the given iteration. finish is allowed for iteration >=
// GoalMinIterations; every earlier iteration is blocked.
func (r *ReActLoop) ShouldBlockFinishAtIteration(iteration int) bool {
	if !r.IsGoalModeEnabled() {
		return false
	}
	if iteration <= 0 {
		return true
	}
	return iteration < r.GetGoalMinIterations()
}

// ApplyGoalModeGate enforces the goal-mode finish gate for the given iteration
// on the operator that will be used to build this iteration's prompt. When the
// gate blocks finish, the operator's disallowLoopExit flag is set so that
// generateSchemaString removes the finish action from the schema.
//
// This is the single application point for the schema-level gate; it is called
// once per iteration right before prompt generation. DisallowNextLoopExit uses
// a Once, so repeated calls are idempotent.
func (r *ReActLoop) ApplyGoalModeGate(operator *LoopActionHandlerOperator, iteration int) {
	if r == nil || operator == nil {
		return
	}
	if r.ShouldBlockFinishAtIteration(iteration) {
		operator.DisallowNextLoopExit()
	}
}
