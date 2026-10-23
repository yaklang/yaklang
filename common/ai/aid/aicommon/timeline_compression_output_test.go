package aicommon

import (
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
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
	throughID := tl.GetMaxID()
	// A result arriving while the model reads the request must stay Open.
	concurrentEntry := uuid.NewString()
	tl.PushText(50, "%s", concurrentEntry)
	const selection = "3-8,12,20-26,37,41-45"
	result, err := tl.ApplyCompressionSelection(&TimelineCompressionSelection{
		ThroughID: &throughID,
		Summary:   "Exploration 10 and 30 was superseded; regression work remains.", RetainedRange: selection,
		RetainedIDs: []int64{3, 5, 8, 12, 20, 23, 26, 37, 41, 45},
	}, "")
	require.NoError(t, err)
	require.Equal(t, []int64{3, 5, 8, 12, 20, 23, 26, 37, 41, 45}, result.RetainedIDs)
	require.Equal(t, []int64{10, 30}, result.RetiredIDs)
	require.Equal(t, throughID, result.ThroughID)
	require.Equal(t, selection, result.RetainedRange)
	view := RenderTimelineFrozenOpen(tl)
	for _, text := range []string{protectedUserInput, protectedToolSchema, protectedEvidence} {
		require.Contains(t, view.PromptText(), text)
	}
	require.Contains(t, view.Open, concurrentEntry)
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

func TestCompressionApplyPreservesNewPromotedEntries(t *testing.T) {
	tl := NewTimeline(nil, nil)
	tl.PushText(1, "old history")
	require.True(t, tl.PushSkillFileContent(2, "demo-skill", "SKILL.md", "old-skill-body", SkillContentSourceAuto, 1, 1, 1))
	require.True(t, tl.PushPromotable(3, TimelinePromotedKindEvidence, TimelinePromotedTargetSemiDynamic1,
		"old-evidence", TimelinePromotedOperationUpsert, `{"id":"old-evidence","content":"old-evidence-body"}`))
	throughID := RenderTimelineFrozenOpen(tl).ThroughID
	require.True(t, tl.PushSkillFileContent(4, "demo-skill", "references/new.md", "new-skill-body", SkillContentSourceAuto, 1, 1, 1))
	require.True(t, tl.PushPromotable(5, TimelinePromotedKindEvidence, TimelinePromotedTargetSemiDynamic1,
		"new-evidence", TimelinePromotedOperationUpsert, `{"id":"new-evidence","content":"new-evidence-body"}`))
	tl.PushText(6, "new-async-result")
	candidates, err := tl.CompressionCandidates(throughID)
	require.NoError(t, err)
	for _, candidate := range candidates {
		require.LessOrEqual(t, candidate.ID, throughID)
	}
	result, err := tl.ApplyCompressionSelection(&TimelineCompressionSelection{
		ThroughID: &throughID, Summary: "old work completed; unused skill and evidence retired",
		RetiredPromotedEntries: []int64{2, 3},
	}, "request before the new results arrived")
	require.NoError(t, err)
	require.Equal(t, int64(3), result.ThroughID)
	require.Equal(t, []int64{1, 2, 3}, result.RetiredIDs)
	view := RenderTimelineFrozenOpen(tl)
	for _, text := range []string{"new-skill-body", "new-evidence-body", "new-async-result"} {
		require.Contains(t, view.PromptText(), text)
	}
	for _, text := range []string{"old-skill-body", "old-evidence-body"} {
		require.NotContains(t, view.PromptText(), text)
	}
	require.True(t, tl.NeedsMemoryFinalization(), "unseen results must not be marked as extracted memory")
	dump, err := MarshalTimeline(tl)
	require.NoError(t, err)
	copy, err := UnmarshalTimeline(dump)
	require.NoError(t, err)
	require.Equal(t, view, RenderTimelineFrozenOpen(copy))
}

func TestCompressionApplyPreservesReservedResultIDs(t *testing.T) {
	tl := NewTimeline(nil, nil)
	tl.PushText(10, "old-first-record")
	tl.PushText(30, "old-last-record")
	request := RenderTimelineFrozenOpen(tl)
	const lateResult = "late-result-with-reserved-ID"
	// Actual tool execution reserves its result ID before running. A later
	// callback can therefore insert below the already rendered watermark.
	tl.PushToolResult(&aitool.ToolResult{ID: 20, Name: "delayed-tool", Success: true, Data: lateResult})
	result, err := tl.ApplyCompressionSelection(&TimelineCompressionSelection{
		ThroughID: &request.ThroughID, ObservedIDs: request.ItemIDs, Summary: "old work summarized",
	}, "request without result 20")
	require.NoError(t, err)
	require.Equal(t, []int64{10, 30}, result.RetiredIDs)
	require.Equal(t, []int64{20}, result.RetainedIDs)
	view := RenderTimelineFrozenOpen(tl)
	require.Contains(t, view.PromptText(), lateResult)
	require.True(t, tl.NeedsMemoryFinalization(), "an unseen result below the watermark must remain uncovered")
	dump, err := MarshalTimeline(tl)
	require.NoError(t, err)
	copy, err := UnmarshalTimeline(dump)
	require.NoError(t, err)
	require.Equal(t, view, RenderTimelineFrozenOpen(copy))
}
