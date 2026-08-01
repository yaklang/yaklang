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
func applyTodoDeltaBottomLine(r *ReActLoop, task aicommon.AIStatefulTask, iteration int, action *aicommon.Action) {
	if r == nil || action == nil || r.config == nil {
		return
	}
	delta, err := aicommon.NormalizeTodoDelta(action)
	if err != nil || delta == nil {
		return // validation already ran inside the AI transaction
	}
	var timelineHook func(string, string)
	if invoker := r.GetInvoker(); invoker != nil {
		timelineHook = invoker.AddToTimeline
	}
	aicommon.ApplyTodoDeltaAndEmit(r.config, r.GetEmitter(), task, aicommon.BuildVerificationTodoScope(task), iteration, delta, timelineHook)
}
