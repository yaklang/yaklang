package loopinfra

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/schema"
)

type directlyCallTestInvoker struct {
	*testInvoker
	tool                  *aitool.Tool
	withoutRequiredName   string
	withoutRequiredParams aitool.InvokeParams
}

func (t *directlyCallTestInvoker) ExecuteToolRequiredAndCallWithoutRequired(ctx context.Context, toolName string, params aitool.InvokeParams, opt ...aicommon.ToolCallerOption) (*aitool.ToolResult, bool, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.withoutRequiredName = toolName
	t.withoutRequiredParams = make(aitool.InvokeParams)
	for key, value := range params {
		t.withoutRequiredParams[key] = value
	}
	return t.toolCallResult, t.toolCallDirectly, t.toolCallErr
}

// DirectlyCallTool mirrors aicommon.ToolCaller.DirectlyCallTool for tests: it
// runs the loop-layer prepare callback (which normalizes/validates params and
// emits progress), records the finalized preset params, and on fallbackToRequire
// delegates to the require path (ExecuteToolRequiredAndCall) — reusing the same
// card semantics as the real implementation.
func (t *directlyCallTestInvoker) DirectlyCallTool(ctx context.Context, toolName string, action *aicommon.Action, prepare aicommon.DirectlyCallPrepareFunc) (*aitool.ToolResult, bool, error) {
	if prepare != nil && t.tool != nil {
		params, fallback, _, err := prepare(action, toolName)
		if err != nil {
			return nil, false, err
		}
		if fallback {
			return t.ExecuteToolRequiredAndCall(ctx, toolName)
		}
		t.mu.Lock()
		t.withoutRequiredName = toolName
		t.withoutRequiredParams = make(aitool.InvokeParams)
		for k, v := range params {
			t.withoutRequiredParams[k] = v
		}
		t.mu.Unlock()
	}
	return t.toolCallResult, t.toolCallDirectly, t.toolCallErr
}

func buildDirectlyCallAction(payload string) *aicommon.Action {
	action, err := aicommon.ExtractAction(payload, schema.AI_REACT_LOOP_ACTION_DIRECTLY_CALL_TOOL)
	if err != nil {
		panic(err)
	}
	return action
}

func TestFormatDirectlyCallToolParamsTimeline(t *testing.T) {
	params := aitool.InvokeParams{
		"title":   "test title",
		"payload": "line1\nline2\nline3",
		"target":  "https://example.com",
	}
	got := formatDirectlyCallToolParamsTimeline("cybersecurity-risk", params, nil)
	assert.Contains(t, got, "直接调用工具cybersecurity-risk生成的参数为：")
	assert.Contains(t, got, "[title]: test title")
	assert.Contains(t, got, "[target]: https://example.com")
	assert.Contains(t, got, "[payload]:\nline1\nline2\nline3")

	blockGot := formatDirectlyCallToolParamsTimeline("bash", aitool.InvokeParams{
		"command": "#!/bin/bash\necho hello",
		"timeout": 20,
	}, []string{"command"})
	assert.Contains(t, blockGot, "[command(BLOCK)]:\n#!/bin/bash\necho hello")
	assert.Contains(t, blockGot, "[timeout]: 20")
}

func TestNormalizeDirectlyCallToolParams_LegacyWrappedString(t *testing.T) {
	params, notes := normalizeDirectlyCallToolParams(`{"@action":"call-tool","tool":"sleep_test","params":{"seconds":0.1,"__DEFAULT__":"ignore"}}`, nil)
	require.Len(t, params, 1)
	assert.Equal(t, 0.1, params.GetFloat("seconds"))
	assert.Contains(t, strings.Join(notes, "\n"), "unwrapped legacy params wrapper")
}

func TestNormalizeDirectlyCallToolParams_NestedWrapperObject(t *testing.T) {
	params, notes := normalizeDirectlyCallToolParams("", aitool.InvokeParams{
		"next_action": aitool.InvokeParams{
			"type": schema.AI_REACT_LOOP_ACTION_DIRECTLY_CALL_TOOL,
			"directly_call_tool_params": aitool.InvokeParams{
				"@action": "call-tool",
				"tool":    "read_file",
				"params": aitool.InvokeParams{
					"path": "/tmp/demo.txt",
				},
			},
		},
	})
	require.Len(t, params, 1)
	assert.Equal(t, "/tmp/demo.txt", params.GetString("path"))
	assert.Contains(t, strings.Join(notes, "\n"), "unwrapped next_action wrapper")
}

func TestDirectlyCallTool_Handler_NormalizesWrappedParamsAndStreamsProgress(t *testing.T) {
	ctx := context.Background()
	testTool := mustNewTool(
		"sleep_test",
		aitool.WithNumberParam("seconds"),
		aitool.WithSimpleCallback(func(params aitool.InvokeParams, stdout io.Writer, stderr io.Writer) (any, error) {
			return "ok", nil
		}),
	)
	cfg := &aicommon.Config{AiToolManager: newToolManagerWithTool(testTool)}
	cfg.GetAiToolManager().AddRecentlyUsedTool(testTool)

	invoker := &directlyCallTestInvoker{testInvoker: newTestInvoker(ctx)}
	invoker.tool = testTool
	invoker.toolCallResult = &aitool.ToolResult{Success: true, Data: "ok"}
	invoker.verifySatisfactionResult = aicommon.NewVerifySatisfactionResult(true, "done", "")

	task := newTestTask(ctx)
	invoker.currentTask = task

	loop := reactloops.NewMinimalReActLoop(cfg, invoker)
	loop.SetCurrentTask(task)

	action := buildDirectlyCallAction(`{
		"@action": "directly_call_tool",
		"directly_call_tool_name": "sleep_test",
		"directly_call_identifier": "sleep_briefly",
		"directly_call_expectations": "~0.1s, instant",
		"directly_call_tool_params": {
			"@action": "call-tool",
			"tool": "sleep_test",
			"params": {
				"seconds": 0.1
			}
		}
	}`)

	require.NoError(t, loopAction_directlyCallTool.ActionVerifier(loop, action))
	op := reactloops.NewActionHandlerOperator(task)
	loopAction_directlyCallTool.ActionHandler(loop, action, op)

	assert.Equal(t, "sleep_test", invoker.withoutRequiredName)
	assert.Equal(t, 0.1, invoker.withoutRequiredParams.GetFloat("seconds"))
	assert.Equal(t, "sleep_briefly", invoker.withoutRequiredParams.GetString(aicommon.ReservedKeyIdentifier))
	assert.Equal(t, "~0.1s, instant", invoker.withoutRequiredParams.GetString(aicommon.ReservedKeyCallExpectations))
	assert.Contains(t, op.GetFeedback().String(), "Prepared directly_call_tool params for 'sleep_test': 1 fields [seconds]")

	timeline := invoker.getTimelineString()
	assert.Contains(t, timeline, "直接调用工具sleep_test生成的参数为：")
	assert.Contains(t, timeline, "[seconds]: 0.1")
	assert.NotContains(t, timeline, "preparing directly_call_tool params")
	assert.NotContains(t, timeline, "normalized 1 param fields")
	assert.NotContains(t, timeline, "calling cached tool")

}

func TestDirectlyCallTool_Handler_MergesAITagParams(t *testing.T) {
	ctx := context.Background()
	testTool := mustNewTool(
		"bash_test",
		aitool.WithStringParam("command"),
		aitool.WithNumberParam("timeout"),
		aitool.WithSimpleCallback(func(params aitool.InvokeParams, stdout io.Writer, stderr io.Writer) (any, error) {
			return "ok", nil
		}),
	)
	cfg := &aicommon.Config{AiToolManager: newToolManagerWithTool(testTool)}
	cfg.GetAiToolManager().AddRecentlyUsedTool(testTool)

	invoker := &directlyCallTestInvoker{testInvoker: newTestInvoker(ctx)}
	invoker.tool = testTool
	invoker.toolCallResult = &aitool.ToolResult{Success: true, Data: "ok"}
	invoker.verifySatisfactionResult = aicommon.NewVerifySatisfactionResult(true, "done", "")

	task := newTestTask(ctx)
	invoker.currentTask = task

	loop := reactloops.NewMinimalReActLoop(cfg, invoker)
	loop.SetCurrentTask(task)

	action := buildDirectlyCallAction(`{
		"@action": "directly_call_tool",
		"directly_call_tool_name": "bash_test",
		"directly_call_tool_params": "{\"timeout\":20}"
	}`)
	action.ForceSet(aicommon.GetToolParamAITagActionKey("command"), "#!/bin/bash\necho hello")

	require.NoError(t, loopAction_directlyCallTool.ActionVerifier(loop, action))
	op := reactloops.NewActionHandlerOperator(task)
	loopAction_directlyCallTool.ActionHandler(loop, action, op)

	assert.Equal(t, "bash_test", invoker.withoutRequiredName)
	assert.Equal(t, 20.0, invoker.withoutRequiredParams.GetFloat("timeout"))
	assert.Equal(t, "#!/bin/bash\necho hello", invoker.withoutRequiredParams.GetString("command"))
	assert.Contains(t, op.GetFeedback().String(), "Prepared directly_call_tool params for 'bash_test': 2 fields [command(BLOCK), timeout]")
	timeline := invoker.getTimelineString()
	assert.Contains(t, timeline, "直接调用工具bash_test生成的参数为：")
	assert.Contains(t, timeline, "[command(BLOCK)]:\n#!/bin/bash\necho hello")
	assert.Contains(t, timeline, "[timeout]: 20")
	assert.NotContains(t, timeline, "merged 1 AITAG block params")
}

func TestDirectlyCallTool_Verifier_AllowsEmptyParamsForParamlessTool(t *testing.T) {
	ctx := context.Background()
	testTool := mustNewTool(
		"ping_test",
		aitool.WithSimpleCallback(func(params aitool.InvokeParams, stdout io.Writer, stderr io.Writer) (any, error) {
			return "ok", nil
		}),
	)
	cfg := &aicommon.Config{AiToolManager: newToolManagerWithTool(testTool)}
	cfg.GetAiToolManager().AddRecentlyUsedTool(testTool)

	invoker := &directlyCallTestInvoker{testInvoker: newTestInvoker(ctx)}
	invoker.tool = testTool
	invoker.toolCallResult = &aitool.ToolResult{Success: true, Data: "ok"}
	invoker.verifySatisfactionResult = aicommon.NewVerifySatisfactionResult(true, "done", "")

	task := newTestTask(ctx)
	invoker.currentTask = task

	loop := reactloops.NewMinimalReActLoop(cfg, invoker)
	loop.SetCurrentTask(task)

	action := buildDirectlyCallAction(`{
		"@action": "directly_call_tool",
		"directly_call_tool_name": "ping_test"
	}`)

	require.NoError(t, loopAction_directlyCallTool.ActionVerifier(loop, action))
	op := reactloops.NewActionHandlerOperator(task)
	loopAction_directlyCallTool.ActionHandler(loop, action, op)

	assert.Equal(t, "ping_test", invoker.withoutRequiredName)
	assert.Empty(t, invoker.withoutRequiredParams)
	assert.NotContains(t, invoker.getTimelineString(), "params validation failed")
}

func TestDirectlyCallTool_Handler_RequiredParamMismatchAddsLatestFewShot(t *testing.T) {
	ctx := context.Background()
	testTool := mustNewTool(
		"sleep_test",
		aitool.WithNumberParam("seconds", aitool.WithParam_Required(true)),
		aitool.WithSimpleCallback(func(params aitool.InvokeParams, stdout io.Writer, stderr io.Writer) (any, error) {
			return "ok", nil
		}),
	)
	cfg := &aicommon.Config{AiToolManager: newToolManagerWithTool(testTool)}
	cfg.GetAiToolManager().AddRecentlyUsedTool(testTool)

	invoker := &directlyCallTestInvoker{testInvoker: newTestInvoker(ctx)}
	invoker.tool = testTool
	invoker.toolCallResult = &aitool.ToolResult{Success: true, Data: "ok"}
	invoker.verifySatisfactionResult = aicommon.NewVerifySatisfactionResult(true, "done", "")

	task := newTestTask(ctx)
	invoker.currentTask = task

	loop := reactloops.NewMinimalReActLoop(cfg, invoker)
	loop.SetCurrentTask(task)

	action := buildDirectlyCallAction(`{
		"@action": "directly_call_tool",
		"directly_call_tool_name": "sleep_test",
		"directly_call_tool_params": {}
	}`)

	require.NoError(t, loopAction_directlyCallTool.ActionVerifier(loop, action))
	op := reactloops.NewActionHandlerOperator(task)
	loopAction_directlyCallTool.ActionHandler(loop, action, op)

	assert.Empty(t, invoker.withoutRequiredName)
	assert.True(t, invoker.toolCallCalled)
	assert.Equal(t, "sleep_test", invoker.toolCallName)
	timeline := invoker.getTimelineString()
	assert.Contains(t, timeline, "params validation failed for cached tool 'sleep_test'")
	assert.Contains(t, timeline, "auto fallback: switching 'sleep_test' from directly_call_tool to @action=require_tool because schema validation failed")
	assert.Contains(t, timeline, `{"@action":"require_tool","tool_require_payload":"sleep_test"}`)
	assert.Contains(t, timeline, `{"@action":"directly_call_tool","directly_call_tool_name":"sleep_test"`)
	assert.NotContains(t, timeline, `"next_action"`)
	assert.Contains(t, op.GetFeedback().String(), "automatically switching to @action=require_tool")
}
