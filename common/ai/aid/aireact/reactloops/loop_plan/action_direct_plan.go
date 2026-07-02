package loop_plan

import (
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/log"
)

var generateDirectPlan = func(r aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopAction(
		"generate_direct_plan",
		"Generate a concise execution plan directly when the goal is clear and the workflow is obvious. Choose this after reviewing user input, timeline, and memory—not via a separate pre-assessment.",
		nil,
		nil,
		func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			task := loop.GetCurrentTask()
			if strings.TrimSpace(loop.Get(PLAN_FACTS_KEY)) == "" && task != nil {
				if incoming := bootstrapFactsFromUserInput(task.GetUserInput()); incoming != "" {
					appendPlanFacts(loop, incoming)
				}
			}

			reactloops.EmitStatus(loop, "正在直接生成任务计划... / Generating direct plan...")
			planData := generateDirectPlanFromUserInput(loop, task)
			if planData == "" {
				op.Fail("failed to generate direct plan from user input")
				return
			}

			loop.Set(PLAN_DATA_KEY, planData)
			r.AddToTimeline("direct_plan_generated", "Direct plan generated from clear user request without deep exploration")
			log.Infof("plan loop: generate_direct_plan completed")
			op.Exit()
		},
	)
}

var beginDeepPlanning = func(r aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopAction(
		"begin_deep_planning",
		"Switch to deep exploration planning when more information gathering is required before generating a plan.",
		nil,
		nil,
		func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			enterDeepPlanMode(loop, "AI escalated to deep planning via begin_deep_planning")
			restoreDeepPlanningActions(loop, r)
			log.Infof("plan loop: begin_deep_planning restored exploration actions")
			op.Continue()
		},
	)
}

func ensureDirectPlanOnFinalize(loop *reactloops.ReActLoop, task aicommon.AIStatefulTask) {
	if !isSimplePlanMode(loop) || hasValidPlan(loop) {
		return
	}
	planData := generateDirectPlanFromUserInput(loop, task)
	if planData != "" {
		loop.Set(PLAN_DATA_KEY, planData)
		log.Infof("plan loop: generated direct plan at finalization fallback")
	}
}
