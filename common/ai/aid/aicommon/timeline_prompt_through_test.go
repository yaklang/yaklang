package aicommon

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestTimelinePromptThroughIDOmitsLaterHistory(t *testing.T) {
	timeline := NewTimeline(nil, nil)
	kept, dropped := uuid.NewString(), uuid.NewString()
	timeline.PushText(1, "%s", kept)
	timeline.PushText(2, "%s", dropped)
	capID := int64(1)
	view := RenderTimelineFrozenOpenWithOptions(timeline, TimelinePromptOptions{ThroughID: &capID})
	require.Equal(t, capID, view.ThroughID)
	require.Equal(t, []int64{1}, view.ItemIDs)
	require.Contains(t, view.PromptText(), kept)
	require.NotContains(t, view.PromptText(), dropped)

	guide, err := timeline.RenderCompressionGuide(TimelineCompressionOptions{
		Context: context.Background(), TargetTokens: 8, ThroughID: &capID,
	})
	require.NoError(t, err)
	require.NotContains(t, guide, dropped)
}

func TestTimelinePromptThroughIDOnlyCapsOpen(t *testing.T) {
	timeline := NewTimeline(nil, nil)
	frozenA, frozenB := uuid.NewString(), uuid.NewString()
	openKept, openDropped := uuid.NewString(), uuid.NewString()
	timeline.PushText(1, "%s", frozenA)
	timeline.PushText(2, "%s", frozenB)
	timeline.FreezeAll()
	timeline.PushText(3, "%s", openKept)
	timeline.PushText(4, "%s", openDropped)
	capID := int64(3)
	view := RenderTimelineFrozenOpenWithOptions(timeline, TimelinePromptOptions{ThroughID: &capID})
	require.Equal(t, capID, view.ThroughID)
	require.Equal(t, []int64{1, 2, 3}, view.ItemIDs)
	require.Contains(t, view.Frozen, frozenA)
	require.Contains(t, view.Frozen, frozenB)
	require.Contains(t, view.Open, openKept)
	require.NotContains(t, view.Open, openDropped)
}

func TestCompressionBoundaryCapsItemIDs(t *testing.T) {
	timeline := NewTimeline(nil, nil)
	timeline.PushText(1, "%s", uuid.NewString())
	timeline.PushText(2, "%s", uuid.NewString())
	through, ids := timeline.CompressionBoundary(1)
	require.Equal(t, int64(1), through)
	require.Equal(t, []int64{1}, ids)
}
