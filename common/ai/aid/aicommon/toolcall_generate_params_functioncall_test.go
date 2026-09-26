package aicommon

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aispec"
)

func TestFunctionCallGenerateParamsSubmissionAndAbandonment(t *testing.T) {
	tool := aitool.NewWithoutCallback("read_file",
		aitool.WithStringParam("path", aitool.WithParam_Required(true)))
	for _, tc := range []struct {
		name      string
		call      bool
		wantError bool
	}{
		{name: "submits one native call", call: true},
		{name: "ordinary response abandons once", wantError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			attempts := 0
			cfg := NewTestConfig(context.Background(), WithEnableFunctionCallMode(true),
				WithAITransactionAutoRetry(2),
				WithAICallback(func(config AICallerConfigIf, request *AIRequest) (*AIResponse, error) {
					attempts++
					require.False(t, request.IsToolCallArgumentsStreamEnabled())
					options := aispec.NewDefaultAIConfig(request.GetExtraSpecOpts()...)
					require.Empty(t, options.Tools, "selected business tool must not be injected into tools")
					require.NotNil(t, options.ToolCallCallback)
					response := config.NewAIResponse()
					if tc.call {
						options.ToolCallCallback([]*aispec.ToolCall{{
							ID: "call-1", Type: "function",
							Function: aispec.FuncReturn{Name: submitToolParamsFunctionName,
								Arguments: `{"params":{"path":"/tmp/report"},"identifier":"read_report"}`},
						}})
					} else {
						response.EmitReasonStream(bytes.NewBufferString("input path cannot be inferred"))
						response.EmitOutputStream(bytes.NewBufferString("Please ask the user for the path."))
					}
					response.Close()
					return response, nil
				}),
			)
			caller, err := NewToolCaller(context.Background(),
				WithToolCaller_AICallerConfig(cfg), WithToolCaller_AICaller(cfg),
				WithToolCaller_Emitter(cfg.GetEmitter()), WithToolCaller_Task(cfg.DefaultTask),
				WithToolCaller_FunctionCallParamsPromptBuilder(func(*aitool.Tool, string) (string, error) {
					return "native prompt", nil
				}),
			)
			require.NoError(t, err)
			result, err := caller.generateParams(tool, func(any) {})
			require.Equal(t, 1, attempts, "a deliberate abandonment must not be retried")
			if tc.wantError {
				var abandoned *ToolParamGenerationAbandonedError
				require.ErrorAs(t, err, &abandoned)
				require.Contains(t, abandoned.Reason, "cannot be inferred")
				require.Contains(t, abandoned.Content, "ask the user")
				require.Nil(t, result)
				return
			}
			require.NoError(t, err)
			require.Equal(t, "/tmp/report", result.Params.GetString("path"))
			require.Equal(t, "read_report", result.Identifier)
			require.Contains(t, result.RawAIResponse, `"params"`)
		})
	}
}

func TestNativeParamSubmissionRejectsOtherOrMultipleCalls(t *testing.T) {
	for _, calls := range [][]*aispec.ToolCall{
		{{Function: aispec.FuncReturn{Name: "other_tool", Arguments: `{}`}}},
		{
			{Index: 0, ID: "one", Function: aispec.FuncReturn{Name: submitToolParamsFunctionName, Arguments: `{}`}},
			{Index: 1, ID: "two", Function: aispec.FuncReturn{Name: submitToolParamsFunctionName, Arguments: `{}`}},
		},
	} {
		collector := &nativeParamSubmission{}
		collector.observe(calls)
		_, _, err := collector.snapshot()
		require.Error(t, err)
	}
}
