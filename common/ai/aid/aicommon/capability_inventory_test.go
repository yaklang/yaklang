package aicommon

import (
	"testing"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon/aiskillloader"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/stretchr/testify/require"
)

func testSkillInventoryVFS() *filesys.VirtualFS {
	vfs := filesys.NewVirtualFs()
	vfs.AddFile("deploy-app/SKILL.md", "---\nname: deploy-app\ndescription: deploy\n---\n# Deploy\n")
	vfs.AddFile("code-review/SKILL.md", "---\nname: code-review\ndescription: review\n---\n# Review\n")
	return vfs
}

type testCapabilityInventoryLoop struct {
	extraTools        []*aitool.Tool
	inventorySkills   []CapabilityInventoryNamedItem
}

func (t *testCapabilityInventoryLoop) PromptCandidateTools() []*aitool.Tool { return nil }
func (t *testCapabilityInventoryLoop) ScenarioToolWhitelist() []string     { return nil }
func (t *testCapabilityInventoryLoop) AllowToolCall() bool                 { return true }
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

func (l *allowToolCallLoop) PromptCandidateTools() []*aitool.Tool               { return nil }
func (l *allowToolCallLoop) ScenarioToolWhitelist() []string                      { return nil }
func (l *allowToolCallLoop) AllowToolCall() bool                                  { return l.allow }
func (l *allowToolCallLoop) DynamicExtraTools() []*aitool.Tool                    { return nil }
func (l *allowToolCallLoop) DynamicForges() []CapabilityInventoryNamedItem        { return nil }
func (l *allowToolCallLoop) InventorySkills() []CapabilityInventoryNamedItem { return nil }

func TestIsFixedInventoryTool(t *testing.T) {
	require.True(t, IsFixedInventoryTool("grep"))
	require.True(t, IsFixedInventoryTool("search_capabilities"))
	require.False(t, IsFixedInventoryTool("runtime_only"))
	require.False(t, IsFixedInventoryTool(""))
}

func TestSelectToolInventoryTools_FiltersHiddenTools(t *testing.T) {
	visible := aitool.NewWithoutCallback("grep", aitool.WithDescription("grep files"))
	hidden := aitool.NewWithoutCallback("send_http_request_by_url", aitool.WithDescription("legacy http"))

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
	require.NoError(t, mgr.LoadSkill("deploy-app"))

	withLoaded := BuildInventorySkillsFromManager(mgr)
	require.Len(t, withLoaded, 2)
	byName = map[string]CapabilityInventoryNamedItem{}
	for _, item := range withLoaded {
		byName[item.Name] = item
	}
	require.Equal(t, CapabilityInventorySkillLoadLoaded, byName["deploy-app"].SkillLoadState)
	require.Equal(t, CapabilityInventorySkillLoadMetadata, byName["code-review"].SkillLoadState)
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
