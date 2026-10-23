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
func TestUnknownSkillHasNoContentHistory(t *testing.T) {
	cfg := NewConfig(context.Background(), WithDisableAutoSkills(true))
	tl := cfg.GetTimeline()
	tl.PushText(cfg.AcquireId(), "ordinary observation")
	require.False(t, tl.HasSkillContentHistory("never-tracked-skill"))
}

// Exact skill bodies move into the typed block after the freeze commit without
// requiring an AI retention selection.
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
