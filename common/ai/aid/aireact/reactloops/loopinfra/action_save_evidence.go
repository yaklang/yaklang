package loopinfra

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/schema"
)

func buildDefaultVerificationPayload(loop *reactloops.ReActLoop) string {
	if loop == nil {
		return "Agent explicitly requested verification of the current work."
	}

	var parts []string
	parts = append(parts, "Agent explicitly requested verification of the current work.")
	parts = append(parts, fmt.Sprintf("Current iteration: %d.", loop.GetCurrentIterationIndex()))

	if last := loop.GetLastAction(); last != nil {
		parts = append(parts, fmt.Sprintf("Last action: %s (iteration %d).", last.ActionType, last.IterationIndex))
	}

	if recent := loop.GetLastNAction(3); len(recent) > 0 {
		names := make([]string, 0, len(recent))
		for _, item := range recent {
			if item == nil || strings.TrimSpace(item.ActionType) == "" {
				continue
			}
			names = append(names, item.ActionType)
		}
		if len(names) > 0 {
			parts = append(parts, fmt.Sprintf("Recent actions: %s.", strings.Join(names, " -> ")))
		}
	}

	parts = append(parts, "Use the full timeline, TODO snapshot, and shared context as the primary evidence for acceptance.")
	return strings.Join(parts, "\n")
}

var loopAction_SaveEvidence = &reactloops.LoopAction{
	ActionType:  schema.AI_REACT_LOOP_ACTION_SAVE_EVIDENCE,
	Description: "Save evidence observations from the current step. Use this action when you have discovered key findings, confirmed facts, or identified significant state changes that should be persisted as evidence for future reference. This action triggers a verification pass that deposits your observations into the evidence store.",
	Options: []aitool.ToolOption{
		aitool.WithStringParam(
			"verification_payload",
			aitool.WithParam_Description("A concise summary of the key findings, confirmed facts, or observations you want to save as evidence. Describe what was discovered, how it was confirmed, and why it matters for the task."),
		),
	},
	StreamFields: []*reactloops.LoopStreamField{
		{FieldName: "verification_payload", AINodeId: "verification_payload"},
	},
	ActionVerifier: func(loop *reactloops.ReActLoop, action *aicommon.Action) error {
		payload := strings.TrimSpace(action.GetString("verification_payload"))
		if payload == "" {
			payload = strings.TrimSpace(action.GetInvokeParams("next_action").GetString("verification_payload"))
		}
		loop.Set("verification_payload", payload)
		return nil
	},
	ActionHandler: func(loop *reactloops.ReActLoop, action *aicommon.Action, operator *reactloops.LoopActionHandlerOperator) {
		task := loop.GetCurrentTask()
		if task == nil {
			operator.Feedback("save_evidence requires an active task context")
			operator.Continue()
			return
		}

		payload := strings.TrimSpace(loop.Get("verification_payload"))
		if payload == "" {
			payload = buildDefaultVerificationPayload(loop)
		}

		invoker := loop.GetInvoker()
		invoker.AddToTimeline("[SAVE_EVIDENCE]", payload)

		ctx := invoker.GetConfig().GetContext()
		if task.GetContext() != nil {
			ctx = task.GetContext()
		}

		verifyResult, err := loop.VerifyUserSatisfactionNow(ctx, task.GetUserInput(), false, payload)
		if err != nil {
			operator.Fail(err)
			return
		}
		if verifyResult == nil {
			operator.Continue()
			return
		}
		// verification 现在是纯观测调用, 不再决定退出. satisfied 仅作为观测
		// 信号沉淀, 退出唯一由 AI 主动 finish action 决定.
		// 关键词: verification 不退, 退出只走 finished, 纯观测角色
		if !verifyResult.Satisfied {
			feedbackMsg := fmt.Sprintf("[Verification] Task not yet satisfied.\nReasoning: %s", verifyResult.Reasoning)
			operator.Feedback(feedbackMsg)
		}
		operator.Continue()
	},
}
