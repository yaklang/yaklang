package aicommon

import (
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"strings"
	"testing"
	"time"
)

func TestEvidenceTimelineProjectionFiltersLegacyBookkeeping(t *testing.T) {
	base := time.Date(2026, 7, 18, 10, 0, 0, 0, time.UTC)
	tl := NewTimeline(nil, nil)
	injectTimelineItem(tl, 1, base, &TextTimelineItem{ID: 1, Text: "[iteration]:\n[default]======== ReAct iteration 1 ========\nReason/Next-Step: keep-decision"})
	injectTimelineItem(tl, 2, base.Add(time.Second), &TextTimelineItem{ID: 2, Text: "[model_thinking]:\nlet me analyze the task and decide which tool to call"})
	injectTimelineItem(tl, 3, base.Add(2*time.Second), &TextTimelineItem{ID: 3, Text: "[TODO_DELTA]:\nDONE[finished]: applied"})
	injectTimelineItem(tl, 4, base.Add(3*time.Second), &TextTimelineItem{ID: 4, Text: "[evidence_ops]:\nUPSERT[evidence-1]: applied"})
	injectTimelineItem(tl, 5, base.Add(4*time.Second), &TextTimelineItem{ID: 5, Text: "[[TODO_DELTA_ERROR]]:\nFAILED DOING[done]: redundant doing: todo already doing\nFAILED DONE[foreign]: todo belongs to another task scope"})
	directParams := &TextTimelineItem{ID: 6, Text: "[DIRECT_CALL_PARAMS]:\n{\"path\":\"KEEP_PARAMS\"}"}
	injectTimelineItem(tl, 6, base.Add(5*time.Second), directParams)
	toolResult := &aitool.ToolResult{ID: 7, Name: "opaque_tool", Success: true, Data: "KEEP_TOOL_RESULT"}
	injectTimelineItem(tl, 7, base.Add(6*time.Second), toolResult)

	raw := tl.Dump()
	prompt := tl.DumpForPrompt()
	require.Contains(t, raw, "DONE[finished]")
	require.Contains(t, raw, "UPSERT[evidence-1]")
	require.Contains(t, raw, "redundant doing")

	require.Contains(t, raw, "let me analyze the task and decide which tool to call")
	require.Contains(t, prompt, "Reason/Next-Step: keep-decision")
	require.NotContains(t, prompt, "let me analyze the task and decide which tool to call")
	require.Contains(t, prompt, "todo belongs to another task scope")
	require.Contains(t, prompt, "KEEP_PARAMS")
	require.Contains(t, prompt, "KEEP_TOOL_RESULT")
	require.NotContains(t, prompt, "DONE[finished]")
	require.NotContains(t, prompt, "UPSERT[evidence-1]")
	require.NotContains(t, prompt, "redundant doing")

	// Opaque values and unaffected text remain the exact same objects. The
	// projector therefore cannot rewrite tool results or DIRECT_CALL_PARAMS.
	toolItem, _ := tl.idToTimelineItem.Get(7)
	require.Same(t, toolItem, projectTimelineItemForPrompt(toolItem))
	paramsItem, _ := tl.idToTimelineItem.Get(6)
	require.Same(t, paramsItem, projectTimelineItemForPrompt(paramsItem))
	require.Same(t, toolResult, projectTimelineItemForPrompt(toolItem).GetValue())
	require.Same(t, directParams, projectTimelineItemForPrompt(paramsItem).GetValue())
}

func TestEvidenceLegacyReceiptDoesNotReplayBodyOrShrinkCache(t *testing.T) {
	for _, content := range []string{"[id: fact]\nLEGACY_DUPLICATE_BODY", "LEGACY_DUPLICATE_BODY"} {
		raw := &TimelineItem{createdAt: time.Now(), value: &TextTimelineItem{ID: 1, Text: "[session_evidence_saved]:\n" + content, ShrinkResult: "LEGACY_DUPLICATE_BODY"}}
		projected := projectTimelineItemForPrompt(raw)
		require.NotContains(t, projected.String(), "LEGACY_DUPLICATE_BODY")
		require.NotContains(t, selectShrunkContent(projected), "LEGACY_DUPLICATE_BODY")
		require.Contains(t, raw.String(), "LEGACY_DUPLICATE_BODY", "raw user audit stays intact")
		if strings.HasPrefix(content, "[id:") {
			require.Contains(t, projected.String(), "[id: fact] saved.")
		}
	}
}
