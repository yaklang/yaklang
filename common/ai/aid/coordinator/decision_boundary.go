package coordinator

import (
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
)

func configureDecisionBoundary(loop *reactloops.ReActLoop) {
	for _, name := range []string{"save_evidence", "directly_call_tool", "require_tool", "adjust_todolist", "ask_for_clarification", "knowledge_enhance_answer", "load_skills", "change_skill_view_offset", "load_skill_resources", "search_capabilities"} {
		action, err := loop.GetActionHandler(name)
		if err != nil {
			continue
		}
		wrapped := *action
		wrapped.FunctionCallAction = nil
		wrapped.ActionHandler = func(l *reactloops.ReActLoop, a *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			l.Set("coordinator_last_action", action.ActionType)
			action.ActionHandler(l, a, op)
			if done, err := op.IsTerminated(); done && err != nil {
				l.Set("coordinator_last_action", "rejected")
			}
		}
		reactloops.WithOverrideLoopAction(&wrapped)(loop)
	}
}
