package mcp

import (
	"encoding/json"
	"fmt"
	"sort"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/omap"
)

func requireSchemaWithoutNullRequired(t *testing.T, schema any) map[string]any {
	t.Helper()

	raw, err := json.Marshal(schema)
	require.NoError(t, err)
	require.NotContains(t, string(raw), `"required":null`)

	var parsed map[string]any
	require.NoError(t, json.Unmarshal(raw, &parsed))
	return parsed
}

func TestToolInputSchema_FromMap_PropertiesAlwaysOrdered(t *testing.T) {
	propertyKeys := []string{"zebra", "alpha", "mango", "delta", "charlie"}
	expectedOrder := make([]string, len(propertyKeys))
	copy(expectedOrder, propertyKeys)
	sort.Slice(expectedOrder, func(i, j int) bool {
		return expectedOrder[i] > expectedOrder[j]
	})

	// Run many times: Go map iteration order varies, but OrderInsert must yield stable descending keys.
	for i := 0; i < 100; i++ {
		properties := make(map[string]any, len(propertyKeys))
		for _, k := range propertyKeys {
			properties[k] = map[string]any{"type": "string"}
		}

		var schema ToolInputSchema
		err := schema.FromMap(map[string]any{
			"type":       "object",
			"properties": properties,
		})
		require.NoError(t, err, "iteration %d", i)
		require.Equal(t, expectedOrder, schema.Properties.Keys(), "iteration %d", i)
	}
}

func TestWithOneOf(t *testing.T) {
	tool := NewTool("tool",
		WithString("test"),
		WithOneOfStruct("option",
			[]PropertyOption{
				Description("an option"),
			},
			[]ToolOption{
				WithString("string", Description("string"), Required()),
			},
			[]ToolOption{
				WithNumber("number", Description("number"), Required()),
			},
		),
	)

	p := tool.InputSchema
	b, err := json.MarshalIndent(p, "", "  ")
	require.NoError(t, err)
	fmt.Println(string(b))
}

func TestWithAnyOf(t *testing.T) {
	tool := NewTool("tool",
		WithString("test"),
		WithAnyOfStruct("option",
			[]PropertyOption{
				Description("an option"),
			},
			[]ToolOption{
				WithString("string", Description("string")),
			},
			[]ToolOption{
				WithNumber("number", Description("number")),
			},
		),
	)

	p := tool.InputSchema
	b, err := json.MarshalIndent(p, "", "  ")
	require.NoError(t, err)
	fmt.Println(string(b))
}

func TestMarshalInputSchema_EmptyPropertiesAlwaysObject(t *testing.T) {
	tool := NewTool("list_all_payload_dictionary_details",
		WithDescription("List all payload dictionary details"),
	)

	raw, err := json.Marshal(tool.InputSchema)
	require.NoError(t, err)
	require.NotContains(t, string(raw), `"required":null`)

	var parsed map[string]any
	require.NoError(t, json.Unmarshal(raw, &parsed))

	properties, ok := parsed["properties"].(map[string]any)
	require.True(t, ok, "properties should be encoded as object")
	require.Len(t, properties, 0)
}

func TestMarshalInputSchema_NestedEmptyPropertiesAlwaysObject(t *testing.T) {
	tool := NewTool("delete_ports",
		WithDescription("Delete ports based with flexible filters"),
		WithStruct("filter", nil,
			WithStruct("pagination",
				[]PropertyOption{
					Description("Pagination settings for the query"),
					Required(),
				},
			),
		),
	)

	raw, err := json.Marshal(tool.InputSchema)
	require.NoError(t, err)

	var parsed map[string]any
	require.NoError(t, json.Unmarshal(raw, &parsed))

	rootProperties, ok := parsed["properties"].(map[string]any)
	require.True(t, ok, "root properties should be object")

	filterSchema, ok := rootProperties["filter"].(map[string]any)
	require.True(t, ok, "filter should be object schema")

	filterProperties, ok := filterSchema["properties"].(map[string]any)
	require.True(t, ok, "filter.properties should be object")

	paginationSchema, ok := filterProperties["pagination"].(map[string]any)
	require.True(t, ok, "pagination should be object schema")

	paginationProperties, ok := paginationSchema["properties"].(map[string]any)
	require.True(t, ok, "pagination.properties should be object")
	require.Len(t, paginationProperties, 0)

	_, hasPaginationRequired := paginationSchema["required"]
	require.False(t, hasPaginationRequired, "pagination.required should be omitted when empty")
}

func TestMarshalInputSchema_StructOptionalRequiredOmitted(t *testing.T) {
	tool := NewTool("tool",
		WithStruct("filter", nil,
			WithStruct("pagination", nil),
		),
	)

	parsed := requireSchemaWithoutNullRequired(t, tool.InputSchema)

	rootProperties, ok := parsed["properties"].(map[string]any)
	require.True(t, ok, "root properties should be object")

	filterSchema, ok := rootProperties["filter"].(map[string]any)
	require.True(t, ok, "filter should be object schema")

	_, hasFilterRequired := filterSchema["required"]
	require.False(t, hasFilterRequired, "filter.required should be omitted when empty")

	filterProperties, ok := filterSchema["properties"].(map[string]any)
	require.True(t, ok, "filter.properties should be object")

	paginationSchema, ok := filterProperties["pagination"].(map[string]any)
	require.True(t, ok, "pagination should be object schema")

	_, hasPaginationRequired := paginationSchema["required"]
	require.False(t, hasPaginationRequired, "pagination.required should be omitted when empty")
}

func TestMarshalInputSchema_OneOfOptionalRequiredOmitted(t *testing.T) {
	tool := NewTool("tool",
		WithOneOfStruct("option", nil,
			[]ToolOption{
				WithString("string", Description("string")),
			},
			[]ToolOption{
				WithNumber("number", Description("number")),
			},
		),
	)

	parsed := requireSchemaWithoutNullRequired(t, tool.InputSchema)

	rootProperties, ok := parsed["properties"].(map[string]any)
	require.True(t, ok, "root properties should be object")

	optionSchema, ok := rootProperties["option"].(map[string]any)
	require.True(t, ok, "option should be object schema")

	oneOf, ok := optionSchema["oneOf"].([]any)
	require.True(t, ok, "option.oneOf should be array")
	require.Len(t, oneOf, 2)

	for _, item := range oneOf {
		branch, ok := item.(map[string]any)
		require.True(t, ok, "oneOf branch should be object")
		_, hasRequired := branch["required"]
		require.False(t, hasRequired, "oneOf branch required should be omitted when empty")
	}
}

func TestMarshalInputSchema_AnyOfOptionalRequiredOmitted(t *testing.T) {
	tool := NewTool("tool",
		WithAnyOfStruct("option", nil,
			[]ToolOption{
				WithString("string", Description("string")),
			},
			[]ToolOption{
				WithNumber("number", Description("number")),
			},
		),
	)

	parsed := requireSchemaWithoutNullRequired(t, tool.InputSchema)

	rootProperties, ok := parsed["properties"].(map[string]any)
	require.True(t, ok, "root properties should be object")

	optionSchema, ok := rootProperties["option"].(map[string]any)
	require.True(t, ok, "option should be object schema")

	anyOf, ok := optionSchema["anyOf"].([]any)
	require.True(t, ok, "option.anyOf should be array")
	require.Len(t, anyOf, 2)

	for _, item := range anyOf {
		branch, ok := item.(map[string]any)
		require.True(t, ok, "anyOf branch should be object")
		_, hasRequired := branch["required"]
		require.False(t, hasRequired, "anyOf branch required should be omitted when empty")
	}
}

func TestMarshalInputSchema_StructArrayOptionalRequiredOmitted(t *testing.T) {
	tool := NewTool("tool",
		WithStructArray("items", nil,
			WithString("name", Description("item name")),
		),
	)

	parsed := requireSchemaWithoutNullRequired(t, tool.InputSchema)

	rootProperties, ok := parsed["properties"].(map[string]any)
	require.True(t, ok, "root properties should be object")

	itemsSchema, ok := rootProperties["items"].(map[string]any)
	require.True(t, ok, "items should be array schema")

	itemSchema, ok := itemsSchema["items"].(map[string]any)
	require.True(t, ok, "items.items should be object schema")

	itemProperties, ok := itemSchema["properties"].(map[string]any)
	require.True(t, ok, "items.items.properties should be object")
	_, hasName := itemProperties["name"]
	require.True(t, hasName, "items.items.properties should contain name")

	_, hasRequired := itemSchema["required"]
	require.False(t, hasRequired, "array item required should be omitted when empty")
}

func TestWithPagingSchema(t *testing.T) {
	tool := NewTool("query_test",
		WithPaging("pagination", []string{"id", "created_at", "updated_at"}),
	)

	raw, err := json.Marshal(tool.InputSchema)
	require.NoError(t, err)

	var parsed map[string]any
	require.NoError(t, json.Unmarshal(raw, &parsed))

	rootProperties, ok := parsed["properties"].(map[string]any)
	require.True(t, ok, "root properties should be object")

	paginationSchema, ok := rootProperties["pagination"].(map[string]any)
	require.True(t, ok, "pagination should be object schema")

	paginationProperties, ok := paginationSchema["properties"].(map[string]any)
	require.True(t, ok, "pagination.properties should be object")

	// order should be asc/desc
	orderSchema, ok := paginationProperties["order"].(map[string]any)
	require.True(t, ok, "order should be object schema")
	require.Equal(t, "string", orderSchema["type"])
	orderEnum, ok := orderSchema["enum"].([]any)
	require.True(t, ok, "order enum should be array")
	require.ElementsMatch(t, []any{"asc", "desc"}, orderEnum)

	// orderby should be field names
	orderBySchema, ok := paginationProperties["orderby"].(map[string]any)
	require.True(t, ok, "orderby should be object schema")
	require.Equal(t, "string", orderBySchema["type"])
	orderByEnum, ok := orderBySchema["enum"].([]any)
	require.True(t, ok, "orderby enum should be array")
	require.ElementsMatch(t, []any{"id", "created_at", "updated_at"}, orderByEnum)
}

func TestWithPagingSchema_NilFieldNamesOmitsEnum(t *testing.T) {
	tool := NewTool("query_test",
		WithPaging("pagination", nil),
	)

	raw, err := json.Marshal(tool.InputSchema)
	require.NoError(t, err)
	require.NotContains(t, string(raw), `"enum":null`)

	var parsed map[string]any
	require.NoError(t, json.Unmarshal(raw, &parsed))

	rootProperties, ok := parsed["properties"].(map[string]any)
	require.True(t, ok, "root properties should be object")

	paginationSchema, ok := rootProperties["pagination"].(map[string]any)
	require.True(t, ok, "pagination should be object schema")

	paginationProperties, ok := paginationSchema["properties"].(map[string]any)
	require.True(t, ok, "pagination.properties should be object")

	orderBySchema, ok := paginationProperties["orderby"].(map[string]any)
	require.True(t, ok, "orderby should be object schema")
	_, hasOrderByEnum := orderBySchema["enum"]
	require.False(t, hasOrderByEnum, "orderby.enum should be omitted when fieldNames is nil")
}

func TestMarshalInputSchema_OmitsBoolRequiredOnProperties(t *testing.T) {
	tool := NewTool("optional_probe",
		WithString("timezone", Description("optional timezone"), func(schema map[string]any) {
			schema["required"] = false
		}),
	)

	raw, err := json.Marshal(tool.InputSchema)
	require.NoError(t, err)
	require.NotContains(t, string(raw), `"required":false`)

	var parsed map[string]any
	require.NoError(t, json.Unmarshal(raw, &parsed))

	props, ok := parsed["properties"].(map[string]any)
	require.True(t, ok, "properties should be object")

	tz, ok := props["timezone"].(map[string]any)
	require.True(t, ok, "timezone property should exist")
	_, hasRequired := tz["required"]
	require.False(t, hasRequired, "property-level required must be omitted")
}

func TestWithPagingSchema_EmptyFieldNamesOmitsEnum(t *testing.T) {
	tool := NewTool("query_test",
		WithPaging("pagination", []string{}),
	)

	raw, err := json.Marshal(tool.InputSchema)
	require.NoError(t, err)
	require.NotContains(t, string(raw), `"enum":null`)

	var parsed map[string]any
	require.NoError(t, json.Unmarshal(raw, &parsed))

	rootProperties, ok := parsed["properties"].(map[string]any)
	require.True(t, ok, "root properties should be object")

	paginationSchema, ok := rootProperties["pagination"].(map[string]any)
	require.True(t, ok, "pagination should be object schema")

	paginationProperties, ok := paginationSchema["properties"].(map[string]any)
	require.True(t, ok, "pagination.properties should be object")

	orderBySchema, ok := paginationProperties["orderby"].(map[string]any)
	require.True(t, ok, "orderby should be object schema")
	_, hasOrderByEnum := orderBySchema["enum"]
	require.False(t, hasOrderByEnum, "orderby.enum should be omitted when fieldNames is empty")
}

// ---------------------------------------------------------------------------
// ToMap vs MarshalJSON: nested required array preservation
//
// Bug (fixed): ToMap() unconditionally deleted the "required" key from every
// top-level property node. This stripped valid []string required arrays
// produced by WithStruct (e.g. {"required":["name"]}), while MarshalJSON
// only stripped the internal bool "required" marker. The mismatch meant:
//   - validateWithSchema (which uses ToMap) silently skipped required-field
//     checks for nested object properties
//   - yakscripttools/base.go persisted ToMap output to DB with required arrays
//     missing, producing silently incomplete stored schemas
//
// The tests below are written as regression guards: each one would fail
// against the pre-fix ToMap implementation.
// ---------------------------------------------------------------------------

// toMapJSON marshals ToMap() output to JSON then unmarshals to map[string]any,
// producing the same plain-Go-types representation that callers like
// validateWithSchema and yakscripttools/base.go work with.
func toMapJSON(t *testing.T, schema ToolInputSchema) map[string]any {
	t.Helper()
	raw, err := json.Marshal(schema.ToMap())
	require.NoError(t, err)
	var m map[string]any
	require.NoError(t, json.Unmarshal(raw, &m))
	return m
}

// marshalJSON unmarshals MarshalJSON output to map[string]any — the schema
// representation sent to external clients (LLMs / Claude Code).
func marshalJSON(t *testing.T, schema ToolInputSchema) map[string]any {
	t.Helper()
	raw, err := json.Marshal(schema)
	require.NoError(t, err)
	var m map[string]any
	require.NoError(t, json.Unmarshal(raw, &m))
	return m
}

// TestToMap_PreservesStructRequiredArray is a regression test for the bug
// where ToMap() stripped valid []string required arrays from WithStruct
// properties. Before the fix, filter["required"] was deleted by ToMap,
// causing validateWithSchema to not enforce required fields and DB-persisted
// schemas to lose required constraints.
func TestToMap_PreservesStructRequiredArray(t *testing.T) {
	tool := NewTool("tool",
		WithStruct("filter", nil,
			WithString("name", Required()),
			WithString("value"),
		),
	)

	m := toMapJSON(t, tool.InputSchema)

	props, ok := m["properties"].(map[string]any)
	require.True(t, ok, "properties should be object")

	filter, ok := props["filter"].(map[string]any)
	require.True(t, ok, "filter should be object schema")

	// Pre-fix: this assertion failed because ToMap deleted filter["required"]
	// regardless of type, stripping the valid []string array.
	required, hasRequired := filter["required"].([]any)
	require.True(t, hasRequired,
		"filter.required must be preserved as []string in ToMap output (pre-fix it was stripped), got: %v", filter)
	require.ElementsMatch(t, []any{"name"}, required,
		"filter.required should contain 'name'")
}

// TestToMap_MatchesMarshalJSON_ForStructRequired ensures ToMap and MarshalJSON
// agree on required arrays for WithStruct properties. Before the fix, they
// disagreed: MarshalJSON preserved required:["name"] while ToMap stripped it,
// meaning the schema shown to the LLM and the schema used for internal
// validation diverged.
func TestToMap_MatchesMarshalJSON_ForStructRequired(t *testing.T) {
	tool := NewTool("tool",
		WithStruct("filter", nil,
			WithString("name", Required()),
		),
	)

	toMapResult := toMapJSON(t, tool.InputSchema)
	marshalResult := marshalJSON(t, tool.InputSchema)

	toMapProps := toMapResult["properties"].(map[string]any)
	marshalProps := marshalResult["properties"].(map[string]any)

	toMapFilter := toMapProps["filter"].(map[string]any)
	marshalFilter := marshalProps["filter"].(map[string]any)

	// Pre-fix: MarshalJSON had required (true), ToMap did not (false) → mismatch
	_, toMapHasReq := toMapFilter["required"]
	_, marshalHasReq := marshalFilter["required"]
	require.Equal(t, marshalHasReq, toMapHasReq,
		"ToMap and MarshalJSON must agree on struct required: MarshalJSON has %v, ToMap has %v",
		marshalHasReq, toMapHasReq)
}

// ---------------------------------------------------------------------------
// WithPaging / WithKVPairs internal representation consistency
//
// Bug (fixed): WithPaging and WithKVPairs built their inner "properties" as
// plain Go map[string]any instead of *omap.OrderedMap. Every other builder
// (WithStruct, WithStructArray, WithOneOfStruct, WithAnyOfStruct) uses
// OrderedMap to preserve field insertion order. The inconsistency meant
// WithPaging/WithKVPairs schema nodes had non-deterministic internal field
// ordering, diverging from the OrderedMap convention used everywhere else.
//
// These tests verify the internal structure uses OrderedMap, so anyone
// reading or iterating these schema nodes (e.g. buildStructOptionsFromMap,
// normalizeSchemaValue) gets consistent, insertion-ordered fields.
// ---------------------------------------------------------------------------

// TestWithPaging_UsesOrderedMapForProperties is a regression test for the bug
// where WithPaging used a plain map[string]any for its inner properties
// instead of *omap.OrderedMap. Before the fix, the type assertion to
// *omap.OrderedMap failed because paginationSchema["properties"] was a plain
// map — inconsistent with WithStruct and other builders.
func TestWithPaging_UsesOrderedMapForProperties(t *testing.T) {
	tool := NewTool("query",
		WithPaging("pagination", []string{"id"}),
	)

	paginationRaw, ok := tool.InputSchema.Properties.Get("pagination")
	require.True(t, ok, "pagination should be registered")

	paginationSchema, ok := paginationRaw.(map[string]any)
	require.True(t, ok, "pagination should be a schema map")

	// Pre-fix: this type assertion failed — properties was a plain map[string]any
	props, ok := paginationSchema["properties"].(*omap.OrderedMap[string, any])
	require.True(t, ok,
		"WithPaging properties must be an OrderedMap for consistency with other builders (pre-fix it was a plain map), got %T",
		paginationSchema["properties"])
	require.Equal(t, []string{"page", "limit", "order", "orderby"}, props.Keys(),
		"OrderedMap should preserve insertion order: page, limit, order, orderby")
}

// TestWithKVPairs_UsesOrderedMapForProperties is a regression test for the bug
// where WithKVPairs used a plain map[string]any for its items.properties
// instead of *omap.OrderedMap. Before the fix, the type assertion to
// *omap.OrderedMap failed because items["properties"] was a plain map.
func TestWithKVPairs_UsesOrderedMapForProperties(t *testing.T) {
	tool := NewTool("tool",
		WithKVPairs("headers"),
	)

	headersRaw, ok := tool.InputSchema.Properties.Get("headers")
	require.True(t, ok, "headers should be registered")

	headersSchema, ok := headersRaw.(map[string]any)
	require.True(t, ok, "headers should be a schema map")

	items, ok := headersSchema["items"].(map[string]any)
	require.True(t, ok, "headers.items should be a map")

	// Pre-fix: this type assertion failed — items.properties was a plain map[string]any
	props, ok := items["properties"].(*omap.OrderedMap[string, any])
	require.True(t, ok,
		"WithKVPairs items.properties must be an OrderedMap for consistency with other builders (pre-fix it was a plain map), got %T",
		items["properties"])
	require.Equal(t, []string{"key", "value"}, props.Keys(),
		"OrderedMap should preserve insertion order: key, value")
}
