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

var loopAction_RequestVerification = &reactloops.LoopAction{
	ActionType:  schema.AI_REACT_LOOP_ACTION_REQUEST_VERIFICATION,
	Description: "Actively trigger a verification pass to check whether the current work already satisfies the task goal.",
	Options: []aitool.ToolOption{
		aitool.WithStringParam(
			"verification_payload",
			aitool.WithParam_Description("Optional concise summary of the current progress, deliverables, or observations that should be used as the direct verification input. If omitted, the runtime will build a default checkpoint summary from recent actions."),
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
			operator.Feedback("request_verification requires an active task context")
			operator.Continue()
			return
		}

		payload := strings.TrimSpace(loop.Get("verification_payload"))
		if payload == "" {
			payload = buildDefaultVerificationPayload(loop)
		}

		invoker := loop.GetInvoker()
		invoker.AddToTimeline("[REQUEST_VERIFICATION]", payload)

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
		if verifyResult.Satisfied {
			operator.Exit()
			return
		}

		feedbackMsg := fmt.Sprintf("[Verification] Task not yet satisfied.\nReasoning: %s", verifyResult.Reasoning)
		if summary := aicommon.FormatVerifyNextMovementsSummary(verifyResult.NextMovements); summary != "" {
			feedbackMsg += fmt.Sprintf("\nNext Steps: %s", summary)
		}
		operator.Feedback(feedbackMsg)
		operator.Continue()
	},
}
