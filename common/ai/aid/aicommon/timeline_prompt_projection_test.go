package aicommon

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

func TestTimelinePromptProjectionFiltersOnlySystemBookkeeping(t *testing.T) {
	base := time.Date(2026, 7, 18, 10, 0, 0, 0, time.UTC)
	tl := NewTimeline(nil, nil)
	injectTimelineItem(tl, 1, base, &TextTimelineItem{ID: 1, Text: "[iteration]:\n[default]======== ReAct iteration 1 ========\nReason/Next-Step: keep-decision"})
	injectTimelineItem(tl, 2, base.Add(time.Second), &TextTimelineItem{ID: 2, Text: "[iteration]:\n[default]ReAct Iteration Done[1] max:100 continue to next iteration"})
	injectTimelineItem(tl, 3, base.Add(2*time.Second), &TextTimelineItem{ID: 3, Text: "[NEXT_MOVEMENTS]:\nDONE[finished]: applied"})
	injectTimelineItem(tl, 4, base.Add(3*time.Second), &TextTimelineItem{ID: 4, Text: "[evidence_ops]:\nUPSERT[evidence-1]: applied"})
	injectTimelineItem(tl, 5, base.Add(4*time.Second), &TextTimelineItem{ID: 5, Text: "[[NEXT_MOVEMENTS_ERROR]]:\nFAILED DOING[done]: redundant doing: todo already doing\nFAILED DONE[foreign]: todo belongs to another task scope"})
	directParams := &TextTimelineItem{ID: 6, Text: "[DIRECT_CALL_PARAMS]:\n{\"path\":\"KEEP_PARAMS\"}"}
	injectTimelineItem(tl, 6, base.Add(5*time.Second), directParams)
	toolResult := &aitool.ToolResult{ID: 7, Name: "opaque_tool", Success: true, Data: "KEEP_TOOL_RESULT"}
	injectTimelineItem(tl, 7, base.Add(6*time.Second), toolResult)

	raw := tl.Dump()
	prompt := tl.DumpForPrompt()
	require.Contains(t, raw, "ReAct Iteration Done")
	require.Contains(t, raw, "DONE[finished]")
	require.Contains(t, raw, "UPSERT[evidence-1]")
	require.Contains(t, raw, "redundant doing")

	require.Contains(t, prompt, "Reason/Next-Step: keep-decision")
	require.Contains(t, prompt, "todo belongs to another task scope")
	require.Contains(t, prompt, "KEEP_PARAMS")
	require.Contains(t, prompt, "KEEP_TOOL_RESULT")
	require.NotContains(t, prompt, "ReAct Iteration Done")
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

func TestTimelinePromptProjectionPreservesRawBucketTopology(t *testing.T) {
	base := time.Date(2026, 7, 18, 11, 0, 0, 0, time.UTC)
	tl := NewTimeline(nil, nil)
	for i := 1; i <= 8; i++ {
		category := "note"
		body := strings.Repeat("payload ", i*20)
		if i%2 == 0 {
			category = "NEXT_MOVEMENTS"
		}
		injectTimelineItem(tl, int64(i), base.Add(time.Duration(i)*time.Minute), &TextTimelineItem{
			ID: int64(i), Text: "[" + category + "]:\n" + body,
		})
	}

	raw := tl.GroupByMinutes(TimelineDumpDefaultIntervalMinutes).GetAllRenderable()
	projected := projectTimelineRenderableBlocksForPrompt(raw)
	require.Len(t, projected, len(raw))
	for i := range raw {
		rawBlock, rawOK := raw[i].(*TimelineIntervalBlock)
		projectedBlock, projectedOK := projected[i].(*TimelineIntervalBlock)
		require.Equal(t, rawOK, projectedOK)
		if !rawOK {
			continue
		}
		require.Equal(t, rawBlock.BucketStart, projectedBlock.BucketStart)
		require.Equal(t, rawBlock.BucketEnd, projectedBlock.BucketEnd)
		require.Equal(t, rawBlock.Open, projectedBlock.Open)
		require.Equal(t, rawBlock.SeqInBucket, projectedBlock.SeqInBucket)
		require.Equal(t, rawBlock.TotalInBucket, projectedBlock.TotalInBucket)
		require.Equal(t, rawBlock.StableNonce(), projectedBlock.StableNonce())
	}
}

func TestTimelinePromptProjectionKeepsRealAndDropsOnlyRedundantErrorLines(t *testing.T) {
	item := &TimelineItem{createdAt: time.Now(), value: &TextTimelineItem{
		ID:   9,
		Text: "[[NEXT_MOVEMENTS_ERROR]]:\nredundant done: todo already done\nmissing required field: id\nillegal op: explode",
	}}
	projected := projectTimelineItemForPrompt(item)
	require.NotNil(t, projected)
	require.NotSame(t, item, projected)
	require.NotContains(t, projected.String(), "todo already done")
	require.Contains(t, projected.String(), "missing required field: id")
	require.Contains(t, projected.String(), "illegal op: explode")

	onlyRedundant := &TimelineItem{createdAt: time.Now(), value: &TextTimelineItem{
		ID: 10, Text: "[NEXT_MOVEMENTS_ERROR]:\nFAILED DONE[x]: redundant done: todo already done",
	}}
	require.Nil(t, projectTimelineItemForPrompt(onlyRedundant))
}

func TestTimelineDumpRecentForPromptKeepsNewestCompleteItemsWithinBudget(t *testing.T) {
	base := time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC)
	tl := NewTimeline(nil, nil)
	injectTimelineItem(tl, 1, base, &TextTimelineItem{ID: 1, Text: "[note]:\nANCIENT " + strings.Repeat("old ", 3000)})
	injectTimelineItem(tl, 2, base.Add(time.Second), &TextTimelineItem{ID: 2, Text: "[NEXT_MOVEMENTS]:\nDROP_BOOKKEEPING"})
	injectTimelineItem(tl, 3, base.Add(2*time.Second), &TextTimelineItem{ID: 3, Text: "[note]:\nRECENT_FACT_ONE"})
	injectTimelineItem(tl, 4, base.Add(3*time.Second), &TextTimelineItem{ID: 4, Text: "[note]:\nRECENT_FACT_TWO"})

	const budget = 128
	prompt := tl.DumpRecentForPrompt(budget)
	require.LessOrEqual(t, MeasureTokens(prompt), budget)
	require.Contains(t, prompt, "<|TIMELINE_RECENT|>")
	require.Contains(t, prompt, "RECENT_FACT_ONE")
	require.Contains(t, prompt, "RECENT_FACT_TWO")
	require.NotContains(t, prompt, "ANCIENT")
	require.NotContains(t, prompt, "DROP_BOOKKEEPING")

	// Prompt projection must not modify the raw Timeline.
	raw := tl.Dump()
	require.Contains(t, raw, "ANCIENT")
	require.Contains(t, raw, "DROP_BOOKKEEPING")
}

func TestTimelineDumpRecentForPromptBoundsOversizedNewestItem(t *testing.T) {
	tl := NewTimeline(nil, nil)
	injectTimelineItem(tl, 1, time.Now(), &TextTimelineItem{
		ID: 1, Text: "[tool_result]:\nHEAD " + strings.Repeat("payload ", 5000) + " TAIL",
	})

	const budget = 96
	prompt := tl.DumpRecentForPrompt(budget)
	require.LessOrEqual(t, MeasureTokens(prompt), budget)
	require.Contains(t, prompt, "HEAD")
	require.Contains(t, prompt, "TAIL")
}

func TestTimelineBatchReducerPromptUsesProjectionWithoutRewritingToolData(t *testing.T) {
	tl := NewTimeline(nil, nil)
	toCompress := []*TimelineItem{
		{createdAt: time.Now(), value: &TextTimelineItem{ID: 1, Text: "[NEXT_MOVEMENTS]:\nDROP_REDUCER_BREADCRUMB"}},
		{createdAt: time.Now(), value: &aitool.ToolResult{ID: 2, Name: "opaque", Success: true, Data: "KEEP_REDUCER_TOOL_DATA"}},
		{createdAt: time.Now(), value: &TextTimelineItem{ID: 3, Text: "[DIRECT_CALL_PARAMS]:\nKEEP_REDUCER_DIRECT_PARAMS"}},
	}
	recentKeep := []*TimelineItem{
		{createdAt: time.Now(), value: &TextTimelineItem{ID: 4, Text: "[evidence_ops]:\nDROP_RECENT_EVIDENCE_BREADCRUMB"}},
		{createdAt: time.Now(), value: &TextTimelineItem{ID: 5, Text: "[reflection]:\nKEEP_RECENT_REFLECTION"}},
	}

	prompt := tl.renderBatchCompressPrompt(nil, toCompress, recentKeep, "PROJECTION")
	require.NotContains(t, prompt, "DROP_REDUCER_BREADCRUMB")
	require.NotContains(t, prompt, "DROP_RECENT_EVIDENCE_BREADCRUMB")
	require.Contains(t, prompt, "KEEP_REDUCER_TOOL_DATA")
	require.Contains(t, prompt, "KEEP_REDUCER_DIRECT_PARAMS")
	require.Contains(t, prompt, "KEEP_RECENT_REFLECTION")
}
