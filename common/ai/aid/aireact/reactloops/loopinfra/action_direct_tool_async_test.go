package loopinfra

import (
	"context"
	"encoding/json"
	"io"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/asyncexecmanager"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/schema"
)

// Observe the real verifier/handler boundary while keeping tool execution local.
// Record either synchronous invocation or asynchronous resource preparation.
type asyncActionTestInvoker struct {
	*directlyCallTestInvoker
	loop         *reactloops.ReActLoop
	action       *aicommon.Action
	scalarStates []any
	groups       []*aicommon.ToolCallGroupRequest
}

func (i *asyncActionTestInvoker) DirectlyCallTool(ctx context.Context, name string, action *aicommon.Action, prepare aicommon.DirectlyCallPrepareFunc) (*aitool.ToolResult, bool, error) {
	return i.directlyCallTestInvoker.DirectlyCallTool(ctx, name, action, func(a *aicommon.Action, name string) (aitool.InvokeParams, bool, *aitool.Tool, error) {
		params, fallback, tool, err := prepare(a, name)
		if err == nil && !fallback {
			i.scalarStates = append(i.scalarStates, i.loop.GetActionExecutionValue(a, actionStateDirectToolAsync))
		}
		return params, fallback, tool, err
	})
}

func (i *asyncActionTestInvoker) ExecuteToolRequiredAndCallWithoutRequired(ctx context.Context, name string, params aitool.InvokeParams, options ...aicommon.ToolCallerOption) (*aitool.ToolResult, bool, error) {
	i.scalarStates = append(i.scalarStates, i.loop.GetActionExecutionValue(i.action, actionStateDirectToolAsync))
	return i.directlyCallTestInvoker.ExecuteToolRequiredAndCallWithoutRequired(ctx, name, params, options...)
}

func (i *asyncActionTestInvoker) PrepareAsyncToolCall(task aicommon.AIStatefulTask, name string, params aitool.InvokeParams, _ ...aicommon.ToolCallerOption) (asyncexecmanager.Resource, func(), error) {
	i.scalarStates = append(i.scalarStates, i.loop.GetActionExecutionValue(i.action, actionStateDirectToolAsync))
	i.withoutRequiredName, i.withoutRequiredParams = name, params
	resource, err := asyncexecmanager.NewResource(func(context.Context) error { return nil },
		func(context.Context) (asyncexecmanager.Observation, error) {
			return asyncexecmanager.Observation{}, nil
		})
	return resource, func() {}, err
}

func (i *asyncActionTestInvoker) ExecuteToolCallGroup(ctx context.Context, task aicommon.AIStatefulTask, request *aicommon.ToolCallGroupRequest) (*aicommon.ToolCallGroupResult, error) {
	i.groups = append(i.groups, request)
	result := &aicommon.ToolCallGroupResult{}
	for _, call := range request.Calls {
		result.Outcomes = append(result.Outcomes, aicommon.ToolCallOutcome{
			Index: call.Index, RequestedTool: call.ToolName, FinalTool: call.ToolName,
			Stage: aicommon.ToolCallStageDone, Result: i.toolCallResult,
		})
	}
	return result, nil
}

func newAsyncActionTestLoop(t *testing.T) (*reactloops.ReActLoop, *asyncActionTestInvoker, aicommon.AIStatefulTask) {
	t.Helper()
	ctx := context.Background()
	tool := mustNewTool("async_fixture",
		aitool.WithStringParam("file", aitool.WithParam_Required(true)),
		// These business parameters must never become scheduling flags.
		aitool.WithBoolParam("async"),
		aitool.WithBoolParam("async_directly_call_tool"),
		aitool.WithRawParam("payload", map[string]any{"type": "object", "additionalProperties": true}),
		aitool.WithSimpleCallback(func(aitool.InvokeParams, io.Writer, io.Writer) (any, error) { return "ok", nil }),
	)
	manager := newToolManagerWithTool(tool)
	manager.AddRecentlyUsedTool(tool)
	base := newTestInvoker(ctx)
	base.toolCallResult = &aitool.ToolResult{Name: "async_fixture", Success: true, Data: "ok"}
	base.verifySatisfactionResult = aicommon.NewVerifySatisfactionResult(true, "done", "")
	task := newTestTask(ctx)
	base.currentTask = task
	invoker := &asyncActionTestInvoker{directlyCallTestInvoker: &directlyCallTestInvoker{testInvoker: base, tool: tool}}
	loop := reactloops.NewMinimalReActLoop(&aicommon.Config{AiToolManager: manager}, invoker)
	loop.SetCurrentTask(task)
	invoker.loop = loop
	t.Cleanup(loop.Release)
	return loop, invoker, task
}

func asyncActionPayload(t *testing.T, fields map[string]any, wrapped bool) string {
	t.Helper()
	payload := map[string]any{"@action": schema.AI_REACT_LOOP_ACTION_DIRECTLY_CALL_TOOL}
	if wrapped {
		payload["next_action"] = fields
	} else {
		for key, value := range fields {
			payload[key] = value
		}
	}
	raw, err := json.Marshal(payload)
	require.NoError(t, err)
	return string(raw)
}

var asyncActionInputs = []struct {
	name  string
	value any
	want  bool
}{
	{"omitted", nil, false},
	{"bool true", true, true}, {"bool false", false, false},
	{"string true", "true", true}, {"string false", "false", false},
	{"padded uppercase", " TRUE ", true},
	{"string one", "1", true}, {"string zero", "0", false},
	{"yes", "yes", true}, {"on", "on", true}, {"no", "no", false},
	{"off", "off", false}, {"padded false", " FALSE ", false},
	{"number one", 1, true}, {"number zero", 0, false},
	{"null", nil, false}, {"empty", "", false},
	{"unrecognized string", "maybe", false}, {"other number", 2, false},
	{"object", map[string]any{"enabled": true}, false}, {"array", []any{true}, false},
}

func TestDirectToolAsyncScalarActionInputs(t *testing.T) {
	for _, handler := range []*reactloops.LoopAction{loopAction_directlyCallTool, nativeDirectToolAction} {
		mode := "legacy"
		if handler == nativeDirectToolAction {
			mode = "native"
		}
		for _, wrapped := range []bool{false, true} {
			layout := "root"
			if wrapped {
				layout = "next_action"
			}
			for _, input := range asyncActionInputs {
				t.Run(mode+"/"+layout+"/"+input.name, func(t *testing.T) {
					loop, invoker, task := newAsyncActionTestLoop(t)
					params := map[string]any{"file": "/scalar", "async": true, "async_directly_call_tool": true}
					fields := map[string]any{"directly_call_tool_name": "async_fixture", "directly_call_tool_params": params}
					if input.name != "omitted" {
						fields[actionStateDirectToolAsync] = input.value
					}
					action := parseToolBatchPromptExample(t, asyncActionPayload(t, fields, wrapped), schema.AI_REACT_LOOP_ACTION_DIRECTLY_CALL_TOOL)
					invoker.action = action
					require.NoError(t, handler.ActionVerifier(loop, action))
					op := reactloops.NewActionHandlerOperator(task)
					handler.ActionHandler(loop, action, op)
					require.Equal(t, []any{input.want}, invoker.scalarStates, "normalized hint must reach the runtime boundary once")
					require.Empty(t, invoker.groups)
					require.Equal(t, "async_fixture", invoker.withoutRequiredName)
					require.Equal(t, aitool.InvokeParams(params), invoker.withoutRequiredParams, "framework flag must not change business parameters")
					require.True(t, op.IsContinued())
				})
			}
		}
	}
}

func TestDirectToolAsyncBatchActionInputs(t *testing.T) {
	for _, handler := range []*reactloops.LoopAction{loopAction_directlyCallTool, nativeDirectToolAction} {
		mode := "legacy"
		if handler == nativeDirectToolAction {
			mode = "native"
		}
		for _, wrapped := range []bool{false, true} {
			layout := "root"
			if wrapped {
				layout = "next_action"
			}
			for _, input := range asyncActionInputs {
				t.Run(mode+"/"+layout+"/"+input.name, func(t *testing.T) {
					loop, invoker, task := newAsyncActionTestLoop(t)
					first := map[string]any{"tool_name": "async_fixture", "params": map[string]any{"file": "/first", "async": true}, "identifier": "first"}
					if input.name != "omitted" {
						first["async"] = input.value
					}
					// A top-level hint and nested business flags must not affect the
					// second item's omitted flag, regardless of first-item conversion.
					fields := map[string]any{
						actionStateDirectToolAsync: true,
						directlyCallToolBatchField: []any{first, map[string]any{
							"tool_name": "async_fixture", "params": map[string]any{"file": "/second", "async_directly_call_tool": true}, "identifier": "second",
						}},
					}
					action := parseToolBatchPromptExample(t, asyncActionPayload(t, fields, wrapped), schema.AI_REACT_LOOP_ACTION_DIRECTLY_CALL_TOOL)
					require.NoError(t, handler.ActionVerifier(loop, action))
					verified := loop.GetActionExecutionValue(action, actionStateDirectToolBatch).(*aicommon.ToolCallGroupRequest)
					require.Equal(t, input.want, verified.Calls[0].Async)
					require.False(t, verified.Calls[1].Async)
					op := reactloops.NewActionHandlerOperator(task)
					handler.ActionHandler(loop, action, op)
					require.Len(t, invoker.groups, 1)
					require.Same(t, verified, invoker.groups[0], "handler must forward its verified request")
					require.Equal(t, input.want, invoker.groups[0].Calls[0].Async)
					require.False(t, invoker.groups[0].Calls[1].Async)
					require.Equal(t, "first", verified.Calls[0].Identifier)
					require.Equal(t, "second", verified.Calls[1].Identifier)
					require.Equal(t, "/first", verified.Calls[0].Params.GetString("file"))
					require.Equal(t, "/second", verified.Calls[1].Params.GetString("file"))
					require.Empty(t, invoker.scalarStates)
					require.True(t, op.IsContinued())
					require.Equal(t, 2, op.GetExecutedToolCallCount())
				})
			}
		}
	}
}

func TestDirectToolAsyncActionStateIsolation(t *testing.T) {
	for _, handler := range []*reactloops.LoopAction{loopAction_directlyCallTool, nativeDirectToolAction} {
		mode := "legacy"
		if handler == nativeDirectToolAction {
			mode = "native"
		}
		t.Run(mode, func(t *testing.T) {
			loop, invoker, task := newAsyncActionTestLoop(t)
			secondParams := any(map[string]any{"file": "/second"})
			if handler == loopAction_directlyCallTool {
				secondParams = `{"file":"/second"}` // Existing scalar JSON-string compatibility.
			}
			payloads := []map[string]any{
				{"directly_call_tool_name": "async_fixture", "directly_call_tool_params": map[string]any{"file": "/first"}, actionStateDirectToolAsync: "true"},
				{"directly_call_tool_name": "async_fixture", "directly_call_tool_params": secondParams},
				{directlyCallToolBatchField: []any{
					map[string]any{"tool_name": "async_fixture", "params": map[string]any{"file": "/batch-first"}, "async": false},
					map[string]any{"tool_name": "async_fixture", "params": map[string]any{"file": "/batch-second"}, "async": "yes"},
				}},
			}
			var actions []*aicommon.Action
			// Native responses may verify every action before executing any of them.
			for _, fields := range payloads {
				action := parseToolBatchPromptExample(t, asyncActionPayload(t, fields, false), schema.AI_REACT_LOOP_ACTION_DIRECTLY_CALL_TOOL)
				require.NoError(t, handler.ActionVerifier(loop, action))
				actions = append(actions, action)
			}
			for _, action := range actions {
				invoker.action = action
				handler.ActionHandler(loop, action, reactloops.NewActionHandlerOperator(task))
			}
			require.Equal(t, []any{true, false}, invoker.scalarStates)
			require.Equal(t, "/second", invoker.withoutRequiredParams.GetString("file"))
			require.Len(t, invoker.groups, 1)
			require.False(t, invoker.groups[0].Calls[0].Async)
			require.True(t, invoker.groups[0].Calls[1].Async)
			require.Equal(t, true, loop.GetActionExecutionValue(actions[0], actionStateDirectToolAsync))
			require.Equal(t, false, loop.GetActionExecutionValue(actions[1], actionStateDirectToolAsync))
			require.Nil(t, loop.GetActionExecutionValue(actions[2], actionStateDirectToolAsync))
		})
	}
}

func TestDirectToolAsyncEmptyProtocolKeys(t *testing.T) {
	for _, handler := range []*reactloops.LoopAction{loopAction_directlyCallTool, nativeDirectToolAction} {
		mode := "legacy"
		if handler == nativeDirectToolAction {
			mode = "native"
		}
		for _, wrapped := range []bool{false, true} {
			layout := "root"
			if wrapped {
				layout = "next_action"
			}
			for _, batch := range []bool{false, true} {
				form := "scalar"
				if batch {
					form = "batch"
				}
				t.Run(mode+"/"+layout+"/"+form, func(t *testing.T) {
					loop, invoker, task := newAsyncActionTestLoop(t)
					// Blank-key contents must not be interpreted as protocol fields.
					noise := map[string]any{actionStateDirectToolAsync: true, "async": true}
					business := map[string]any{"": "keep", " ": "also keep", "async": true}
					params := map[string]any{"file": "/first", "payload": business}
					fields := map[string]any{"": noise, " \t ": nil}
					if batch {
						fields[directlyCallToolBatchField] = []any{
							map[string]any{"": noise, " \t ": nil, "tool_name": "async_fixture", "params": params},
							map[string]any{"": "", "\u3000": false, "tool_name": "async_fixture", "params": map[string]any{"file": "/second"}, "async": "true"},
						}
					} else {
						fields["directly_call_tool_name"] = "async_fixture"
						fields["directly_call_tool_params"] = params
					}
					action := parseToolBatchPromptExample(t, asyncActionPayload(t, fields, wrapped), schema.AI_REACT_LOOP_ACTION_DIRECTLY_CALL_TOOL)
					invoker.action = action
					require.NoError(t, handler.ActionVerifier(loop, action))
					op := reactloops.NewActionHandlerOperator(task)
					handler.ActionHandler(loop, action, op)
					if batch {
						require.Len(t, invoker.groups, 1)
						require.False(t, invoker.groups[0].Calls[0].Async)
						require.True(t, invoker.groups[0].Calls[1].Async)
						require.Equal(t, business, map[string]any(invoker.groups[0].Calls[0].Params.GetObject("payload")))
					} else {
						require.Equal(t, []any{false}, invoker.scalarStates)
						require.Equal(t, business, map[string]any(invoker.withoutRequiredParams.GetObject("payload")))
					}
					require.True(t, op.IsContinued())
				})
			}
		}
	}
}

func TestDirectToolLookupIgnoresEmptyKeys(t *testing.T) {
	action := aicommon.NewSimpleAction("directly_call_tool", aitool.InvokeParams{
		"": true, " ": true, "next_action": map[string]any{"": true},
	})
	for _, key := range []string{"", " ", "\t\n", "\u3000"} {
		value, exists := lookupCanonicalActionParam(action, key)
		require.False(t, exists)
		require.Nil(t, value)
	}
	value, exists := lookupCanonicalActionParam(nil, "async")
	require.False(t, exists)
	require.Nil(t, value)
}

func TestDirectToolEmptyKeyCannotReplaceRequiredField(t *testing.T) {
	for _, handler := range []*reactloops.LoopAction{loopAction_directlyCallTool, nativeDirectToolAction} {
		for _, missing := range []string{"tool_name", "params"} {
			loop, invoker, _ := newAsyncActionTestLoop(t)
			first := map[string]any{"tool_name": "async_fixture", "params": map[string]any{"file": "/first"}}
			first[""] = map[string]any{missing: first[missing]}
			delete(first, missing)
			fields := map[string]any{directlyCallToolBatchField: []any{
				first, map[string]any{"tool_name": "async_fixture", "params": map[string]any{"file": "/second"}},
			}}
			action := parseToolBatchPromptExample(t, asyncActionPayload(t, fields, false), schema.AI_REACT_LOOP_ACTION_DIRECTLY_CALL_TOOL)
			require.ErrorContains(t, handler.ActionVerifier(loop, action), missing+" is required")
			require.Nil(t, loop.GetActionExecutionValue(action, actionStateDirectToolBatch))
			require.Empty(t, invoker.groups)
		}
	}
}
