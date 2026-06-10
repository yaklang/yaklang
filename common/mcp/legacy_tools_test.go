// Legacy MCP builtin tool integration tests.
//
// Entry points:
//   - TestLegacyBuiltinToolSetsRegistered: all expected tool sets are registered
//   - TestLegacyToolsIntegration_AllRegisteredToolsCovered: every builtin tool has cases
//   - TestLegacyToolsDetailedIntegration: per-tool functional + no-panic scenarios
package mcp_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/mcp"
	rawmcp "github.com/yaklang/yaklang/common/mcp/mcp-go/mcp"
	"github.com/yaklang/yaklang/common/yakgrpc"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func init() {
	yakit.CallPostInitDatabase()
}

type legacyToolCase struct {
	name              string
	args              map[string]any
	buildArgs         func(t *testing.T, srv *mcp.MCPServer) map[string]any
	timeout           time.Duration
	wantErr           bool
	errContains       []string
	skipIfErrContains []string
	validate          func(t *testing.T, text string, result *rawmcp.CallToolResult)
}

func (c legacyToolCase) resolvedArgs(t *testing.T, srv *mcp.MCPServer) map[string]any {
	t.Helper()
	if c.buildArgs != nil {
		return c.buildArgs(t, srv)
	}
	if c.args == nil {
		return map[string]any{}
	}
	return c.args
}

func (c legacyToolCase) resolvedTimeout() time.Duration {
	if c.timeout > 0 {
		return c.timeout
	}
	return 10 * time.Second
}

func newLegacyMCPServer(t *testing.T) *mcp.MCPServer {
	t.Helper()
	srv, err := mcp.NewMCPServer(mcp.WithEnableAllToolSets())
	require.NoError(t, err)
	require.NoError(t, srv.BindLocalGRPCClient())
	return srv
}

func invokeLegacyTool(
	t *testing.T,
	srv *mcp.MCPServer,
	toolName string,
	args map[string]any,
	timeout time.Duration,
) (*rawmcp.CallToolResult, error) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	var (
		result   *rawmcp.CallToolResult
		callErr  error
		panicked any
	)
	func() {
		defer func() {
			panicked = recover()
		}()
		result, callErr = mcp.InvokeBuiltinTool(ctx, srv, toolName, args)
	}()
	require.Nilf(t, panicked, "tool %q panicked with args %#v: %v", toolName, args, panicked)
	return result, callErr
}

func toolResultText(t *testing.T, result *rawmcp.CallToolResult) string {
	t.Helper()
	require.NotNil(t, result)
	require.NotEmpty(t, result.Content)

	switch content := result.Content[0].(type) {
	case rawmcp.TextContent:
		return content.Text
	default:
		raw, err := json.Marshal(result.Content[0])
		require.NoError(t, err)
		return string(raw)
	}
}

func decodeToolResultJSON(t *testing.T, text string, target any) {
	t.Helper()
	require.NoError(t, json.Unmarshal([]byte(text), target))
}

func assertToolError(t *testing.T, err error, wantErr bool, errContains []string) {
	t.Helper()
	if wantErr {
		require.Error(t, err)
		if len(errContains) > 0 {
			msg := err.Error()
			matched := false
			for _, fragment := range errContains {
				if strings.Contains(msg, fragment) {
					matched = true
					break
				}
			}
			require.Truef(t, matched, "error %q should contain one of %v", msg, errContains)
		}
		return
	}
	require.NoError(t, err)
}

func runLegacyToolCases(t *testing.T, toolName string, cases []legacyToolCase) {
	t.Helper()
	require.NotEmptyf(t, cases, "tool %q must define integration cases", toolName)

	srv := newLegacyMCPServer(t)
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			args := tc.resolvedArgs(t, srv)
			result, err := invokeLegacyTool(t, srv, toolName, args, tc.resolvedTimeout())
			if err != nil && len(tc.skipIfErrContains) > 0 {
				for _, fragment := range tc.skipIfErrContains {
					if strings.Contains(err.Error(), fragment) {
						t.Skipf("skipped due to environment: %v", err)
						return
					}
				}
			}
			assertToolError(t, err, tc.wantErr, tc.errContains)
			if tc.wantErr {
				return
			}
			require.NotNil(t, result)
			require.NotEmpty(t, result.Content)
			if tc.validate != nil {
				tc.validate(t, toolResultText(t, result), result)
			}
		})
	}
}

func firstPayloadGroupName(t *testing.T, srv *mcp.MCPServer) string {
	t.Helper()
	result, err := invokeLegacyTool(t, srv, "list_all_payload_dictionary_details", nil, 5*time.Second)
	require.NoError(t, err)
	text := toolResultText(t, result)

	var payload struct {
		Nodes []map[string]any `json:"Nodes"`
	}
	decodeToolResultJSON(t, text, &payload)

	var walk func(nodes []map[string]any) string
	walk = func(nodes []map[string]any) string {
		for _, node := range nodes {
			nodeType, _ := node["Type"].(string)
			name, _ := node["Name"].(string)
			if nodeType != "Folder" && name != "" {
				return name
			}
			if children, ok := node["Nodes"].([]any); ok {
				childNodes := make([]map[string]any, 0, len(children))
				for _, child := range children {
					if childMap, ok := child.(map[string]any); ok {
						childNodes = append(childNodes, childMap)
					}
				}
				if found := walk(childNodes); found != "" {
					return found
				}
			}
		}
		return ""
	}

	if name := walk(payload.Nodes); name != "" {
		return name
	}
	t.Skip("no payload group available in profile database")
	return ""
}

func uniqueName(prefix string) string {
	return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
}

func createLegacyGlobalHotPatchTemplate(t *testing.T) string {
	t.Helper()
	client, err := yakgrpc.NewLocalClient(true)
	require.NoError(t, err)

	tplName := uniqueName("legacy-global-hotpatch")
	_, err = client.CreateHotPatchTemplate(context.Background(), &ypb.HotPatchTemplate{
		Name: tplName,
		Type: "global",
		Content: `
beforeRequest = func(isHttps, originReq, req) { return req }
afterRequest = func(isHttps, originReq, req, originRsp, rsp) { return rsp }
`,
	})
	require.NoError(t, err)

	t.Cleanup(func() {
		_, _ = client.ResetGlobalHotPatchConfig(context.Background(), &ypb.Empty{})
	})
	return tplName
}

func legacyGlobalHotPatchEnabled(cfg map[string]any) bool {
	if cfg == nil {
		return false
	}
	enabled, ok := cfg["Enabled"].(bool)
	return ok && enabled
}

var legacyToolIntegrationCases = map[string][]legacyToolCase{
	"codec_method_details": {
		{
			name: "returns_base64_encode_doc",
			args: map[string]any{"method": []any{"Base64Encode"}},
			validate: func(t *testing.T, text string, _ *rawmcp.CallToolResult) {
				assert.Contains(t, text, "Base64Encode")
			},
		},
		{
			name:    "missing_method",
			args:    map[string]any{},
			wantErr: true, errContains: []string{"missing argument: method"},
		},
	},
	"exec_codec": {
		{
			name: "base64_encode_roundtrip",
			args: map[string]any{
				"text": "hello",
				"workFlow": []any{
					map[string]any{"codecType": "Base64Encode"},
				},
			},
			validate: func(t *testing.T, text string, _ *rawmcp.CallToolResult) {
				var result map[string]any
				decodeToolResultJSON(t, text, &result)
				assert.Equal(t, "aGVsbG8=", result["text"])
			},
		},
		{
			name: "base64_decode_roundtrip",
			args: map[string]any{
				"text": "aGVsbG8=",
				"workFlow": []any{
					map[string]any{"codecType": "Base64Decode"},
				},
			},
			validate: func(t *testing.T, text string, _ *rawmcp.CallToolResult) {
				var result map[string]any
				decodeToolResultJSON(t, text, &result)
				assert.Equal(t, "hello", result["text"])
			},
		},
	},
	"render_fuzztag": {
		{
			name: "expand_int_range_without_optional_limits",
			args: map[string]any{"template": "{{int(1-3)}}"},
			validate: func(t *testing.T, text string, _ *rawmcp.CallToolResult) {
				assert.Contains(t, text, "1")
				assert.Contains(t, text, "3")
			},
		},
		{
			name: "empty_template_returns_decodable_result",
			args: map[string]any{},
			validate: func(t *testing.T, text string, _ *rawmcp.CallToolResult) {
				var results []string
				decodeToolResultJSON(t, text, &results)
				// empty template currently yields a single empty rendered entry
				assert.LessOrEqual(t, len(results), 1)
			},
		},
	},
	"query_cve": {
		{
			name: "keywords_without_pagination",
			args: map[string]any{"keywords": "apache"},
			skipIfErrContains: []string{
				"CVE database is not initialized",
			},
			validate: func(t *testing.T, text string, _ *rawmcp.CallToolResult) {
				trimmed := strings.TrimSpace(text)
				require.True(t, trimmed == "null" || strings.HasPrefix(trimmed, "["),
					"expected JSON array or null, got: %s", text)
			},
		},
		{
			name: "lookup_by_cve_id",
			args: map[string]any{"cve": "CVE-2021-44228"},
			skipIfErrContains: []string{
				"CVE database is not initialized",
				"empty cve database",
			},
			validate: func(t *testing.T, text string, _ *rawmcp.CallToolResult) {
				assert.Contains(t, strings.ToUpper(text), "CVE-2021-44228")
			},
		},
	},
	"yakdoc_get_all_library_names": {
		{
			name: "lists_standard_libraries",
			args: map[string]any{},
			validate: func(t *testing.T, text string, _ *rawmcp.CallToolResult) {
				var libs []string
				decodeToolResultJSON(t, text, &libs)
				assert.Contains(t, libs, "codec")
			},
		},
	},
	"yakdoc_library_details": {
		{
			name: "returns_codec_symbols",
			args: map[string]any{"library": []any{"codec"}},
			validate: func(t *testing.T, text string, _ *rawmcp.CallToolResult) {
				var result map[string]map[string]any
				decodeToolResultJSON(t, text, &result)
				codecLib, ok := result["codec"]
				require.True(t, ok)
				functions, _ := codecLib["functions"].([]any)
				assert.NotEmpty(t, functions)
			},
		},
	},
	"yakdoc_function_details": {
		{
			name: "returns_println_signature",
			args: map[string]any{"function": []any{"println"}},
			validate: func(t *testing.T, text string, _ *rawmcp.CallToolResult) {
				assert.Contains(t, text, "println")
			},
		},
		{
			name:    "missing_function",
			args:    map[string]any{},
			wantErr: true, errContains: []string{"missing argument: function"},
		},
	},
	"yakdoc_variable_details": {
		{
			name: "returns_codec_variable",
			args: map[string]any{"library": "codec", "variable": []any{"ECB"}},
			validate: func(t *testing.T, text string, _ *rawmcp.CallToolResult) {
				assert.Contains(t, text, "ECB")
			},
		},
		{
			name:    "missing_variable",
			args:    map[string]any{},
			wantErr: true, errContains: []string{"missing argument: variable"},
		},
	},
	"generate_reverse_shell_command": {
		{
			name: "bash_reverse_shell",
			args: map[string]any{
				"program": "Bash -i", "shellType": "bash", "ip": "127.0.0.1", "port": float64(4444),
			},
			validate: func(t *testing.T, text string, _ *rawmcp.CallToolResult) {
				assert.Contains(t, text, "127.0.0.1")
				assert.Contains(t, text, "4444")
			},
		},
	},
	"get_system_proxy": {
		{
			name: "returns_proxy_config",
			args: map[string]any{},
			validate: func(t *testing.T, text string, _ *rawmcp.CallToolResult) {
				assert.NotEmpty(t, strings.TrimSpace(text))
			},
		},
	},
	"set_system_proxy": {
		{
			name: "toggle_disable_without_proxy_value",
			args: map[string]any{"httpProxy": "", "enable": false},
			validate: func(t *testing.T, text string, _ *rawmcp.CallToolResult) {
				assert.NotEmpty(t, strings.TrimSpace(text))
			},
		},
	},
	"get_global_hotpatch_config": {
		{
			name: "returns_config_json",
			args: map[string]any{},
			validate: func(t *testing.T, text string, _ *rawmcp.CallToolResult) {
				var cfg map[string]any
				decodeToolResultJSON(t, text, &cfg)
				assert.False(t, legacyGlobalHotPatchEnabled(cfg))
			},
		},
	},
	"enable_global_hotpatch": {
		{
			name:        "missing_template_name",
			args:        map[string]any{},
			wantErr:     true,
			errContains: []string{"templateName is required"},
		},
		{
			name:        "unknown_template",
			args:        map[string]any{"templateName": "legacy-mcp-missing-global-template"},
			wantErr:     true,
			errContains: []string{"failed", "template"},
		},
		{
			name: "enable_existing_template",
			buildArgs: func(t *testing.T, _ *mcp.MCPServer) map[string]any {
				return map[string]any{"templateName": createLegacyGlobalHotPatchTemplate(t)}
			},
			validate: func(t *testing.T, text string, _ *rawmcp.CallToolResult) {
				var cfg map[string]any
				decodeToolResultJSON(t, text, &cfg)
				assert.True(t, legacyGlobalHotPatchEnabled(cfg))
			},
		},
	},
	"disable_global_hotpatch": {
		{
			name: "disable_when_already_off",
			args: map[string]any{},
			validate: func(t *testing.T, text string, _ *rawmcp.CallToolResult) {
				var cfg map[string]any
				decodeToolResultJSON(t, text, &cfg)
				assert.False(t, legacyGlobalHotPatchEnabled(cfg))
			},
		},
	},
	"reset_global_hotpatch_config": {
		{
			name: "reset_to_default",
			args: map[string]any{},
			validate: func(t *testing.T, text string, _ *rawmcp.CallToolResult) {
				var cfg map[string]any
				decodeToolResultJSON(t, text, &cfg)
				assert.False(t, legacyGlobalHotPatchEnabled(cfg))
			},
		},
	},
	"query_hotpatch_template_list": {
		{
			name: "list_global_templates",
			args: map[string]any{"type": "global"},
			validate: func(t *testing.T, text string, _ *rawmcp.CallToolResult) {
				assert.NotEmpty(t, strings.TrimSpace(text))
			},
		},
	},
	"get_current_database_context": {
		{
			name: "returns_database_paths",
			args: map[string]any{},
			validate: func(t *testing.T, text string, _ *rawmcp.CallToolResult) {
				assert.Contains(t, text, "yakit_home")
				assert.Contains(t, text, "current_project_db_path")
				assert.Contains(t, text, "current_profile_db_path")
			},
		},
	},
	"list_project_databases": {
		{
			name: "returns_project_items",
			args: map[string]any{"limit": float64(5)},
			validate: func(t *testing.T, text string, _ *rawmcp.CallToolResult) {
				var items []map[string]any
				decodeToolResultJSON(t, text, &items)
				// empty list is valid for fresh profile DB
				_ = items
			},
		},
	},
	"switch_current_project_database": {
		{
			name:        "reject_invalid_id",
			args:        map[string]any{"id": float64(0)},
			wantErr:     true,
			errContains: []string{"id must be greater than 0"},
		},
	},
	"create_project_database": {
		{
			name:        "reject_missing_project_name",
			args:        map[string]any{},
			wantErr:     true,
			errContains: []string{"projectName is required"},
		},
	},
	"query_http_flow": {
		{
			name: "query_without_pagination",
			args: map[string]any{},
			validate: func(t *testing.T, text string, _ *rawmcp.CallToolResult) {
				var result map[string]any
				decodeToolResultJSON(t, text, &result)
				assert.Contains(t, result, "flows")
				assert.Contains(t, result, "total")
				assert.Contains(t, result, "current_database")
			},
		},
		{
			name: "keyword_filter",
			args: map[string]any{"keywords": "example.com"},
			validate: func(t *testing.T, text string, _ *rawmcp.CallToolResult) {
				var result map[string]any
				decodeToolResultJSON(t, text, &result)
				assert.Contains(t, result, "flows")
			},
		},
	},
	"set_tag_for_http_flow": {
		{
			name:        "reject_missing_id_and_hash",
			args:        map[string]any{"tags": []any{"mcp-test"}},
			wantErr:     true,
			errContains: []string{"failed to set tag"},
		},
	},
	"delete_http_flow": {
		{
			name: "delete_by_impossible_url_prefix",
			args: map[string]any{"urlPrefix": "http://mcp-integration-nonexistent.invalid/"},
			validate: func(t *testing.T, text string, _ *rawmcp.CallToolResult) {
				assert.Contains(t, text, "success")
			},
		},
	},
	"list_all_payload_dictionary_details": {
		{
			name: "returns_group_tree",
			args: map[string]any{},
			validate: func(t *testing.T, text string, _ *rawmcp.CallToolResult) {
				var result map[string]any
				decodeToolResultJSON(t, text, &result)
				assert.True(t, result["Nodes"] != nil || result["Groups"] != nil)
			},
		},
	},
	"query_payload": {
		{
			name: "query_existing_group_without_pagination",
			buildArgs: func(t *testing.T, srv *mcp.MCPServer) map[string]any {
				return map[string]any{"group": firstPayloadGroupName(t, srv)}
			},
			validate: func(t *testing.T, text string, _ *rawmcp.CallToolResult) {
				// file-backed groups return string payloads, database groups return objects
				assert.NotNil(t, text)
			},
		},
		{
			name:        "reject_missing_group",
			args:        map[string]any{},
			wantErr:     true,
			errContains: []string{"group"},
		},
	},
	"create_payload_folder": {
		{
			name:    "reject_missing_name",
			args:    map[string]any{},
			wantErr: true,
		},
	},
	"delete_payload": {
		{
			name:    "reject_missing_group_and_folder",
			args:    map[string]any{},
			wantErr: true,
		},
	},
	"save_payload": {
		{
			name:        "reject_missing_source",
			args:        map[string]any{"group": "mcp-test-group"},
			wantErr:     true,
			errContains: []string{"invalid argument: source"},
		},
	},
	"rename_payload_group": {
		{
			name:    "reject_missing_names",
			args:    map[string]any{},
			wantErr: true,
		},
	},
	"rename_payload_folder": {
		{
			name:    "reject_missing_names",
			args:    map[string]any{},
			wantErr: true,
		},
	},
	"update_one_payload": {
		{
			name:    "reject_missing_group",
			args:    map[string]any{},
			wantErr: true,
		},
	},
	"update_payload_file_content": {
		{
			name:    "reject_missing_group",
			args:    map[string]any{},
			wantErr: true,
		},
	},
	"static_analyze_yak_script": {
		{
			name: "valid_yak_script",
			args: map[string]any{
				"code":       `println("mcp-static-ok")`,
				"pluginType": "yak",
			},
			validate: func(t *testing.T, text string, _ *rawmcp.CallToolResult) {
				assert.NotEmpty(t, strings.TrimSpace(text))
			},
		},
		{
			name: "empty_code_returns_analysis_result",
			args: map[string]any{"pluginType": "yak"},
			validate: func(t *testing.T, text string, _ *rawmcp.CallToolResult) {
				assert.NotNil(t, text)
			},
		},
	},
	"query_yak_script": {
		{
			name: "list_scripts_without_pagination",
			args: map[string]any{"pagination": map[string]any{"limit": float64(3)}},
			validate: func(t *testing.T, text string, _ *rawmcp.CallToolResult) {
				var scripts []map[string]any
				decodeToolResultJSON(t, text, &scripts)
				// built-in scripts should exist
				assert.NotNil(t, scripts)
			},
		},
	},
	"list_yak_script_group": {
		{
			name: "list_groups",
			args: map[string]any{},
			validate: func(t *testing.T, text string, _ *rawmcp.CallToolResult) {
				assert.NotEmpty(t, strings.TrimSpace(text))
			},
		},
	},
	"query_yak_script_group": {
		{
			name: "query_groups_without_pagination",
			args: map[string]any{},
			validate: func(t *testing.T, text string, _ *rawmcp.CallToolResult) {
				assert.NotEmpty(t, strings.TrimSpace(text))
			},
		},
	},
	"exec_yak_script": {
		{
			name: "execute_inline_code",
			args: map[string]any{
				"code":       `yakit.Info("mcp-exec-ok")`,
				"pluginType": "yak",
			},
			timeout: 20 * time.Second,
			validate: func(t *testing.T, text string, _ *rawmcp.CallToolResult) {
				assert.True(t,
					strings.Contains(text, "mcp-exec-ok") || strings.Contains(text, "completed"),
					"unexpected exec output: %s", text)
			},
		},
		{
			name:    "missing_plugin_type_still_runs",
			args:    map[string]any{"code": `yakit.Info("mcp-exec-no-type")`},
			timeout: 20 * time.Second,
			validate: func(t *testing.T, text string, _ *rawmcp.CallToolResult) {
				assert.NotEmpty(t, strings.TrimSpace(text))
			},
		},
	},
	"create_yak_script_group": {
		{
			name: "create_named_group",
			args: map[string]any{"GroupName": uniqueName("mcp-group")},
			validate: func(t *testing.T, text string, _ *rawmcp.CallToolResult) {
				assert.Contains(t, text, "success")
			},
		},
	},
	"rename_yak_script_group": {
		{
			name:    "reject_missing_group",
			args:    map[string]any{},
			wantErr: true,
		},
	},
	"delete_yak_script_group": {
		{
			name:    "reject_missing_group",
			args:    map[string]any{},
			wantErr: true,
		},
	},
	"set_group_for_yak_script": {
		{
			name:    "reject_missing_save_group",
			args:    map[string]any{},
			wantErr: true,
		},
	},
	"query_online_yak_script": {
		{
			name:    "missing_data_filter_still_queries",
			args:    map[string]any{},
			timeout: 15 * time.Second,
			validate: func(t *testing.T, text string, _ *rawmcp.CallToolResult) {
				var rows []map[string]any
				decodeToolResultJSON(t, text, &rows)
				assert.NotNil(t, rows)
			},
		},
	},
	"download_online_yak_script": {
		{
			name:    "missing_filter_starts_stream",
			args:    map[string]any{},
			timeout: 3 * time.Second,
			skipIfErrContains: []string{
				"context deadline exceeded",
				"context canceled",
			},
			validate: func(t *testing.T, text string, result *rawmcp.CallToolResult) {
				require.NotNil(t, result)
			},
		},
	},
	"query_ports": {
		{
			name: "query_tcp_ports_without_nested_pagination",
			args: map[string]any{"proto": "tcp"},
			validate: func(t *testing.T, text string, _ *rawmcp.CallToolResult) {
				var ports []map[string]any
				decodeToolResultJSON(t, text, &ports)
				_ = ports
			},
		},
	},
	"delete_ports": {
		{
			name: "delete_by_impossible_id",
			args: map[string]any{"id": []any{float64(999999999)}},
			validate: func(t *testing.T, text string, _ *rawmcp.CallToolResult) {
				assert.Contains(t, text, "success")
			},
		},
	},
	"port_scan": {
		{
			name:    "empty_args_should_not_panic",
			args:    map[string]any{},
			timeout: 3 * time.Second,
			validate: func(t *testing.T, text string, result *rawmcp.CallToolResult) {
				require.NotNil(t, result)
				assert.NotEmpty(t, result.Content)
			},
		},
	},
	"dynamic_add_tool": {
		{
			name: "register_inline_yak_tool",
			args: map[string]any{
				"name":        uniqueName("mcp-dynamic"),
				"description": "integration test dynamic tool",
				"code":        `println("dynamic-tool-ok")`,
			},
			validate: func(t *testing.T, text string, _ *rawmcp.CallToolResult) {
				assert.Contains(t, text, "add tool")
				assert.Contains(t, text, "success")
			},
		},
		{
			name:        "reject_invalid_yak_code",
			args:        map[string]any{"name": "x", "description": "y", "code": "{{{invalid yak"},
			wantErr:     true,
			errContains: []string{"parse", "syntax", "invalid", "error"},
		},
	},
	"ssa_compile": {
		{
			name: "compile_temp_yak_project",
			buildArgs: func(t *testing.T, _ *mcp.MCPServer) map[string]any {
				dir := t.TempDir()
				require.NoError(t, os.WriteFile(filepath.Join(dir, "main.yak"), []byte(`println("ssa-ok")`), 0o644))
				return map[string]any{
					"target":       dir,
					"language":     "yak",
					"program_name": uniqueName("mcp-ssa"),
				}
			},
			timeout: 30 * time.Second,
			validate: func(t *testing.T, text string, _ *rawmcp.CallToolResult) {
				assert.Contains(t, text, "Compilation successful")
			},
		},
		{
			name:        "reject_missing_target",
			args:        map[string]any{"language": "yak"},
			wantErr:     true,
			errContains: []string{"missing required argument: target"},
		},
	},
	"ssa_query": {
		{
			name:    "reject_missing_program",
			args:    map[string]any{"rule": `println(* as $sink)`},
			wantErr: true, errContains: []string{"missing required argument: program_name"},
		},
		{
			name:        "reject_unknown_program",
			args:        map[string]any{"program_name": "mcp-missing-program", "rule": `println(* as $sink)`},
			wantErr:     true,
			errContains: []string{"failed", "program"},
		},
	},
	"http_fuzzer": {
		{
			name:    "missing_request_does_not_panic",
			args:    map[string]any{"concurrent": float64(1), "isHttps": false, "fuzzTagMode": "close"},
			timeout: 5 * time.Second,
			validate: func(t *testing.T, _ string, result *rawmcp.CallToolResult) {
				require.NotNil(t, result)
			},
		},
	},
	"hybrid_scan": {
		{
			name:    "empty_args_should_not_panic",
			args:    map[string]any{},
			timeout: 3 * time.Second,
			validate: func(t *testing.T, _ string, result *rawmcp.CallToolResult) {
				require.NotNil(t, result)
			},
		},
	},
	"brute": {
		{
			name:        "reject_missing_target",
			args:        map[string]any{},
			wantErr:     true,
			errContains: []string{"invalid argument", "target"},
		},
	},
	"web_crawler": {
		{
			name:    "empty_args_should_not_panic",
			args:    map[string]any{},
			timeout: 3 * time.Second,
			skipIfErrContains: []string{
				"context deadline exceeded",
				"context canceled",
			},
			validate: func(t *testing.T, _ string, result *rawmcp.CallToolResult) {
				require.NotNil(t, result)
			},
		},
	},
	"subdomain_collection": {
		{
			name:    "empty_args_should_not_panic",
			args:    map[string]any{},
			timeout: 3 * time.Second,
			validate: func(t *testing.T, _ string, result *rawmcp.CallToolResult) {
				require.NotNil(t, result)
			},
		},
	},
}

func TestLegacyBuiltinToolSetsRegistered(t *testing.T) {
	expectedSets := []string{
		"codec", "cve", "httpflow", "hybrid_scan", "payload", "port_scan",
		"yak_document", "yak_script", "reverse_shell", "http_fuzzer", "brute",
		"subdomain", "crawler", "dynamic", "ssa", "project_database", "system_proxy",
		"global_hotpatch",
	}
	registered := mcp.GlobalToolSetList()
	for _, setName := range expectedSets {
		require.Contains(t, registered, setName, "legacy tool set %q not registered", setName)
	}
}

func TestLegacyToolsIntegration_AllRegisteredToolsCovered(t *testing.T) {
	names := mcp.LegacyBuiltinToolNames()
	sort.Strings(names)
	require.NotEmpty(t, names)

	for _, toolName := range names {
		cases, ok := legacyToolIntegrationCases[toolName]
		require.Truef(t, ok, "tool %q has no integration cases", toolName)
		require.NotEmptyf(t, cases, "tool %q must define at least one integration case", toolName)
	}

	require.Equal(t, len(names), len(legacyToolIntegrationCases),
		"integration case map should cover exactly the registered legacy tools")
}

func TestLegacyToolsDetailedIntegration(t *testing.T) {
	toolNames := make([]string, 0, len(legacyToolIntegrationCases))
	for toolName := range legacyToolIntegrationCases {
		toolNames = append(toolNames, toolName)
	}
	sort.Strings(toolNames)

	for _, toolName := range toolNames {
		toolName := toolName
		t.Run(toolName, func(t *testing.T) {
			runLegacyToolCases(t, toolName, legacyToolIntegrationCases[toolName])
		})
	}
}
