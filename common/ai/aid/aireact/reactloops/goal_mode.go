package reactloops

import (
	"context"
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
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

// GoalAcceptanceReviewResult holds the result of an LLM review against the
// acceptance criteria.
type GoalAcceptanceReviewResult struct {
	Passed bool
	Reason string // why it failed (empty when passed)
}

// CheckGoalAcceptanceCriteria performs an LLM review of the current timeline
// against the configured acceptance criteria. Returns passed=true if the
// criteria are satisfied, or passed=false with a reason describing what is
// missing. Returns passed=true (no-op) if the acceptance criteria gate is
// disabled or goal mode is not enabled.
func (r *ReActLoop) CheckGoalAcceptanceCriteria(ctx context.Context) *GoalAcceptanceReviewResult {
	result := &GoalAcceptanceReviewResult{Passed: true}
	if r == nil || !r.IsGoalModeEnabled() {
		return result
	}
	cfg, ok := r.config.(*aicommon.Config)
	if !ok {
		return result
	}
	criteria := strings.TrimSpace(cfg.GetGoalAcceptanceCriteria())
	if criteria == "" {
		return result // acceptance criteria gate disabled
	}

	if r.GetInvoker() == nil {
		log.Warnf("goal acceptance check: invoker is nil, skipping")
		return result
	}

	cfg.ScheduleAuxiliaryTask(ctx, aicommon.CallerLabelGoalAcceptanceReview,
		func() string {
			// Gather and bound the evidence only when the task is scheduled.
			timelineDiff := r.GetTimelineDiffWithoutUpdate()
			if strings.TrimSpace(timelineDiff) == "" {
				timelineDiff = "(no timeline content available)"
			}
			const maxTimelineChars = 8000
			if len(timelineDiff) > maxTimelineChars {
				timelineDiff = timelineDiff[:maxTimelineChars] + "\n... (truncated)"
			}
			return fmt.Sprintf(`You are a strict acceptance reviewer. Determine whether the work done so far satisfies the acceptance criteria.

## Acceptance Criteria
%s

## Work Done (Timeline Summary)
%s

## Task
Review the timeline evidence against the acceptance criteria. If ALL criteria are satisfied, set "passed" to true. If any criterion is not met, set "passed" to false and explain exactly what is missing or incomplete in "reason".

Be rigorous: only pass when there is concrete evidence in the timeline that each criterion is satisfied. Do not infer completion from absence of information.`, criteria, timelineDiff)
		},
		func(action *aicommon.Action) {
			params := action.GetParams()
			result.Passed = params.GetBool("passed")
			result.Reason = strings.TrimSpace(params.GetString("reason"))
		},
		aicommon.WithAuxiliaryOnError(func(err error) {
			log.Warnf("goal acceptance review failed: %v, allowing finish", err)
		}),
		aicommon.WithAuxiliaryOutputs(
			aitool.WithBoolParam("passed",
				aitool.WithParam_Description("true if all acceptance criteria are satisfied"),
			),
			aitool.WithStringParam("reason",
				aitool.WithParam_Description("when passed=false, explain what is missing or incomplete; leave empty when passed=true"),
			),
		),
	)
	// Errors and skipped calls leave the existing fail-open default intact.
	return result
}
