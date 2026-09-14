package aicommon

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aispec"
)

func fixedParamTestCaller(t *testing.T, tool *aitool.Tool, functionCall bool, responses []string, nonce string, extra ...ConfigOption) (*ToolCaller, *atomic.Int32) {
	t.Helper()
	var calls atomic.Int32
	options := []ConfigOption{WithAITransactionAutoRetry(int64(len(responses))),
		WithEnableFunctionCallMode(functionCall),
		WithAIRetryWaitFunc(func(ctx context.Context, _ time.Duration) error { return ctx.Err() }),
		WithAICallback(func(config AICallerConfigIf, request *AIRequest) (*AIResponse, error) {
			index := int(calls.Add(1)) - 1
			require.Less(t, index, len(responses))
			if index > 0 {
				require.Contains(t, request.GetPrompt(), "content", "retry must explain the missing required parameter")
			}
			rsp := config.NewAIResponse()
			rsp.EmitOutputStream(strings.NewReader(responses[index]))
			rsp.Close()
			return rsp, nil
		})}
	cfg := NewTestConfig(context.Background(), append(options, extra...)...)
	caller, err := NewToolCaller(context.Background(), WithToolCaller_AICallerConfig(cfg), WithToolCaller_AICaller(cfg),
		WithToolCaller_Emitter(cfg.GetEmitter()), WithToolCaller_Task(cfg.DefaultTask),
		WithToolCaller_CallToolID("fixed-params"),
		WithToolCaller_GenerateToolParamsBuilderWithMeta(func(*aitool.Tool, string) (*ToolParamsPromptMeta, error) {
			return &ToolParamsPromptMeta{Prompt: "generate parameters for the selected tool", Nonce: nonce, ParamNames: tool.Params().Keys()}, nil
		}))
	require.NoError(t, err)
	return caller, &calls
}

func fixedParamWriteTool() *aitool.Tool {
	return aitool.NewWithoutCallback("write_file",
		aitool.WithStringParam("file", aitool.WithParam_Required(true)),
		aitool.WithStringParam("content", aitool.WithParam_Required(true)),
		aitool.WithBoolParam("force", aitool.WithParam_Default(false)),
	)
}

func TestGenerateParams_FixedToolProtocol(t *testing.T) {
	content := "first line\n中文: \\\"quoted\\\"\n{\"@action\":\"finish\"}\n"
	bare, err := json.Marshal(map[string]any{"file": "/tmp/example.txt", "content": content, "force": "true"})
	require.NoError(t, err)
	params := `{"file":"/tmp/example.txt","content":"true","force":true}`
	for _, test := range []struct {
		name, raw, content string
		valid              bool
	}{
		{"bare_logged_shape", string(bare), content, true},
		{"canonical", `{"@action":"call-tool","tool":"write_file","params":` + params + `}`, "true", true},
		{"alias", `{"action":"call-tool","tool":"write_file","params":` + params + `}`, "true", true},
		{"params_only", `{"params":` + params + `}`, "true", true},
		{"omitted_action", `{"tool":"write_file","params":` + params + `}`, "true", true},
		{"omitted_tool", `{"@action":"call-tool","params":` + params + `}`, "true", true},
		{"diagnostic_metadata", `{"@action":"call-tool","params":` + params + `,"diagnostic_marker":"raw-response-only"}`, "true", true},
		{"markdown", "```json\n" + string(bare) + "\n```", content, true},
		{"missing_content", `{"@action":"call-tool","tool":"write_file","params":{"file":"/tmp/example.txt","force":true}}`, "", false},
		{"wrong_tool_with_action", `{"@action":"call-tool","tool":"delete_file","params":` + params + `}`, "", false},
		{"wrong_tool_without_action", `{"tool":"delete_file","params":` + params + `}`, "", false},
		{"wrong_action", `{"@action":"finish","tool":"write_file","params":` + params + `}`, "", false},
		{"conflicting_action", `{"action":"call-tool","@action":"finish","params":` + params + `}`, "", false},
		{"null_params", `{"@action":"call-tool","params":null}`, "", false},
		{"array_params", `{"@action":"call-tool","params":[]}`, "", false},
		{"unknown_bare_field", `{"file":"/tmp/example.txt","content":"true","surprise":1}`, "", false},
		{"mixed_locations", `{"params":{"file":"/tmp/example.txt"},"content":"true"}`, "", false},
		{"nested_wrapper", `{"wrapper":{"params":` + params + `}}`, "", false},
		{"multiple_objects", string(bare) + "\n" + string(bare), "", false},
		{"truncated", strings.TrimSuffix(string(bare), "}"), "", false},
		{"duplicate_tool", `{"@action":"call-tool","tool":"delete_file","tool":"write_file","params":` + params + `}`, "", false},
		{"invalid_boolean", `{"file":"/tmp/example.txt","content":"true","force":"yes"}`, "", false},
	} {
		for _, functionCall := range []bool{false, true} {
			mode := "text"
			if functionCall {
				mode = "functioncall"
			}
			t.Run(test.name+"/"+mode, func(t *testing.T) {
				tool := fixedParamWriteTool()
				caller, calls := fixedParamTestCaller(t, tool, functionCall, []string{test.raw}, "")
				result, err := caller.generateParams(tool, func(any) {})
				require.EqualValues(t, 1, calls.Load())
				if !test.valid {
					require.Error(t, err)
					require.Nil(t, result)
					return
				}
				require.NoError(t, err)
				require.Equal(t, test.content, result.Params["content"])
				require.Equal(t, true, result.Params["force"])
				require.Equal(t, test.raw, result.RawAIResponse)
				require.Len(t, result.Params, 3)
			})
		}
	}
}

func TestGenerateParams_FixedToolRetryIsolation(t *testing.T) {
	for _, functionCall := range []bool{false, true} {
		tool := fixedParamWriteTool()
		first := `{"@action":"call-tool","identifier":"discard_me","call_expectations":"discard_me","params":{"file":"/tmp/old.txt","force":true,"stale":"discard_me"}}`
		second := `{"params":{"file":"/tmp/new.txt","content":"new content"}}`
		caller, calls := fixedParamTestCaller(t, tool, functionCall, []string{first, second}, "")
		result, err := caller.generateParams(tool, func(any) {})
		require.NoError(t, err)
		require.EqualValues(t, 2, calls.Load())
		require.Equal(t, aitool.InvokeParams{"file": "/tmp/new.txt", "content": "new content", "force": false}, result.Params)
		require.Empty(t, result.Identifier)
		require.Empty(t, result.CallExpectations)
		require.Equal(t, second, result.RawAIResponse)

		caller, calls = fixedParamTestCaller(t, tool, functionCall, []string{first, `{"params":{"content":"new content"}}`}, "")
		result, err = caller.generateParams(tool, func(any) {})
		require.Error(t, err, "two incomplete attempts must not combine into valid parameters")
		require.Nil(t, result)
		require.EqualValues(t, 2, calls.Load())
	}
}

func TestGenerateParams_FixedToolAITagBoundaries(t *testing.T) {
	const nonce = "current_nonce_42"
	const content = "literal JSON: {\"@action\":\"finish\"}\n中文 and \\ paths"
	const header = `{"params":{"file":"/tmp/example.txt"}}`
	block := "<|TOOL_PARAM_content_" + nonce + "|>\n" + content + "\n<|TOOL_PARAM_content_END_" + nonce + "|>"
	literal, err := json.Marshal(map[string]any{"file": "/tmp/example.txt", "content": block})
	require.NoError(t, err)
	for _, test := range []struct {
		name, raw, want string
		valid           bool
	}{
		{"external_block", header + "\n" + block, content, true},
		{"literal_tag_in_json", string(literal), block, true},
		{"empty_block_is_present", header + "\n<|TOOL_PARAM_content_" + nonce + "|>\n<|TOOL_PARAM_content_END_" + nonce + "|>", "", true},
		{"truncated_block", header + "\n<|TOOL_PARAM_content_" + nonce + "|>\npartial content", "", false},
		{"mismatched_end", header + "\n<|TOOL_PARAM_content_" + nonce + "|>\npartial\n<|TOOL_PARAM_content_END_old|>", "", false},
		{"duplicate_blocks", header + "\n" + block + "\n" + block, "", false},
		{"unknown_block", header + "\n<|TOOL_PARAM_unknown_" + nonce + "|>\ntext\n<|TOOL_PARAM_unknown_END_" + nonce + "|>", "", false},
		{"json_after_block", header + "\n" + block + "\n{}", "", false},
	} {
		for _, functionCall := range []bool{false, true} {
			mode := "text"
			if functionCall {
				mode = "functioncall"
			}
			t.Run(test.name+"/"+mode, func(t *testing.T) {
				tool := fixedParamWriteTool()
				caller, _ := fixedParamTestCaller(t, tool, functionCall, []string{test.raw}, nonce)
				result, err := caller.generateParams(tool, func(any) {})
				if !test.valid {
					require.Error(t, err)
					return
				}
				require.NoError(t, err)
				require.Equal(t, test.want, result.Params["content"])
			})
		}
	}
}

func TestGenerateParams_FixedToolReservedBusinessFields(t *testing.T) {
	tool := aitool.NewWithoutCallback("business",
		aitool.WithStringParam("action", aitool.WithParam_Required(true)),
		aitool.WithStringParam("tool", aitool.WithParam_Required(true)),
		aitool.WithStringParam("identifier", aitool.WithParam_Required(true)),
	)
	params := `{"action":"append","tool":"business value","identifier":"record"}`
	caller, _ := fixedParamTestCaller(t, tool, false, []string{`{"params":` + params + `,"identifier":"call_label"}`}, "")
	result, err := caller.generateParams(tool, func(any) {})
	require.NoError(t, err)
	require.Equal(t, aitool.InvokeParams{"action": "append", "tool": "business value", "identifier": "record"}, result.Params)
	require.Equal(t, "call_label", result.Identifier)
	caller, _ = fixedParamTestCaller(t, tool, false, []string{params}, "")
	_, err = caller.generateParams(tool, func(any) {})
	require.ErrorContains(t, err, "ambiguous bare parameter")

	// An optional business field named params must not be silently unwrapped
	// into a different, schema-valid object.
	tool = aitool.NewWithoutCallback("business", aitool.WithRawParam("params", map[string]any{"type": "object"}))
	for _, raw := range []string{`{"params":{"key":"value"}}`, `{"params":{"params":{"key":"value"}}}`} {
		caller, _ = fixedParamTestCaller(t, tool, false, []string{raw}, "")
		_, err = caller.generateParams(tool, func(any) {})
		require.ErrorContains(t, err, "ambiguous business parameter named params")
	}
	caller, _ = fixedParamTestCaller(t, tool, false, []string{`{"@action":"call-tool","params":{"params":{"key":"value"}}}`}, "")
	result, err = caller.generateParams(tool, func(any) {})
	require.NoError(t, err)
	require.Equal(t, map[string]any{"key": "value"}, result.Params["params"])
}

func TestGenerateParams_FixedToolNativeIdentity(t *testing.T) {
	for _, test := range []struct {
		name  string
		calls []*aispec.ToolCall
		valid bool
	}{
		{"selected", []*aispec.ToolCall{{ID: "one", Function: aispec.FuncReturn{Name: "write_file"}}}, true},
		{"wrong", []*aispec.ToolCall{{ID: "one", Function: aispec.FuncReturn{Name: "delete_file"}}}, false},
		{"fragmented", []*aispec.ToolCall{{ID: "one", Function: aispec.FuncReturn{Name: "write_"}}, {Function: aispec.FuncReturn{Name: "file"}}, {Function: aispec.FuncReturn{Name: "write_file"}}}, true},
		{"multiple", []*aispec.ToolCall{{ID: "one", Function: aispec.FuncReturn{Name: "write_file"}}, {Index: 1, ID: "two", Function: aispec.FuncReturn{Name: "write_file"}}}, false},
		{"missing_name", []*aispec.ToolCall{{ID: "one", Function: aispec.FuncReturn{Arguments: "{}"}}}, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			tool := fixedParamWriteTool()
			caller, _ := fixedParamTestCaller(t, tool, true, []string{`{}`}, "",
				WithAICallback(func(config AICallerConfigIf, request *AIRequest) (*AIResponse, error) {
					options := aispec.NewDefaultAIConfig(request.GetExtraSpecOpts()...)
					require.NotNil(t, options.ToolCallCallback)
					for _, call := range test.calls {
						options.ToolCallCallback([]*aispec.ToolCall{call})
					}
					rsp := config.NewAIResponse()
					rsp.EmitOutputStream(strings.NewReader(`{"file":"/tmp/example.txt","content":"safe text"}`))
					rsp.Close()
					return rsp, nil
				}))
			_, err := caller.generateParams(tool, func(any) {})
			if test.valid {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}

func TestFixedToolParamResponseWaitsForEOF(t *testing.T) {
	reader := newEOFGatedReader(`{"file":"/tmp/example.txt","content":"safe text"}`)
	finished := make(chan error, 1)
	go func() {
		_, err := extractFixedToolParamResponse(context.Background(), reader, fixedParamWriteTool())
		finished <- err
	}()
	<-reader.waiting
	select {
	case err := <-finished:
		t.Fatalf("parameter generation finished before EOF: %v", err)
	default:
	}
	close(reader.release)
	require.NoError(t, <-finished)
}

func TestGenerateParams_FixedToolEmptyAndUnionTypes(t *testing.T) {
	clock := aitool.NewWithoutCallback("clock")
	for _, raw := range []string{`{}`, `{"params":{}}`} {
		caller, _ := fixedParamTestCaller(t, clock, false, []string{raw}, "")
		result, err := caller.generateParams(clock, func(any) {})
		require.NoError(t, err)
		require.Empty(t, result.Params)
	}
	for _, raw := range []string{"", " ", `[]`, `null`, `{}` + `{}`} {
		caller, _ := fixedParamTestCaller(t, clock, false, []string{raw}, "")
		_, err := caller.generateParams(clock, func(any) {})
		require.Error(t, err)
	}
	tool := aitool.NewWithoutCallback("union", aitool.WithRawParam("value", map[string]any{"type": []string{"boolean", "string"}}))
	caller, _ := fixedParamTestCaller(t, tool, false, []string{`{"value":"false"}`}, "")
	result, err := caller.generateParams(tool, func(any) {})
	require.NoError(t, err)
	require.Equal(t, "false", result.Params["value"], "a schema that accepts strings must preserve strings")
}

func TestToolCaller_FixedParamsValidatedBeforeInvocation(t *testing.T) {
	var invoked atomic.Int32
	tool, err := aitool.New("write_file",
		aitool.WithStringParam("file", aitool.WithParam_Required(true)),
		aitool.WithStringParam("content", aitool.WithParam_Required(true)),
		aitool.WithSimpleCallback(func(params aitool.InvokeParams, _, _ io.Writer) (any, error) {
			invoked.Add(1)
			require.Equal(t, "safe text", params["content"])
			return "ok", nil
		}))
	require.NoError(t, err)
	caller, calls := fixedParamTestCaller(t, tool, false, []string{
		`{"@action":"call-tool","params":{"file":"/tmp/example.txt"}}`,
		`{"file":"/tmp/example.txt","content":"safe text"}`,
	}, "", WithAgreePolicy(AgreePolicyYOLO))
	result, _, err := caller.CallTool(tool)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, result.Success)
	require.EqualValues(t, 2, calls.Load())
	require.EqualValues(t, 1, invoked.Load())
}
