package aicommon

import (
	"context"
	"fmt"
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

	require.True(t, tl.PushSkillFileContent(cfg.AcquireId(), "demo-skill", "SKILL.md", "SKILL_BODY_SECRET", SkillContentSourceAuto, 1, 1, 1))
	after := RenderTimelineFrozenOpen(tl)

	require.Contains(t, after.Open, "SKILL_BODY_SECRET")
	require.Equal(t, before.Frozen, after.Frozen)
	require.True(t, skillVisible(tl, "demo-skill"))
}

// A second read must not be blocked by a stale loader cache once the previous
// body was removed, and must not be deduped by skill name alone.
func TestSkillContentNotDedupedBySkillName(t *testing.T) {
	cfg := NewConfig(context.Background(), WithDisableAutoSkills(true))
	tl := cfg.GetTimeline()
	tl.SetTimelineBucketByteSize(-1)

	require.True(t, tl.PushSkillFileContent(cfg.AcquireId(), "demo-skill", "SKILL.md", "MAIN_BODY", SkillContentSourceAuto, 1, 1, 1))
	require.True(t, tl.PushSkillFileContent(cfg.AcquireId(), "demo-skill", "references/api.md", "RESOURCE_BODY", SkillContentSourceAuto, 1, 1, 1))

	open := RenderTimelineFrozenOpen(tl).Open
	require.Contains(t, open, "MAIN_BODY")
	require.Contains(t, open, "RESOURCE_BODY")
}

// Skill bodies are separate original items, kept unless explicitly blacklisted.
func TestSkillContentStaysExactDuringCompression(t *testing.T) {
	cfg := NewConfig(context.Background(), WithDisableAutoSkills(true))
	tl := cfg.GetTimeline()
	tl.SetTimelineBucketByteSize(-1)

	tl.PushText(cfg.AcquireId(), strings.Repeat("ordinary observation ", 40))
	require.True(t, tl.PushSkillFileContent(cfg.AcquireId(), "demo-skill", "SKILL.md", "CANDIDATE_BODY_SECRET", SkillContentSourceAuto, 1, 1, 1))

	snapshot, err := tl.captureCompressionSnapshot()
	require.NoError(t, err)
	require.Len(t, snapshot.PromotedItems, 1)
	require.Equal(t, TimelinePromotedKindSkillContent, snapshot.PromotedItems[0].Kind)
	require.Contains(t, snapshot.PromotedItems[0].PromptText, "CANDIDATE_BODY_SECRET")
	require.NotContains(t, snapshot.ExactItemIDs, snapshot.PromotedItems[0].ID)
	found := false
	for _, item := range snapshot.Items {
		if strings.Contains(item.PromptText, "CANDIDATE_BODY_SECRET") {
			found = true
		}
	}
	require.False(t, found, "skill body must not be a compression candidate")
}

// Skill bodies are exact promoted state: compression keeps them unless the
// model blacklists the entry, so an untouched body survives the summary.
func TestSkillContentSurvivesCompression(t *testing.T) {
	registerTimelineTestLiteForge(t)
	cfg := NewConfig(context.Background(), WithDisableAutoSkills(true),
		WithSpeedPriorityAICallback(func(_ AICallerConfigIf, req *AIRequest) (*AIResponse, error) {
			// The body is listed exactly once as a complete item; it is not
			// duplicated into the ordinary history sections.
			require.Equal(t, 1, strings.Count(req.GetPrompt(), "SKILL_SUMMARIZED_SECRET"))
			rsp := NewUnboundAIResponse()
			rsp.EmitOutputStream(strings.NewReader(compressionMockSummary("Ordinary history summarized.")))
			rsp.Close()
			return rsp, nil
		}))
	tl := cfg.GetTimeline()
	tl.SetTimelineBucketByteSize(-1)
	tl.PushText(cfg.AcquireId(), strings.Repeat("old ordinary observation ", 60))
	require.True(t, tl.PushSkillFileContent(cfg.AcquireId(), "demo-skill", "SKILL.md", "SKILL_SUMMARIZED_SECRET", SkillContentSourceAuto, 1, 1, 1))

	_, err := tl.CompressOnce(compressionTestOptions())
	require.NoError(t, err)

	materials := RenderTimelineFrozenOpen(tl)
	require.Contains(t, materials.SkillsContext, "SKILL_SUMMARIZED_SECRET")
	require.NotContains(t, materials.Open, "SKILL_SUMMARIZED_SECRET")
	require.True(t, skillVisible(tl, "demo-skill"))
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
	require.True(t, tl.PushSkillMetadata(cfg.AcquireId(), "demo-skill", "RETIRED_BODY_SECRET", SkillContentSourceManual))

	_, err := tl.CompressOnce(compressionTestOptions())
	require.NoError(t, err)
	require.True(t, skillVisible(tl, "demo-skill"))
	require.True(t, tl.HasSkillContentHistory("demo-skill"))
}

// A Timeline with no skill record at all is a genuine old-session case.
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
	require.True(t, tl.PushSkillFileContent(cfg.AcquireId(), "demo-skill", "SKILL.md", "SKILL_KEPT_SECRET", SkillContentSourceAuto, 1, 1, 1))

	_, err := tl.CompressOnce(compressionTestOptions())
	require.NoError(t, err)

	require.Contains(t, tl.RenderSkillContentForPrompt(), "SKILL_KEPT_SECRET")
	require.True(t, skillVisible(tl, "demo-skill"))
	require.NotContains(t, RenderTimelineFrozenOpen(tl).Open, "SKILL_KEPT_SECRET")
}

// Pending skill content is visible to the trigger and can be explicitly
// retired when lossless freezing alone cannot meet the lower target.
func TestSkillContentPendingDeltaTriggersCompression(t *testing.T) {
	registerTimelineTestLiteForge(t)
	requests := 0
	var skillID int64
	cfg := NewConfig(context.Background(), WithDisableAutoSkills(true),
		WithSpeedPriorityAICallback(func(_ AICallerConfigIf, req *AIRequest) (*AIResponse, error) {
			requests++
			rsp := NewUnboundAIResponse()
			rsp.EmitOutputStream(strings.NewReader(compressionMockWithRetire("", fmt.Sprint(skillID))))
			rsp.Close()
			return rsp, nil
		}))
	tl := cfg.GetTimeline()
	tl.SetTimelineBucketByteSize(-1)
	skillID = cfg.AcquireId()
	require.True(t, tl.PushSkillFileContent(skillID, "demo-skill", "SKILL.md", "ONLY_SKILL_HISTORY_SECRET "+strings.Repeat("skill step detail ", 200), SkillContentSourceAuto, 1, 1, 1))

	tl.SetTimelineContentLimit(1)
	_, err := tl.CompressBeforePrompt(compressionTestOptions())
	require.NoError(t, err)
	require.Equal(t, 1, requests, "decidable promoted content asks once")
	require.GreaterOrEqual(t, tl.frozenThroughLocked(), skillID)
	require.NotContains(t, tl.RenderSkillContentForPrompt(), "ONLY_SKILL_HISTORY_SECRET")
}

// The rendered compression request lists the skill entry with its full
// effective body, exactly once and never duplicated into ordinary sections.
func TestSkillContentListedWithFullBodyInCompressionRequest(t *testing.T) {
	registerTimelineTestLiteForge(t)
	cfg := NewConfig(context.Background(), WithDisableAutoSkills(true))
	tl := cfg.GetTimeline()
	tl.SetTimelineBucketByteSize(-1)
	tl.PushText(cfg.AcquireId(), "ORDINARY_HISTORY_SECRET "+strings.Repeat("ordinary detail ", 40))
	body := "SKILL_PREVIEW_HEAD " + strings.Repeat("skill body detail ", 80) + "SKILL_DEEP_SECRET"
	require.True(t, tl.PushSkillFileContent(cfg.AcquireId(), "demo-skill", "references/api.md", body, SkillContentSourceAuto, 1, 1, 1))

	snapshot, err := tl.captureCompressionSnapshot()
	require.NoError(t, err)
	prompt, err := renderCompressionSummaryPrompt(snapshot)
	require.NoError(t, err)
	require.Len(t, snapshot.PromotedItems, 1)
	skillID := snapshot.PromotedItems[0].ID
	requireCompressionPromotedEntry(t, prompt, skillID, "kind=skill name=demo-skill:references/api.md:1-1 operation=upsert", "[skill demo-skill | references/api.md:1-1/1 END]\n"+body)
	require.Equal(t, 1, strings.Count(prompt, "SKILL_DEEP_SECRET"))
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
	require.True(t, tl.PushSkillMetadata(cfg.AcquireId(), "forced-skill", "FORCED_CANDIDATE_SECRET", SkillContentSourceForced))

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
	require.True(t, skillVisible(tl, "forced-skill"))
	require.Contains(t, tl.RenderSkillContentForPrompt(), "FORCED_CANDIDATE_SECRET")
}

// A body already promoted into the cached prefix is listed as a deletable
// entry and is kept by default across later compressions.
func TestPromotedSkillContentKeptByDefaultAcrossCompression(t *testing.T) {
	registerTimelineTestLiteForge(t)
	requests := 0
	cfg := NewConfig(context.Background(), WithDisableAutoSkills(true),
		WithSpeedPriorityAICallback(func(_ AICallerConfigIf, req *AIRequest) (*AIResponse, error) {
			requests++
			require.Equal(t, 1, strings.Count(req.GetPrompt(), "PROMOTED_SKILL_SECRET"))
			rsp := NewUnboundAIResponse()
			rsp.EmitOutputStream(strings.NewReader(compressionMockSummary("ordinary history summarized")))
			rsp.Close()
			return rsp, nil
		}))
	tl := cfg.GetTimeline()
	tl.SetTimelineBucketByteSize(-1)
	tl.PushText(cfg.AcquireId(), strings.Repeat("ordinary history detail ", 400))
	require.True(t, tl.PushSkillFileContent(cfg.AcquireId(), "demo-skill", "SKILL.md", "PROMOTED_SKILL_SECRET "+strings.Repeat("promoted skill detail ", 400), SkillContentSourceAuto, 1, 1, 1))

	// Freeze so the body is promoted into the cached Skill block (Semi/typed view).
	tl.FreezeAll()
	require.Contains(t, tl.RenderSkillContentForPrompt(), "PROMOTED_SKILL_SECRET")

	_, err := tl.CompressOnce(compressionTestOptions())
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
	require.True(t, tl.PushSkillFileContent(cfg.AcquireId(), "demo-skill", "SKILL.md", "DELETE_ME_SECRET", SkillContentSourceManual, 1, 1, 1))

	tl.FreezeAll()
	require.Contains(t, tl.RenderSkillContentForPrompt(), "DELETE_ME_SECRET")
	frozenThroughBefore := tl.frozenThroughLocked()
	freezeVersionBefore := tl.freezeState.Version

	require.True(t, tl.DeleteSkillContent("demo-skill"))
	require.NotContains(t, tl.RenderSkillContentForPrompt(), "DELETE_ME_SECRET")
	require.False(t, skillVisible(tl, "demo-skill"))
	require.Equal(t, frozenThroughBefore, tl.frozenThroughLocked(), "删除不得改写冻结水位")
	require.Equal(t, freezeVersionBefore, tl.freezeState.Version, "删除不得重排既有冻结批次")

	// 删除未知技能是 no-op。
	require.False(t, tl.DeleteSkillContent("never-loaded"))

	// 重新读取会追加一份新正文, 冻结提交后再次可见。
	require.True(t, tl.PushSkillFileContent(cfg.AcquireId(), "demo-skill", "SKILL.md", "RELOADED_SECRET", SkillContentSourceManual, 1, 1, 1))
	tl.FreezeAll()
	require.Contains(t, tl.RenderSkillContentForPrompt(), "RELOADED_SECRET")
	require.NotContains(t, tl.RenderSkillContentForPrompt(), "DELETE_ME_SECRET")
}
