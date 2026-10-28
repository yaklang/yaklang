package aicommon

import (
	"fmt"
	"testing"

	"github.com/google/uuid"
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
	evidence := uuid.NewString()
	require.True(t, tl.PushPromotable(2, TimelinePromotedKindEvidence, TimelinePromotedTargetSemiDynamic1,
		"e1", TimelinePromotedOperationUpsert, fmt.Sprintf(`{"id":"e1","content":%q}`, evidence)))
	_, err := tl.ApplyCompressionSelection(&TimelineCompressionSelection{Summary: "summary"}, "")
	require.NoError(t, err)
	require.Contains(t, RenderTimelineFrozenOpen(tl).PromptText(), evidence)
}
