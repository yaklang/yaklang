package aicommon

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCompressionUsesTimelineIDs(t *testing.T) {
	snapshot := &timelineCompressionSnapshot{
		Items: []timelineCompressionSnapshotItem{
			{ID: 12, Timestamp: 1_000, PromptText: "ITEM_ID=12 timestamp_ms=1000 [text]\nverified edge case"},
		},
		PromotedItems: []timelineCompressionPromotedItem{
			{timelineCompressionSnapshotItem: timelineCompressionSnapshotItem{ID: 40}, Kind: TimelinePromotedKindRecentTool, Key: "read_file", Operation: TimelinePromotedOperationUpsert},
			{timelineCompressionSnapshotItem: timelineCompressionSnapshotItem{ID: 41}, Kind: TimelinePromotedKindSkillContent, Key: "legacy-scan\x00page", Operation: TimelinePromotedOperationUpsert},
			{timelineCompressionSnapshotItem: timelineCompressionSnapshotItem{ID: 42}, Kind: TimelinePromotedKindEvidence, Key: "auth-bypass", Operation: TimelinePromotedOperationUpsert},
		},
	}
	options := TimelineCompressionOptions{PromptTokens: 1000, TargetTokens: 500, MaxSummaryTokens: TimelineCompressionMaxSummaryTokens}
	guide := renderCompressionGuide(snapshot, options)
	require.Contains(t, guide, "当前整份主循环 prompt：1000 tokens")
	require.Contains(t, guide, "接近 500 tokens")
	require.Contains(t, guide, "普通历史 ID 从哪里看")
	require.Contains(t, guide, "HH:MM:SS [类型]")
	require.NotContains(t, guide, "verified edge case")
	require.NotContains(t, guide, "ID=12")
	require.Contains(t, guide, "legacy-scan")
	require.Contains(t, guide, "[id: ...]")
	require.NotContains(t, guide, "auth-bypass")
	require.NotContains(t, guide, "read_file")
	require.NotContains(t, guide, "ITEM_ID=")
	require.NotContains(t, guide, "RETIRE_ID")
	require.NotContains(t, guide, "PERSISTENT")
}
