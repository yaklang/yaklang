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

func TestBuildSessionSkillListsSplitLoadedAndAvailable(t *testing.T) {
	cfg := newSessionSkillQueryConfig(t)
	mgr := cfg.GetSkillsContextManager()
	require.NoError(t, mgr.LoadSkill("loaded"))

	loaded := BuildSessionLoadedSkillInventory(mgr)
	require.Equal(t, 1, loaded.Total)
	require.Len(t, loaded.Skills, 1)
	require.Equal(t, "loaded", loaded.Skills[0].Name)
	require.Equal(t, CapabilityInventoryStageLoaded, loaded.Skills[0].Stage)
	require.Equal(t, CapabilityInventorySkillLoadLoaded, loaded.Skills[0].Data.(map[string]any)["skill_load_state"])

	available, err := BuildSessionAvailableSkillInventory(mgr, "", 1, 100)
	require.NoError(t, err)
	require.Equal(t, 1, available.Total)
	require.Len(t, available.Skills, 1)
	require.Equal(t, "available", available.Skills[0].Name)
	for _, item := range append(loaded.Skills, available.Skills...) {
		require.Equal(t, "skill", item.Type)
		require.Equal(t, CapabilityInventoryPositionSemiDynamic, item.Position)
		require.False(t, item.IsFixed)
	}
	require.Equal(t, CapabilityInventoryStageMetadata, available.Skills[0].Stage)
	require.Equal(t, CapabilityInventorySkillLoadMetadata, available.Skills[0].Data.(map[string]any)["skill_load_state"])
	require.NotContains(t, available.Skills[0].Data.(map[string]any), "token_count")

	require.True(t, mgr.UnloadSkill("loaded"))
	require.Empty(t, BuildSessionLoadedSkillInventory(mgr).Skills)
	available, err = BuildSessionAvailableSkillInventory(mgr, "", 1, 100)
	require.NoError(t, err)
	require.Equal(t, 2, available.Total)
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

	loaded := BuildSessionLoadedSkillInventory(mgr)
	require.Len(t, loaded.Skills, 1)
	require.Equal(t, "loaded", loaded.Skills[0].Name)

	first, err := BuildSessionAvailableSkillInventory(mgr, "", 1, 40)
	require.NoError(t, err)
	require.Len(t, first.Skills, 40)
	require.Equal(t, 100, first.Total)
	require.True(t, first.HasMore)
	require.Equal(t, 1, first.Page)
	require.Equal(t, 40, first.PageSize)

	last, err := BuildSessionAvailableSkillInventory(mgr, "", 3, 40)
	require.NoError(t, err)
	require.Len(t, last.Skills, 20)
	require.Equal(t, 100, last.Total)
	require.False(t, last.HasMore)
	for _, item := range append(first.Skills, last.Skills...) {
		require.NotEqual(t, "loaded", item.Name)
		require.Equal(t, CapabilityInventoryStageMetadata, item.Stage)
	}
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

	inventory, err := BuildSessionAvailableSkillInventory(mgr, "", 1, 100)
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

func TestBuildSessionAvailableSkillInventoryMergesLocalAndDatabaseSources(t *testing.T) {
	db := newAutoloadTestDB(t)
	defer db.Close()
	dbVFS := filesys.NewVirtualFs()
	dbVFS.AddFile("db-alpha/SKILL.md", buildSkillMD("db-alpha", "Database alpha", "DB_ALPHA"))
	dbVFS.AddFile("db-omega/SKILL.md", buildSkillMD("db-omega", "Database omega", "DB_OMEGA"))
	dbSource, err := aiskillloader.NewAutoSkillLoader(aiskillloader.WithAutoLoad_FileSystem(dbVFS))
	require.NoError(t, err)
	persisted, err := aiskillloader.ImportAISkillsToDB(db, dbSource)
	require.NoError(t, err)
	require.Equal(t, 2, persisted)

	localVFS := filesys.NewVirtualFs()
	for _, name := range []string{"local-alpha", "local-mid", "local-omega"} {
		localVFS.AddFile(name+"/SKILL.md", buildSkillMD(name, "Local "+name, "LOCAL"))
	}
	loader, err := aiskillloader.NewAutoSkillLoader(
		aiskillloader.WithAutoLoad_FileSystem(localVFS),
		aiskillloader.WithAutoLoad_Database(db),
	)
	require.NoError(t, err)
	mgr := aiskillloader.NewSkillsContextManager(loader)
	bindTestTimeline(mgr, NewConfig(context.Background(), WithDisableAutoSkills(true)))
	require.NoError(t, mgr.LoadSkill("local-mid"))

	first, err := BuildSessionAvailableSkillInventory(mgr, "", 1, 2)
	require.NoError(t, err)
	require.Equal(t, 4, first.Total)
	require.Equal(t, []string{"db-alpha", "db-omega"}, []string{first.Skills[0].Name, first.Skills[1].Name})
	require.True(t, first.HasMore)

	second, err := BuildSessionAvailableSkillInventory(mgr, "", 2, 2)
	require.NoError(t, err)
	require.Equal(t, 4, second.Total)
	require.Equal(t, []string{"local-alpha", "local-omega"}, []string{second.Skills[0].Name, second.Skills[1].Name})
	require.False(t, second.HasMore)

	loaded := BuildSessionLoadedSkillInventory(mgr)
	require.Equal(t, 1, loaded.Total)
	require.Equal(t, "local-mid", loaded.Skills[0].Name)
}

func TestBuildSessionAvailableSkillSearchFiltersFullCatalog(t *testing.T) {
	db := newAutoloadTestDB(t)
	defer db.Close()
	dbVFS := filesys.NewVirtualFs()
	dbVFS.AddFile("db-scan/SKILL.md", buildSkillMD("db-scan", "Database vulnerability scan", "DB_SCAN"))
	dbVFS.AddFile("db-other/SKILL.md", buildSkillMD("db-other", "Database unrelated guide", "DB_OTHER"))
	dbSource, err := aiskillloader.NewAutoSkillLoader(aiskillloader.WithAutoLoad_FileSystem(dbVFS))
	require.NoError(t, err)
	persisted, err := aiskillloader.ImportAISkillsToDB(db, dbSource)
	require.NoError(t, err)
	require.Equal(t, 2, persisted)

	localVFS := filesys.NewVirtualFs()
	for i := 0; i < 60; i++ {
		name := fmt.Sprintf("scan-%03d", i)
		localVFS.AddFile(name+"/SKILL.md", buildSkillMD(name, "Local vulnerability scan", "LOCAL_SCAN"))
	}
	localVFS.AddFile("loaded-scan/SKILL.md", buildSkillMD("loaded-scan", "Loaded vulnerability scan", "LOADED_SCAN"))
	localVFS.AddFile("plain-note/SKILL.md", buildSkillMD("plain-note", "Unrelated guide", "PLAIN"))
	loader, err := aiskillloader.NewAutoSkillLoader(
		aiskillloader.WithAutoLoad_FileSystem(localVFS),
		aiskillloader.WithAutoLoad_Database(db),
	)
	require.NoError(t, err)
	mgr := aiskillloader.NewSkillsContextManager(loader)
	bindTestTimeline(mgr, NewConfig(context.Background(), WithDisableAutoSkills(true)))
	require.NoError(t, mgr.LoadSkill("loaded-scan"))

	first, err := BuildSessionAvailableSkillInventory(mgr, "scan", 1, 50)
	require.NoError(t, err)
	require.Equal(t, "scan", first.Query)
	require.Equal(t, 61, first.Total)
	require.Len(t, first.Skills, 50)
	require.True(t, first.HasMore)
	require.Equal(t, "db-scan", first.Skills[0].Name)
	for _, item := range first.Skills {
		require.NotEqual(t, "loaded-scan", item.Name)
		require.Equal(t, CapabilityInventoryStageMetadata, item.Stage)
	}

	second, err := BuildSessionAvailableSkillInventory(mgr, "scan", 2, 50)
	require.NoError(t, err)
	require.Equal(t, 61, second.Total)
	require.Len(t, second.Skills, 11)
	require.False(t, second.HasMore)
	require.Equal(t, "scan-059", second.Skills[len(second.Skills)-1].Name)
	repeated, err := BuildSessionAvailableSkillInventory(mgr, "scan", 2, 50)
	require.NoError(t, err)
	require.Equal(t, second.Skills[0].Name, repeated.Skills[0].Name)
	require.Equal(t, second.Skills[len(second.Skills)-1].Name, repeated.Skills[len(repeated.Skills)-1].Name)

	databaseOnly, err := BuildSessionAvailableSkillInventory(mgr, "database", 1, 100)
	require.NoError(t, err)
	require.Equal(t, 2, databaseOnly.Total)
	require.Equal(t, []string{"db-other", "db-scan"}, []string{databaseOnly.Skills[0].Name, databaseOnly.Skills[1].Name})

	blank, err := BuildSessionAvailableSkillInventory(mgr, "  ", 1, 100)
	require.NoError(t, err)
	require.Empty(t, blank.Query)
	require.Equal(t, 63, blank.Total)
	for _, item := range blank.Skills {
		require.NotEqual(t, "loaded-scan", item.Name)
	}
}

func TestBuildSessionAvailableSkillSearchUsesGenericLoaderMetadata(t *testing.T) {
	vfs := filesys.NewVirtualFs()
	vfs.AddFile("archive-only/SKILL.md", buildSkillMD(
		"archive-only",
		"Archived security guide",
		"ARCHIVE_ONLY",
	))
	vfs.AddFile("loaded-archive/SKILL.md", buildSkillMD(
		"loaded-archive",
		"Archived security notes",
		"LOADED_ARCHIVE",
	))
	loader, err := aiskillloader.NewFSSkillLoader(vfs)
	require.NoError(t, err)
	mgr := aiskillloader.NewSkillsContextManager(loader)
	bindTestTimeline(mgr, NewConfig(context.Background(), WithDisableAutoSkills(true)))
	require.NoError(t, mgr.LoadSkill("loaded-archive"))

	result, err := BuildSessionAvailableSkillInventory(mgr, "archived security", 1, 100)
	require.NoError(t, err)
	require.Equal(t, 1, result.Total)
	require.Len(t, result.Skills, 1)
	require.Equal(t, "archive-only", result.Skills[0].Name)
	require.Equal(t, CapabilityInventoryStageMetadata, result.Skills[0].Stage)
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
	require.Empty(t, BuildSessionLoadedSkillInventory(nil).Skills)
	inventory, err := BuildSessionAvailableSkillInventory(nil, "", 1, 100)
	require.NoError(t, err)
	require.Empty(t, inventory.Skills)
}
