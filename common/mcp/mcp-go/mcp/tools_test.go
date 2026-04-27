package mcp

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
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
