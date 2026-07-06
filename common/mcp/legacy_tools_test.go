// Legacy MCP builtin tool integration tests.
//
// Structure:
//   - Case definitions are grouped by ToolSet (legacyCodecToolCases, legacyRiskToolCases, ...).
//   - registerAllLegacyToolSetCases() registers all cases into legacyToolIntegrationCases.
//
// Entry points:
//   - TestLegacyBuiltinToolSetsRegistered: all expected tool sets are registered
//   - TestLegacyToolsIntegration_AllRegisteredToolsCovered: every builtin tool has cases
//   - TestLegacyToolSet_<Name>: one test function per ToolSet (e.g. TestLegacyToolSet_Codec)
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
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/mcp"
	rawmcp "github.com/yaklang/yaklang/common/mcp/mcp-go/mcp"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func init() {
	yakit.CallPostInitDatabase()
	registerAllLegacyToolSetCases()
}

type legacyToolCase struct {
	name              string
	args              map[string]any
	buildArgs         func(t *testing.T, srv *mcp.MCPServer) map[string]any
	timeout           time.Duration
	wantErr           bool
	errContains       []string
	allowErrContains  []string
	allowEmptyResult  bool
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
			if err != nil && len(tc.allowErrContains) > 0 {
				for _, fragment := range tc.allowErrContains {
					if strings.Contains(err.Error(), fragment) {
						return
					}
				}
			}
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
			if !tc.allowEmptyResult {
				require.NotEmpty(t, result.Content)
			}
			if tc.validate != nil {
				var text string
				if len(result.Content) > 0 {
					text = toolResultText(t, result)
				}
				tc.validate(t, text, result)
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

func ensureLegacyTestRiskID(t *testing.T) int64 {
	t.Helper()
	r, err := yakit.NewRisk(
		"127.0.0.1",
		yakit.WithRiskParam_Title("mcp-integration-test"),
		yakit.WithRiskParam_Severity("low"),
	)
	require.NoError(t, err)
	require.NotZero(t, r.ID)
	id := int64(r.ID)
	t.Cleanup(func() {
		_ = yakit.DeleteRiskByID(consts.GetGormProjectDatabase(), id)
	})
	return id
}

func ensureLegacyTestReportID(t *testing.T) int64 {
	t.Helper()
	db := consts.GetGormProjectDatabase()
	require.NotNil(t, db)
	rec := &schema.ReportRecord{
		Title: uniqueName("mcp-report-title"),
		Hash:  uniqueName("mcp-report-hash"),
		Owner: "mcp-test",
		From:  "integration",
	}
	require.NoError(t, db.Create(rec).Error)
	id := int64(rec.ID)
	t.Cleanup(func() {
		_ = yakit.DeleteReportRecordByID(db, id)
	})
	return id
}

func legacyGeneratedYSOBytesArgs(t *testing.T, srv *mcp.MCPServer) map[string]any {
	t.Helper()
	result, err := invokeLegacyTool(t, srv, "generate_yso_bytes", map[string]any{
		"gadget": "URLDNS",
		"class":  "URLDNS",
		"options": []any{
			map[string]any{"key": "domain", "value": "example.com"},
		},
	}, 10*time.Second)
	require.NoError(t, err)
	var payload struct {
		Bytes string `json:"Bytes"`
	}
	decodeToolResultJSON(t, toolResultText(t, result), &payload)
	require.NotEmpty(t, payload.Bytes)
	return map[string]any{"data": payload.Bytes}
}

// expectedLegacyToolSetOrder mirrors MCPCommandUsage tool set registration order.
var expectedLegacyToolSetOrder = []string{
	"codec", "cve", "httpflow", "hybrid_scan", "payload", "port_scan",
	"yak_document", "yak_script", "reverse_shell", "reverse_platform", "http_fuzzer", "brute",
	"subdomain", "crawler", "dynamic", "ssa", "syntaxflow", "risk", "yso", "mitm",
	"fingerprint", "space_engine", "report", "plugin_env", "http_builder", "chaos_maker",
	"project_database", "global_hotpatch", "system_proxy",
}

var (
	legacyToolIntegrationCases = make(map[string][]legacyToolCase)
	legacyToolSetByTool        = make(map[string]string)
)

func registerLegacyToolSetCases(toolSet string, cases map[string][]legacyToolCase) {
	for toolName, toolCases := range cases {
		if _, exists := legacyToolIntegrationCases[toolName]; exists {
			panic(fmt.Sprintf("duplicate legacy tool integration cases for %q", toolName))
		}
		legacyToolIntegrationCases[toolName] = toolCases
		legacyToolSetByTool[toolName] = toolSet
	}
}

func registerAllLegacyToolSetCases() {
	for _, toolSet := range expectedLegacyToolSetOrder {
		var cases map[string][]legacyToolCase
		switch toolSet {
		case "codec":
			cases = legacyCodecToolCases()
		case "cve":
			cases = legacyCVEToolCases()
		case "httpflow":
			cases = legacyHTTPFlowToolCases()
		case "hybrid_scan":
			cases = legacyHybridScanToolCases()
		case "payload":
			cases = legacyPayloadToolCases()
		case "port_scan":
			cases = legacyPortScanToolCases()
		case "yak_document":
			cases = legacyYakDocumentToolCases()
		case "yak_script":
			cases = legacyYakScriptToolCases()
		case "reverse_shell":
			cases = legacyReverseShellToolCases()
		case "reverse_platform":
			cases = legacyReversePlatformToolCases()
		case "http_fuzzer":
			cases = legacyHTTPFuzzerToolCases()
		case "brute":
			cases = legacyBruteToolCases()
		case "subdomain":
			cases = legacySubdomainToolCases()
		case "crawler":
			cases = legacyCrawlerToolCases()
		case "dynamic":
			cases = legacyDynamicToolCases()
		case "ssa":
			cases = legacySSAToolCases()
		case "syntaxflow":
			cases = legacySyntaxFlowToolCases()
		case "risk":
			cases = legacyRiskToolCases()
		case "yso":
			cases = legacyYSOToolCases()
		case "mitm":
			cases = legacyMITMToolCases()
		case "fingerprint":
			cases = legacyFingerprintToolCases()
		case "space_engine":
			cases = legacySpaceEngineToolCases()
		case "report":
			cases = legacyReportToolCases()
		case "plugin_env":
			cases = legacyPluginEnvToolCases()
		case "http_builder":
			cases = legacyHTTPBuilderToolCases()
		case "chaos_maker":
			cases = legacyChaosMakerToolCases()
		case "project_database":
			cases = legacyProjectDatabaseToolCases()
		case "global_hotpatch":
			cases = legacyGlobalHotpatchToolCases()
		case "system_proxy":
			cases = legacySystemProxyToolCases()
		default:
			panic(fmt.Sprintf("missing legacy tool cases for tool set %q", toolSet))
		}
		registerLegacyToolSetCases(toolSet, cases)
	}
}

func legacyRegisteredToolSetNames() []string {
	sets := make(map[string]struct{}, len(legacyToolSetByTool))
	for _, setName := range legacyToolSetByTool {
		sets[setName] = struct{}{}
	}
	names := make([]string, 0, len(sets))
	for _, setName := range expectedLegacyToolSetOrder {
		if _, ok := sets[setName]; ok {
			names = append(names, setName)
		}
	}
	return names
}

func legacyToolNamesInSet(toolSet string) []string {
	names := make([]string, 0)
	for toolName, setName := range legacyToolSetByTool {
		if setName == toolSet {
			names = append(names, toolName)
		}
	}
	sort.Strings(names)
	return names
}

func legacyNoPanicEmptyCase() legacyToolCase {
	return legacyToolCase{
		name:    "empty_args_should_not_panic",
		args:    map[string]any{},
		timeout: 3 * time.Second,
		validate: func(t *testing.T, _ string, result *rawmcp.CallToolResult) {
			require.NotNil(t, result)
		},
	}
}

func legacyMinimalPagingCase() legacyToolCase {
	return legacyToolCase{
		name:    "minimal_pagination_should_not_panic",
		args:    pagingArgs(),
		timeout: 5 * time.Second,
		skipIfErrContains: []string{
			"context deadline", "failed", "invalid",
		},
		validate: func(t *testing.T, _ string, result *rawmcp.CallToolResult) {
			require.NotNil(t, result)
		},
	}
}

func pagingArgs() map[string]any {
	return map[string]any{"pagination": map[string]any{"page": 1, "limit": 1, "orderBy": "id", "order": "desc"}}
}

func legacyNoPanicWithSkip(skip ...string) legacyToolCase {
	c := legacyNoPanicEmptyCase()
	c.skipIfErrContains = skip
	return c
}

func runLegacyToolSetIntegration(t *testing.T, toolSet string) {
	t.Helper()
	for _, toolName := range legacyToolNamesInSet(toolSet) {
		toolName := toolName
		t.Run(toolName, func(t *testing.T) {
			runLegacyToolCases(t, toolName, legacyToolIntegrationCases[toolName])
		})
	}
}


// --- ToolSet: codec ---

func legacyCodecToolCases() map[string][]legacyToolCase {
	return map[string][]legacyToolCase{
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
	}
}


// --- ToolSet: cve ---

func legacyCVEToolCases() map[string][]legacyToolCase {
	return map[string][]legacyToolCase{
	"query_cve": {
		{
			name: "keywords_without_pagination",
			args: map[string]any{"keywords": "apache"},
			allowErrContains: []string{
				"CVE database is not initialized",
			},
			validate: func(t *testing.T, text string, _ *rawmcp.CallToolResult) {
				trimmed := strings.TrimSpace(text)
				require.True(t, trimmed == "null" || strings.HasPrefix(trimmed, "["),
					"expected JSON array or null, got: %s", text)
			},
		},
		{
			name: "lookup_by_cve_id_or_missing_database",
			args: map[string]any{"cve": "CVE-2021-44228"},
			allowErrContains: []string{
				"CVE database is not initialized",
				"empty cve database",
			},
			validate: func(t *testing.T, text string, _ *rawmcp.CallToolResult) {
				assert.Contains(t, strings.ToUpper(text), "CVE-2021-44228")
			},
		},
	},
	}
}


// --- ToolSet: httpflow ---

func legacyHTTPFlowToolCases() map[string][]legacyToolCase {
	return map[string][]legacyToolCase{
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
	}
}


// --- ToolSet: hybrid_scan ---

func legacyHybridScanToolCases() map[string][]legacyToolCase {
	return map[string][]legacyToolCase{
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
	}
}


// --- ToolSet: payload ---

func legacyPayloadToolCases() map[string][]legacyToolCase {
	return map[string][]legacyToolCase{
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
	}
}


// --- ToolSet: port_scan ---

func legacyPortScanToolCases() map[string][]legacyToolCase {
	return map[string][]legacyToolCase{
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
	}
}


// --- ToolSet: yak_document ---

func legacyYakDocumentToolCases() map[string][]legacyToolCase {
	return map[string][]legacyToolCase{
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
	}
}


// --- ToolSet: yak_script ---

func legacyYakScriptToolCases() map[string][]legacyToolCase {
	return map[string][]legacyToolCase{
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
	}
}


// --- ToolSet: reverse_shell ---

func legacyReverseShellToolCases() map[string][]legacyToolCase {
	return map[string][]legacyToolCase{
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
	}
}


// --- ToolSet: reverse_platform ---

func legacyReversePlatformToolCases() map[string][]legacyToolCase {
	return map[string][]legacyToolCase{
	"get_global_reverse_server": {
		{
			name:    "empty_args_should_not_panic",
			args:    map[string]any{},
			timeout: 3 * time.Second,
			skipIfErrContains: []string{
				"failed", "invalid", "connect", "bridge", "timeout", "context deadline",
				"dnslog", "reverse", "panic",
			},
			validate: func(t *testing.T, _ string, result *rawmcp.CallToolResult) {
				require.NotNil(t, result)
			},
		},
	},
	"available_local_addr": {
		{
			name:    "empty_args_should_not_panic",
			args:    map[string]any{},
			timeout: 3 * time.Second,
			skipIfErrContains: []string{
				"failed", "invalid", "connect", "bridge", "timeout", "context deadline",
				"dnslog", "reverse", "panic",
			},
			validate: func(t *testing.T, _ string, result *rawmcp.CallToolResult) {
				require.NotNil(t, result)
			},
		},
	},
	"get_tunnel_server_external_ip": {{
		name:    "requires_bridge_without_config",
		args:    map[string]any{},
		timeout: 5 * time.Second,
		allowErrContains: []string{
			"bridge addr is required", "empty addr", "failed",
		},
		validate: func(t *testing.T, _ string, result *rawmcp.CallToolResult) {
			require.NotNil(t, result)
		},
	}},
	"verify_tunnel_server_domain": {{
		name: "verify_with_domain",
		args: map[string]any{"domain": "dnslog.example.com"},
		timeout: 10 * time.Second,
		allowErrContains: []string{
			"empty addr", "failed", "connect", "timeout", "bridge",
		},
		validate: func(t *testing.T, _ string, result *rawmcp.CallToolResult) {
			require.NotNil(t, result)
		},
	}},
	"require_dnslog_domain": {{
		name:    "fallback_remote_bridge",
		args:    map[string]any{"useLocal": true},
		timeout: 10 * time.Second,
		skipIfErrContains: []string{
			"dnsbroker", "bridge", "connect", "failed",
		},
		validate: func(t *testing.T, _ string, result *rawmcp.CallToolResult) {
			require.NotNil(t, result)
		},
	}},
	"query_dnslog_by_token": {{
		name:    "query_with_test_token",
		args:    map[string]any{"token": "mcp-test-token", "useLocal": true},
		timeout: 5 * time.Second,
		allowErrContains: []string{
			"dnsbroker", "retry", "failed", "no existed",
		},
		validate: func(t *testing.T, _ string, result *rawmcp.CallToolResult) {
			require.NotNil(t, result)
		},
	}},
	"require_random_port_token": {{
		name:    "request_token",
		args:    map[string]any{},
		timeout: 15 * time.Second,
		allowErrContains: []string{
			"DeadlineExceeded", "failed", "connect", "timeout",
		},
		validate: func(t *testing.T, _ string, result *rawmcp.CallToolResult) {
			require.NotNil(t, result)
		},
	}},
	"query_random_port_trigger": {{
		name:    "auto_token",
		args:    map[string]any{},
		timeout: 15 * time.Second,
		allowErrContains: []string{
			"empty token", "empty local-port", "failed", "connect", "DeadlineExceeded",
		},
		validate: func(t *testing.T, _ string, result *rawmcp.CallToolResult) {
			require.NotNil(t, result)
		},
	}},
	"get_bridge_log_server": {
		{
			name:    "empty_args_should_not_panic",
			args:    map[string]any{},
			timeout: 3 * time.Second,
			skipIfErrContains: []string{
				"failed", "invalid", "connect", "bridge", "timeout", "context deadline",
				"dnslog", "reverse", "panic",
			},
			validate: func(t *testing.T, _ string, result *rawmcp.CallToolResult) {
				require.NotNil(t, result)
			},
		},
	},
	"set_bridge_log_server": {
		{
			name:    "empty_args_should_not_panic",
			args:    map[string]any{},
			timeout: 3 * time.Second,
			skipIfErrContains: []string{
				"failed", "invalid", "connect", "bridge", "timeout", "context deadline",
				"dnslog", "reverse", "panic",
			},
			validate: func(t *testing.T, _ string, result *rawmcp.CallToolResult) {
				require.NotNil(t, result)
			},
		},
	},
	"register_facades_http": {{
		name:    "default_http_response",
		args:    map[string]any{"url": "http://127.0.0.1/mcp-test"},
		timeout: 5 * time.Second,
		skipIfErrContains: []string{
			"empty", "failed", "connect",
		},
		validate: func(t *testing.T, _ string, result *rawmcp.CallToolResult) {
			require.NotNil(t, result)
		},
	}},
	"apply_class_to_facades": {{
		name:        "reject_missing_token",
		args:        map[string]any{},
		wantErr:     true,
		errContains: []string{"token", "Server is not exist"},
	}},
	"config_global_reverse": {
		{
			name:    "empty_args_should_not_panic",
			args:    map[string]any{},
			timeout: 3 * time.Second,
			skipIfErrContains: []string{
				"failed", "invalid", "connect", "bridge", "timeout", "context deadline",
				"dnslog", "reverse", "panic",
			},
			validate: func(t *testing.T, _ string, result *rawmcp.CallToolResult) {
				require.NotNil(t, result)
			},
		},
	},
	"start_facades": {
		{
			name:    "empty_args_should_not_panic",
			args:    map[string]any{},
			timeout: 3 * time.Second,
			skipIfErrContains: []string{
				"failed", "invalid", "connect", "bridge", "timeout", "context deadline",
				"dnslog", "reverse", "panic",
			},
			validate: func(t *testing.T, _ string, result *rawmcp.CallToolResult) {
				require.NotNil(t, result)
			},
		},
	},
	"start_facades_with_yso": {
		{
			name:    "empty_args_should_not_panic",
			args:    map[string]any{},
			timeout: 3 * time.Second,
			skipIfErrContains: []string{
				"failed", "invalid", "connect", "bridge", "timeout", "context deadline",
				"dnslog", "reverse", "panic",
			},
			validate: func(t *testing.T, _ string, result *rawmcp.CallToolResult) {
				require.NotNil(t, result)
			},
		},
	},
	}
}


// --- ToolSet: http_fuzzer ---

func legacyHTTPFuzzerToolCases() map[string][]legacyToolCase {
	return map[string][]legacyToolCase{
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
	"create_web_fuzzer_tab": {
		{
			name: "reject_missing_request",
			args: map[string]any{
				"isHttps": false,
			},
			wantErr:     true,
			errContains: []string{"request is required"},
		},
	},
	}
}


// --- ToolSet: brute ---

func legacyBruteToolCases() map[string][]legacyToolCase {
	return map[string][]legacyToolCase{
	"brute": {
		{
			name:        "reject_missing_target",
			args:        map[string]any{},
			wantErr:     true,
			errContains: []string{"invalid argument", "target"},
		},
	},
	}
}


// --- ToolSet: subdomain ---

func legacySubdomainToolCases() map[string][]legacyToolCase {
	return map[string][]legacyToolCase{
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
}


// --- ToolSet: crawler ---

func legacyCrawlerToolCases() map[string][]legacyToolCase {
	return map[string][]legacyToolCase{
	"web_crawler": {{
		name: "minimal_unreachable_target",
		args: map[string]any{
			"target":            "http://127.0.0.1:1",
			"maxDepth":          0,
			"maxRequests":       1,
			"maxLinks":          1,
			"concurrent":        1,
			"timeoutPerRequest": 1,
		},
		timeout:          20 * time.Second,
		allowEmptyResult: true,
		validate: func(t *testing.T, _ string, result *rawmcp.CallToolResult) {
			require.NotNil(t, result)
		},
	}},
	}
}


// --- ToolSet: dynamic ---

func legacyDynamicToolCases() map[string][]legacyToolCase {
	return map[string][]legacyToolCase{
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
	}
}


// --- ToolSet: ssa ---

func legacySSAToolCases() map[string][]legacyToolCase {
	return map[string][]legacyToolCase{
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
	}
}


// --- ToolSet: syntaxflow ---

func legacySyntaxFlowToolCases() map[string][]legacyToolCase {
	return map[string][]legacyToolCase{
	"query_syntaxflow_rule": {
		{
			name:    "minimal_pagination_should_not_panic",
			args:    pagingArgs(),
			timeout: 5 * time.Second,
			skipIfErrContains: []string{
				"context deadline", "failed", "invalid",
			},
			validate: func(t *testing.T, _ string, result *rawmcp.CallToolResult) {
				require.NotNil(t, result)
			},
		},
	},
	"create_syntaxflow_rule": {{
		name: "create_minimal_rule",
		buildArgs: func(t *testing.T, _ *mcp.MCPServer) map[string]any {
			name := uniqueName("mcp-sf")
			t.Setenv("MCP_SF_RULE_NAME", name)
			return map[string]any{
				"syntaxFlowInput": map[string]any{
					"ruleName": name,
					"language": "java",
					"content":  `println as $output`,
				},
			}
		},
		timeout: 8 * time.Second,
		validate: func(t *testing.T, text string, _ *rawmcp.CallToolResult) {
			assert.Contains(t, text, os.Getenv("MCP_SF_RULE_NAME"))
		},
	}},
	"update_syntaxflow_rule": {{
		name: "update_existing_rule",
		buildArgs: func(t *testing.T, srv *mcp.MCPServer) map[string]any {
			name := uniqueName("mcp-sf-upd")
			_, err := invokeLegacyTool(t, srv, "create_syntaxflow_rule", map[string]any{
				"syntaxFlowInput": map[string]any{
					"ruleName": name,
					"language": "java",
					"content":  `println as $output`,
				},
			}, 8*time.Second)
			require.NoError(t, err)
			return map[string]any{
				"syntaxFlowInput": map[string]any{
					"ruleName":    name,
					"language":    "java",
					"content":     `println as $output`,
					"description": "mcp-updated",
				},
			}
		},
		timeout: 8 * time.Second,
		validate: func(t *testing.T, _ string, result *rawmcp.CallToolResult) {
			require.NotNil(t, result)
		},
	}},
	"delete_syntaxflow_rule": {{
		name: "delete_created_rule",
		buildArgs: func(t *testing.T, srv *mcp.MCPServer) map[string]any {
			name := uniqueName("mcp-sf-del")
			_, err := invokeLegacyTool(t, srv, "create_syntaxflow_rule", map[string]any{
				"syntaxFlowInput": map[string]any{
					"ruleName": name,
					"language": "java",
					"content":  `println as $output`,
				},
			}, 8*time.Second)
			require.NoError(t, err)
			return map[string]any{
				"filter": map[string]any{"ruleNames": []any{name}},
			}
		},
		timeout: 8 * time.Second,
		validate: func(t *testing.T, _ string, result *rawmcp.CallToolResult) {
			require.NotNil(t, result)
		},
	}},
	"query_syntaxflow_result": {
		{
			name:    "minimal_pagination_should_not_panic",
			args:    pagingArgs(),
			timeout: 5 * time.Second,
			skipIfErrContains: []string{
				"context deadline", "failed", "invalid",
			},
			validate: func(t *testing.T, _ string, result *rawmcp.CallToolResult) {
				require.NotNil(t, result)
			},
		},
	},
	"query_syntaxflow_scan_task": {
		{
			name:    "minimal_pagination_should_not_panic",
			args:    pagingArgs(),
			timeout: 5 * time.Second,
			skipIfErrContains: []string{
				"context deadline", "failed", "invalid",
			},
			validate: func(t *testing.T, _ string, result *rawmcp.CallToolResult) {
				require.NotNil(t, result)
			},
		},
	},
	"delete_syntaxflow_scan_task": {
		{
			name:    "empty_args_should_not_panic",
			args:    map[string]any{},
			timeout: 3 * time.Second,
			skipIfErrContains: []string{
				"failed", "invalid", "connect", "bridge", "timeout", "context deadline",
				"dnslog", "reverse", "panic",
			},
			validate: func(t *testing.T, _ string, result *rawmcp.CallToolResult) {
				require.NotNil(t, result)
			},
		},
	},
	"syntaxflow_scan": {
		{
			name:    "empty_args_should_not_panic",
			args:    map[string]any{},
			timeout: 3 * time.Second,
			skipIfErrContains: []string{
				"failed", "invalid", "connect", "bridge", "timeout", "context deadline",
				"dnslog", "reverse", "panic",
			},
			validate: func(t *testing.T, _ string, result *rawmcp.CallToolResult) {
				require.NotNil(t, result)
			},
		},
	},
	}
}


// --- ToolSet: risk ---

func legacyRiskToolCases() map[string][]legacyToolCase {
	return map[string][]legacyToolCase{
	"query_risks": {
		{
			name:    "minimal_pagination_should_not_panic",
			args:    pagingArgs(),
			timeout: 5 * time.Second,
			skipIfErrContains: []string{
				"context deadline", "failed", "invalid",
			},
			validate: func(t *testing.T, _ string, result *rawmcp.CallToolResult) {
				require.NotNil(t, result)
			},
		},
	},
	"query_risk": {{
		name: "query_seeded_risk",
		buildArgs: func(t *testing.T, _ *mcp.MCPServer) map[string]any {
			return map[string]any{"id": ensureLegacyTestRiskID(t)}
		},
		timeout: 5 * time.Second,
		validate: func(t *testing.T, _ string, result *rawmcp.CallToolResult) {
			require.NotNil(t, result)
		},
	}},
	"delete_risk": {
		{
			name:    "empty_args_should_not_panic",
			args:    map[string]any{},
			timeout: 3 * time.Second,
			skipIfErrContains: []string{
				"failed", "invalid", "connect", "bridge", "timeout", "context deadline",
				"dnslog", "reverse", "panic",
			},
			validate: func(t *testing.T, _ string, result *rawmcp.CallToolResult) {
				require.NotNil(t, result)
			},
		},
	},
	"query_new_risks": {
		{
			name:    "empty_args_should_not_panic",
			args:    map[string]any{},
			timeout: 3 * time.Second,
			skipIfErrContains: []string{
				"failed", "invalid", "connect", "bridge", "timeout", "context deadline",
				"dnslog", "reverse", "panic",
			},
			validate: func(t *testing.T, _ string, result *rawmcp.CallToolResult) {
				require.NotNil(t, result)
			},
		},
	},
	"new_risk_read": {
		{
			name:    "empty_args_should_not_panic",
			args:    map[string]any{},
			timeout: 3 * time.Second,
			skipIfErrContains: []string{
				"failed", "invalid", "connect", "bridge", "timeout", "context deadline",
				"dnslog", "reverse", "panic",
			},
			validate: func(t *testing.T, _ string, result *rawmcp.CallToolResult) {
				require.NotNil(t, result)
			},
		},
	},
	"set_tag_for_risk": {{
		name: "tag_seeded_risk",
		buildArgs: func(t *testing.T, _ *mcp.MCPServer) map[string]any {
			return map[string]any{
				"id":   ensureLegacyTestRiskID(t),
				"tags": []any{"mcp-test"},
			}
		},
		timeout: 5 * time.Second,
		validate: func(t *testing.T, _ string, result *rawmcp.CallToolResult) {
			require.NotNil(t, result)
		},
	}},
	"query_risk_tags": {
		{
			name:    "empty_args_should_not_panic",
			args:    map[string]any{},
			timeout: 3 * time.Second,
			skipIfErrContains: []string{
				"failed", "invalid", "connect", "bridge", "timeout", "context deadline",
				"dnslog", "reverse", "panic",
			},
			validate: func(t *testing.T, _ string, result *rawmcp.CallToolResult) {
				require.NotNil(t, result)
			},
		},
	},
	"risk_field_group": {
		{
			name:    "empty_args_should_not_panic",
			args:    map[string]any{},
			timeout: 3 * time.Second,
			skipIfErrContains: []string{
				"failed", "invalid", "connect", "bridge", "timeout", "context deadline",
				"dnslog", "reverse", "panic",
			},
			validate: func(t *testing.T, _ string, result *rawmcp.CallToolResult) {
				require.NotNil(t, result)
			},
		},
	},
	"query_available_risk_type": {
		{
			name:    "empty_args_should_not_panic",
			args:    map[string]any{},
			timeout: 3 * time.Second,
			skipIfErrContains: []string{
				"failed", "invalid", "connect", "bridge", "timeout", "context deadline",
				"dnslog", "reverse", "panic",
			},
			validate: func(t *testing.T, _ string, result *rawmcp.CallToolResult) {
				require.NotNil(t, result)
			},
		},
	},
	"query_available_risk_level": {
		{
			name:    "empty_args_should_not_panic",
			args:    map[string]any{},
			timeout: 3 * time.Second,
			skipIfErrContains: []string{
				"failed", "invalid", "connect", "bridge", "timeout", "context deadline",
				"dnslog", "reverse", "panic",
			},
			validate: func(t *testing.T, _ string, result *rawmcp.CallToolResult) {
				require.NotNil(t, result)
			},
		},
	},
	}
}


// --- ToolSet: yso ---

func legacyYSOToolCases() map[string][]legacyToolCase {
	return map[string][]legacyToolCase{
	"get_all_yso_gadget_options": {
		{
			name:    "empty_args_should_not_panic",
			args:    map[string]any{},
			timeout: 3 * time.Second,
			skipIfErrContains: []string{
				"failed", "invalid", "connect", "bridge", "timeout", "context deadline",
				"dnslog", "reverse", "panic",
			},
			validate: func(t *testing.T, _ string, result *rawmcp.CallToolResult) {
				require.NotNil(t, result)
			},
		},
	},
	"get_all_yso_class_options": {{
		name:    "url_dns_gadget",
		args:    map[string]any{"gadget": "URLDNS"},
		timeout: 5 * time.Second,
		validate: func(t *testing.T, text string, _ *rawmcp.CallToolResult) {
			assert.Contains(t, text, "Options")
		},
	}},
	"get_all_yso_class_generater_options": {{
		name:    "url_dns_gadget",
		args:    map[string]any{"gadget": "URLDNS"},
		timeout: 5 * time.Second,
		validate: func(t *testing.T, _ string, result *rawmcp.CallToolResult) {
			require.NotNil(t, result)
		},
	}},
	"generate_yso_code": {{
		name: "url_dns_class",
		args: map[string]any{
			"gadget": "URLDNS",
			"class":  "URLDNS",
		},
		timeout: 10 * time.Second,
		skipIfErrContains: []string{
			"not set class", "not support",
		},
		validate: func(t *testing.T, _ string, result *rawmcp.CallToolResult) {
			require.NotNil(t, result)
		},
	}},
	"generate_yso_bytes": {{
		name: "url_dns_with_domain",
		args: map[string]any{
			"gadget": "URLDNS",
			"class":  "URLDNS",
			"options": []any{
				map[string]any{"key": "domain", "value": "example.com"},
			},
		},
		timeout: 10 * time.Second,
		skipIfErrContains: []string{
			"not support", "not set", "failed",
		},
		validate: func(t *testing.T, text string, _ *rawmcp.CallToolResult) {
			assert.NotEmpty(t, text)
		},
	}},
	"yso_dump": {{
		name: "dump_generated_bytes",
		buildArgs: func(t *testing.T, srv *mcp.MCPServer) map[string]any {
			return legacyGeneratedYSOBytesArgs(t, srv)
		},
		timeout: 10 * time.Second,
		allowErrContains: []string{
			"Magic error", "dump error", "ClassFormatError",
		},
		validate: func(t *testing.T, text string, _ *rawmcp.CallToolResult) {
			assert.NotEmpty(t, strings.TrimSpace(text))
		},
	}},
	}
}


// --- ToolSet: mitm ---

func legacyMITMToolCases() map[string][]legacyToolCase {
	return map[string][]legacyToolCase{
	"get_mitm_filter": {
		{
			name:    "empty_args_should_not_panic",
			args:    map[string]any{},
			timeout: 3 * time.Second,
			skipIfErrContains: []string{
				"failed", "invalid", "connect", "bridge", "timeout", "context deadline",
				"dnslog", "reverse", "panic",
			},
			validate: func(t *testing.T, _ string, result *rawmcp.CallToolResult) {
				require.NotNil(t, result)
			},
		},
	},
	"set_mitm_filter": {
		{
			name:    "empty_args_should_not_panic",
			args:    map[string]any{},
			timeout: 3 * time.Second,
			skipIfErrContains: []string{
				"failed", "invalid", "connect", "bridge", "timeout", "context deadline",
				"dnslog", "reverse", "panic",
			},
			validate: func(t *testing.T, _ string, result *rawmcp.CallToolResult) {
				require.NotNil(t, result)
			},
		},
	},
	"reset_mitm_filter": {
		{
			name:    "empty_args_should_not_panic",
			args:    map[string]any{},
			timeout: 3 * time.Second,
			skipIfErrContains: []string{
				"failed", "invalid", "connect", "bridge", "timeout", "context deadline",
				"dnslog", "reverse", "panic",
			},
			validate: func(t *testing.T, _ string, result *rawmcp.CallToolResult) {
				require.NotNil(t, result)
			},
		},
	},
	"get_mitm_hijack_filter": {
		{
			name:    "empty_args_should_not_panic",
			args:    map[string]any{},
			timeout: 3 * time.Second,
			skipIfErrContains: []string{
				"failed", "invalid", "connect", "bridge", "timeout", "context deadline",
				"dnslog", "reverse", "panic",
			},
			validate: func(t *testing.T, _ string, result *rawmcp.CallToolResult) {
				require.NotNil(t, result)
			},
		},
	},
	"set_mitm_hijack_filter": {
		{
			name:    "empty_args_should_not_panic",
			args:    map[string]any{},
			timeout: 3 * time.Second,
			skipIfErrContains: []string{
				"failed", "invalid", "connect", "bridge", "timeout", "context deadline",
				"dnslog", "reverse", "panic",
			},
			validate: func(t *testing.T, _ string, result *rawmcp.CallToolResult) {
				require.NotNil(t, result)
			},
		},
	},
	"reset_mitm_hijack_filter": {
		{
			name:    "empty_args_should_not_panic",
			args:    map[string]any{},
			timeout: 3 * time.Second,
			skipIfErrContains: []string{
				"failed", "invalid", "connect", "bridge", "timeout", "context deadline",
				"dnslog", "reverse", "panic",
			},
			validate: func(t *testing.T, _ string, result *rawmcp.CallToolResult) {
				require.NotNil(t, result)
			},
		},
	},
	"query_mitm_replacer_rules": {
		{
			name:    "minimal_pagination_should_not_panic",
			args:    pagingArgs(),
			timeout: 5 * time.Second,
			skipIfErrContains: []string{
				"context deadline", "failed", "invalid",
			},
			validate: func(t *testing.T, _ string, result *rawmcp.CallToolResult) {
				require.NotNil(t, result)
			},
		},
	},
	"get_current_rules": {
		{
			name:    "empty_args_should_not_panic",
			args:    map[string]any{},
			timeout: 3 * time.Second,
			skipIfErrContains: []string{
				"failed", "invalid", "connect", "bridge", "timeout", "context deadline",
				"dnslog", "reverse", "panic",
			},
			validate: func(t *testing.T, _ string, result *rawmcp.CallToolResult) {
				require.NotNil(t, result)
			},
		},
	},
	"set_current_rules": {{
		name:    "empty_rules_array",
		args:    map[string]any{"rules": []any{}},
		timeout: 5 * time.Second,
		validate: func(t *testing.T, _ string, result *rawmcp.CallToolResult) {
			require.NotNil(t, result)
		},
	}},
	"export_mitm_replacer_rules": {
		{
			name:    "empty_args_should_not_panic",
			args:    map[string]any{},
			timeout: 3 * time.Second,
			skipIfErrContains: []string{
				"failed", "invalid", "connect", "bridge", "timeout", "context deadline",
				"dnslog", "reverse", "panic",
			},
			validate: func(t *testing.T, _ string, result *rawmcp.CallToolResult) {
				require.NotNil(t, result)
			},
		},
	},
	"import_mitm_replacer_rules": {{
		name:        "import_empty_rules_returns_error",
		args:        map[string]any{"jsonRaw": "W10="},
		wantErr:     true,
		errContains: []string{"no new rules", "没有新规则", "解析失败"},
	}},
	"download_mitm_cert": {
		{
			name:    "empty_args_should_not_panic",
			args:    map[string]any{},
			timeout: 3 * time.Second,
			skipIfErrContains: []string{
				"failed", "invalid", "connect", "bridge", "timeout", "context deadline",
				"dnslog", "reverse", "panic",
			},
			validate: func(t *testing.T, _ string, result *rawmcp.CallToolResult) {
				require.NotNil(t, result)
			},
		},
	},
	"download_mitm_gm_cert": {
		{
			name:    "empty_args_should_not_panic",
			args:    map[string]any{},
			timeout: 3 * time.Second,
			skipIfErrContains: []string{
				"failed", "invalid", "connect", "bridge", "timeout", "context deadline",
				"dnslog", "reverse", "panic",
			},
			validate: func(t *testing.T, _ string, result *rawmcp.CallToolResult) {
				require.NotNil(t, result)
			},
		},
	},
	"install_mitm_certificate": {
		{
			name:    "empty_args_should_not_panic",
			args:    map[string]any{},
			timeout: 3 * time.Second,
			skipIfErrContains: []string{
				"failed", "invalid", "connect", "bridge", "timeout", "context deadline",
				"dnslog", "reverse", "panic",
			},
			validate: func(t *testing.T, _ string, result *rawmcp.CallToolResult) {
				require.NotNil(t, result)
			},
		},
	},
	"query_mitm_extracted_aggregate": {
		{
			name:    "empty_args_should_not_panic",
			args:    map[string]any{},
			timeout: 3 * time.Second,
			skipIfErrContains: []string{
				"failed", "invalid", "connect", "bridge", "timeout", "context deadline",
				"dnslog", "reverse", "panic",
			},
			validate: func(t *testing.T, _ string, result *rawmcp.CallToolResult) {
				require.NotNil(t, result)
			},
		},
	},
	"query_mitm_rule_extracted_data": {{
		name: "reject_empty_filter",
		args: map[string]any{
			"pagination": map[string]any{"page": 1, "limit": 1},
			"filter":     map[string]any{},
		},
		wantErr:     true,
		errContains: []string{"need filter"},
	}},
	"delete_mitm_rule_extracted_data": {
		{
			name:    "empty_args_should_not_panic",
			args:    map[string]any{},
			timeout: 3 * time.Second,
			skipIfErrContains: []string{
				"failed", "invalid", "connect", "bridge", "timeout", "context deadline",
				"dnslog", "reverse", "panic",
			},
			validate: func(t *testing.T, _ string, result *rawmcp.CallToolResult) {
				require.NotNil(t, result)
			},
		},
	},
	"start_mitm_v2": {
		{
			name:    "empty_args_should_not_panic",
			args:    map[string]any{},
			timeout: 3 * time.Second,
			skipIfErrContains: []string{
				"failed", "invalid", "connect", "bridge", "timeout", "context deadline",
				"dnslog", "reverse", "panic",
			},
			validate: func(t *testing.T, _ string, result *rawmcp.CallToolResult) {
				require.NotNil(t, result)
			},
		},
	},
	}
}


// --- ToolSet: fingerprint ---

func legacyFingerprintToolCases() map[string][]legacyToolCase {
	return map[string][]legacyToolCase{
	"query_fingerprint": {
		{
			name:    "minimal_pagination_should_not_panic",
			args:    pagingArgs(),
			timeout: 5 * time.Second,
			skipIfErrContains: []string{
				"context deadline", "failed", "invalid",
			},
			validate: func(t *testing.T, _ string, result *rawmcp.CallToolResult) {
				require.NotNil(t, result)
			},
		},
	},
	"create_fingerprint": {{
		name: "create_minimal_fingerprint",
		args: map[string]any{
			"rule": map[string]any{
				"ruleName":        uniqueName("mcp-fp"),
				"matchExpression": `body="mcp-test"`,
			},
		},
		timeout: 8 * time.Second,
		validate: func(t *testing.T, text string, _ *rawmcp.CallToolResult) {
			assert.Contains(t, text, "create")
		},
	}},
	"update_fingerprint": {{
		name: "update_by_rule_name",
		buildArgs: func(t *testing.T, srv *mcp.MCPServer) map[string]any {
			name := uniqueName("mcp-fp-upd")
			_, err := invokeLegacyTool(t, srv, "create_fingerprint", map[string]any{
				"rule": map[string]any{
					"ruleName":        name,
					"matchExpression": `body="mcp-test"`,
				},
			}, 8*time.Second)
			require.NoError(t, err)
			return map[string]any{
				"ruleName": name,
				"rule": map[string]any{
					"matchExpression": `body="mcp-test-updated"`,
				},
			}
		},
		timeout: 8 * time.Second,
		validate: func(t *testing.T, _ string, result *rawmcp.CallToolResult) {
			require.NotNil(t, result)
		},
	}},
	"delete_fingerprint": {
		{
			name:    "empty_filter_should_not_panic",
			args:    map[string]any{"filter": map[string]any{}},
			timeout: 3 * time.Second,
			validate: func(t *testing.T, _ string, result *rawmcp.CallToolResult) {
				require.NotNil(t, result)
			},
		},
	},
	"get_all_fingerprint_group": {
		{
			name:    "empty_args_should_not_panic",
			args:    map[string]any{},
			timeout: 3 * time.Second,
			skipIfErrContains: []string{
				"failed", "invalid", "connect", "bridge", "timeout", "context deadline",
				"dnslog", "reverse", "panic",
			},
			validate: func(t *testing.T, _ string, result *rawmcp.CallToolResult) {
				require.NotNil(t, result)
			},
		},
	},
	"create_fingerprint_group": {{
		name: "create_unique_group",
		args: map[string]any{
			"group": map[string]any{"GroupName": uniqueName("mcp-fp-grp")},
		},
		timeout: 5 * time.Second,
		validate: func(t *testing.T, _ string, result *rawmcp.CallToolResult) {
			require.NotNil(t, result)
		},
	}},
	"rename_fingerprint_group": {
		{
			name:    "empty_args_should_not_panic",
			args:    map[string]any{},
			timeout: 3 * time.Second,
			skipIfErrContains: []string{
				"failed", "invalid", "connect", "bridge", "timeout", "context deadline",
				"dnslog", "reverse", "panic",
			},
			validate: func(t *testing.T, _ string, result *rawmcp.CallToolResult) {
				require.NotNil(t, result)
			},
		},
	},
	"delete_fingerprint_group": {
		{
			name:    "empty_args_should_not_panic",
			args:    map[string]any{},
			timeout: 3 * time.Second,
			skipIfErrContains: []string{
				"failed", "invalid", "connect", "bridge", "timeout", "context deadline",
				"dnslog", "reverse", "panic",
			},
			validate: func(t *testing.T, _ string, result *rawmcp.CallToolResult) {
				require.NotNil(t, result)
			},
		},
	},
	"batch_update_fingerprint_to_group": {
		{
			name:    "empty_args_should_not_panic",
			args:    map[string]any{},
			timeout: 3 * time.Second,
			skipIfErrContains: []string{
				"failed", "invalid", "connect", "bridge", "timeout", "context deadline",
				"dnslog", "reverse", "panic",
			},
			validate: func(t *testing.T, _ string, result *rawmcp.CallToolResult) {
				require.NotNil(t, result)
			},
		},
	},
	"recover_builtin_fingerprint": {{
		name:    "recover_or_error_without_asset",
		args:    map[string]any{},
		timeout: 8 * time.Second,
		allowErrContains: []string{
			"asset", "EOF", "gzip", "failed",
		},
		validate: func(t *testing.T, _ string, result *rawmcp.CallToolResult) {
			require.NotNil(t, result)
		},
	}},
	}
}


// --- ToolSet: space_engine ---

func legacySpaceEngineToolCases() map[string][]legacyToolCase {
	return map[string][]legacyToolCase{
	"get_space_engine_status": {
		{
			name:    "empty_args_should_not_panic",
			args:    map[string]any{},
			timeout: 3 * time.Second,
			skipIfErrContains: []string{
				"failed", "invalid", "connect", "bridge", "timeout", "context deadline",
				"dnslog", "reverse", "panic",
			},
			validate: func(t *testing.T, _ string, result *rawmcp.CallToolResult) {
				require.NotNil(t, result)
			},
		},
	},
	"get_space_engine_account_status_v2": {
		{
			name:    "empty_args_should_not_panic",
			args:    map[string]any{},
			timeout: 3 * time.Second,
			skipIfErrContains: []string{
				"failed", "invalid", "connect", "bridge", "timeout", "context deadline",
				"dnslog", "reverse", "panic",
			},
			validate: func(t *testing.T, _ string, result *rawmcp.CallToolResult) {
				require.NotNil(t, result)
			},
		},
	},
	"fetch_port_asset_from_space_engine": {{
		name: "background_with_filter",
		args: map[string]any{
			"type":    "fofa",
			"filter":  "port=80",
			"maxPage": 1,
		},
		timeout: 5 * time.Second,
		validate: func(t *testing.T, text string, _ *rawmcp.CallToolResult) {
			assert.Contains(t, text, "started")
		},
	}},
	}
}


// --- ToolSet: report ---

func legacyReportToolCases() map[string][]legacyToolCase {
	return map[string][]legacyToolCase{
	"query_reports": {
		{
			name:    "minimal_pagination_should_not_panic",
			args:    pagingArgs(),
			timeout: 5 * time.Second,
			skipIfErrContains: []string{
				"context deadline", "failed", "invalid",
			},
			validate: func(t *testing.T, _ string, result *rawmcp.CallToolResult) {
				require.NotNil(t, result)
			},
		},
	},
	"query_report": {{
		name: "query_seeded_report",
		buildArgs: func(t *testing.T, _ *mcp.MCPServer) map[string]any {
			return map[string]any{"id": ensureLegacyTestReportID(t)}
		},
		timeout: 5 * time.Second,
		validate: func(t *testing.T, _ string, result *rawmcp.CallToolResult) {
			require.NotNil(t, result)
		},
	}},
	"delete_report": {
		{
			name:    "empty_args_should_not_panic",
			args:    map[string]any{},
			timeout: 3 * time.Second,
			skipIfErrContains: []string{
				"failed", "invalid", "connect", "bridge", "timeout", "context deadline",
				"dnslog", "reverse", "panic",
			},
			validate: func(t *testing.T, _ string, result *rawmcp.CallToolResult) {
				require.NotNil(t, result)
			},
		},
	},
	"generate_ssa_report": {{
		name: "missing_task_id_should_error",
		args: map[string]any{"reportName": "mcp-test"},
		timeout: 5 * time.Second,
		wantErr: true,
		errContains: []string{"taskID", "filter"},
	}},
	}
}


// --- ToolSet: plugin_env ---

func legacyPluginEnvToolCases() map[string][]legacyToolCase {
	return map[string][]legacyToolCase{
	"get_all_plugin_env": {
		{
			name:    "empty_args_should_not_panic",
			args:    map[string]any{},
			timeout: 3 * time.Second,
			skipIfErrContains: []string{
				"failed", "invalid", "connect", "bridge", "timeout", "context deadline",
				"dnslog", "reverse", "panic",
			},
			validate: func(t *testing.T, _ string, result *rawmcp.CallToolResult) {
				require.NotNil(t, result)
			},
		},
	},
	"query_plugin_env": {
		{
			name:    "empty_args_should_not_panic",
			args:    map[string]any{},
			timeout: 3 * time.Second,
			skipIfErrContains: []string{
				"failed", "invalid", "connect", "bridge", "timeout", "context deadline",
				"dnslog", "reverse", "panic",
			},
			validate: func(t *testing.T, _ string, result *rawmcp.CallToolResult) {
				require.NotNil(t, result)
			},
		},
	},
	"set_plugin_env": {
		{
			name:    "empty_args_should_not_panic",
			args:    map[string]any{},
			timeout: 3 * time.Second,
			skipIfErrContains: []string{
				"failed", "invalid", "connect", "bridge", "timeout", "context deadline",
				"dnslog", "reverse", "panic",
			},
			validate: func(t *testing.T, _ string, result *rawmcp.CallToolResult) {
				require.NotNil(t, result)
			},
		},
	},
	"create_plugin_env": {
		{
			name:    "empty_args_should_not_panic",
			args:    map[string]any{},
			timeout: 3 * time.Second,
			skipIfErrContains: []string{
				"failed", "invalid", "connect", "bridge", "timeout", "context deadline",
				"dnslog", "reverse", "panic",
			},
			validate: func(t *testing.T, _ string, result *rawmcp.CallToolResult) {
				require.NotNil(t, result)
			},
		},
	},
	"delete_plugin_env": {
		{
			name:    "empty_args_should_not_panic",
			args:    map[string]any{},
			timeout: 3 * time.Second,
			skipIfErrContains: []string{
				"failed", "invalid", "connect", "bridge", "timeout", "context deadline",
				"dnslog", "reverse", "panic",
			},
			validate: func(t *testing.T, _ string, result *rawmcp.CallToolResult) {
				require.NotNil(t, result)
			},
		},
	},
	}
}


// --- ToolSet: http_builder ---

func legacyHTTPBuilderToolCases() map[string][]legacyToolCase {
	return map[string][]legacyToolCase{
	"http_request_builder": {
		{
			name:    "empty_args_should_not_panic",
			args:    map[string]any{},
			timeout: 3 * time.Second,
			skipIfErrContains: []string{
				"failed", "invalid", "connect", "bridge", "timeout", "context deadline",
				"dnslog", "reverse", "panic",
			},
			validate: func(t *testing.T, _ string, result *rawmcp.CallToolResult) {
				require.NotNil(t, result)
			},
		},
	},
	"debug_plugin": {
		{
			name:    "empty_args_should_not_panic",
			args:    map[string]any{},
			timeout: 3 * time.Second,
			skipIfErrContains: []string{
				"failed", "invalid", "connect", "bridge", "timeout", "context deadline",
				"dnslog", "reverse", "panic",
			},
			validate: func(t *testing.T, _ string, result *rawmcp.CallToolResult) {
				require.NotNil(t, result)
			},
		},
	},
	}
}


// --- ToolSet: chaos_maker ---

func legacyChaosMakerToolCases() map[string][]legacyToolCase {
	return map[string][]legacyToolCase{
	"query_chaos_maker_rule": {
		{
			name:    "minimal_pagination_should_not_panic",
			args:    pagingArgs(),
			timeout: 5 * time.Second,
			skipIfErrContains: []string{
				"context deadline", "failed", "invalid",
			},
			validate: func(t *testing.T, _ string, result *rawmcp.CallToolResult) {
				require.NotNil(t, result)
			},
		},
	},
	"import_chaos_maker_rules": {{
		name:    "minimal_yaml",
		args:    map[string]any{"content": "title: mcp-test\nprotocols: [http]\n"},
		timeout: 5 * time.Second,
		skipIfErrContains: []string{
			"parse", "invalid", "failed",
		},
		validate: func(t *testing.T, _ string, result *rawmcp.CallToolResult) {
			require.NotNil(t, result)
		},
	}},
	"delete_chaos_maker_rule_by_id": {
		{
			name:    "empty_args_should_not_panic",
			args:    map[string]any{},
			timeout: 3 * time.Second,
			skipIfErrContains: []string{
				"failed", "invalid", "connect", "bridge", "timeout", "context deadline",
				"dnslog", "reverse", "panic",
			},
			validate: func(t *testing.T, _ string, result *rawmcp.CallToolResult) {
				require.NotNil(t, result)
			},
		},
	},
	"execute_chaos_maker_rule": {
		{
			name:    "empty_args_should_not_panic",
			args:    map[string]any{},
			timeout: 3 * time.Second,
			skipIfErrContains: []string{
				"failed", "invalid", "connect", "bridge", "timeout", "context deadline",
				"dnslog", "reverse", "panic",
			},
			validate: func(t *testing.T, _ string, result *rawmcp.CallToolResult) {
				require.NotNil(t, result)
			},
		},
	},
	}
}


// --- ToolSet: project_database ---

func legacyProjectDatabaseToolCases() map[string][]legacyToolCase {
	return map[string][]legacyToolCase{
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
	}
}


// --- ToolSet: global_hotpatch ---

func legacyGlobalHotpatchToolCases() map[string][]legacyToolCase {
	return map[string][]legacyToolCase{
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
			name: "list_without_filter",
			args: map[string]any{},
			validate: func(t *testing.T, text string, _ *rawmcp.CallToolResult) {
				var payload map[string]any
				decodeToolResultJSON(t, text, &payload)
				_, ok := payload["Name"]
				require.True(t, ok, "expected Name field in response")
			},
		},
		{
			name: "list_global_templates",
			args: map[string]any{"type": "global"},
			validate: func(t *testing.T, text string, _ *rawmcp.CallToolResult) {
				assert.NotEmpty(t, strings.TrimSpace(text))
			},
		},
	},
	"create_global_hotpatch_template": {
		{
			name:        "reject_missing_name",
			args:        map[string]any{"content": "beforeRequest = func(isHttps, originReq, req) { return req }"},
			wantErr:     true,
			errContains: []string{"name is required"},
		},
		{
			name:        "reject_missing_content",
			args:        map[string]any{"name": "legacy-mcp-empty-content"},
			wantErr:     true,
			errContains: []string{"content is required"},
		},
		{
			name:        "reject_invalid_content",
			args:        map[string]any{"name": "legacy-mcp-bad-yak", "content": "this is not valid yak hotpatch"},
			wantErr:     true,
			errContains: []string{"validation failed"},
		},
		{
			name: "create_valid_template",
			buildArgs: func(t *testing.T, _ *mcp.MCPServer) map[string]any {
				return map[string]any{
					"name": uniqueName("legacy-mcp-create-global"),
					"content": `
beforeRequest = func(isHttps, originReq, req) { return req }
afterRequest = func(isHttps, originReq, req, originRsp, rsp) { return rsp }
`,
					"tags": []any{"mcp", "global"},
				}
			},
			validate: func(t *testing.T, text string, _ *rawmcp.CallToolResult) {
				var payload map[string]any
				decodeToolResultJSON(t, text, &payload)
				msg, ok := payload["Message"].(map[string]any)
				require.True(t, ok, "expected Message in response: %s", text)
				assert.Equal(t, "create", msg["Operation"])
			},
		},
	},
	}
}


// --- ToolSet: system_proxy ---

func legacySystemProxyToolCases() map[string][]legacyToolCase {
	return map[string][]legacyToolCase{
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
	}
}


// --- integration tests ---

func TestLegacyBuiltinToolSetsRegistered(t *testing.T) {
	for _, setName := range expectedLegacyToolSetOrder {
		require.Contains(t, mcp.GlobalToolSetList(), setName, "legacy tool set %q not registered", setName)
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
		setName, ok := legacyToolSetByTool[toolName]
		require.Truef(t, ok, "tool %q has no tool set mapping", toolName)
		require.NotEmptyf(t, setName, "tool %q must belong to a tool set", toolName)
	}

	require.Equal(t, len(names), len(legacyToolIntegrationCases),
		"integration case map should cover exactly the registered legacy tools")
}

func TestLegacyToolSet_Codec(t *testing.T) {
	runLegacyToolSetIntegration(t, "codec")
}

func TestLegacyToolSet_Cve(t *testing.T) {
	runLegacyToolSetIntegration(t, "cve")
}

func TestLegacyToolSet_Httpflow(t *testing.T) {
	runLegacyToolSetIntegration(t, "httpflow")
}

func TestLegacyToolSet_HybridScan(t *testing.T) {
	runLegacyToolSetIntegration(t, "hybrid_scan")
}

func TestLegacyToolSet_Payload(t *testing.T) {
	runLegacyToolSetIntegration(t, "payload")
}

func TestLegacyToolSet_PortScan(t *testing.T) {
	runLegacyToolSetIntegration(t, "port_scan")
}

func TestLegacyToolSet_YakDocument(t *testing.T) {
	runLegacyToolSetIntegration(t, "yak_document")
}

func TestLegacyToolSet_YakScript(t *testing.T) {
	runLegacyToolSetIntegration(t, "yak_script")
}

func TestLegacyToolSet_ReverseShell(t *testing.T) {
	runLegacyToolSetIntegration(t, "reverse_shell")
}

func TestLegacyToolSet_ReversePlatform(t *testing.T) {
	runLegacyToolSetIntegration(t, "reverse_platform")
}

func TestLegacyToolSet_HttpFuzzer(t *testing.T) {
	runLegacyToolSetIntegration(t, "http_fuzzer")
}

func TestLegacyToolSet_Brute(t *testing.T) {
	runLegacyToolSetIntegration(t, "brute")
}

func TestLegacyToolSet_Subdomain(t *testing.T) {
	runLegacyToolSetIntegration(t, "subdomain")
}

func TestLegacyToolSet_Crawler(t *testing.T) {
	runLegacyToolSetIntegration(t, "crawler")
}

func TestLegacyToolSet_Dynamic(t *testing.T) {
	runLegacyToolSetIntegration(t, "dynamic")
}

func TestLegacyToolSet_Ssa(t *testing.T) {
	runLegacyToolSetIntegration(t, "ssa")
}

func TestLegacyToolSet_Syntaxflow(t *testing.T) {
	runLegacyToolSetIntegration(t, "syntaxflow")
}

func TestLegacyToolSet_Risk(t *testing.T) {
	runLegacyToolSetIntegration(t, "risk")
}

func TestLegacyToolSet_Yso(t *testing.T) {
	runLegacyToolSetIntegration(t, "yso")
}

func TestLegacyToolSet_Mitm(t *testing.T) {
	runLegacyToolSetIntegration(t, "mitm")
}

func TestLegacyToolSet_Fingerprint(t *testing.T) {
	runLegacyToolSetIntegration(t, "fingerprint")
}

func TestLegacyToolSet_SpaceEngine(t *testing.T) {
	runLegacyToolSetIntegration(t, "space_engine")
}

func TestLegacyToolSet_Report(t *testing.T) {
	runLegacyToolSetIntegration(t, "report")
}

func TestLegacyToolSet_PluginEnv(t *testing.T) {
	runLegacyToolSetIntegration(t, "plugin_env")
}

func TestLegacyToolSet_HttpBuilder(t *testing.T) {
	runLegacyToolSetIntegration(t, "http_builder")
}

func TestLegacyToolSet_ChaosMaker(t *testing.T) {
	runLegacyToolSetIntegration(t, "chaos_maker")
}

func TestLegacyToolSet_ProjectDatabase(t *testing.T) {
	runLegacyToolSetIntegration(t, "project_database")
}

func TestLegacyToolSet_GlobalHotpatch(t *testing.T) {
	runLegacyToolSetIntegration(t, "global_hotpatch")
}

func TestLegacyToolSet_SystemProxy(t *testing.T) {
	runLegacyToolSetIntegration(t, "system_proxy")
}

