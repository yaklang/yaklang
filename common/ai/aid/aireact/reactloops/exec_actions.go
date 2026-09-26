package reactloops

import (
	"encoding/json"
	"fmt"
	"github.com/yaklang/yaklang/common/ai/aid/aiprojection"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

const functionCallActionResponseTimelineEntry = "FUNCTION_CALL_ACTION_RESPONSE"

type loopActionsDisposition uint8

const (
	loopActionsContinue loopActionsDisposition = iota
	loopActionsExit
	loopActionsAsync
	loopActionsError
)

type loopActionsResult struct {
	operator *LoopActionHandlerOperator
	result   loopActionsDisposition
	err      error
	// Internal execution facts are combined once per model response, even when
	// the response contains several native function calls.
	appliedTodoDelta         *aicommon.TodoDelta
	countsEffectiveIteration bool
	// An admission rejection starts another model iteration without a
	// post-iteration callback, matching the existing single-action path.
	skipPostIteration     bool
	skipErrorFinalization bool
	skipErrorSummary      bool
}

type loopActionEventSink func(LoopCall, string, string)

func emitLoopActionEvent(sink loopActionEventSink, call LoopCall, status, detail string) {
	if sink != nil {
		sink(call, status, detail)
	}
}

// appendFunctionCallActionResponse records a complete, immutable protocol
// group before any handler runs. The tool messages acknowledge receipt; the
// actual outcomes are ordinary timeline events written by the handlers below.
func (r *ReActLoop) appendFunctionCallActionResponse(calls []LoopCall, descriptor *LoopResultDescriptor) error {
	if r == nil || len(calls) == 0 || descriptor == nil {
		return fmt.Errorf("function-call action response requires calls and a response descriptor")
	}
	projected, ok := r.GetInvoker().(promptProjectedTimelineRuntime)
	if !ok {
		return fmt.Errorf("function-call mode requires a prompt-projected timeline runtime")
	}
	snapshot := descriptor.Snapshot()
	if !snapshot.Complete || snapshot.ProviderFinishReason != "tool_calls" {
		return fmt.Errorf("function-call action response requires a complete tool_calls response")
	}
	var response struct {
		Content          string `json:"content"`
		ReasoningContent string `json:"reasoning_content"`
	}
	if err := json.Unmarshal([]byte(snapshot.ResponseJSON), &response); err != nil {
		return fmt.Errorf("decode function-call response: %w", err)
	}
	toolCalls := make([]*aispec.ToolCall, 0, len(calls))
	messages := make([]any, 0, len(calls)+1)
	seen := make(map[string]struct{}, len(calls))
	var display strings.Builder
	display.WriteString("Model requested actions:")
	for _, call := range calls {
		if call.Action == nil || call.LoopAction == nil || !validActionToolName(call.Action.Name()) ||
			!validActionReplayCallID(call.ToolCallID) || !validActionReplayArguments(call.ArgumentsJSON) {
			return fmt.Errorf("invalid native action call for replay: id=%q", call.ToolCallID)
		}
		if _, exists := seen[call.ToolCallID]; exists {
			return fmt.Errorf("duplicate native tool call id %q", call.ToolCallID)
		}
		seen[call.ToolCallID] = struct{}{}
		toolCalls = append(toolCalls, &aispec.ToolCall{
			Index: call.Index, ID: call.ToolCallID, Type: "function", Description: call.Description,
			Function: aispec.FuncReturn{Name: call.Action.Name(), Arguments: call.ArgumentsJSON},
		})
		fmt.Fprintf(&display, "\n- %s (tool_call_id=%s): received; execution outcome follows in timeline", call.Action.Name(), call.ToolCallID)
	}
	messages = append(messages, map[string]any{
		"role": "assistant", "content": response.Content,
		"reasoning_content": response.ReasoningContent, "tool_calls": toolCalls,
	})
	for _, call := range calls {
		ack, err := json.Marshal(map[string]string{
			"status": "accepted", "tool_call_id": call.ToolCallID,
			"detail": "Execution status and result follow in timeline events with this tool_call_id.",
		})
		if err != nil {
			return err
		}
		messages = append(messages, map[string]any{
			"role": "tool", "tool_call_id": call.ToolCallID, "content": string(ack),
		})
	}
	encoded, err := json.Marshal(messages)
	if err != nil {
		return fmt.Errorf("encode function-call action response: %w", err)
	}
	projection, err := aiprojection.CreateActionResponse(encoded)
	if err != nil {
		return err
	}
	projected.AddToTimelineWithPromptProjection(functionCallActionResponseTimelineEntry, display.String(), projection)
	return nil
}

func validActionReplayCallID(id string) bool {
	if len(id) == 0 || len(id) > 128 {
		return false
	}
	for _, char := range id {
		if char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' ||
			char >= '0' && char <= '9' || char == '_' || char == '-' {
			continue
		}
		return false
	}
	return true
}

func validActionReplayArguments(arguments string) bool {
	var object map[string]json.RawMessage
	return json.Unmarshal([]byte(arguments), &object) == nil && object != nil
}

func (r *ReActLoop) appendFunctionCallActionEvent(call LoopCall, status, detail string) {
	if r == nil || r.GetInvoker() == nil || call.ToolCallID == "" {
		return
	}
	if len(detail) > 4096 {
		detail = detail[:4096] + "…"
	}
	encoded, err := json.Marshal(map[string]string{
		"tool_call_id": call.ToolCallID,
		"action":       call.Action.Name(),
		"status":       status,
		"detail":       detail,
	})
	if err == nil {
		r.GetInvoker().AddToTimeline("FUNCTION_CALL_ACTION_RESULT", string(encoded))
	}
}

// execCalls runs every resolved action through the same path, regardless of
// how the model expressed its calls. Protocol-specific timeline events are
// supplied by the caller through an optional sink.
func (r *ReActLoop) execCalls(
	calls []LoopCall, iteration int, task aicommon.AIStatefulTask,
	prompt string, done *utils.Once, eventSink loopActionEventSink,
) loopActionsResult {
	result := loopActionsResult{operator: newLoopActionHandlerOperator(task), result: loopActionsContinue}
	if len(calls) == 0 {
		result.result, result.err = loopActionsError, fmt.Errorf("AI loop returned no actions")
		return result
	}
	r.Delete(loopVarNativeTodoBatchAdjusted)
	defer r.Delete(loopVarNativeTodoBatchAdjusted)
	remainingAdjustments := 0
	if r.functionCallMode {
		for _, call := range calls {
			if call.Action == nil || call.Action.Name() != nativeAdjustTodolistActionName {
				continue
			}
			if delta, err := aicommon.NormalizeTodoDelta(call.Action); err == nil && delta != nil {
				remainingAdjustments++
			}
		}
	}
	appliedAdjustment := false
	answerDelivered := false
	countIteration := false
	defer func() {
		// An answer may precede a planned adjustment. If that adjustment later
		// fails or is skipped, restore the ordinary duplicate-answer guard.
		if r.functionCallMode && answerDelivered && !appliedAdjustment {
			r.Set(loopVarDirectlyAnswerDeliveredWithoutTodoDelta, true)
			r.subAgentAnswerSeen = r.subAgentModelSeen
		}
		if countIteration {
			r.effectiveIterationCount++
		}
	}()
	for index, call := range calls {
		if appliedAdjustment || remainingAdjustments > 0 {
			r.Set(loopVarNativeTodoBatchAdjusted, true)
		} else {
			r.Delete(loopVarNativeTodoBatchAdjusted)
		}
		outcome := r.execOneCall(call, iteration, task, prompt, done, index, len(calls), eventSink)
		countIteration = countIteration || outcome.countsEffectiveIteration
		if call.Action != nil && call.Action.Name() == nativeAdjustTodolistActionName {
			if delta, err := aicommon.NormalizeTodoDelta(call.Action); err == nil && delta != nil {
				remainingAdjustments--
			}
			appliedAdjustment = appliedAdjustment || outcome.appliedTodoDelta != nil && outcome.appliedTodoDelta.HasChanges()
		}
		if call.Action != nil && call.Action.Name() == loopAction_DirectlyAnswer.ActionType &&
			outcome.err == nil && !outcome.skipPostIteration {
			answerDelivered = true
		}
		if len(calls) == 1 {
			result.operator = outcome.operator
		} else {
			mergeLoopActionOperator(result.operator, outcome.operator)
		}
		if outcome.result != loopActionsContinue || outcome.skipPostIteration {
			result.result, result.err, result.skipPostIteration = outcome.result, outcome.err, outcome.skipPostIteration
			result.skipErrorFinalization, result.skipErrorSummary = outcome.skipErrorFinalization, outcome.skipErrorSummary
			for _, skipped := range calls[index+1:] {
				emitLoopActionEvent(eventSink, skipped, "skipped", "An earlier action ended this batch before this call could run.")
			}
			return result
		}
	}
	return result
}

func mergeLoopActionOperator(dst, src *LoopActionHandlerOperator) {
	if dst == nil || src == nil {
		return
	}
	if feedback := strings.TrimSpace(src.GetFeedback().String()); feedback != "" {
		dst.Feedback(feedback)
	}
	dst.nextActionMustUse = append(dst.nextActionMustUse, src.nextActionMustUse...)
	dst.nextActionDisabled = append(dst.nextActionDisabled, src.nextActionDisabled...)
	if src.GetDisallowLoopExit() {
		dst.DisallowNextLoopExit()
	}
	if src.IsContinued() {
		dst.Continue()
	}
}

func (r *ReActLoop) execOneCall(
	call LoopCall, iteration int, task aicommon.AIStatefulTask,
	prompt string, done *utils.Once, position, callCount int, eventSink loopActionEventSink,
) loopActionsResult {
	op := newLoopActionHandlerOperator(task)
	result := loopActionsResult{operator: op, result: loopActionsContinue}
	action, handler := call.Action, call.LoopAction
	if action == nil || handler == nil {
		result.result, result.err = loopActionsError, fmt.Errorf("nil action or handler for call %q", call.ToolCallID)
		return result
	}
	actionName := action.Name()
	emitLoopActionEvent(eventSink, call, "started", "Executing action.")
	r.UserStatus("正在执行下一步", "Executing the next step", aicommon.WithStatusCode("action.running"))
	toolNames := extractToolNamesFromAction(action)
	record := &ActionRecord{
		ActionType: action.ActionType(), ActionName: actionName,
		ActionParams: cloneActionParams(action.GetParams()), IterationIndex: iteration,
		ToolNames: toolNames, ToolCallCount: len(toolNames),
	}
	if len(toolNames) > 0 {
		record.ToolName = toolNames[0]
	}
	r.actionHistoryMutex.Lock()
	r.actionHistory = append(r.actionHistory, record)
	r.actionHistoryMutex.Unlock()
	artifactSuffix := call.ToolCallID
	if artifactSuffix == "" && callCount > 1 {
		artifactSuffix = fmt.Sprintf("call_%d", position+1)
	}
	if artifactSuffix == "" {
		r.emitActionExecutionRecord(task, action, iteration, prompt)
	} else {
		r.emitActionExecutionRecord(task, action, iteration, prompt, artifactSuffix)
	}
	appliedTodoDelta := applyTodoDeltaBottomLine(r, task, iteration, action)
	result.appliedTodoDelta = appliedTodoDelta
	if IsSubAgentControlAction(actionName) {
		r.subAgentControlIterations++
	} else if actionName == nativeAdjustTodolistActionName {
		result.countsEffectiveIteration = appliedTodoDelta != nil && appliedTodoDelta.HasChanges()
	} else {
		result.countsEffectiveIteration = r.shouldAdvanceEffectiveIteration(task, appliedTodoDelta)
	}
	if handler.AsyncMode || actionName == schema.AI_REACT_LOOP_ACTION_REQUIRE_AI_BLUEPRINT ||
		actionName == schema.AI_REACT_LOOP_ACTION_REQUEST_PLAN || actionName == schema.AI_REACT_LOOP_ACTION_REQUEST_PLAN_EXECUTION {
		if reason := r.SubAgentFinishBlockReason(); reason != "" {
			op.Feedback(reason)
			op.Continue()
			emitLoopActionEvent(eventSink, call, "rejected", reason)
			result.skipPostIteration = true
			return result
		}
	}
	if handler.AsyncMode {
		r.UserStatus("这项工作已转入后台继续处理", "This work is continuing in the background", aicommon.WithStatusCode("task.background"))
		if task.IsAsyncMode() {
			r.UserStatus("这项工作正在后台继续处理", "This work is continuing in the background", aicommon.WithStatusCode("task.background"))
			log.Warnf("ReactLoop[%v] rejecting static async action '%v' because the current task is already in async mode", r.loopName, actionName)
			msg := fmt.Sprintf("REJECTED: action '%s' requires async mode, but the current task is already running asynchronously. "+
				"You MUST NOT start another async operation while one is in progress. "+
				"Wait for the current async task to complete, or choose a synchronous action instead.", actionName)
			r.GetInvoker().AddToTimeline("[ASYNC_ACTION_REJECTED]", msg)
			op.Feedback(msg)
			op.Continue()
			emitLoopActionEvent(eventSink, call, "rejected", msg)
			result.skipPostIteration = true
			return result
		}
		task.SetAsyncMode(true)
		r.emitter.EmitJSON(schema.EVENT_TYPE_AI_TASK_SWITCHED_TO_ASYNC, "react_task_mode_changed", map[string]any{
			"task_id": task.GetId(), "loop_name": r.loopName, "task_index": task.GetIndex(), "task_user_input": task.GetUserInput(),
		})
		if r.onAsyncTaskTrigger != nil {
			r.onAsyncTaskTrigger(handler, task)
		}
		done.Do(func() { log.Infof("async mode, not update task status in mainloop") })
	}
	if handler.ActionHandler == nil {
		result.result, result.err = loopActionsError, fmt.Errorf("action[%s] has no ActionHandler", actionName)
		emitLoopActionEvent(eventSink, call, "failed", result.err.Error())
		return result
	}
	// Transaction-time verification may accept an adjust_todolist that turns
	// out to be idempotent at apply time. Recheck before delivering a repeated
	// answer so a no-op adjustment cannot bypass the duplicate-output guard.
	if r.functionCallMode && actionName == loopAction_DirectlyAnswer.ActionType {
		if err := RejectDuplicateDirectlyAnswerWithoutTodoDelta(r, action); err != nil {
			op.Feedback(err.Error())
			op.Continue()
			emitLoopActionEvent(eventSink, call, "rejected", err.Error())
			result.skipPostIteration = true
			return result
		}
	}
	if err := task.GetContext().Err(); err != nil {
		result.result, result.err = loopActionsError, fmt.Errorf("task context done before action handler: %w", err)
		result.skipErrorFinalization, result.skipErrorSummary = true, true
		emitLoopActionEvent(eventSink, call, "failed", result.err.Error())
		return result
	}
	invoker := r.GetInvoker()
	previousTask := invoker.GetCurrentTask()
	invoker.SetCurrentTask(task)
	func() {
		defer invoker.SetCurrentTask(previousTask)
		r.UserStatus("正在执行下一步", "Executing the next step", aicommon.WithStatusCode("action.running"))
		handler.ActionHandler(r, action, op)
	}()
	r.applyActionExecutionRecord(record, op)
	if err := r.recordSubAgentControl(actionName, op, appliedTodoDelta); err != nil {
		result.result, result.err = loopActionsError, err
		result.skipErrorSummary = true
		emitLoopActionEvent(eventSink, call, "failed", err.Error())
		return result
	}
	if handler.ActionType != loopAction_Finish.ActionType && handler.ActionType != nativeAdjustTodolistActionName {
		r.recordCurrentTodoIteration(task)
	}
	if terminated, opErr := op.IsTerminated(); terminated {
		if opErr != nil {
			result.result, result.err = loopActionsError, opErr
			result.skipErrorSummary = true
			emitLoopActionEvent(eventSink, call, "failed", opErr.Error())
			return result
		}
		if reason := r.admitSubAgentExit(op); reason != "" {
			op.Feedback(reason)
			op.Continue()
			emitLoopActionEvent(eventSink, call, "rejected", reason)
			result.skipPostIteration = true
			return result
		}
		emitLoopActionEvent(eventSink, call, "completed", strings.TrimSpace(op.GetFeedback().String()))
		result.result = loopActionsExit
		return result
	}
	if !(op.IsAsyncModeRequested() || task.IsAsyncMode()) {
		if err := task.GetContext().Err(); err != nil {
			result.result, result.err = loopActionsError, fmt.Errorf("task context done after action handler: %w", err)
			result.skipErrorFinalization, result.skipErrorSummary = true, true
			emitLoopActionEvent(eventSink, call, "failed", result.err.Error())
			return result
		}
	}
	r.MaybeTriggerPerceptionAfterAction(iteration)
	if handler.AsyncMode || op.IsAsyncModeRequested() {
		var timelineHook func(string, string)
		if invoker != nil {
			timelineHook = invoker.AddToTimeline
		}
		aicommon.DeferOpenTodosOnAsyncHandoff(r.config, r.emitter, task, iteration, timelineHook)
		if !handler.AsyncMode {
			task.SetAsyncMode(true)
			r.emitter.EmitJSON(schema.EVENT_TYPE_AI_TASK_SWITCHED_TO_ASYNC, "react_task_mode_changed", map[string]any{
				"task_id": task.GetId(), "loop_name": r.loopName, "task_index": task.GetIndex(), "task_user_input": task.GetUserInput(),
			})
			if r.onAsyncTaskTrigger != nil {
				r.onAsyncTaskTrigger(handler, task)
			}
			done.Do(func() { log.Infof("dynamic async mode, not update task status in mainloop") })
		}
		r.UserStatus("这项工作已转入后台继续处理", "This work is continuing in the background", aicommon.WithStatusCode("task.background"))
		emitLoopActionEvent(eventSink, call, "async", "Execution continues in the background; later timeline events may contain the final result.")
		result.result = loopActionsAsync
		return result
	}
	emitLoopActionEvent(eventSink, call, "completed", strings.TrimSpace(op.GetFeedback().String()))
	return result
}
