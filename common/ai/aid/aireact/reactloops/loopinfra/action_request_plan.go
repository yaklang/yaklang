package loopinfra

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_plan"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
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

		if strings.TrimSpace(rewriteQuery) == "" {
			rewriteQuery = task.GetUserInput()
		}

		planTask := aicommon.NewStatefulTaskBase(
			task.GetId(),
			rewriteQuery,
			task.GetContext(),
			task.GetEmitter(),
		)

		appendPlanPrompt := func(tagName, prompt string) string {
			if strings.TrimSpace(prompt) == "" {
				return ""
			}
			nonce := utils.RandStringBytes(8)
			return fmt.Sprintf(
				"\n<|%s_%s|>\n"+
					"%s\n"+
					"<|%s_END_%s|>\n",
				tagName, nonce, prompt, tagName, nonce)
		}

		var planPrompt string
		if globalConfig := yakit.GetCachedAIGlobalConfig(); globalConfig != nil && globalConfig.GetAIPlanPrompt() != "" {
			planPrompt += appendPlanPrompt("AI_PLAN", globalConfig.GetAIPlanPrompt())
		}
		cfg := invoker.GetConfig()
		if cfg != nil {
			if userPlanPrompt := cfg.GetConfigString("plan_prompt"); userPlanPrompt != "" {
				planPrompt += appendPlanPrompt("USER_PLAN", userPlanPrompt)
			}
			if planPrompt != "" {
				cfg.SetConfig(loop_plan.PLAN_PROMPT_KEY, planPrompt)
			}
		}

		var planLoop *reactloops.ReActLoop
		opts := []any{
			reactloops.WithOnLoopInstanceCreated(func(l *reactloops.ReActLoop) {
				planLoop = l
			}),
		}

		_, err := invoker.ExecuteLoopTaskIF(schema.AI_REACT_LOOP_NAME_PLAN, planTask, opts...)
		if err != nil {
			operator.Fail(err)
			return
		}

		if planLoop == nil {
			operator.Fail(utils.Error("plan loop instance not created"))
			return
		}

		planData := planLoop.Get(loop_plan.PLAN_DATA_KEY)
		if planData == "" {
			operator.Fail(utils.Error("plan loop finished without producing plan data"))
			return
		}

		planInput := &aicommon.ExecutePlanInput{
			PlanPayload:  rewriteQuery,
			PlanData:     planData,
			PlanFacts:    planLoop.Get(loop_plan.PLAN_FACTS_KEY),
			PlanDocument: planLoop.Get(loop_plan.PLAN_DOCUMENT_KEY),
		}

		reviewedInput, err := invoker.ForceReviewExecutePlan(task.GetContext(), planInput)
		if err != nil {
			operator.Fail(err)
			return
		}

		invoker.AsyncExecutePlan(task.GetContext(), reviewedInput, func(err error) {
			loop.FinishAsyncTask(task, err)
		})
	},
	OutputExamples: loopAction_RequestPlanAndExecution.OutputExamples,
}
