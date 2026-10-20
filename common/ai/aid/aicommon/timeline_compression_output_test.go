package aicommon

import (
	"context"
	"fmt"
	"math"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

func TestCompressionRetainedRanges(t *testing.T) {
	items := []timelineCompressionSnapshotItem{
		{ID: 3, PromptText: "reproduction start"}, {ID: 5, PromptText: "reproduction detail"}, {ID: 8, PromptText: "reproduction end"},
		{ID: 10, PromptText: "obsolete exploration"}, {ID: 12, PromptText: "constraint"},
		{ID: 20, PromptText: "verification start"}, {ID: 23, PromptText: "verification end"},
		{ID: 24}, // bookkeeping is not offered, even inside a range
		{ID: 26, PromptText: "verification result"}, {ID: 37, PromptText: "independent finding"},
		{ID: 41, PromptText: "regression start"}, {ID: 45, PromptText: "regression end"},
		{ID: math.MaxInt64, PromptText: "last possible ID"},
	}
	for _, tc := range []struct {
		name string
		text string
		want []int64
	}{
		{"none", "", nil},
		{"single", "12", []int64{12}},
		{"scattered IDs", "3,12,37", []int64{3, 12, 37}},
		{"single range", "20-26", []int64{20, 23, 26}},
		{"scattered ranges", "3-8,20-26,41-45", []int64{3, 5, 8, 20, 23, 26, 41, 45}},
		{"mixed IDs and ranges", "3-8,12,20-26,37,41-45", []int64{3, 5, 8, 12, 20, 23, 26, 37, 41, 45}},
		{"unordered overlapping duplicates", "41-45, 12,3-8,5-8,12", []int64{3, 5, 8, 12, 41, 45}},
		{"large sparse range", "45-9223372036854775807", []int64{45, math.MaxInt64}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseCompressionRetainedRange(tc.text, items)
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
	for _, text := range []string{"99", "0", "-3", "8-3", "3-99", "3-", "3--8", "3-5-8", "3,,8", "3,", "3.5", "+3", "3-+8", "24", "3-24", "3-9223372036854775808", "copied history text"} {
		t.Run("reject "+text, func(t *testing.T) {
			ids, err := parseCompressionRetainedRange(text, items)
			require.ErrorContains(t, err, "retain_timeline")
			require.Empty(t, ids, "an invalid segment must reject the entire selection")
		})
	}
}

func TestCompressionDecisionCommitsScatteredRanges(t *testing.T) {
	tl := NewTimeline(nil, nil)
	for _, id := range []int64{3, 5, 8, 10, 12, 20, 23, 26, 30, 37, 41, 45} {
		tl.PushText(id, "original-%d", id)
	}
	tl.PushUserInteraction(UserInteractionStage_FreeInput, 6, "", "PROTECTED_USER_INPUT")
	pushCompressionTool(t, tl, 21, "protected-tool", TimelinePromotedOperationUpsert, "PROTECTED_TOOL_SCHEMA")
	require.True(t, tl.PushPromotable(22, TimelinePromotedKindEvidence, TimelinePromotedTargetSemiDynamic1,
		"e1", TimelinePromotedOperationUpsert, `{"id":"e1","content":"PROTECTED_EVIDENCE"}`))
	decision, err := tl.BeginCompressionDecision(TimelineCompressionOptions{Context: context.Background(), PromptTokens: 1000, TargetTokens: 500})
	require.NoError(t, err)
	t.Cleanup(decision.Abort)
	// Appends after the snapshot are not affected by its selection.
	tl.PushText(50, "CONCURRENT_NEW_ENTRY")
	const selection = "3-8,12,20-26,37,41-45"
	require.NoError(t, decision.Validate(NewSimpleAction(TimelineCompressionAction, aitool.InvokeParams{
		"summary": "Exploration 10 and 30 was superseded; regression work remains.", "retain_timeline": selection,
		"retire_skill_names": []any{}, "retire_evidence_ids": []any{},
	})))
	result, err := decision.Commit()
	require.NoError(t, err)
	require.Equal(t, []int64{3, 5, 8, 12, 20, 23, 26, 37, 41, 45}, result.RetainedIDs)
	require.Equal(t, []int64{10, 30}, result.RetiredIDs)
	require.Equal(t, selection, result.RetainedRange)
	view := RenderTimelineFrozenOpen(tl)
	for _, text := range []string{"PROTECTED_USER_INPUT", "PROTECTED_TOOL_SCHEMA", "PROTECTED_EVIDENCE"} {
		require.Contains(t, view.PromptText(), text)
	}
	require.Contains(t, view.Open, "CONCURRENT_NEW_ENTRY")
	restored, err := MarshalTimeline(tl)
	require.NoError(t, err)
	copy, err := UnmarshalTimeline(restored)
	require.NoError(t, err)
	require.Equal(t, view, RenderTimelineFrozenOpen(copy))
	for _, id := range result.RetainedIDs {
		require.Contains(t, view.Frozen, fmt.Sprintf("original-%d", id))
	}
}

func TestCompressionDecisionRejectsOldRetentionProtocol(t *testing.T) {
	decision := &CompressionDecision{snapshot: &timelineCompressionSnapshot{Items: []timelineCompressionSnapshotItem{{ID: 12, PromptText: "history"}}}}
	decision.buildGuide()
	for _, raw := range []any{nil, 12, []any{"12"}, []any{"10:00:00 [text] history"}} {
		err := decision.Validate(NewSimpleAction(TimelineCompressionAction, aitool.InvokeParams{"summary": "summary", "retain_timeline": raw}))
		require.ErrorContains(t, err, "retain_timeline 必须是 ID/范围字符串")
	}
	err := decision.Validate(NewSimpleAction("timeline-summary", aitool.InvokeParams{"summary": "summary", "retain_timeline": "12"}))
	require.ErrorContains(t, err, "必须输出 compress_timeline")
}
