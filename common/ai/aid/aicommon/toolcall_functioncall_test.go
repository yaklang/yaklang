package aicommon

import (
	"bytes"
	"context"
	"io"
	"testing"

	"github.com/segmentio/ksuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

func TestGenerateParams_TextStream_AITAG(t *testing.T) {
	nonce := "testnonce123"
	cfg := NewTestConfig(
		context.Background(),
		WithID("text-aitag-"+ksuid.New().String()),
		WithEnableFunctionCallMode(false),
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
