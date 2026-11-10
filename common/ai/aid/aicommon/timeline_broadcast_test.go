package aicommon

import (
	"context"
	"strings"
	"testing"

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
	releaseClean, err := parent.SubscribeTimeline(clean)
	require.NoError(t, err)
	t.Cleanup(releaseClean)

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

func TestTimelineBroadcastUnsubscribesOnReleaseAndMerge(t *testing.T) {
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

func TestTimelineBroadcasterSubscribeAndUnsubscribe(t *testing.T) {
	broadcaster := NewTimelineBroadcaster()
	var received int64
	id := broadcaster.Subscribe(func(item *TimelineItem) error {
		received = item.GetID()
		return nil
	})

	require.NoError(t, broadcaster.Broadcast(NewTimelineItem(&TextTimelineItem{ID: 7, Text: "event"})))
	require.Equal(t, int64(7), received)

	broadcaster.Unsubscribe(id)
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

func TestConfigSetTimelineMovesBroadcastSubscription(t *testing.T) {
	config := newTimelineBroadcastConfig(t)
	previous := config.Timeline
	next := NewTimeline(config, nil)
	require.NoError(t, config.SetTimeline(next))

	require.NoError(t, config.BroadcastTimelineText("after switch"))

	require.NotZero(t, timelineItemIDContaining(next, "after switch"))
	require.Zero(t, timelineItemIDContaining(previous, "after switch"))
}
