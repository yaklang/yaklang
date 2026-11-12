package aicommon

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/schema"
)

func TestToolCallSummaryUsesCompletedToolResult(t *testing.T) {
	const sentinel = "summary-result-sentinel-31f8"

	var (
		mu        sync.Mutex
		summaries []string
	)
	cfg := NewTestConfig(
		context.Background(),
		WithWorkdir(t.TempDir()),
		WithAgreePolicy(AgreePolicyYOLO),
		WithEventHandler(func(event *schema.AiOutputEvent) {
			if event == nil || event.Type != schema.EVENT_TOOL_CALL_SUMMARY {
				return
			}
			var payload struct {
				Summary string `json:"summary"`
			}
			if err := json.Unmarshal(event.Content, &payload); err != nil {
				t.Errorf("decode tool summary event: %v", err)
				return
			}
			mu.Lock()
			summaries = append(summaries, payload.Summary)
			mu.Unlock()
		}),
	)
	tool, err := aitool.New(
		"summary_probe",
		aitool.WithSimpleCallback(func(aitool.InvokeParams, io.Writer, io.Writer) (any, error) {
			return sentinel, nil
		}),
	)
	require.NoError(t, err)

	caller, err := NewToolCaller(
		context.Background(),
		WithToolCaller_AICallerConfig(cfg),
		WithToolCaller_Emitter(cfg.GetEmitter()),
		WithToolCaller_Task(cfg.DefaultTask),
		WithToolCaller_Reason("verify populated summary"),
	)
	require.NoError(t, err)
	result, _, err := caller.CallToolWithExistedParams(tool, aitool.InvokeParams{})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, result.Success, result.Error)

	mu.Lock()
	defer mu.Unlock()
	require.Len(t, summaries, 1, "one settled tool call must emit exactly one summary")
	require.NotEqual(t, "null", summaries[0])
	require.NotEmpty(t, strings.TrimSpace(summaries[0]))
	require.Contains(t, summaries[0], sentinel)
}
