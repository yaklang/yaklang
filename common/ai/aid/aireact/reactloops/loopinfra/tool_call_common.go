package loopinfra

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools"
	"github.com/yaklang/yaklang/common/utils"
)

// Failed direct calls expose the real schema without executing or generating
// parameters. Cache admission still honors the loop's protocol and token budget.
func directToolRetryFeedback(loop *reactloops.ReActLoop, name, reason string, mayHaveExecuted bool) string {
	schemaState := "工具不可用；先核对当前工具列表中的准确名称，不要编造参数。"
	if config := loop.GetConfig(); config != nil && config.GetAiToolManager() != nil {
		tool, err := config.GetAiToolManager().GetToolByName(name)
		if err == nil && tool != nil && !buildinaitools.IsMCPPendingStub(tool) &&
			(!buildinaitools.IsMCPToolName(name) || aicommon.IsMCPServersAllowedConfig(config)) {
			mutation := loop.RecordRecentlyUsedTool(tool)
			if mutation.Upsert != nil || mutation.Reuse != nil {
				schemaState = "完整 Schema 已放入 CACHE_TOOL_CALL；按定义修正完整参数，不要重复加载或委托其他模型生成参数。"
			} else {
				schemaState = "工具存在，但 Schema 超出当前缓存预算；缩小缓存范围后按真实定义构参，不要猜测。"
			}
		}
	}
	execution := "本次未执行工具。"
	if mayHaveExecuted {
		execution = "先检查工具执行结果及副作用，只重试尚未完成的操作，避免重复执行。"
	}
	hint := fmt.Sprintf("工具 %q 调用失败。\nreason: %s\nretry: %s%s修正后继续 directly_call_tool 完成本任务，不要等待用户说继续。", name, reason, schemaState, execution)
	loop.GetInvoker().AddToTimeline("direct_tool_retry", hint)
	return hint
}

type directToolValidationError struct {
	error
	feedback string
}

func (e *directToolValidationError) Unwrap() error { return e.error }

func directToolParameterError(loop *reactloops.ReActLoop, name string, err error) error {
	feedback := directToolRetryFeedback(loop, name, err.Error(), false)
	return &directToolValidationError{utils.Wrap(err, "reason: direct-call validation failed; retry: correct arguments using the tool schema and retry directly_call_tool; no tool was executed; runtime never generates parameters"), feedback}
}

// Successful direct calls refresh their schemas at the same result boundary
// for scalar and batch execution. Validation failures load schemas separately.
func recordSuccessfulToolCache(loop *reactloops.ReActLoop, name string, result *aitool.ToolResult, callErr error) {
	config := loop.GetConfig()
	if config == nil || config.GetAiToolManager() == nil || callErr != nil || result == nil || !result.Success {
		return
	}
	if tool, err := config.GetAiToolManager().GetToolByName(name); err == nil && tool != nil {
		loop.RecordRecentlyUsedTool(tool)
	}
}

// resolveToolCallReason extracts the human-readable reason for a tool call from
// the action: it prefers the action-specific reason field (e.g. tool_call_reason)
// and falls back to human_readable_thought when the AI omitted the dedicated
// reason field. Native direct calls use this helper; text directly_call_tool reads its reason inside
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

// handleToolCallResult handles the shared direct-call execution result boundary.
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
	// A non-nil ToolResult is the objective boundary that proves the plugin
	// callback settled. Count both completed and protocol-failed calls. Review-driven
	// direct_answer/cancel is terminal user intent, not a tool execution.
	if !directly && result != nil {
		operator.MarkToolExecuted()
	}
	var reconsider *aicommon.ToolReviewReconsiderError
	if errors.As(err, &reconsider) {
		operator.Feedback(reconsider.Feedback)
		operator.Continue()
		return
	}
	if err != nil {
		errMsg := fmt.Sprintf("Tool '%s' invocation protocol failed: %v.", toolPayload, err)
		invoker.AddToTimeline("[TOOL_PROTOCOL_ERROR]", errMsg)
		emitToolResultStatus(loop, toolPayload, false)

		resolved := loop.ResolveIdentifier(toolPayload)
		var validationErr *directToolValidationError
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			operator.Feedback(errMsg + "任务已取消或超时，不要继续执行工具。")
		} else if errors.As(err, &validationErr) {
			operator.Feedback(validationErr.feedback)
		} else if buildinaitools.IsMCPToolName(toolPayload) && buildinaitools.IsMCPInitializingError(err) {
			operator.Feedback(errMsg + "\n\n[MCP] This MCP tool is still connecting to its remote server. " +
				"Retry the same tool with directly_call_tool after it is ready.\n" + directToolRetryFeedback(loop, toolPayload, err.Error(), true))
		} else if !resolved.IsUnknown() && resolved.IdentityType != aicommon.ResolvedAs_Tool {
			invoker.AddToTimeline("identifier_resolved", resolved.Suggestion)
			operator.Feedback(errMsg + "\n\n" + resolved.Suggestion)
		} else {
			operator.Feedback(directToolRetryFeedback(loop, toolPayload, err.Error(), true))
		}
		operator.Continue()
		return
	}

	if directly {
		loop.StopSubAgentsForUser()
		answer, answerErr := invoker.DirectlyAnswer(ctx,
			"在上一次工具调用中，用户中断了工具执行，要求直接回答一些问题。一般这种情况出现在用户认为这个任务不应该使用工具或者工具无法满足需求的情况下。", nil)
		if answerErr != nil {
			operator.Fail(utils.Error("DirectlyAnswer fail, reason: " + answerErr.Error()))
			return
		}
		invoker.AddToTimeline("directly-answer", answer)
		operator.ExitForUser()
		return
	}

	if result == nil {
		msg := fmt.Sprintf("tool call [%v] returned nil result", toolPayload)
		invoker.AddToTimeline("error", msg)
		emitToolResultStatus(loop, toolPayload, false)
		operator.Feedback(directToolRetryFeedback(loop, toolPayload, msg, true))
		operator.Continue()
		return
	}

	if result.Success {
		reactloops.MarkEditBeforeExecutionCompleted(loop, toolPayload)
		emitToolResultStatus(loop, toolPayload, true)
	}

	if result.Error != "" {
		invoker.AddToTimeline("call["+toolPayload+"] protocol-error", result.Error)
		emitToolResultStatus(loop, toolPayload, false)
		if buildinaitools.IsMCPToolName(toolPayload) && buildinaitools.IsMCPInitializingMessage(result.Error) {
			operator.Feedback(
				"[MCP] Tool '" + toolPayload + "' is still initializing. " +
					"Retry the same tool with directly_call_tool after it is ready.\n" + directToolRetryFeedback(loop, toolPayload, result.Error, true),
			)
		} else {
			operator.Feedback(directToolRetryFeedback(loop, toolPayload, result.Error, true))
		}
	}

	if status, detail := result.GetExecutionStatus(); result.Error == "" && status == aitool.ToolExecutionStatusFailed {
		operator.Feedback(directToolRetryFeedback(loop, toolPayload, detail, true))
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

	// verification 现在是纯观测调用, 不再决定退出. 当本轮触发了 verification
	// 且观测到未满足时, 把 reasoning 作为 feedback 沉淀给下一轮; satisfied
	// 时也只继续, 退出唯一由 AI 主动 finish action 决定.
	// 关键词: verification 不退, 退出只走 finished, 纯观测角色
	if !verifyResult.Satisfied {
		feedbackMsg := fmt.Sprintf("[Verification] Task not yet satisfied.\nReasoning: %s", verifyResult.Reasoning)
		operator.Feedback(feedbackMsg)
	}
	operator.Continue()
}
