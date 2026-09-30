package reactloops

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

const forcedToolCallAttempts = 3

func (r *ReActLoop) RequestForcedToolCall(ctx context.Context, task aicommon.AIStatefulTask, toolName string) (LoopCall, error) {
	if utils.IsNil(task) {
		return LoopCall{}, utils.Error("forced tool call requires a task")
	}
	if !r.actions.Have(schema.AI_REACT_LOOP_ACTION_DIRECTLY_CALL_TOOL) {
		action, err := requireRegisteredLoopAction(schema.AI_REACT_LOOP_ACTION_DIRECTLY_CALL_TOOL, "forced tool call")
		if err != nil {
			return LoopCall{}, err
		}
		r.actions.Set(action.ActionType, action)
	}
	operator := newLoopActionHandlerOperator(task)
	userInput := task.GetUserInput()
	var frozenUserContext string
	if provider, ok := task.(aicommon.CacheableUserInputProvider); ok {
		userInput, frozenUserContext = provider.GetUserInputSplitForCache()
	}
	var lastErr error
	for attempt := 0; attempt < forcedToolCallAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return LoopCall{}, err
		}
		currentNonce := utils.RandAlphaNumStringBytes(5)
		directive := fmt.Sprintf("Call exactly one tool using directly_call_tool. The tool name must be %q. Read its complete schema in CACHE_TOOL_CALL and construct arguments for the current task. Do not call another tool, return a batch, finish, or answer directly.", toolName)
		prompt, err := r.generateLoopPrompt(currentNonce, userInput, frozenUserContext, nil,
			r.GetCurrentMemoriesContent(), operator)
		if err != nil {
			return LoopCall{}, err
		}
		prompt += "\n\n" + directive
		streamWg := new(sync.WaitGroup)
		calls, stopReason, descriptor, err := r.callAILoopTransaction(streamWg, prompt, currentNonce, operator,
			r.emitLoopGeneralOutput, r.emitLoopFunctionCallOutput, ctx)
		streamWg.Wait()
		if ctx.Err() != nil {
			r.clearCallsExecutionValues(calls)
			return LoopCall{}, ctx.Err()
		}
		if err != nil {
			r.clearCallsExecutionValues(calls)
			return LoopCall{}, err
		}
		for _, call := range calls {
			if call.Action != nil {
				if err := call.Action.WaitParseResult(ctx); err != nil {
					r.clearCallsExecutionValues(calls)
					return LoopCall{}, err
				}
				call.Action.WaitStream(ctx)
			}
		}
		if stopReason == LoopStopToolCalls {
			if err := r.appendFunctionCallActionResponse(calls, descriptor); err != nil {
				r.clearCallsExecutionValues(calls)
				return LoopCall{}, err
			}
		} else {
			r.recordModelThinkingTimeline(strings.TrimSpace(r.takeModelThinkingForTimeline()),
				r.Get("last_ai_decision_response"), currentNonce, len(calls) > 0 && calls[0].Action != nil)
		}
		if lastErr = validateForcedToolCall(ctx, calls, toolName); lastErr != nil {
			for _, call := range calls {
				r.appendFunctionCallActionEvent(call, "rejected", lastErr.Error())
			}
			r.clearCallsExecutionValues(calls)
			feedback := fmt.Sprintf("Attempt %d/%d: %v. No tool was executed; call only %q.", attempt+1, forcedToolCallAttempts, lastErr, toolName)
			r.GetInvoker().AddToTimeline("forced_tool_call_retry", feedback)
			operator = newLoopActionHandlerOperator(task)
			operator.Feedback(feedback)
			continue
		}
		call := calls[0]
		return call, nil
	}
	return LoopCall{}, fmt.Errorf("AI failed to call tool %q after %d attempts: %w", toolName, forcedToolCallAttempts, lastErr)
}

func validateForcedToolCall(ctx context.Context, calls []LoopCall, toolName string) error {
	if len(calls) != 1 || calls[0].Action == nil || calls[0].LoopAction == nil || calls[0].LoopAction.ActionHandler == nil {
		return utils.Error("expected exactly one directly_call_tool action")
	}
	call := calls[0]
	if err := call.Action.WaitParseResult(ctx); err != nil {
		return err
	}
	if call.LoopAction.ActionType != schema.AI_REACT_LOOP_ACTION_DIRECTLY_CALL_TOOL {
		return utils.Errorf("expected directly_call_tool, got %q", call.LoopAction.ActionType)
	}
	params := call.Action.GetParams()
	nested := params.GetObject("next_action")
	if _, exists := params["directly_call_tool_calls"]; exists {
		return utils.Error("forced tool call requires a single call, not a batch")
	}
	if _, exists := nested["directly_call_tool_calls"]; exists {
		return utils.Error("forced tool call requires a single call, not a batch")
	}
	name := params.GetString("directly_call_tool_name")
	if name == "" {
		name = nested.GetString("directly_call_tool_name")
	}
	if strings.TrimSpace(name) != toolName {
		return utils.Errorf("expected tool %q, got %q", toolName, name)
	}
	return nil
}
