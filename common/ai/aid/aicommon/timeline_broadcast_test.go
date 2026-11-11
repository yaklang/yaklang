package aicommon

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func newTimelineBroadcastConfig(t *testing.T, opts ...ConfigOption) *Config {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	opts = append([]ConfigOption{WithContext(ctx), WithDisableAutoSkills(true)}, opts...)
	return NewConfig(ctx, opts...)
}

func requireTimelineText(t *testing.T, timeline *Timeline, id int64, text string) {
	t.Helper()
	timeline.mu.RLock()
	defer timeline.mu.RUnlock()
	item, ok := timeline.idToTimelineItem.Get(id)
	require.True(t, ok)
	require.NotNil(t, item)
	require.Contains(t, item.String(), text)
}

func timelineItemIDContaining(timeline *Timeline, text string) int64 {
	var id int64
	timeline.mu.RLock()
	timeline.idToTimelineItem.ForEach(func(itemID int64, item *TimelineItem) bool {
		if item != nil && strings.Contains(item.String(), text) {
			id = itemID
			return false
		}
		return true
	})
	timeline.mu.RUnlock()
	return id
}

func timelineItemCountContaining(timeline *Timeline, text string) int {
	count := 0
	timeline.mu.RLock()
	timeline.idToTimelineItem.ForEach(func(_ int64, item *TimelineItem) bool {
		if item != nil && strings.Contains(item.String(), text) {
			count++
		}
		return true
	})
	timeline.mu.RUnlock()
	return count
}

func TestTimelineBroadcastReachesParentForkAndTaskOnlyTimeline(t *testing.T) {
	parent := newTimelineBroadcastConfig(t)
	unrelated := newTimelineBroadcastConfig(t)
	parent.Timeline.PushText(parent.AcquireId(), "history before fork")

	fork, err := parent.Timeline.ForkForTask("task", "task", parent, parent)
	require.NoError(t, err)
	t.Cleanup(fork.Release)

	clean := NewPrivateTimeline(parent, nil)
	clean.SoftBindConfig(parent, parent)
	unregisterClean, err := parent.RegisterTimeline(clean)
	require.NoError(t, err)
	t.Cleanup(unregisterClean)

	require.Empty(t, clean.GetTimelineItemIDs(), "task-only timelines start clean")

	require.NoError(t, parent.BroadcastTimelineText("shared update"))

	parentID := timelineItemIDContaining(parent.Timeline, "shared update")
	forkID := timelineItemIDContaining(fork.Branch, "shared update")
	cleanID := timelineItemIDContaining(clean, "shared update")
	require.NotZero(t, parentID)
	require.NotZero(t, forkID)
	require.NotZero(t, cleanID)
	require.NotEqual(t, parentID, forkID)
	require.NotEqual(t, parentID, cleanID)
	requireTimelineText(t, parent.Timeline, parentID, "shared update")
	requireTimelineText(t, fork.Branch, forkID, "shared update")
	requireTimelineText(t, clean, cleanID, "shared update")
	require.NotContains(t, clean.Dump(), "history before fork")
	require.Zero(t, timelineItemIDContaining(unrelated.Timeline, "shared update"))
}

func TestTimelineBroadcastUnregistersOnReleaseAndMerge(t *testing.T) {
	parent := newTimelineBroadcastConfig(t)
	releasedFork, err := parent.Timeline.ForkForTask("released", "released", parent, parent)
	require.NoError(t, err)
	releasedFork.Release()

	mergedFork, err := parent.Timeline.ForkForTask("merged", "merged", parent, parent)
	require.NoError(t, err)
	require.NoError(t, parent.BroadcastTimelineText("shared before merge"))
	result, err := mergedFork.MergeBack()
	require.NoError(t, err)
	require.NotNil(t, result)

	require.NoError(t, parent.BroadcastTimelineText("after forks ended"))

	require.NotZero(t, timelineItemIDContaining(parent.Timeline, "after forks ended"))
	require.Zero(t, timelineItemCountContaining(releasedFork.Branch, "after forks ended"))
	require.Zero(t, timelineItemCountContaining(mergedFork.Branch, "after forks ended"))
}

func TestTimelineBroadcasterRegisterAndUnregister(t *testing.T) {
	broadcaster := NewTimelineBroadcaster()
	var received []int64
	broadcaster.Register("test", func(items ...*TimelineItem) error {
		for _, item := range items {
			received = append(received, item.GetID())
		}
		return nil
	})

	require.NoError(t, broadcaster.Broadcast(
		NewTimelineItem(&TextTimelineItem{Text: "event-1"}),
		NewTimelineItem(&TextTimelineItem{Text: "event-2"}),
	))
	require.Equal(t, []int64{0, 0}, received)

	broadcaster.Unregister("test")
	received = nil
	require.NoError(t, broadcaster.Broadcast(NewTimelineItem(&TextTimelineItem{Text: "ignored"})))
	require.Empty(t, received)
}

func TestTimelineBroadcastAssignsLocalIDsAndDeduplicatesByHistory(t *testing.T) {
	parent := newTimelineBroadcastConfig(t)
	fork, err := parent.Timeline.ForkForTask("task", "task", parent, parent)
	require.NoError(t, err)
	t.Cleanup(fork.Release)

	item := NewTimelineItem(&TextTimelineItem{Text: "immutable fact"})
	require.NoError(t, parent.BroadcastTimelineItem(item))
	require.NoError(t, parent.BroadcastTimelineItem(item))

	parentID := timelineItemIDContaining(parent.Timeline, "immutable fact")
	branchID := timelineItemIDContaining(fork.Branch, "immutable fact")
	require.NotZero(t, parentID)
	require.NotZero(t, branchID)
	require.NotEqual(t, parentID, branchID)
	require.Equal(t, 1, timelineItemCountContaining(parent.Timeline, "immutable fact"))
	require.Equal(t, 1, timelineItemCountContaining(fork.Branch, "immutable fact"))

	parent.Timeline.mu.RLock()
	parentHistoryID := parent.Timeline.idToTimelineItem.Values()[0].historyID
	parent.Timeline.mu.RUnlock()
	fork.Branch.mu.RLock()
	branchHistoryID := fork.Branch.idToTimelineItem.Values()[0].historyID
	fork.Branch.mu.RUnlock()
	require.Equal(t, parentHistoryID, branchHistoryID)
}

func TestTimelineBroadcastRejectsNonEmptyItemID(t *testing.T) {
	config := newTimelineBroadcastConfig(t)
	err := config.BroadcastTimelineItem(NewTimelineItem(&TextTimelineItem{ID: 7, Text: "invalid"}))
	require.ErrorContains(t, err, "item ID must be empty")
	require.Zero(t, timelineItemCountContaining(config.Timeline, "invalid"))
}

func TestTimelineBroadcastBatchAssignsLocalIDs(t *testing.T) {
	config := newTimelineBroadcastConfig(t)
	fork, err := config.Timeline.ForkForTask("batch", "batch", config, config)
	require.NoError(t, err)
	t.Cleanup(fork.Release)

	first := NewTimelineItem(&TextTimelineItem{Text: "batch first"})
	second := NewTimelineItem(&TextTimelineItem{Text: "batch second"})
	require.NoError(t, config.BroadcastTimelineItem(first, second))

	for _, timeline := range []*Timeline{config.Timeline, fork.Branch} {
		require.NotZero(t, timelineItemIDContaining(timeline, "batch first"))
		require.NotZero(t, timelineItemIDContaining(timeline, "batch second"))
	}
	require.NotEqual(
		t,
		timelineItemIDContaining(config.Timeline, "batch first"),
		timelineItemIDContaining(fork.Branch, "batch first"),
	)
}

func TestTimelineBroadcastValidatesBatchBeforeDispatch(t *testing.T) {
	config := newTimelineBroadcastConfig(t)
	valid := NewTimelineItem(&TextTimelineItem{Text: "must stay absent"})
	invalid := NewTimelineItem(&TextTimelineItem{ID: 9, Text: "invalid"})

	err := config.BroadcastTimelineItem(valid, invalid)
	require.ErrorContains(t, err, "item 1")
	require.Zero(t, timelineItemCountContaining(config.Timeline, "must stay absent"))
}

func TestTimelineBroadcastChecksEvidenceAgainstEachTimeline(t *testing.T) {
	parent := newTimelineBroadcastConfig(t)
	seed, err := BuildSessionEvidenceUpsert("shared-fact", "FIRST")
	require.NoError(t, err)
	require.True(t, parent.ApplySessionEvidenceOpsWithChange([]EvidenceOperation{seed}))

	fork, err := parent.Timeline.ForkForTask("evidence", "evidence", parent, parent)
	require.NoError(t, err)
	t.Cleanup(fork.Release)

	clean := NewPrivateTimeline(parent, nil)
	clean.SoftBindConfig(parent, parent)
	unregisterClean, err := parent.RegisterTimeline(clean)
	require.NoError(t, err)
	t.Cleanup(unregisterClean)

	parentMax, forkMax := parent.Timeline.GetMaxID(), fork.Branch.GetMaxID()
	require.NoError(t, parent.BroadcastTimelineItem(newEvidenceTestItem(t, seed.ID, seed.Content)))
	require.Equal(t, parentMax, parent.Timeline.GetMaxID())
	require.Equal(t, forkMax, fork.Branch.GetMaxID())
	cleanStore, found := clean.evidenceStore()
	require.True(t, found)
	require.Contains(t, cleanStore.Render(), "FIRST")

	updated, err := BuildSessionEvidenceUpsert("shared-fact", "SECOND")
	require.NoError(t, err)
	require.NoError(t, parent.BroadcastTimelineItem(newEvidenceTestItem(t, updated.ID, updated.Content)))
	for _, timeline := range []*Timeline{parent.Timeline, fork.Branch, clean} {
		store, found := timeline.evidenceStore()
		require.True(t, found)
		require.Contains(t, store.Render(), "SECOND")
	}
}

func TestSessionEvidenceLocalApplyStaysOnOwningTimeline(t *testing.T) {
	parent := newTimelineBroadcastConfig(t)
	fork, err := parent.Timeline.ForkForTask("local-evidence", "local evidence", parent, parent)
	require.NoError(t, err)
	t.Cleanup(fork.Release)

	require.True(t, parent.ApplySessionEvidenceOpsWithChange([]EvidenceOperation{{
		Op:      "add",
		ID:      "local-only",
		Content: "this finding belongs to the parent timeline only",
	}}))
	parentStore, found := parent.Timeline.evidenceStore()
	require.True(t, found)
	require.Contains(t, parentStore.Render(), "local-only")

	forkStore, found := fork.Branch.evidenceStore()
	require.False(t, found)
	require.NotContains(t, forkStore.Render(), "local-only")
}

func TestTimelineForkWaitsForInFlightBroadcast(t *testing.T) {
	config := newTimelineBroadcastConfig(t)
	started := make(chan struct{})
	release := make(chan struct{})
	config.TimelineBroadcaster.Register("test", func(...*TimelineItem) error {
		close(started)
		<-release
		return nil
	})
	t.Cleanup(func() { config.TimelineBroadcaster.Unregister("test") })

	broadcastDone := make(chan error, 1)
	go func() {
		broadcastDone <- config.BroadcastTimelineText("broadcast during fork")
	}()
	<-started

	type forkResult struct {
		fork *TimelineFork
		err  error
	}
	forkDone := make(chan forkResult, 1)
	go func() {
		fork, err := config.Timeline.ForkForTask("task", "task", config, config)
		forkDone <- forkResult{fork: fork, err: err}
	}()

	select {
	case <-forkDone:
		t.Fatal("fork completed while a publication was still in flight")
	case <-time.After(50 * time.Millisecond):
	}

	close(release)
	require.NoError(t, <-broadcastDone)
	result := <-forkDone
	require.NoError(t, result.err)
	require.NotNil(t, result.fork)
	t.Cleanup(result.fork.Release)
	require.Contains(t, result.fork.Branch.Dump(), "broadcast during fork")
}

func TestConfigSetTimelineMovesBroadcastRegistration(t *testing.T) {
	config := newTimelineBroadcastConfig(t)
	previous := config.Timeline
	peerTimeline := NewTimeline(config, nil)
	peer := NewConfig(context.Background(),
		WithID(config.Id),
		WithTimelineBroadcaster(config.TimelineBroadcaster),
		WithTimeline(peerTimeline),
		WithDisableAutoSkills(true),
	)
	t.Cleanup(peer.UnregisterTimeline)
	next := NewTimeline(config, nil)
	require.NoError(t, config.SetTimeline(next))

	require.NoError(t, config.BroadcastTimelineText("after switch"))

	require.NotZero(t, timelineItemIDContaining(next, "after switch"))
	require.NotZero(t, timelineItemIDContaining(peerTimeline, "after switch"))
	require.Zero(t, timelineItemIDContaining(previous, "after switch"))
}
