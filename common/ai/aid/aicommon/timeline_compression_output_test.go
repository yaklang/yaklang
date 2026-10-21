package aicommon

import (
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// The action layer expands retain_timeline into IDs before apply; this test
// covers the apply semantics: unretained history is retired while protected
// materials and retained entries survive.
func TestCompressionApplyRetiresUnretainedHistory(t *testing.T) {
	tl := NewTimeline(nil, nil)
	originalText := map[int64]string{}
	for _, id := range []int64{3, 5, 8, 10, 12, 20, 23, 26, 30, 37, 41, 45} {
		originalText[id] = uuid.NewString()
		tl.PushText(id, "%s", originalText[id])
	}
	protectedUserInput := uuid.NewString()
	protectedToolSchema := uuid.NewString()
	protectedEvidence := uuid.NewString()
	tl.PushUserInteraction(UserInteractionStage_FreeInput, 6, "", protectedUserInput)
	pushCompressionTool(t, tl, 21, "protected-tool", TimelinePromotedOperationUpsert, protectedToolSchema)
	require.True(t, tl.PushPromotable(22, TimelinePromotedKindEvidence, TimelinePromotedTargetSemiDynamic1,
		"e1", TimelinePromotedOperationUpsert, fmt.Sprintf(`{"id":"e1","content":%q}`, protectedEvidence)))
	// Apply reads the timeline at apply time. An ordinary entry that is present
	// and not retained is retired, even if it was appended after the guide.
	concurrentEntry := uuid.NewString()
	tl.PushText(50, "%s", concurrentEntry)
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
	for _, text := range []string{protectedUserInput, protectedToolSchema, protectedEvidence} {
		require.Contains(t, view.PromptText(), text)
	}
	require.NotContains(t, view.PromptText(), concurrentEntry)
	restored, err := MarshalTimeline(tl)
	require.NoError(t, err)
	copy, err := UnmarshalTimeline(restored)
	require.NoError(t, err)
	require.Equal(t, view, RenderTimelineFrozenOpen(copy))
	for _, id := range result.RetainedIDs {
		require.Contains(t, view.Frozen, originalText[id])
	}
	for _, id := range result.RetiredIDs {
		if text, ok := originalText[id]; ok {
			require.NotContains(t, view.Frozen, text)
		}
	}
}
