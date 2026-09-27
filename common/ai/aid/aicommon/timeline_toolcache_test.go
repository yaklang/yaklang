package aicommon

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func importToolCacheEvent(tl *Timeline, id int64, at time.Time, operation, key, payload string) {
	importFreezeItem(tl, id, at, &PromotableTimelineItem{
		ID: id, Kind: TimelinePromotedKindRecentTool, TargetSection: TimelinePromotedTargetSemiDynamic1,
		Key: key, Operation: operation, Payload: payload, PayloadHash: promotedPayloadHash(payload),
	})
}

func requireToolCacheOrder(t *testing.T, tl *Timeline, first, second string) string {
	t.Helper()
	semi := RenderTimelineFrozenOpen(tl).PromotedSemiDynamic1
	require.Contains(t, semi, first)
	require.Contains(t, semi, second)
	require.Less(t, strings.Index(semi, first), strings.Index(semi, second))
	return semi
}

// A/B schemas are frozen first. Reusing A and then B only appends small Open
// events. Freezing A's event changes Semi to B/A; B's still-open reuse cannot
// change that prefix until its own freeze. Neither reuse copies a schema.
func TestToolCacheReuseOnlyReordersFrozenEvents(t *testing.T) {
	tl := NewTimeline(nil, nil)
	base := time.Date(2026, 9, 27, 10, 0, 0, 0, time.UTC)
	importToolCacheEvent(tl, 1, base, TimelinePromotedOperationUpsert, "alpha", "SCHEMA_ALPHA")
	importToolCacheEvent(tl, 2, base.Add(time.Second), TimelinePromotedOperationUpsert, "beta", "SCHEMA_BETA")
	tl.FreezeAll()
	initial := requireToolCacheOrder(t, tl, "SCHEMA_ALPHA", "SCHEMA_BETA")
	initialState := cloneTimelinePromotedState(tl.promotedState)
	importToolCacheEvent(tl, 3, base.Add(2*time.Second), TimelinePromotedOperationReuse, "alpha", "")
	openA := RenderTimelineFrozenOpen(tl).PromotedOpen
	importToolCacheEvent(tl, 4, base.Add(4*time.Minute), TimelinePromotedOperationReuse, "beta", "")
	before, err := MarshalTimeline(tl)
	require.NoError(t, err)
	for i := 0; i < 3; i++ {
		view := RenderTimelineFrozenOpen(tl)
		require.Equal(t, initial, view.PromotedSemiDynamic1)
		require.Contains(t, view.PromotedOpen, "reused recent tool: alpha")
		require.Contains(t, view.PromotedOpen, "reused recent tool: beta")
		require.NotContains(t, view.PromotedOpen, "SCHEMA_")
		// Exclude the closing wrapper: previously appended event text stays put.
		require.True(t, strings.HasPrefix(view.PromotedOpen, strings.TrimSuffix(openA, "\n<|CACHE_TOOL_CALL_END_[current-nonce]|>")))
	}
	after, err := MarshalTimeline(tl)
	require.NoError(t, err)
	require.Equal(t, before, after, "rendering is read-only")
	require.Equal(t, initialState, tl.promotedState)

	partial := tl.Freeze()
	require.Equal(t, []int64{3}, partial.NewlyFrozenIDs)
	require.Positive(t, partial.PendingBytes, "pending reuse contributes to the freeze budget")
	requireToolCacheOrder(t, tl, "SCHEMA_BETA", "SCHEMA_ALPHA")
	require.Equal(t, []string{"alpha", "beta"}, tl.effectivePromotedKeys(TimelinePromotedTargetSemiDynamic1, TimelinePromotedKindRecentTool), "runtime restore sees pending reuse without moving Semi")
	alpha := tl.promotedState.Entries[TimelinePromotedTargetSemiDynamic1][TimelinePromotedKindRecentTool]["alpha"]
	require.Equal(t, int64(1), alpha.SourceItemID)
	require.Equal(t, int64(3), alpha.LastUsedItemID)
	require.Equal(t, promotedPayloadHash("SCHEMA_ALPHA"), alpha.PayloadHash)
	require.Equal(t, "SCHEMA_ALPHA", alpha.Payload)

	require.Equal(t, []int64{4}, tl.FreezeAll().NewlyFrozenIDs)
	final := requireToolCacheOrder(t, tl, "SCHEMA_ALPHA", "SCHEMA_BETA")
	require.Equal(t, 1, strings.Count(final, "SCHEMA_ALPHA"))
	require.Empty(t, RenderTimelineFrozenOpen(tl).PromotedOpen)
	require.Empty(t, tl.FreezeAll().NewlyFrozenIDs)
	require.Equal(t, final, RenderTimelineFrozenOpen(tl).PromotedSemiDynamic1)
}

func TestToolCacheReuseValidation(t *testing.T) {
	tl := NewTimeline(nil, nil)
	push := func(id int64, operation, payload string) bool {
		return tl.PushPromotable(id, TimelinePromotedKindRecentTool, TimelinePromotedTargetSemiDynamic1, "alpha", operation, payload)
	}
	require.False(t, push(1, TimelinePromotedOperationReuse, ""), "missing schema needs a full upsert")
	require.True(t, push(2, TimelinePromotedOperationUpsert, "SCHEMA_ALPHA"))
	require.True(t, push(3, TimelinePromotedOperationReuse, ""))
	require.False(t, push(4, TimelinePromotedOperationReuse, "unexpected schema"))
	require.False(t, tl.PushPromotable(4, TimelinePromotedKindEvidence, TimelinePromotedTargetSemiDynamic1, "alpha", TimelinePromotedOperationReuse, ""))
	require.True(t, push(5, TimelinePromotedOperationDelete, ""))
	require.False(t, push(6, TimelinePromotedOperationReuse, ""), "reuse cannot resurrect an evicted tool")
	require.True(t, push(7, TimelinePromotedOperationUpsert, "SCHEMA_ALPHA_V2"))
	require.True(t, push(8, TimelinePromotedOperationReuse, ""))
	tl.FreezeAll()
	entry := tl.promotedState.Entries[TimelinePromotedTargetSemiDynamic1][TimelinePromotedKindRecentTool]["alpha"]
	require.Equal(t, "SCHEMA_ALPHA_V2", entry.Payload)
	require.Equal(t, int64(7), entry.SourceItemID)
	require.Equal(t, int64(8), entry.LastUsedItemID)
}

func TestToolCacheReuseRestoreReassignAndRollback(t *testing.T) {
	tl := NewTimeline(nil, nil)
	base := time.Date(2026, 9, 27, 10, 0, 0, 0, time.UTC)
	importToolCacheEvent(tl, 1, base, TimelinePromotedOperationUpsert, "alpha", "SCHEMA_ALPHA")
	importToolCacheEvent(tl, 2, base.Add(time.Second), TimelinePromotedOperationUpsert, "beta", "SCHEMA_BETA")
	tl.FreezeAll()
	importToolCacheEvent(tl, 3, base.Add(2*time.Second), TimelinePromotedOperationReuse, "alpha", "")
	tl.FreezeAll()
	frozen := requireToolCacheOrder(t, tl, "SCHEMA_BETA", "SCHEMA_ALPHA")
	importToolCacheEvent(tl, 4, base.Add(3*time.Second), TimelinePromotedOperationReuse, "beta", "")
	raw, err := MarshalTimeline(tl)
	require.NoError(t, err)
	restored, err := UnmarshalTimeline(raw)
	require.NoError(t, err)
	require.Equal(t, frozen, RenderTimelineFrozenOpen(restored).PromotedSemiDynamic1)
	require.Equal(t, []string{"alpha", "beta"}, restored.effectivePromotedKeys(TimelinePromotedTargetSemiDynamic1, TimelinePromotedKindRecentTool))
	nextID := int64(100)
	restored.ReassignIDs(func() int64 { nextID++; return nextID })
	entry := restored.promotedState.Entries[TimelinePromotedTargetSemiDynamic1][TimelinePromotedKindRecentTool]["alpha"]
	require.Equal(t, int64(101), entry.SourceItemID)
	require.Equal(t, int64(103), entry.LastUsedItemID)
	require.Equal(t, frozen, RenderTimelineFrozenOpen(restored).PromotedSemiDynamic1)
	restored.TruncateAfter(102)
	requireToolCacheOrder(t, restored, "SCHEMA_ALPHA", "SCHEMA_BETA")
	require.Equal(t, []string{"alpha", "beta"}, restored.effectivePromotedKeys(TimelinePromotedTargetSemiDynamic1, TimelinePromotedKindRecentTool))
}

func TestToolCacheReuseMalformedJournalDoesNotResurrect(t *testing.T) {
	tl := NewTimeline(nil, nil)
	base := time.Date(2026, 9, 27, 10, 0, 0, 0, time.UTC)
	importToolCacheEvent(tl, 1, base, TimelinePromotedOperationUpsert, "alpha", "SCHEMA_ALPHA")
	importToolCacheEvent(tl, 2, base.Add(time.Second), TimelinePromotedOperationDelete, "alpha", "")
	// Simulate an imported invalid journal, bypassing PushPromotable validation.
	importToolCacheEvent(tl, 3, base.Add(2*time.Second), TimelinePromotedOperationReuse, "alpha", "")
	importToolCacheEvent(tl, 4, base.Add(3*time.Second), "unknown", "ghost", "SCHEMA_GHOST")
	require.Empty(t, tl.effectivePromotedKeys(TimelinePromotedTargetSemiDynamic1, TimelinePromotedKindRecentTool))
	tl.FreezeAll()
	require.Empty(t, RenderTimelineFrozenOpen(tl).PromotedSemiDynamic1)
	require.Empty(t, tl.effectivePromotedKeys(TimelinePromotedTargetSemiDynamic1, TimelinePromotedKindRecentTool))
}

func TestToolCacheLegacyOrderUsesSourceAndDeterministicTieBreak(t *testing.T) {
	state := newTimelinePromotedState()
	state.Entries[TimelinePromotedTargetSemiDynamic1] = map[string]map[string]*PromotedTimelineEntry{
		TimelinePromotedKindRecentTool: {
			"alpha": {Payload: "SCHEMA_ALPHA", SourceItemID: 2},
			"zeta":  {Payload: "SCHEMA_ZETA", SourceItemID: 1},
			"beta":  {Payload: "SCHEMA_BETA", SourceItemID: 2},
			"nil":   nil,
		},
	}
	semi := renderPromotedRecentTools(state)
	require.Less(t, strings.Index(semi, "SCHEMA_ZETA"), strings.Index(semi, "SCHEMA_ALPHA"))
	require.Less(t, strings.Index(semi, "SCHEMA_ALPHA"), strings.Index(semi, "SCHEMA_BETA"))
	require.Equal(t, semi, renderPromotedRecentTools(state))
}
