package reactloops

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/mock"
	"github.com/yaklang/yaklang/common/ai/aid/aiprojection"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/utils"
)

type actionExecutionTestInvoker struct {
	*mock.MockInvoker
	mu       sync.Mutex
	timeline *aicommon.Timeline
	nextID   int64
	entries  []string
}

func (i *actionExecutionTestInvoker) AddToTimeline(entry, content string) {
	i.add(entry, content, "")
}

func (i *actionExecutionTestInvoker) AddToTimelineWithPromptProjection(entry, content, promptContent string) {
	i.add(entry, content, promptContent)
}

func (i *actionExecutionTestInvoker) add(entry, content, promptContent string) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.nextID++
	i.entries = append(i.entries, entry+":"+content)
	display := fmt.Sprintf("[%s]:\n%s", entry, content)
	if promptContent == "" {
		i.timeline.PushText(i.nextID, display)
		return
	}
	i.timeline.PushTextWithPromptProjection(i.nextID, display, fmt.Sprintf("[%s]:\n%s", entry, promptContent))
}

func newActionExecutionTestLoop(t *testing.T) (*ReActLoop, *actionExecutionTestInvoker, aicommon.AIStatefulTask) {
	t.Helper()
	config := mock.NewMockedAIConfig(context.Background()).(*mock.MockedAIConfig)
	base := mock.NewMockInvoker(context.Background())
	base.SetConfig(config)
	invoker := &actionExecutionTestInvoker{MockInvoker: base, timeline: aicommon.NewTimeline(nil, nil)}
	loop := NewMinimalReActLoop(config, invoker)
	loop.loopName = "actions-test"
	task := newMockSimpleTask("actions-test-task", "1")
	loop.SetCurrentTask(task)
	return loop, invoker, task
}

func actionExecutionTestDescriptor() *LoopResultDescriptor {
	descriptor := newLoopResultDescriptor("functioncall")
	descriptor.setProviderFinishReason("tool_calls", []byte(`{"finish_reason":"tool_calls"}`))
	descriptor.finish(nil, "model content", "model reasoning", nil)
	return descriptor
}

func actionExecutionTestCall(index int, id, name string, handler LoopActionHandlerFunc) LoopCall {
	action := aicommon.NewSimpleAction(name, aitool.InvokeParams(map[string]any{"@action": name}))
	return LoopCall{
		Action: action, LoopAction: &LoopAction{ActionType: name, ActionHandler: handler},
		ToolCallID: id, Index: index, ArgumentsJSON: "{}",
	}
}

func TestExecuteFunctionCallActionsSerialAndProjectsOneAssistantManyTools(t *testing.T) {
	loop, invoker, task := newActionExecutionTestLoop(t)
	var order []string
	calls := []LoopCall{
		actionExecutionTestCall(0, "call_a", "accept", func(_ *ReActLoop, _ *aicommon.Action, op *LoopActionHandlerOperator) {
			order = append(order, "a")
			invoker.AddToTimeline("ACTION_OUTPUT", "result A")
			op.Feedback("feedback A")
			op.Continue()
		}),
		actionExecutionTestCall(1, "call_b", "inspect", func(_ *ReActLoop, _ *aicommon.Action, op *LoopActionHandlerOperator) {
			order = append(order, "b")
			invoker.AddToTimeline("ACTION_OUTPUT", "result B")
			op.Feedback("feedback B")
			op.Continue()
		}),
	}
	require.NoError(t, loop.appendFunctionCallActionResponse(calls, actionExecutionTestDescriptor()))
	result := loop.execCalls(calls, 1, task, "prompt", utils.NewOnce(), loop.appendFunctionCallActionEvent)
	require.NoError(t, result.err)
	require.Equal(t, loopActionsContinue, result.result)
	require.Equal(t, []string{"a", "b"}, order)
	require.Len(t, loop.actionHistory, 2)
	require.Contains(t, result.operator.GetFeedback().String(), "feedback A")
	require.Contains(t, result.operator.GetFeedback().String(), "feedback B")
	require.True(t, strings.HasPrefix(invoker.entries[0], functionCallActionResponseTimelineEntry+":"))
	require.Contains(t, strings.Join(invoker.entries, "\n"), `"tool_call_id":"call_a"`)
	require.Contains(t, strings.Join(invoker.entries, "\n"), `"tool_call_id":"call_b"`)
	require.Less(t, strings.Index(strings.Join(invoker.entries, "\n"), "result A"), strings.Index(strings.Join(invoker.entries, "\n"), "result B"))

	parts := aicommon.RenderTimelineFrozenOpenWithLatestModelReplay(invoker.timeline)
	prompt := strings.Join([]string{
		"<|PROMPT_SECTION_high-static|>system<|PROMPT_SECTION_END_high-static|>",
		"<|PROMPT_SECTION_timeline-open|>" + parts.Open + "<|PROMPT_SECTION_END_timeline-open|>",
		"<|PROMPT_SECTION_dynamic_n|>next instruction<|PROMPT_SECTION_dynamic_END_n|>",
	}, "\n")
	projected := aiprojection.ProjectAndObserve("test-model", aiprojection.CreateTemplate(prompt))
	require.NotNil(t, projected)
	var roles []string
	for _, message := range projected.Messages {
		roles = append(roles, message.Role)
	}
	require.Equal(t, []string{"system", "user", "assistant", "tool", "tool", "user"}, roles)
	require.Len(t, projected.Messages[2].ToolCalls, 2)
	require.Equal(t, "model content", projected.Messages[2].Content)
	require.Equal(t, "model reasoning", projected.Messages[2].ReasoningContent)
	require.Equal(t, "call_a", projected.Messages[3].ToolCallID)
	require.Equal(t, "call_b", projected.Messages[4].ToolCallID)
	require.Contains(t, projected.Messages[5].Content, "result A")
	require.Contains(t, projected.Messages[5].Content, "result B")
}

func TestExecuteFunctionCallActionsFailureSkipsRemainingCalls(t *testing.T) {
	loop, invoker, task := newActionExecutionTestLoop(t)
	secondRan := false
	calls := []LoopCall{
		actionExecutionTestCall(0, "call_fail", "accept", func(_ *ReActLoop, _ *aicommon.Action, op *LoopActionHandlerOperator) {
			op.Fail("action failed")
		}),
		actionExecutionTestCall(1, "call_skip", "inspect", func(_ *ReActLoop, _ *aicommon.Action, _ *LoopActionHandlerOperator) {
			secondRan = true
		}),
	}
	require.NoError(t, loop.appendFunctionCallActionResponse(calls, actionExecutionTestDescriptor()))
	result := loop.execCalls(calls, 1, task, "prompt", utils.NewOnce(), loop.appendFunctionCallActionEvent)
	require.Equal(t, loopActionsError, result.result)
	require.ErrorContains(t, result.err, "action failed")
	require.False(t, secondRan)
	require.Contains(t, strings.Join(invoker.entries, "\n"), `"status":"skipped"`)
	require.Contains(t, strings.Join(invoker.entries, "\n"), `"tool_call_id":"call_skip"`)
	require.Len(t, loop.actionHistory, 1)
}

func TestNativeAdjustTodolistAppliesBeforeAnswerAndCountsOneIteration(t *testing.T) {
	loop, invoker, task := newActionExecutionTestLoop(t)
	loop.functionCallMode = true
	loop.Set(loopVarDirectlyAnswerDeliveredWithoutTodoDelta, true)
	makeCalls := func() []LoopCall {
		adjust := aicommon.NewSimpleAction(nativeAdjustTodolistActionName, aitool.InvokeParams{
			"todo_delta": map[string]any{
				"add":     []any{map[string]any{"id": "followup", "text": "Inspect the next file"}},
				"current": "followup",
			},
		})
		return []LoopCall{
			{Action: adjust, LoopAction: loopAction_AdjustTodolistNative, ToolCallID: "call_todo", Index: 0,
				ArgumentsJSON: `{"todo_delta":{"add":[{"id":"followup","text":"Inspect the next file"}],"current":"followup"}}`},
			actionExecutionTestCall(1, "call_answer", "directly_answer", func(l *ReActLoop, a *aicommon.Action, op *LoopActionHandlerOperator) {
				require.True(t, directlyAnswerHasTodoDelta(l, a))
				op.Continue()
			}),
		}
	}
	calls := makeCalls()
	require.NoError(t, loop.appendFunctionCallActionResponse(calls, actionExecutionTestDescriptor()))
	result := loop.execCalls(calls, 1, task, "prompt", utils.NewOnce(), loop.appendFunctionCallActionEvent)
	require.NoError(t, result.err)
	require.Equal(t, loopActionsContinue, result.result)
	require.Equal(t, 1, loop.effectiveIterationCount)
	require.Nil(t, loop.GetVariable(loopVarNativeTodoBatchAdjusted))
	open := loop.config.ActiveVerificationTodoItemsByScope(aicommon.BuildVerificationTodoScope(task))
	require.Len(t, open, 1)
	require.Equal(t, "followup", open[0].ID)
	require.Contains(t, strings.Join(invoker.entries, "\n"), "TODO_DELTA")

	// Repeating the same adjustment is a no-op. It must not authorize a second
	// unchanged answer or consume another effective iteration.
	answerRan := false
	repeated := makeCalls()
	repeated[1].LoopAction.ActionHandler = func(_ *ReActLoop, _ *aicommon.Action, _ *LoopActionHandlerOperator) { answerRan = true }
	result = loop.execCalls(repeated, 2, task, "prompt", utils.NewOnce(), loop.appendFunctionCallActionEvent)
	require.NoError(t, result.err)
	require.True(t, result.skipPostIteration)
	require.False(t, answerRan)
	require.Equal(t, 1, loop.effectiveIterationCount)
}

func TestNativeAdjustTodolistStandaloneFocusAndMixedBatch(t *testing.T) {
	loop, invoker, task := newActionExecutionTestLoop(t)
	loop.functionCallMode = true
	adjust := func(delta map[string]any, id string) LoopCall {
		return LoopCall{
			Action:     aicommon.NewSimpleAction(nativeAdjustTodolistActionName, aitool.InvokeParams{"todo_delta": delta}),
			LoopAction: loopAction_AdjustTodolistNative, ToolCallID: id,
		}
	}
	result := loop.execCalls([]LoopCall{adjust(map[string]any{}, "todo_noop")}, 0, task, "prompt", utils.NewOnce(), nil)
	require.NoError(t, result.err)
	require.Equal(t, 0, loop.effectiveIterationCount, "an empty maintenance call must not consume the work budget")
	initial := adjust(map[string]any{
		"add": []any{
			map[string]any{"id": "inspect", "text": "Inspect the source"},
			map[string]any{"id": "verify", "text": "Verify the result"},
		},
		"current": "inspect",
	}, "todo_initial")
	result = loop.execCalls([]LoopCall{initial}, 1, task, "prompt", utils.NewOnce(), nil)
	require.NoError(t, result.err)
	_, current, _ := loop.config.SnapshotCanonicalTodos(aicommon.BuildVerificationTodoScope(task))
	require.Equal(t, "inspect", current)
	require.Equal(t, 1, loop.effectiveIterationCount)

	var order []string
	business := actionExecutionTestCall(0, "business", "inspect", func(_ *ReActLoop, _ *aicommon.Action, op *LoopActionHandlerOperator) {
		order = append(order, "business")
		op.Continue()
	})
	focus := adjust(map[string]any{"current": "verify"}, "todo_focus")
	focus.Index = 1
	result = loop.execCalls([]LoopCall{business, focus}, 2, task, "prompt", utils.NewOnce(), nil)
	require.NoError(t, result.err)
	require.Equal(t, []string{"business"}, order)
	_, current, _ = loop.config.SnapshotCanonicalTodos(aicommon.BuildVerificationTodoScope(task))
	require.Equal(t, "verify", current)
	require.Equal(t, 2, loop.effectiveIterationCount, "one batch consumes at most one effective iteration")

	invalid := adjust(map[string]any{"current": "unknown"}, "todo_invalid")
	result = loop.execCalls([]LoopCall{invalid, business}, 3, task, "prompt", utils.NewOnce(), nil)
	require.NoError(t, result.err)
	require.Equal(t, []string{"business", "business"}, order, "invalid TODO maintenance must not suppress another valid tool")
	_, current, _ = loop.config.SnapshotCanonicalTodos(aicommon.BuildVerificationTodoScope(task))
	require.Equal(t, "verify", current)
	require.Equal(t, 2, loop.effectiveIterationCount)
	require.Contains(t, strings.Join(invoker.entries, "\n"), "TODO_DELTA_ERROR")

	loop.Set(loopVarDirectlyAnswerDeliveredWithoutTodoDelta, true)
	answerRan := false
	answer := actionExecutionTestCall(0, "answer", "directly_answer", func(l *ReActLoop, a *aicommon.Action, op *LoopActionHandlerOperator) {
		answerRan = true
		require.True(t, directlyAnswerHasTodoDelta(l, a), "a later adjustment keeps this batch active")
		op.Continue()
	})
	lateFocus := adjust(map[string]any{"current": "inspect"}, "todo_after_answer")
	lateFocus.Index = 1
	result = loop.execCalls([]LoopCall{answer, lateFocus}, 4, task, "prompt", utils.NewOnce(), nil)
	require.NoError(t, result.err)
	require.True(t, answerRan)
	_, current, _ = loop.config.SnapshotCanonicalTodos(aicommon.BuildVerificationTodoScope(task))
	require.Equal(t, "inspect", current)
	require.Equal(t, 3, loop.effectiveIterationCount)

	addLater := adjust(map[string]any{"add": []any{map[string]any{"id": "followup", "text": "Inspect the follow-up"}}}, "todo_add")
	focusLater := adjust(map[string]any{"current": "followup"}, "todo_focus_new")
	result = loop.execCalls([]LoopCall{addLater, focusLater}, 5, task, "prompt", utils.NewOnce(), nil)
	require.NoError(t, result.err)
	_, current, _ = loop.config.SnapshotCanonicalTodos(aicommon.BuildVerificationTodoScope(task))
	require.Equal(t, "followup", current)
	require.Equal(t, 4, loop.effectiveIterationCount)

	loop.Delete(loopVarDirectlyAnswerDeliveredWithoutTodoDelta)
	failedLateFocus := adjust(map[string]any{"current": "unknown"}, "todo_failed_after_answer")
	result = loop.execCalls([]LoopCall{answer, failedLateFocus}, 6, task, "prompt", utils.NewOnce(), nil)
	require.NoError(t, result.err)
	require.Equal(t, true, loop.GetVariable(loopVarDirectlyAnswerDeliveredWithoutTodoDelta),
		"a failed later adjustment must not allow unlimited repeated answers")
}

func TestFunctionCallActionResponseRejectsUnprojectableCallBeforeRunning(t *testing.T) {
	loop, invoker, _ := newActionExecutionTestLoop(t)
	ran := false
	call := actionExecutionTestCall(0, "invalid/id", "accept", func(_ *ReActLoop, _ *aicommon.Action, _ *LoopActionHandlerOperator) {
		ran = true
	})
	err := loop.appendFunctionCallActionResponse([]LoopCall{call}, actionExecutionTestDescriptor())
	require.ErrorContains(t, err, "invalid native action call for replay")
	require.False(t, ran)
	require.Empty(t, invoker.entries)
}

func TestExecCallsNormalModeUsesSameSerialExecutor(t *testing.T) {
	for _, count := range []int{1, 2} {
		t.Run(fmt.Sprintf("%d_calls", count), func(t *testing.T) {
			loop, invoker, task := newActionExecutionTestLoop(t)
			var order []int
			var singleOperator *LoopActionHandlerOperator
			calls := make([]LoopCall, 0, count)
			for index := 0; index < count; index++ {
				position := index
				calls = append(calls, actionExecutionTestCall(index, "", "accept", func(_ *ReActLoop, _ *aicommon.Action, op *LoopActionHandlerOperator) {
					order = append(order, position)
					if count == 1 {
						singleOperator = op
					}
					invoker.AddToTimeline("ACTION_OUTPUT", fmt.Sprintf("result %d", position))
					op.Feedback(fmt.Sprintf("feedback %d", position))
					op.NextAction(fmt.Sprintf("next_%d", position))
					op.Continue()
				}))
			}
			result := loop.execCalls(calls, 1, task, "prompt", utils.NewOnce(), nil)
			require.NoError(t, result.err)
			require.Equal(t, loopActionsContinue, result.result)
			require.Len(t, loop.actionHistory, count)
			require.Len(t, order, count)
			require.Len(t, result.operator.GetNextActionMustUse(), count)
			if count == 1 {
				require.Same(t, singleOperator, result.operator)
			}
			for index := 0; index < count; index++ {
				require.Equal(t, index, order[index])
				require.Contains(t, result.operator.GetFeedback().String(), fmt.Sprintf("feedback %d", index))
			}
			require.NotContains(t, strings.Join(invoker.entries, "\n"), functionCallActionResponseTimelineEntry)
			require.NotContains(t, strings.Join(invoker.entries, "\n"), "FUNCTION_CALL_ACTION_RESULT")
		})
	}
}

func TestExecCallsStopsNormalBatchAfterExit(t *testing.T) {
	loop, invoker, task := newActionExecutionTestLoop(t)
	secondRan := false
	calls := []LoopCall{
		actionExecutionTestCall(0, "", "accept", func(_ *ReActLoop, _ *aicommon.Action, op *LoopActionHandlerOperator) {
			op.Exit()
		}),
		actionExecutionTestCall(1, "", "inspect", func(_ *ReActLoop, _ *aicommon.Action, _ *LoopActionHandlerOperator) {
			secondRan = true
		}),
	}
	result := loop.execCalls(calls, 1, task, "prompt", utils.NewOnce(), nil)
	require.NoError(t, result.err)
	require.Equal(t, loopActionsExit, result.result)
	require.False(t, secondRan)
	require.Len(t, loop.actionHistory, 1)
	require.NotContains(t, strings.Join(invoker.entries, "\n"), "FUNCTION_CALL_ACTION_RESULT")
}
