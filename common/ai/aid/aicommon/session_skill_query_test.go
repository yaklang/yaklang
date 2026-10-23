package aicommon

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/aiskillloader"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
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
		WithID("runtime-session-skill"),
		WithPersistentSessionId("persistent-session-skill"),
	)
}

func TestBuildSessionSkillInventoryMatchesCapabilityInventoryShape(t *testing.T) {
	cfg := newSessionSkillQueryConfig(t)
	mgr := cfg.GetSkillsContextManager()
	require.NoError(t, mgr.LoadSkill("loaded"))

	inventory, err := BuildSessionSkillInventory(mgr, 1, 100)
	require.NoError(t, err)
	require.Len(t, inventory.Skills, 2)
	require.Equal(t, 2, inventory.Total)
	require.False(t, inventory.HasMore)
	byName := make(map[string]CapabilityInventoryItem, len(inventory.Skills))
	for _, item := range inventory.Skills {
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
	inventory, err = BuildSessionSkillInventory(mgr, 1, 100)
	require.NoError(t, err)
	require.Equal(t, CapabilityInventoryStageMetadata, inventory.Skills[1].Stage)
}

func TestBuildSessionSkillInventoryPaginatesWithoutPromptBudget(t *testing.T) {
	vfs := filesys.NewVirtualFs()
	vfs.AddFile("loaded/SKILL.md", buildSkillMD("loaded", "Loaded guide", "LOADED"))
	for i := 0; i < 100; i++ {
		name := fmt.Sprintf("available-%03d", i)
		vfs.AddFile(name+"/SKILL.md", buildSkillMD(name, "Available guide", "BODY"))
	}
	cfg := NewConfig(context.Background(),
		WithDisableAutoSkills(true),
		WithSkillsFS(vfs),
	)
	mgr := cfg.GetSkillsContextManager()
	require.NoError(t, mgr.LoadSkill("loaded"))

	first, err := BuildSessionSkillInventory(mgr, 1, 40)
	require.NoError(t, err)
	require.Len(t, first.Skills, 40)
	require.Equal(t, 101, first.Total)
	require.True(t, first.HasMore)
	require.Equal(t, 1, first.Page)
	require.Equal(t, 40, first.PageSize)

	last, err := BuildSessionSkillInventory(mgr, 3, 40)
	require.NoError(t, err)
	require.Len(t, last.Skills, 21)
	require.Equal(t, 101, last.Total)
	require.False(t, last.HasMore)
	require.Equal(t, "loaded", last.Skills[len(last.Skills)-1].Name)
	require.Equal(t, CapabilityInventoryStageLoaded, last.Skills[len(last.Skills)-1].Stage)
}

func TestBuildSessionSkillInventoryIncludesLazyDatabaseSkills(t *testing.T) {
	db := newAutoloadTestDB(t)
	defer db.Close()
	source, err := aiskillloader.NewAutoSkillLoader(aiskillloader.WithAutoLoad_FileSystem(buildAutoloadVFS()))
	require.NoError(t, err)
	persisted, err := aiskillloader.ImportAISkillsToDB(db, source)
	require.NoError(t, err)
	require.Equal(t, 3, persisted)
	forges, err := yakit.GetAIForgesByType(db, schema.FORGE_TYPE_SkillMD)
	require.NoError(t, err)
	require.Len(t, forges, 3)

	loader, err := aiskillloader.NewAutoSkillLoader(aiskillloader.WithAutoLoad_Database(db))
	require.NoError(t, err)
	require.Empty(t, loader.AllSkillMetas(), "fixture must keep database skills out of the eager cache")
	metas, total, err := loader.ListSkillMetas(0, 0)
	require.NoError(t, err)
	require.Equal(t, 3, total)
	require.Len(t, metas, 3)
	mgr := aiskillloader.NewSkillsContextManager(loader)

	inventory, err := BuildSessionSkillInventory(mgr, 1, 100)
	require.NoError(t, err)
	require.Equal(t, 3, inventory.Total)
	require.Len(t, inventory.Skills, 3)
	names := make(map[string]bool)
	for _, item := range inventory.Skills {
		names[item.Name] = true
		require.Equal(t, CapabilityInventoryStageMetadata, item.Stage)
	}
	require.True(t, names["deploy-app"])
	require.True(t, names["vuln-scan"])
	require.True(t, names["code-review"])
	require.Empty(t, loader.AllSkillMetas(), "listing inventory must not populate the lazy loader cache")
}

func TestBuildSessionSkillDetailUsesSkillMDBeforeLoading(t *testing.T) {
	cfg := newSessionSkillQueryConfig(t)
	detail, err := BuildSessionSkillDetail(cfg.GetSkillsContextManager(), "available")
	require.NoError(t, err)
	require.False(t, detail.Loaded)
	require.Equal(t, "available", detail.Name)
	require.Equal(t, "Available guide", detail.Meta.Description)
	require.Contains(t, detail.SkillMDContent, "AVAILABLE_SKILL_BODY")
	require.Contains(t, detail.FileTree, "SKILL.md")
	require.Empty(t, detail.PromptContent)

	raw, err := json.Marshal(detail)
	require.NoError(t, err)
	require.Contains(t, string(raw), `"skill_md":`)
	require.NotContains(t, string(raw), `"file_system":`)
	require.NotContains(t, string(raw), `"parts":`)
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
	require.Contains(t, detail.PromptContent, "TIMELINE_METADATA")
	require.Contains(t, detail.PromptContent, "TIMELINE_BODY")
}

func TestBuildSessionSkillResourceURL(t *testing.T) {
	cfg := newSessionSkillQueryConfig(t)
	rawURL := BuildSessionSkillResourceURL(cfg, "loaded skill")
	parsed, err := url.Parse(rawURL)
	require.NoError(t, err)
	require.Equal(t, SessionSkillResourceURLPath, parsed.Path)
	require.Equal(t, "loaded skill", parsed.Query().Get("skill_name"))
	require.Equal(t, "persistent-session-skill", parsed.Query().Get("session_id"))
	require.Equal(t, "runtime-session-skill", parsed.Query().Get("runtime_id"))
	require.Empty(t, BuildSessionSkillResourceURL(nil, "loaded"))
	require.Empty(t, BuildSessionSkillResourceURL(cfg, ""))
}

func TestBuildSessionSkillDetailUnknownAndEmptyName(t *testing.T) {
	mgr := newSessionSkillQueryConfig(t).GetSkillsContextManager()
	_, err := BuildSessionSkillDetail(mgr, "")
	require.ErrorContains(t, err, "name is empty")
	_, err = BuildSessionSkillDetail(mgr, "missing")
	require.ErrorContains(t, err, `skill "missing" not found`)
	inventory, err := BuildSessionSkillInventory(nil, 1, 100)
	require.NoError(t, err)
	require.Empty(t, inventory.Skills)
}
