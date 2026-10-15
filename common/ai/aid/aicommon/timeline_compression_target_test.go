package aicommon

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func mainTimelineTokens(tl *Timeline) int {
	return MeasureTokens(RenderTimelineFrozenOpenWithOptions(tl, TimelinePromptOptions{PromoteUserInput: true}).PromptText())
}

func TestTimelineCompressionMeasuresMainPromptNotJournal(t *testing.T) {
	tl := NewTimeline(nil, nil)
	for id := int64(1); id <= 10; id++ {
		pushCompressionTool(t, tl, id, "alpha", "upsert", fmt.Sprintf("OLD_%d ", id)+strings.Repeat("old schema ", 600))
	}
	pushCompressionTool(t, tl, 11, "alpha", "upsert", "CURRENT_SMALL_SCHEMA")
	tl.FreezeAll()
	snapshot, err := tl.buildCompressionSnapshot()
	require.NoError(t, err)
	rendered := mainTimelineTokens(tl)
	require.Greater(t, snapshot.InputTokens, rendered*10, "retained operation history is not model occupancy")
	tl.SetTimelineContentLimit(int64(rendered + 1))
	before, err := MarshalTimeline(tl)
	require.NoError(t, err)
	result, err := tl.CompressBeforePrompt(compressionTestOptions())
	require.NoError(t, err)
	require.Nil(t, result, "below limit must not build/clean compression candidates or invoke AI")
	after, err := MarshalTimeline(tl)
	require.NoError(t, err)
	require.Equal(t, before, after)
}

func TestTimelineCompressionCompactionCanMeetLowerTargetWithoutAI(t *testing.T) {
	tl := NewTimeline(nil, nil)
	calls := 0
	bindCompressionMock(t, tl, func(*AIRequest) (string, error) {
		calls++
		return compressionMockSummary("unexpected"), nil
	})
	for id := int64(1); id <= 20; id++ {
		pushCompressionTool(t, tl, id, "alpha", "upsert", strings.Repeat("same schema ", 300))
	}
	limit := int64(mainTimelineTokens(tl) / 2)
	tl.SetTimelineContentLimit(limit)
	result, err := tl.CompressBeforePrompt(compressionTestOptions())
	require.NoError(t, err)
	require.Zero(t, calls)
	require.Len(t, result.RetiredPromotedEntryIDs, 19)
	require.LessOrEqual(t, int64(mainTimelineTokens(tl)), limit/timelineCompressionTargetDivisor)
	require.Contains(t, RenderTimelineFrozenOpen(tl).PromotedRecentTools, "same schema")
}

func TestTimelineCompressionBelowLimitAfterCompactionStillNeedsAI(t *testing.T) {
	tl := NewTimeline(nil, nil)
	for id := int64(1); id <= 3; id++ {
		pushCompressionTool(t, tl, id, "alpha", "upsert",
			fmt.Sprintf("SCHEMA_VERSION_%d ", id)+strings.Repeat("schema field ", 500))
	}
	limit := int64(mainTimelineTokens(tl) / 2)
	tl.SetTimelineContentLimit(limit)
	calls := 0
	bindCompressionMock(t, tl, func(req *AIRequest) (string, error) {
		calls++
		require.Less(t, int64(mainTimelineTokens(tl)), limit, "cleanup already got below the trigger")
		require.Greater(t, int64(mainTimelineTokens(tl)), limit/timelineCompressionTargetDivisor)
		require.NotContains(t, req.GetPrompt(), "SCHEMA_VERSION_1")
		require.NotContains(t, req.GetPrompt(), "SCHEMA_VERSION_2")
		require.Contains(t, req.GetPrompt(), "SCHEMA_VERSION_3")
		current := int64(mainTimelineTokens(tl))
		target := max(int64(1), limit/timelineCompressionTargetDivisor)
		require.Contains(t, req.GetPrompt(), fmt.Sprintf("当前主模型 Timeline：%d tokens。", current))
		require.Contains(t, req.GetPrompt(), fmt.Sprintf("目标：不超过 %d tokens。", target))
		require.Contains(t, req.GetPrompt(), fmt.Sprintf("还需要减少：%d tokens。", current-target))
		return compressionMockWithRetire("", "3"), nil
	})
	tl.SetTimelineContentLimit(limit)
	result, err := tl.CompressBeforePrompt(compressionTestOptions())
	require.NoError(t, err)
	require.Equal(t, 1, calls)
	require.Equal(t, []int64{1, 2, 3}, result.RetiredIDs)
	require.LessOrEqual(t, int64(mainTimelineTokens(tl)), limit/timelineCompressionTargetDivisor)
}

func TestTimelineCompressionOneAIRoundDoesNotContinueBelowTarget(t *testing.T) {
	tl := NewTimeline(nil, nil)
	tl.PushText(1, "ordinary history that still needs one summary")
	calls := 0
	bindCompressionMock(t, tl, func(*AIRequest) (string, error) {
		calls++
		return compressionMockSummary("finding"), nil
	})
	tl.SetTimelineContentLimit(1)
	result, err := tl.CompressBeforePrompt(compressionTestOptions())
	require.NoError(t, err)
	require.Equal(t, 1, calls)
	require.Equal(t, "finding", result.Summary)
	// One check stops after its AI round. The rendered result is still over this
	// tiny limit, so the next check performs its own round.
	_, err = tl.CompressBeforePrompt(compressionTestOptions())
	require.NoError(t, err)
	require.Equal(t, 2, calls)
}

func TestTimelineCompressionFoldsWithoutAIOnceBeforeOneAIRound(t *testing.T) {
	tl := NewTimeline(nil, nil)
	tl.SetTimelineBucketByteSize(-1)
	tl.PushText(1, "ordinary history that still needs one summary")
	pushCompressionTool(t, tl, 2, "alpha", "upsert", "REDUNDANT_SCHEMA")
	pushCompressionTool(t, tl, 3, "alpha", "upsert", "INITIAL_SCHEMA")
	calls, foldCommits := 0, 0
	tl.RegisterCompressFreezeCallback("count-folds", func(event TimelineCompressFreezeEvent) {
		if event.Compression.InputTokens == 0 {
			foldCommits++
		}
	})
	bindCompressionMock(t, tl, func(req *AIRequest) (string, error) {
		calls++
		require.NotContains(t, req.GetPrompt(), "REDUNDANT_SCHEMA")
		require.Contains(t, req.GetPrompt(), "INITIAL_SCHEMA")
		// Operations appended while the model runs belong to the next check.
		pushCompressionTool(t, tl, 4, "alpha", "upsert", "APPENDED_SCHEMA_1")
		pushCompressionTool(t, tl, 5, "alpha", "upsert", "APPENDED_SCHEMA_2")
		return compressionMockSummary("finding"), nil
	})
	tl.SetTimelineContentLimit(1)

	result, err := tl.CompressBeforePrompt(compressionTestOptions())
	require.NoError(t, err)
	require.Equal(t, 1, calls)
	require.Equal(t, 1, foldCommits)
	require.Equal(t, []int64{2}, result.RetiredPromotedEntryIDs)
	require.Contains(t, RenderTimelineFrozenOpen(tl).Open, "APPENDED_SCHEMA_1")
	require.Contains(t, RenderTimelineFrozenOpen(tl).Open, "APPENDED_SCHEMA_2")
}

func TestTimelineCompressionOneRoundAboveTargetStillSucceeds(t *testing.T) {
	tl := NewTimeline(nil, nil)
	tl.PushText(1, "original history that still needs one summary")
	calls := 0
	bindCompressionMock(t, tl, func(*AIRequest) (string, error) {
		calls++
		return compressionMockSummary("finding remains"), nil
	})
	tl.SetTimelineContentLimit(1)
	result, err := tl.CompressBeforePrompt(compressionTestOptions())
	require.NoError(t, err)
	require.Equal(t, 1, calls)
	require.NotNil(t, result)
	require.GreaterOrEqual(t, int64(mainTimelineTokens(tl)), int64(1))
}

func TestTimelineCompressionOneRoundPreservesProtectedInput(t *testing.T) {
	tl := NewTimeline(nil, nil)
	tl.EnsureTaskUserInput("task", "protected user request", func() int64 { return 1 })
	tl.PushText(2, "ordinary history that still needs one summary")
	calls := 0
	bindCompressionMock(t, tl, func(*AIRequest) (string, error) {
		calls++
		return compressionMockWithRetire("done", "1"), nil
	})
	tl.SetTimelineContentLimit(1)
	result, err := tl.CompressBeforePrompt(compressionTestOptions())
	require.NoError(t, err)
	require.Equal(t, 1, calls)
	require.Equal(t, "done", result.Summary)
	view := RenderTimelineFrozenOpenWithOptions(tl, TimelinePromptOptions{PromoteUserInput: true})
	require.Contains(t, view.PromotedUserInputHistory, "protected user request")
}

func TestTimelinePromptMaterialsSharedByTemplateAndAccounting(t *testing.T) {
	tl := NewTimeline(nil, nil)
	tl.PushText(1, "FROZEN_HISTORY")
	pushCompressionTool(t, tl, 2, "alpha", "upsert", "TOOL_SCHEMA")
	require.True(t, tl.PushSkillFileContent(3, "demo", "ref.md", "SKILL_BODY", SkillContentSourceManual, 1, 1, 1))
	require.True(t, tl.PushPromotable(4, TimelinePromotedKindEvidence, TimelinePromotedTargetSemiDynamic1,
		"e1", "upsert", `{"id":"e1","content":"EVIDENCE_BODY"}`))
	tl.EnsureTaskUserInput("task", "USER_INPUT", func() int64 { return 5 })
	tl.FreezeAll()
	tl.PushText(6, "OPEN_HISTORY")
	view := RenderTimelineFrozenOpenWithOptions(tl, TimelinePromptOptions{PromoteUserInput: true})
	materials := &PromptMaterials{TimelineFrozen: view.Frozen, TimelineOpen: view.Open, SkillsContext: view.SkillsContext,
		PromotedRecentTools: view.PromotedRecentTools, PromotedUserInputHistory: view.PromotedUserInputHistory,
		SessionEvidenceSemiDynamic: view.EvidenceSemiDynamic}
	prefix, err := NewDefaultPromptPrefixBuilder().AssemblePromptPrefix(materials)
	require.NoError(t, err)
	templateText := prefix.FrozenBlock + prefix.SemiDynamic + prefix.TimelineOpen
	for _, marker := range []string{"FROZEN_HISTORY", "OPEN_HISTORY", "SKILL_BODY", "TOOL_SCHEMA", "EVIDENCE_BODY", "USER_INPUT"} {
		require.Equal(t, 1, strings.Count(templateText, marker))
		require.Equal(t, 1, strings.Count(view.PromptText(), marker))
	}
}
