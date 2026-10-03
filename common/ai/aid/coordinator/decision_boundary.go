package coordinator

import (
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"time"
)

// One model response may contain several actions. Keep their actual outcomes
// together rather than letting the final wait/TODO action hide a prior decision.
type decisionBoundary struct {
	considered   bool
	handledUser  bool
	continuation bool
	toolResult   bool
	rejected     bool
	yield        bool
	timeout      time.Duration
}

func currentDecision(loop *reactloops.ReActLoop) *decisionBoundary {
	d, _ := loop.GetVariable("coordinator_decision_boundary").(*decisionBoundary)
	if d == nil {
		d = &decisionBoundary{}
		loop.Set("coordinator_decision_boundary", d)
	}
	return d
}

func recordDecision(loop *reactloops.ReActLoop, name string, err error) {
	d := currentDecision(loop)
	if err != nil {
		d.rejected = true
		return
	}
	d.considered = true
	switch name {
	case "modify_plan", "retry_task", "cancel_tasks", "directly_answer", "ask_for_clarification":
		d.handledUser = true
	}
	switch name {
	case "directly_call_tool", "require_tool", "inspect_task", "save_evidence", "knowledge_enhance_answer", "load_skills", "change_skill_view_offset", "load_skill_resources", "search_capabilities", "modify_plan", "create_report", "modify_report", "ask_for_clarification":
		// Tool results and intermediate edits require a next local decision even
		// while workers are running. A TODO title is never parsed as a dependency.
		d.continuation = true
	}
	switch name {
	case "directly_call_tool", "require_tool", "inspect_task", "knowledge_enhance_answer", "load_skills", "change_skill_view_offset", "load_skill_resources", "search_capabilities", "ask_for_clarification":
		d.toolResult = true
	}
}

func configureDecisionBoundary(loop *reactloops.ReActLoop) {
	for _, name := range []string{"save_evidence", "directly_call_tool", "require_tool", "adjust_todolist", "ask_for_clarification", "knowledge_enhance_answer", "load_skills", "change_skill_view_offset", "load_skill_resources", "search_capabilities"} {
		action, err := loop.GetActionHandler(name)
		if err != nil {
			continue
		}
		wrapped := *action
		wrapped.FunctionCallAction = nil
		wrapped.ActionHandler = func(l *reactloops.ReActLoop, a *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			action.ActionHandler(l, a, op)
			_, err := op.IsTerminated()
			recordDecision(l, action.ActionType, err)
		}
		reactloops.WithOverrideLoopAction(&wrapped)(loop)
	}
}
