package loopinfra

import (
	"context"
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools"
	"github.com/yaklang/yaklang/common/utils"
)

// resolveToolCallReason extracts the human-readable reason for a tool call from
// the action: it prefers the action-specific reason field (e.g. tool_call_reason)
// and falls back to human_readable_thought when the AI omitted the dedicated
// reason field. Used by require_tool; directly_call_tool reads its reason inside
// aicommon.ToolCaller.DirectlyCallTool (so the card is emitted before the reason
// streams in).
func resolveToolCallReason(action *aicommon.Action, reasonKey string) string {
	if action == nil {
		return ""
	}
	if r := strings.TrimSpace(action.GetString(reasonKey)); r != "" {
		return r
	}
	return strings.TrimSpace(action.GetString("human_readable_thought"))
}

// handleToolCallResult processes the result returned by ExecuteToolRequiredAndCall
// or ExecuteToolRequiredAndCallWithoutRequired. It is shared by require_tool and
// directly_call_tool action handlers.
func handleToolCallResult(
	loop *reactloops.ReActLoop,
	ctx context.Context,
	invoker aicommon.AIInvokeRuntime,
	toolPayload string,
	result *aitool.ToolResult,
	directly bool,
	err error,
	operator *reactloops.LoopActionHandlerOperator,
) {
	if err != nil {
		errMsg := fmt.Sprintf("Tool '%s' execution failed: %v.", toolPayload, err)
		invoker.AddToTimeline("[TOOL_EXECUTION_ERROR]", errMsg)
		loopInfraStatus(loop, "工具调用失败 / Tool Call Failed")

		resolved := loop.ResolveIdentifier(toolPayload)
		if buildinaitools.IsMCPToolName(toolPayload) && buildinaitools.IsMCPInitializingError(err) {
			operator.Feedback(errMsg + "\n\n[MCP] This MCP tool is still connecting to its remote server. " +
				"Wait a few seconds and call the same tool again with require_tool; do NOT switch to an unrelated tool.")
			operator.SetReflectionLevel(reactloops.ReflectionLevel_Standard)
		} else if !resolved.IsUnknown() && resolved.IdentityType != aicommon.ResolvedAs_Tool {
			invoker.AddToTimeline("identifier_resolved", resolved.Suggestion)
			operator.Feedback(errMsg + "\n\n" + resolved.Suggestion)
			operator.SetReflectionLevel(reactloops.ReflectionLevel_Critical)
		} else {
			operator.Feedback(errMsg + " Please try a different tool or approach.")
			operator.SetReflectionLevel(reactloops.ReflectionLevel_Critical)
		}
		operator.SetReflectionData("tool_error", err.Error())
		operator.SetReflectionData("tool_name", toolPayload)
		if operator.GetReflectionLevel() == reactloops.ReflectionLevel_Critical {
			operator.SetReflectionData("resolved_type", string(resolved.IdentityType))
		}
		operator.Continue()
		return
	}

	if directly {
		answer, answerErr := invoker.DirectlyAnswer(ctx,
			"在上一次工具调用中，用户中断了工具执行，要求直接回答一些问题。一般这种情况出现在用户认为这个任务不应该使用工具或者工具无法满足需求的情况下。", nil)
		if answerErr != nil {
			operator.Fail(utils.Error("DirectlyAnswer fail, reason: " + answerErr.Error()))
			return
		}
		invoker.AddToTimeline("directly-answer", answer)
		operator.Exit()
		return
	}

	if result == nil {
		msg := fmt.Sprintf("tool call [%v] returned nil result", toolPayload)
		invoker.AddToTimeline("error", msg)
		loopInfraStatus(loop, "工具无结果 / Tool Returned No Result")
		operator.Continue()
		return
	}

	if result.Success {
		reactloops.MarkEditBeforeExecutionCompleted(loop, toolPayload)
		loopInfraStatus(loop, "工具调用完成 / Tool Call Complete")
	}

	if result.Error != "" {
		invoker.AddToTimeline("call["+toolPayload+"] error", result.Error)
		loopInfraStatus(loop, "工具返回错误 / Tool Returned Error")
		if buildinaitools.IsMCPToolName(toolPayload) && buildinaitools.IsMCPInitializingMessage(result.Error) {
			operator.Feedback(
				"[MCP] Tool '" + toolPayload + "' is still initializing. " +
					"Wait briefly, then retry the same tool with require_tool. Do NOT substitute an unrelated tool.",
			)
		}
	}

	task := loop.GetCurrentTask()
	if task == nil {
		operator.Continue()
		return
	}

	verifyResult, triggered, verifyErr := loop.MaybeVerifyUserSatisfaction(ctx, task.GetUserInput(), true, toolPayload)
	if verifyErr != nil {
		operator.Fail(verifyErr)
		return
	}
	if !triggered || verifyResult == nil {
		operator.Continue()
		return
	}

	if verifyResult.Satisfied && !aicommon.HasNewTodoAddOps(verifyResult.NextMovements) {
		operator.Exit()
		return
	}

	feedbackMsg := fmt.Sprintf("[Verification] Task not yet satisfied.\nReasoning: %s", verifyResult.Reasoning)
	if summary := aicommon.FormatVerifyNextMovementsSummary(verifyResult.NextMovements); summary != "" {
		feedbackMsg += fmt.Sprintf("\nNext Steps: %s", summary)
	}
	operator.Feedback(feedbackMsg)
	operator.Continue()
}
