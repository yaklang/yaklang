package mcp

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/mcp/mcp-go/mcp"
)

func listRegisteredToolNames(t *testing.T, opts ...McpServerOption) map[string]struct{} {
	t.Helper()

	srv, err := NewMCPServer(opts...)
	require.NoError(t, err)

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

func TestLegacyMCPToolSetOptions_DefaultEnableAll(t *testing.T) {
	opts, err := legacyMCPToolSetOptions(nil, nil)
	require.NoError(t, err)
	require.NotEmpty(t, opts)

	names := listRegisteredToolNames(t, opts...)
	require.NotEmpty(t, names)
	require.Contains(t, names, "exec_codec")
	require.Contains(t, names, "query_cve")
}

func TestLegacyMCPToolSetOptions_DisableSubtractsFromDefaultAll(t *testing.T) {
	disabled := []string{
		"cve", "hybrid_scan", "payload", "port_scan", "yak_document", "yak_script",
		"reverse_shell", "brute", "subdomain", "crawler", "dynamic", "ssa", "system_proxy",
	}
	opts, err := legacyMCPToolSetOptions(nil, disabled)
	require.NoError(t, err)

	names := listRegisteredToolNames(t, opts...)
	require.NotEmpty(t, names, "CLI with only --disable-tool must still expose remaining legacy tools")

	require.Contains(t, names, "exec_codec")
	require.NotContains(t, names, "query_cve")
	require.NotContains(t, names, "start_port_scan")
	require.NotContains(t, names, "exec_yak_script")
}

func TestLegacyMCPToolSetOptions_ExplicitToolNarrowsSet(t *testing.T) {
	opts, err := legacyMCPToolSetOptions([]string{"codec"}, nil)
	require.NoError(t, err)

	names := listRegisteredToolNames(t, opts...)
	require.Contains(t, names, "exec_codec")
	require.NotContains(t, names, "query_cve")
	require.NotContains(t, names, "start_port_scan")
}

func TestLegacyMCPToolSetOptions_ExplicitToolWithDisable(t *testing.T) {
	opts, err := legacyMCPToolSetOptions([]string{"codec", "cve"}, []string{"cve"})
	require.NoError(t, err)

	names := listRegisteredToolNames(t, opts...)
	require.Contains(t, names, "exec_codec")
	require.NotContains(t, names, "query_cve")
}

func TestLegacyMCPToolSetOptions_InvalidDisableSetFailsAtNewServer(t *testing.T) {
	opts, err := legacyMCPToolSetOptions(nil, []string{"not_a_real_tool_set"})
	require.NoError(t, err)
	_, err = NewMCPServer(opts...)
	require.Error(t, err)
}

func TestMCPServerOptInWithoutEnableAllExposesNoLegacyTools(t *testing.T) {
	// gRPC tier path: disable-only without enable must not accidentally expose legacy tools.
	srv, err := NewMCPServer(WithDisableToolSet("cve"))
	require.NoError(t, err)

	message := []byte(`{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`)
	response := srv.server.HandleMessage(context.Background(), message)

	errResp, ok := response.(mcp.JSONRPCError)
	require.True(t, ok)
	require.Equal(t, mcp.METHOD_NOT_FOUND, errResp.Error.Code)
	require.Contains(t, errResp.Error.Message, "Tools not supported")
}

func TestMCPServerCLIStyleDefaultEnableAllWithDisable_JSONRoundTrip(t *testing.T) {
	opts, err := legacyMCPToolSetOptions(nil, []string{"cve", "port_scan"})
	require.NoError(t, err)

	srv, err := NewMCPServer(opts...)
	require.NoError(t, err)

	message := []byte(`{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`)
	response := srv.server.HandleMessage(context.Background(), message)

	raw, err := json.Marshal(response)
	require.NoError(t, err)
	require.True(t, json.Valid(raw))
}
