package aicommon

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func compressionMockWithRetire(summary, retire string) string {
	payload, _ := json.Marshal(map[string]any{"@action": "timeline-summary", "summary": summary,
		"ratain_timeline_item_range": "", "retire_promoted_entries": retire, "memory_entities": []any{}})
	return string(payload)
}

func requireCompressionPromotedEntry(t testing.TB, prompt string, id int64, metadata, body string) {
	t.Helper()
	entry := fmt.Sprintf("========== RETIRE_ID %d ==========\n%s\n%s\n========== END RETIRE_ID %d ==========",
		id, metadata, strings.TrimRight(body, "\n"), id)
	require.Equal(t, 1, strings.Count(prompt, entry), "item %d must associate its ID and metadata with its complete original body", id)
}

func pushCompressionTool(t *testing.T, tl *Timeline, id int64, key, operation, payload string) {
	t.Helper()
	require.True(t, tl.PushPromotable(id, TimelinePromotedKindRecentTool, TimelinePromotedTargetSemiDynamic1,
		key, operation, payload))
}

func promotedSnapshotIDs(items []timelineCompressionPromotedItem) []int64 {
	var ids []int64
	for _, item := range items {
		ids = append(ids, item.ID)
	}
	return ids
}

func TestTimelineCompressionPromotedSelectionDefaultsToKeep(t *testing.T) {
	snapshot := &timelineCompressionSnapshot{
		Items:        []timelineCompressionSnapshotItem{{ID: 1, PromptText: "ordinary"}},
		ExactItemIDs: []int64{3},
		PromotedItems: []timelineCompressionPromotedItem{{
			timelineCompressionSnapshotItem: timelineCompressionSnapshotItem{ID: 2},
		}},
	}
	for _, raw := range []any{nil, "", "bad", "2-3", 2, []any{"2"}, "1,3,99"} {
		require.Empty(t, parsePromotedRetirementSelection(snapshot, raw), "raw=%v", raw)
	}
	require.Equal(t, []int64{2}, parsePromotedRetirementSelection(snapshot, "2,2,bad,99"))
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
