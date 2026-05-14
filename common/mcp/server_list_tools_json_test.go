package mcp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/log"
	mcpserver "github.com/yaklang/yaklang/common/mcp/mcp-go/server"
	"github.com/yaklang/yaklang/common/utils/omap"
)

func TestMCPServerListToolsJSONFormat(t *testing.T) {
	log.SetLevel(log.FatalLevel)

	s, err := NewMCPServer()
	require.NoError(t, err)

	testServer := mcpserver.NewTestServer(s.server)
	defer testServer.Close()

	println(testServer.URL)
	time.Sleep(10 * time.Hour)
	sseResp, messageURL := openMcpSSESession(t, testServer.URL)
	defer sseResp.Body.Close()

	initializeResp := postMcpJSONRequest(t, messageURL, map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "initialize",
		"params": map[string]any{
			"protocolVersion": "2024-11-05",
			"clientInfo": map[string]any{
				"name":    "test-list-tools-json-client",
				"version": "1.0.0",
			},
		},
	})
	defer initializeResp.Body.Close()
	require.Equal(t, http.StatusAccepted, initializeResp.StatusCode)

	listResp := postMcpJSONRequest(t, messageURL, map[string]any{
		"jsonrpc": "2.0",
		"id":      2,
		"method":  "tools/list",
		"params":  map[string]any{},
	})
	defer listResp.Body.Close()
	require.Equal(t, http.StatusAccepted, listResp.StatusCode)

	rawJSON, err := io.ReadAll(listResp.Body)
	// println(string(rawJSON))
	require.NoError(t, err)
	require.True(t, json.Valid(rawJSON), "tools/list response should be valid JSON: %s", string(rawJSON))

	var response struct {
		JSONRPC string `json:"jsonrpc"`
		ID      int    `json:"id"`
		Result  struct {
			Tools []struct {
				Name        string         `json:"name"`
				Description string         `json:"description,omitempty"`
				InputSchema map[string]any `json:"inputSchema"`
			} `json:"tools"`
		} `json:"result"`
		Error any `json:"error,omitempty"`
	}
	require.NoError(t, json.Unmarshal(rawJSON, &response))

	require.Equal(t, "2.0", response.JSONRPC)
	require.Equal(t, 2, response.ID)
	require.Nil(t, response.Error)
	require.NotEmpty(t, response.Result.Tools)

	expectedToolNames := collectExpectedSSEToolNames()
	actualToolNames := make(map[string]struct{}, len(response.Result.Tools))
	for _, tool := range response.Result.Tools {
		require.NotEmpty(t, tool.Name)
		require.NotNil(t, tool.InputSchema)

		schemaType, ok := tool.InputSchema["type"].(string)
		require.True(t, ok)
		require.Equal(t, "object", schemaType)

		actualToolNames[tool.Name] = struct{}{}
	}

	for name := range expectedToolNames {
		_, ok := actualToolNames[name]
		require.Truef(t, ok, "missing tool from SSE tools/list response JSON: %s", name)
	}

	_, hasDynamicAddTool := actualToolNames["dynamic_add_tool"]
	require.False(t, hasDynamicAddTool, "dynamic_add_tool should be hidden over SSE")

	_, hasExecYakScript := actualToolNames["exec_yak_script"]
	require.False(t, hasExecYakScript, "exec_yak_script should be hidden over SSE")
}

func collectExpectedSSEToolNames() map[string]struct{} {
	expected := collectExpectedGlobalToolNames()
	delete(expected, "dynamic_add_tool")
	delete(expected, "exec_yak_script")
	return expected
}

func collectExpectedGlobalToolNames() map[string]struct{} {
	expected := make(map[string]struct{}, len(globalTools))
	for name := range globalTools {
		expected[name] = struct{}{}
	}
	for _, toolSet := range globalToolSets {
		for name := range toolSet.Tools {
			expected[name] = struct{}{}
		}
	}
	return expected
}

func openMcpSSESession(t *testing.T, baseURL string) (*http.Response, string) {
	t.Helper()

	req, err := http.NewRequest(http.MethodGet, baseURL+"/sse", nil)
	require.NoError(t, err)
	req.Close = true

	resp, err := newMcpTestHTTPClient().Do(req)
	require.NoError(t, err)

	buf := make([]byte, 1024)
	n, err := resp.Body.Read(buf)
	require.NoError(t, err)

	endpointEvent := string(buf[:n])
	require.Contains(t, endpointEvent, "event: endpoint")

	messageURL := strings.TrimSpace(strings.Split(strings.Split(endpointEvent, "data: ")[1], "\n")[0])
	return resp, messageURL
}

func postMcpJSONRequest(t *testing.T, messageURL string, payload map[string]any) *http.Response {
	t.Helper()

	body, err := json.Marshal(payload)
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodPost, messageURL, bytes.NewBuffer(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Close = true

	resp, err := newMcpTestHTTPClient().Do(req)
	require.NoError(t, err)
	return resp
}

func newMcpTestHTTPClient() *http.Client {
	return &http.Client{
		Transport: &http.Transport{DisableKeepAlives: true},
	}
}

// toolNamePattern matches the allowed character set from MCP spec draft:
// uppercase/lowercase ASCII letters, digits, underscore, hyphen, dot.
// Spaces, commas and other special characters are explicitly disallowed.
const toolNameAllowedChars = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789_-."
const toolNameMaxLen = 128

// TestMCPToolSchemaLint checks every registered tool against:
//  1. MCP protocol spec (draft 2025-11-25) constraints on tool metadata and inputSchema
//  2. Gemini function-calling API constraints on JSON Schema enum usage
//
// MCP spec rules (source: spec.modelcontextprotocol.io/specification/draft):
//
//	[spec-name-empty]        tool name MUST NOT be empty
//	[spec-name-length]       tool name SHOULD be ≤ 128 characters
//	[spec-name-chars]        tool name SHOULD only contain [A-Za-z0-9_\-.], no spaces or commas
//	[spec-schema-type]       inputSchema.type MUST be "object"
//	[spec-required-strings]  inputSchema.required entries MUST be strings
//
// Gemini model compatibility rules:
//
//	[enum-non-string]        enum is only allowed on string-typed fields
//	[enum-on-array]          array fields must not carry a top-level enum (use items.enum)
//	[enum-empty-string]      enum values must not be empty strings
//	[enum-value-non-string]  enum values must themselves be strings, not numbers or booleans
func TestMCPToolSchemaLint(t *testing.T) {
	tools := collectAllRegisteredToolHandlers()
	require.NotEmpty(t, tools, "no tools registered")

	var violations []string
	for _, twh := range tools {
		tool := twh.tool
		toolPath := "tool=" + tool.Name

		// --- MCP spec: tool name constraints ---
		if tool.Name == "" {
			violations = append(violations, "[spec-name-empty] (anonymous tool): name must not be empty")
		} else {
			if len(tool.Name) > toolNameMaxLen {
				violations = append(violations,
					fmt.Sprintf("[spec-name-length] %s: name length %d exceeds 128-character limit", toolPath, len(tool.Name)))
			}
			for _, ch := range tool.Name {
				if !strings.ContainsRune(toolNameAllowedChars, ch) {
					violations = append(violations,
						fmt.Sprintf("[spec-name-chars] %s: name contains disallowed character %q (only [A-Za-z0-9_-.] allowed)", toolPath, ch))
					break
				}
			}
		}

		// --- MCP spec: inputSchema.type must be "object" ---
		if tool.InputSchema.Type != "object" {
			violations = append(violations,
				fmt.Sprintf("[spec-schema-type] %s: inputSchema.type must be \"object\", got %q", toolPath, tool.InputSchema.Type))
		}

		// --- MCP spec: required entries must be non-empty strings ---
		for i, req := range tool.InputSchema.Required {
			if req == "" {
				violations = append(violations,
					fmt.Sprintf("[spec-required-strings] %s: inputSchema.required[%d] is an empty string", toolPath, i))
			}
		}

		// --- Schema node lint (Gemini compat + recursive) ---
		if tool.InputSchema.Properties != nil {
			tool.InputSchema.Properties.ForEach(func(propName string, propVal any) bool {
				prop, ok := propVal.(map[string]any)
				if !ok {
					return true
				}
				lintSchemaNode(toolPath+"."+propName, prop, &violations)
				return true
			})
		}
	}

	if len(violations) > 0 {
		t.Errorf("found %d schema lint violation(s):\n%s",
			len(violations), strings.Join(violations, "\n"))
	}
}

// lintSchemaNode recursively validates a single schema node against compatibility rules.
func lintSchemaNode(path string, schema map[string]any, violations *[]string) {
	fieldType, _ := schema["type"].(string)
	rawEnum, hasEnum := schema["enum"]

	if hasEnum {
		// Rule 1 & 2: enum is only allowed on string-typed fields; array is explicitly forbidden.
		if fieldType == "array" {
			*violations = append(*violations,
				fmt.Sprintf("[enum-on-array] %s: enum on array type is rejected by Gemini; move enum into items", path))
		} else if fieldType != "" && fieldType != "string" {
			*violations = append(*violations,
				fmt.Sprintf("[enum-non-string] %s: enum is only allowed for STRING type, got %q", path, fieldType))
		}
		if enumSlice, ok := rawEnum.([]any); ok {
			for i, v := range enumSlice {
				switch val := v.(type) {
				case string:
					// Rule 3: enum values must not be empty strings.
					if val == "" {
						*violations = append(*violations,
							fmt.Sprintf("[enum-empty-string] %s: enum[%d] is an empty string, rejected by Gemini", path, i))
					}
				default:
					// Rule 4: enum values themselves must be strings (Gemini rejects non-string enum values).
					*violations = append(*violations,
						fmt.Sprintf("[enum-value-non-string] %s: enum[%d] has non-string value %T(%v), rejected by Gemini", path, i, v, v))
				}
			}
		}
	}

	// Recurse into nested object properties.
	// WithStruct stores properties as *omap.OrderedMap; WithStructArray stores them
	// as flat keys in the items map. Handle both cases.
	lintPropertiesValue(path, schema["properties"], violations)

	// Recurse into array items.
	if items, ok := schema["items"].(map[string]any); ok {
		lintSchemaNode(path+".items", items, violations)
		// WithStructArray stores child properties directly in items (not under a
		// "properties" sub-key), so also scan non-schema keys for child schemas.
		lintStructArrayItems(path+".items", items, violations)
	}
	// Recurse into oneOf / anyOf sub-schemas.
	for _, keyword := range []string{"oneOf", "anyOf"} {
		if arr, ok := schema[keyword].([]any); ok {
			for i, sub := range arr {
				if child, ok := sub.(map[string]any); ok {
					subPath := fmt.Sprintf("%s.%s[%d]", path, keyword, i)
					lintSchemaNode(subPath, child, violations)
					lintPropertiesValue(subPath, child["properties"], violations)
				}
			}
		}
	}
}

// lintPropertiesValue handles both map[string]any and *omap.OrderedMap[string,any]
// representations of a "properties" value and recurses into each child property.
func lintPropertiesValue(path string, raw any, violations *[]string) {
	switch props := raw.(type) {
	case map[string]any:
		for k, v := range props {
			if child, ok := v.(map[string]any); ok {
				lintSchemaNode(path+"."+k, child, violations)
			}
		}
	case *omap.OrderedMap[string, any]:
		if props == nil {
			return
		}
		props.ForEach(func(k string, v any) bool {
			if child, ok := v.(map[string]any); ok {
				lintSchemaNode(path+"."+k, child, violations)
			}
			return true
		})
	}
}

// knownSchemaKeys are top-level JSON Schema keywords; everything else in a
// WithStructArray items map is a child property schema.
var knownSchemaKeys = map[string]struct{}{
	"type": {}, "properties": {}, "items": {}, "required": {},
	"enum": {}, "description": {}, "default": {}, "title": {},
	"minimum": {}, "maximum": {}, "minLength": {}, "maxLength": {},
	"pattern": {}, "multipleOf": {}, "oneOf": {}, "anyOf": {}, "allOf": {},
}

// lintStructArrayItems handles the flat-property layout produced by WithStructArray,
// where child property schemas are stored directly as keys in the items map.
func lintStructArrayItems(path string, items map[string]any, violations *[]string) {
	for k, v := range items {
		if _, known := knownSchemaKeys[k]; known {
			continue
		}
		if child, ok := v.(map[string]any); ok {
			lintSchemaNode(path+"."+k, child, violations)
		}
	}
}

// collectAllRegisteredToolHandlers returns all ToolWithHandler entries from globalTools.
// globalToolSets entries are already copied into globalTools by AddGlobalToolSet, so
// iterating globalTools alone is sufficient and avoids duplicate checks.
func collectAllRegisteredToolHandlers() []*ToolWithHandler {
	var tools []*ToolWithHandler
	for _, t := range globalTools {
		tools = append(tools, t)
	}
	return tools
}

// TestMCPToolSchemaLintRules verifies every lint rule fires correctly on
// synthetic violating inputs, ensuring the checker has not silently regressed.
func TestMCPToolSchemaLintRules(t *testing.T) {
	// --- schema-node-level rules (lintSchemaNode) ---
	nodeCases := []struct {
		name           string
		schema         map[string]any
		wantRulePrefix string
	}{
		// Gemini compat rules
		{
			name:           "number with enum",
			schema:         map[string]any{"type": "number", "enum": []any{0, 1, 2}},
			wantRulePrefix: "[enum-non-string]",
		},
		{
			name:           "integer with enum",
			schema:         map[string]any{"type": "integer", "enum": []any{1, 2}},
			wantRulePrefix: "[enum-non-string]",
		},
		{
			name:           "boolean with enum",
			schema:         map[string]any{"type": "boolean", "enum": []any{true, false}},
			wantRulePrefix: "[enum-non-string]",
		},
		{
			name:           "array with top-level enum",
			schema:         map[string]any{"type": "array", "enum": []any{"a", "b"}, "items": map[string]any{"type": "string"}},
			wantRulePrefix: "[enum-on-array]",
		},
		{
			name:           "string enum with empty value",
			schema:         map[string]any{"type": "string", "enum": []any{"", "ok"}},
			wantRulePrefix: "[enum-empty-string]",
		},
		{
			name:           "string field with numeric enum values",
			schema:         map[string]any{"type": "string", "enum": []any{0, 1, 2}},
			wantRulePrefix: "[enum-value-non-string]",
		},
		{
			name:           "number field with numeric enum values",
			schema:         map[string]any{"type": "number", "enum": []any{0, 1, 2}},
			wantRulePrefix: "[enum-non-string]",
		},
	}
	for _, tc := range nodeCases {
		t.Run("schema/"+tc.name, func(t *testing.T) {
			var violations []string
			lintSchemaNode("test", tc.schema, &violations)
			require.NotEmpty(t, violations, "expected lint to catch a violation but got none")
			require.True(t, strings.HasPrefix(violations[0], tc.wantRulePrefix),
				"expected violation starting with %q, got: %s", tc.wantRulePrefix, violations[0])
		})
	}

	// --- tool-level MCP spec rules ---
	// These are exercised via lintToolMeta, a small helper that runs only the
	// tool-level checks so we can test them in isolation.
	toolCases := []struct {
		name           string
		toolName       string
		schemaType     string
		required       []string
		wantRulePrefix string
	}{
		{
			name:           "empty tool name",
			toolName:       "",
			schemaType:     "object",
			wantRulePrefix: "[spec-name-empty]",
		},
		{
			name:           "tool name too long",
			toolName:       strings.Repeat("a", 129),
			schemaType:     "object",
			wantRulePrefix: "[spec-name-length]",
		},
		{
			name:           "tool name with space",
			toolName:       "bad name",
			schemaType:     "object",
			wantRulePrefix: "[spec-name-chars]",
		},
		{
			name:           "tool name with comma",
			toolName:       "bad,name",
			schemaType:     "object",
			wantRulePrefix: "[spec-name-chars]",
		},
		{
			name:           "inputSchema type not object",
			toolName:       "ok_name",
			schemaType:     "string",
			wantRulePrefix: "[spec-schema-type]",
		},
		{
			name:           "required contains empty string",
			toolName:       "ok_name",
			schemaType:     "object",
			required:       []string{"valid", ""},
			wantRulePrefix: "[spec-required-strings]",
		},
	}
	for _, tc := range toolCases {
		t.Run("tool/"+tc.name, func(t *testing.T) {
			violations := lintToolMeta(tc.toolName, tc.schemaType, tc.required)
			require.NotEmpty(t, violations, "expected lint to catch a violation but got none")
			require.True(t, strings.HasPrefix(violations[0], tc.wantRulePrefix),
				"expected violation starting with %q, got: %s", tc.wantRulePrefix, violations[0])
		})
	}
}

// lintToolMeta runs the tool-level MCP spec checks in isolation, for testability.
func lintToolMeta(name, schemaType string, required []string) []string {
	var violations []string
	toolPath := "tool=" + name
	if name == "" {
		violations = append(violations, "[spec-name-empty] (anonymous tool): name must not be empty")
	} else {
		if len(name) > toolNameMaxLen {
			violations = append(violations,
				fmt.Sprintf("[spec-name-length] %s: name length %d exceeds 128-character limit", toolPath, len(name)))
		}
		for _, ch := range name {
			if !strings.ContainsRune(toolNameAllowedChars, ch) {
				violations = append(violations,
					fmt.Sprintf("[spec-name-chars] %s: name contains disallowed character %q", toolPath, ch))
				break
			}
		}
	}
	if schemaType != "object" {
		violations = append(violations,
			fmt.Sprintf("[spec-schema-type] %s: inputSchema.type must be \"object\", got %q", toolPath, schemaType))
	}
	for i, req := range required {
		if req == "" {
			violations = append(violations,
				fmt.Sprintf("[spec-required-strings] %s: inputSchema.required[%d] is an empty string", toolPath, i))
		}
	}
	return violations
}

// TestMCPToolSchemaDump prints every registered tool's full JSON schema to test output.
// Run with -v to see the full dump; useful for manual review of all schemas at once.
func TestMCPToolSchemaDump(t *testing.T) {
	tools := collectAllRegisteredToolHandlers()
	require.NotEmpty(t, tools, "no tools registered")

	// Sort tool names for stable output.
	names := make([]string, 0, len(tools))
	byName := make(map[string]*ToolWithHandler, len(tools))
	for _, twh := range tools {
		names = append(names, twh.tool.Name)
		byName[twh.tool.Name] = twh
	}
	sortStrings(names)

	for _, name := range names {
		twh := byName[name]
		schemaBytes, err := json.MarshalIndent(twh.tool.InputSchema, "  ", "  ")
		require.NoError(t, err)
		t.Logf("=== tool: %s ===\n  %s\n", name, string(schemaBytes))
	}
	t.Logf("total tools: %d", len(tools))
}

func sortStrings(ss []string) {
	for i := 1; i < len(ss); i++ {
		for j := i; j > 0 && ss[j] < ss[j-1]; j-- {
			ss[j], ss[j-1] = ss[j-1], ss[j]
		}
	}
}
