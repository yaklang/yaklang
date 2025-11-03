package loopinfra

import (
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

var loopAction_RequestPlanAndExecution = &reactloops.LoopAction{
	AsyncMode:   true,
	ActionType:  schema.AI_REACT_LOOP_ACTION_REQUEST_PLAN_EXECUTION,
	Description: `Request a detailed plan and execute it step-by-step to achieve the user's goal.`,
	Options: []aitool.ToolOption{
		aitool.WithStringParam(
			"plan_request_payload",
			aitool.WithParam_Description("USE THIS FIELD ONLY IF @action is 'request_plan_and_execution'. Provide a one-sentence summary of the complex task that needs a multi-step plan. This summary will trigger a more advanced planning system. Example: 'Create a marketing plan for a new product launch.'"),
		),
	},
	StreamFields: []*reactloops.LoopStreamField{
		{FieldName: `plan_request_payload`},
	},
	ActionVerifier: func(loop *reactloops.ReActLoop, action *aicommon.Action) error {
		// Check if there's already a plan execution task running
		invoker := loop.GetInvoker()
		if reactInvoker, ok := invoker.(interface {
			GetCurrentPlanExecutionTask() aicommon.AIStatefulTask
		}); ok {
			if reactInvoker.GetCurrentPlanExecutionTask() != nil {
				return utils.Errorf("another plan execution task is already running, please wait for it to complete or use directly_answer to provide the result")
			}
		}

		improveQuery := action.GetString("plan_request_payload")
		if improveQuery == "" {
			improveQuery = action.GetInvokeParams("next_action").GetString("plan_request_payload")
		}
		if improveQuery == "" {
			return utils.Errorf("request_plan_and_execution action must have 'plan_request_payload' field")
		}
		loop.Set("plan_request_payload", improveQuery)
		return nil
	},
	ActionHandler: func(loop *reactloops.ReActLoop, action *aicommon.Action, operator *reactloops.LoopActionHandlerOperator) {
		task := operator.GetTask()

		rewriteQuery := loop.Get("plan_request_payload")
		invoker := loop.GetInvoker()
		invoker.AsyncPlanAndExecute(task.GetContext(), rewriteQuery, func(err error) {
			loop.FinishAsyncTask(task, err)
		})
	},
}
