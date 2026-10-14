package aicommon

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/aiskillloader"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools"
	"github.com/yaklang/yaklang/common/utils/filesys"
)

func testSkillInventoryVFS() *filesys.VirtualFS {
	vfs := filesys.NewVirtualFs()
	vfs.AddFile("deploy-app/SKILL.md", "---\nname: deploy-app\ndescription: deploy\n---\n# Deploy\n")
	vfs.AddFile("code-review/SKILL.md", "---\nname: code-review\ndescription: review\n---\n# Review\n")
	return vfs
}

type testCapabilityInventoryLoop struct {
	extraTools      []*aitool.Tool
	inventorySkills []CapabilityInventoryNamedItem
}

func (t *testCapabilityInventoryLoop) PromptCandidateTools() []*aitool.Tool { return nil }
func (t *testCapabilityInventoryLoop) ScenarioToolWhitelist() []string      { return nil }
func (t *testCapabilityInventoryLoop) AllowToolCall() bool                  { return true }
func (t *testCapabilityInventoryLoop) DynamicExtraTools() []*aitool.Tool {
	return t.extraTools
}
func (t *testCapabilityInventoryLoop) DynamicForges() []CapabilityInventoryNamedItem { return nil }
func (t *testCapabilityInventoryLoop) InventorySkills() []CapabilityInventoryNamedItem {
	return t.inventorySkills
}

func TestBuildCapabilityInventoryPayload_SplitsFixedAndDynamicTools(t *testing.T) {
	fixedTool := aitool.NewWithoutCallback("grep", aitool.WithDescription("grep files"))
	runtimeTool := aitool.NewWithoutCallback("runtime_only", aitool.WithDescription("loaded later"))

	toolManager := buildinaitools.NewToolManager(
		buildinaitools.WithExtendTools([]*aitool.Tool{fixedTool, runtimeTool}, true),
	)

	cfg := &Config{
		AiToolManager: toolManager,
		TopToolsCount: 100,
	}

	payload := BuildCapabilityInventoryPayload(cfg, nil)

	fixedNames := make(map[string]struct{})
	for _, tool := range payload.Fixed.Tools {
		fixedNames[tool.Name] = struct{}{}
	}
	require.Contains(t, fixedNames, "grep")
	require.Contains(t, fixedNames, "runtime_only")
	require.Empty(t, payload.Dynamic.Tools, "display tools stay in fixed section (FrozenBlock prompt position)")
}

func TestBuildCapabilityInventoryPayload_IncludesExtraCapabilitiesTools(t *testing.T) {
	fixedTool := aitool.NewWithoutCallback("grep", aitool.WithDescription("grep files"))
	extraTool := aitool.NewWithoutCallback("extra_discovered", aitool.WithDescription("from intent"))

	toolManager := buildinaitools.NewToolManager(
		buildinaitools.WithExtendTools([]*aitool.Tool{fixedTool, extraTool}, true),
	)

	cfg := &Config{
		AiToolManager: toolManager,
		TopToolsCount: 100,
	}

	payload := BuildCapabilityInventoryPayload(cfg, &testCapabilityInventoryLoop{
		extraTools: []*aitool.Tool{extraTool},
	})

	foundExtra := false
	for _, item := range payload.Dynamic.Tools {
		if item.Name == "extra_discovered" {
			foundExtra = true
		}
	}
	require.True(t, foundExtra)
}

func TestBuildCapabilityInventoryPayload_RespectsAllowToolCall(t *testing.T) {
	tool := aitool.NewWithoutCallback("grep", aitool.WithDescription("grep files"))
	toolManager := buildinaitools.NewToolManager(
		buildinaitools.WithExtendTools([]*aitool.Tool{tool}, true),
	)
	cfg := &Config{
		AiToolManager: toolManager,
		TopToolsCount: 100,
	}

	payloadAllowed := BuildCapabilityInventoryPayload(cfg, &testCapabilityInventoryLoop{})
	require.NotEmpty(t, payloadAllowed.Fixed.Tools)

	payloadDisallowed := BuildCapabilityInventoryPayload(cfg, &allowToolCallLoop{allow: false})
	require.Empty(t, payloadDisallowed.Fixed.Tools)
	require.Empty(t, payloadDisallowed.Dynamic.Tools)
}

type allowToolCallLoop struct {
	allow bool
}

func (l *allowToolCallLoop) PromptCandidateTools() []*aitool.Tool            { return nil }
func (l *allowToolCallLoop) ScenarioToolWhitelist() []string                 { return nil }
func (l *allowToolCallLoop) AllowToolCall() bool                             { return l.allow }
func (l *allowToolCallLoop) DynamicExtraTools() []*aitool.Tool               { return nil }
func (l *allowToolCallLoop) DynamicForges() []CapabilityInventoryNamedItem   { return nil }
func (l *allowToolCallLoop) InventorySkills() []CapabilityInventoryNamedItem { return nil }

func TestIsFixedInventoryTool(t *testing.T) {
	require.True(t, IsFixedInventoryTool("grep"))
	require.True(t, IsFixedInventoryTool("search_capabilities"))
	require.False(t, IsFixedInventoryTool("runtime_only"))
	require.False(t, IsFixedInventoryTool(""))
}

func TestSelectToolInventoryTools_FiltersHiddenTools(t *testing.T) {
	visible := aitool.NewWithoutCallback("grep", aitool.WithDescription("grep files"))
	hidden := aitool.NewWithoutCallback("walking_plan", aitool.WithDescription("amap walking"))

	cfg := &Config{TopToolsCount: 100}
	selection := ResolvePromptToolInventory(cfg, []*aitool.Tool{visible, hidden}, nil, true)

	require.Len(t, selection.VisibleTools, 1)
	require.Equal(t, "grep", selection.VisibleTools[0].Name)
	require.Len(t, selection.DisplayTools, 1)
	require.Equal(t, "grep", selection.DisplayTools[0].Name)
}

func TestResolvePromptCandidateTools_FallsBackToEnableTools(t *testing.T) {
	tool := aitool.NewWithoutCallback("grep", aitool.WithDescription("grep"))
	toolManager := buildinaitools.NewToolManager(
		buildinaitools.WithExtendTools([]*aitool.Tool{tool}, true),
	)
	cfg := &Config{AiToolManager: toolManager}

	got := ResolvePromptCandidateTools(cfg, nil)
	require.NotEmpty(t, got)
	found := false
	for _, item := range got {
		if item != nil && item.Name == "grep" {
			found = true
			break
		}
	}
	require.True(t, found)
}

func TestBuildInventorySkillsFromLoader_MetadataAndLoaded(t *testing.T) {
	loader, err := aiskillloader.NewFSSkillLoader(testSkillInventoryVFS())
	require.NoError(t, err)

	all := BuildInventorySkillsFromLoader(loader, nil)
	require.Len(t, all, 2)
	byName := map[string]CapabilityInventoryNamedItem{}
	for _, item := range all {
		byName[item.Name] = item
	}
	require.Equal(t, CapabilityInventorySkillLoadMetadata, byName["code-review"].SkillLoadState)
	require.Equal(t, CapabilityInventorySkillLoadMetadata, byName["deploy-app"].SkillLoadState)

	loaded, err := loader.LoadSkill("deploy-app")
	require.NoError(t, err)
	require.NotNil(t, loaded)

	mgr := aiskillloader.NewSkillsContextManager(loader)
	cfg := NewConfig(nil, WithDisableAutoSkills(true))
	bindTestTimeline(mgr, cfg)
	require.NoError(t, mgr.LoadSkill("deploy-app"))

	withLoaded := BuildInventorySkillsFromManager(mgr)
	require.Len(t, withLoaded, 2)
	byName = map[string]CapabilityInventoryNamedItem{}
	for _, item := range withLoaded {
		byName[item.Name] = item
	}
	require.Equal(t, CapabilityInventorySkillLoadLoaded, byName["deploy-app"].SkillLoadState)
	require.Equal(t, CapabilityInventorySkillLoadMetadata, byName["code-review"].SkillLoadState)
	require.Equal(t, mgr.GetCurrentSelectedSkills()[0].TokenCount, byName["deploy-app"].TokenCount)
	require.Positive(t, byName["deploy-app"].TokenCount)
	require.Zero(t, byName["code-review"].TokenCount)
}

func TestBuildInventorySkillsFromManagerIncludesTimelineSkillTokens(t *testing.T) {
	for _, withLoader := range []bool{false, true} {
		name := "timeline_only"
		if withLoader {
			name = "outside_registry_budget"
		}
		t.Run(name, func(t *testing.T) {
			var loader aiskillloader.SkillLoader
			if withLoader {
				var err error
				loader, err = aiskillloader.NewFSSkillLoader(testSkillInventoryVFS())
				require.NoError(t, err)
			}
			mgr := aiskillloader.NewSkillsContextManager(loader,
				aiskillloader.WithManagerTokenEstimator(func(string) int {
					return aiskillloader.MetadataListMaxTokens + 1
				}))
			cfg := NewConfig(nil, WithDisableAutoSkills(true))
			bindTestTimeline(mgr, cfg)
			tl := cfg.GetTimeline()
			require.True(t, tl.PushSkillMetadata(cfg.AcquireId(), "deploy-app", "SKILL_METADATA", SkillContentSourceManual))
			require.True(t, tl.PushSkillFileContent(cfg.AcquireId(), "deploy-app", "SKILL.md", "SKILL_BODY", SkillContentSourceManual, 1, 1, 1))
			expected := MeasureTokens("SKILL_METADATA") + MeasureTokens("SKILL_BODY")

			items := BuildInventorySkillsFromManager(mgr)
			require.Len(t, items, 1, "active Timeline skills must be included regardless of the loader registry")
			require.Equal(t, "deploy-app", items[0].Name)
			require.Equal(t, CapabilityInventorySkillLoadLoaded, items[0].SkillLoadState)
			require.Equal(t, expected, items[0].TokenCount)
			tl.FreezeAll()
			require.Equal(t, items, BuildInventorySkillsFromManager(mgr))
		})
	}
}

func TestCapabilityInventorySkillTokensReachBothFrontendPayloads(t *testing.T) {
	loader, err := aiskillloader.NewFSSkillLoader(testSkillInventoryVFS())
	require.NoError(t, err)
	mgr := aiskillloader.NewSkillsContextManager(loader)
	cfg := NewConfig(nil, WithDisableAutoSkills(true), WithDisallowMCPServers(true))
	bindTestTimeline(mgr, cfg)
	require.NoError(t, mgr.LoadSkill("deploy-app"))

	loop := &testCapabilityInventoryLoop{inventorySkills: BuildInventorySkillsFromManager(mgr)}
	expected := mgr.GetCurrentSelectedSkills()[0].TokenCount
	items := BuildCapabilityInventoryItems(cfg, loop)
	skills := map[string]CapabilityInventoryItem{}
	for _, item := range items {
		if item.Type == "skill" {
			skills[item.Name] = item
		}
	}
	require.Equal(t, CapabilityInventoryStageLoaded, skills["deploy-app"].Stage)
	require.Equal(t, expected, skills["deploy-app"].Data.(map[string]any)["token_count"])
	require.NotContains(t, skills["code-review"].Data.(map[string]any), "token_count")

	// A session_snapshot JSON round trip changes Data's integer to float64.
	raw, err := json.Marshal(&SessionSnapshot{Capabilities: items})
	require.NoError(t, err)
	var snapshot SessionSnapshot
	require.NoError(t, json.Unmarshal(raw, &snapshot))
	legacy := CapabilityInventoryPayloadFromItems(snapshot.Capabilities, nil)
	byName := map[string]CapabilityInventoryNamedItem{}
	for _, skill := range legacy.Fixed.Skills {
		byName[skill.Name] = skill
	}
	require.Equal(t, expected, byName["deploy-app"].TokenCount)
	require.Zero(t, byName["code-review"].TokenCount)
	loadedJSON, err := json.Marshal(byName["deploy-app"])
	require.NoError(t, err)
	var loadedFields map[string]any
	require.NoError(t, json.Unmarshal(loadedJSON, &loadedFields))
	require.Equal(t, float64(expected), loadedFields["token_count"])
	metadataJSON, err := json.Marshal(byName["code-review"])
	require.NoError(t, err)
	var metadataFields map[string]any
	require.NoError(t, json.Unmarshal(metadataJSON, &metadataFields))
	require.NotContains(t, metadataFields, "token_count", "registry-only skills have no loaded size")

	require.NoError(t, mgr.ChangeViewOffset("deploy-app", "SKILL.md", 2))
	loop.inventorySkills = BuildInventorySkillsFromManager(mgr)
	larger := BuildCapabilityInventoryPayload(cfg, loop)
	for _, skill := range larger.Fixed.Skills {
		if skill.Name == "deploy-app" {
			require.Greater(t, skill.TokenCount, expected, "paging adds another retained range")
			require.Equal(t, mgr.GetCurrentSelectedSkills()[0].TokenCount, skill.TokenCount)
		}
	}
	require.True(t, mgr.UnloadSkill("deploy-app"))
	loop.inventorySkills = BuildInventorySkillsFromManager(mgr)
	for _, item := range BuildCapabilityInventoryItems(cfg, loop) {
		if item.Type == "skill" {
			require.Equal(t, CapabilityInventoryStageMetadata, item.Stage)
			require.NotContains(t, item.Data.(map[string]any), "token_count", "unloading clears previously reported sizes")
		}
	}
}

func TestBuildCapabilityInventoryPayload_IncludesInventorySkills(t *testing.T) {
	loader, err := aiskillloader.NewFSSkillLoader(testSkillInventoryVFS())
	require.NoError(t, err)

	cfg := &Config{TopToolsCount: 100}
	payload := BuildCapabilityInventoryPayload(cfg, &testCapabilityInventoryLoop{
		inventorySkills: BuildInventorySkillsFromLoader(loader, nil),
	})

	require.Len(t, payload.Fixed.Skills, 2)
	for _, skill := range payload.Fixed.Skills {
		require.Equal(t, "skill", skill.Category)
		require.Equal(t, CapabilityInventorySkillLoadMetadata, skill.SkillLoadState)
	}
	require.Empty(t, payload.Dynamic.Skills)
}

func TestSelectToolInventoryTools_MoreToolsCount(t *testing.T) {
	cfg := &Config{TopToolsCount: 1}
	tools := []*aitool.Tool{
		aitool.NewWithoutCallback("grep", aitool.WithDescription("grep files")),
		aitool.NewWithoutCallback("read_file", aitool.WithDescription("read file")),
	}
	selection := ResolvePromptToolInventory(cfg, tools, nil, true)

	require.Len(t, selection.VisibleTools, 2)
	require.GreaterOrEqual(t, len(selection.DisplayTools), 1)
	require.Equal(t, len(selection.VisibleTools)-len(selection.DisplayTools), selection.MoreToolsCount())
}

func TestCapabilityInventoryPayloadFromItems_SplitsByPromptPosition(t *testing.T) {
	items := []CapabilityInventoryItem{
		{Name: "grep", Type: "aitool", Position: CapabilityInventoryPositionFrozenBlock},
		{Name: "extra_tool", Type: "aitool", Position: CapabilityInventoryPositionDynamic},
		{Name: "deploy-app", Type: "skill", Stage: CapabilityInventoryStageMetadata, Position: CapabilityInventoryPositionSemiDynamic},
		{Name: "intent-skill", Type: "skill", Stage: CapabilityInventoryStageMetadata, Position: CapabilityInventoryPositionDynamic},
		{Name: "my-forge", Type: "forge", Position: CapabilityInventoryPositionDynamic},
	}
	payload := CapabilityInventoryPayloadFromItems(items, nil)

	require.Len(t, payload.Fixed.Tools, 1)
	require.Equal(t, "grep", payload.Fixed.Tools[0].Name)
	require.Len(t, payload.Dynamic.Tools, 1)
	require.Equal(t, "extra_tool", payload.Dynamic.Tools[0].Name)

	require.Len(t, payload.Fixed.Skills, 1)
	require.Equal(t, "deploy-app", payload.Fixed.Skills[0].Name)
	require.Len(t, payload.Dynamic.Skills, 1)
	require.Equal(t, "intent-skill", payload.Dynamic.Skills[0].Name)

	require.Len(t, payload.Dynamic.Forges, 1)
	require.Equal(t, "my-forge", payload.Dynamic.Forges[0].Name)
}

func TestCapabilityInventoryPayloadFromItemsPreservesDecodedSkillTokens(t *testing.T) {
	items := []CapabilityInventoryItem{
		{Name: "loaded-skill", Type: "skill", Stage: CapabilityInventoryStageLoaded,
			Position: CapabilityInventoryPositionSemiDynamic, Data: map[string]any{"token_count": 1_500_000}},
		{Name: "metadata-skill", Type: "skill", Stage: CapabilityInventoryStageMetadata,
			Position: CapabilityInventoryPositionSemiDynamic, Data: map[string]any{"token_count": 999}},
	}
	raw, err := json.Marshal(items)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(raw, &items))
	payload := CapabilityInventoryPayloadFromItems(items, nil)
	require.Len(t, payload.Fixed.Skills, 2)
	require.Equal(t, 1_500_000, payload.Fixed.Skills[0].TokenCount, "float64 JSON counts must not be lost through scientific-notation string conversion")
	require.Zero(t, payload.Fixed.Skills[1].TokenCount, "metadata must never inherit a stale loaded count")
}
