package loopinfra

import (
	"context"
	"encoding/json"
	"io"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools/yakscripttools"
	"github.com/yaklang/yaklang/common/schema"
)

func newToolBatchTestManager(t *testing.T) (*buildinaitools.AiToolManager, *aitool.Tool, *aitool.Tool) {
	t.Helper()
	readFile := mustNewTool(
		"read_file",
		// Keep this fixture aligned with the production read_file Yak tool. A
		// stale "path" fixture previously let prompt examples pass CI while the
		// real tool rejected them and forced an extra reasoning-model retry.
		aitool.WithStringParam("file", aitool.WithParam_Required(true)),
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

func newToolBatchTestLoop(t *testing.T, emitters ...*aicommon.Emitter) (*reactloops.ReActLoop, *testInvoker) {
	t.Helper()
	ctx := context.Background()
	manager, readFile, _ := newToolBatchTestManager(t)
	manager.AddRecentlyUsedTool(readFile)
	cfg := &aicommon.Config{AiToolManager: manager}
	if len(emitters) > 0 {
		cfg.Emitter = emitters[0]
	}
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
		{name: "preferred object", params: map[string]any{"file": "/workspace/go.mod"}, valid: true},
		{name: "legacy JSON string", params: `{"file":"/workspace/go.mod"}`, valid: true},
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
// renderers, with the reliable scalar form taught before optional batching.
func TestToolCallPromptExamples_ParseAndVerifyExactBytes(t *testing.T) {
	t.Run("directly_call_tool scalar", func(t *testing.T) {
		loop, _ := newToolBatchTestLoop(t)
		requirePromptExampleMatchesActionSchema(t, directlyCallToolScalarOutputExampleJSON, loopAction_directlyCallTool)
		action := parseToolBatchPromptExample(t, directlyCallToolScalarOutputExampleJSON, schema.AI_REACT_LOOP_ACTION_DIRECTLY_CALL_TOOL)

		require.NoError(t, loopAction_directlyCallTool.ActionVerifier(loop, action))
		assert.Nil(t, loop.GetActionExecutionValue(action, actionStateDirectToolBatch))
		assert.Equal(t, "read_file", loop.GetActionExecutionValue(action, "directly_call_tool_name"))
		assert.Less(t,
			strings.Index(loopAction_directlyCallTool.OutputExamples, directlyCallToolScalarOutputExampleJSON),
			strings.Index(loopAction_directlyCallTool.OutputExamples, directlyCallToolBatchOutputExampleJSON),
			"the reliable scalar form should be taught before optional batching",
		)
		assert.Contains(t, loopAction_directlyCallTool.OutputExamples, directlyCallToolScalarOutputExampleJSON)
		assert.Contains(t, loopAction_directlyCallTool.OutputExamples, directlyCallToolBatchOutputExampleJSON)
	})

	t.Run("directly_call_tool batch", func(t *testing.T) {
		loop, _ := newToolBatchTestLoop(t)
		requirePromptExampleMatchesActionSchema(t, directlyCallToolBatchOutputExampleJSON, loopAction_directlyCallTool)
		action := parseToolBatchPromptExample(t, directlyCallToolBatchOutputExampleJSON, schema.AI_REACT_LOOP_ACTION_DIRECTLY_CALL_TOOL)

		require.NoError(t, loopAction_directlyCallTool.ActionVerifier(loop, action))
		request, ok := loop.GetActionExecutionValue(action, actionStateDirectToolBatch).(*aicommon.ToolCallGroupRequest)
		require.True(t, ok)
		require.Len(t, request.Calls, 2)
		assert.NotNil(t, request.Calls[0].Params)
		assert.Equal(t, "read_file", request.Calls[0].ToolName)
		assert.Equal(t, "/workspace/go.mod", request.Calls[0].Params.GetString("file"))
		assert.Equal(t, "read_go_mod", request.Calls[0].Identifier)
		assert.Equal(t, "/workspace/README.md", request.Calls[1].Params.GetString("file"))
		assert.Contains(t, loopAction_directlyCallTool.OutputExamples, directlyCallToolBatchOutputExampleJSON)
	})

	t.Run("require_tool scalar", func(t *testing.T) {
		loop, _ := newToolBatchTestLoop(t)
		requirePromptExampleMatchesActionSchema(t, requireToolScalarOutputExampleJSON, loopAction_toolRequireAndCall)
		action := parseToolBatchPromptExample(t, requireToolScalarOutputExampleJSON, schema.AI_REACT_LOOP_ACTION_REQUIRE_TOOL)

		require.NoError(t, loopAction_toolRequireAndCall.ActionVerifier(loop, action))
		assert.Equal(t, []string{"grep"}, loop.GetActionExecutionValue(action, actionStateToolSchemaNames))
		assert.Less(t,
			strings.Index(loopAction_toolRequireAndCall.OutputExamples, requireToolScalarOutputExampleJSON),
			strings.Index(loopAction_toolRequireAndCall.OutputExamples, requireToolBatchOutputExampleJSON),
			"the reliable scalar form should be taught before optional batching",
		)
		assert.Contains(t, loopAction_toolRequireAndCall.OutputExamples, requireToolScalarOutputExampleJSON)
		assert.Contains(t, loopAction_toolRequireAndCall.OutputExamples, requireToolBatchOutputExampleJSON)
	})

	t.Run("require_tool batch", func(t *testing.T) {
		loop, _ := newToolBatchTestLoop(t)
		requirePromptExampleMatchesActionSchema(t, requireToolBatchOutputExampleJSON, loopAction_toolRequireAndCall)
		action := parseToolBatchPromptExample(t, requireToolBatchOutputExampleJSON, schema.AI_REACT_LOOP_ACTION_REQUIRE_TOOL)

		require.NoError(t, loopAction_toolRequireAndCall.ActionVerifier(loop, action))
		names, ok := loop.GetActionExecutionValue(action, actionStateToolSchemaNames).([]string)
		require.True(t, ok)
		require.Len(t, names, 2)
		assert.Equal(t, "grep", names[0])
		assert.Equal(t, "read_file", names[1])
		assert.Contains(t, loopAction_toolRequireAndCall.OutputExamples, requireToolBatchOutputExampleJSON)
	})
}

// Cross-check the prompt fixture against the embedded production read_file
// definition, rather than only against the test double. This catches a future
// CLI parameter rename before a highly weighted few-shot teaches every model
// an invalid direct-call payload.
func TestToolCallPromptExamples_MatchProductionReadFileParameter(t *testing.T) {
	source, err := yakscripttools.GetEmbedFS().ReadFile("yakscriptforai/fs/read_file.yak")
	require.NoError(t, err)

	pathParamPattern := regexp.MustCompile(`cli\.String\("([^"]+)",\s*cli\.setRequired\(true\),\s*cli\.setHelp\("target file absolute path`)
	match := pathParamPattern.FindSubmatch(source)
	require.Len(t, match, 2, "production read_file must expose a required absolute-path parameter")
	pathParamName := string(match[1])

	var scalar map[string]any
	require.NoError(t, json.Unmarshal([]byte(directlyCallToolScalarOutputExampleJSON), &scalar))
	scalarParams, ok := scalar["directly_call_tool_params"].(map[string]any)
	require.True(t, ok)
	assert.Contains(t, scalarParams, pathParamName)

	var batch map[string]any
	require.NoError(t, json.Unmarshal([]byte(directlyCallToolBatchOutputExampleJSON), &batch))
	calls, ok := batch[directlyCallToolBatchField].([]any)
	require.True(t, ok)
	for index, rawCall := range calls {
		call, ok := rawCall.(map[string]any)
		require.True(t, ok, "call %d must be an object", index)
		params, ok := call["params"].(map[string]any)
		require.True(t, ok, "call %d params must be an object", index)
		assert.Contains(t, params, pathParamName, "call %d must match production read_file", index)
	}
}

func TestToolCallActionsOnlyAdvertiseUnifiedPayloads(t *testing.T) {
	for _, action := range []*reactloops.LoopAction{loopAction_toolRequireAndCall, nativeToolSchemaLoadAction} {
		schemaTool := newPromptActionSchemaTool(t, action)
		require.Contains(t, schemaTool.ParamsJsonSchemaString(), requireToolPayloadField)
		require.NotContains(t, schemaTool.ParamsJsonSchemaString(), "tool_require_calls")
		require.Contains(t, action.Description, "directly_call_tool")
	}
	for _, action := range []*reactloops.LoopAction{loopAction_directlyCallTool, nativeDirectToolAction} {
		schemaTool := newPromptActionSchemaTool(t, action)
		require.Contains(t, schemaTool.ParamsJsonSchemaString(), directlyCallToolBatchField)
		require.NotContains(t, schemaTool.ParamsJsonSchemaString(), "directly_call_tool_calls")
	}
}

func TestToolParameterGroupSchema_DeclaresStrictObjectArrays(t *testing.T) {
	for _, action := range []*reactloops.LoopAction{loopAction_directlyCallTool, nativeDirectToolAction} {
		options := make([]any, 0, len(action.Options))
		for _, option := range action.Options {
			options = append(options, option)
		}
		var root map[string]any
		require.NoError(t, json.Unmarshal([]byte(aitool.NewObjectSchema(options...)), &root))
		property := root["properties"].(map[string]any)[directlyCallToolBatchField].(map[string]any)
		require.Equal(t, "array", property["type"])
		require.EqualValues(t, 2, property["minItems"])
		require.EqualValues(t, aicommon.DefaultToolBatchMaxCalls, property["maxItems"])
		item := property["items"].(map[string]any)
		require.Equal(t, false, item["additionalProperties"])
		require.ElementsMatch(t, []any{"tool_name", "params"}, item["required"])
	}
	for _, action := range []*reactloops.LoopAction{loopAction_toolRequireAndCall, nativeToolSchemaLoadAction} {
		schemaTool := newPromptActionSchemaTool(t, action)
		valid, errs := schemaTool.ValidateParams(aitool.InvokeParams{"@action": "require_tool", "identifier": "load", requireToolPayloadField: "read_file"})
		require.True(t, valid, "%v", errs)
		valid, errs = schemaTool.ValidateParams(aitool.InvokeParams{"@action": "require_tool", "identifier": "load", requireToolPayloadField: []any{"read_file", "grep"}})
		require.True(t, valid, "%v", errs)
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
			name:       "not_an_array",
			payload:    `{"@action":"directly_call_tool","directly_call_tool_params_group":{"tool_name":"read_file","params":{"file":"/a"}}}`,
			errContain: "array of objects",
		},
		{
			name:       "one_item",
			payload:    `{"@action":"directly_call_tool","directly_call_tool_params_group":[{"tool_name":"read_file","params":{"file":"/a"}}]}`,
			errContain: "at least 2",
		},
		{
			name: "second_params_invalid",
			payload: `{
				"@action":"directly_call_tool",
				"directly_call_tool_params_group":[
					{"tool_name":"read_file","params":{"file":"/valid"}},
					{"tool_name":"read_file","params":{}}
				]
			}`,
			errContain: "params are invalid",
		},
		{
			name: "duplicate_identifier",
			payload: `{
				"@action":"directly_call_tool",
				"directly_call_tool_params_group":[
					{"tool_name":"read_file","params":{"file":"/a"},"identifier":"same"},
					{"tool_name":"read_file","params":{"file":"/b"},"identifier":"same"}
				]
			}`,
			errContain: "duplicates",
		},
		{
			name: "cross_action_batch_is_rejected",
			payload: `{
				"@action":"directly_call_tool",
				"directly_call_tool_params_group":[
					{"tool_name":"read_file","params":{"file":"/a"}},
					{"tool_name":"read_file","params":{"file":"/b"}}
				],
				"require_tool_payload":["grep", "read_file"]
			}`,
			errContain: "cannot be combined with require_tool fields",
		},
		{
			name: "params_array_is_not_an_object",
			payload: `{
				"@action":"directly_call_tool",
				"directly_call_tool_params_group":[
					{"tool_name":"read_file","params":[]},
					{"tool_name":"read_file","params":{"file":"/b"}}
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
			assert.Nil(t, loop.GetActionExecutionValue(action, actionStateDirectToolBatch), "invalid batch must never reach the handler")
		})
	}
}

func TestRequireToolPayloadRejectsExecutionAndRetiredFields(t *testing.T) {
	for _, raw := range []string{
		`{"@action":"require_tool","require_tool_payload":[{"tool_name":"read_file","params":{"file":"a"}}]}`,
		`{"@action":"require_tool","tool_require_calls":[{"tool_name":"read_file"}]}`,
		`{"@action":"require_tool","require_tool_payload":["read_file"],"directly_call_tool_params_group":[{"tool_name":"read_file","params":{"file":"a"}},{"tool_name":"read_file","params":{"file":"b"}}]}`,
	} {
		loop, _ := newToolBatchTestLoop(t)
		action := parseToolBatchPromptExample(t, raw, "require_tool")
		require.Error(t, verifyToolSchemaLoad(loop, action))
		require.Nil(t, loop.GetActionExecutionValue(action, actionStateToolSchemaNames))
	}
}

func TestToolScalarVerifier_PreservesLegacyPriorityWhenMalformedActionAlsoContainsBatch(t *testing.T) {
	tests := []struct {
		name       string
		actionType string
		payload    string
		verify     func(*reactloops.ReActLoop, *aicommon.Action) error
		stateKey   string
		wantValue  any
	}{
		{
			name:       "direct",
			actionType: schema.AI_REACT_LOOP_ACTION_DIRECTLY_CALL_TOOL,
			payload:    `{"@action":"directly_call_tool","directly_call_tool_name":"read_file","directly_call_tool_params":{"file":"/scalar"},"directly_call_tool_params_group":[{"tool_name":"read_file","params":{"file":"/a"}},{"tool_name":"read_file","params":{"file":"/b"}}]}`,
			verify:     loopAction_directlyCallTool.ActionVerifier,
			stateKey:   "directly_call_tool_name",
			wantValue:  "read_file",
		},
		{
			name:       "require",
			actionType: schema.AI_REACT_LOOP_ACTION_REQUIRE_TOOL,
			payload:    `{"@action":"require_tool","require_tool_payload":"grep","tool_require_calls":[{"tool_name":"grep"},{"tool_name":"read_file"}]}`,
			verify:     loopAction_toolRequireAndCall.ActionVerifier,
			stateKey:   actionStateToolSchemaNames,
			wantValue:  []string{"grep"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			loop, _ := newToolBatchTestLoop(t)
			action := parseToolBatchPromptExample(t, test.payload, test.actionType)
			require.NoError(t, test.verify(loop, action))
			require.Equal(t, test.wantValue, loop.GetActionExecutionValue(action, test.stateKey))
			require.Nil(t, loop.GetActionExecutionValue(action, actionStateDirectToolBatch))
		})
	}
}

func TestToolBatchVerifier_RejectsTruncatedAction(t *testing.T) {
	loop, _ := newToolBatchTestLoop(t)
	action, err := aicommon.ExtractActionFromStream(
		context.Background(),
		strings.NewReader(`{"@action":"directly_call_tool","directly_call_tool_params_group":[{"tool_name":"read_file","params":{"file":"/a"}},{"tool_name":`),
		schema.AI_REACT_LOOP_ACTION_DIRECTLY_CALL_TOOL,
	)
	require.NoError(t, err)
	require.Error(t, loopAction_directlyCallTool.ActionVerifier(loop, action))
	assert.Nil(t, loop.GetActionExecutionValue(action, actionStateDirectToolBatch))
}

type eofGateReader struct {
	reader  *strings.Reader
	waiting chan struct{}
	release chan struct{}
	once    sync.Once
}

func (r *eofGateReader) Read(p []byte) (int, error) {
	if r.reader.Len() > 0 {
		return r.reader.Read(p)
	}
	r.once.Do(func() { close(r.waiting) })
	<-r.release
	return 0, io.EOF
}

func TestToolBatchVerifier_WaitsForCompleteResponseEOF(t *testing.T) {
	payload := `{"@action":"directly_call_tool","directly_call_tool_params_group":[{"tool_name":"read_file","params":{"file":"/a"}},{"tool_name":"read_file","params":{"file":"/b"}}]}`
	source := &eofGateReader{
		reader:  strings.NewReader(payload),
		waiting: make(chan struct{}),
		release: make(chan struct{}),
	}
	action, err := aicommon.ExtractActionFromStream(
		context.Background(), source, schema.AI_REACT_LOOP_ACTION_DIRECTLY_CALL_TOOL,
	)
	require.NoError(t, err)

	loop, _ := newToolBatchTestLoop(t)
	verifyDone := make(chan error, 1)
	go func() { verifyDone <- loopAction_directlyCallTool.ActionVerifier(loop, action) }()
	<-source.waiting
	select {
	case err := <-verifyDone:
		t.Fatalf("batch verifier returned before response EOF: %v", err)
	case <-time.After(20 * time.Millisecond):
	}

	close(source.release)
	require.NoError(t, <-verifyDone)
	require.NotNil(t, loop.GetActionExecutionValue(action, actionStateDirectToolBatch))
}

func TestToolScalarVerifier_ReturnsBeforeCompleteResponseEOF(t *testing.T) {
	tests := []struct {
		name       string
		actionType string
		payload    string
		verify     func(*reactloops.ReActLoop, *aicommon.Action) error
		stateKey   string
		want       any
	}{
		{
			name:       "direct",
			actionType: schema.AI_REACT_LOOP_ACTION_DIRECTLY_CALL_TOOL,
			payload:    `{"@action":"directly_call_tool","directly_call_tool_name":"read_file","directly_call_tool_params":{"file":"/a"}}`,
			verify:     loopAction_directlyCallTool.ActionVerifier,
			stateKey:   "directly_call_tool_name",
			want:       "read_file",
		},
		{
			name:       "require",
			actionType: schema.AI_REACT_LOOP_ACTION_REQUIRE_TOOL,
			payload:    `{"@action":"require_tool","require_tool_payload":"grep","human_readable_thought":"prepare search"}`,
			verify:     loopAction_toolRequireAndCall.ActionVerifier,
			stateKey:   actionStateToolSchemaNames,
			want:       []string{"grep"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			source := &eofGateReader{
				reader:  strings.NewReader(test.payload),
				waiting: make(chan struct{}),
				release: make(chan struct{}),
			}
			action, err := aicommon.ExtractActionFromStream(context.Background(), source, test.actionType)
			require.NoError(t, err)
			loop, _ := newToolBatchTestLoop(t)
			verifyDone := make(chan error, 1)
			go func() { verifyDone <- test.verify(loop, action) }()
			<-source.waiting

			select {
			case err := <-verifyDone:
				require.NoError(t, err)
				require.Equal(t, test.want, loop.GetActionExecutionValue(action, test.stateKey))
			case <-time.After(time.Second):
				t.Fatal("legacy scalar verifier waited for response EOF")
			}

			close(source.release)
			require.NoError(t, action.WaitParseResult(context.Background()))
		})
	}
}

type toolBatchHandlerTestInvoker struct {
	*testInvoker
	request     *aicommon.ToolCallGroupRequest
	result      *aicommon.ToolCallGroupResult
	err         error
	verifyCalls int
}

func (i *toolBatchHandlerTestInvoker) VerifyUserSatisfaction(
	ctx context.Context,
	query string,
	isToolCall bool,
	payload string,
) (*aicommon.VerifySatisfactionResult, error) {
	i.verifyCalls++
	return i.verifySatisfactionResult, nil
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
		return nil, false, &aicommon.ToolCallRetryError{ToolName: name, Reason: "invalid explicit arguments"}
	}
	return i.invokeTool(ctx, name, tool, params)
}

func (i *executingToolBatchTestInvoker) ExecuteToolCallGroup(
	ctx context.Context,
	task aicommon.AIStatefulTask,
	request *aicommon.ToolCallGroupRequest,
) (*aicommon.ToolCallGroupResult, error) {
	i.requests++
	result := &aicommon.ToolCallGroupResult{
		BatchID:  request.BatchID,
		Outcomes: make([]aicommon.ToolCallOutcome, len(request.Calls)),
	}
	for index, call := range request.Calls {
		params := call.Params

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

func (i *toolBatchHandlerTestInvoker) ExecuteToolCallGroup(
	ctx context.Context,
	task aicommon.AIStatefulTask,
	request *aicommon.ToolCallGroupRequest,
) (*aicommon.ToolCallGroupResult, error) {
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
	invoker.result = &aicommon.ToolCallGroupResult{Outcomes: []aicommon.ToolCallOutcome{
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
	assert.Contains(t, op.GetFeedback().String(), "Tool batch settled: 2 calls")
	assert.Contains(t, op.GetFeedback().String(), "execution_unknown=2")
	assert.Contains(t, invoker.getTimelineString(), "TOOL_BATCH_RESULT")
	assert.Equal(t, 2, op.GetExecutedToolCallCount())
	assert.Equal(t, 1, invoker.verifyCalls)
}

func TestToolBatchActionHandler_ZeroInvokeDoesNotVerifyOrMarkExecution(t *testing.T) {
	newFixture := func(result *aicommon.ToolCallGroupResult) (*toolBatchHandlerTestInvoker, *reactloops.ReActLoop, *reactloops.LoopActionHandlerOperator, *aicommon.ToolCallGroupRequest) {
		ctx := context.Background()
		manager, _, _ := newToolBatchTestManager(t)
		cfg := &aicommon.Config{AiToolManager: manager}
		task := newTestTask(ctx)
		invoker := &toolBatchHandlerTestInvoker{testInvoker: newTestInvoker(ctx), result: result}
		invoker.currentTask = task
		loop := reactloops.NewMinimalReActLoop(cfg, invoker)
		loop.SetCurrentTask(task)
		request := &aicommon.ToolCallGroupRequest{Calls: []aicommon.ToolCallGroupCall{
			{Index: 0, ToolName: "read_file"},
			{Index: 1, ToolName: "grep"},
		}}
		return invoker, loop, reactloops.NewActionHandlerOperator(task), request
	}

	t.Run("whole batch rejected at admission", func(t *testing.T) {
		result := &aicommon.ToolCallGroupResult{Outcomes: []aicommon.ToolCallOutcome{
			{Index: 0, RequestedTool: "read_file", Stage: aicommon.ToolCallStageValidationFailed, Err: assert.AnError},
			{Index: 1, RequestedTool: "grep", Stage: aicommon.ToolCallStageCancelled, Err: context.Canceled},
		}}
		invoker, loop, op, request := newFixture(result)
		handleToolBatchActionResult(loop, context.Background(), invoker, request, result, nil, op)

		require.Zero(t, op.GetExecutedToolCallCount())
		require.Zero(t, invoker.verifyCalls,
			"zero plugin callbacks must not trigger satisfaction verification")
		require.True(t, op.IsContinued())
	})

	t.Run("review direct answer", func(t *testing.T) {
		result := &aicommon.ToolCallGroupResult{DirectlyAnswer: true, Outcomes: []aicommon.ToolCallOutcome{
			{Index: 0, RequestedTool: "read_file", Stage: aicommon.ToolCallStageCancelled, DirectlyAnswer: true},
			{Index: 1, RequestedTool: "grep", Stage: aicommon.ToolCallStageCancelled},
		}}
		invoker, loop, op, request := newFixture(result)
		handleToolBatchActionResult(loop, context.Background(), invoker, request, result, nil, op)

		require.Zero(t, op.GetExecutedToolCallCount())
		require.Zero(t, invoker.verifyCalls)
		terminated, err := op.IsTerminated()
		require.True(t, terminated)
		require.NoError(t, err)
	})

	t.Run("failed callback still is execution", func(t *testing.T) {
		result := &aicommon.ToolCallGroupResult{Outcomes: []aicommon.ToolCallOutcome{
			{Index: 0, RequestedTool: "read_file", FinalTool: "read_file", Stage: aicommon.ToolCallStageInvokeFailed, Result: &aitool.ToolResult{Name: "read_file", Success: false, Error: "fixture failure"}},
			{Index: 1, RequestedTool: "grep", Stage: aicommon.ToolCallStagePrepareFailed, Err: assert.AnError},
		}}
		invoker, loop, op, request := newFixture(result)
		handleToolBatchActionResult(loop, context.Background(), invoker, request, result, nil, op)

		require.Equal(t, 1, op.GetExecutedToolCallCount())
		require.Equal(t, 1, invoker.verifyCalls,
			"a settled failed ToolResult is still high-value objective feedback")
		require.True(t, op.IsContinued())
	})

	t.Run("protocol-complete semantic failure stays failed", func(t *testing.T) {
		result := &aicommon.ToolCallGroupResult{Outcomes: []aicommon.ToolCallOutcome{
			{
				Index:         0,
				RequestedTool: "bash",
				FinalTool:     "bash",
				Stage:         aicommon.ToolCallStageDone,
				Result: &aitool.ToolResult{Name: "bash", Success: true, Data: &aitool.ToolExecutionResult{Result: map[string]any{
					"exit_code":          7,
					"exit_code_accepted": false,
				}}},
			},
			{Index: 1, RequestedTool: "grep", Stage: aicommon.ToolCallStageValidationFailed, Err: assert.AnError},
		}}
		invoker, loop, op, request := newFixture(result)
		handleToolBatchActionResult(loop, context.Background(), invoker, request, result, nil, op)

		feedback := op.GetFeedback().String()
		require.Contains(t, feedback, "execution_failed=1")
		require.Contains(t, feedback, "protocol-completed; execution-failed")
		require.Contains(t, feedback, "exit_code=7")
		require.NotContains(t, feedback, "bash: done")
	})
}

func TestToolBatchActionHandler_CachesSuccessAndFailedSchemaForRetry(t *testing.T) {
	ctx := context.Background()
	manager, readFile, grep := newToolBatchTestManager(t)
	cfg := &aicommon.Config{AiToolManager: manager, Timeline: aicommon.NewTimeline(nil, nil)}
	task := newTestTask(ctx)
	invoker := &toolBatchHandlerTestInvoker{testInvoker: newTestInvoker(ctx)}
	invoker.currentTask = task
	invoker.result = &aicommon.ToolCallGroupResult{Outcomes: []aicommon.ToolCallOutcome{
		{Index: 0, RequestedTool: readFile.Name, FinalTool: readFile.Name, Stage: aicommon.ToolCallStageDone, Result: &aitool.ToolResult{Name: readFile.Name, Success: true}},
		{Index: 1, RequestedTool: grep.Name, FinalTool: grep.Name, Stage: aicommon.ToolCallStageInvokeFailed, Result: &aitool.ToolResult{Name: grep.Name, Success: false, Error: "fixture failure"}},
	}}
	loop := reactloops.NewMinimalReActLoop(cfg, invoker)
	loop.SetCurrentTask(task)
	request := &aicommon.ToolCallGroupRequest{Calls: []aicommon.ToolCallGroupCall{
		{Index: 0, ToolName: readFile.Name, Params: aitool.InvokeParams{"file": "/workspace/go.mod"}},
		{Index: 1, ToolName: grep.Name, Params: aitool.InvokeParams{"pattern": "auth"}},
	}}

	op := reactloops.NewActionHandlerOperator(task)
	handleToolBatchActionResult(loop, ctx, invoker, request, invoker.result, nil, op)

	require.True(t, manager.IsRecentlyUsedTool(readFile.Name), "a protocol-complete child must remain available for future scalar direct calls")
	require.True(t, manager.IsRecentlyUsedTool(grep.Name), "a failed child must expose its schema for corrected direct retry")
	require.ElementsMatch(t, []string{readFile.Name, grep.Name}, manager.GetRecentToolNames())
	require.Contains(t, op.GetFeedback().String(), "reason: protocol-error: fixture failure")
	require.Contains(t, op.GetFeedback().String(), "retry:")
	require.Contains(t, op.GetFeedback().String(), "避免重复执行")
	require.Equal(t, 2, op.GetExecutedToolCallCount(), "failed result is not converted to success or hidden")
	materials := aicommon.RenderTimelineFrozenOpen(cfg.Timeline)
	require.Contains(t, materials.Open+materials.PromotedRecentTools, readFile.Name)
	require.Contains(t, materials.Open+materials.PromotedRecentTools, grep.Name)
	require.True(t, op.IsContinued())
}

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
			expected:   nil, // require_tool only loads schemas, does not execute
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

			if test.action.ActionType == schema.AI_REACT_LOOP_ACTION_REQUIRE_TOOL {
				// require_tool only loads schemas; no batch execution occurs.
				assert.Equal(t, 0, invoker.requests)
				assert.Empty(t, invoker.executed)
				assert.True(t, op.IsContinued())
			} else {
				require.Equal(t, 1, invoker.requests, "one model action must dispatch one joined batch")
				require.Equal(t, test.expected, invoker.executed)
				require.True(t, op.IsContinued())
				require.Contains(t, op.GetFeedback().String(), "Tool batch settled: 2 calls")
			}
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
			expectedParam: "file",
			expectedValue: "/workspace/go.mod",
		},
		{
			name:          "require_tool",
			raw:           requireToolScalarOutputExampleJSON,
			actionType:    schema.AI_REACT_LOOP_ACTION_REQUIRE_TOOL,
			action:        loopAction_toolRequireAndCall,
			expectedTool:  "grep",
			expectedParam: "",
			expectedValue: "",
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

			if test.action.ActionType == schema.AI_REACT_LOOP_ACTION_REQUIRE_TOOL {
				// require_tool only loads schemas; it does not execute any tool.
				assert.Empty(t, invoker.executed)
				assert.True(t, op.IsContinued())
			} else {
				require.Equal(t, []string{test.expectedTool}, invoker.executed)
				require.Len(t, invoker.received, 1)
				assert.Equal(t, test.expectedValue, invoker.received[0].GetString(test.expectedParam))
				assert.True(t, op.IsContinued())
			}
		})
	}
}

func TestToolParameterGroupWithoutRuntimeReturnsRetryWithoutExecution(t *testing.T) {
	var statuses []aicommon.StatusPayload
	emitter := aicommon.NewEmitter("unsupported-group-status", func(event *schema.AiOutputEvent) (*schema.AiOutputEvent, error) {
		if event.NodeId == "status" {
			var status aicommon.StatusPayload
			require.NoError(t, json.Unmarshal(event.Content, &status))
			statuses = append(statuses, status)
		}
		return event, nil
	})
	loop, invoker := newToolBatchTestLoop(t, emitter)
	action := parseToolBatchPromptExample(t, directlyCallToolBatchOutputExampleJSON, "directly_call_tool")
	require.NoError(t, loopAction_directlyCallTool.ActionVerifier(loop, action))
	operator := reactloops.NewActionHandlerOperator(newTestTask(context.Background()))
	loopAction_directlyCallTool.ActionHandler(loop, action, operator)
	require.True(t, operator.IsContinued())
	require.Zero(t, operator.GetExecutedToolCallCount())
	require.False(t, invoker.toolCallCalled)
	require.Contains(t, operator.GetFeedback().String(), "reason:")
	require.Contains(t, operator.GetFeedback().String(), "retry:")
	require.Contains(t, invoker.getTimelineString(), "完整 Schema 已放入 CACHE_TOOL_CALL")
	require.NotEmpty(t, statuses)
	require.Equal(t, "tool.batch.failed", statuses[len(statuses)-1].Code)
	for _, status := range statuses {
		require.NotEqual(t, "tool.batch.running", status.Code, "unsupported runtime did not start calls")
	}
}
