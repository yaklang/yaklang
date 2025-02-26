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
