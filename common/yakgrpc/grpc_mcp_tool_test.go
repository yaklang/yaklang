package yakgrpc

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	mcpmodel "github.com/yaklang/yaklang/common/mcp/mcp-go/mcp"
	rawserver "github.com/yaklang/yaklang/common/mcp/mcp-go/server"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func init() {
	db := consts.GetGormProfileDatabase()
	db.AutoMigrate(&schema.MCPToolConfig{})
	db.AutoMigrate(&schema.MCPServer{})
}

// newStreamableMCPServer spins up an in-process MCP server over Streamable HTTP
// with the provided tools pre-registered. Callers must defer testServer.Close().
func newStreamableMCPServer(t *testing.T, tools ...*mcpmodel.Tool) (testServerURL string, close func()) {
	t.Helper()
	srv := rawserver.NewMCPServer("test-bridge-server", "1.0.0")
	for _, tool := range tools {
		t := tool // capture
		srv.AddTool(t, func(ctx context.Context, req mcpmodel.CallToolRequest) (*mcpmodel.CallToolResult, error) {
			return &mcpmodel.CallToolResult{
				Content: []interface{}{mcpmodel.TextContent{Type: "text", Text: "ok"}},
			}, nil
		})
	}
	ts := rawserver.NewStreamableHTTPTestServer(srv)
	return ts.URL + rawserver.DefaultStreamableHTTPPath, ts.Close
}

// seedMCPServer registers an external MCP server in the profile DB so that
// YieldEnabledMCPServers will pick it up. Returns cleanup func.
func seedMCPServer(t *testing.T, name, toolListURL string) func() {
	t.Helper()
	db := consts.GetGormProfileDatabase()
	srv := &schema.MCPServer{
		Name:   name,
		Type:   "streamable_http",
		URL:    toolListURL,
		Enable: true,
	}
	require.NoError(t, yakit.CreateOrUpdateMCPServer(db, srv))
	return func() {
		db.Unscoped().Where("name = ?", name).Delete(&schema.MCPServer{})
		db.Unscoped().Where("server_name = ?", name).Delete(&schema.MCPToolConfig{})
	}
}

// cleanupToolConfigs removes test tool rows from the profile DB.
func cleanupToolConfigs(toolNames ...string) {
	db := consts.GetGormProfileDatabase()
	db.Unscoped().Where("tool_name IN (?)", toolNames).Delete(&schema.MCPToolConfig{})
}

// ─────────────────────────────────────────────────────────────────────────────
// SetMCPToolEnabled
// ─────────────────────────────────────────────────────────────────────────────

func TestGRPC_SetMCPToolEnabled(t *testing.T) {
	grpcSrv, err := NewServer()
	require.NoError(t, err)
	ctx := context.Background()

	db := consts.GetGormProfileDatabase()
	const toolName = "grpc_test_tool_enable"
	defer cleanupToolConfigs(toolName)

	// Pre-create a config row so we have something to toggle.
	_, err = yakit.GetOrCreateMCPToolConfig(db, toolName, schema.MCPToolSourceBuiltin, "", "test tool")
	require.NoError(t, err)

	t.Run("disables an existing tool", func(t *testing.T) {
		resp, err := grpcSrv.SetMCPToolEnabled(ctx, &ypb.SetMCPToolEnabledRequest{
			ToolName: toolName,
			Enable:   false,
		})
		require.NoError(t, err)
		assert.True(t, resp.GetOk())

		cfg, err := yakit.GetMCPToolConfigByName(db, toolName)
		require.NoError(t, err)
		assert.False(t, cfg.Enable)
	})

	t.Run("re-enables the tool", func(t *testing.T) {
		resp, err := grpcSrv.SetMCPToolEnabled(ctx, &ypb.SetMCPToolEnabledRequest{
			ToolName: toolName,
			Enable:   true,
		})
		require.NoError(t, err)
		assert.True(t, resp.GetOk())

		cfg, err := yakit.GetMCPToolConfigByName(db, toolName)
		require.NoError(t, err)
		assert.True(t, cfg.Enable)
	})

	t.Run("rejects empty tool name", func(t *testing.T) {
		resp, err := grpcSrv.SetMCPToolEnabled(ctx, &ypb.SetMCPToolEnabledRequest{
			ToolName: "",
			Enable:   false,
		})
		require.NoError(t, err)
		assert.False(t, resp.GetOk())
	})
}

// ─────────────────────────────────────────────────────────────────────────────
// GetMCPToolList — builtin tools
// ─────────────────────────────────────────────────────────────────────────────

func TestGRPC_GetMCPToolList_BuiltinTools(t *testing.T) {
	grpcSrv, err := NewServer()
	require.NoError(t, err)
	ctx := context.Background()

	// Call with source=builtin to limit scope to builtin tools only.
	resp, err := grpcSrv.GetMCPToolList(ctx, &ypb.GetMCPToolListRequest{
		Source:     schema.MCPToolSourceBuiltin,
		Pagination: &ypb.Paging{Page: 1, Limit: 50},
	})
	require.NoError(t, err)
	require.NotNil(t, resp)

	// The global registry should have registered at least one builtin set.
	assert.Greater(t, resp.GetTotal(), int64(0), "expected at least one builtin tool to be registered")

	// Every returned item must be source=builtin and have a non-empty description.
	for _, tool := range resp.GetTools() {
		assert.Equal(t, schema.MCPToolSourceBuiltin, tool.GetSource())
		assert.NotEmpty(t, tool.GetToolName())
		assert.NotEmpty(t, tool.GetDescription(),
			"builtin tool %q should have a description from the live Go definition", tool.GetToolName())
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// GetMCPToolList — bridge tools (ForceSync=true)
// ─────────────────────────────────────────────────────────────────────────────

func TestGRPC_GetMCPToolList_BridgeTools(t *testing.T) {
	// Register two tools on an in-process MCP server.
	toolAlpha := mcpmodel.NewTool("alpha", mcpmodel.WithDescription("Alpha tool description"))
	toolBeta := mcpmodel.NewTool("beta", mcpmodel.WithDescription("Beta tool description"))

	serverURL, closeServer := newStreamableMCPServer(t, toolAlpha, toolBeta)
	defer closeServer()

	const srvName = "grpc-test-bridge-srv"
	cleanup := seedMCPServer(t, srvName, serverURL)
	defer cleanup()

	grpcSrv, err := NewServer()
	require.NoError(t, err)
	ctx := context.Background()

	// ForceSync=true to ensure we actually connect and discover tools.
	resp, err := grpcSrv.GetMCPToolList(ctx, &ypb.GetMCPToolListRequest{
		Source:     schema.MCPToolSourceBridge,
		ServerName: srvName,
		ForceSync:  true,
		Pagination: &ypb.Paging{Page: 1, Limit: 20},
	})
	require.NoError(t, err)
	require.NotNil(t, resp)

	assert.Equal(t, int64(2), resp.GetTotal())

	toolNames := make(map[string]string) // canonical name → description
	for _, tool := range resp.GetTools() {
		assert.Equal(t, schema.MCPToolSourceBridge, tool.GetSource())
		assert.Equal(t, srvName, tool.GetServerName())
		toolNames[tool.GetToolName()] = tool.GetDescription()
	}

	assert.Contains(t, toolNames, buildBridgeToolName(srvName, "alpha"))
	assert.Contains(t, toolNames, buildBridgeToolName(srvName, "beta"))
	assert.Equal(t, "Alpha tool description", toolNames[buildBridgeToolName(srvName, "alpha")])
	assert.Equal(t, "Beta tool description", toolNames[buildBridgeToolName(srvName, "beta")])
}

// ─────────────────────────────────────────────────────────────────────────────
// GetMCPToolList — reconcile removes stale tools on ForceSync
// ─────────────────────────────────────────────────────────────────────────────

func TestGRPC_GetMCPToolList_ForceSyncRemovesStaleTools(t *testing.T) {
	// Start with two tools; then shrink to one and verify the removed one disappears.
	toolAlpha := mcpmodel.NewTool("alpha_stale", mcpmodel.WithDescription("Alpha"))
	toolBeta := mcpmodel.NewTool("beta_stale", mcpmodel.WithDescription("Beta"))

	serverURL, closeServer := newStreamableMCPServer(t, toolAlpha, toolBeta)
	defer closeServer()

	const srvName = "grpc-test-stale-srv"
	cleanup := seedMCPServer(t, srvName, serverURL)
	defer cleanup()

	grpcSrv, err := NewServer()
	require.NoError(t, err)
	ctx := context.Background()

	// First sync: discovers both tools.
	_, err = grpcSrv.GetMCPToolList(ctx, &ypb.GetMCPToolListRequest{
		Source:     schema.MCPToolSourceBridge,
		ServerName: srvName,
		ForceSync:  true,
		Pagination: &ypb.Paging{Page: 1, Limit: 20},
	})
	require.NoError(t, err)

	// Manually inject a stale row that the server no longer returns.
	db := consts.GetGormProfileDatabase()
	err = yakit.UpsertMCPToolConfigDescription(db,
		buildBridgeToolName(srvName, "ghost_tool"),
		schema.MCPToolSourceBridge, srvName, "should be removed")
	require.NoError(t, err)

	// Second sync with ForceSync=true: ghost_tool should be pruned.
	resp, err := grpcSrv.GetMCPToolList(ctx, &ypb.GetMCPToolListRequest{
		Source:     schema.MCPToolSourceBridge,
		ServerName: srvName,
		ForceSync:  true,
		Pagination: &ypb.Paging{Page: 1, Limit: 20},
	})
	require.NoError(t, err)
	assert.Equal(t, int64(2), resp.GetTotal(), "ghost tool must have been pruned")

	for _, tool := range resp.GetTools() {
		assert.NotEqual(t, buildBridgeToolName(srvName, "ghost_tool"), tool.GetToolName())
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// GetMCPToolDetail
// ─────────────────────────────────────────────────────────────────────────────

func TestGRPC_GetMCPToolDetail(t *testing.T) {
	toolAlpha := mcpmodel.NewTool("detail_alpha", mcpmodel.WithDescription("Detail alpha desc"))

	serverURL, closeServer := newStreamableMCPServer(t, toolAlpha)
	defer closeServer()

	const srvName = "grpc-test-detail-srv"
	cleanup := seedMCPServer(t, srvName, serverURL)
	defer cleanup()

	grpcSrv, err := NewServer()
	require.NoError(t, err)
	ctx := context.Background()

	// Populate the DB via GetMCPToolList first.
	_, err = grpcSrv.GetMCPToolList(ctx, &ypb.GetMCPToolListRequest{
		Source:     schema.MCPToolSourceBridge,
		ServerName: srvName,
		ForceSync:  true,
		Pagination: &ypb.Paging{Page: 1, Limit: 20},
	})
	require.NoError(t, err)

	t.Run("returns correct detail for bridge tool", func(t *testing.T) {
		canonicalName := buildBridgeToolName(srvName, "detail_alpha")
		detail, err := grpcSrv.GetMCPToolDetail(ctx, &ypb.GetMCPToolDetailRequest{
			ToolName: canonicalName,
		})
		require.NoError(t, err)
		assert.Equal(t, canonicalName, detail.GetToolName())
		assert.Equal(t, schema.MCPToolSourceBridge, detail.GetSource())
		assert.Equal(t, srvName, detail.GetServerName())
		assert.Equal(t, "Detail alpha desc", detail.GetDescription())
	})

	t.Run("returns error for unknown tool", func(t *testing.T) {
		_, err := grpcSrv.GetMCPToolDetail(ctx, &ypb.GetMCPToolDetailRequest{
			ToolName: "mcp_nonexistent_srv_nonexistent_tool",
		})
		assert.Error(t, err)
	})

	t.Run("returns error for empty tool name", func(t *testing.T) {
		_, err := grpcSrv.GetMCPToolDetail(ctx, &ypb.GetMCPToolDetailRequest{
			ToolName: "",
		})
		assert.Error(t, err)
	})
}

// ─────────────────────────────────────────────────────────────────────────────
// Disabled tools are excluded from GetMCPToolList when OnlyEnabled=true
// ─────────────────────────────────────────────────────────────────────────────

func TestGRPC_GetMCPToolList_OnlyEnabledFiltersDisabled(t *testing.T) {
	toolA := mcpmodel.NewTool("en_tool_a", mcpmodel.WithDescription("Tool A"))
	toolB := mcpmodel.NewTool("en_tool_b", mcpmodel.WithDescription("Tool B"))

	serverURL, closeServer := newStreamableMCPServer(t, toolA, toolB)
	defer closeServer()

	const srvName = "grpc-test-filter-srv"
	cleanup := seedMCPServer(t, srvName, serverURL)
	defer cleanup()

	grpcSrv, err := NewServer()
	require.NoError(t, err)
	ctx := context.Background()

	// Discover tools.
	_, err = grpcSrv.GetMCPToolList(ctx, &ypb.GetMCPToolListRequest{
		Source:     schema.MCPToolSourceBridge,
		ServerName: srvName,
		ForceSync:  true,
		Pagination: &ypb.Paging{Page: 1, Limit: 20},
	})
	require.NoError(t, err)

	// Disable tool_a.
	canonicalA := buildBridgeToolName(srvName, "en_tool_a")
	_, disableErr := grpcSrv.SetMCPToolEnabled(ctx, &ypb.SetMCPToolEnabledRequest{
		ToolName: canonicalA,
		Enable:   false,
	})
	require.NoError(t, disableErr)

	// OnlyEnabled=true should exclude tool_a.
	resp, err := grpcSrv.GetMCPToolList(ctx, &ypb.GetMCPToolListRequest{
		Source:      schema.MCPToolSourceBridge,
		ServerName:  srvName,
		OnlyEnabled: true,
		Pagination:  &ypb.Paging{Page: 1, Limit: 20},
	})
	require.NoError(t, err)
	assert.Equal(t, int64(1), resp.GetTotal())
	assert.Equal(t, buildBridgeToolName(srvName, "en_tool_b"), resp.GetTools()[0].GetToolName())
}
