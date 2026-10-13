package aicommon

import (
	"context"
	"encoding/json"
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

	require.True(t, tl.PushSkillContent(cfg.AcquireId(), "demo-skill", "", "SKILL_BODY_SECRET", SkillContentSourceAuto))
	after := RenderTimelineFrozenOpen(tl)

	require.Contains(t, after.Open, "SKILL_BODY_SECRET")
	require.Equal(t, before.Frozen, after.Frozen)
	require.True(t, tl.HasVisibleSkillContent("demo-skill"))
}

// A second read must not be blocked by a stale loader cache once the previous
// body was summarized away, and must not be deduped by skill name alone.
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

// Skill bodies join the unified candidate set instead of being permanently exact.
func TestSkillContentIsCompressionCandidate(t *testing.T) {
	cfg := NewConfig(context.Background(), WithDisableAutoSkills(true))
	tl := cfg.GetTimeline()
	tl.SetTimelineBucketByteSize(-1)

	tl.PushText(cfg.AcquireId(), strings.Repeat("ordinary observation ", 40))
	require.True(t, tl.PushSkillContent(cfg.AcquireId(), "demo-skill", "", "CANDIDATE_BODY_SECRET", SkillContentSourceAuto))

	snapshot, err := tl.captureCompressionSnapshot()
	require.NoError(t, err)
	for _, id := range snapshot.ExactItemIDs {
		item, ok := tl.idToTimelineItem.Get(id)
		require.True(t, ok)
		if op, ok := item.value.(*PromotableTimelineItem); ok && op != nil {
			require.NotEqual(t, TimelinePromotedKindSkillContent, op.Kind)
		}
	}
	found := false
	for _, item := range snapshot.Items {
		if strings.Contains(item.PromptText, "CANDIDATE_BODY_SECRET") {
			found = true
		}
	}
	require.True(t, found, "skill body must be visible to the compression candidate set")
}

// Unselected skill bodies must not be resurrected by promoted-state rebuild.
func TestSkillContentNotResurrectedAfterCompression(t *testing.T) {
	registerTimelineTestLiteForge(t)
	cfg := NewConfig(context.Background(), WithDisableAutoSkills(true),
		WithSpeedPriorityAICallback(func(_ AICallerConfigIf, req *AIRequest) (*AIResponse, error) {
			require.Contains(t, req.GetPrompt(), "SKILL_SUMMARIZED_SECRET")
			rsp := NewUnboundAIResponse()
			rsp.EmitOutputStream(strings.NewReader(compressionMockSummary("Skill guidance summarized.")))
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
	require.NotContains(t, materials.Open, "SKILL_SUMMARIZED_SECRET")
	require.NotContains(t, materials.Frozen, "SKILL_SUMMARIZED_SECRET")
	require.NotContains(t, tl.RenderSkillContentForPrompt(), "SKILL_SUMMARIZED_SECRET")
	require.False(t, tl.HasVisibleSkillContent("demo-skill"))
	require.Contains(t, tl.compressedHead.Text, "summarized")
}

// Restoring an old session's loaded-skill name list must not re-expand a body
// that this (new-version) Timeline already summarized away. Only a Timeline with
// no skill-content history at all represents a genuine old-session migration.
func TestRetiredSkillBodyHasNoRestorableHistory(t *testing.T) {
	registerTimelineTestLiteForge(t)
	cfg := NewConfig(context.Background(), WithDisableAutoSkills(true),
		WithSpeedPriorityAICallback(func(_ AICallerConfigIf, req *AIRequest) (*AIResponse, error) {
			rsp := NewUnboundAIResponse()
			rsp.EmitOutputStream(strings.NewReader(compressionMockSummary("Skill summarized.")))
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
	require.False(t, tl.HasVisibleSkillContent("demo-skill"))
	// The journal still knows this skill was tracked: restore must not re-expand it.
	require.True(t, tl.HasSkillContentHistory("demo-skill"))
}

// A Timeline with no skill-content record at all is a genuine old-session case.
func TestUnknownSkillHasNoContentHistory(t *testing.T) {
	cfg := NewConfig(context.Background(), WithDisableAutoSkills(true))
	tl := cfg.GetTimeline()
	tl.PushText(cfg.AcquireId(), "ordinary observation")
	require.False(t, tl.HasSkillContentHistory("never-tracked-skill"))
}

// Retained skill bodies move into the typed block only after the commit.
func TestSkillContentPromotedOnlyWhenRetained(t *testing.T) {
	registerTimelineTestLiteForge(t)
	var skillID int64
	cfg := NewConfig(context.Background(), WithDisableAutoSkills(true),
		WithSpeedPriorityAICallback(func(_ AICallerConfigIf, req *AIRequest) (*AIResponse, error) {
			payload, _ := json.Marshal(map[string]any{
				"@action": "timeline-summary", "summary": "kept skill",
				"ratain_timeline_item_range": fmt.Sprintf("%d", skillID), "memory_entities": []any{},
			})
			rsp := NewUnboundAIResponse()
			rsp.EmitOutputStream(strings.NewReader(string(payload)))
			rsp.Close()
			return rsp, nil
		}))
	tl := cfg.GetTimeline()
	tl.SetTimelineBucketByteSize(-1)
	tl.PushText(cfg.AcquireId(), strings.Repeat("old ordinary observation ", 60))
	skillID = cfg.AcquireId()
	require.True(t, tl.PushSkillContent(skillID, "demo-skill", "", "SKILL_KEPT_SECRET", SkillContentSourceAuto))

	_, err := tl.CompressOnce(compressionTestOptions())
	require.NoError(t, err)

	require.Contains(t, tl.RenderSkillContentForPrompt(), "SKILL_KEPT_SECRET")
	require.True(t, tl.HasVisibleSkillContent("demo-skill"))
	require.NotContains(t, RenderTimelineFrozenOpen(tl).Open, "SKILL_KEPT_SECRET")
}

// Skill/Evidence bodies alone must be able to trigger one real compression:
// promoted bodies must not be excluded from the size accounting forever.
func TestSkillContentAloneTriggersCompression(t *testing.T) {
	registerTimelineTestLiteForge(t)
	requests := 0
	cfg := NewConfig(context.Background(), WithDisableAutoSkills(true),
		WithSpeedPriorityAICallback(func(_ AICallerConfigIf, req *AIRequest) (*AIResponse, error) {
			requests++
			require.Contains(t, req.GetPrompt(), "ONLY_SKILL_HISTORY_SECRET")
			rsp := NewUnboundAIResponse()
			rsp.EmitOutputStream(strings.NewReader(compressionMockSummary("Only skill history summarized.")))
			rsp.Close()
			return rsp, nil
		}))
	tl := cfg.GetTimeline()
	tl.SetTimelineBucketByteSize(-1)
	require.True(t, tl.PushSkillContent(cfg.AcquireId(), "demo-skill", "",
		"ONLY_SKILL_HISTORY_SECRET "+strings.Repeat("skill step detail ", 200), SkillContentSourceAuto))

	tl.SetTimelineContentLimit(1)
	_, err := tl.CompressBeforePrompt(compressionTestOptions())
	require.NoError(t, err)
	require.Equal(t, 1, requests)
	require.NotNil(t, tl.compressedHead)
}

// The rendered compression request must expose the skill body as a selectable
// item, not as opaque bookkeeping.
func TestSkillContentAppearsInCompressionRequest(t *testing.T) {
	registerTimelineTestLiteForge(t)
	cfg := NewConfig(context.Background(), WithDisableAutoSkills(true))
	tl := cfg.GetTimeline()
	tl.SetTimelineBucketByteSize(-1)
	require.True(t, tl.PushSkillContent(cfg.AcquireId(), "demo-skill", "references/api.md",
		"REQUEST_VISIBLE_SECRET", SkillContentSourceAuto))

	snapshot, err := tl.captureCompressionSnapshot()
	require.NoError(t, err)
	prompt, err := renderCompressionSummaryPrompt(snapshot)
	require.NoError(t, err)
	require.Contains(t, prompt, "REQUEST_VISIBLE_SECRET")
	require.Contains(t, prompt, "[skill_content]")
}

// 用户指定加载的正文不再是「不可总结」的特例: 它同样进入统一候选集, 只在
// 候选文本里带 [source: user-forced] 提示, 供压缩模型判断是否保留原文。
func TestForcedSkillContentIsSummarizableCandidate(t *testing.T) {
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

	// The forced body must be offered as a candidate, with its source marker visible.
	snapshot, err := tl.captureCompressionSnapshot()
	require.NoError(t, err)
	found := false
	for _, item := range snapshot.Items {
		if strings.Contains(item.PromptText, "FORCED_CANDIDATE_SECRET") {
			require.Contains(t, item.PromptText, SkillContentSourceForced)
			found = true
		}
	}
	require.True(t, found, "forced body must join the unified summarizable candidates")

	// Summarizing it away is allowed: 加载来源不再提供强制保护。
	_, err = tl.CompressOnce(compressionTestOptions())
	require.NoError(t, err)
	require.False(t, tl.HasVisibleSkillContent("forced-skill"))
	require.NotContains(t, tl.RenderSkillContentForPrompt(), "FORCED_CANDIDATE_SECRET")
}

// A body already promoted into the cached prefix must still count toward the
// compression trigger; "already promoted" must not mean "never reducible".
func TestPromotedSkillContentStillCountsTowardTrigger(t *testing.T) {
	registerTimelineTestLiteForge(t)
	requests := 0
	cfg := NewConfig(context.Background(), WithDisableAutoSkills(true),
		WithSpeedPriorityAICallback(func(_ AICallerConfigIf, req *AIRequest) (*AIResponse, error) {
			requests++
			rsp := NewUnboundAIResponse()
			rsp.EmitOutputStream(strings.NewReader(compressionMockSummary("Promoted skill summarized.")))
			rsp.Close()
			return rsp, nil
		}))
	tl := cfg.GetTimeline()
	tl.SetTimelineBucketByteSize(-1)
	require.True(t, tl.PushSkillContent(cfg.AcquireId(), "demo-skill", "",
		"PROMOTED_SKILL_SECRET "+strings.Repeat("promoted skill detail ", 400), SkillContentSourceAuto))

	// Freeze so the body is promoted into the cached Skill block (Semi/typed view).
	tl.FreezeAll()
	require.Contains(t, tl.RenderSkillContentForPrompt(), "PROMOTED_SKILL_SECRET")

	tl.SetTimelineContentLimit(1)
	_, err := tl.CompressBeforePrompt(compressionTestOptions())
	require.NoError(t, err)
	require.Equal(t, 1, requests, "promoted body must still be counted and reducible")
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
