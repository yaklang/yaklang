package aicommon

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

func TestCompressionDecisionUsesVisibleAnchors(t *testing.T) {
	decision := &CompressionDecision{snapshot: &timelineCompressionSnapshot{
		RetainedContext: map[string]string{"user_query": "CURRENT_QUERY"},
		PromptBudget:    timelineCompressionPromptBudget{Current: 1000, Target: 500},
		TargetTokens:    500,
		Items: []timelineCompressionSnapshotItem{
			{ID: 12, Timestamp: 1_000, PromptText: "ITEM_ID=12 timestamp_ms=1000 [text]\nverified edge case"},
		},
		PromotedItems: []timelineCompressionPromotedItem{
			{timelineCompressionSnapshotItem: timelineCompressionSnapshotItem{ID: 40}, Kind: TimelinePromotedKindRecentTool, Key: "read_file", Operation: TimelinePromotedOperationUpsert},
			{timelineCompressionSnapshotItem: timelineCompressionSnapshotItem{ID: 41}, Kind: TimelinePromotedKindSkillContent, Key: "legacy-scan\x00page", Operation: TimelinePromotedOperationUpsert},
			{timelineCompressionSnapshotItem: timelineCompressionSnapshotItem{ID: 42}, Kind: TimelinePromotedKindEvidence, Key: "auth-bypass", Operation: TimelinePromotedOperationUpsert},
		},
	}}
	view := decision.buildView()
	require.Contains(t, view.DynamicGuide, "CURRENT_QUERY")
	require.Contains(t, view.DynamicGuide, "verified edge case")
	require.Contains(t, view.DynamicGuide, "legacy-scan")
	require.Contains(t, view.DynamicGuide, "auth-bypass")
	require.NotContains(t, view.DynamicGuide, "read_file")
	require.NotContains(t, view.DynamicGuide, "ITEM_ID=")
	require.NotContains(t, view.DynamicGuide, "RETIRE_ID")
	action := NewSimpleAction(TimelineCompressionAction, aitool.InvokeParams{
		"summary":             "验证仍待复核。\n本次淘汰的 promoted\n- skill legacy-scan：后续不再使用。",
		"retain_timeline":     []any{view.Anchors[0].Token},
		"retire_skill_names":  []any{"legacy-scan"},
		"retire_evidence_ids": []any{},
		"memory_entities":     []any{},
	})
	require.NoError(t, decision.Validate(action))
	require.Equal(t, []int64{12}, decision.output.RetainedIDs)
	require.Equal(t, []int64{41}, decision.output.RetiredPromotedEntries)
	bad := NewSimpleAction(TimelineCompressionAction, aitool.InvokeParams{
		"summary": "x", "retain_timeline": []any{"12"}, "retire_skill_names": []any{}, "retire_evidence_ids": []any{}, "memory_entities": []any{},
	})
	require.Error(t, decision.Validate(bad))
}
