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

func TestTimelineCompressionPromotedSnapshotUsesOriginalItems(t *testing.T) {
	tl := NewTimeline(nil, nil)
	pushCompressionTool(t, tl, 1, "alpha", TimelinePromotedOperationUpsert, "SCHEMA_V1")
	pushCompressionTool(t, tl, 2, "alpha", TimelinePromotedOperationReuse, "")
	tl.FreezeAll()
	pushCompressionTool(t, tl, 3, "alpha", TimelinePromotedOperationUpsert, "SCHEMA_V2")
	pushCompressionTool(t, tl, 4, "alpha", TimelinePromotedOperationDelete, "")
	body := strings.Repeat("full original line ", 8000) + "SKILL_BODY_END"
	require.True(t, tl.PushSkillFileContent(5, "demo", "ref.md", body, SkillContentSourceManual, 1, 10, 20))
	require.True(t, tl.PushSkillFileContent(6, "demo", "ref.md", "SECOND_READ", SkillContentSourceManual, 1, 10, 20))
	tl.EnsureTaskUserInput("task", "PROTECTED_USER", func() int64 { return 7 })
	require.True(t, tl.PushPromotable(8, "future-kind", TimelinePromotedTargetSemiDynamic1,
		"future", TimelinePromotedOperationUpsert, "PROTECTED_UNKNOWN"))
	// A stale materialized cache cannot supply compression candidates.
	tl.mu.Lock()
	tl.promotedState.Entries[TimelinePromotedTargetSemiDynamic1][TimelinePromotedKindRecentTool]["alpha"].Payload = "STALE_STATE"
	tl.mu.Unlock()
	before, err := MarshalTimeline(tl)
	require.NoError(t, err)
	snapshot, err := tl.buildCompressionSnapshot()
	require.NoError(t, err)
	require.Equal(t, []int64{1, 2, 3, 4, 5, 6}, promotedSnapshotIDs(snapshot.PromotedItems))
	require.Equal(t, []int64{7, 8}, snapshot.ExactItemIDs)
	require.True(t, snapshot.PromotedItems[0].Frozen)
	require.False(t, snapshot.PromotedItems[2].Frozen)
	prompt, err := renderCompressionSummaryPrompt(snapshot)
	require.NoError(t, err)
	for _, marker := range []string{"SCHEMA_V1", "SCHEMA_V2", "SKILL_BODY_END", "SECOND_READ", "[REUSE] alpha", "[DELETE] alpha"} {
		require.Contains(t, prompt, marker)
	}
	require.NotContains(t, prompt, "STALE_STATE")
	require.NotContains(t, prompt, "\x00")
	require.Contains(t, prompt, "PROTECTED_USER")
	require.Contains(t, snapshot.InputText, "PROTECTED_UNKNOWN")
	require.Equal(t, MeasureTokens(snapshot.InputText), snapshot.InputTokens)
	after, err := MarshalTimeline(tl)
	require.NoError(t, err)
	require.Equal(t, before, after, "capture must not compact, freeze, or rebuild state")
	oldPrompt := renderCompressionPromotedItems(snapshot.PromotedItems)
	tl.mu.Lock()
	live, _ := tl.idToTimelineItem.Get(3)
	live.value.(*PromotableTimelineItem).Payload = "LATER_EDIT"
	tl.mu.Unlock()
	require.Equal(t, oldPrompt, renderCompressionPromotedItems(snapshot.PromotedItems), "snapshot must be detached")
}

func TestTimelineCompressionPromotedRetiresOnlyNamedItems(t *testing.T) {
	tl := NewTimeline(nil, nil)
	tl.PushText(1, "ordinary history")
	pushCompressionTool(t, tl, 2, "alpha", TimelinePromotedOperationUpsert, "OLD_SCHEMA")
	pushCompressionTool(t, tl, 3, "alpha", TimelinePromotedOperationUpsert, "CURRENT_SCHEMA")
	require.True(t, tl.PushSkillFileContent(4, "demo", "ref.md", "READ_A", SkillContentSourceAuto, 1, 1, 2))
	require.True(t, tl.PushSkillFileContent(5, "demo", "ref.md", "READ_B", SkillContentSourceAuto, 2, 2, 2))
	require.True(t, tl.PushPromotable(6, TimelinePromotedKindEvidence, TimelinePromotedTargetSemiDynamic1,
		"e1", TimelinePromotedOperationUpsert, `{"id":"e1","content":"OLD_EVIDENCE"}`))
	require.True(t, tl.PushPromotable(7, TimelinePromotedKindEvidence, TimelinePromotedTargetSemiDynamic1,
		"e1", TimelinePromotedOperationUpsert, `{"id":"e1","content":"CURRENT_EVIDENCE"}`))
	bindCompressionMock(t, tl, func(*AIRequest) (string, error) {
		return compressionMockWithRetire("summary", "2,4,6"), nil
	})
	result, err := tl.CompressOnce(compressionTestOptions())
	require.NoError(t, err)
	require.Equal(t, []int64{1, 2, 4, 6}, result.RetiredIDs)
	require.Equal(t, []int64{2, 4, 6}, result.RetiredPromotedEntryIDs)
	view := RenderTimelineFrozenOpen(tl)
	require.Contains(t, view.PromotedRecentTools, "CURRENT_SCHEMA")
	require.NotContains(t, view.PromotedRecentTools, "OLD_SCHEMA")
	require.Contains(t, view.SkillsContext, "READ_B")
	require.NotContains(t, view.SkillsContext, "READ_A")
	require.Contains(t, view.EvidenceSemiDynamic, "CURRENT_EVIDENCE")
	require.NotContains(t, view.EvidenceSemiDynamic, "OLD_EVIDENCE")
	raw, err := MarshalTimeline(tl)
	require.NoError(t, err)
	restored, err := UnmarshalTimeline(raw)
	require.NoError(t, err)
	require.Equal(t, view, RenderTimelineFrozenOpen(restored))
}

func TestTimelineCompressionPromotedDeletesExactIDs(t *testing.T) {
	for _, tc := range []struct {
		name, retire string
		ops          []string
		newOp        string
		wantRetired  []int64
	}{
		{"delete-source-keeps-reuse", "1", []string{"upsert", "reuse"}, "", []int64{1}},
		{"remove-source-and-reuse", "1,2", []string{"upsert", "reuse"}, "", []int64{1, 2}},
		{"delete-latest-keeps-old-schema", "2", []string{"upsert", "upsert"}, "", []int64{2}},
		{"delete-tombstone-keeps-source", "2", []string{"upsert", "delete"}, "", []int64{2}},
		{"remove-dead-chain", "1,2", []string{"upsert", "delete"}, "", []int64{1, 2}},
		{"later-reuse-does-not-cancel", "1", []string{"upsert"}, "reuse", []int64{1}},
		{"later-upsert-does-not-cancel", "1", []string{"upsert"}, "upsert", []int64{1}},
		{"later-delete-does-not-cancel", "1", []string{"upsert"}, "delete", []int64{1}},
		{"later-id-is-not-a-candidate", "1,2", []string{"upsert"}, "upsert", []int64{1}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tl := NewTimeline(nil, nil)
			for i, op := range tc.ops {
				payload := ""
				if op == TimelinePromotedOperationUpsert {
					payload = fmt.Sprintf("SCHEMA_%d", i)
				}
				pushCompressionTool(t, tl, int64(i+1), "alpha", op, payload)
			}
			var receipt TimelineCompressFreezeEvent
			tl.RegisterCompressFreezeCallback("actual-result", func(event TimelineCompressFreezeEvent) { receipt = event })
			bindCompressionMock(t, tl, func(*AIRequest) (string, error) {
				if tc.newOp != "" {
					payload := ""
					if tc.newOp == "upsert" {
						payload = "NEW_SCHEMA"
					}
					pushCompressionTool(t, tl, int64(len(tc.ops)+1), "alpha", tc.newOp, payload)
				}
				return compressionMockWithRetire("", tc.retire), nil
			})
			result, err := tl.CompressOnce(compressionTestOptions())
			require.NoError(t, err)
			require.Equal(t, tc.wantRetired, result.RetiredIDs)
			require.Equal(t, tc.wantRetired, result.RetiredPromotedEntryIDs)
			require.Equal(t, *result, receipt.Compression)
			retired := make(map[int64]bool)
			for _, id := range tc.wantRetired {
				retired[id] = true
			}
			for _, id := range tl.idToTimelineItem.Keys() {
				item, ok := tl.idToTimelineItem.Get(id)
				require.True(t, ok)
				require.Equal(t, retired[id], item.deleted, "item %d must only be retired when named", id)
			}
			if tc.newOp != "" {
				require.Contains(t, RenderTimelineFrozenOpen(tl).Open, strings.ToUpper(tc.newOp))
			}
		})
	}
}

func TestTimelineCompressionPromotedRebuildsFromRemainingItems(t *testing.T) {
	for _, tc := range []struct {
		kind, key, oldPayload, newPayload string
	}{
		{TimelinePromotedKindRecentTool, "alpha", "OLD_BODY", "NEW_BODY"},
		{TimelinePromotedKindEvidence, "e1", `{"id":"e1","content":"OLD_BODY"}`, `{"id":"e1","content":"NEW_BODY"}`},
		{TimelinePromotedKindSkillContent, skillPartIdentity(skillContentPayload{Name: "demo"}),
			`{"name":"demo","content":"OLD_BODY"}`, `{"name":"demo","content":"NEW_BODY"}`},
	} {
		for _, operation := range []string{TimelinePromotedOperationUpsert, TimelinePromotedOperationDelete} {
			t.Run(tc.kind+"/"+operation, func(t *testing.T) {
				tl := NewTimeline(nil, nil)
				require.True(t, tl.PushPromotable(1, tc.kind, TimelinePromotedTargetSemiDynamic1,
					tc.key, TimelinePromotedOperationUpsert, tc.oldPayload))
				require.True(t, tl.PushPromotable(2, tc.kind, TimelinePromotedTargetSemiDynamic1,
					tc.key, operation, tc.newPayload))
				bindCompressionMock(t, tl, func(*AIRequest) (string, error) {
					return compressionMockWithRetire("", "2"), nil
				})

				result, err := tl.CompressOnce(compressionTestOptions())
				require.NoError(t, err)
				require.Equal(t, []int64{2}, result.RetiredIDs)
				require.Equal(t, []int64{2}, result.RetiredPromotedEntryIDs)
				// Only item 2 was selected. Rebuild normally from surviving item 1,
				// whether item 2 was another body or a tombstone.
				entries := tl.effectivePromotedEntries(TimelinePromotedTargetSemiDynamic1, tc.kind)
				require.Len(t, entries, 1)
				require.Contains(t, entries[0].Payload, "OLD_BODY")
				require.NotContains(t, entries[0].Payload, "NEW_BODY")
				view := RenderTimelineFrozenOpen(tl)
				raw, err := MarshalTimeline(tl)
				require.NoError(t, err)
				restored, err := UnmarshalTimeline(raw)
				require.NoError(t, err)
				require.Equal(t, view, RenderTimelineFrozenOpen(restored))
			})
		}
	}
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
