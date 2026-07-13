package mcp

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/mcp/mcp-go/mcp"
)

func TestMCPToolSetCatalogMatchesRegistration(t *testing.T) {
	registered := GlobalToolSetList()
	catalogNames := AllMCPToolSetNames()
	require.ElementsMatch(t, registered, catalogNames, "catalog must list every registered tool set exactly once")

	for _, entry := range MCPToolSetCatalog() {
		require.Contains(t, globalToolSets, entry.Name)
		require.NotEmpty(t, entry.Summary)
		require.NotEmpty(t, ToolNamesInSet(entry.Name), "tool set %q must expose tools", entry.Name)
	}
}

func TestDefaultMCPToolSets_Classification(t *testing.T) {
	require.Len(t, DefaultMCPToolSets, 12)
	require.Len(t, OptionalMCPToolSets, 11)
	require.Len(t, InternalMCPToolSets, 1)

	for _, name := range DefaultMCPToolSets {
		require.True(t, IsDefaultMCPToolSet(name), "%q should be default tier", name)
		tier, ok := MCPToolSetTierOf(name)
		require.True(t, ok)
		require.Equal(t, ToolSetTierDefault, tier)
	}

	for _, name := range OptionalMCPToolSets {
		require.False(t, IsDefaultMCPToolSet(name), "%q should not be default", name)
	}

	require.Contains(t, DefaultMCPToolSets, "reverse_platform")
	require.Contains(t, DefaultMCPToolSets, "risk")
	require.Contains(t, DefaultMCPToolSets, "syntaxflow")
	require.Contains(t, DefaultMCPToolSets, "mitm")

	require.NotContains(t, DefaultMCPToolSets, "hybrid_scan")
	require.NotContains(t, DefaultMCPToolSets, "payload")
	require.NotContains(t, DefaultMCPToolSets, "yak_script")
	require.NotContains(t, DefaultMCPToolSets, "dynamic")
}

func TestDefaultMCPToolCount_IsReasonable(t *testing.T) {
	count := DefaultMCPToolCount()
	require.GreaterOrEqual(t, count, 40, "default should cover core workflows")
	require.LessOrEqual(t, count, 55, "default should stay lean vs full %d tools", len(globalTools))
}

func TestBuiltinToolSetOf_DefaultTier(t *testing.T) {
	setName, ok := BuiltinToolSetOf("require_dnslog_domain")
	require.True(t, ok)
	require.Equal(t, "reverse_platform", setName)
	require.True(t, IsDefaultBuiltinTool("require_dnslog_domain"))

	setName, ok = BuiltinToolSetOf("save_payload")
	require.True(t, ok)
	require.Equal(t, "payload", setName)
	require.False(t, IsDefaultBuiltinTool("save_payload"))
}

func TestDefaultMCPToolSets_CoversExpectedTools(t *testing.T) {
	opts, err := legacyMCPToolSetOptions(nil, nil, false)
	require.NoError(t, err)

	srv, err := NewMCPServer(opts...)
	require.NoError(t, err)

	names := listToolNamesFromMCPServer(t, srv)

	for _, setName := range DefaultMCPToolSets {
		for _, toolName := range ToolNamesInSet(setName) {
			require.Contains(t, names, toolName, "default startup should expose %q from set %q", toolName, setName)
		}
	}

	for _, optionalSet := range OptionalMCPToolSets {
		for _, toolName := range ToolNamesInSet(optionalSet) {
			require.NotContains(t, names, toolName, "optional set %q tool %q should stay hidden", optionalSet, toolName)
		}
	}
	for _, toolName := range ToolNamesInSet("dynamic") {
		require.NotContains(t, names, toolName, "internal set tool %q should stay hidden", toolName)
	}
}

func listToolNamesFromMCPServer(t *testing.T, srv *MCPServer) map[string]struct{} {
	t.Helper()

	message := []byte(`{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`)
	response := srv.server.HandleMessage(context.Background(), message)

	resp, ok := response.(mcp.JSONRPCResponse)
	require.True(t, ok)

	listResult, ok := resp.Result.(mcp.ListToolsResult)
	require.True(t, ok)

	names := make(map[string]struct{}, len(listResult.Tools))
	for _, tool := range listResult.Tools {
		names[tool.Name] = struct{}{}
	}
	return names
}
