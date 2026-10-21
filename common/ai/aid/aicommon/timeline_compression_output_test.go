package aicommon

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCompressionCommitsScatteredRanges(t *testing.T) {
	tl := NewTimeline(nil, nil)
	for _, id := range []int64{3, 5, 8, 10, 12, 20, 23, 26, 30, 37, 41, 45} {
		tl.PushText(id, "original-%d", id)
	}
	tl.PushUserInteraction(UserInteractionStage_FreeInput, 6, "", "PROTECTED_USER_INPUT")
	pushCompressionTool(t, tl, 21, "protected-tool", TimelinePromotedOperationUpsert, "PROTECTED_TOOL_SCHEMA")
	require.True(t, tl.PushPromotable(22, TimelinePromotedKindEvidence, TimelinePromotedTargetSemiDynamic1,
		"e1", TimelinePromotedOperationUpsert, `{"id":"e1","content":"PROTECTED_EVIDENCE"}`))
	// Apply reads the timeline at apply time. An ordinary entry that is present
	// and not retained is retired, even if it was appended after the guide.
	tl.PushText(50, "CONCURRENT_NEW_ENTRY")
	const selection = "3-8,12,20-26,37,41-45"
	result, err := tl.ApplyCompressionSelection(&TimelineCompressionSelection{
		Summary: "Exploration 10 and 30 was superseded; regression work remains.", RetainedRange: selection,
		RetainedIDs: []int64{3, 5, 8, 12, 20, 23, 26, 37, 41, 45},
	}, "")
	require.NoError(t, err)
	require.Equal(t, []int64{3, 5, 8, 12, 20, 23, 26, 37, 41, 45}, result.RetainedIDs)
	require.Equal(t, []int64{10, 30, 50}, result.RetiredIDs)
	require.Equal(t, selection, result.RetainedRange)
	view := RenderTimelineFrozenOpen(tl)
	for _, text := range []string{"PROTECTED_USER_INPUT", "PROTECTED_TOOL_SCHEMA", "PROTECTED_EVIDENCE"} {
		require.Contains(t, view.PromptText(), text)
	}
	require.NotContains(t, view.PromptText(), "CONCURRENT_NEW_ENTRY")
	restored, err := MarshalTimeline(tl)
	require.NoError(t, err)
	copy, err := UnmarshalTimeline(restored)
	require.NoError(t, err)
	require.Equal(t, view, RenderTimelineFrozenOpen(copy))
	for _, id := range result.RetainedIDs {
		require.Contains(t, view.Frozen, fmt.Sprintf("original-%d", id))
	}
}
