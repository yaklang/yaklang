package aicommon

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func pushCompressionTool(t *testing.T, tl *Timeline, id int64, key, operation, payload string) {
	t.Helper()
	require.True(t, tl.PushPromotable(id, TimelinePromotedKindRecentTool, TimelinePromotedTargetSemiDynamic1,
		key, operation, payload))
}

func TestTimelineCompressionPromotedSelectionDefaultsToKeep(t *testing.T) {
	tl := NewTimeline(nil, nil)
	tl.PushText(1, "ordinary")
	require.True(t, tl.PushPromotable(2, TimelinePromotedKindEvidence, TimelinePromotedTargetSemiDynamic1,
		"e1", TimelinePromotedOperationUpsert, `{"id":"e1","content":"evidence"}`))
	_, err := tl.ApplyCompressionSelection(&TimelineCompressionSelection{Summary: "summary"}, "")
	require.NoError(t, err)
	require.Contains(t, RenderTimelineFrozenOpen(tl).PromptText(), "e1")
}
