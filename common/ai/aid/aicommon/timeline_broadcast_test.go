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

	sharedID := timelineItemIDContaining(parent.Timeline, "shared update")
	require.NotZero(t, sharedID)
	requireTimelineText(t, parent.Timeline, sharedID, "shared update")
	requireTimelineText(t, fork.Branch, sharedID, "shared update")
	requireTimelineText(t, clean, sharedID, "shared update")
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

	id := timelineItemIDContaining(parent.Timeline, "after forks ended")
	require.NotZero(t, id)

	releasedFork.Branch.mu.RLock()
	_, releasedHasItem := releasedFork.Branch.idToTimelineItem.Get(id)
	releasedFork.Branch.mu.RUnlock()
	require.False(t, releasedHasItem)

	mergedFork.Branch.mu.RLock()
	_, mergedHasItem := mergedFork.Branch.idToTimelineItem.Get(id)
	mergedFork.Branch.mu.RUnlock()
	require.False(t, mergedHasItem)
}

func TestTimelineBroadcasterRegisterAndUnregister(t *testing.T) {
	broadcaster := NewTimelineBroadcaster()
	var received int64
	broadcaster.Register("test", func(item *TimelineItem) error {
		received = item.GetID()
		return nil
	})

	require.NoError(t, broadcaster.Broadcast(NewTimelineItem(&TextTimelineItem{ID: 7, Text: "event"})))
	require.Equal(t, int64(7), received)

	broadcaster.Unregister("test")
	received = 0
	require.NoError(t, broadcaster.Broadcast(NewTimelineItem(&TextTimelineItem{ID: 8, Text: "ignored"})))
	require.Zero(t, received)
}

func TestTimelineBroadcastDoesNotMutateHistoricalItem(t *testing.T) {
	parent := newTimelineBroadcastConfig(t)
	fork, err := parent.Timeline.ForkForTask("task", "task", parent, parent)
	require.NoError(t, err)
	t.Cleanup(fork.Release)

	item := NewTimelineItem(&TextTimelineItem{ID: parent.AcquireId(), Text: "immutable fact"})
	require.NoError(t, parent.BroadcastTimelineItem(item))
	require.NoError(t, parent.BroadcastTimelineItem(item))

	requireTimelineText(t, parent.Timeline, item.GetID(), "immutable fact")
	requireTimelineText(t, fork.Branch, item.GetID(), "immutable fact")

	parent.Timeline.mu.RLock()
	parentHistoryID := parent.Timeline.idToTimelineItem.Values()[0].historyID
	parent.Timeline.mu.RUnlock()
	fork.Branch.mu.RLock()
	branchHistoryID := fork.Branch.idToTimelineItem.Values()[0].historyID
	fork.Branch.mu.RUnlock()
	require.Equal(t, parentHistoryID, branchHistoryID)
}

func TestTimelineForkWaitsForInFlightBroadcast(t *testing.T) {
	config := newTimelineBroadcastConfig(t)
	started := make(chan struct{})
	release := make(chan struct{})
	config.TimelineBroadcaster.Register("test", func(*TimelineItem) error {
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
