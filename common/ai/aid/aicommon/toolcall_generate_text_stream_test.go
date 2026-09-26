package aicommon

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aispec"
)

func TestTextStreamGenerateParamsPreservesActionProtocol(t *testing.T) {
	tool := aitool.NewWithoutCallback("read_file",
		aitool.WithStringParam("path", aitool.WithParam_Required(true)))
	cfg := NewTestConfig(context.Background(), WithEnableFunctionCallMode(false),
		WithAICallback(func(config AICallerConfigIf, request *AIRequest) (*AIResponse, error) {
			require.Equal(t, "text-mode prompt", request.GetPrompt())
			require.False(t, request.IsToolCallArgumentsStreamEnabled())
			require.Empty(t, aispec.NewDefaultAIConfig(request.GetExtraSpecOpts()...).Tools)
			response := config.NewAIResponse()
			response.EmitOutputStream(bytes.NewBufferString(`{"@action":"call-tool","tool":"read_file","params":{"path":"/tmp/text"}}`))
			response.Close()
			return response, nil
		}),
	)
	caller, err := NewToolCaller(context.Background(),
		WithToolCaller_AICallerConfig(cfg), WithToolCaller_AICaller(cfg),
		WithToolCaller_Emitter(cfg.GetEmitter()), WithToolCaller_Task(cfg.DefaultTask),
		WithToolCaller_GenerateToolParamsBuilderWithMeta(func(*aitool.Tool, string) (*ToolParamsPromptMeta, error) {
			return &ToolParamsPromptMeta{Prompt: "text-mode prompt"}, nil
		}),
	)
	require.NoError(t, err)
	result, err := caller.generateParams(tool, func(any) {})
	require.NoError(t, err)
	require.Equal(t, "/tmp/text", result.Params.GetString("path"))
}
