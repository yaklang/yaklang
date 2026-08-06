package reactloops

import (
	"fmt"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/log"
)

// suppressInvalidTodoDelta keeps TODO maintenance subordinate to the selected
// ReAct action. The invalid delta is rejected atomically and recorded for a
// later correction, while the already-valid tool/answer action is preserved.
// Finish remains safe because its handler independently refuses to exit while
// the current scope owns open TODOs.
func suppressInvalidTodoDelta(r *ReActLoop, action *aicommon.Action, err error) {
	if action == nil || err == nil {
		return
	}
	message := fmt.Sprintf(
		"Ignored invalid todo_delta; TODO state was not changed, but the main action will continue. Correct the delta in a later normal action: %v",
		err,
	)
	if r != nil {
		if invoker := r.GetInvoker(); invoker != nil {
			invoker.AddToTimeline("TODO_DELTA_ERROR", message)
		}
	}
	log.Warnf("%s", message)
	action.DeleteParam("todo_delta")
}

// applyTodoDeltaBottomLine applies the optional increment before the selected
// action handler, so tool calls, directly_answer, and finish share one path.
// It returns the parsed *TodoDelta (nil when absent/invalid) so the caller can
// reuse it to decide whether this iteration effectively advanced the TODO state.
func applyTodoDeltaBottomLine(r *ReActLoop, task aicommon.AIStatefulTask, iteration int, action *aicommon.Action) *aicommon.TodoDelta {
	if r == nil || action == nil || r.config == nil {
		return nil
	}
	delta, err := aicommon.NormalizeTodoDelta(action)
	if err != nil || delta == nil {
		return nil // validation already ran inside the AI transaction
	}
	var timelineHook func(string, string)
	if invoker := r.GetInvoker(); invoker != nil {
		timelineHook = invoker.AddToTimeline
	}
	aicommon.ApplyTodoDeltaAndEmit(r.config, r.GetEmitter(), task, aicommon.BuildVerificationTodoScope(task), iteration, delta, timelineHook)
	return delta
}

// advanceEffectiveIteration updates effectiveIterationCount based on whether
// this iteration actually advanced the TODO state.
//
// A loop iteration counts as "effective" when:
//   - the action carried a todo_delta with real changes (add/update/close/
//     current), OR
//   - there are no active TODOs in the current scope (planning/recon phase,
//     nothing to advance yet).
//
// When the scope has active TODOs but the action produced no todo_delta (a
// "stall" iteration — the AI called a tool / asked for clarification but did
// not move any TODO), effectiveIterationCount is NOT incremented, so stall
// iterations do not consume the iteration budget.
//
// The raw loop cycle counter (currentIterationIndex) is unaffected and
// continues to be used for prompt building, goal-mode gating, timeline, etc.
//
// 关键词: 有效迭代, todo 推进判定, 空转轮不计入
func (r *ReActLoop) advanceEffectiveIteration(task aicommon.AIStatefulTask, delta *aicommon.TodoDelta) {
	if r == nil || task == nil {
		return
	}
	scope := aicommon.BuildVerificationTodoScope(task)
	hasActiveTodo := false
	if r.config != nil {
		active := r.config.ActiveVerificationTodoItemsByScope(scope)
		hasActiveTodo = len(active) > 0
	}
	progressed := delta != nil && delta.HasChanges()

	if !hasActiveTodo || progressed {
		// Effective: either we have no TODOs to advance (planning phase) or we
		// actually advanced one this iteration.
		r.effectiveIterationCount++
	}
}
