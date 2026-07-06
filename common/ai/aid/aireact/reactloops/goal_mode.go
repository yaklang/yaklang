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

func (r *ReActLoop) ShouldBlockFinishAtIteration(iteration int) bool {
	if !r.IsGoalModeEnabled() {
		return false
	}
	if iteration <= 0 {
		return true
	}
	return iteration < r.GetGoalMinIterations()
}

func (r *ReActLoop) ApplyGoalModeNextIterationGate(operator *LoopActionHandlerOperator, nextIteration int) {
	if r == nil || operator == nil {
		return
	}
	if r.ShouldBlockFinishAtIteration(nextIteration) {
		operator.DisallowNextLoopExit()
	}
}
