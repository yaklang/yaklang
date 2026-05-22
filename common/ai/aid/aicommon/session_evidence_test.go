package aicommon

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestEvidenceStore_ApplyOperationsAtMetadataAndLegacyUnmarshal(t *testing.T) {
	store := NewEvidenceStore()
	store.ApplyOperationsAt([]EvidenceOperation{{Op: "add", ID: "a", Content: "first"}}, 100)
	require.Len(t, store.Items, 1)
	require.Equal(t, int64(100), store.Items[0].CreatedUnix)
	require.Equal(t, int64(100), store.Items[0].UpdatedUnix)

	store.ApplyOperationsAt([]EvidenceOperation{{Op: "update", ID: "a", Content: "second"}}, 120)
	require.Len(t, store.Items, 1)
	require.Equal(t, int64(100), store.Items[0].CreatedUnix)
	require.Equal(t, int64(120), store.Items[0].UpdatedUnix)
	require.Equal(t, "second", store.Items[0].Content)

	store.ApplyOperationsAt([]EvidenceOperation{{Op: "delete", ID: "a"}}, 130)
	require.Empty(t, store.Items)

	legacy := UnmarshalEvidenceStore(`{"items":[{"id":"legacy-id","content":"legacy content"}]}`)
	require.Len(t, legacy.Items, 1)
	require.Equal(t, int64(1), legacy.Items[0].CreatedUnix)
	require.Equal(t, int64(1), legacy.Items[0].UpdatedUnix)
}

func TestRenderSessionEvidenceFrozenOpen_SplitByFrozenTime(t *testing.T) {
	store := &EvidenceStore{
		Items: []EvidenceItem{
			{ID: "old", Content: "old evidence", CreatedUnix: 10, UpdatedUnix: 10},
			{ID: "new", Content: "new evidence", CreatedUnix: 30, UpdatedUnix: 30},
		},
	}
	state := NewSessionEvidenceRenderState()
	blocks := RenderSessionEvidenceFrozenOpen(state, store, 20)
	require.Contains(t, blocks.Frozen, "[id: old]")
	require.NotContains(t, blocks.Frozen, "[id: new]")
	require.Contains(t, blocks.Open, "[id: new]")
	require.NotContains(t, blocks.Open, "[id: old]")
}

func TestRenderSessionEvidenceFrozenOpen_UpdateFrozenIDEmitsOpenOverride(t *testing.T) {
	store := &EvidenceStore{
		Items: []EvidenceItem{
			{ID: "same", Content: "before", CreatedUnix: 10, UpdatedUnix: 10},
		},
	}
	state := NewSessionEvidenceRenderState()
	first := RenderSessionEvidenceFrozenOpen(state, store, 20)

	store.Items[0].Content = "after"
	store.Items[0].UpdatedUnix = 30
	second := RenderSessionEvidenceFrozenOpen(state, store, 20)

	require.Equal(t, first.Frozen, second.Frozen)
	require.Contains(t, second.Open, "[id: same]")
	require.Contains(t, second.Open, "[OVERRIDE]")
	require.Contains(t, second.Open, "after")
}

func TestRenderSessionEvidenceFrozenOpen_DeleteFrozenIDEmitsTombstone(t *testing.T) {
	store := &EvidenceStore{
		Items: []EvidenceItem{
			{ID: "gone", Content: "frozen body", CreatedUnix: 10, UpdatedUnix: 10},
		},
	}
	state := NewSessionEvidenceRenderState()
	first := RenderSessionEvidenceFrozenOpen(state, store, 20)

	store.Items = nil
	second := RenderSessionEvidenceFrozenOpen(state, store, 20)
	require.Equal(t, first.Frozen, second.Frozen)
	require.Contains(t, second.Open, "[id: gone]")
	require.Contains(t, second.Open, "[TOMBSTONE]")

	advanced := RenderSessionEvidenceFrozenOpen(state, store, 40)
	require.Empty(t, advanced.Frozen)
	require.Empty(t, advanced.Open)
}

func TestSessionEvidencePromptPlacement_FrozenAndOpenSections(t *testing.T) {
	baseTime := time.Date(2026, 5, 2, 10, 0, 0, 0, time.UTC)
	timeline := NewTimeline(nil, nil)
	injectTimelineItem(timeline, 1, baseTime.Add(30*time.Second), makeToolResult(1, "scan", true, "scan-ok"))
	injectTimelineItem(timeline, 2, baseTime.Add(4*time.Minute), makeToolResult(2, "verify", true, "verify-ok"))

	cfg := NewConfig(context.Background())
	cfg.Timeline = timeline
	store := &EvidenceStore{
		Items: []EvidenceItem{
			{
				ID:          "old",
				Content:     "frozen evidence",
				CreatedUnix: baseTime.Add(30 * time.Second).Unix(),
				UpdatedUnix: baseTime.Add(30 * time.Second).Unix(),
			},
			{
				ID:          "new",
				Content:     "open evidence",
				CreatedUnix: baseTime.Add(4 * time.Minute).Unix(),
				UpdatedUnix: baseTime.Add(4 * time.Minute).Unix(),
			},
		},
	}
	cfg.GetSessionPromptState().SetSessionEvidence(store.Marshal())

	frozenOpen := BuildPromptFrozenOpenMaterials(cfg, "nsevid")
	materials := &PromptMaterials{
		TaskInstruction: "instruction",
		Schema:          `{"type":"object"}`,
	}
	ApplyPromptFrozenOpenMaterials(materials, frozenOpen)

	prompt, err := NewDefaultPromptPrefixBuilder().AssemblePromptWithDynamicSection(
		materials,
		"session-evidence-placement-dynamic",
		"dynamic",
		nil,
		"nsevid",
	)
	require.NoError(t, err)

	frozenStartIdx := strings.Index(prompt, "<|AI_CACHE_FROZEN_")
	frozenEndIdx := strings.Index(prompt, "<|AI_CACHE_FROZEN_END_")
	frozenEvidenceIdx := strings.Index(prompt, "<|SESSION_EVIDENCE_FROZEN_")
	timelineOpenIdx := strings.Index(prompt, "<|PROMPT_SECTION_timeline-open|>")
	openEvidenceIdx := strings.Index(prompt, "<|SESSION_EVIDENCE_nsevid|>")
	require.NotEqual(t, -1, frozenStartIdx)
	require.NotEqual(t, -1, frozenEndIdx)
	require.NotEqual(t, -1, frozenEvidenceIdx)
	require.NotEqual(t, -1, timelineOpenIdx)
	require.NotEqual(t, -1, openEvidenceIdx)
	require.Less(t, frozenStartIdx, frozenEvidenceIdx)
	require.Less(t, frozenEvidenceIdx, frozenEndIdx)
	require.Less(t, frozenEndIdx, timelineOpenIdx)
	require.Less(t, timelineOpenIdx, openEvidenceIdx)
}

func TestSessionPromptState_GetSessionEvidenceFrozenOpenBlocksPrunesFrozenStateWithBudgetTrim(t *testing.T) {
	state := NewSessionPromptState()
	huge := strings.Repeat("very long evidence ", 20000)
	store := &EvidenceStore{
		Items: []EvidenceItem{
			{ID: "old", Content: huge, CreatedUnix: 1, UpdatedUnix: 1},
			{ID: "new", Content: "short", CreatedUnix: 200, UpdatedUnix: 200},
		},
	}
	state.SetSessionEvidence(store.Marshal())

	blocks := state.GetSessionEvidenceFrozenOpenBlocks(100, "nbudget")
	require.NotContains(t, blocks.Frozen, "[id: old]")
	require.NotContains(t, blocks.Open, "[id: old]")
	require.NotContains(t, blocks.Open, "[TOMBSTONE]")
	require.Contains(t, blocks.Open, "[id: new]")
}

func TestSessionPromptState_EvidencePersistenceUsesLiveStoreOnly(t *testing.T) {
	s := NewSessionPromptState()
	s.ApplySessionEvidenceOps([]EvidenceOperation{
		{Op: "add", ID: "a", Content: "A"},
		{Op: "add", ID: "b", Content: "B"},
		{Op: "delete", ID: "a"},
	})
	store := UnmarshalEvidenceStore(s.GetSessionEvidence())
	require.Len(t, store.Items, 1)
	require.Equal(t, "b", store.Items[0].ID)
}
