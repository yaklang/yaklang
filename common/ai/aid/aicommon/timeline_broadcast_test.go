package aicommon

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/schema"
)

func newTimelineBroadcastConfig(t *testing.T, opts ...ConfigOption) *Config {
	t.Helper()
	cfg := NewTestConfig(context.Background(), append([]ConfigOption{
		WithDisableAutoSkills(true), WithDisableCreateDBRuntime(true), WithSequence(100),
	}, opts...)...)
	t.Cleanup(func() { cfg.ReleaseTimelineSubscription(); cfg.cancel() })
	return cfg
}

func broadcastSubscriptionCounts(b *TimelineBroadcaster) (int, int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	refs := 0
	for _, subscription := range b.subscribers {
		refs += subscription.refs
	}
	return len(b.subscribers), refs
}

func requireBroadcastItem(t *testing.T, timeline *Timeline, id int64) *TimelineItem {
	t.Helper()
	timeline.mu.RLock()
	defer timeline.mu.RUnlock()
	item, ok := timeline.idToTimelineItem.Get(id)
	require.True(t, ok)
	require.NotNil(t, item)
	return item
}

func TestTimelineBroadcastSessionAndForkIsolation(t *testing.T) {
	parent := newTimelineBroadcastConfig(t)
	parent.Timeline.PushText(parent.AcquireId(), "inherited history")
	first, err := parent.Timeline.ForkForTask("first", "first", parent, parent)
	require.NoError(t, err)
	t.Cleanup(first.Release)
	second, err := parent.Timeline.ForkForTask("second", "second", parent, parent)
	require.NoError(t, err)
	t.Cleanup(second.Release)
	// The child has a distinct prompt state and no persistent session ID.
	opts := append(ConvertConfigToOptionsWithoutHotPatch(parent), WithTimeline(first.Branch),
		WithSessionPromptState(parent.SessionPromptState.ForkForTaskOnlySubAgent()), WithPersistentSessionId(""))
	child := newTimelineBroadcastConfig(t, opts...)
	unrelated := newTimelineBroadcastConfig(t)
	require.Same(t, parent.TimelineBroadcaster, child.TimelineBroadcaster)
	first.Branch.PushText(child.AcquireId(), "private child history")
	require.NotContains(t, parent.Timeline.Dump(), "private child history")
	require.NotContains(t, second.Branch.Dump(), "private child history")

	item := NewTimelineItem(&TextTimelineItem{ID: child.AcquireId(), Text: "shared discovery"})
	require.NoError(t, child.BroadcastTimelineItem(item))
	for _, timeline := range []*Timeline{parent.Timeline, first.Branch, second.Branch} {
		received := requireBroadcastItem(t, timeline, item.GetID())
		require.Equal(t, item.historyID, received.historyID)
		require.True(t, item.createdAt.Equal(received.createdAt))
		require.Contains(t, timeline.Dump(), "shared discovery")
	}
	require.NotContains(t, unrelated.Timeline.Dump(), "shared discovery")
	require.NoError(t, parent.BroadcastTimelineText("parent update"))
	require.Contains(t, first.Branch.Dump(), "parent update")
	require.Contains(t, second.Branch.Dump(), "parent update")

	third, err := parent.Timeline.ForkForTask("later", "later", parent, parent)
	require.NoError(t, err)
	t.Cleanup(third.Release)
	require.Contains(t, third.Branch.Dump(), "shared discovery")
	before := len(third.Branch.GetTimelineItemIDs())
	require.NoError(t, child.BroadcastTimelineItem(item))
	require.Len(t, third.Branch.GetTimelineItemIDs(), before)
	count, _ := broadcastSubscriptionCounts(parent.TimelineBroadcaster)
	require.Equal(t, 4, count)
}

func TestTimelineBroadcastBindingAndReferenceCounts(t *testing.T) {
	parent := newTimelineBroadcastConfig(t)
	shared := newTimelineBroadcastConfig(t, ConvertConfigToOptionsWithoutHotPatch(parent)...)
	old := parent.Timeline
	for i := 0; i < 3; i++ {
		require.NoError(t, WithTimeline(old)(shared))
	}
	count, refs := broadcastSubscriptionCounts(parent.TimelineBroadcaster)
	require.Equal(t, 1, count)
	require.Equal(t, 2, refs)
	legacy := newTimelineBroadcastConfig(t, WithTimeline(old), WithSeqIdProvider(parent.SeqIdProvider))
	require.Same(t, parent.TimelineBroadcaster, legacy.TimelineBroadcaster)
	legacy.ReleaseTimelineSubscription()

	next := NewPrivateTimeline(parent, nil)
	require.NoError(t, parent.SetTimeline(next))
	require.NoError(t, parent.BroadcastTimelineText("both active containers"))
	require.Contains(t, old.Dump(), "both active containers")
	require.Contains(t, next.Dump(), "both active containers")
	shared.ReleaseTimelineSubscription()
	shared.ReleaseTimelineSubscription()
	require.NoError(t, parent.BroadcastTimelineText("new container only"))
	require.NotContains(t, old.Dump(), "new container only")
	count, refs = broadcastSubscriptionCounts(parent.TimelineBroadcaster)
	require.Equal(t, 1, count)
	require.Equal(t, 1, refs)

	// Options may select a timeline before they select its session broadcaster.
	selected := NewTimeline(nil, nil)
	joined := newTimelineBroadcastConfig(t, WithTimeline(selected), WithTimelineBroadcaster(parent.TimelineBroadcaster))
	require.Same(t, selected, joined.Timeline)
	require.NoError(t, parent.BroadcastTimelineText("late option join"))
	require.Contains(t, selected.Dump(), "late option join")

	fork, err := next.ForkForTask("released", "released", parent, parent)
	require.NoError(t, err)
	fork.Release()
	fork.Release()
	require.NoError(t, parent.BroadcastTimelineText("after fork release"))
	require.NotContains(t, fork.Branch.Dump(), "after fork release")

	ctx, cancel := context.WithCancel(context.Background())
	cancelled := newTimelineBroadcastConfig(t, WithTimelineBroadcaster(parent.TimelineBroadcaster), WithContext(ctx))
	cancel()
	require.Eventually(t, func() bool { return cancelled.Timeline.getTimelineBroadcaster() == nil }, time.Second, time.Millisecond)
	require.ErrorIs(t, cancelled.BroadcastTimelineText("cancelled"), context.Canceled)
}

func TestTimelineBroadcastRuntimeIDsAndReadViews(t *testing.T) {
	parent := newTimelineBroadcastConfig(t)
	fork, err := parent.Timeline.ForkForTask("fork", "fork", parent, parent)
	require.NoError(t, err)
	t.Cleanup(fork.Release)
	raw, err := MarshalTimeline(parent.Timeline)
	require.NoError(t, err)
	require.NotContains(t, raw, parent.Timeline.GetTimelineID())
	restored, err := UnmarshalTimeline(raw)
	require.NoError(t, err)
	view := parent.Timeline.CopyReducibleTimelineWithMemory()
	sub := parent.Timeline.CreateSubTimeline()
	ids := map[string]bool{}
	for _, timeline := range []*Timeline{parent.Timeline, fork.Branch, restored, view, sub} {
		id := timeline.GetTimelineID()
		require.NotEmpty(t, id)
		require.False(t, ids[id])
		ids[id] = true
	}
	helper := newTimelineBroadcastConfig(t, append(ConvertConfigToOptionsWithoutHotPatch(parent),
		WithTimeline(view), WithDisableTimelineBroadcast(true))...)
	require.Same(t, parent.TimelineBroadcaster, helper.TimelineBroadcaster)
	require.NoError(t, parent.BroadcastTimelineText("live containers only"))
	require.NotContains(t, view.Dump(), "live containers only")
	require.NotContains(t, sub.Dump(), "live containers only")
	require.NotContains(t, restored.Dump(), "live containers only")
}

func TestTimelineBroadcastCopiesCallbacksAndUI(t *testing.T) {
	var uiEvents atomic.Int64
	uiDone := make(chan struct{}, 10)
	emitter := NewEmitter("broadcast", func(e *schema.AiOutputEvent) (*schema.AiOutputEvent, error) {
		if e.NodeId == "timeline_item" {
			uiEvents.Add(1)
			uiDone <- struct{}{}
		}
		return e, nil
	})
	parent := newTimelineBroadcastConfig(t, WithEmitter(emitter))
	fork, err := parent.Timeline.ForkForTask("child", "child", parent, parent)
	require.NoError(t, err)
	t.Cleanup(fork.Release)
	var parentInputs, childInputs atomic.Int64
	parent.Timeline.RegisterItemInputCallback("count", func(TimelineItemInputEvent) { parentInputs.Add(1) })
	fork.Branch.RegisterItemInputCallback("count", func(TimelineItemInputEvent) { childInputs.Add(1) })
	item := NewTimelineItem(&aitool.ToolResult{ID: parent.AcquireId(), Name: "read", Data: map[string]any{"body": "original"}})
	require.NoError(t, parent.BroadcastTimelineItem(item))
	select {
	case <-uiDone:
	case <-time.After(time.Second):
		t.Fatal("missing broadcast UI event")
	}
	require.EqualValues(t, 1, uiEvents.Load())
	require.EqualValues(t, 1, parentInputs.Load())
	require.EqualValues(t, 1, childInputs.Load())
	rootCopy := requireBroadcastItem(t, parent.Timeline, item.GetID())
	childCopy := requireBroadcastItem(t, fork.Branch, item.GetID())
	require.NotSame(t, rootCopy, childCopy)
	childCopy.SetShrinkResult("child shrink")
	childCopy.value.(*aitool.ToolResult).Data.(map[string]any)["body"] = "child edit"
	require.Empty(t, rootCopy.GetShrinkResult())
	require.Equal(t, "original", rootCopy.value.(*aitool.ToolResult).Data.(map[string]any)["body"])

	// Reentry can read other recipients and publish another item: no callback
	// executes while the broadcaster or any recipient is locked.
	parent.Timeline.RegisterItemInputCallback("reentry", func(event TimelineItemInputEvent) {
		if event.ID == item.GetID()+1 {
			requireBroadcastItem(t, fork.Branch, event.ID)
			require.NoError(t, parent.BroadcastTimelineText("nested broadcast"))
		}
	})
	require.NoError(t, parent.BroadcastTimelineText("trigger reentry"))
	require.Contains(t, fork.Branch.Dump(), "nested broadcast")
}

func TestTimelineBroadcastConflictIsAtomicAndRetriesStayRetired(t *testing.T) {
	parent := newTimelineBroadcastConfig(t)
	fork, err := parent.Timeline.ForkForTask("child", "child", parent, parent)
	require.NoError(t, err)
	t.Cleanup(fork.Release)
	id := parent.AcquireId()
	fork.Branch.PushText(id, "private conflicting ID")
	require.Error(t, parent.BroadcastTimelineItem(NewTimelineItem(&TextTimelineItem{ID: id, Text: "must not leak"})))
	require.NotContains(t, parent.Timeline.Dump(), "must not leak")
	require.NotContains(t, fork.Branch.Dump(), "must not leak")

	item := NewTimelineItem(&TextTimelineItem{ID: parent.AcquireId(), Text: "retired broadcast"})
	require.NoError(t, parent.BroadcastTimelineItem(item))
	parent.Timeline.SoftDelete(item.GetID())
	require.NoError(t, fork.Branch.TruncateAfter(id))
	require.NoError(t, parent.BroadcastTimelineItem(item))
	require.NotContains(t, parent.Timeline.Dump(), "retired broadcast")
	require.NotContains(t, fork.Branch.Dump(), "retired broadcast")
	require.Error(t, parent.BroadcastTimelineItem(nil))
	require.Error(t, parent.BroadcastTimelineItem(NewTimelineItem(&TextTimelineItem{})))
}

func TestTimelineBroadcastExistingItemAndMergeAfterCompression(t *testing.T) {
	parent := newTimelineBroadcastConfig(t)
	fork, err := parent.Timeline.ForkForTask("child", "child", parent, parent)
	require.NoError(t, err)
	t.Cleanup(fork.Release)
	id := parent.AcquireId()
	parent.Timeline.PushText(id, "existing discovery")
	item := requireBroadcastItem(t, parent.Timeline, id)
	require.NoError(t, parent.BroadcastTimelineItem(item))
	require.Equal(t, 1, strings.Count(parent.Timeline.Dump(), "existing discovery"))
	branchID := parent.AcquireId()
	fork.Branch.PushText(branchID, "private merge result")
	result, err := fork.MergeBack()
	require.NoError(t, err)
	require.Equal(t, 1, result.ActiveItemsMerged)
	require.Equal(t, 1, strings.Count(parent.Timeline.Dump(), "existing discovery"))

	compressed, err := parent.Timeline.ForkForTask("compressed", "compressed", parent, parent)
	require.NoError(t, err)
	t.Cleanup(compressed.Release)
	shared := NewTimelineItem(&TextTimelineItem{ID: parent.AcquireId(), Text: "broadcast at summary boundary"})
	require.NoError(t, parent.BroadcastTimelineItem(shared))
	// Model a committed compression on each side, including retirement of the
	// original. The delivery receipts must survive independently of live items.
	for _, timeline := range []*Timeline{parent.Timeline, compressed.Branch} {
		timeline.mu.Lock()
		timeline.retireTimelineItemLocked(shared.GetID())
		timeline.compressedHead = &TimelineCompressedHead{Text: "compressed facts", CoveredEndItemID: shared.GetID(), Version: 1}
		timeline.mu.Unlock()
	}
	require.NoError(t, parent.BroadcastTimelineItem(shared))
	result, err = compressed.MergeBack()
	require.NoError(t, err)
	require.Equal(t, 1, result.CompressedHeadsMerged)
	require.Contains(t, parent.Timeline.Dump(), "[subtask_summary]")
	require.Greater(t, parent.Timeline.GetMaxID(), shared.GetID())
}

func TestTimelineBroadcastConcurrentForkAndDelivery(t *testing.T) {
	parent := newTimelineBroadcastConfig(t)
	const total = 30
	var wg sync.WaitGroup
	errors := make(chan error, total*2)
	forks := make(chan *TimelineFork, total)
	start := make(chan struct{})
	for i := 0; i < total; i++ {
		wg.Add(2)
		go func(i int) {
			defer wg.Done()
			<-start
			errors <- parent.BroadcastTimelineText(fmt.Sprintf("concurrent-message-%02d", i))
		}(i)
		go func(i int) {
			defer wg.Done()
			<-start
			fork, err := parent.Timeline.ForkForTask(fmt.Sprint(i), "concurrent fork", parent, parent)
			errors <- err
			if fork != nil {
				forks <- fork
			}
		}(i)
	}
	close(start)
	wg.Wait()
	close(errors)
	close(forks)
	for err := range errors {
		require.NoError(t, err)
	}
	for fork := range forks {
		t.Cleanup(fork.Release)
		require.Len(t, fork.Branch.GetTimelineItemIDs(), total)
		for _, id := range parent.Timeline.GetTimelineItemIDs() {
			root := requireBroadcastItem(t, parent.Timeline, id)
			child := requireBroadcastItem(t, fork.Branch, id)
			require.Equal(t, root.historyID, child.historyID)
		}
		// Every fork snapshot remains serializable after concurrent fanout.
		raw, err := MarshalTimeline(fork.Branch)
		require.NoError(t, err)
		require.True(t, json.Valid([]byte(raw)))
	}
}
