package aicommon

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

func pushCompressionTool(t *testing.T, tl *Timeline, id int64, key, operation, payload string) {
	t.Helper()
	require.True(t, tl.PushPromotable(id, TimelinePromotedKindRecentTool, TimelinePromotedTargetSemiDynamic1,
		key, operation, payload))
}

func TestTimelineCompressionPromotedSelectionDefaultsToKeep(t *testing.T) {
	decision := &CompressionDecision{snapshot: &timelineCompressionSnapshot{
		Items:        []timelineCompressionSnapshotItem{{ID: 1, PromptText: "ordinary"}},
		ExactItemIDs: []int64{3},
		PromotedItems: []timelineCompressionPromotedItem{{
			timelineCompressionSnapshotItem: timelineCompressionSnapshotItem{ID: 2},
			Kind:                            TimelinePromotedKindEvidence, Key: "e1",
		}},
	}}
	decision.buildGuide()
	action := NewSimpleAction(TimelineCompressionAction, aitool.InvokeParams{"summary": "summary", "retain_timeline": ""})
	require.NoError(t, decision.Validate(action))
	require.Empty(t, decision.output.RetiredPromotedEntries)
	for _, raw := range []any{2, []any{2}, []any{"unknown"}} {
		action := NewSimpleAction(TimelineCompressionAction, aitool.InvokeParams{
			"summary": "summary", "retain_timeline": "", "retire_evidence_ids": raw,
		})
		require.Error(t, decision.Validate(action), "raw=%v", raw)
	}
}

func TestTimelineCompressionCompactionPreservesEffectiveToolsAndEvidence(t *testing.T) {
	tl := NewTimeline(nil, nil)
	pushCompressionTool(t, tl, 1, "alpha", "upsert", "OLD")
	pushCompressionTool(t, tl, 2, "alpha", "reuse", "")
	pushCompressionTool(t, tl, 3, "alpha", "upsert", "CURRENT")
	pushCompressionTool(t, tl, 4, "alpha", "reuse", "")
	pushCompressionTool(t, tl, 5, "beta", "upsert", "BETA")
	pushCompressionTool(t, tl, 6, "alpha", "reuse", "")
	tl.FreezeAll()
	pushCompressionTool(t, tl, 7, "beta", "delete", "")
	require.True(t, tl.PushPromotable(8, TimelinePromotedKindEvidence, TimelinePromotedTargetSemiDynamic1,
		"e1", "upsert", `{"id":"e1","content":"obsolete"}`))
	require.True(t, tl.PushPromotable(9, TimelinePromotedKindEvidence, TimelinePromotedTargetSemiDynamic1,
		"e1", "upsert", `{"id":"e1","content":"current"}`))
	toolsBefore := tl.effectivePromotedEntries(TimelinePromotedTargetSemiDynamic1, TimelinePromotedKindRecentTool)
	storeBefore, _ := tl.evidenceStore()
	_, acquired := tl.tryBeginCompression()
	require.True(t, acquired)
	commit, err := tl.foldTimelineWithoutAI(context.Background())
	tl.finishCompression([]*timelineCompressionCommit{commit})
	require.NoError(t, err)
	require.NotNil(t, commit)
	result := commit.Result
	require.Equal(t, []int64{1, 2, 4, 5, 7, 8}, result.RetiredIDs)
	require.Equal(t, toolsBefore, tl.effectivePromotedEntries(TimelinePromotedTargetSemiDynamic1, TimelinePromotedKindRecentTool))
	storeAfter, _ := tl.evidenceStore()
	require.Equal(t, storeBefore, storeAfter)
	require.Empty(t, RenderTimelineFrozenOpen(tl).Open)
	require.Contains(t, RenderTimelineFrozenOpen(tl).PromotedRecentTools, "CURRENT")
	raw, err := MarshalTimeline(tl)
	require.NoError(t, err)
	restored, err := UnmarshalTimeline(raw)
	require.NoError(t, err)
	require.Equal(t, RenderTimelineFrozenOpen(tl), RenderTimelineFrozenOpen(restored))
	_, acquired = tl.tryBeginCompression()
	require.True(t, acquired)
	again, err := tl.foldTimelineWithoutAI(context.Background())
	tl.finishCompression([]*timelineCompressionCommit{again})
	require.NoError(t, err)
	require.Nil(t, again, "already tidy journal must be a no-op")
}
