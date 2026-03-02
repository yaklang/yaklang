package mcp

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

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
}
