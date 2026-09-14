package aireact

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/schema"
)

func TestInvokeBlueprint_DoesNotEmitModelExchangeReferences(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	forgeName, cleanup := createMockForge(t)
	defer cleanup()
	var mu sync.Mutex
	var events []*schema.AiOutputEvent
	rawResponse := `{"@action":"call-ai-blueprint","params":{"query":"blueprint business query"},"diagnostic_marker":"raw-response-only"}`
	react, err := NewTestReAct(
		aicommon.WithContext(ctx),
		aicommon.WithAgreePolicy(aicommon.AgreePolicyYOLO),
		aicommon.WithEventHandler(func(event *schema.AiOutputEvent) {
			mu.Lock()
			defer mu.Unlock()
			events = append(events, event)
		}),
		aicommon.WithAICallback(func(config aicommon.AICallerConfigIf, request *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			require.Contains(t, request.GetPrompt(), forgeName)
			response := config.NewAIResponse()
			response.EmitOutputStream(strings.NewReader(rawResponse))
			response.Close()
			return response, nil
		}),
	)
	require.NoError(t, err)
	forge, params, err := react.invokeBlueprint(forgeName)
	require.NoError(t, err)
	require.Equal(t, forgeName, forge.ForgeName)
	require.Equal(t, "blueprint business query", params.GetString("query"))
	react.WaitForStream()
	mu.Lock()
	defer mu.Unlock()
	var output strings.Builder
	for _, event := range events {
		require.NotEqual(t, schema.EVENT_TYPE_REFERENCE_MATERIAL, event.Type,
			"blueprint prompts and raw responses are not reference materials")
		if event.Type == schema.EVENT_TYPE_STREAM && event.NodeId == "call-forge" {
			output.Write(event.StreamDelta)
		}
	}
	require.Contains(t, output.String(), "blueprint business query")
}
