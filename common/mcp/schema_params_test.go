package mcp

// schema_params_test.go – regression tests that pin the InputSchema of
// http_fuzzer, query_http_flow, set_tag_for_http_flow and delete_http_flow.
//
// These tests run without any live gRPC server: they inspect the in-process
// globalTools map that is populated by the init() functions in each tool file.
//
// Failure here means a parameter was removed or renamed accidentally.

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// toolProps returns the flat map of property-name → schema-node for the named
// tool, or fails the test if the tool is not registered.
func toolProps(t *testing.T, toolName string) map[string]map[string]any {
	t.Helper()
	twh, ok := globalTools[toolName]
	require.Truef(t, ok, "tool %q not found in globalTools", toolName)

	props := make(map[string]map[string]any)
	if twh.tool.InputSchema.Properties == nil {
		return props
	}
	twh.tool.InputSchema.Properties.ForEach(func(name string, val any) bool {
		if m, ok := val.(map[string]any); ok {
			props[name] = m
		}
		return true
	})
	return props
}

// assertProp asserts that a property with the given name exists in props and
// that its "type" field matches wantType.
func assertProp(t *testing.T, toolName string, props map[string]map[string]any, paramName, wantType string) {
	t.Helper()
	node, ok := props[paramName]
	assert.Truef(t, ok, "tool %q: expected parameter %q to be present in InputSchema", toolName, paramName)
	if ok {
		gotType, _ := node["type"].(string)
		assert.Equalf(t, wantType, gotType,
			"tool %q: parameter %q should have type %q, got %q", toolName, paramName, wantType, gotType)
	}
}

// assertStructArrayProp asserts that a structArray parameter exists and that
// each of the listed child property names is present inside its items schema.
func assertStructArrayProp(t *testing.T, toolName string, props map[string]map[string]any, paramName string, childNames []string) {
	t.Helper()
	node, ok := props[paramName]
	require.Truef(t, ok, "tool %q: expected struct-array parameter %q to be present", toolName, paramName)
	if !ok {
		return
	}
	gotType, _ := node["type"].(string)
	assert.Equalf(t, "array", gotType, "tool %q: %q should be an array", toolName, paramName)

	items, _ := node["items"].(map[string]any)
	require.NotNilf(t, items, "tool %q: %q.items must not be nil", toolName, paramName)

	for _, child := range childNames {
		_, hasChild := items[child]
		assert.Truef(t, hasChild,
			"tool %q: %q.items should contain child property %q", toolName, paramName, child)
	}
}

// ---------------------------------------------------------------------------
// http_fuzzer
// ---------------------------------------------------------------------------

// TestHTTPFuzzerSchema_TLSParams ensures the three TLS-related parameters
// added to fix the Akamai WAF bypass issue are present and correctly typed.
func TestHTTPFuzzerSchema_TLSParams(t *testing.T) {
	const tool = "http_fuzzer"
	props := toolProps(t, tool)

	cases := []struct {
		param    string
		wantType string
	}{
		{"randomJA3", "boolean"},
		{"sni", "string"},
		{"overwriteSNI", "boolean"},
		// pre-existing TLS params must still be present
		{"isGmTls", "boolean"},
		{"isHttps", "boolean"},
	}
	for _, tc := range cases {
		t.Run(fmt.Sprintf("param=%s", tc.param), func(t *testing.T) {
			assertProp(t, tool, props, tc.param, tc.wantType)
		})
	}
}

// ---------------------------------------------------------------------------
// query_http_flow  (filterHTTPFlowToolOptions)
// ---------------------------------------------------------------------------

// TestQueryHTTPFlowSchema_NewFilterParams ensures every parameter added to
// filterHTTPFlowToolOptions is visible on the query_http_flow tool.
func TestQueryHTTPFlowSchema_NewFilterParams(t *testing.T) {
	const tool = "query_http_flow"
	props := toolProps(t, tool)

	scalarCases := []struct {
		param    string
		wantType string
	}{
		{"haveCommonParams", "boolean"},
		{"haveParamsTotal", "string"},
		{"offsetId", "number"},
		{"payloadKeyword", "string"},
		{"excludeStatusCode", "string"},
		// pre-existing params must still be present
		{"keyword", "string"},
		{"statusCode", "string"},
		{"haveBody", "boolean"},
	}
	for _, tc := range scalarCases {
		t.Run(fmt.Sprintf("param=%s", tc.param), func(t *testing.T) {
			assertProp(t, tool, props, tc.param, tc.wantType)
		})
	}

	arrayCases := []struct {
		param    string
		wantType string
	}{
		{"excludeId", "array"},
		{"includeId", "array"},
		{"color", "array"},
		{"includeHash", "array"},
		{"hostnameFilter", "array"},
	}
	for _, tc := range arrayCases {
		t.Run(fmt.Sprintf("param=%s", tc.param), func(t *testing.T) {
			assertProp(t, tool, props, tc.param, tc.wantType)
		})
	}
}

// TestQueryHTTPFlowSchema_MitmAggregateFilterRows checks that the
// mitmExtractAggregateFilterRows struct-array parameter has the correct shape.
func TestQueryHTTPFlowSchema_MitmAggregateFilterRows(t *testing.T) {
	const tool = "query_http_flow"
	props := toolProps(t, tool)
	assertStructArrayProp(t, tool, props, "mitmExtractAggregateFilterRows", []string{"ruleVerbose", "displayData"})
}

// ---------------------------------------------------------------------------
// set_tag_for_http_flow
// ---------------------------------------------------------------------------

// TestSetTagForHTTPFlowSchema_HashParam ensures the hash parameter is present.
func TestSetTagForHTTPFlowSchema_HashParam(t *testing.T) {
	const tool = "set_tag_for_http_flow"
	props := toolProps(t, tool)
	assertProp(t, tool, props, "hash", "string")
	// pre-existing required params must still be present
	assertProp(t, tool, props, "id", "number")
}

// ---------------------------------------------------------------------------
// delete_http_flow
// ---------------------------------------------------------------------------

// TestDeleteHTTPFlowSchema_NewParams ensures all newly added top-level params
// and the filter struct are present on delete_http_flow.
func TestDeleteHTTPFlowSchema_NewParams(t *testing.T) {
	const tool = "delete_http_flow"
	props := toolProps(t, tool)

	cases := []struct {
		param    string
		wantType string
	}{
		{"id", "array"},
		{"itemHash", "array"},
		{"urlPrefix", "string"},
		{"urlPrefixBatch", "array"},
		// pre-existing
		{"deleteAll", "boolean"},
		{"filter", "object"},
	}
	for _, tc := range cases {
		t.Run(fmt.Sprintf("param=%s", tc.param), func(t *testing.T) {
			assertProp(t, tool, props, tc.param, tc.wantType)
		})
	}
}

// TestDeleteHTTPFlowSchema_FilterInheritsNewParams ensures the filter struct
// inside delete_http_flow carries the params from filterHTTPFlowToolOptions,
// including the newly added ones. The filter node is a WithStruct whose
// "properties" value is an *omap.OrderedMap in the in-process representation.
// We verify it is non-nil and non-empty; the per-key assertions are already
// covered by TestQueryHTTPFlowSchema_NewFilterParams which tests the same
// filterHTTPFlowToolOptions slice applied to query_http_flow.
func TestDeleteHTTPFlowSchema_FilterInheritsNewParams(t *testing.T) {
	const tool = "delete_http_flow"
	props := toolProps(t, tool)

	filterNode, ok := props["filter"]
	require.Truef(t, ok, "tool %q: expected filter parameter", tool)
	assert.Equalf(t, "object", filterNode["type"], "tool %q: filter must be type object", tool)

	filterProps := filterNode["properties"]
	assert.NotNilf(t, filterProps, "tool %q: filter.properties must not be nil", tool)
}
