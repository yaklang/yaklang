package aicommon

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// New Skill bodies must first appear in the Open tail and must NOT change the
// cached prefix until an existing freeze/compression commit promotes them.
func TestSkillContentStaysInOpenUntilFrozen(t *testing.T) {
	cfg := NewConfig(context.Background(), WithDisableAutoSkills(true))
	tl := cfg.GetTimeline()
	tl.SetTimelineBucketByteSize(-1)

	tl.PushText(cfg.AcquireId(), "ordinary observation")
	before := RenderTimelineFrozenOpen(tl)

	require.True(t, tl.PushSkillContent(cfg.AcquireId(), "demo-skill", "", "SKILL_BODY_SECRET", SkillContentSourceAuto))
	after := RenderTimelineFrozenOpen(tl)

	require.Contains(t, after.Open, "SKILL_BODY_SECRET")
	require.Equal(t, before.Frozen, after.Frozen)
	require.True(t, tl.HasVisibleSkillContent("demo-skill"))
}

// A second read must not be blocked by a stale loader cache once the previous
// body was removed, and must not be deduped by skill name alone.
func TestSkillContentNotDedupedBySkillName(t *testing.T) {
	cfg := NewConfig(context.Background(), WithDisableAutoSkills(true))
	tl := cfg.GetTimeline()
	tl.SetTimelineBucketByteSize(-1)

	require.True(t, tl.PushSkillContent(cfg.AcquireId(), "demo-skill", "SKILL.md", "MAIN_BODY", SkillContentSourceAuto))
	require.True(t, tl.PushSkillContent(cfg.AcquireId(), "demo-skill", "references/api.md", "RESOURCE_BODY", SkillContentSourceAuto))

	open := RenderTimelineFrozenOpen(tl).Open
	require.Contains(t, open, "MAIN_BODY")
	require.Contains(t, open, "RESOURCE_BODY")
}

// Skill bodies remain exact promoted state for now: they are not offered as
// compression candidates. Compression must not summarize or retire them.
func TestSkillContentStaysExactDuringCompression(t *testing.T) {
	cfg := NewConfig(context.Background(), WithDisableAutoSkills(true))
	tl := cfg.GetTimeline()
	tl.SetTimelineBucketByteSize(-1)

	tl.PushText(cfg.AcquireId(), strings.Repeat("ordinary observation ", 40))
	require.True(t, tl.PushSkillContent(cfg.AcquireId(), "demo-skill", "", "CANDIDATE_BODY_SECRET", SkillContentSourceAuto))

	snapshot, err := tl.captureCompressionSnapshot()
	require.NoError(t, err)
	skillExact := false
	for _, id := range snapshot.ExactItemIDs {
		item, ok := tl.idToTimelineItem.Get(id)
		require.True(t, ok)
		op, ok := item.value.(*PromotableTimelineItem)
		if ok && op != nil && op.Kind == TimelinePromotedKindSkillContent {
			skillExact = true
		}
	}
	require.True(t, skillExact, "skill body must stay in the exact item set")
	found := false
	for _, item := range snapshot.Items {
		if strings.Contains(item.PromptText, "CANDIDATE_BODY_SECRET") {
			found = true
		}
	}
	require.False(t, found, "skill body must not be a compression candidate")
}

// Skill bodies are exact promoted state for now: compression summaries ordinary
// history but must not retire or replace the skill body.
func TestSkillContentSurvivesCompression(t *testing.T) {
	registerTimelineTestLiteForge(t)
	cfg := NewConfig(context.Background(), WithDisableAutoSkills(true),
		WithSpeedPriorityAICallback(func(_ AICallerConfigIf, req *AIRequest) (*AIResponse, error) {
			require.NotContains(t, req.GetPrompt(), "SKILL_SUMMARIZED_SECRET")
			rsp := NewUnboundAIResponse()
			rsp.EmitOutputStream(strings.NewReader(compressionMockSummary("Ordinary history summarized.")))
			rsp.Close()
			return rsp, nil
		}))
	tl := cfg.GetTimeline()
	tl.SetTimelineBucketByteSize(-1)
	tl.PushText(cfg.AcquireId(), strings.Repeat("old ordinary observation ", 60))
	require.True(t, tl.PushSkillContent(cfg.AcquireId(), "demo-skill", "", "SKILL_SUMMARIZED_SECRET", SkillContentSourceAuto))

	_, err := tl.CompressOnce(compressionTestOptions())
	require.NoError(t, err)

	materials := RenderTimelineFrozenOpen(tl)
	require.Contains(t, materials.SkillsContext, "SKILL_SUMMARIZED_SECRET")
	require.NotContains(t, materials.Open, "SKILL_SUMMARIZED_SECRET")
	require.True(t, tl.HasVisibleSkillContent("demo-skill"))
	require.Contains(t, tl.compressedHead.Text, "summarized")
}

// Exact skill state still carries load history so an old-session name list does
// not duplicate a body that is already tracked by this Timeline.
func TestExactSkillBodyKeepsTimelineHistory(t *testing.T) {
	registerTimelineTestLiteForge(t)
	cfg := NewConfig(context.Background(), WithDisableAutoSkills(true),
		WithSpeedPriorityAICallback(func(_ AICallerConfigIf, req *AIRequest) (*AIResponse, error) {
			rsp := NewUnboundAIResponse()
			rsp.EmitOutputStream(strings.NewReader(compressionMockSummary("Ordinary history summarized.")))
			rsp.Close()
			return rsp, nil
		}))
	tl := cfg.GetTimeline()
	tl.SetTimelineBucketByteSize(-1)
	tl.PushText(cfg.AcquireId(), strings.Repeat("ordinary observation ", 60))
	require.True(t, tl.PushSkillContent(cfg.AcquireId(), "demo-skill", skillMainBodyPathSentinel,
		"RETIRED_BODY_SECRET", SkillContentSourceManual))

	_, err := tl.CompressOnce(compressionTestOptions())
	require.NoError(t, err)
	require.True(t, tl.HasVisibleSkillContent("demo-skill"))
	require.True(t, tl.HasSkillContentHistory("demo-skill"))
}

// A Timeline with no skill-content record at all is a genuine old-session case.
func TestUnknownSkillHasNoContentHistory(t *testing.T) {
	cfg := NewConfig(context.Background(), WithDisableAutoSkills(true))
	tl := cfg.GetTimeline()
	tl.PushText(cfg.AcquireId(), "ordinary observation")
	require.False(t, tl.HasSkillContentHistory("never-tracked-skill"))
}

// Exact skill bodies move into the typed block after the freeze commit without
// requiring an AI retention selection.
func TestSkillContentPromotedAfterFreeze(t *testing.T) {
	registerTimelineTestLiteForge(t)
	cfg := NewConfig(context.Background(), WithDisableAutoSkills(true),
		WithSpeedPriorityAICallback(func(_ AICallerConfigIf, req *AIRequest) (*AIResponse, error) {
			rsp := NewUnboundAIResponse()
			rsp.EmitOutputStream(strings.NewReader(compressionMockSummary("ordinary history summarized")))
			rsp.Close()
			return rsp, nil
		}))
	tl := cfg.GetTimeline()
	tl.SetTimelineBucketByteSize(-1)
	tl.PushText(cfg.AcquireId(), strings.Repeat("old ordinary observation ", 60))
	require.True(t, tl.PushSkillContent(cfg.AcquireId(), "demo-skill", "", "SKILL_KEPT_SECRET", SkillContentSourceAuto))

	_, err := tl.CompressOnce(compressionTestOptions())
	require.NoError(t, err)

	require.Contains(t, tl.RenderSkillContentForPrompt(), "SKILL_KEPT_SECRET")
	require.True(t, tl.HasVisibleSkillContent("demo-skill"))
	require.NotContains(t, RenderTimelineFrozenOpen(tl).Open, "SKILL_KEPT_SECRET")
}

// Pending exact skill content participates in the freeze threshold like other
// promoted state, but the compression request itself must not receive it.
func TestSkillContentPendingDeltaTriggersFreezeOnly(t *testing.T) {
	registerTimelineTestLiteForge(t)
	requests := 0
	cfg := NewConfig(context.Background(), WithDisableAutoSkills(true),
		WithSpeedPriorityAICallback(func(_ AICallerConfigIf, req *AIRequest) (*AIResponse, error) {
			requests++
			rsp := NewUnboundAIResponse()
			rsp.EmitOutputStream(strings.NewReader(compressionMockSummary("unexpected")))
			rsp.Close()
			return rsp, nil
		}))
	tl := cfg.GetTimeline()
	tl.SetTimelineBucketByteSize(-1)
	skillID := cfg.AcquireId()
	require.True(t, tl.PushSkillContent(skillID, "demo-skill", "",
		"ONLY_SKILL_HISTORY_SECRET "+strings.Repeat("skill step detail ", 200), SkillContentSourceAuto))

	tl.SetTimelineContentLimit(1)
	_, err := tl.CompressBeforePrompt(compressionTestOptions())
	require.NoError(t, err)
	require.Zero(t, requests, "exact skill content must not require an AI summary")
	require.GreaterOrEqual(t, tl.frozenThroughLocked(), skillID)
	require.Contains(t, tl.RenderSkillContentForPrompt(), "ONLY_SKILL_HISTORY_SECRET")
}

// The rendered compression request must not expose exact skill bodies.
func TestSkillContentAbsentFromCompressionRequest(t *testing.T) {
	registerTimelineTestLiteForge(t)
	cfg := NewConfig(context.Background(), WithDisableAutoSkills(true))
	tl := cfg.GetTimeline()
	tl.SetTimelineBucketByteSize(-1)
	tl.PushText(cfg.AcquireId(), "ORDINARY_HISTORY_SECRET "+strings.Repeat("ordinary detail ", 40))
	require.True(t, tl.PushSkillContent(cfg.AcquireId(), "demo-skill", "references/api.md",
		"REQUEST_VISIBLE_SECRET", SkillContentSourceAuto))

	snapshot, err := tl.captureCompressionSnapshot()
	require.NoError(t, err)
	prompt, err := renderCompressionSummaryPrompt(snapshot)
	require.NoError(t, err)
	require.NotContains(t, prompt, "REQUEST_VISIBLE_SECRET")
	require.Contains(t, prompt, "ORDINARY_HISTORY_SECRET")
}

// 用户指定加载与普通加载一样属于精确提升态, 不因来源差异进入压缩候选。
func TestForcedSkillContentStaysExact(t *testing.T) {
	registerTimelineTestLiteForge(t)
	cfg := NewConfig(context.Background(), WithDisableAutoSkills(true),
		WithSpeedPriorityAICallback(func(_ AICallerConfigIf, req *AIRequest) (*AIResponse, error) {
			rsp := NewUnboundAIResponse()
			rsp.EmitOutputStream(strings.NewReader(compressionMockSummary("Forced skill summarized.")))
			rsp.Close()
			return rsp, nil
		}))
	tl := cfg.GetTimeline()
	tl.SetTimelineBucketByteSize(-1)
	tl.PushText(cfg.AcquireId(), strings.Repeat("old ordinary observation ", 60))
	require.True(t, tl.PushSkillContent(cfg.AcquireId(), "forced-skill", skillMainBodyPathSentinel,
		"FORCED_CANDIDATE_SECRET", SkillContentSourceForced))

	// The forced body must stay exact and absent from the AI request.
	snapshot, err := tl.captureCompressionSnapshot()
	require.NoError(t, err)
	found := false
	for _, item := range snapshot.Items {
		if strings.Contains(item.PromptText, "FORCED_CANDIDATE_SECRET") {
			found = true
		}
	}
	require.False(t, found, "forced body must not be a compression candidate")

	_, err = tl.CompressOnce(compressionTestOptions())
	require.NoError(t, err)
	require.True(t, tl.HasVisibleSkillContent("forced-skill"))
	require.Contains(t, tl.RenderSkillContentForPrompt(), "FORCED_CANDIDATE_SECRET")
}

// A body already promoted into the cached prefix stays exact across later
// compressions and is not offered to the summarizer.
func TestPromotedSkillContentStaysExactAcrossCompression(t *testing.T) {
	registerTimelineTestLiteForge(t)
	requests := 0
	cfg := NewConfig(context.Background(), WithDisableAutoSkills(true),
		WithSpeedPriorityAICallback(func(_ AICallerConfigIf, req *AIRequest) (*AIResponse, error) {
			requests++
			require.NotContains(t, req.GetPrompt(), "PROMOTED_SKILL_SECRET")
			rsp := NewUnboundAIResponse()
			rsp.EmitOutputStream(strings.NewReader(compressionMockSummary("ordinary history summarized")))
			rsp.Close()
			return rsp, nil
		}))
	tl := cfg.GetTimeline()
	tl.SetTimelineBucketByteSize(-1)
	tl.PushText(cfg.AcquireId(), strings.Repeat("ordinary history detail ", 400))
	require.True(t, tl.PushSkillContent(cfg.AcquireId(), "demo-skill", "",
		"PROMOTED_SKILL_SECRET "+strings.Repeat("promoted skill detail ", 400), SkillContentSourceAuto))

	// Freeze so the body is promoted into the cached Skill block (Semi/typed view).
	tl.FreezeAll()
	require.Contains(t, tl.RenderSkillContentForPrompt(), "PROMOTED_SKILL_SECRET")

	tl.SetTimelineContentLimit(1)
	_, err := tl.CompressBeforePrompt(compressionTestOptions())
	require.NoError(t, err)
	require.Equal(t, 1, requests, "ordinary history still triggers one compression")
	require.Contains(t, tl.RenderSkillContentForPrompt(), "PROMOTED_SKILL_SECRET")
}

// 显式移除技能正文只退休 item, 不回滚已提交的冻结边界: 上游可缓存前缀的
// 成员集合保持不变, 正文从渲染中消失, 之后可重新追加。
func TestDeleteSkillContentRetiresWithoutRewritingFreeze(t *testing.T) {
	cfg := NewConfig(context.Background(), WithDisableAutoSkills(true))
	tl := cfg.GetTimeline()
	tl.SetTimelineBucketByteSize(-1)
	tl.PushText(cfg.AcquireId(), "ordinary observation")
	require.True(t, tl.PushSkillContent(cfg.AcquireId(), "demo-skill", "", "DELETE_ME_SECRET", SkillContentSourceManual))

	tl.FreezeAll()
	require.Contains(t, tl.RenderSkillContentForPrompt(), "DELETE_ME_SECRET")
	frozenThroughBefore := tl.frozenThroughLocked()
	freezeVersionBefore := tl.freezeState.Version

	require.True(t, tl.DeleteSkillContent("demo-skill"))
	require.NotContains(t, tl.RenderSkillContentForPrompt(), "DELETE_ME_SECRET")
	require.False(t, tl.HasVisibleSkillContent("demo-skill"))
	require.Equal(t, frozenThroughBefore, tl.frozenThroughLocked(), "删除不得改写冻结水位")
	require.Equal(t, freezeVersionBefore, tl.freezeState.Version, "删除不得重排既有冻结批次")

	// 删除未知技能是 no-op。
	require.False(t, tl.DeleteSkillContent("never-loaded"))

	// 重新读取会追加一份新正文, 冻结提交后再次可见。
	require.True(t, tl.PushSkillContent(cfg.AcquireId(), "demo-skill", "", "RELOADED_SECRET", SkillContentSourceManual))
	tl.FreezeAll()
	require.Contains(t, tl.RenderSkillContentForPrompt(), "RELOADED_SECRET")
	require.NotContains(t, tl.RenderSkillContentForPrompt(), "DELETE_ME_SECRET")
}
