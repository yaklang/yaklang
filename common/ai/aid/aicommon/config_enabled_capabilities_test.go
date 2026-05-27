package aicommon

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
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

	caps := cfg.GetEnabledCapabilities()
	require.Len(t, caps, 2)
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
