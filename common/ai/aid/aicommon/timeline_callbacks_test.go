package aicommon

import (
	"encoding/json"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/schema"
)

func TestTimelineCallbacksItemInputs(t *testing.T) {
	tl := NewTimeline(nil, nil)
	var events []TimelineItemInputEvent
	tl.RegisterItemInputCallback("audit", func(event TimelineItemInputEvent) {
		// Reading the committed Timeline inside the callback must not deadlock.
		tl.mu.RLock()
		stored := tl.idToTimelineItem.Have(event.ID)
		tl.mu.RUnlock()
		require.True(t, stored)
		var item TimelineItem
		require.NoError(t, json.Unmarshal([]byte(event.ItemJSON), &item))
		require.Equal(t, event.ID, item.GetID())
		require.False(t, event.Timestamp.IsZero())
		events = append(events, event)
	})
	tl.PushText(1, "first observation")
	tl.PushTextWithPromptProjection(2, "visible output", "prompt projection")
	tl.PushUserInteraction(UserInteractionStage_FreeInput, 3, "question", "answer")
	tl.PushToolResult(&aitool.ToolResult{ID: 4, Name: "read_file", Data: "tool output"})
	tl.EnsureTaskUserInput("task-1", "current request", func() int64 { return 5 })
	tl.EnsureTaskUserInput("task-1", "current request", func() int64 { t.Fatal("duplicate ingress"); return 6 })
	tl.pushUserInputRecord(schema.AIAgentUserInputRecord{UserInput: "next input", Round: 2}, 6)
	require.True(t, tl.PushPromotable(7, TimelinePromotedKindRecentTool, TimelinePromotedTargetSemiDynamic1,
		"read_file", TimelinePromotedOperationUpsert, "exact tool schema"))
	require.False(t, tl.PushPromotable(7, TimelinePromotedKindRecentTool, TimelinePromotedTargetSemiDynamic1,
		"read_file", TimelinePromotedOperationUpsert, "duplicate"))
	_, err := tl.applyEvidenceOperations([]EvidenceOperation{{Op: "add", ID: "e1", Content: "exact evidence"}}, "", func() int64 { return 8 })
	require.NoError(t, err)
	require.Len(t, events, 8)
	for i, event := range events {
		require.Equal(t, int64(i+1), event.ID)
	}
	require.Contains(t, events[1].ItemJSON, "prompt projection")
	require.Contains(t, events[2].ItemJSON, "answer")
	require.Contains(t, events[5].ItemJSON, `"round":2`)
	require.Contains(t, events[6].ItemJSON, "exact tool schema")
	require.Contains(t, events[7].ItemJSON, "exact evidence")
}

func TestTimelineCallbacksRegistrationAndReentry(t *testing.T) {
	tl := NewTimeline(nil, nil)
	var calls []string
	tl.RegisterItemInputCallback("replace", func(TimelineItemInputEvent) { t.Fatal("superseded callback") })
	tl.RegisterItemInputCallback("replace", func(event TimelineItemInputEvent) {
		calls = append(calls, "first")
		tl.RegisterItemInputCallback("replace", nil)
		tl.RegisterItemInputCallback("late", func(TimelineItemInputEvent) { calls = append(calls, "late") })
		tl.PushText(event.ID+1, "write from callback")
	})
	tl.RegisterItemInputCallback("panic", func(TimelineItemInputEvent) { panic("listener failure") })
	tl.RegisterItemInputCallback("tail", func(event TimelineItemInputEvent) {
		calls = append(calls, "tail")
		require.Contains(t, tl.Dump(), "write from callback")
	})
	tl.PushText(1, "trigger")
	// The inner write sees the new registry; the outer receipt retains its list.
	require.Equal(t, []string{"first", "tail", "late", "tail"}, calls)
	require.Equal(t, []int64{1, 2}, tl.GetTimelineItemIDs())
}

func TestTimelineCallbacksFreeze(t *testing.T) {
	tl := NewTimeline(nil, nil)
	var receipts []TimelineFreezeResult
	tl.RegisterFreezeCallback("mutator", func(event TimelineFreezeResult) {
		event.NewlyFrozenIDs[0] = -1
		event.Promotions[0].Payload = "mutated"
	})
	tl.RegisterFreezeCallback("audit", func(event TimelineFreezeResult) {
		require.NotEmpty(t, RenderTimelineFrozenOpen(tl).Frozen)
		receipts = append(receipts, event)
	})
	tl.RegisterCompressFreezeCallback("unexpected", func(TimelineCompressFreezeEvent) { t.Fatal("ordinary freeze") })
	tl.RegisterSummaryCallback("unexpected", func(TimelineSummaryEvent) { t.Fatal("ordinary freeze") })
	tl.PushText(1, "ordinary history")
	tl.PushUserInteraction(UserInteractionStage_FreeInput, 2, "", "exact user request")
	result := tl.FreezeAll()
	require.Len(t, receipts, 1)
	require.Equal(t, result, receipts[0])
	require.Equal(t, []int64{1, 2}, result.NewlyFrozenIDs)
	require.Contains(t, result.Promotions[0].Payload, "exact user request")
	require.Empty(t, tl.FreezeAll().NewlyFrozenIDs)
	require.Len(t, receipts, 1, "no-op freeze must not notify")
	// Exercise the complete-bucket primitive too, rather than only FreezeAll.
	tl.SetTimelineBucketByteSize(1)
	tl.PushText(3, "new bucket")
	// This receipt has no exact promotions, so remove the earlier mutator.
	tl.RegisterFreezeCallback("mutator", nil)
	require.Equal(t, []int64{3}, tl.Freeze().NewlyFrozenIDs)
	require.Len(t, receipts, 2)
}

func TestTimelineCallbacksEvidenceBatch(t *testing.T) {
	tl := NewTimeline(nil, nil)
	var inputs []int64
	tl.RegisterItemInputCallback("consumer", func(event TimelineItemInputEvent) {
		store, found := tl.evidenceStore()
		require.True(t, found)
		require.Len(t, store.Items, 2, "callback sees the full committed evidence batch")
		inputs = append(inputs, event.ID)
	})
	id := int64(0)
	store := NewEvidenceStore()
	store.ApplyOperations([]EvidenceOperation{{Op: "add", ID: "e1", Content: "first"}, {Op: "add", ID: "e2", Content: "second"}})
	require.NoError(t, tl.replaceEvidence(store, func() int64 { id++; return id }))
	require.Equal(t, []int64{1, 2}, inputs)
	require.NoError(t, tl.replaceEvidence(store, func() int64 { t.Fatal("unchanged evidence"); return 3 }))
	require.Equal(t, []int64{1, 2}, inputs)
	store.Items[0].Content = "updated"
	require.Error(t, tl.replaceEvidence(store, func() int64 { return 1 }))
	require.Equal(t, []int64{1, 2}, inputs, "failed batch must not notify")
}

func TestTimelineCallbacksDoNotPersistOrInherit(t *testing.T) {
	parent := NewTimeline(nil, nil)
	parent.PushText(1, "existing history")
	before, err := MarshalTimeline(parent)
	require.NoError(t, err)
	beforePrompt := parent.DumpForPrompt()
	var inputs []int64
	parent.RegisterItemInputCallback("audit", func(event TimelineItemInputEvent) { inputs = append(inputs, event.ID) })
	parent.RegisterFreezeCallback("audit", func(TimelineFreezeResult) { t.Fatal("inherited freeze listener") })
	parent.RegisterCompressFreezeCallback("audit", func(TimelineCompressFreezeEvent) { t.Fatal("inherited compress listener") })
	parent.RegisterSummaryCallback("audit", func(TimelineSummaryEvent) { t.Fatal("inherited summary listener") })
	parent.RegisterMemoryCallback("audit", func(TimelineMemoryEvent) { t.Fatal("inherited memory listener") })
	after, err := MarshalTimeline(parent)
	require.NoError(t, err)
	require.Equal(t, string(before), string(after), "listeners do not alter persisted state or cache inputs")
	require.Equal(t, beforePrompt, parent.DumpForPrompt())
	restored, err := UnmarshalTimeline(after)
	require.NoError(t, err)
	copy := parent.CopyReducibleTimelineWithMemory()
	sub := parent.CreateSubTimeline(1)
	fork, err := parent.ForkForTask("1", "example worker", nil, nil)
	require.NoError(t, err)
	for _, detached := range []*Timeline{restored, copy, sub, fork.Branch} {
		detached.PushText(2, "detached write")
		detached.FreezeAll()
		require.Empty(t, detached.itemInputCallbacks.snapshot())
		require.Empty(t, detached.compressFreezeCallbacks.snapshot())
		require.Empty(t, detached.summaryCallbacks.snapshot())
		require.Empty(t, detached.memoryCallbacks.snapshot())
	}
	require.Empty(t, inputs)
	_, err = fork.MergeBack()
	require.NoError(t, err)
	require.Equal(t, []int64{2}, inputs, "only newly committed parent input is notified")
	_, err = fork.MergeBack()
	require.Error(t, err)
	require.Equal(t, []int64{2}, inputs, "failed merge must not notify")
}

func TestTimelineCallbacksConcurrentRegistration(t *testing.T) {
	tl := NewTimeline(nil, nil)
	var count atomic.Int64
	tl.RegisterItemInputCallback("audit", func(TimelineItemInputEvent) { count.Add(1) })
	var wg sync.WaitGroup
	for worker := 0; worker < 4; worker++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			for i := 0; i < 25; i++ {
				tl.RegisterItemInputCallback("temporary", func(TimelineItemInputEvent) {})
				tl.PushText(int64(worker*25+i+1), "concurrent observation")
				tl.RegisterItemInputCallback("temporary", nil)
			}
		}(worker)
	}
	wg.Wait()
	require.Equal(t, int64(100), count.Load())
	require.Len(t, tl.GetTimelineItemIDs(), 100)
}

func ExampleTimeline_RegisterItemInputCallback() {
	tl := NewTimeline(nil, nil)
	// Future consumers can enqueue the immutable receipts for batch processing.
	var batch []TimelineItemInputEvent
	tl.RegisterItemInputCallback("memory-batch", func(event TimelineItemInputEvent) {
		batch = append(batch, event)
	})
	tl.RegisterFreezeCallback("memory-batch", func(event TimelineFreezeResult) {
		// Use NewlyFrozenIDs to select entries from batch, plus exact Promotions.
		_ = event
	})
	tl.RegisterCompressFreezeCallback("memory-batch", func(event TimelineCompressFreezeEvent) {
		// Reuse this compression receipt instead of requesting a second summary.
		_ = event
	})
	tl.RegisterSummaryCallback("memory-batch", func(event TimelineSummaryEvent) {
		summary, _ := io.ReadAll(event.Summary)
		_, _, _ = strings.TrimSpace(string(summary)), event.Prompt, event.RetainedIDs
	})
	tl.RegisterMemoryCallback("memory-batch", func(event TimelineMemoryEvent) {
		// Enqueue candidates for storage independently of summary consumers.
		_, _ = event.MemoryEntities, event.Err
	})
	tl.PushText(1, "verified observation")
	tl.FreezeAll()
	tl.RegisterItemInputCallback("memory-batch", nil)
	_ = batch
}
