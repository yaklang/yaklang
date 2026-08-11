package loopinfra

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools"
	"github.com/yaklang/yaklang/common/schema"
)

func newToolBatchTestManager(t *testing.T) (*buildinaitools.AiToolManager, *aitool.Tool, *aitool.Tool) {
	t.Helper()
	readFile := mustNewTool(
		"read_file",
		aitool.WithStringParam("path", aitool.WithParam_Required(true)),
		aitool.WithSimpleCallback(func(params aitool.InvokeParams, stdout io.Writer, stderr io.Writer) (any, error) {
			return "content", nil
		}),
	)
	grep := mustNewTool(
		"grep",
		aitool.WithStringParam("path"),
		aitool.WithStringParam("pattern"),
		aitool.WithSimpleCallback(func(params aitool.InvokeParams, stdout io.Writer, stderr io.Writer) (any, error) {
			return "matches", nil
		}),
	)
	tools := []*aitool.Tool{readFile, grep}
	manager := buildinaitools.NewToolManagerByToolGetter(
		func() []*aitool.Tool { return tools },
		buildinaitools.WithExtendTools(tools, true),
	)
	return manager, readFile, grep
}

func newToolBatchTestLoop(t *testing.T) (*reactloops.ReActLoop, *testInvoker) {
	t.Helper()
	ctx := context.Background()
	manager, readFile, _ := newToolBatchTestManager(t)
	manager.AddRecentlyUsedTool(readFile)
	cfg := &aicommon.Config{AiToolManager: manager}
	invoker := newTestInvoker(ctx)
	loop := reactloops.NewMinimalReActLoop(cfg, invoker)
	return loop, invoker
}

func parseToolBatchPromptExample(t *testing.T, raw, actionType string) *aicommon.Action {
	t.Helper()
	action, err := aicommon.ExtractValidActionFromStream(
		context.Background(),
		strings.NewReader(raw),
		actionType,
	)
	require.NoError(t, err)
	return action
}

func newPromptActionSchemaTool(t *testing.T, action *reactloops.LoopAction) *aitool.Tool {
	t.Helper()
	options := []aitool.ToolOption{
		aitool.WithStringParam("@action",
			aitool.WithParam_EnumString(action.ActionType),
			aitool.WithParam_Required(true)),
		aitool.WithStringParam("identifier", aitool.WithParam_Required(true)),
		aitool.WithStringParam("human_readable_thought"),
		aitool.WithSimpleCallback(func(params aitool.InvokeParams, stdout io.Writer, stderr io.Writer) (any, error) {
			return nil, nil
		}),
	}
	options = append(options, action.Options...)
	return mustNewTool("prompt_action_schema", options...)
}

func requirePromptExampleMatchesActionSchema(t *testing.T, raw string, action *reactloops.LoopAction) {
	t.Helper()
	schemaTool := newPromptActionSchemaTool(t, action)
	var payload map[string]any
	require.NoError(t, json.Unmarshal([]byte(raw), &payload))
	valid, validationErrors := schemaTool.ValidateParams(payload)
	require.True(t, valid, "prompt example does not match emitted schema: %s", strings.Join(validationErrors, "; "))
}

func TestDirectToolScalarParamsSchema_AcceptsObjectAndLegacyJSONString(t *testing.T) {
	schemaTool := newPromptActionSchemaTool(t, loopAction_directlyCallTool)
	base := map[string]any{
		"@action":                 schema.AI_REACT_LOOP_ACTION_DIRECTLY_CALL_TOOL,
		"identifier":              "read_project_config",
		"directly_call_tool_name": "read_file",
	}
	for _, test := range []struct {
		name   string
		params any
		valid  bool
	}{
		{name: "preferred object", params: map[string]any{"path": "/workspace/go.mod"}, valid: true},
		{name: "legacy JSON string", params: `{"path":"/workspace/go.mod"}`, valid: true},
		{name: "array remains invalid", params: []any{"/workspace/go.mod"}, valid: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			payload := make(map[string]any, len(base)+1)
			for key, value := range base {
				payload[key] = value
			}
			payload["directly_call_tool_params"] = test.params
			valid, validationErrors := schemaTool.ValidateParams(payload)
			assert.Equal(t, test.valid, valid, strings.Join(validationErrors, "; "))
		})
	}
}

// The scalar and batch strings embedded into the emitted Schema descriptions
// are not documentation-only pseudo-JSON. This test makes CI run all four exact
// payloads through the production streaming parser, verifier and action-schema
// validator. The same strings remain together in OutputExamples for custom
// renderers, with the single-call form taught before the batch form.
func TestToolCallPromptExamples_ParseAndVerifyExactBytes(t *testing.T) {
	t.Run("directly_call_tool scalar", func(t *testing.T) {
		loop, _ := newToolBatchTestLoop(t)
		requirePromptExampleMatchesActionSchema(t, directlyCallToolScalarOutputExampleJSON, loopAction_directlyCallTool)
		action := parseToolBatchPromptExample(t, directlyCallToolScalarOutputExampleJSON, schema.AI_REACT_LOOP_ACTION_DIRECTLY_CALL_TOOL)

		require.NoError(t, loopAction_directlyCallTool.ActionVerifier(loop, action))
		assert.Nil(t, loop.GetVariable(loopVarDirectToolBatch))
		assert.Equal(t, "read_file", loop.Get("directly_call_tool_name"))
		assert.Less(t,
			strings.Index(loopAction_directlyCallTool.OutputExamples, directlyCallToolScalarOutputExampleJSON),
			strings.Index(loopAction_directlyCallTool.OutputExamples, directlyCallToolBatchOutputExampleJSON),
			"the scalar form should be taught before the batch optimization",
		)
		assert.Contains(t, loopAction_directlyCallTool.OutputExamples, directlyCallToolScalarOutputExampleJSON)
		assert.Contains(t, loopAction_directlyCallTool.OutputExamples, directlyCallToolBatchOutputExampleJSON)
	})

	t.Run("directly_call_tool batch", func(t *testing.T) {
		loop, _ := newToolBatchTestLoop(t)
		requirePromptExampleMatchesActionSchema(t, directlyCallToolBatchOutputExampleJSON, loopAction_directlyCallTool)
		action := parseToolBatchPromptExample(t, directlyCallToolBatchOutputExampleJSON, schema.AI_REACT_LOOP_ACTION_DIRECTLY_CALL_TOOL)

		require.NoError(t, loopAction_directlyCallTool.ActionVerifier(loop, action))
		request, ok := loop.GetVariable(loopVarDirectToolBatch).(*aicommon.ToolBatchRequest)
		require.True(t, ok)
		require.Len(t, request.Calls, 2)
		assert.Equal(t, aicommon.ToolCallModeDirect, request.Calls[0].Mode)
		assert.Equal(t, "read_file", request.Calls[0].ToolName)
		assert.Equal(t, "/workspace/go.mod", request.Calls[0].Params.GetString("path"))
		assert.Equal(t, "read_go_mod", request.Calls[0].Identifier)
		assert.Equal(t, "/workspace/README.md", request.Calls[1].Params.GetString("path"))
		assert.Contains(t, loopAction_directlyCallTool.OutputExamples, directlyCallToolBatchOutputExampleJSON)
	})

	t.Run("require_tool scalar", func(t *testing.T) {
		loop, _ := newToolBatchTestLoop(t)
		requirePromptExampleMatchesActionSchema(t, requireToolScalarOutputExampleJSON, loopAction_toolRequireAndCall)
		action := parseToolBatchPromptExample(t, requireToolScalarOutputExampleJSON, schema.AI_REACT_LOOP_ACTION_REQUIRE_TOOL)

		require.NoError(t, loopAction_toolRequireAndCall.ActionVerifier(loop, action))
		assert.Nil(t, loop.GetVariable(loopVarRequireToolBatch))
		assert.Equal(t, "grep", loop.Get("tool_require_payload"))
		assert.Less(t,
			strings.Index(loopAction_toolRequireAndCall.OutputExamples, requireToolScalarOutputExampleJSON),
			strings.Index(loopAction_toolRequireAndCall.OutputExamples, requireToolBatchOutputExampleJSON),
			"the scalar form should be taught before the batch optimization",
		)
		assert.Contains(t, loopAction_toolRequireAndCall.OutputExamples, requireToolScalarOutputExampleJSON)
		assert.Contains(t, loopAction_toolRequireAndCall.OutputExamples, requireToolBatchOutputExampleJSON)
	})

	t.Run("require_tool batch", func(t *testing.T) {
		loop, _ := newToolBatchTestLoop(t)
		requirePromptExampleMatchesActionSchema(t, requireToolBatchOutputExampleJSON, loopAction_toolRequireAndCall)
		action := parseToolBatchPromptExample(t, requireToolBatchOutputExampleJSON, schema.AI_REACT_LOOP_ACTION_REQUIRE_TOOL)

		require.NoError(t, loopAction_toolRequireAndCall.ActionVerifier(loop, action))
		request, ok := loop.GetVariable(loopVarRequireToolBatch).(*aicommon.ToolBatchRequest)
		require.True(t, ok)
		require.Len(t, request.Calls, 2)
		assert.Equal(t, aicommon.ToolCallModeRequire, request.Calls[0].Mode)
		assert.Equal(t, "grep", request.Calls[0].ToolName)
		assert.Nil(t, request.Calls[0].Params)
		assert.Equal(t, "read_file", request.Calls[1].ToolName)
		assert.Contains(t, loopAction_toolRequireAndCall.OutputExamples, requireToolBatchOutputExampleJSON)
	})
}

func TestToolBatchSchema_DeclaresStrictObjectArrays(t *testing.T) {
	tests := []struct {
		name      string
		field     string
		action    *reactloops.LoopAction
		required  []string
		forbidden string
		example   string
	}{
		{
			name:      "direct",
			field:     directlyCallToolBatchField,
			action:    loopAction_directlyCallTool,
			required:  []string{"tool_name", "params"},
			forbidden: "tool_require_calls",
			example:   directlyCallToolBatchOutputExampleJSON,
		},
		{
			name:      "require",
			field:     requireToolBatchField,
			action:    loopAction_toolRequireAndCall,
			required:  []string{"tool_name"},
			forbidden: "params",
			example:   requireToolBatchOutputExampleJSON,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			schemaOptions := make([]any, 0, len(tt.action.Options))
			for _, option := range tt.action.Options {
				schemaOptions = append(schemaOptions, option)
			}
			raw := aitool.NewObjectSchema(schemaOptions...)
			var root map[string]any
			require.NoError(t, json.Unmarshal([]byte(raw), &root))
			properties := root["properties"].(map[string]any)
			arraySchema := properties[tt.field].(map[string]any)
			assert.Equal(t, "array", arraySchema["type"])
			assert.EqualValues(t, 2, arraySchema["minItems"])
			assert.EqualValues(t, aicommon.DefaultToolBatchMaxCalls, arraySchema["maxItems"])
			assert.Contains(t, arraySchema["description"], tt.example,
				"the exact CI-parsed example must be present in the emitted action schema prompt")
			itemSchema := arraySchema["items"].(map[string]any)
			assert.Equal(t, "object", itemSchema["type"])
			assert.Equal(t, false, itemSchema["additionalProperties"])
			requiredRaw := itemSchema["required"].([]any)
			required := make([]string, 0, len(requiredRaw))
			for _, value := range requiredRaw {
				required = append(required, value.(string))
			}
			assert.ElementsMatch(t, tt.required, required)
			itemProperties := itemSchema["properties"].(map[string]any)
			if tt.name == "require" {
				_, hasParams := itemProperties[tt.forbidden]
				assert.False(t, hasParams)
			}
		})
	}
}

func TestToolBatchMaxCalls_ClampsRawConfigToPublishedSchema(t *testing.T) {
	loop, _ := newToolBatchTestLoop(t)
	cfg := loop.GetConfig().(*aicommon.Config)
	cfg.KeyValueConfig = aicommon.NewKeyValueConfig()
	cfg.SetConfig(aicommon.ConfigKeyToolBatchMaxCalls, 99)
	require.Equal(t, aicommon.DefaultToolBatchMaxCalls, toolBatchMaxCalls(loop))

	cfg.SetConfig(aicommon.ConfigKeyToolBatchMaxCalls, 1)
	require.Equal(t, 2, toolBatchMaxCalls(loop))
}

func TestDirectToolBatchVerifier_RejectsAmbiguousOrInvalidBatchBeforeHandler(t *testing.T) {
	tests := []struct {
		name       string
		payload    string
		errContain string
	}{
		{
			name: "scalar_and_batch",
			payload: `{
				"@action":"directly_call_tool",
				"directly_call_tool_name":"read_file",
				"directly_call_tool_params":{"path":"/scalar"},
				"directly_call_tool_calls":[
					{"tool_name":"read_file","params":{"path":"/a"}},
					{"tool_name":"read_file","params":{"path":"/b"}}
				]
			}`,
			errContain: "cannot be combined",
		},
		{
			name:       "not_an_array",
			payload:    `{"@action":"directly_call_tool","directly_call_tool_calls":{"tool_name":"read_file","params":{"path":"/a"}}}`,
			errContain: "array of objects",
		},
		{
			name:       "one_item",
			payload:    `{"@action":"directly_call_tool","directly_call_tool_calls":[{"tool_name":"read_file","params":{"path":"/a"}}]}`,
			errContain: "at least 2",
		},
		{
			name: "second_params_invalid",
			payload: `{
				"@action":"directly_call_tool",
				"directly_call_tool_calls":[
					{"tool_name":"read_file","params":{"path":"/valid"}},
					{"tool_name":"read_file","params":{}}
				]
			}`,
			errContain: "params are invalid",
		},
		{
			name: "duplicate_identifier",
			payload: `{
				"@action":"directly_call_tool",
				"directly_call_tool_calls":[
					{"tool_name":"read_file","params":{"path":"/a"},"identifier":"same"},
					{"tool_name":"read_file","params":{"path":"/b"},"identifier":"same"}
				]
			}`,
			errContain: "duplicates",
		},
		{
			name: "cross_action_batch_is_rejected",
			payload: `{
				"@action":"directly_call_tool",
				"directly_call_tool_calls":[
					{"tool_name":"read_file","params":{"path":"/a"}},
					{"tool_name":"read_file","params":{"path":"/b"}}
				],
				"tool_require_calls":[
					{"tool_name":"grep"},
					{"tool_name":"read_file"}
				]
			}`,
			errContain: "cannot be combined with require_tool fields",
		},
		{
			name: "params_array_is_not_an_object",
			payload: `{
				"@action":"directly_call_tool",
				"directly_call_tool_calls":[
					{"tool_name":"read_file","params":[]},
					{"tool_name":"read_file","params":{"path":"/b"}}
				]
			}`,
			errContain: "params must be a non-null JSON object",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loop, _ := newToolBatchTestLoop(t)
			action := parseToolBatchPromptExample(t, tt.payload, schema.AI_REACT_LOOP_ACTION_DIRECTLY_CALL_TOOL)
			err := loopAction_directlyCallTool.ActionVerifier(loop, action)
			require.ErrorContains(t, err, tt.errContain)
			assert.Nil(t, loop.GetVariable(loopVarDirectToolBatch), "invalid batch must never reach the handler")
		})
	}
}

func TestRequireToolBatchVerifier_RejectsParamsAndMixedForms(t *testing.T) {
	tests := []struct {
		name       string
		payload    string
		errContain string
	}{
		{
			name: "params_are_forbidden",
			payload: `{
				"@action":"require_tool",
				"tool_require_calls":[
					{"tool_name":"grep","params":{"pattern":"auth"}},
					{"tool_name":"read_file"}
				]
			}`,
			errContain: "unknown fields: params",
		},
		{
			name: "scalar_and_batch",
			payload: `{
				"@action":"require_tool",
				"tool_require_payload":"grep",
				"tool_require_calls":[
					{"tool_name":"grep"},
					{"tool_name":"read_file"}
				]
			}`,
			errContain: "cannot be combined",
		},
		{
			name: "cross_action_batch_is_rejected",
			payload: `{
				"@action":"require_tool",
				"tool_require_calls":[
					{"tool_name":"grep"},
					{"tool_name":"read_file"}
				],
				"directly_call_tool_calls":[
					{"tool_name":"read_file","params":{"path":"/a"}},
					{"tool_name":"read_file","params":{"path":"/b"}}
				]
			}`,
			errContain: "cannot be combined with directly_call_tool fields",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loop, _ := newToolBatchTestLoop(t)
			action := parseToolBatchPromptExample(t, tt.payload, schema.AI_REACT_LOOP_ACTION_REQUIRE_TOOL)
			err := loopAction_toolRequireAndCall.ActionVerifier(loop, action)
			require.ErrorContains(t, err, tt.errContain)
			assert.Nil(t, loop.GetVariable(loopVarRequireToolBatch))
		})
	}
}

func TestToolBatchVerifier_RejectsTruncatedAction(t *testing.T) {
	loop, _ := newToolBatchTestLoop(t)
	action, err := aicommon.ExtractActionFromStream(
		context.Background(),
		strings.NewReader(`{"@action":"directly_call_tool","directly_call_tool_calls":[{"tool_name":"read_file","params":{"path":"/a"}},{"tool_name":`),
		schema.AI_REACT_LOOP_ACTION_DIRECTLY_CALL_TOOL,
	)
	require.NoError(t, err)
	require.Error(t, loopAction_directlyCallTool.ActionVerifier(loop, action))
	assert.Nil(t, loop.GetVariable(loopVarDirectToolBatch))
}

type toolBatchHandlerTestInvoker struct {
	*testInvoker
	request *aicommon.ToolBatchRequest
	result  *aicommon.ToolBatchResult
	err     error
}

type executingToolBatchTestInvoker struct {
	*testInvoker
	manager  *buildinaitools.AiToolManager
	requests int
	executed []string
}

type executingToolScalarTestInvoker struct {
	*testInvoker
	manager   *buildinaitools.AiToolManager
	generated map[string]aitool.InvokeParams
	executed  []string
	received  []aitool.InvokeParams
}

func cloneScalarPromptParams(params aitool.InvokeParams) aitool.InvokeParams {
	cloned := make(aitool.InvokeParams, len(params))
	for key, value := range params {
		cloned[key] = value
	}
	return cloned
}

func (i *executingToolScalarTestInvoker) invokeTool(
	ctx context.Context,
	name string,
	tool *aitool.Tool,
	params aitool.InvokeParams,
) (*aitool.ToolResult, bool, error) {
	if tool == nil {
		var err error
		tool, err = i.manager.GetToolByName(name)
		if err != nil {
			return nil, false, err
		}
	}
	params = cloneScalarPromptParams(params)
	delete(params, aicommon.ReservedKeyIdentifier)
	delete(params, aicommon.ReservedKeyCallExpectations)
	result, err := tool.InvokeWithParams(
		params,
		aitool.WithContext(ctx),
		aitool.WithOutputCapture(false),
	)
	if err == nil {
		i.executed = append(i.executed, name)
		i.received = append(i.received, params)
	}
	return result, false, err
}

func (i *executingToolScalarTestInvoker) ExecuteToolRequiredAndCall(
	ctx context.Context,
	name string,
	options ...aicommon.ToolCallerOption,
) (*aitool.ToolResult, bool, error) {
	return i.invokeTool(ctx, name, nil, i.generated[name])
}

func (i *executingToolScalarTestInvoker) DirectlyCallTool(
	ctx context.Context,
	name string,
	action *aicommon.Action,
	prepare aicommon.DirectlyCallPrepareFunc,
) (*aitool.ToolResult, bool, error) {
	params, fallback, tool, err := prepare(action, name)
	if err != nil {
		return nil, false, err
	}
	if fallback {
		return i.ExecuteToolRequiredAndCall(ctx, name)
	}
	return i.invokeTool(ctx, name, tool, params)
}

func (i *executingToolBatchTestInvoker) ExecuteToolBatch(
	ctx context.Context,
	task aicommon.AIStatefulTask,
	request *aicommon.ToolBatchRequest,
) (*aicommon.ToolBatchResult, error) {
	i.requests++
	result := &aicommon.ToolBatchResult{
		BatchID:  request.BatchID,
		Outcomes: make([]aicommon.ToolCallOutcome, len(request.Calls)),
	}
	for index, call := range request.Calls {
		params := call.Params
		if call.Mode == aicommon.ToolCallModeRequire {
			switch call.ToolName {
			case "grep":
				params = aitool.InvokeParams{"path": "/workspace", "pattern": "auth"}
			case "read_file":
				params = aitool.InvokeParams{"path": "/workspace/project.json"}
			}
		}
		tool, lookupErr := i.manager.GetToolByName(call.ToolName)
		if lookupErr != nil {
			return nil, lookupErr
		}
		toolResult, invokeErr := tool.InvokeWithParams(
			params,
			aitool.WithContext(ctx),
			aitool.WithOutputCapture(false),
		)
		stage := aicommon.ToolCallStageDone
		if invokeErr != nil {
			stage = aicommon.ToolCallStageInvokeFailed
		} else {
			i.executed = append(i.executed, call.ToolName)
		}
		result.Outcomes[index] = aicommon.ToolCallOutcome{
			Index:         index,
			RequestedTool: call.ToolName,
			FinalTool:     call.ToolName,
			Stage:         stage,
			Result:        toolResult,
			Err:           invokeErr,
		}
	}
	return result, nil
}

func (i *toolBatchHandlerTestInvoker) ExecuteToolBatch(
	ctx context.Context,
	task aicommon.AIStatefulTask,
	request *aicommon.ToolBatchRequest,
) (*aicommon.ToolBatchResult, error) {
	i.request = request
	return i.result, i.err
}

func TestToolBatchActionHandler_UsesBatchRuntimeOnce(t *testing.T) {
	ctx := context.Background()
	manager, readFile, _ := newToolBatchTestManager(t)
	manager.AddRecentlyUsedTool(readFile)
	cfg := &aicommon.Config{AiToolManager: manager}
	task := newTestTask(ctx)
	invoker := &toolBatchHandlerTestInvoker{testInvoker: newTestInvoker(ctx)}
	invoker.currentTask = task
	invoker.result = &aicommon.ToolBatchResult{Outcomes: []aicommon.ToolCallOutcome{
		{Index: 0, RequestedTool: "read_file", FinalTool: "read_file", Stage: aicommon.ToolCallStageDone, Result: &aitool.ToolResult{Name: "read_file", Success: true}},
		{Index: 1, RequestedTool: "read_file", FinalTool: "read_file", Stage: aicommon.ToolCallStageDone, Result: &aitool.ToolResult{Name: "read_file", Success: true}},
	}}
	loop := reactloops.NewMinimalReActLoop(cfg, invoker)
	loop.SetCurrentTask(task)
	action := parseToolBatchPromptExample(t, directlyCallToolBatchOutputExampleJSON, schema.AI_REACT_LOOP_ACTION_DIRECTLY_CALL_TOOL)
	require.NoError(t, loopAction_directlyCallTool.ActionVerifier(loop, action))

	op := reactloops.NewActionHandlerOperator(task)
	loopAction_directlyCallTool.ActionHandler(loop, action, op)

	require.NotNil(t, invoker.request)
	assert.Len(t, invoker.request.Calls, 2)
	assert.True(t, op.IsContinued())
	assert.Contains(t, op.GetFeedback().String(), "Tool batch finished: 2 calls")
	assert.Contains(t, invoker.getTimelineString(), "TOOL_BATCH_RESULT")
}

func TestToolBatchSerialFallback_SettlesNilAndDirectAnswerOutcomes(t *testing.T) {
	t.Run("nil result is failure", func(t *testing.T) {
		invoker := newTestInvoker(context.Background())
		result := executeToolBatchSerialFallback(context.Background(), invoker, &aicommon.ToolBatchRequest{
			Calls: []aicommon.ToolBatchCall{
				{Index: 0, Mode: aicommon.ToolCallModeRequire, ToolName: "grep", Identifier: "first"},
				{Index: 1, Mode: aicommon.ToolCallModeRequire, ToolName: "read_file", Identifier: "second"},
			},
		})
		require.Len(t, result.Outcomes, 2)
		for _, outcome := range result.Outcomes {
			require.Equal(t, aicommon.ToolCallStageInvokeFailed, outcome.Stage)
			require.ErrorContains(t, outcome.Err, "no result")
		}
	})

	t.Run("direct answer cancels current and queued children", func(t *testing.T) {
		invoker := newTestInvoker(context.Background())
		invoker.toolCallDirectly = true
		result := executeToolBatchSerialFallback(context.Background(), invoker, &aicommon.ToolBatchRequest{
			Calls: []aicommon.ToolBatchCall{
				{Index: 0, Mode: aicommon.ToolCallModeRequire, ToolName: "grep"},
				{Index: 1, Mode: aicommon.ToolCallModeRequire, ToolName: "read_file"},
				{Index: 2, Mode: aicommon.ToolCallModeRequire, ToolName: "search"},
			},
		})
		require.True(t, result.DirectlyAnswer)
		require.Len(t, result.Outcomes, 3)
		for _, outcome := range result.Outcomes {
			require.Equal(t, aicommon.ToolCallStageCancelled, outcome.Stage)
		}
	})

	t.Run("context cancellation stops queued children", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		invoker := &cancelAfterFirstFallbackInvoker{
			testInvoker: newTestInvoker(ctx),
			cancel:      cancel,
		}
		result := executeToolBatchSerialFallback(ctx, invoker, &aicommon.ToolBatchRequest{
			Calls: []aicommon.ToolBatchCall{
				{Index: 0, Mode: aicommon.ToolCallModeRequire, ToolName: "grep"},
				{Index: 1, Mode: aicommon.ToolCallModeRequire, ToolName: "read_file"},
				{Index: 2, Mode: aicommon.ToolCallModeRequire, ToolName: "search"},
			},
		})
		require.Equal(t, 1, invoker.calls)
		require.Equal(t, aicommon.ToolCallStageDone, result.Outcomes[0].Stage)
		require.Equal(t, aicommon.ToolCallStageCancelled, result.Outcomes[1].Stage)
		require.Equal(t, aicommon.ToolCallStageCancelled, result.Outcomes[2].Stage)
	})
}

type cancelAfterFirstFallbackInvoker struct {
	*testInvoker
	cancel context.CancelFunc
	calls  int
}

func (i *cancelAfterFirstFallbackInvoker) ExecuteToolRequiredAndCall(
	ctx context.Context,
	name string,
	opts ...aicommon.ToolCallerOption,
) (*aitool.ToolResult, bool, error) {
	i.calls++
	if i.calls == 1 {
		i.cancel()
	}
	return &aitool.ToolResult{Name: name, Success: true}, false, nil
}

// This is the executable-prompt invariant: the exact JSON bytes embedded in
// the action schema are parsed, verified, dispatched through the real handler,
// and end in actual tool callbacks. Keeping both action modes here prevents a
// documentation-only example from drifting away from executable behaviour.
func TestToolBatchPromptExamples_ExecuteActualToolCallbacks(t *testing.T) {
	tests := []struct {
		name       string
		raw        string
		actionType string
		action     *reactloops.LoopAction
		expected   []string
	}{
		{
			name:       "directly_call_tool",
			raw:        directlyCallToolBatchOutputExampleJSON,
			actionType: schema.AI_REACT_LOOP_ACTION_DIRECTLY_CALL_TOOL,
			action:     loopAction_directlyCallTool,
			expected:   []string{"read_file", "read_file"},
		},
		{
			name:       "require_tool",
			raw:        requireToolBatchOutputExampleJSON,
			actionType: schema.AI_REACT_LOOP_ACTION_REQUIRE_TOOL,
			action:     loopAction_toolRequireAndCall,
			expected:   []string{"grep", "read_file"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			manager, readFile, _ := newToolBatchTestManager(t)
			manager.AddRecentlyUsedTool(readFile)
			cfg := &aicommon.Config{AiToolManager: manager}
			task := newTestTask(ctx)
			invoker := &executingToolBatchTestInvoker{
				testInvoker: newTestInvoker(ctx),
				manager:     manager,
			}
			invoker.currentTask = task
			loop := reactloops.NewMinimalReActLoop(cfg, invoker)
			loop.SetCurrentTask(task)

			action := parseToolBatchPromptExample(t, test.raw, test.actionType)
			require.NoError(t, test.action.ActionVerifier(loop, action))
			op := reactloops.NewActionHandlerOperator(task)
			test.action.ActionHandler(loop, action, op)

			require.Equal(t, 1, invoker.requests, "one model action must dispatch one joined batch")
			require.Equal(t, test.expected, invoker.executed)
			require.True(t, op.IsContinued())
			require.Contains(t, op.GetFeedback().String(), "Tool batch finished: 2 calls")
		})
	}
}

// Single-call examples have the same executable-prompt guarantee as batch
// examples: exact taught bytes must reach the legacy scalar handler and an
// actual tool callback. This prevents adding batch support from accidentally
// turning the scalar examples into stale documentation.
func TestToolScalarPromptExamples_ExecuteActualToolCallbacks(t *testing.T) {
	tests := []struct {
		name          string
		raw           string
		actionType    string
		action        *reactloops.LoopAction
		expectedTool  string
		expectedParam string
		expectedValue string
	}{
		{
			name:          "directly_call_tool",
			raw:           directlyCallToolScalarOutputExampleJSON,
			actionType:    schema.AI_REACT_LOOP_ACTION_DIRECTLY_CALL_TOOL,
			action:        loopAction_directlyCallTool,
			expectedTool:  "read_file",
			expectedParam: "path",
			expectedValue: "/workspace/go.mod",
		},
		{
			name:          "require_tool",
			raw:           requireToolScalarOutputExampleJSON,
			actionType:    schema.AI_REACT_LOOP_ACTION_REQUIRE_TOOL,
			action:        loopAction_toolRequireAndCall,
			expectedTool:  "grep",
			expectedParam: "pattern",
			expectedValue: "auth",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			manager, readFile, _ := newToolBatchTestManager(t)
			manager.AddRecentlyUsedTool(readFile)
			cfg := &aicommon.Config{AiToolManager: manager}
			task := newTestTask(ctx)
			invoker := &executingToolScalarTestInvoker{
				testInvoker: newTestInvoker(ctx),
				manager:     manager,
				generated: map[string]aitool.InvokeParams{
					"grep": {"path": "/workspace", "pattern": "auth"},
				},
			}
			invoker.currentTask = task
			loop := reactloops.NewMinimalReActLoop(cfg, invoker)
			loop.SetCurrentTask(task)

			requirePromptExampleMatchesActionSchema(t, test.raw, test.action)
			action := parseToolBatchPromptExample(t, test.raw, test.actionType)
			require.NoError(t, test.action.ActionVerifier(loop, action))
			op := reactloops.NewActionHandlerOperator(task)
			test.action.ActionHandler(loop, action, op)

			require.Equal(t, []string{test.expectedTool}, invoker.executed)
			require.Len(t, invoker.received, 1)
			assert.Equal(t, test.expectedValue, invoker.received[0].GetString(test.expectedParam))
			assert.True(t, op.IsContinued())
		})
	}
}
