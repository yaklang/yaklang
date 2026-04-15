package loopinfra

import (
	"context"
	"fmt"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/utils"
)

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

		resolved := loop.ResolveIdentifier(toolPayload)
		if !resolved.IsUnknown() && resolved.IdentityType != aicommon.ResolvedAs_Tool {
			invoker.AddToTimeline("identifier_resolved", resolved.Suggestion)
			operator.Feedback(errMsg + "\n\n" + resolved.Suggestion)
		} else {
			operator.Feedback(errMsg + " Please try a different tool or approach.")
		}

		operator.SetReflectionLevel(reactloops.ReflectionLevel_Critical)
		operator.SetReflectionData("tool_error", err.Error())
		operator.SetReflectionData("tool_name", toolPayload)
		operator.SetReflectionData("resolved_type", string(resolved.IdentityType))
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
		operator.Continue()
		return
	}

	if result.Success {
		reactloops.MarkEditBeforeExecutionCompleted(loop, toolPayload)
	}

	if result.Error != "" {
		invoker.AddToTimeline("call["+toolPayload+"] error", result.Error)
	}

	task := loop.GetCurrentTask()
	verifyResult, verifyErr := invoker.VerifyUserSatisfaction(ctx, task.GetUserInput(), true, toolPayload)
	if verifyErr != nil {
		operator.Fail(verifyErr)
		return
	}

	if len(verifyResult.OutputFiles) > 0 {
		cfg := loop.GetConfig()
		for _, filePath := range verifyResult.OutputFiles {
			providerName := "output_file:" + filePath
			cfg.GetContextProviderManager().RegisterTracedContent(
				providerName,
				aicommon.OutputFileContextProvider(filePath),
			)
			if emitter := cfg.GetEmitter(); emitter != nil {
				emitter.EmitPinFilename(filePath)
			}
		}
	}

	loop.PushSatisfactionRecordWithCompletedTaskIndex(
		verifyResult.Satisfied, verifyResult.Reasoning,
		verifyResult.CompletedTaskIndex, verifyResult.NextMovements, verifyResult.Evidence, verifyResult.OutputFiles,
		verifyResult.EvidenceOps,
	)

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
}
