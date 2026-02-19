package loopinfra

import (
	"fmt"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

var loopAction_toolRequireAndCall = &reactloops.LoopAction{
	ActionType:  schema.AI_REACT_LOOP_ACTION_REQUIRE_TOOL,
	Description: "Require tool call",
	Options: []aitool.ToolOption{
		aitool.WithStringParam(
			"tool_require_payload",
			aitool.WithParam_Description("USE THIS FIELD ONLY IF type is 'require_tool'. Provide the exact name of the tool you need to use (e.g., 'check-yaklang-syntax', 'yak-document'). Another system will handle the parameter generation based on this name. Do NOT include tool arguments here."),
		),
	},
	ActionVerifier: func(loop *reactloops.ReActLoop, action *aicommon.Action) error {
		payload := action.GetString("tool_require_payload")
		if payload == "" {
			payload = action.GetInvokeParams("next_action").GetString("tool_require_payload")
		}
		if payload == "" {
			return utils.Error("tool_require_payload is required for ActionRequireTool but empty")
		}
		loop.Set("tool_require_payload", payload)
		return nil
	},
	ActionHandler: func(loop *reactloops.ReActLoop, action *aicommon.Action, operator *reactloops.LoopActionHandlerOperator) {
		toolPayload := loop.Get("tool_require_payload")
		if toolPayload == "" {
			operator.Feedback(utils.Error("tool_require_payload is required for ActionRequireTool but empty"))
			return
		}
		invoker := loop.GetInvoker()
		ctx := invoker.GetConfig().GetContext()
		t := loop.GetCurrentTask()
		if t != nil {
			ctx = t.GetContext()
		}
		result, directly, err := invoker.ExecuteToolRequiredAndCall(ctx, toolPayload)
		if err != nil {
			// Record the error in timeline and allow AI to retry with a different tool or approach
			errMsg := fmt.Sprintf("Tool '%s' execution failed: %v.", toolPayload, err)
			invoker.AddToTimeline("[TOOL_EXECUTION_ERROR]", errMsg)

			// Try to resolve the identifier - it might be a forge or skill, not a tool
			resolved := loop.ResolveIdentifier(toolPayload)
			if !resolved.IsUnknown() && resolved.IdentityType != aicommon.ResolvedAs_Tool {
				// The identifier exists as a different type - provide clear guidance
				invoker.AddToTimeline("identifier_resolved", resolved.Suggestion)
				operator.Feedback(errMsg + "\n\n" + resolved.Suggestion)
			} else {
				operator.Feedback(errMsg + " Please try a different tool or approach.")
			}

			// Set reflection level to help AI understand the failure
			operator.SetReflectionLevel(reactloops.ReflectionLevel_Critical)
			operator.SetReflectionData("tool_error", err.Error())
			operator.SetReflectionData("tool_name", toolPayload)
			operator.SetReflectionData("resolved_type", string(resolved.IdentityType))
			// Continue the loop to give AI a chance to retry
			operator.Continue()
			return
		}
		if directly {
			answer, err := invoker.DirectlyAnswer(ctx, "在上一次工具调用中，用户中断了工具执行，要求直接回答一些问题", nil)
			if err != nil {
				operator.Fail(utils.Error("DirectlyAnswer fail, reason: " + err.Error()))
				return
			}
			invoker.AddToTimeline("directly-answer", answer)
			operator.Exit()
			return
		}

		if result == nil {
			msg := fmt.Sprintf("ExecuteToolRequiredAndCall[%v] returned nil result", toolPayload)
			invoker.AddToTimeline("error", msg)
			operator.Continue()
			return
		}

		if result.Error != "" {
			invoker.AddToTimeline("call["+toolPayload+"] error", result.Error)
		}

		task := loop.GetCurrentTask()
		verifyResult, err := invoker.VerifyUserSatisfaction(ctx, task.GetUserInput(), true, toolPayload)
		if err != nil {
			operator.Fail(err)
			return
		}
		loop.PushSatisfactionRecordWithCompletedTaskIndex(verifyResult.Satisfied, verifyResult.Reasoning, verifyResult.CompletedTaskIndex, verifyResult.NextMovements)

		if verifyResult.Satisfied {
			operator.Exit()
			return
		}

		feedbackMsg := fmt.Sprintf("[Verification] Task not yet satisfied.\nReasoning: %s", verifyResult.Reasoning)
		if verifyResult.NextMovements != "" {
			feedbackMsg += fmt.Sprintf("\nNext Steps: %s", verifyResult.NextMovements)
		}
		operator.Feedback(feedbackMsg)
		operator.Continue()
	},
}
