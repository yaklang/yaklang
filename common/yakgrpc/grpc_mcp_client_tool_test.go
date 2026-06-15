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
	// Drop the table first so that any stale schema from a previous run
	// (e.g. leftover full_name NOT NULL column from MCPServerToolConfig) does
	// not interfere. AutoMigrate re-creates it with the correct structure.
	db.DropTableIfExists(&schema.MCPClientToolConfig{})
	if err := db.AutoMigrate(
		&schema.MCPServer{},
		&schema.MCPServerToolConfig{},
		&schema.MCPClientToolConfig{},
	).Error; err != nil {
		panic(err)
	}
}

// newStreamableMCPServer spins up an in-process MCP server over Streamable HTTP
// with the provided tools pre-registered. Callers must defer the returned close func.
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
		_ = yakit.DeleteMCPServerToolConfigs(db, name)
		db.Unscoped().Where("server_name = ?", name).Delete(&schema.MCPClientToolConfig{})
		db.Unscoped().Where("name = ?", name).Delete(&schema.MCPServer{})
	}
}

// cleanupClientToolConfigs removes test tool rows from the profile DB.
func cleanupClientToolConfigs(toolNames ...string) {
	db := consts.GetGormProfileDatabase()
	db.Unscoped().Where("tool_name IN (?)", toolNames).Delete(&schema.MCPClientToolConfig{})
}

func TestSeedMCPServer_CleanupRemovesMCPServerToolConfigs(t *testing.T) {
	const srvName = "grpc-test-cleanup-srv"
	db := consts.GetGormProfileDatabase()

	cleanup := seedMCPServer(t, srvName, "http://unused.example/mcp")
	require.NoError(t, yakit.SyncAndCacheMCPServerTools(db, srvName, []yakit.MCPToolEntry{
		{
			ToolName:    "cleanup_tool",
			FullName:    buildBridgeToolName(srvName, "cleanup_tool"),
			Description: "leftover test row",
			ParamsJSON:  "[]",
		},
	}))

	var before int
	require.NoError(t, db.Model(&schema.MCPServerToolConfig{}).
		Where("server_name = ?", srvName).Count(&before).Error)
	require.Equal(t, 1, before)

	cleanup()

	var after int
	require.NoError(t, db.Model(&schema.MCPServerToolConfig{}).
		Where("server_name = ?", srvName).Count(&after).Error)
	assert.Equal(t, 0, after)
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
	defer cleanupClientToolConfigs(toolName)

	_, err = yakit.GetOrCreateMCPClientToolConfig(db, toolName, schema.MCPClientToolSourceBuiltin, "", "test tool")
	require.NoError(t, err)

	t.Run("disables an existing tool", func(t *testing.T) {
		resp, err := grpcSrv.SetMCPToolEnabled(ctx, &ypb.SetMCPToolEnabledRequest{
			ToolName: toolName,
			Enable:   false,
		})
		require.NoError(t, err)
		assert.True(t, resp.GetOk())

		cfg, err := yakit.GetMCPClientToolConfigByName(db, toolName)
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

		cfg, err := yakit.GetMCPClientToolConfigByName(db, toolName)
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

	resp, err := grpcSrv.GetMCPToolList(ctx, &ypb.GetMCPToolListRequest{
		Source:     schema.MCPClientToolSourceBuiltin,
		Pagination: &ypb.Paging{Page: 1, Limit: 50},
	})
	require.NoError(t, err)
	require.NotNil(t, resp)

	assert.Greater(t, resp.GetTotal(), int64(0), "expected at least one builtin tool to be registered")

	for _, tool := range resp.GetTools() {
		assert.Equal(t, schema.MCPClientToolSourceBuiltin, tool.GetSource())
		assert.NotEmpty(t, tool.GetToolName())
		assert.NotEmpty(t, tool.GetDescription(),
			"builtin tool %q should have a description from the live Go definition", tool.GetToolName())
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// GetMCPToolList — bridge tools (ForceSync=true)
// ─────────────────────────────────────────────────────────────────────────────

func TestGRPC_GetMCPToolList_BridgeTools(t *testing.T) {
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

	resp, err := grpcSrv.GetMCPToolList(ctx, &ypb.GetMCPToolListRequest{
		Source:     schema.MCPClientToolSourceBridge,
		ServerName: srvName,
		ForceSync:  true,
		Pagination: &ypb.Paging{Page: 1, Limit: 20},
	})
	require.NoError(t, err)
	require.NotNil(t, resp)

	assert.Equal(t, int64(2), resp.GetTotal())

	toolNames := make(map[string]string)
	for _, tool := range resp.GetTools() {
		assert.Equal(t, schema.MCPClientToolSourceBridge, tool.GetSource())
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

	_, err = grpcSrv.GetMCPToolList(ctx, &ypb.GetMCPToolListRequest{
		Source:     schema.MCPClientToolSourceBridge,
		ServerName: srvName,
		ForceSync:  true,
		Pagination: &ypb.Paging{Page: 1, Limit: 20},
	})
	require.NoError(t, err)

	// Inject a stale row the server no longer returns.
	db := consts.GetGormProfileDatabase()
	err = yakit.UpsertMCPClientToolConfigDescription(db,
		buildBridgeToolName(srvName, "ghost_tool"),
		schema.MCPClientToolSourceBridge, srvName, "should be removed")
	require.NoError(t, err)

	resp, err := grpcSrv.GetMCPToolList(ctx, &ypb.GetMCPToolListRequest{
		Source:     schema.MCPClientToolSourceBridge,
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

	_, err = grpcSrv.GetMCPToolList(ctx, &ypb.GetMCPToolListRequest{
		Source:     schema.MCPClientToolSourceBridge,
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
		assert.Equal(t, schema.MCPClientToolSourceBridge, detail.GetSource())
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

	_, err = grpcSrv.GetMCPToolList(ctx, &ypb.GetMCPToolListRequest{
		Source:     schema.MCPClientToolSourceBridge,
		ServerName: srvName,
		ForceSync:  true,
		Pagination: &ypb.Paging{Page: 1, Limit: 20},
	})
	require.NoError(t, err)

	canonicalA := buildBridgeToolName(srvName, "en_tool_a")
	_, disableErr := grpcSrv.SetMCPToolEnabled(ctx, &ypb.SetMCPToolEnabledRequest{
		ToolName: canonicalA,
		Enable:   false,
	})
	require.NoError(t, disableErr)

	resp, err := grpcSrv.GetMCPToolList(ctx, &ypb.GetMCPToolListRequest{
		Source:      schema.MCPClientToolSourceBridge,
		ServerName:  srvName,
		OnlyEnabled: true,
		Pagination:  &ypb.Paging{Page: 1, Limit: 20},
	})
	require.NoError(t, err)
	assert.Equal(t, int64(1), resp.GetTotal())
	assert.Equal(t, buildBridgeToolName(srvName, "en_tool_b"), resp.GetTools()[0].GetToolName())
}

// ─────────────────────────────────────────────────────────────────────────────
// GetMCPToolList — keyword search
// ─────────────────────────────────────────────────────────────────────────────

// Keyword filter must match tool names and descriptions; unrelated tools must
// not appear in the result.
func TestGRPC_GetMCPToolList_KeywordSearch(t *testing.T) {
	toolFoo := mcpmodel.NewTool("kw_foo_tool", mcpmodel.WithDescription("foo unique description"))
	toolBar := mcpmodel.NewTool("kw_bar_tool", mcpmodel.WithDescription("bar unique description"))

	serverURL, closeServer := newStreamableMCPServer(t, toolFoo, toolBar)
	defer closeServer()

	const srvName = "grpc-test-kw-srv"
	cleanup := seedMCPServer(t, srvName, serverURL)
	defer cleanup()

	grpcSrv, err := NewServer()
	require.NoError(t, err)
	ctx := context.Background()

	// Populate DB.
	_, err = grpcSrv.GetMCPToolList(ctx, &ypb.GetMCPToolListRequest{
		Source: schema.MCPClientToolSourceBridge, ServerName: srvName,
		ForceSync: true, Pagination: &ypb.Paging{Page: 1, Limit: 20},
	})
	require.NoError(t, err)

	t.Run("keyword matches tool name", func(t *testing.T) {
		resp, err := grpcSrv.GetMCPToolList(ctx, &ypb.GetMCPToolListRequest{
			Keyword:    "kw_foo",
			Pagination: &ypb.Paging{Page: 1, Limit: 20},
		})
		require.NoError(t, err)
		require.Equal(t, int64(1), resp.GetTotal())
		assert.Contains(t, resp.GetTools()[0].GetToolName(), "kw_foo")
	})

	t.Run("keyword matches description", func(t *testing.T) {
		resp, err := grpcSrv.GetMCPToolList(ctx, &ypb.GetMCPToolListRequest{
			Keyword:    "bar unique",
			Pagination: &ypb.Paging{Page: 1, Limit: 20},
		})
		require.NoError(t, err)
		require.Equal(t, int64(1), resp.GetTotal())
		assert.Contains(t, resp.GetTools()[0].GetToolName(), "kw_bar")
	})

	t.Run("unmatched keyword returns empty", func(t *testing.T) {
		resp, err := grpcSrv.GetMCPToolList(ctx, &ypb.GetMCPToolListRequest{
			Keyword:    "zzz_no_match_keyword",
			ServerName: srvName,
			Pagination: &ypb.Paging{Page: 1, Limit: 20},
		})
		require.NoError(t, err)
		assert.Equal(t, int64(0), resp.GetTotal())
		assert.Empty(t, resp.GetTools())
	})
}

// ─────────────────────────────────────────────────────────────────────────────
// GetMCPToolList — pagination: Total is consistent with actual rows
// ─────────────────────────────────────────────────────────────────────────────

func TestGRPC_GetMCPToolList_PaginationConsistency(t *testing.T) {
	// Register 3 bridge tools.
	tools := []*mcpmodel.Tool{
		mcpmodel.NewTool("pg_tool_1", mcpmodel.WithDescription("pg1")),
		mcpmodel.NewTool("pg_tool_2", mcpmodel.WithDescription("pg2")),
		mcpmodel.NewTool("pg_tool_3", mcpmodel.WithDescription("pg3")),
	}
	serverURL, closeServer := newStreamableMCPServer(t, tools...)
	defer closeServer()

	const srvName = "grpc-test-pg-srv"
	cleanup := seedMCPServer(t, srvName, serverURL)
	defer cleanup()

	grpcSrv, err := NewServer()
	require.NoError(t, err)
	ctx := context.Background()

	_, err = grpcSrv.GetMCPToolList(ctx, &ypb.GetMCPToolListRequest{
		Source: schema.MCPClientToolSourceBridge, ServerName: srvName,
		ForceSync: true, Pagination: &ypb.Paging{Page: 1, Limit: 20},
	})
	require.NoError(t, err)

	// Fetch page 1 with limit=2, then page 2 with limit=2.
	resp1, err := grpcSrv.GetMCPToolList(ctx, &ypb.GetMCPToolListRequest{
		Source: schema.MCPClientToolSourceBridge, ServerName: srvName,
		Pagination: &ypb.Paging{Page: 1, Limit: 2},
	})
	require.NoError(t, err)
	assert.Equal(t, int64(3), resp1.GetTotal(), "Total must reflect all rows, not just this page")
	assert.Len(t, resp1.GetTools(), 2)

	resp2, err := grpcSrv.GetMCPToolList(ctx, &ypb.GetMCPToolListRequest{
		Source: schema.MCPClientToolSourceBridge, ServerName: srvName,
		Pagination: &ypb.Paging{Page: 2, Limit: 2},
	})
	require.NoError(t, err)
	assert.Equal(t, int64(3), resp2.GetTotal())
	assert.Len(t, resp2.GetTools(), 1)

	// Page 1 and page 2 results must be disjoint.
	names1 := map[string]struct{}{}
	for _, tool := range resp1.GetTools() {
		names1[tool.GetToolName()] = struct{}{}
	}
	for _, tool := range resp2.GetTools() {
		assert.NotContains(t, names1, tool.GetToolName())
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// GetMCPToolDetail — builtin tool has description and params from live definition
// ─────────────────────────────────────────────────────────────────────────────

func TestGRPC_GetMCPToolDetail_BuiltinToolMeta(t *testing.T) {
	grpcSrv, err := NewServer()
	require.NoError(t, err)
	ctx := context.Background()

	// Populate builtin tools via GetMCPToolList.
	resp, err := grpcSrv.GetMCPToolList(ctx, &ypb.GetMCPToolListRequest{
		Source:     schema.MCPClientToolSourceBuiltin,
		Pagination: &ypb.Paging{Page: 1, Limit: 1},
	})
	require.NoError(t, err)
	require.Greater(t, resp.GetTotal(), int64(0), "need at least one builtin tool")

	toolName := resp.GetTools()[0].GetToolName()
	detail, err := grpcSrv.GetMCPToolDetail(ctx, &ypb.GetMCPToolDetailRequest{ToolName: toolName})
	require.NoError(t, err)

	assert.Equal(t, toolName, detail.GetToolName())
	assert.Equal(t, schema.MCPClientToolSourceBuiltin, detail.GetSource())
	// Builtin tools must always carry a live description.
	assert.NotEmpty(t, detail.GetDescription(),
		"builtin tool %q must have description from live Go definition", toolName)
}

// ─────────────────────────────────────────────────────────────────────────────
// SetMCPToolEnabled — unknown tool name is rejected
// ─────────────────────────────────────────────────────────────────────────────

func TestGRPC_SetMCPToolEnabled_UnknownToolReturnsError(t *testing.T) {
	grpcSrv, err := NewServer()
	require.NoError(t, err)
	ctx := context.Background()

	// Tool has never been discovered — SetMCPToolEnabled must reject the request.
	resp, err := grpcSrv.SetMCPToolEnabled(ctx, &ypb.SetMCPToolEnabledRequest{
		ToolName: "grpc_phantom_tool_that_does_not_exist",
		Enable:   false,
	})
	require.NoError(t, err)
	assert.False(t, resp.GetOk(), "must return Ok=false for an unknown tool")
	assert.NotEmpty(t, resp.GetReason(), "must return a non-empty reason")
}

// ─────────────────────────────────────────────────────────────────────────────
// ForceSync=false — no network, serve entirely from DB cache
// ─────────────────────────────────────────────────────────────────────────────

// When ForceSync=false the server is never dialed. We prove this by shutting
// down the external MCP server before the second call: the cached DB rows must
// still be returned without error, and no network error should surface.
func TestGRPC_GetMCPToolList_ForceSyncFalseServesCache(t *testing.T) {
	tool := mcpmodel.NewTool("cache_tool", mcpmodel.WithDescription("cache desc"))

	serverURL, closeServer := newStreamableMCPServer(t, tool)

	const srvName = "grpc-test-cache-srv"
	cleanup := seedMCPServer(t, srvName, serverURL)
	defer cleanup()

	grpcSrv, err := NewServer()
	require.NoError(t, err)
	ctx := context.Background()

	// Populate cache via ForceSync=true while the server is up.
	_, err = grpcSrv.GetMCPToolList(ctx, &ypb.GetMCPToolListRequest{
		Source: schema.MCPClientToolSourceBridge, ServerName: srvName,
		ForceSync: true, Pagination: &ypb.Paging{Page: 1, Limit: 20},
	})
	require.NoError(t, err)

	// Take down the server — any dial attempt will fail.
	closeServer()

	// ForceSync=false must read from cache and succeed.
	resp, err := grpcSrv.GetMCPToolList(ctx, &ypb.GetMCPToolListRequest{
		Source: schema.MCPClientToolSourceBridge, ServerName: srvName,
		ForceSync: false, Pagination: &ypb.Paging{Page: 1, Limit: 20},
	})
	require.NoError(t, err)
	assert.Equal(t, int64(1), resp.GetTotal(), "cached row must be returned when ForceSync=false")
	assert.Equal(t, buildBridgeToolName(srvName, "cache_tool"), resp.GetTools()[0].GetToolName())
}

// ─────────────────────────────────────────────────────────────────────────────
// GetMCPToolList prunes builtin rows for tools no longer in the registry
// ─────────────────────────────────────────────────────────────────────────────

// Simulates a builtin tool being removed or renamed: manually insert a stale
// builtin row, then call GetMCPToolList. The stale row must not appear in the
// response because DeleteStaleMCPClientBuiltinTools prunes it during sync.
func TestGRPC_GetMCPToolList_PrunesStaleBuiltinRows(t *testing.T) {
	db := consts.GetGormProfileDatabase()

	const staleName = "grpc_test_stale_builtin_tool_xyz"
	// Insert a row as if it were a real builtin (it is not in GlobalBuiltinTools).
	_, err := yakit.GetOrCreateMCPClientToolConfig(db, staleName, schema.MCPClientToolSourceBuiltin, "", "stale desc")
	require.NoError(t, err)
	defer db.Where("tool_name = ?", staleName).Delete(&schema.MCPClientToolConfig{})

	grpcSrv, err := NewServer()
	require.NoError(t, err)
	ctx := context.Background()

	resp, err := grpcSrv.GetMCPToolList(ctx, &ypb.GetMCPToolListRequest{
		Source:     schema.MCPClientToolSourceBuiltin,
		Keyword:    staleName,
		Pagination: &ypb.Paging{Page: 1, Limit: 20},
	})
	require.NoError(t, err)
	assert.Equal(t, int64(0), resp.GetTotal(), "stale builtin row must have been pruned by GetMCPToolList")
}

// ─────────────────────────────────────────────────────────────────────────────
// Bridge tool removed on remote — stale row persists until ForceSync=true
// ─────────────────────────────────────────────────────────────────────────────

// When a remote server removes a tool:
//   - ForceSync=false still returns the stale cached row (no network).
//   - ForceSync=true dials the server and prunes the deleted row.
func TestGRPC_GetMCPToolList_BridgeToolRemovedRequiresForceSync(t *testing.T) {
	toolA := mcpmodel.NewTool("rm2_tool_a", mcpmodel.WithDescription("A"))
	toolB := mcpmodel.NewTool("rm2_tool_b", mcpmodel.WithDescription("B"))

	serverURL, closeServer := newStreamableMCPServer(t, toolA, toolB)
	defer closeServer()

	const srvName = "grpc-test-rm2-srv"
	cleanup := seedMCPServer(t, srvName, serverURL)
	defer cleanup()

	grpcSrv, err := NewServer()
	require.NoError(t, err)
	ctx := context.Background()

	// Populate DB with both tools.
	_, err = grpcSrv.GetMCPToolList(ctx, &ypb.GetMCPToolListRequest{
		Source: schema.MCPClientToolSourceBridge, ServerName: srvName,
		ForceSync: true, Pagination: &ypb.Paging{Page: 1, Limit: 20},
	})
	require.NoError(t, err)

	// Restart server with only toolA — toolB has been removed.
	closeServer()
	serverURL2, closeServer2 := newStreamableMCPServer(t, toolA)
	defer closeServer2()

	db := consts.GetGormProfileDatabase()
	db.Model(&schema.MCPServer{}).Where("name = ?", srvName).Update("url", serverURL2)

	// ForceSync=false — stale row for rm2_tool_b must still be in cache.
	resp, err := grpcSrv.GetMCPToolList(ctx, &ypb.GetMCPToolListRequest{
		Source: schema.MCPClientToolSourceBridge, ServerName: srvName,
		ForceSync: false, Pagination: &ypb.Paging{Page: 1, Limit: 20},
	})
	require.NoError(t, err)
	assert.Equal(t, int64(2), resp.GetTotal(), "stale row must persist when ForceSync=false")

	// ForceSync=true — dials server and prunes the removed tool.
	resp, err = grpcSrv.GetMCPToolList(ctx, &ypb.GetMCPToolListRequest{
		Source: schema.MCPClientToolSourceBridge, ServerName: srvName,
		ForceSync: true, Pagination: &ypb.Paging{Page: 1, Limit: 20},
	})
	require.NoError(t, err)
	assert.Equal(t, int64(1), resp.GetTotal(), "removed tool must be pruned after ForceSync=true")
	assert.Equal(t, buildBridgeToolName(srvName, "rm2_tool_a"), resp.GetTools()[0].GetToolName())
}

// ─────────────────────────────────────────────────────────────────────────────
// Bridge tool renamed on remote — requires ForceSync=true to reflect change
// ─────────────────────────────────────────────────────────────────────────────

// When a remote server renames a tool, ForceSync=false returns the stale name
// from cache; ForceSync=true performs the full diff and replaces the old row.
func TestGRPC_GetMCPToolList_BridgeToolRenamedRequiresForceSync(t *testing.T) {
	oldTool := mcpmodel.NewTool("rename_old", mcpmodel.WithDescription("original"))

	serverURL, closeServer := newStreamableMCPServer(t, oldTool)
	defer closeServer()

	const srvName = "grpc-test-rename-srv"
	cleanup := seedMCPServer(t, srvName, serverURL)
	defer cleanup()

	grpcSrv, err := NewServer()
	require.NoError(t, err)
	ctx := context.Background()

	// Discover the original tool name.
	_, err = grpcSrv.GetMCPToolList(ctx, &ypb.GetMCPToolListRequest{
		Source: schema.MCPClientToolSourceBridge, ServerName: srvName,
		ForceSync: true, Pagination: &ypb.Paging{Page: 1, Limit: 20},
	})
	require.NoError(t, err)

	// Replace server with a renamed tool.
	closeServer()
	newTool := mcpmodel.NewTool("rename_new", mcpmodel.WithDescription("renamed"))
	serverURL2, closeServer2 := newStreamableMCPServer(t, newTool)
	defer closeServer2()

	db := consts.GetGormProfileDatabase()
	db.Model(&schema.MCPServer{}).Where("name = ?", srvName).Update("url", serverURL2)

	// ForceSync=false — still serves stale cache, old name visible.
	resp, err := grpcSrv.GetMCPToolList(ctx, &ypb.GetMCPToolListRequest{
		Source: schema.MCPClientToolSourceBridge, ServerName: srvName,
		ForceSync: false, Pagination: &ypb.Paging{Page: 1, Limit: 20},
	})
	require.NoError(t, err)
	assert.Equal(t, int64(1), resp.GetTotal())
	assert.Equal(t, buildBridgeToolName(srvName, "rename_old"), resp.GetTools()[0].GetToolName(),
		"stale name must still appear when ForceSync=false")

	// ForceSync=true — full diff: old row deleted, new row inserted.
	resp, err = grpcSrv.GetMCPToolList(ctx, &ypb.GetMCPToolListRequest{
		Source: schema.MCPClientToolSourceBridge, ServerName: srvName,
		ForceSync: true, Pagination: &ypb.Paging{Page: 1, Limit: 20},
	})
	require.NoError(t, err)
	assert.Equal(t, int64(1), resp.GetTotal(), "only the new name must exist after ForceSync=true")
	assert.Equal(t, buildBridgeToolName(srvName, "rename_new"), resp.GetTools()[0].GetToolName())

	_, detailErr := grpcSrv.GetMCPToolDetail(ctx, &ypb.GetMCPToolDetailRequest{
		ToolName: buildBridgeToolName(srvName, "rename_old"),
	})
	assert.Error(t, detailErr, "old tool name must be gone after ForceSync=true")
}

// ─────────────────────────────────────────────────────────────────────────────
// Bridge tool newly added on remote — requires ForceSync=true to discover
// ─────────────────────────────────────────────────────────────────────────────

// When a remote server exposes a new tool, ForceSync=false returns the old
// cached list; ForceSync=true dials the server and picks up the new tool.
func TestGRPC_GetMCPToolList_BridgeToolAddedRequiresForceSync(t *testing.T) {
	toolExisting := mcpmodel.NewTool("add2_existing", mcpmodel.WithDescription("existing"))

	serverURL, closeServer := newStreamableMCPServer(t, toolExisting)
	defer closeServer()

	const srvName = "grpc-test-add2-srv"
	cleanup := seedMCPServer(t, srvName, serverURL)
	defer cleanup()

	grpcSrv, err := NewServer()
	require.NoError(t, err)
	ctx := context.Background()

	// Seed cache with the original single tool.
	_, err = grpcSrv.GetMCPToolList(ctx, &ypb.GetMCPToolListRequest{
		Source: schema.MCPClientToolSourceBridge, ServerName: srvName,
		ForceSync: true, Pagination: &ypb.Paging{Page: 1, Limit: 20},
	})
	require.NoError(t, err)

	// Restart the server with an extra tool.
	closeServer()
	toolNew := mcpmodel.NewTool("add2_new", mcpmodel.WithDescription("newly added"))
	serverURL2, closeServer2 := newStreamableMCPServer(t, toolExisting, toolNew)
	defer closeServer2()

	db := consts.GetGormProfileDatabase()
	db.Model(&schema.MCPServer{}).Where("name = ?", srvName).Update("url", serverURL2)

	// ForceSync=false — new tool not yet visible, cache has 1 row.
	resp, err := grpcSrv.GetMCPToolList(ctx, &ypb.GetMCPToolListRequest{
		Source: schema.MCPClientToolSourceBridge, ServerName: srvName,
		ForceSync: false, Pagination: &ypb.Paging{Page: 1, Limit: 20},
	})
	require.NoError(t, err)
	assert.Equal(t, int64(1), resp.GetTotal(), "new tool must NOT appear when ForceSync=false")

	// ForceSync=true — dials server and inserts the new tool row.
	resp, err = grpcSrv.GetMCPToolList(ctx, &ypb.GetMCPToolListRequest{
		Source: schema.MCPClientToolSourceBridge, ServerName: srvName,
		ForceSync: true, Pagination: &ypb.Paging{Page: 1, Limit: 20},
	})
	require.NoError(t, err)
	assert.Equal(t, int64(2), resp.GetTotal(), "new tool must appear after ForceSync=true")

	names := make(map[string]struct{})
	for _, tool := range resp.GetTools() {
		names[tool.GetToolName()] = struct{}{}
	}
	assert.Contains(t, names, buildBridgeToolName(srvName, "add2_existing"))
	assert.Contains(t, names, buildBridgeToolName(srvName, "add2_new"))
}
