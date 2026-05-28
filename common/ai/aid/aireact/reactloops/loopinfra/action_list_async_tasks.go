package loopinfra

import (
	"encoding/json"

	"github.com/yaklang/yaklang/common/ai/aid"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

var loopAction_ListAsyncTasks = &reactloops.LoopAction{
	ActionType: schema.AI_REACT_LOOP_ACTION_LIST_ASYNC_TASKS,
	Description: "List all asynchronous tasks, currently executing tasks/subtasks, and each task's 10 most recent timeline text outputs. " +
		"Use when plan execution, forge, or other async work is in flight and you need visibility before choosing the next action.",
	ActionHandler: func(loop *reactloops.ReActLoop, action *aicommon.Action, operator *reactloops.LoopActionHandlerOperator) {
		invoker := loop.GetInvoker()
		provider, ok := invoker.(aid.TaskRuntimeReportProvider)
		if !ok || provider == nil {
			operator.Feedback("list_async_tasks is only available in the main ReAct runtime")
			operator.Continue()
			return
		}

		report := provider.CollectTaskRuntimeReport()
		raw, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			operator.Feedback(utils.Errorf("failed to marshal task runtime report: %v", err).Error())
			operator.Continue()
			return
		}

		invoker.AddToTimeline("[LIST_ASYNC_TASKS]", string(raw))
		operator.Feedback(string(raw))
		operator.Continue()
	},
}
