package aicommon

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

func TestCompressionDecisionUsesTimelineIDs(t *testing.T) {
	decision := &CompressionDecision{options: TimelineCompressionOptions{PromptTokens: 1000, TargetTokens: 500}, snapshot: &timelineCompressionSnapshot{
		Items: []timelineCompressionSnapshotItem{
			{ID: 12, Timestamp: 1_000, PromptText: "ITEM_ID=12 timestamp_ms=1000 [text]\nverified edge case"},
		},
		PromotedItems: []timelineCompressionPromotedItem{
			{timelineCompressionSnapshotItem: timelineCompressionSnapshotItem{ID: 40}, Kind: TimelinePromotedKindRecentTool, Key: "read_file", Operation: TimelinePromotedOperationUpsert},
			{timelineCompressionSnapshotItem: timelineCompressionSnapshotItem{ID: 41}, Kind: TimelinePromotedKindSkillContent, Key: "legacy-scan\x00page", Operation: TimelinePromotedOperationUpsert},
			{timelineCompressionSnapshotItem: timelineCompressionSnapshotItem{ID: 42}, Kind: TimelinePromotedKindEvidence, Key: "auth-bypass", Operation: TimelinePromotedOperationUpsert},
		},
	}}
	decision.buildGuide()
	guide := decision.DynamicGuide()
	require.Contains(t, guide, "当前整份主循环 prompt：1000 tokens")
	require.Contains(t, guide, "接近 500 tokens")
	require.Contains(t, guide, "ID=12 verified edge case")
	require.Contains(t, guide, "legacy-scan")
	require.Contains(t, guide, "auth-bypass")
	require.NotContains(t, guide, "read_file")
	require.NotContains(t, guide, "ITEM_ID=")
	require.NotContains(t, guide, "RETIRE_ID")
	require.NotContains(t, guide, "PERSISTENT")
	action := NewSimpleAction(TimelineCompressionAction, aitool.InvokeParams{
		"summary":             "验证仍待复核。\n本次淘汰的 promoted\n- skill legacy-scan：后续不再使用。",
		"retain_timeline":     "12",
		"retire_skill_names":  []any{"legacy-scan"},
		"retire_evidence_ids": []any{},
		"memory_entities":     []any{},
	})
	require.NoError(t, decision.Validate(action))
	require.Equal(t, []int64{12}, decision.output.RetainedIDs)
	require.Equal(t, "12", decision.output.RetainedRange)
	require.Equal(t, []int64{41}, decision.output.RetiredPromotedEntries)
	bad := NewSimpleAction(TimelineCompressionAction, aitool.InvokeParams{
		"summary": "x", "retain_timeline": "99", "retire_skill_names": []any{}, "retire_evidence_ids": []any{}, "memory_entities": []any{},
	})
	require.Error(t, decision.Validate(bad))
	require.Nil(t, decision.output, "a rejected correction must not leave a stale validated selection")
}
