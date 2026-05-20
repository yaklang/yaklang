package aicommon

import (
	"testing"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools"
	"github.com/stretchr/testify/require"
)

type testCapabilityInventoryLoop struct {
	extraTools []*aitool.Tool
}

func (t *testCapabilityInventoryLoop) PromptCandidateTools() []*aitool.Tool { return nil }
func (t *testCapabilityInventoryLoop) ScenarioToolWhitelist() []string     { return nil }
func (t *testCapabilityInventoryLoop) AllowToolCall() bool                 { return true }
func (t *testCapabilityInventoryLoop) DynamicExtraTools() []*aitool.Tool {
	return t.extraTools
}
func (t *testCapabilityInventoryLoop) DynamicForges() []CapabilityInventoryNamedItem { return nil }
func (t *testCapabilityInventoryLoop) LoadedSkills() []CapabilityInventoryNamedItem { return nil }

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

	require.NotEmpty(t, payload.Fixed.Tools)
	require.Equal(t, "grep", payload.Fixed.Tools[0].Name)

	foundRuntimeInDynamic := false
	for _, item := range payload.Dynamic.Tools {
		if item.Name == "runtime_only" {
			foundRuntimeInDynamic = true
		}
	}
	require.True(t, foundRuntimeInDynamic, "runtime-only tool should appear in dynamic section")
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
func (l *allowToolCallLoop) LoadedSkills() []CapabilityInventoryNamedItem       { return nil }

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
