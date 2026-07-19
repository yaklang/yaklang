package aicommon

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRecentTimelineForValueFeedbackIsBoundedAndKeepsNewestFacts(t *testing.T) {
	timeline := NewTimeline(nil, nil)
	timeline.PushText(1, "[note]:\nANCIENT_VALUE_FEEDBACK_FACT "+strings.Repeat("old-payload ", 12000))
	timeline.PushText(2, "[note]:\nRECENT_VALUE_FEEDBACK_FACT")

	projected := recentTimelineForValueFeedback(timeline)
	require.LessOrEqual(t, MeasureTokens(projected), ValueFeedbackRecentTimelineTokens)
	require.Contains(t, projected, "RECENT_VALUE_FEEDBACK_FACT")
	require.NotContains(t, projected, "ANCIENT_VALUE_FEEDBACK_FACT")
	require.Len(t, timeline.GetTimelineItemIDs(), 2, "projection must not delete source Timeline items")
}
