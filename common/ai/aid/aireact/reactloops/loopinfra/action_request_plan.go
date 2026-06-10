package loopinfra

import (
	"context"
	"errors"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

var loopAction_RequestPlan = &reactloops.LoopAction{
	AsyncMode:   false,
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

		// TODO(debug): restore plan loop execution after review/execute flow debugging.
		/*
			planTask := aicommon.NewStatefulTaskBase(
				task.GetId()+"_plan",
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
		*/
		planInput := mockRequestPlanInputForDebug(rewriteQuery)

		planCtx := task.GetContext()
		session, err := invoker.BeginPlanCoordinatorSession(planCtx, planInput, true)
		if err != nil {
			operator.Fail(err)
			return
		}
		defer session.Close()

		if err := session.ReviewPlan(planCtx); err != nil {
			if isPlanReviewUserCancelled(err, planCtx, task.GetContext()) {
				operator.Exit()
				return
			}
			operator.Fail(err)
			return
		}

		operator.RequestAsyncMode()
		task.SetAsyncMode(true)
		invoker.AsyncExecuteCod(planCtx, session.CoordinatorID(), func(err error) {
			loop.FinishAsyncTask(task, err)
		})
	},
	OutputExamples: loopAction_RequestPlanAndExecution.OutputExamples,
}

func isPlanReviewUserCancelled(err error, contexts ...context.Context) bool {
	if err == nil {
		return false
	}
	for _, ctx := range contexts {
		if ctx != nil && errors.Is(ctx.Err(), context.Canceled) {
			return true
		}
	}
	if errors.Is(err, context.Canceled) {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "user review params is nil") ||
		strings.Contains(msg, "user review failed")
}

func mockRequestPlanInputForDebug(planPayload string) *aicommon.ExecutePlanInput {
	planData := string(utils.Jsonify(map[string]any{
		"@action":        "plan",
		"main_task":      "调试计划",
		"main_task_goal": "验证 request_plan 的 review 与异步执行流程",
		"tasks": []map[string]any{
			{
				"subtask_name": "收集目标信息",
				"subtask_goal": "根据用户诉求整理关键上下文: " + planPayload,
			},
			{
				"subtask_name": "输出执行结论",
				"subtask_goal": "基于前两步结果给出可交付结论",
			},
		},
	}))
	return &aicommon.ExecutePlanInput{
		PlanPayload:  planPayload,
		PlanData:     planData,
		PlanFacts:    "debug: mocked plan facts",
		PlanDocument: "debug: mocked plan document",
	}
}
