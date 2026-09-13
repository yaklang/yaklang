package aicommon

import (
	"context"
	"io"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/schema"
)

func TestToolCaller_ParamsDoNotEmitModelExchangeReferences(t *testing.T) {
	var mu sync.Mutex
	var events []*schema.AiOutputEvent
	const prompt = "private parameter-generation prompt"
	const rawResponse = `{"@action":"call-tool","params":{"id":42},"diagnostic_marker":"raw-response-only"}`
	cfg := NewTestConfig(context.Background(),
		WithEventHandler(func(event *schema.AiOutputEvent) {
			mu.Lock()
			defer mu.Unlock()
			events = append(events, event)
		}),
		WithAICallback(func(config AICallerConfigIf, request *AIRequest) (*AIResponse, error) {
			require.Contains(t, request.GetPrompt(), prompt)
			response := config.NewAIResponse()
			response.EmitOutputStream(strings.NewReader(rawResponse))
			response.Close()
			return response, nil
		}),
	)
	tool, err := aitool.New("reference_test_tool",
		aitool.WithIntegerParam("id", aitool.WithParam_Required(true)),
		aitool.WithSimpleCallback(func(aitool.InvokeParams, io.Writer, io.Writer) (any, error) { return "unused", nil }),
	)
	require.NoError(t, err)
	caller, err := NewToolCaller(context.Background(),
		WithToolCaller_AICallerConfig(cfg), WithToolCaller_AICaller(cfg),
		WithToolCaller_Emitter(cfg.GetEmitter()), WithToolCaller_Task(cfg.DefaultTask),
		WithToolCaller_CallToolID("reference-test-call"),
		WithToolCaller_GenerateToolParamsBuilder(func(*aitool.Tool, string) (string, error) { return prompt, nil }),
	)
	require.NoError(t, err)
	result, err := caller.generateParams(tool, func(any) {})
	require.NoError(t, err)
	require.Equal(t, int64(42), result.Params.GetInt("id"))
	require.Equal(t, rawResponse, result.RawAIResponse, "internal parameter recovery must retain the raw response")
	cfg.GetEmitter().WaitForStream()
	mu.Lock()
	defer mu.Unlock()
	var sawProgress bool
	for _, event := range events {
		require.NotEqual(t, schema.EVENT_TYPE_REFERENCE_MATERIAL, event.Type,
			"parameter-generation prompts and raw responses are not reference materials")
		if event.Type == schema.EVENT_TYPE_STREAM_START && event.NodeId == "generating-tool-call-params" {
			sawProgress = true
		}
	}
	require.True(t, sawProgress, "parameter-generation progress must remain visible")
}
