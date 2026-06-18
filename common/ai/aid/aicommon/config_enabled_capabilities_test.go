package aicommon

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestParseEnabledCapabilitiesFromProto(t *testing.T) {
	params := &ypb.AIStartParams{
		EnabledCapabilities: []*ypb.AIEnabledCapability{
			{Name: " read_file ", Type: "TOOL"},
			{Name: "recon", Type: "skill"},
			{Name: "csrf-check", Type: "plugin"},
			{Name: "code-review", Type: "forge"},
			{Name: "my-mcp-server", Type: "mcp_tool"},
			{Name: "mcp_server_echo", Type: "mcp"},
			{Name: "", Type: "tool"},
			{Name: "dup", Type: "tool"},
			{Name: "dup", Type: "tool"},
		},
	}

	got := ParseEnabledCapabilitiesFromProto(params)
	require.Len(t, got, 7)
	require.Equal(t, EnabledCapability{Name: "read_file", Type: EnabledCapabilityTypeTool}, got[0])
	require.Equal(t, EnabledCapability{Name: "recon", Type: EnabledCapabilityTypeSkill}, got[1])
	require.Equal(t, EnabledCapability{Name: "csrf-check", Type: EnabledCapabilityTypePlugin}, got[2])
	require.Equal(t, EnabledCapability{Name: "code-review", Type: EnabledCapabilityTypeForge}, got[3])
	require.Equal(t, EnabledCapability{Name: "my-mcp-server", Type: EnabledCapabilityTypeMCPTool}, got[4])
	require.Equal(t, EnabledCapability{Name: "mcp_server_echo", Type: EnabledCapabilityTypeMCPTool}, got[5])
}

func TestWithEnabledCapabilities_StoresAndAppliesTools(t *testing.T) {
	cfg := NewConfig(context.Background(), WithDisableAutoSkills(true))
	require.NotNil(t, cfg.AiToolManager)

	err := WithEnabledCapabilities(
		EnabledCapability{Name: "read_file", Type: EnabledCapabilityTypeTool},
	)(cfg)
	require.NoError(t, err)

	caps := cfg.GetEnabledCapabilities()
	require.Len(t, caps, 1)
	require.Equal(t, "read_file", caps[0].Name)
}

func TestMergeEnabledCapabilitiesHotpatch(t *testing.T) {
	base := &ypb.AIStartParams{
		EnabledCapabilities: []*ypb.AIEnabledCapability{
			{Name: "read_file", Type: "tool"},
		},
	}
	patch := &ypb.AIStartParams{
		EnabledCapabilities: []*ypb.AIEnabledCapability{
			{Name: "recon", Type: "skill"},
			{Name: "read_file", Type: "tool"},
		},
	}

	merged := MergeEnabledCapabilitiesHotpatch(base, patch)
	require.Len(t, merged, 2)
	require.Equal(t, "read_file", merged[0].GetName())
	require.Equal(t, "recon", merged[1].GetName())
}

func TestProcessHotPatchMessage_EnabledCapabilitiesMerge(t *testing.T) {
	cfg := NewConfig(context.Background(), WithDisableAutoSkills(true))
	require.NoError(t, WithEnabledCapabilities(
		EnabledCapability{Name: "read_file", Type: EnabledCapabilityTypeTool},
	)(cfg))

	var hotpatched []EnabledCapability
	cfg.SetCapabilityHotpatchHandler(func(enable bool, caps []EnabledCapability) {
		require.True(t, enable)
		hotpatched = append(hotpatched, caps...)
	})

	opts := cfg.ProcessHotPatchMessage(&ypb.AIInputEvent{
		IsConfigHotpatch: true,
		HotpatchType:     HotPatchType_EnabledCapabilities,
		Params: &ypb.AIStartParams{
			EnabledCapabilities: []*ypb.AIEnabledCapability{
				{Name: "grep", Type: "tool"},
			},
		},
	})
	require.Len(t, opts, 1)
	require.NoError(t, opts[0](cfg))

	require.Len(t, hotpatched, 1)
	require.Equal(t, "grep", hotpatched[0].Name)
	// Hotpatch is prompt-level only; startup registry stays unchanged.
	caps := cfg.GetEnabledCapabilities()
	require.Len(t, caps, 1)
	require.Equal(t, "read_file", caps[0].Name)
}

func TestSubtractEnabledCapabilitiesHotpatch(t *testing.T) {
	base := &ypb.AIStartParams{
		EnabledCapabilities: []*ypb.AIEnabledCapability{
			{Name: "read_file", Type: "tool"},
			{Name: "recon", Type: "skill"},
		},
	}
	patch := &ypb.AIStartParams{
		EnabledCapabilities: []*ypb.AIEnabledCapability{
			{Name: "read_file", Type: "tool"},
		},
	}

	remaining := SubtractEnabledCapabilitiesHotpatch(base, patch)
	require.Len(t, remaining, 1)
	require.Equal(t, "recon", remaining[0].GetName())
}

func TestProcessHotPatchMessage_DisabledCapabilities(t *testing.T) {
	cfg := NewConfig(context.Background(), WithDisableAutoSkills(true))
	require.NoError(t, WithEnabledCapabilities(
		EnabledCapability{Name: "read_file", Type: EnabledCapabilityTypeTool},
		EnabledCapability{Name: "grep", Type: EnabledCapabilityTypeTool},
	)(cfg))

	var hotpatched []EnabledCapability
	cfg.SetCapabilityHotpatchHandler(func(enable bool, caps []EnabledCapability) {
		require.False(t, enable)
		hotpatched = append(hotpatched, caps...)
	})

	opts := cfg.ProcessHotPatchMessage(&ypb.AIInputEvent{
		IsConfigHotpatch: true,
		HotpatchType:     HotPatchType_DisabledCapabilities,
		Params: &ypb.AIStartParams{
			EnabledCapabilities: []*ypb.AIEnabledCapability{
				{Name: "read_file", Type: "tool"},
			},
		},
	})
	require.Len(t, opts, 1)
	require.NoError(t, opts[0](cfg))

	require.Len(t, hotpatched, 1)
	require.Equal(t, "read_file", hotpatched[0].Name)
	// Hotpatch is prompt-level only; startup registry stays unchanged.
	caps := cfg.GetEnabledCapabilities()
	require.Len(t, caps, 2)
}

func TestWithDisabledCapabilities_RemovesTool(t *testing.T) {
	cfg := NewConfig(context.Background(), WithDisableAutoSkills(true))
	toolName := "ls"
	require.NoError(t, WithEnabledCapabilities(
		EnabledCapability{Name: toolName, Type: EnabledCapabilityTypeTool},
	)(cfg))
	enabledBefore, err := cfg.AiToolManager.GetEnableTools()
	require.NoError(t, err)
	require.Contains(t, toolNames(enabledBefore), toolName)

	require.NoError(t, WithDisabledCapabilities(
		EnabledCapability{Name: toolName, Type: EnabledCapabilityTypeTool},
	)(cfg))
	enabledAfter, err := cfg.AiToolManager.GetEnableTools()
	require.NoError(t, err)
	require.NotContains(t, toolNames(enabledAfter), toolName)
	require.Empty(t, cfg.GetEnabledCapabilities())
}

func TestWithDisabledCapabilities_RemovesAppendedPlugin(t *testing.T) {
	cfg := NewConfig(context.Background(), WithDisableAutoSkills(true))
	pluginTool := aitool.NewWithoutCallback("runtime_plugin_demo", aitool.WithDescription("test plugin"))
	require.NoError(t, cfg.AiToolManager.AppendTools(pluginTool))
	require.NoError(t, WithEnabledCapabilities(
		EnabledCapability{Name: "runtime_plugin_demo", Type: EnabledCapabilityTypePlugin},
	)(cfg))

	_, err := cfg.AiToolManager.GetToolByName("runtime_plugin_demo")
	require.NoError(t, err)

	require.NoError(t, WithDisabledCapabilities(
		EnabledCapability{Name: "runtime_plugin_demo", Type: EnabledCapabilityTypePlugin},
	)(cfg))
	_, err = cfg.AiToolManager.GetToolByName("runtime_plugin_demo")
	require.Error(t, err)
}

func toolNames(tools []*aitool.Tool) []string {
	names := make([]string, 0, len(tools))
	for _, tool := range tools {
		if tool != nil {
			names = append(names, tool.Name)
		}
	}
	return names
}

func TestWithEnabledCapabilities_EmitsCapabilityInventory(t *testing.T) {
	emitted := false
	cfg := NewConfig(context.Background(), WithDisableAutoSkills(true))
	cfg.SetCapabilityInventoryEmitHandler(func() {
		emitted = true
	})
	require.NoError(t, WithEnabledCapabilities(
		EnabledCapability{Name: "ls", Type: EnabledCapabilityTypeTool},
	)(cfg))
	require.True(t, emitted)
}

func TestWithDisabledCapabilities_EmitsCapabilityInventory(t *testing.T) {
	emitted := false
	cfg := NewConfig(context.Background(), WithDisableAutoSkills(true))
	require.NoError(t, WithEnabledCapabilities(
		EnabledCapability{Name: "ls", Type: EnabledCapabilityTypeTool},
	)(cfg))
	cfg.SetCapabilityInventoryEmitHandler(func() {
		emitted = true
	})
	require.NoError(t, WithDisabledCapabilities(
		EnabledCapability{Name: "ls", Type: EnabledCapabilityTypeTool},
	)(cfg))
	require.True(t, emitted)
}

func TestGetEnabledCapabilityNamesByType(t *testing.T) {
	cfg := NewConfig(context.Background(), WithDisableAutoSkills(true))
	require.NoError(t, WithEnabledCapabilities(
		EnabledCapability{Name: "recon", Type: EnabledCapabilityTypeSkill},
		EnabledCapability{Name: "code-review", Type: EnabledCapabilityTypeForge},
		EnabledCapability{Name: "read_file", Type: EnabledCapabilityTypeTool},
	)(cfg))

	require.Equal(t, []string{"recon"}, cfg.GetEnabledSkillNames())
	require.Equal(t, []string{"code-review"}, cfg.GetEnabledForgeNames())
}

func TestProcessHotPatchMessage_CapabilityHotpatch_RespectsTaskId(t *testing.T) {
	cfgA := NewConfig(context.Background(), WithDisableAutoSkills(true))
	cfgB := NewConfig(context.Background(), WithDisableAutoSkills(true))
	cfgA.SetHotpatchCurrentTaskIdResolver(func() string { return "task-a" })
	cfgB.SetHotpatchCurrentTaskIdResolver(func() string { return "task-b" })
	cfgA.SetCapabilityHotpatchHandler(func(enable bool, caps []EnabledCapability) {
		require.True(t, enable)
		require.Len(t, caps, 1)
	})

	opts := cfgA.ProcessHotPatchMessage(&ypb.AIInputEvent{
		IsConfigHotpatch: true,
		HotpatchType:     HotPatchType_EnabledCapabilities,
		TaskId:           "task-a",
		Params: &ypb.AIStartParams{
			EnabledCapabilities: []*ypb.AIEnabledCapability{
				{Name: "grep", Type: "tool"},
			},
		},
	})
	require.Len(t, opts, 1)

	require.NoError(t, opts[0](cfgA))
	// Hotpatch should NOT mutate enabledCapabilities registry; it is prompt-level only.
	require.Empty(t, cfgA.GetEnabledCapabilities())

	require.NoError(t, opts[0](cfgB))
	require.Empty(t, cfgB.GetEnabledCapabilities())
}

func TestProcessHotPatchMessage_CapabilityHotpatch_EmptyTaskIdAppliesToAll(t *testing.T) {
	cfgA := NewConfig(context.Background(), WithDisableAutoSkills(true))
	cfgB := NewConfig(context.Background(), WithDisableAutoSkills(true))
	cfgA.SetHotpatchCurrentTaskIdResolver(func() string { return "task-a" })
	cfgB.SetHotpatchCurrentTaskIdResolver(func() string { return "task-b" })
	var calledA, calledB bool
	cfgA.SetCapabilityHotpatchHandler(func(enable bool, caps []EnabledCapability) { calledA = true })
	cfgB.SetCapabilityHotpatchHandler(func(enable bool, caps []EnabledCapability) { calledB = true })

	opts := cfgA.ProcessHotPatchMessage(&ypb.AIInputEvent{
		IsConfigHotpatch: true,
		HotpatchType:     HotPatchType_EnabledCapabilities,
		Params: &ypb.AIStartParams{
			EnabledCapabilities: []*ypb.AIEnabledCapability{
				{Name: "grep", Type: "tool"},
			},
		},
	})
	require.Len(t, opts, 1)

	require.NoError(t, opts[0](cfgA))
	require.NoError(t, opts[0](cfgB))
	require.True(t, calledA)
	require.True(t, calledB)
}
