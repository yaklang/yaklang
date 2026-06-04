package loopinfra

import (
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/schema"
)

var loopAction_RequestPlan = &reactloops.LoopAction{
	AsyncMode:   true,
	ActionType:  schema.AI_REACT_LOOP_ACTION_REQUEST_PLAN,
	Description: `Request a detailed plan for a complex task. After user review approves the plan, execution starts asynchronously.`,
	Options:     loopAction_RequestPlanAndExecution.Options,
	StreamFields: []*reactloops.LoopStreamField{
		{FieldName: `plan_request_payload`, AINodeId: "plan"},
	},
	ActionVerifier: func(loop *reactloops.ReActLoop, action *aicommon.Action) error {
		return verifyPlanRequestPayload(loop, action, schema.AI_REACT_LOOP_ACTION_REQUEST_PLAN)
	},
	ActionHandler: func(loop *reactloops.ReActLoop, action *aicommon.Action, operator *reactloops.LoopActionHandlerOperator) {
		task := operator.GetTask()
		rewriteQuery := loop.Get("plan_request_payload")
		invoker := loop.GetInvoker()
		invoker.AsyncPlanOnly(task.GetContext(), rewriteQuery, func(err error) {
			loop.FinishAsyncTask(task, err)
		})
	},
	OutputExamples: loopAction_RequestPlanAndExecution.OutputExamples,
}
