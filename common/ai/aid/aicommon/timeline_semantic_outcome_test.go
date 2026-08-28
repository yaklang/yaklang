package aicommon

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

func TestEmergencySummaryPreservesSemanticExecutionFailure(t *testing.T) {
	timeline := NewTimeline(nil, nil)
	toolResult := &aitool.ToolResult{
		ID:      7001,
		Name:    "bash",
		Success: true,
		Data: &aitool.ToolExecutionResult{Result: map[string]any{
			"exit_code":          7,
			"exit_code_accepted": false,
		}},
	}
	timeline.PushToolResult(toolResult)
	item, ok := timeline.idToTimelineItem.Get(toolResult.ID)
	require.True(t, ok)

	summary := timeline.createEmergencySummary(item, toolResult.ID)
	require.Contains(t, summary, "tool:bash execution-failed")
	require.Contains(t, summary, "exit_code=7")
	require.NotContains(t, summary, "tool:bash completed")
}

func TestStructuredCompressedMemoryKeepsOpenFailuresOutOfCompletedWork(t *testing.T) {
	action, err := ExtractValidActionFromStream(context.Background(), strings.NewReader(`{
		"@action":"timeline-reducer",
		"key_findings":["confirmed fact"],
		"completed_work":"verified branch completed",
		"open_failures":"branch-2 exit_code=7; exit_code_accepted=false; retry required",
		"failed_and_resolved":"branch-1 failed then passed an independent verification"
	}`), "timeline-reducer")
	require.NoError(t, err)

	memory := buildStructuredCompressedMemory(action)
	require.Contains(t, memory, "## Open Failures\nbranch-2 exit_code=7")
	require.Contains(t, memory, "## Completed Work\nverified branch completed")
	require.Less(t, strings.Index(memory, "## Open Failures"), strings.Index(memory, "## Completed Work"))

	// Low-priority sections may be removed under pressure, but unresolved
	// failures must remain represented as control-critical state.
	compressed := enforceOutputTokenBudget(memory+"\n\n## Discarded\n"+strings.Repeat("noise ", 400), int64(MeasureTokens(memory)+10))
	require.Contains(t, compressed, "## Open Failures")
	require.Contains(t, compressed, "exit_code_accepted=false")
}

func TestBatchCompressPromptDoesNotReinjectExistingCompressedHead(t *testing.T) {
	timeline := NewTimeline(nil, nil)
	timeline.compressedHead = &TimelineCompressedHead{Text: "OLD_HEAD_MUST_NOT_BE_SENT_TO_REDUCER"}
	item := &TimelineItem{value: &TextTimelineItem{Text: "new historical item"}}

	prompt := timeline.renderBatchCompressPrompt([]*TimelineItem{item}, nil, "NOHEAD", 10, 100)
	require.NotEmpty(t, prompt)
	require.NotContains(t, prompt, "OLD_HEAD_MUST_NOT_BE_SENT_TO_REDUCER")
	require.NotContains(t, prompt, "CURRENT_COMPRESSED_HEAD")
	require.Contains(t, prompt, "open_failures")
}
