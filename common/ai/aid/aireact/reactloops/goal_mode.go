package reactloops

import (
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
)

func (r *ReActLoop) IsGoalModeEnabled() bool {
	if r == nil || r.config == nil {
		return false
	}
	// Goal mode is a top-level-only strategy: it forces a minimum-iteration
	// finish gate, which must never apply to a forked sub agent (a sub agent
	// should be free to finish as soon as its single goal is done). Even if a
	// sub-agent config somehow had EnableGoalMode set, this guard forces it off.
	if r.IsSubAgent() {
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

// GetMaxSubAgents returns the max simultaneous sub-agent concurrency for multi-agent mode.
// Like GoalMinIterations it reads from config with a normalized default; the
// absolute ceiling remains AbsoluteMaxSubAgentConcurrency.
func (r *ReActLoop) GetMaxSubAgents() int {
	if r == nil || r.config == nil {
		return int(aicommon.DefaultMaxSubAgentConcurrency)
	}
	if cfg, ok := r.config.(interface{ GetMaxSubAgents() int64 }); ok {
		return int(cfg.GetMaxSubAgents())
	}
	return int(aicommon.DefaultMaxSubAgentConcurrency)
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
	return iteration-r.subAgentControlIterations < r.GetGoalMinIterations()
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

// --- Gate 2: Time Window ---

// ShouldBlockFinishByDeadline reports whether the finish should be blocked
// because the goal time window has not elapsed yet. Returns false if the
// time window gate is disabled (GoalDurationSeconds == 0) or not yet started.
//
// The deadline is started lazily on the first call via StartGoalDeadline,
// so the window measures actual execution time from the first finish attempt
// rather than from session creation.
func (r *ReActLoop) ShouldBlockFinishByDeadline() bool {
	if !r.IsGoalModeEnabled() {
		return false
	}
	cfg, ok := r.config.(*aicommon.Config)
	if !ok {
		return false
	}
	if cfg.GetGoalDurationSeconds() == 0 {
		return false // time window gate disabled
	}
	// Lazily start the deadline on first check
	cfg.StartGoalDeadline()
	return !cfg.IsGoalDeadlinePassed()
}

// --- Gate 3: Acceptance Criteria ---

// goalAcceptanceCriteria returns the configured criteria when goal acceptance
// applies to this loop. Sub-agents and non-goal-mode loops never enter the
// review state.
func (r *ReActLoop) goalAcceptanceCriteria() string {
	if r == nil || !r.IsGoalModeEnabled() {
		return ""
	}
	cfg, ok := r.config.(interface{ GetGoalAcceptanceCriteria() string })
	if !ok {
		return ""
	}
	return strings.TrimSpace(cfg.GetGoalAcceptanceCriteria())
}

// armGoalAcceptanceReview moves a finish request that passed all host gates
// into a one-turn main-model acceptance review. The next prompt keeps the normal
// dynamic section; the acceptance instruction is carried by Timeline.
func (r *ReActLoop) armGoalAcceptanceReview() bool {
	if r == nil || r.isGoalAcceptanceMode() || r.goalAcceptanceCriteria() == "" {
		return false
	}
	r.promptMode = aicommon.LoopPromptModeGoalAcceptance
	return true
}

func (r *ReActLoop) discardGoalAcceptanceReviewForWork() {
	if r != nil && r.isGoalAcceptanceMode() {
		r.promptMode = aicommon.LoopPromptModeNormal
	}
}

// prepareGoalAcceptanceActionBatch reports whether a review turn chose work
// instead of finish. The caller resets promptMode when the action batch ends.
func (r *ReActLoop) prepareGoalAcceptanceActionBatch(calls []LoopCall) bool {
	if r == nil || !r.isGoalAcceptanceMode() {
		return false
	}
	for _, call := range calls {
		if call.Action != nil && call.Action.Name() == loopAction_Finish.ActionType {
			return false
		}
	}
	if r.invoker != nil {
		r.invoker.AddToTimeline("[GOAL_ACCEPTANCE_CONTINUE]",
			"the main model selected a work action during goal acceptance review; returning to normal mode")
	}
	return true
}

func (r *ReActLoop) isGoalAcceptanceMode() bool {
	return r != nil && r.promptMode == aicommon.LoopPromptModeGoalAcceptance
}
