package aicommon

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/aiskillloader"
	"github.com/yaklang/yaklang/common/utils/filesys"
)

func newSessionSkillQueryConfig(t *testing.T) *Config {
	t.Helper()
	vfs := filesys.NewVirtualFs()
	vfs.AddFile("loaded/SKILL.md", buildSkillMD("loaded", "Loaded guide", "LOADED_SKILL_BODY"))
	vfs.AddFile("loaded/references/guide.md", "# Reference\nREFERENCE_BODY")
	vfs.AddFile("available/SKILL.md", buildSkillMD("available", "Available guide", "AVAILABLE_SKILL_BODY"))
	return NewConfig(context.Background(),
		WithDisableAutoSkills(true),
		WithSkillsFS(vfs),
	)
}

func TestBuildSessionSkillInventoryMatchesCapabilityInventoryShape(t *testing.T) {
	cfg := newSessionSkillQueryConfig(t)
	mgr := cfg.GetSkillsContextManager()
	require.NoError(t, mgr.LoadSkill("loaded"))

	items := BuildSessionSkillInventory(mgr)
	require.Len(t, items, 2)
	byName := make(map[string]CapabilityInventoryItem, len(items))
	for _, item := range items {
		require.Equal(t, "skill", item.Type)
		require.Equal(t, CapabilityInventoryPositionSemiDynamic, item.Position)
		require.False(t, item.IsFixed)
		byName[item.Name] = item
	}
	require.Equal(t, CapabilityInventoryStageLoaded, byName["loaded"].Stage)
	require.Equal(t, CapabilityInventorySkillLoadLoaded, byName["loaded"].Data.(map[string]any)["skill_load_state"])
	require.Equal(t, CapabilityInventoryStageMetadata, byName["available"].Stage)
	require.Equal(t, CapabilityInventorySkillLoadMetadata, byName["available"].Data.(map[string]any)["skill_load_state"])
	require.NotContains(t, byName["available"].Data.(map[string]any), "token_count")
	require.True(t, mgr.UnloadSkill("loaded"))
	require.Equal(t, CapabilityInventoryStageMetadata, BuildSessionSkillInventory(mgr)[1].Stage)
}

func TestBuildSessionSkillDetailUsesSkillMDBeforeLoading(t *testing.T) {
	cfg := newSessionSkillQueryConfig(t)
	detail, err := BuildSessionSkillDetail(cfg.GetSkillsContextManager(), "available")
	require.NoError(t, err)
	require.False(t, detail.Loaded)
	require.Equal(t, "available", detail.Name)
	require.Equal(t, "Available guide", detail.Meta.Description)
	require.Contains(t, detail.SkillMDContent, "AVAILABLE_SKILL_BODY")
	require.True(t, detail.FileSystemAvailable)
	require.Contains(t, detail.FileTree, "SKILL.md")
	require.Empty(t, detail.PromptContent)
	require.Empty(t, detail.Parts)

	raw, err := json.Marshal(detail)
	require.NoError(t, err)
	require.Contains(t, string(raw), `"skill_md":`)
	require.NotContains(t, string(raw), `"file_system":`)
	require.False(t, skillVisible(cfg.GetTimeline(), "available"), "detail lookup must not load the skill into Timeline")
}

func TestBuildSessionSkillDetailUsesVisibleTimelineContentWhenLoaded(t *testing.T) {
	cfg := newSessionSkillQueryConfig(t)
	mgr := cfg.GetSkillsContextManager()
	require.NoError(t, mgr.LoadSkill("loaded"))
	_, err := mgr.LoadSkillResource("loaded", "references/guide.md")
	require.NoError(t, err)
	cfg.GetTimeline().FreezeAll()
	before, err := MarshalTimeline(cfg.GetTimeline())
	require.NoError(t, err)

	detail, err := BuildSessionSkillDetail(mgr, "loaded")
	require.NoError(t, err)
	require.True(t, detail.Loaded)
	require.Empty(t, detail.SkillMDContent)
	require.True(t, detail.FileSystemAvailable)
	require.Len(t, detail.Parts, 3)
	require.Contains(t, detail.PromptContent, "=== Skill: loaded ===")
	require.Contains(t, detail.PromptContent, "[metadata]")
	require.Contains(t, detail.PromptContent, "[SKILL.md:1-5/5 END]")
	require.Contains(t, detail.PromptContent, "LOADED_SKILL_BODY")
	require.Contains(t, detail.PromptContent, "[references/guide.md:")
	require.Contains(t, detail.PromptContent, "REFERENCE_BODY")
	require.Less(t, strings.Index(detail.PromptContent, "[metadata]"), strings.Index(detail.PromptContent, "[SKILL.md:"))

	after, err := MarshalTimeline(cfg.GetTimeline())
	require.NoError(t, err)
	require.Equal(t, before, after, "detail query must remain read-only")
}

func TestBuildSessionSkillDetailCanReadTimelineWithoutLoader(t *testing.T) {
	tl := NewTimeline(nil, nil)
	require.True(t, tl.PushSkillMetadata(1, "timeline-only", "TIMELINE_METADATA", SkillContentSourceManual))
	require.True(t, tl.PushSkillFileContent(2, "timeline-only", "SKILL.md", "TIMELINE_BODY", SkillContentSourceManual, 1, 2, 2))
	mgr := aiskillloader.NewSkillsContextManager(nil)
	mgr.BindTimelineSink(NewSkillContentTimelineAdapter(tl, func() int64 { return tl.GetMaxID() + 1 }))

	detail, err := BuildSessionSkillDetail(mgr, "timeline-only")
	require.NoError(t, err)
	require.True(t, detail.Loaded)
	require.False(t, detail.FileSystemAvailable)
	require.Contains(t, detail.PromptContent, "TIMELINE_METADATA")
	require.Contains(t, detail.PromptContent, "TIMELINE_BODY")
}

func TestBuildSessionSkillDetailUnknownAndEmptyName(t *testing.T) {
	mgr := newSessionSkillQueryConfig(t).GetSkillsContextManager()
	_, err := BuildSessionSkillDetail(mgr, "")
	require.ErrorContains(t, err, "name is empty")
	_, err = BuildSessionSkillDetail(mgr, "missing")
	require.ErrorContains(t, err, `skill "missing" not found`)
	items := BuildSessionSkillInventory(nil)
	require.Empty(t, items)
}
