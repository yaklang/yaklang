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
// renderers, with the preferred independent-batch form taught before the
// single-call fallback.
func TestToolCallPromptExamples_ParseAndVerifyExactBytes(t *testing.T) {
	t.Run("directly_call_tool scalar", func(t *testing.T) {
		loop, _ := newToolBatchTestLoop(t)
		requirePromptExampleMatchesActionSchema(t, directlyCallToolScalarOutputExampleJSON, loopAction_directlyCallTool)
		action := parseToolBatchPromptExample(t, directlyCallToolScalarOutputExampleJSON, schema.AI_REACT_LOOP_ACTION_DIRECTLY_CALL_TOOL)

		require.NoError(t, loopAction_directlyCallTool.ActionVerifier(loop, action))
		assert.Nil(t, loop.GetVariable(loopVarDirectToolBatch))
		assert.Equal(t, "read_file", loop.Get("directly_call_tool_name"))
		assert.Less(t,
			strings.Index(loopAction_directlyCallTool.OutputExamples, directlyCallToolBatchOutputExampleJSON),
			strings.Index(loopAction_directlyCallTool.OutputExamples, directlyCallToolScalarOutputExampleJSON),
			"the preferred independent-batch form should be taught before the scalar fallback",
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
		assert.Nil(t, loop.GetVariable(loopVarRequireToolBatch))
		assert.Equal(t, "grep", loop.Get("tool_require_payload"))
		assert.Less(t,
			strings.Index(loopAction_toolRequireAndCall.OutputExamples, requireToolBatchOutputExampleJSON),
			strings.Index(loopAction_toolRequireAndCall.OutputExamples, requireToolScalarOutputExampleJSON),
			"the preferred independent-batch form should be taught before the scalar fallback",
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

func TestToolCallActionDescriptionsUseChineseBatchFirstPolicy(t *testing.T) {
	tests := []struct {
		name        string
		action      *reactloops.LoopAction
		batchField  string
		scalarField string
	}{
		{
			name:        "direct",
			action:      loopAction_directlyCallTool,
			batchField:  directlyCallToolBatchField,
			scalarField: "directly_call_tool_name",
		},
		{
			name:        "require",
			action:      loopAction_toolRequireAndCall,
			batchField:  requireToolBatchField,
			scalarField: "tool_require_payload",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			require.Contains(t, test.action.Description, "先枚举本轮已明确的真实调用")
			require.Contains(t, test.action.Description, "优先使用")
			require.Contains(t, test.action.Description, "不要拆成多个单工具轮次")
			require.NotContains(t, test.action.Description, "For one call")
			require.NotContains(t, test.action.Description, "Required only")

			schemaOptions := make([]any, 0, len(test.action.Options))
			for _, option := range test.action.Options {
				schemaOptions = append(schemaOptions, option)
			}
			var root map[string]any
			require.NoError(t, json.Unmarshal([]byte(aitool.NewObjectSchema(schemaOptions...)), &root))
			properties := root["properties"].(map[string]any)
			batchDescription := properties[test.batchField].(map[string]any)["description"].(string)
			scalarDescription := properties[test.scalarField].(map[string]any)["description"].(string)
			require.Contains(t, batchDescription, "优先使用本数组")
			require.Contains(t, batchDescription, "不要为了沿用单工具而拆成多轮")
			require.Contains(t, scalarDescription, "仅当本轮恰好一个")
		})
	}
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
			name:       "not_an_array",
			payload:    `{"@action":"directly_call_tool","directly_call_tool_calls":{"tool_name":"read_file","params":{"file":"/a"}}}`,
			errContain: "array of objects",
		},
		{
			name:       "one_item",
			payload:    `{"@action":"directly_call_tool","directly_call_tool_calls":[{"tool_name":"read_file","params":{"file":"/a"}}]}`,
			errContain: "at least 2",
		},
		{
			name: "second_params_invalid",
			payload: `{
				"@action":"directly_call_tool",
				"directly_call_tool_calls":[
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
				"directly_call_tool_calls":[
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
				"directly_call_tool_calls":[
					{"tool_name":"read_file","params":{"file":"/a"}},
					{"tool_name":"read_file","params":{"file":"/b"}}
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
			name: "cross_action_batch_is_rejected",
			payload: `{
				"@action":"require_tool",
				"tool_require_calls":[
					{"tool_name":"grep"},
					{"tool_name":"read_file"}
				],
				"directly_call_tool_calls":[
					{"tool_name":"read_file","params":{"file":"/a"}},
					{"tool_name":"read_file","params":{"file":"/b"}}
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

func TestToolScalarVerifier_PreservesLegacyPriorityWhenMalformedActionAlsoContainsBatch(t *testing.T) {
	tests := []struct {
		name       string
		actionType string
		payload    string
		verify     func(*reactloops.ReActLoop, *aicommon.Action) error
		stateKey   string
		wantValue  string
	}{
		{
			name:       "direct",
			actionType: schema.AI_REACT_LOOP_ACTION_DIRECTLY_CALL_TOOL,
			payload:    `{"@action":"directly_call_tool","directly_call_tool_name":"read_file","directly_call_tool_params":{"file":"/scalar"},"directly_call_tool_calls":[{"tool_name":"read_file","params":{"file":"/a"}},{"tool_name":"read_file","params":{"file":"/b"}}]}`,
			verify:     loopAction_directlyCallTool.ActionVerifier,
			stateKey:   "directly_call_tool_name",
			wantValue:  "read_file",
		},
		{
			name:       "require",
			actionType: schema.AI_REACT_LOOP_ACTION_REQUIRE_TOOL,
			payload:    `{"@action":"require_tool","tool_require_payload":"grep","tool_require_calls":[{"tool_name":"grep"},{"tool_name":"read_file"}]}`,
			verify:     loopAction_toolRequireAndCall.ActionVerifier,
			stateKey:   "tool_require_payload",
			wantValue:  "grep",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			loop, _ := newToolBatchTestLoop(t)
			action := parseToolBatchPromptExample(t, test.payload, test.actionType)
			require.NoError(t, test.verify(loop, action))
			require.Equal(t, test.wantValue, loop.Get(test.stateKey))
			require.Nil(t, loop.GetVariable(loopVarDirectToolBatch))
			require.Nil(t, loop.GetVariable(loopVarRequireToolBatch))
		})
	}
}

func TestToolBatchVerifier_RejectsTruncatedAction(t *testing.T) {
	loop, _ := newToolBatchTestLoop(t)
	action, err := aicommon.ExtractActionFromStream(
		context.Background(),
		strings.NewReader(`{"@action":"directly_call_tool","directly_call_tool_calls":[{"tool_name":"read_file","params":{"file":"/a"}},{"tool_name":`),
		schema.AI_REACT_LOOP_ACTION_DIRECTLY_CALL_TOOL,
	)
	require.NoError(t, err)
	require.Error(t, loopAction_directlyCallTool.ActionVerifier(loop, action))
	assert.Nil(t, loop.GetVariable(loopVarDirectToolBatch))
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
	payload := `{"@action":"directly_call_tool","directly_call_tool_calls":[{"tool_name":"read_file","params":{"file":"/a"}},{"tool_name":"read_file","params":{"file":"/b"}}]}`
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
	require.NotNil(t, loop.GetVariable(loopVarDirectToolBatch))
}

func TestToolScalarVerifier_ReturnsBeforeCompleteResponseEOF(t *testing.T) {
	tests := []struct {
		name       string
		actionType string
		payload    string
		verify     func(*reactloops.ReActLoop, *aicommon.Action) error
		stateKey   string
		want       string
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
			payload:    `{"@action":"require_tool","tool_require_payload":"grep","human_readable_thought":"prepare search"}`,
			verify:     loopAction_toolRequireAndCall.ActionVerifier,
			stateKey:   "tool_require_payload",
			want:       "grep",
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
				require.Equal(t, test.want, loop.Get(test.stateKey))
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
	request     *aicommon.ToolBatchRequest
	result      *aicommon.ToolBatchResult
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
				params = aitool.InvokeParams{"file": "/workspace/project.json"}
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
	assert.Equal(t, 2, op.GetExecutedToolCallCount())
	assert.Equal(t, 1, invoker.verifyCalls)
}

func TestToolBatchActionHandler_ZeroInvokeDoesNotVerifyOrMarkExecution(t *testing.T) {
	newFixture := func(result *aicommon.ToolBatchResult) (*toolBatchHandlerTestInvoker, *reactloops.ReActLoop, *reactloops.LoopActionHandlerOperator, *aicommon.ToolBatchRequest) {
		ctx := context.Background()
		manager, _, _ := newToolBatchTestManager(t)
		cfg := &aicommon.Config{AiToolManager: manager}
		task := newTestTask(ctx)
		invoker := &toolBatchHandlerTestInvoker{testInvoker: newTestInvoker(ctx), result: result}
		invoker.currentTask = task
		loop := reactloops.NewMinimalReActLoop(cfg, invoker)
		loop.SetCurrentTask(task)
		request := &aicommon.ToolBatchRequest{Calls: []aicommon.ToolBatchCall{
			{Index: 0, Mode: aicommon.ToolCallModeDirect, ToolName: "read_file"},
			{Index: 1, Mode: aicommon.ToolCallModeDirect, ToolName: "grep"},
		}}
		return invoker, loop, reactloops.NewActionHandlerOperator(task), request
	}

	t.Run("whole batch rejected at admission", func(t *testing.T) {
		result := &aicommon.ToolBatchResult{Outcomes: []aicommon.ToolCallOutcome{
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
		result := &aicommon.ToolBatchResult{DirectlyAnswer: true, Outcomes: []aicommon.ToolCallOutcome{
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
		result := &aicommon.ToolBatchResult{Outcomes: []aicommon.ToolCallOutcome{
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
}

func TestToolBatchActionHandler_CachesOnlySuccessfulChildren(t *testing.T) {
	ctx := context.Background()
	manager, readFile, grep := newToolBatchTestManager(t)
	cfg := &aicommon.Config{AiToolManager: manager, Timeline: aicommon.NewTimeline(nil, nil)}
	task := newTestTask(ctx)
	invoker := &toolBatchHandlerTestInvoker{testInvoker: newTestInvoker(ctx)}
	invoker.currentTask = task
	invoker.result = &aicommon.ToolBatchResult{Outcomes: []aicommon.ToolCallOutcome{
		{Index: 0, RequestedTool: readFile.Name, FinalTool: readFile.Name, Stage: aicommon.ToolCallStageDone, Result: &aitool.ToolResult{Name: readFile.Name, Success: true}},
		{Index: 1, RequestedTool: grep.Name, FinalTool: grep.Name, Stage: aicommon.ToolCallStageInvokeFailed, Result: &aitool.ToolResult{Name: grep.Name, Success: false, Error: "fixture failure"}},
	}}
	loop := reactloops.NewMinimalReActLoop(cfg, invoker)
	loop.SetCurrentTask(task)
	request := &aicommon.ToolBatchRequest{Calls: []aicommon.ToolBatchCall{
		{Index: 0, Mode: aicommon.ToolCallModeDirect, ToolName: readFile.Name, Params: aitool.InvokeParams{"file": "/workspace/go.mod"}},
		{Index: 1, Mode: aicommon.ToolCallModeDirect, ToolName: grep.Name, Params: aitool.InvokeParams{"pattern": "auth"}},
	}}

	op := reactloops.NewActionHandlerOperator(task)
	handleToolBatchActionResult(loop, ctx, invoker, request, invoker.result, nil, op)

	require.True(t, manager.IsRecentlyUsedTool(readFile.Name), "a successful child must remain available for future scalar direct calls")
	require.False(t, manager.IsRecentlyUsedTool(grep.Name), "a failed child must not be promoted into the direct-call cache")
	require.Equal(t, []string{readFile.Name}, manager.GetRecentToolNames())
	materials := aicommon.RenderTimelineFrozenOpen(cfg.Timeline)
	require.Contains(t, materials.PromotedOpen+materials.PromotedSemiDynamic1, readFile.Name)
	require.NotContains(t, materials.PromotedOpen+materials.PromotedSemiDynamic1, grep.Name)
	require.True(t, op.IsContinued())
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
			expectedParam: "file",
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
