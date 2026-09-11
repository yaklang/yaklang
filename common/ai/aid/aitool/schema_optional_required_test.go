package aitool

import (
	"encoding/json"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/stretchr/testify/require"
)

func TestToolSchemaOptionalRequiredIsValid(t *testing.T) {
	for _, tc := range []struct {
		name string
		opts []ToolOption
	}{
		{"optional", []ToolOption{WithStringParam("url")}},
		{"oneOf", []ToolOption{WithOneOfStructParam("choice", nil, []ToolOption{WithStringParam("url")})}},
		{"anyOf", []ToolOption{WithAnyOfStructParam("choice", nil, []ToolOption{WithStringParam("url")})}},
		{"noParams", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tool := newTool("optional_tool", tc.opts...)
			var document any
			require.NoError(t, json.Unmarshal([]byte(tool.ToJSONSchemaString()), &document))
			compiler := jsonschema.NewCompiler()
			require.NoError(t, compiler.AddResource("tool.json", document))
			schema, err := compiler.Compile("tool.json")
			require.NoError(t, err, "upstream validators reject required:null")
			require.NoError(t, schema.Validate(map[string]any{"@action": "call-tool", "tool": "optional_tool", "params": map[string]any{}}))
		})
	}
}
