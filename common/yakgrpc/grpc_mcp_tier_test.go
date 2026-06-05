package yakgrpc

import (
	"context"
	"io"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/mcp"
	mcpclient "github.com/yaklang/yaklang/common/mcp/mcp-go/client"
	mcpmodel "github.com/yaklang/yaklang/common/mcp/mcp-go/mcp"
	rawmcp "github.com/yaklang/yaklang/common/mcp/mcp-go/mcp"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

var tierTestNameSanitizer = regexp.MustCompile(`[^a-z0-9_]+`)

func sanitizeTierTestName(name string) string {
	name = strings.ToLower(name)
	name = strings.ReplaceAll(name, "/", "_")
	return tierTestNameSanitizer.ReplaceAllString(name, "_")
}

func syncBridgeToolsForTierTest(t *testing.T, srvName string) {
	t.Helper()
	grpcSrv, err := NewServer()
	require.NoError(t, err)
	_, err = grpcSrv.GetMCPToolList(context.Background(), &ypb.GetMCPToolListRequest{
		Source:     schema.MCPClientToolSourceBridge,
		ServerName: srvName,
		ForceSync:  true,
		Pagination: &ypb.Paging{Page: 1, Limit: 50},
	})
	require.NoError(t, err)
}

func seedBridgeMCPServerForTierTest(t *testing.T, remoteToolName string) (srvName, bridgeCanonical string, cleanup func()) {
	t.Helper()
	remoteTool := mcpmodel.NewTool(remoteToolName, mcpmodel.WithDescription(remoteToolName+" description"))
	serverURL, closeServer := newStreamableMCPServer(t, remoteTool)
	srvName = sanitizeTierTestName(t.Name())
	if len(srvName) > 48 {
		srvName = srvName[:48]
	}
	bridgeCanonical = buildBridgeToolName(srvName, remoteToolName)
	cleanupServer := seedMCPServer(t, srvName, serverURL)
	cleanup = func() {
		cleanupServer()
		closeServer()
		cleanupClientToolConfigs(bridgeCanonical)
	}
	return srvName, bridgeCanonical, cleanup
}

func mustSetMCPToolEnabled(t *testing.T, toolName string, enable bool) {
	t.Helper()
	grpcSrv, err := NewServer()
	require.NoError(t, err)
	resp, err := grpcSrv.SetMCPToolEnabled(context.Background(), &ypb.SetMCPToolEnabledRequest{
		ToolName: toolName,
		Enable:   enable,
	})
	require.NoError(t, err)
	require.True(t, resp.GetOk(), "SetMCPToolEnabled(%q, %v) failed: %s", toolName, enable, resp.GetReason())
}

func ensureMCPToolConfigExists(t *testing.T, toolName, source, serverName string) {
	t.Helper()
	db := consts.GetGormProfileDatabase()
	_, err := yakit.GetOrCreateMCPClientToolConfig(db, toolName, source, serverName, "")
	require.NoError(t, err)
}

// withOnlyEnabledMCPServer disables every other MCP server row in the profile DB
// so bridge sync/start tests only dial the in-process mock server.
func withOnlyEnabledMCPServer(t *testing.T, keepServerName string, fn func()) {
	t.Helper()
	db := consts.GetGormProfileDatabase()
	var servers []*schema.MCPServer
	require.NoError(t, db.Find(&servers).Error)

	previous := make(map[uint]bool, len(servers))
	for _, srv := range servers {
		previous[srv.ID] = srv.Enable
		wantEnable := srv.Name == keepServerName
		if srv.Enable != wantEnable {
			require.NoError(t, db.Model(&schema.MCPServer{}).Where("id = ?", srv.ID).UpdateColumn("enable", wantEnable).Error)
		}
	}

	t.Cleanup(func() {
		for id, enable := range previous {
			_ = db.Model(&schema.MCPServer{}).Where("id = ?", id).UpdateColumn("enable", enable).Error
		}
	})
	fn()
}

// startMCPListToolNames boots StartMcpServer with the given tier flags and returns
// the tool names exposed by the running MCP server.
func startMCPListToolNames(t *testing.T, req *ypb.StartMcpServerRequest) []string {
	t.Helper()

	client, err := NewLocalClient()
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	stream, err := client.StartMcpServer(ctx, req)
	require.NoError(t, err)

	var serverURL string
	for {
		resp, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			if serverURL != "" && strings.Contains(err.Error(), "context canceled") {
				break
			}
			require.NoError(t, err)
		}
		if resp.GetServerUrl() != "" {
			serverURL = resp.GetServerUrl()
		}
		if resp.GetStatus() == "running" {
			break
		}
	}
	require.NotEmpty(t, serverURL)

	mcpClient, err := mcpclient.NewSSEMCPClient(serverURL)
	require.NoError(t, err)
	defer mcpClient.Close()

	clientCtx, clientCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer clientCancel()

	require.NoError(t, mcpClient.Start(clientCtx))

	initReq := rawmcp.InitializeRequest{}
	initReq.Params.ProtocolVersion = rawmcp.LATEST_PROTOCOL_VERSION
	initReq.Params.ClientInfo = rawmcp.Implementation{Name: "tier-test-client", Version: "1.0.0"}
	_, err = mcpClient.Initialize(clientCtx, initReq)
	require.NoError(t, err)

	toolsResult, err := mcpClient.ListTools(clientCtx, rawmcp.ListToolsRequest{})
	if err != nil {
		// Some MCP clients return this when the server exposes zero tools.
		if strings.Contains(strings.ToLower(err.Error()), "tools not supported") {
			return nil
		}
		require.NoError(t, err)
	}

	names := make([]string, 0, len(toolsResult.Tools))
	for _, tool := range toolsResult.Tools {
		names = append(names, tool.Name)
	}

	// Tear down the streaming StartMcpServer call so bridge MCP clients release
	// their outbound connections before httptest mock servers are closed.
	cancel()
	time.Sleep(200 * time.Millisecond)
	return names
}

func containsTool(names []string, target string) bool {
	for _, name := range names {
		if name == target {
			return true
		}
	}
	return false
}

func TestGRPC_StartMcpServer_TierRawOnly_NoTools(t *testing.T) {
	names := startMCPListToolNames(t, &ypb.StartMcpServerRequest{
		Host:                    "127.0.0.1",
		Port:                    0,
		EnableAll:               false,
		EnableAIToolFramework:   false,
		EnableBridgeExternalMCP: false,
	})
	assert.Empty(t, names, "raw MCP server should not expose tools when all tiers are disabled")
}

func TestGRPC_StartMcpServer_TierLegacyOnly(t *testing.T) {
	names := startMCPListToolNames(t, &ypb.StartMcpServerRequest{
		Host:                  "127.0.0.1",
		Port:                  0,
		EnableAll:             true,
		EnableAIToolFramework: false,
	})

	assert.True(t, containsTool(names, "port_scan"), "legacy tier should expose port_scan")
	assert.False(t, containsTool(names, "now"), "aitool-only builtin should not appear without EnableAIToolFramework")
}

func TestGRPC_StartMcpServer_TierAIToolFrameworkOnly(t *testing.T) {
	names := startMCPListToolNames(t, &ypb.StartMcpServerRequest{
		Host:                  "127.0.0.1",
		Port:                  0,
		EnableAll:             false,
		EnableAIToolFramework: true,
	})

	assert.True(t, containsTool(names, "now"), "aitool-framework tier should expose now")
	assert.False(t, containsTool(names, "port_scan"), "legacy tool should not appear when EnableAll is false")
}

func TestGRPC_StartMcpServer_TierBothEnabled(t *testing.T) {
	names := startMCPListToolNames(t, &ypb.StartMcpServerRequest{
		Host:                  "127.0.0.1",
		Port:                  0,
		EnableAll:             true,
		EnableAIToolFramework: true,
	})

	assert.True(t, containsTool(names, "port_scan"))
	assert.True(t, containsTool(names, "now"))
}

func TestGRPC_StartMcpServer_DisabledToolNotExposed(t *testing.T) {
	grpcSrv, err := NewServer()
	require.NoError(t, err)
	ctx := context.Background()

	const legacyTool = "port_scan"
	_, err = grpcSrv.GetMCPToolList(ctx, &ypb.GetMCPToolListRequest{
		Pagination: &ypb.Paging{Page: 1, Limit: 200},
	})
	require.NoError(t, err)

	resp, err := grpcSrv.SetMCPToolEnabled(ctx, &ypb.SetMCPToolEnabledRequest{
		ToolName: legacyTool,
		Enable:   false,
	})
	require.NoError(t, err)
	require.True(t, resp.GetOk())
	t.Cleanup(func() {
		_, _ = grpcSrv.SetMCPToolEnabled(ctx, &ypb.SetMCPToolEnabledRequest{
			ToolName: legacyTool,
			Enable:   true,
		})
	})

	names := startMCPListToolNames(t, &ypb.StartMcpServerRequest{
		Host:                  "127.0.0.1",
		Port:                  0,
		EnableAll:             true,
		EnableAIToolFramework: false,
	})
	assert.False(t, containsTool(names, legacyTool), "per-tool disable should hide tool on next start")
}

func TestGRPC_GetMCPToolList_SourceSeparation(t *testing.T) {
	grpcSrv, err := NewServer()
	require.NoError(t, err)
	ctx := context.Background()

	db := consts.GetGormProfileDatabase()
	legacyNames := make(map[string]struct{})
	for name := range mcp.GlobalBuiltinTools() {
		legacyNames[name] = struct{}{}
	}
	aitoolNames := make(map[string]struct{})
	for _, tool := range buildinaitools.GetAllToolsDynamically(db) {
		if tool != nil && tool.Name != "" {
			aitoolNames[tool.Name] = struct{}{}
		}
	}
	require.NotEmpty(t, legacyNames)
	require.NotEmpty(t, aitoolNames)

	resp, err := grpcSrv.GetMCPToolList(ctx, &ypb.GetMCPToolListRequest{
		Pagination: &ypb.Paging{Page: 1, Limit: 500},
	})
	require.NoError(t, err)
	require.NotNil(t, resp)

	var legacyCount, aitoolCount int
	for _, tool := range resp.GetTools() {
		switch tool.GetSource() {
		case schema.MCPClientToolSourceBuiltin:
			legacyCount++
			assert.Contains(t, legacyNames, tool.GetToolName())
		case schema.MCPClientToolSourceAITool:
			aitoolCount++
			assert.Contains(t, aitoolNames, tool.GetToolName())
		case schema.MCPClientToolSourceBridge:
			// optional in this test
		default:
			t.Fatalf("unexpected source %q for tool %q", tool.GetSource(), tool.GetToolName())
		}
	}

	assert.Greater(t, legacyCount, 0, "legacy builtin tools should be listed separately")
	assert.Greater(t, aitoolCount, 0, "aitool-framework builtins should be listed separately")
}

func TestGRPC_GetMCPToolList_AIToolHasParams(t *testing.T) {
	grpcSrv, err := NewServer()
	require.NoError(t, err)
	ctx := context.Background()

	resp, err := grpcSrv.GetMCPToolList(ctx, &ypb.GetMCPToolListRequest{
		Source:     schema.MCPClientToolSourceAITool,
		Pagination: &ypb.Paging{Page: 1, Limit: 200},
	})
	require.NoError(t, err)

	var nowTool *ypb.MCPClientToolConfig
	for _, tool := range resp.GetTools() {
		if tool.GetToolName() == "now" {
			nowTool = tool
			break
		}
	}
	require.NotNil(t, nowTool, "aitool source should include now")
	assert.NotEmpty(t, nowTool.GetDescription())
	assert.NotEmpty(t, nowTool.GetParams(), "aitool builtin should attach params metadata")
}

// ─────────────────────────────────────────────────────────────────────────────
// StartMcpServer tier integration — mock bridge + MCP client ListTools
// ─────────────────────────────────────────────────────────────────────────────

func TestGRPCMUSTPASS_StartMcpServer_TierBridgeOnly(t *testing.T) {
	const remoteTool = "tier_bridge_echo"
	srvName, bridgeCanonical, cleanup := seedBridgeMCPServerForTierTest(t, remoteTool)
	defer cleanup()

	withOnlyEnabledMCPServer(t, srvName, func() {
		syncBridgeToolsForTierTest(t, srvName)

		names := startMCPListToolNames(t, &ypb.StartMcpServerRequest{
			Host:                    "127.0.0.1",
			Port:                    0,
			EnableAll:               false,
			EnableAIToolFramework:   false,
			EnableBridgeExternalMCP: true,
		})

		assert.True(t, containsTool(names, bridgeCanonical), "bridge tier should expose synced bridge tool")
		assert.False(t, containsTool(names, "port_scan"), "legacy tool must stay hidden when EnableAll=false")
		assert.False(t, containsTool(names, "now"), "aitool builtin must stay hidden when EnableAIToolFramework=false")
	})
}

func TestGRPCMUSTPASS_StartMcpServer_TierAllThreeSources(t *testing.T) {
	const remoteTool = "tier_all_three_tool"
	srvName, bridgeCanonical, cleanup := seedBridgeMCPServerForTierTest(t, remoteTool)
	defer cleanup()

	withOnlyEnabledMCPServer(t, srvName, func() {
		syncBridgeToolsForTierTest(t, srvName)

		names := startMCPListToolNames(t, &ypb.StartMcpServerRequest{
			Host:                    "127.0.0.1",
			Port:                    0,
			EnableAll:               true,
			EnableAIToolFramework:   true,
			EnableBridgeExternalMCP: true,
		})

		assert.True(t, containsTool(names, "port_scan"), "legacy builtin should be exposed")
		assert.True(t, containsTool(names, "now"), "aitool-framework builtin should be exposed")
		assert.True(t, containsTool(names, bridgeCanonical), "bridge tool should be exposed")
	})
}

func TestGRPCMUSTPASS_StartMcpServer_AIToolWithoutBridge(t *testing.T) {
	const remoteTool = "tier_ai_no_bridge_tool"
	srvName, bridgeCanonical, cleanup := seedBridgeMCPServerForTierTest(t, remoteTool)
	defer cleanup()

	withOnlyEnabledMCPServer(t, srvName, func() {
		syncBridgeToolsForTierTest(t, srvName)

		names := startMCPListToolNames(t, &ypb.StartMcpServerRequest{
			Host:                    "127.0.0.1",
			Port:                    0,
			EnableAll:               false,
			EnableAIToolFramework:   true,
			EnableBridgeExternalMCP: false,
		})

		assert.True(t, containsTool(names, "now"))
		assert.False(t, containsTool(names, "port_scan"))
		assert.False(t, containsTool(names, bridgeCanonical), "bridge tools must not load without EnableBridgeExternalMCP")
	})
}

func TestGRPCMUSTPASS_StartMcpServer_DisabledBridgeToolNotExposed(t *testing.T) {
	const remoteTool = "tier_bridge_disable_me"
	srvName, bridgeCanonical, cleanup := seedBridgeMCPServerForTierTest(t, remoteTool)
	defer func() {
		ensureMCPToolConfigExists(t, bridgeCanonical, schema.MCPClientToolSourceBridge, srvName)
		mustSetMCPToolEnabled(t, bridgeCanonical, true)
		cleanup()
	}()

	withOnlyEnabledMCPServer(t, srvName, func() {
		syncBridgeToolsForTierTest(t, srvName)
		ensureMCPToolConfigExists(t, bridgeCanonical, schema.MCPClientToolSourceBridge, srvName)
		mustSetMCPToolEnabled(t, bridgeCanonical, false)

		names := startMCPListToolNames(t, &ypb.StartMcpServerRequest{
			Host:                    "127.0.0.1",
			Port:                    0,
			EnableAll:               false,
			EnableAIToolFramework:   false,
			EnableBridgeExternalMCP: true,
		})
		assert.False(t, containsTool(names, bridgeCanonical), "disabled bridge tool must be filtered on start")
	})
}

func TestGRPCMUSTPASS_StartMcpServer_DisabledAIToolNotExposed(t *testing.T) {
	const aitoolName = "now"
	t.Cleanup(func() {
		mustSetMCPToolEnabled(t, aitoolName, true)
	})

	grpcSrv, err := NewServer()
	require.NoError(t, err)
	_, err = grpcSrv.GetMCPToolList(context.Background(), &ypb.GetMCPToolListRequest{
		Source:     schema.MCPClientToolSourceAITool,
		Pagination: &ypb.Paging{Page: 1, Limit: 50},
	})
	require.NoError(t, err)
	mustSetMCPToolEnabled(t, aitoolName, false)

	names := startMCPListToolNames(t, &ypb.StartMcpServerRequest{
		Host:                    "127.0.0.1",
		Port:                    0,
		EnableAll:               false,
		EnableAIToolFramework:   true,
		EnableBridgeExternalMCP: false,
	})
	assert.False(t, containsTool(names, aitoolName), "disabled aitool builtin must be filtered on start")
}
