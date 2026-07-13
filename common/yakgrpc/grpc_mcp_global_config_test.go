package yakgrpc

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/mcp"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestGRPC_GetMCPGlobalConfig_CatalogDefaults(t *testing.T) {
	client, err := NewLocalClient()
	require.NoError(t, err)

	cfg, err := client.GetMCPGlobalConfig(context.Background(), &ypb.Empty{})
	require.NoError(t, err)
	assert.True(t, cfg.GetUsesCatalogDefaults())
	assert.Equal(t, mcp.CatalogDefaultMCPToolSets, cfg.GetDefaultToolSets())
}

func TestGRPC_SetMCPGlobalConfig_AffectsStartMcpServerDefaults(t *testing.T) {
	grpcSrv, err := NewServer()
	require.NoError(t, err)
	db := grpcSrv.GetProfileDatabase()
	t.Cleanup(func() {
		_, _ = yakit.ResetMCPGlobalConfig(db)
		yakit.SetCachedMCPGlobalConfigForTest(nil)
	})

	_, err = grpcSrv.SetMCPGlobalConfig(context.Background(), &ypb.MCPGlobalConfig{
		DefaultToolSets: []string{"codec", "reverse_platform"},
	})
	require.NoError(t, err)

	names := startMCPListToolNames(t, &ypb.StartMcpServerRequest{
		Host:                  "127.0.0.1",
		Port:                  0,
		EnableAll:             false,
		EnableAIToolFramework: false,
	})

	assert.True(t, containsTool(names, "exec_codec"))
	assert.True(t, containsTool(names, "require_dnslog_domain"))
	assert.False(t, containsTool(names, "port_scan"))
}

func TestGRPC_GetToolSetList_IncludesMetadata(t *testing.T) {
	grpcSrv, err := NewServer()
	require.NoError(t, err)

	resp, err := grpcSrv.GetToolSetList(context.Background(), &ypb.Empty{})
	require.NoError(t, err)
	require.NotEmpty(t, resp.GetToolSetList())

	var reversePlatform *ypb.ToolSetInfo
	for _, item := range resp.GetToolSetList() {
		if item.GetName() == "reverse_platform" {
			reversePlatform = item
			break
		}
	}
	require.NotNil(t, reversePlatform)
	assert.Equal(t, "default", reversePlatform.GetTier())
	assert.NotEmpty(t, reversePlatform.GetSummary())
	assert.Greater(t, reversePlatform.GetToolCount(), int32(0))
	assert.True(t, reversePlatform.GetEnabledByDefault())
}

func TestGRPC_SetMCPGlobalConfig_InvalidToolSetRejected(t *testing.T) {
	grpcSrv, err := NewServer()
	require.NoError(t, err)

	_, err = grpcSrv.SetMCPGlobalConfig(context.Background(), &ypb.MCPGlobalConfig{
		DefaultToolSets: []string{"not_a_real_tool_set"},
	})
	require.Error(t, err)
}

func TestGRPC_ResetMCPGlobalConfig(t *testing.T) {
	grpcSrv, err := NewServer()
	require.NoError(t, err)
	t.Cleanup(func() {
		yakit.SetCachedMCPGlobalConfigForTest(nil)
	})

	_, err = grpcSrv.SetMCPGlobalConfig(context.Background(), &ypb.MCPGlobalConfig{
		DefaultToolSets: []string{"codec"},
	})
	require.NoError(t, err)

	reset, err := grpcSrv.ResetMCPGlobalConfig(context.Background(), &ypb.Empty{})
	require.NoError(t, err)
	assert.True(t, reset.GetUsesCatalogDefaults())
	assert.Equal(t, mcp.CatalogDefaultMCPToolSets, reset.GetDefaultToolSets())
}

func TestGRPC_SetMCPGlobalConfig_AppliesAIFlagsOnDefaultStart(t *testing.T) {
	grpcSrv, err := NewServer()
	require.NoError(t, err)
	db := grpcSrv.GetProfileDatabase()
	t.Cleanup(func() {
		_, _ = yakit.ResetMCPGlobalConfig(db)
		yakit.SetCachedMCPGlobalConfigForTest(nil)
	})

	_, err = grpcSrv.SetMCPGlobalConfig(context.Background(), &ypb.MCPGlobalConfig{
		DefaultToolSets:         []string{"codec"},
		EnableAIToolFramework:   false,
		EnableBridgeExternalMCP: false,
	})
	require.NoError(t, err)

	// Request tries to enable AI framework, but default-start assigns from global config.
	names := startMCPListToolNames(t, &ypb.StartMcpServerRequest{
		Host:                  "127.0.0.1",
		Port:                  0,
		EnableAll:             false,
		EnableAIToolFramework: true,
	})
	assert.True(t, containsTool(names, "exec_codec"))
}

func TestGRPC_SetMCPGlobalConfig_SyncsToolEnableList(t *testing.T) {
	grpcSrv, err := NewServer()
	require.NoError(t, err)
	db := grpcSrv.GetProfileDatabase()
	t.Cleanup(func() {
		_, _ = yakit.ResetMCPGlobalConfig(db)
		yakit.SetCachedMCPGlobalConfigForTest(nil)
	})

	// Ensure rows exist via list sync path.
	_, err = grpcSrv.GetMCPToolList(context.Background(), &ypb.GetMCPToolListRequest{})
	require.NoError(t, err)

	_, err = grpcSrv.SetMCPGlobalConfig(context.Background(), &ypb.MCPGlobalConfig{
		DefaultToolSets: []string{"codec"},
	})
	require.NoError(t, err)

	codec, err := yakit.GetMCPClientToolConfigByName(db, "exec_codec")
	require.NoError(t, err)
	assert.True(t, codec.Enable)

	payload, err := yakit.GetMCPClientToolConfigByName(db, "save_payload")
	require.NoError(t, err)
	assert.False(t, payload.Enable)
}

