package aicommon

import (
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// Import without committing a freeze, to exercise the explicit transaction.
func importFreezeItem(tl *Timeline, id int64, ts time.Time, value TimelineItemValue) {
	tl.mu.Lock()
	defer tl.mu.Unlock()
	tl.idToTs.Set(id, ts.UnixMilli())
	item := &TimelineItem{createdAt: ts, value: value}
	tl.OrderInsertId(id, item)
	tl.OrderInsertTs(ts.UnixMilli(), item)
}

func freezeMutation(id int64, payload string) *PromotableTimelineItem {
	return &PromotableTimelineItem{ID: id, Kind: TimelinePromotedKindRecentTool,
		TargetSection: TimelinePromotedTargetSemiDynamic1, Key: "alpha", Operation: TimelinePromotedOperationUpsert,
		Payload: payload, PayloadHash: promotedPayloadHash(payload)}
}

func TestTimelineFreezePromotionPayloadTriggersBudget(t *testing.T) {
	for _, adaptive := range []bool{false, true} {
		name := "fixed"
		if adaptive {
			name = "adaptive"
		}
		t.Run(name, func(t *testing.T) {
			tl := NewTimeline(nil, nil)
			tl.SetTimelineBucketByteSize(512)
			if adaptive {
				tl.SetTimelineBucketSizer(FixedBucketSizer(512))
			}
			payload := `{"type":"object","description":"` + strings.Repeat("schema", 200) + `"}`
			require.True(t, tl.PushPromotable(1, TimelinePromotedKindRecentTool, TimelinePromotedTargetSemiDynamic1, "alpha", TimelinePromotedOperationUpsert, payload))
			result := tl.Freeze()
			require.Equal(t, int64(1), result.ThroughID)
			require.Equal(t, int64(1), result.Version)
			require.Equal(t, []int64{1}, result.NewlyFrozenIDs, "only the explicit freeze commits, never append")
			rendered := RenderTimelineFrozenOpen(tl)
			require.Empty(t, rendered.Open)
			require.Contains(t, rendered.PromotedSemiDynamic1, payload)
			require.Empty(t, tl.GetTimelineItemIDs(), "schema must never enter AI compression")
			require.Empty(t, tl.Dump(), "no control-plane noise in the ordinary dump")
		})
	}
}

func TestTimelineFreezeAtomicReceiptAndReadOnlyRendering(t *testing.T) {
	tl := NewTimeline(nil, nil)
	tl.SetTimelineBucketByteSize(512)
	ts := time.Date(2026, 9, 27, 10, 0, 0, 0, time.UTC)
	importFreezeItem(tl, 1, ts, &TextTimelineItem{ID: 1, Text: "ordinary history"})
	importFreezeItem(tl, 2, ts.Add(time.Second), freezeMutation(2, strings.Repeat("SCHEMA", 150)))
	// Reading a prompt, including pending payloads, never commits a transition.
	before, err := MarshalTimeline(tl)
	require.NoError(t, err)
	for i := 0; i < 3; i++ {
		require.Empty(t, RenderTimelineFrozenOpen(tl).PromotedSemiDynamic1)
	}
	after, err := MarshalTimeline(tl)
	require.NoError(t, err)
	require.JSONEq(t, string(before), string(after))
	result := tl.Freeze()
	require.Equal(t, []int64{1, 2}, result.NewlyFrozenIDs)
	require.Len(t, result.Promotions, 1)
	require.Equal(t, int64(2), result.Promotions[0].ID)
	require.Zero(t, result.PendingBytes)
	require.Equal(t, int64(1), result.Version)
	require.Contains(t, RenderTimelineFrozenOpen(tl).Frozen, "ordinary history")
	result.Promotions[0].Payload = "caller mutation"
	snapshot := tl.FreezeSnapshot()
	snapshot.Batches[0].IDs[0] = 999
	require.NotContains(t, RenderTimelineFrozenOpen(tl).PromotedSemiDynamic1, "caller mutation")
	require.Equal(t, int64(1), tl.FreezeSnapshot().Batches[0].IDs[0])
	require.Empty(t, tl.Freeze().NewlyFrozenIDs)
	frozen := RenderTimelineFrozenOpen(tl)
	tl.SetTimelineBucketByteSize(4096)
	injectTimelineItem(tl, 3, ts.Add(2*time.Second), &TextTimelineItem{ID: 3, Text: "new tail"})
	next := RenderTimelineFrozenOpen(tl)
	require.Equal(t, frozen.Frozen, next.Frozen)
	require.Equal(t, frozen.PromotedSemiDynamic1, next.PromotedSemiDynamic1)
	require.Contains(t, next.Open, "new tail")
}

func TestTimelineFreezeMixedItemsShareBudget(t *testing.T) {
	ts := time.Date(2026, 9, 27, 10, 0, 0, 0, time.UTC)
	build := func(withPromotion bool) *Timeline {
		tl := NewTimeline(nil, nil)
		tl.SetTimelineBucketByteSize(700)
		injectTimelineItem(tl, 1, ts, &TextTimelineItem{ID: 1, Text: strings.Repeat("a", 200)})
		if withPromotion {
			injectTimelineItem(tl, 2, ts.Add(time.Second), freezeMutation(2, strings.Repeat("s", 300)))
		}
		injectTimelineItem(tl, 3, ts.Add(2*time.Second), &TextTimelineItem{ID: 3, Text: strings.Repeat("b", 200)})
		return tl
	}
	require.Empty(t, RenderTimelineFrozenOpen(build(false)).Frozen)
	mixed := build(true)
	require.Equal(t, int64(2), mixed.Freeze().ThroughID)
	require.Contains(t, RenderTimelineFrozenOpen(mixed).PromotedSemiDynamic1, strings.Repeat("s", 300))
	require.Contains(t, RenderTimelineFrozenOpen(mixed).Open, strings.Repeat("b", 200))
}

func TestTimelineFreezeRestoreForkRollbackAndRemap(t *testing.T) {
	ts := time.Date(2026, 9, 27, 10, 0, 0, 0, time.UTC)
	tl := NewTimeline(nil, nil)
	injectTimelineItem(tl, 1, ts, &TextTimelineItem{ID: 1, Text: "history"})
	injectTimelineItem(tl, 2, ts.Add(time.Second), freezeMutation(2, "original schema"))
	tl.FreezeAll()
	frozen := RenderTimelineFrozenOpen(tl)
	injectTimelineItem(tl, 3, ts.Add(2*time.Second), &TextTimelineItem{ID: 3, Text: "tail"})
	dump, err := MarshalTimeline(tl)
	require.NoError(t, err)
	restored, err := UnmarshalTimeline(dump)
	require.NoError(t, err)
	require.Equal(t, tl.FreezeSnapshot(), restored.FreezeSnapshot())
	require.Equal(t, RenderTimelineFrozenOpen(tl), RenderTimelineFrozenOpen(restored))
	next := int64(100)
	restored.ReassignIDs(func() int64 { next++; return next })
	require.Equal(t, int64(102), restored.Freeze().ThroughID)
	require.Equal(t, frozen.Frozen, RenderTimelineFrozenOpen(restored).Frozen)
	require.Equal(t, frozen.PromotedSemiDynamic1, RenderTimelineFrozenOpen(restored).PromotedSemiDynamic1)
	require.Contains(t, RenderTimelineFrozenOpen(restored).Open, "tail")
	fork, err := tl.ForkForTask("child", "test", nil, nil)
	require.NoError(t, err)
	require.Equal(t, tl.FreezeSnapshot(), fork.Branch.FreezeSnapshot())
	injectTimelineItem(fork.Branch, 4, ts.Add(3*time.Second), freezeMutation(4, "child schema"))
	fork.Branch.FreezeAll()
	require.Equal(t, frozen.PromotedSemiDynamic1, RenderTimelineFrozenOpen(tl).PromotedSemiDynamic1, "fork must not mutate parent's committed snapshot")
	_, err = fork.MergeBack()
	require.NoError(t, err)
	require.Contains(t, RenderTimelineFrozenOpen(tl).Open, "child schema", "a child freeze must not force parent promotion")
	tl.FreezeAll()
	require.Contains(t, RenderTimelineFrozenOpen(tl).PromotedSemiDynamic1, "child schema")
	tl.TruncateAfter(2)
	require.Equal(t, frozen.PromotedSemiDynamic1, RenderTimelineFrozenOpen(tl).PromotedSemiDynamic1)
	require.Empty(t, RenderTimelineFrozenOpen(tl).Open)
}

func TestTimelineFreezeLegacyRestorePreservesPromotion(t *testing.T) {
	tl := NewTimeline(nil, nil)
	require.True(t, tl.PushPromotable(1, TimelinePromotedKindRecentTool, TimelinePromotedTargetSemiDynamic1, "alpha", TimelinePromotedOperationUpsert, "original"))
	tl.FreezeAll()
	require.True(t, tl.PushPromotable(2, TimelinePromotedKindRecentTool, TimelinePromotedTargetSemiDynamic1, "alpha", TimelinePromotedOperationDelete, ""))
	dump, err := MarshalTimeline(tl)
	require.NoError(t, err)
	var legacy map[string]json.RawMessage
	require.NoError(t, json.Unmarshal([]byte(dump), &legacy))
	delete(legacy, "freeze_state")
	legacyDump, err := json.Marshal(legacy)
	require.NoError(t, err)
	restored, err := UnmarshalTimeline(string(legacyDump))
	require.NoError(t, err)
	result := RenderTimelineFrozenOpen(restored)
	require.Contains(t, result.PromotedSemiDynamic1, "original")
	require.Contains(t, result.Open, "[DELETE] alpha")
	require.Equal(t, int64(1), restored.Freeze().ThroughID)
}

func TestTimelineFreezeDisabledAdaptiveSizerAndReadOnlyCalls(t *testing.T) {
	tl := NewTimeline(nil, nil)
	calls := 0
	tl.SetTimelineBucketSizer(BucketSizerFunc(func(ctx BucketSizerContext) int64 { calls++; return -1 }))
	require.True(t, tl.PushPromotable(1, TimelinePromotedKindRecentTool, TimelinePromotedTargetSemiDynamic1, "alpha", TimelinePromotedOperationUpsert, strings.Repeat("large", 15000)))
	require.Empty(t, tl.FreezeSnapshot().Batches)
	before := calls
	RenderTimelineFrozenOpen(tl)
	RenderTimelineFrozenOpen(tl)
	require.Equal(t, before, calls, "read-only rendering must not invoke an adaptive decision")
	tl.FreezeAll()
	require.NotEmpty(t, RenderTimelineFrozenOpen(tl).PromotedSemiDynamic1)
}

func TestTimelineFreezeOutOfOrderTimestampsNeverLoseItems(t *testing.T) {
	tl := NewTimeline(nil, nil)
	ts := time.Date(2026, 9, 27, 10, 0, 0, 0, time.UTC)
	importFreezeItem(tl, 1, ts.Add(10*time.Minute), freezeMutation(1, "later time, earlier ID"))
	importFreezeItem(tl, 2, ts, &TextTimelineItem{ID: 2, Text: "earlier time, later ID"})
	require.Empty(t, tl.Freeze().NewlyFrozenIDs)
	require.Contains(t, RenderTimelineFrozenOpen(tl).Open, "earlier time, later ID")
	require.Contains(t, RenderTimelineFrozenOpen(tl).Open, "later time, earlier ID")
	tl.FreezeAll()
	require.Contains(t, RenderTimelineFrozenOpen(tl).PromotedSemiDynamic1, "later time, earlier ID")
	require.Contains(t, RenderTimelineFrozenOpen(tl).Frozen, "earlier time, later ID")
}

func TestTimelineFreezeConcurrentAppendAndRender(t *testing.T) {
	tl := NewTimeline(nil, nil)
	tl.SetTimelineBucketByteSize(512)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := int64(1); i <= 40; i++ {
			tl.PushPromotable(i, TimelinePromotedKindRecentTool, TimelinePromotedTargetSemiDynamic1, "alpha", TimelinePromotedOperationUpsert, strings.Repeat("schema", 30))
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 40; i++ {
			RenderTimelineFrozenOpen(tl)
			tl.FreezeSnapshot()
		}
	}()
	wg.Wait()
	tl.FreezeAll()
	require.Equal(t, int64(40), tl.Freeze().ThroughID)
	require.Empty(t, RenderTimelineFrozenOpen(tl).Open)
}

func TestTimelineFreezeCarriesTaskContextAcrossCommittedByteBuckets(t *testing.T) {
	tl := NewTimeline(nil, nil)
	tl.SetTimelineBucketByteSize(256)
	ts := time.Date(2026, 9, 27, 10, 0, 0, 0, time.UTC)
	injectTimelineItem(tl, 1, ts, &TextTimelineItem{ID: 1, Text: "[note] [task:1-2]:\n" + strings.Repeat("old ", 100)})
	injectTimelineItem(tl, 2, ts.Add(time.Second), makeToolResult(2, "example", true, "tail"))
	require.Contains(t, RenderTimelineFrozenOpen(tl).Open, "task=1-2")
	tl.FreezeAll()
	require.Contains(t, RenderTimelineFrozenOpen(tl).Frozen, "task=1-2")
}
