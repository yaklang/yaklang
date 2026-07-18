package aicommon

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

func injectPromotable(tl *Timeline, id int64, ts time.Time, operation, key, payload string) {
	injectTimelineItem(tl, id, ts, &PromotableTimelineItem{
		ID:            id,
		Kind:          TimelinePromotedKindRecentTool,
		TargetSection: TimelinePromotedTargetSemiDynamic1,
		Key:           key,
		Operation:     operation,
		Payload:       payload,
		PayloadHash:   promotedPayloadHash(payload),
	})
}

func TestConfigRecordRecentlyUsedToolHasSinglePromptSourceAndNoopReuse(t *testing.T) {
	cfg := NewConfig(context.Background())
	tool := aitool.NewWithoutCallback("alpha_tool",
		aitool.WithDescription("alpha"),
		aitool.WithStringParam("path"),
	)
	mutation := cfg.RecordRecentlyUsedTool(tool)
	require.NotNil(t, mutation.Upsert)
	first := BuildPromptFrozenOpenMaterials(cfg)
	require.Contains(t, first.TimelineOpen, "## Tool: alpha_tool")
	require.Empty(t, first.PromotedSemiDynamic1)
	maxID := cfg.GetTimeline().GetMaxID()

	mutation = cfg.RecordRecentlyUsedTool(tool)
	require.Nil(t, mutation.Upsert)
	require.Empty(t, mutation.Deleted)
	require.Equal(t, maxID, cfg.GetTimeline().GetMaxID())
	second := BuildPromptFrozenOpenMaterials(cfg)
	require.Equal(t, first.TimelineOpen, second.TimelineOpen)

	cfg.GetTimeline().ForcePromoteAll()
	promoted := BuildPromptFrozenOpenMaterials(cfg)
	require.NotContains(t, promoted.TimelineOpen, "## Tool: alpha_tool")
	require.Contains(t, promoted.PromotedSemiDynamic1, "## Tool: alpha_tool")
	require.Equal(t, 1, strings.Count(promoted.PromotedSemiDynamic1, "Direct Params Schema"))
}

func TestTimelinePromotedStateOpenSealDeleteAndRollback(t *testing.T) {
	base := time.Date(2026, 7, 18, 10, 0, 0, 0, time.UTC)
	tl := NewTimeline(nil, nil)
	injectTimelineItem(tl, 1, base, &TextTimelineItem{ID: 1, Text: "ordinary-old"})
	ordinaryBefore := tl.Dump()
	injectPromotable(tl, 2, base.Add(time.Second), TimelinePromotedOperationUpsert, "alpha", "## Tool: alpha\nSCHEMA_ALPHA")

	// Control items do not alter ordinary buckets, nonces, diffs, stats or UI.
	require.Equal(t, ordinaryBefore, tl.Dump())
	require.Equal(t, []int64{1}, tl.GetTimelineItemIDs())
	require.Len(t, tl.getActiveTimelineItemIDs(), 1)
	require.Len(t, tl.ToTimelineItemOutputLastN(10), 1)

	first := RenderTimelineFrozenOpen(tl)
	require.Empty(t, first.PromotedSemiDynamic1)
	require.Contains(t, first.Open, "SCHEMA_ALPHA")
	require.Equal(t, 1, strings.Count(first.Open, "How to use directly_call_tool"))

	// A successor bucket seals the old narrative bucket and jointly promotes its
	// control mutation into stable Semi1.
	injectTimelineItem(tl, 3, base.Add(4*time.Minute), &TextTimelineItem{ID: 3, Text: "ordinary-new"})
	sealed := RenderTimelineFrozenOpen(tl)
	require.Contains(t, sealed.Frozen, "ordinary-old")
	require.NotContains(t, sealed.Frozen, "SCHEMA_ALPHA")
	require.Contains(t, sealed.Open, "ordinary-new")
	require.NotContains(t, sealed.Open, "SCHEMA_ALPHA")
	require.Contains(t, sealed.PromotedSemiDynamic1, "SCHEMA_ALPHA")
	require.Equal(t, 1, strings.Count(sealed.PromotedSemiDynamic1, "How to use directly_call_tool"))

	// Serialization preserves both journal and materialized watermark.
	raw, err := MarshalTimeline(tl)
	require.NoError(t, err)
	restored, err := UnmarshalTimeline(raw)
	require.NoError(t, err)
	require.Equal(t, sealed.PromotedSemiDynamic1, RenderTimelineFrozenOpen(restored).PromotedSemiDynamic1)

	// Delete is first visible as an Open tombstone, then removes the stable view
	// on the next seal. Rolling back restores the previous materialized version.
	injectPromotable(tl, 4, base.Add(4*time.Minute+time.Second), TimelinePromotedOperationDelete, "alpha", "")
	pendingDelete := RenderTimelineFrozenOpen(tl)
	require.Contains(t, pendingDelete.PromotedSemiDynamic1, "SCHEMA_ALPHA")
	require.Contains(t, pendingDelete.Open, "invalidated recent tool: alpha")
	require.Equal(t, 1, strings.Count(pendingDelete.PromotedSemiDynamic1+pendingDelete.Open, "How to use directly_call_tool"))

	injectTimelineItem(tl, 5, base.Add(8*time.Minute), &TextTimelineItem{ID: 5, Text: "ordinary-later"})
	deleted := RenderTimelineFrozenOpen(tl)
	require.Empty(t, deleted.PromotedSemiDynamic1)
	require.NotContains(t, deleted.Open, "invalidated recent tool")

	tl.TruncateAfter(3)
	rolledBack := RenderTimelineFrozenOpen(tl)
	require.Contains(t, rolledBack.PromotedSemiDynamic1, "SCHEMA_ALPHA")
}

func TestTimelinePromotedStateStableOrdering(t *testing.T) {
	base := time.Date(2026, 7, 18, 11, 0, 0, 0, time.UTC)
	tl := NewTimeline(nil, nil)
	injectTimelineItem(tl, 1, base, &TextTimelineItem{ID: 1, Text: "old"})
	injectPromotable(tl, 2, base.Add(time.Second), TimelinePromotedOperationUpsert, "zeta", "## Tool: zeta")
	injectPromotable(tl, 3, base.Add(2*time.Second), TimelinePromotedOperationUpsert, "alpha", "## Tool: alpha")
	injectTimelineItem(tl, 4, base.Add(4*time.Minute), &TextTimelineItem{ID: 4, Text: "new"})

	a := RenderTimelineFrozenOpen(tl).PromotedSemiDynamic1
	b := RenderTimelineFrozenOpen(tl).PromotedSemiDynamic1
	require.Equal(t, a, b)
	require.Less(t, strings.Index(a, "## Tool: alpha"), strings.Index(a, "## Tool: zeta"))
}

func TestTimelineForkMergesPromotionJournalWithoutDiffNoise(t *testing.T) {
	base := time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC)
	parent := NewTimeline(nil, nil)
	injectTimelineItem(parent, 1, base, &TextTimelineItem{ID: 1, Text: "parent"})
	fork, err := parent.ForkForTask("1-1", "child", nil, nil)
	require.NoError(t, err)
	require.NotNil(t, fork)
	injectPromotable(fork.Branch, 2, base.Add(time.Second), TimelinePromotedOperationUpsert, "branch_tool", "## Tool: branch_tool")

	diff, err := fork.Diff()
	require.NoError(t, err)
	require.Empty(t, diff)
	merged, err := fork.MergeBack()
	require.NoError(t, err)
	require.Equal(t, 0, merged.ActiveItemsMerged, "control entries must not inflate ordinary event statistics")
	require.Contains(t, RenderTimelineFrozenOpen(parent).Open, "## Tool: branch_tool")
}
