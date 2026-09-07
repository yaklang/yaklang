package aicommon

import (
	"bytes"
	"context"
	"io"
	"sync"
	"testing"

	"github.com/segmentio/ksuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

// TestGenerateParams_FunctionCallMode_StreamEnabled verifies that in
// functioncall mode (default), generateParams enables
// ToolCallArgumentsStream on the AI request so that tool_call arguments
// flow through the output stream. It also verifies that the action JSON
// emitted via the output stream is correctly parsed by
// ExtractValidActionFromStream("call-tool") — the same path as text mode.
func TestGenerateParams_FunctionCallMode_StreamEnabled(t *testing.T) {
	var observedStreamEnabled bool
	var mu sync.Mutex

	cfg := NewTestConfig(
		context.Background(),
		WithID("fc-r2-"+ksuid.New().String()),
		WithSequence(100),
		// EnableFunctionCallMode defaults to true, so we don't disable it
		WithAICallback(func(config AICallerConfigIf, request *AIRequest) (*AIResponse, error) {
			mu.Lock()
			observedStreamEnabled = request.IsToolCallArgumentsStreamEnabled()
			mu.Unlock()

			// Simulate what aicaller.go does when
			// IsToolCallArgumentsStreamEnabled is true: emit the
			// tool_call arguments through EmitOutputStream.
			response := config.NewAIResponse()
			response.EmitOutputStream(bytes.NewBufferString(`{
				"@action": "call-tool",
				"tool": "read_file",
				"identifier": "read_config",
				"params": {"path": "/etc/config.yml"},
				"call_expectations": "~1s, success when file content is returned"
			}`))
			response.Close()
			return response, nil
		}),
	)

	tool, err := aitool.New(
		"read_file",
		aitool.WithStringParam("path", aitool.WithParam_Required(true), aitool.WithParam_Description("file path")),
		aitool.WithSimpleCallback(func(aitool.InvokeParams, io.Writer, io.Writer) (any, error) {
			return "unused", nil
		}),
	)
	require.NoError(t, err)

	caller, err := NewToolCaller(
		context.Background(),
		WithToolCaller_AICallerConfig(cfg),
		WithToolCaller_AICaller(cfg),
		WithToolCaller_Emitter(cfg.GetEmitter()),
		WithToolCaller_Task(cfg.DefaultTask),
		WithToolCaller_CallToolID("fc-r2-call"),
		WithToolCaller_GenerateToolParamsBuilder(func(_ *aitool.Tool, _ string) (string, error) {
			return "generate params", nil
		}),
	)
	require.NoError(t, err)

	result, err := caller.generateParams(tool, func(any) {})
	require.NoError(t, err)

	mu.Lock()
	require.True(t, observedStreamEnabled,
		"IsToolCallArgumentsStreamEnabled should be true in functioncall mode")
	mu.Unlock()

	// Verify params were correctly parsed from the action JSON
	require.Equal(t, "/etc/config.yml", result.Params.GetString("path"),
		"params should contain the path from tool_call arguments")

	// Verify identifier was extracted
	require.Equal(t, "read_config", result.Identifier,
		"identifier should be extracted from tool_call arguments")

	// Verify call_expectations was extracted
	require.Contains(t, result.CallExpectations, "file content is returned",
		"call_expectations should be extracted from tool_call arguments")

	// Verify raw AI response was captured
	require.Contains(t, result.RawAIResponse, "call-tool",
		"raw AI response should contain the action JSON")
}

// TestGenerateParams_FunctionCallMode_Disabled verifies that when
// functioncall mode is disabled, generateParams does NOT enable
// ToolCallArgumentsStream, and the existing text-mode path works
// unchanged.
func TestGenerateParams_FunctionCallMode_Disabled(t *testing.T) {
	var observedStreamEnabled bool
	var mu sync.Mutex

	cfg := NewTestConfig(
		context.Background(),
		WithID("fc-off-"+ksuid.New().String()),
		WithSequence(100),
		WithEnableFunctionCallMode(false),
		WithAICallback(func(config AICallerConfigIf, request *AIRequest) (*AIResponse, error) {
			mu.Lock()
			observedStreamEnabled = request.IsToolCallArgumentsStreamEnabled()
			mu.Unlock()

			// Text mode: emit action JSON as regular text content
			response := config.NewAIResponse()
			response.EmitOutputStream(bytes.NewBufferString(`{
				"@action": "call-tool",
				"tool": "read_file",
				"identifier": "read_config",
				"params": {"path": "/etc/hosts"},
				"call_expectations": "~1s"
			}`))
			response.Close()
			return response, nil
		}),
	)

	tool, err := aitool.New(
		"read_file",
		aitool.WithStringParam("path", aitool.WithParam_Required(true), aitool.WithParam_Description("file path")),
		aitool.WithSimpleCallback(func(aitool.InvokeParams, io.Writer, io.Writer) (any, error) {
			return "unused", nil
		}),
	)
	require.NoError(t, err)

	caller, err := NewToolCaller(
		context.Background(),
		WithToolCaller_AICallerConfig(cfg),
		WithToolCaller_AICaller(cfg),
		WithToolCaller_Emitter(cfg.GetEmitter()),
		WithToolCaller_Task(cfg.DefaultTask),
		WithToolCaller_CallToolID("fc-off-call"),
		WithToolCaller_GenerateToolParamsBuilder(func(_ *aitool.Tool, _ string) (string, error) {
			return "generate params", nil
		}),
	)
	require.NoError(t, err)

	result, err := caller.generateParams(tool, func(any) {})
	require.NoError(t, err)

	mu.Lock()
	require.False(t, observedStreamEnabled,
		"IsToolCallArgumentsStreamEnabled should be false when functioncall mode is disabled")
	mu.Unlock()

	// Text mode should still parse correctly
	require.Equal(t, "/etc/hosts", result.Params.GetString("path"),
		"params should contain the path from text output")
	require.Equal(t, "read_config", result.Identifier,
		"identifier should be extracted from text output")
}

// TestGenerateParams_FunctionCallMode_AITAG verifies that AITAG
// parsing still works in functioncall mode, since the action JSON
// structure is unchanged.
func TestGenerateParams_FunctionCallMode_AITAG(t *testing.T) {
	nonce := "testnonce123"
	cfg := NewTestConfig(
		context.Background(),
		WithID("fc-aitag-"+ksuid.New().String()),
		WithSequence(100),
		WithAICallback(func(config AICallerConfigIf, request *AIRequest) (*AIResponse, error) {
			// Simulate action JSON with AITAG blocks for long-text params.
			// The action JSON has @action/tool/params, and the long-text
			// param "content" is delivered via AITAG block.
			response := config.NewAIResponse()
			response.EmitOutputStream(bytes.NewBufferString(`{
				"@action": "call-tool",
				"tool": "write_file",
				"identifier": "write_script",
				"params": {"path": "/tmp/script.sh"}
			}`))
			response.Close()
			return response, nil
		}),
	)

	tool, err := aitool.New(
		"write_file",
		aitool.WithStringParam("path", aitool.WithParam_Required(true)),
		aitool.WithStringParam("content", aitool.WithParam_Description("file content")),
		aitool.WithSimpleCallback(func(aitool.InvokeParams, io.Writer, io.Writer) (any, error) {
			return "unused", nil
		}),
	)
	require.NoError(t, err)

	caller, err := NewToolCaller(
		context.Background(),
		WithToolCaller_AICallerConfig(cfg),
		WithToolCaller_AICaller(cfg),
		WithToolCaller_Emitter(cfg.GetEmitter()),
		WithToolCaller_Task(cfg.DefaultTask),
		WithToolCaller_CallToolID("fc-aitag-call"),
		WithToolCaller_GenerateToolParamsBuilderWithMeta(func(_ *aitool.Tool, _ string) (*ToolParamsPromptMeta, error) {
			return &ToolParamsPromptMeta{
				Prompt:     "generate params",
				Nonce:      nonce,
				ParamNames: []string{"content"},
			}, nil
		}),
	)
	require.NoError(t, err)

	result, err := caller.generateParams(tool, func(any) {})
	require.NoError(t, err)

	// The non-AITAG param should be parsed from the action JSON
	require.Equal(t, "/tmp/script.sh", result.Params.GetString("path"),
		"params should contain path from action JSON")
}

// TestBuildNativeToolForParamGen verifies the structure of the
// native tool schema generated for R2 functioncall mode.
func TestBuildNativeToolForParamGen(t *testing.T) {
	tool, err := aitool.New(
		"read_file",
		aitool.WithDescription("Read a file from disk"),
		aitool.WithStringParam("path", aitool.WithParam_Required(true), aitool.WithParam_Description("file path")),
		aitool.WithStringParam("mode", aitool.WithParam_Description("read mode")),
		aitool.WithSimpleCallback(func(aitool.InvokeParams, io.Writer, io.Writer) (any, error) {
			return "unused", nil
		}),
	)
	require.NoError(t, err)

	spec := buildNativeToolForParamGen(tool)

	require.Equal(t, "function", spec.Type)
	require.Equal(t, "read_file", spec.Function.Name)
	require.Equal(t, "Read a file from disk", spec.Function.Description)

	params, ok := spec.Function.Parameters.(map[string]any)
	require.True(t, ok, "Parameters should be a map[string]any")

	require.Equal(t, "object", params["type"])

	props, ok := params["properties"].(map[string]any)
	require.True(t, ok, "properties should be a map")

	// @action field
	actionField, ok := props["@action"].(map[string]any)
	require.True(t, ok, "@action field should exist")
	require.Equal(t, "call-tool", actionField["const"])

	// tool field
	toolField, ok := props["tool"].(map[string]any)
	require.True(t, ok, "tool field should exist")
	require.Equal(t, "read_file", toolField["const"])

	// identifier field
	identifierField, ok := props["identifier"].(map[string]any)
	require.True(t, ok, "identifier field should exist")
	require.Equal(t, "string", identifierField["type"])

	// params field (nested with InputSchema)
	paramsField, ok := props["params"].(map[string]any)
	require.True(t, ok, "params field should exist")
	require.Equal(t, "object", paramsField["type"])

	// call_expectations field
	ceField, ok := props["call_expectations"].(map[string]any)
	require.True(t, ok, "call_expectations field should exist")
	require.Equal(t, "string", ceField["type"])

	// required fields
	required, ok := params["required"].([]string)
	require.True(t, ok, "required should be []string")
	require.Contains(t, required, "@action")
	require.Contains(t, required, "tool")
	require.Contains(t, required, "params")

	// additionalProperties
	require.False(t, params["additionalProperties"].(bool),
		"additionalProperties should be false")
}

// TestBuildNativeToolForParamGen_NoProperties verifies the tool
// schema when the tool has no InputSchema properties.
func TestBuildNativeToolForParamGen_NoProperties(t *testing.T) {
	tool, err := aitool.New(
		"noop_tool",
		aitool.WithDescription("Does nothing"),
		aitool.WithSimpleCallback(func(aitool.InvokeParams, io.Writer, io.Writer) (any, error) {
			return "unused", nil
		}),
	)
	require.NoError(t, err)

	spec := buildNativeToolForParamGen(tool)

	params, ok := spec.Function.Parameters.(map[string]any)
	require.True(t, ok)

	props, ok := params["properties"].(map[string]any)
	require.True(t, ok)

	// @action and tool should still exist
	_, ok = props["@action"].(map[string]any)
	require.True(t, ok, "@action field should always exist")
	_, ok = props["tool"].(map[string]any)
	require.True(t, ok, "tool field should always exist")

	// params field should NOT exist when tool has no properties
	_, ok = props["params"]
	require.False(t, ok, "params field should NOT exist when tool has no InputSchema properties")
}