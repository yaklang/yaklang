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
			aitool.WithParam_Description("USE THIS FIELD ONLY IF type is 'request_plan_and_execution'. Provide a one-sentence summary of the complex task that needs a multi-step plan. This summary will trigger a more advanced planning system. Example: 'Create a marketing plan for a new product launch.'"),
		),
	},
	ActionVerifier: func(loop *reactloops.ReActLoop, action *aicommon.Action) error {
		improveQuery := action.GetString("plan_request_payload")
		if improveQuery == "" {
			return utils.Errorf("request_plan_and_execution action must have 'plan_request_payload' field")
		}
		return nil
	},
	ActionHandler: func(loop *reactloops.ReActLoop, action *aicommon.Action, operator *reactloops.LoopActionHandlerOperator) {
		task := operator.GetTask()

		rewriteQuery := action.GetString("plan_request_payload")
		invoker := loop.GetInvoker()

		err := invoker.AsyncPlanAndExecute(task.GetContext(), rewriteQuery, func() {
			task.SetStatus(aicommon.AITaskState_Completed)
		})
		if err != nil {
			operator.Fail(utils.Wrap(err, "AsyncPlanAndExecute"))
			return
		}
		operator.Continue()
	},
}
