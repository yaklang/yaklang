package buildinaitools

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

func TestRecentToolCacheSchemaPreservesConstraints(t *testing.T) {
	tool := makeTool("constrained", "constraint fixture", aitool.WithStringParam("path"))
	tool.InputSchema.AllOf = []any{map[string]any{"required": []string{"path"}}}
	wrapped := tool.ToJSONSchemaString()
	var original map[string]any
	require.NoError(t, json.Unmarshal([]byte(wrapped), &original))
	expected := original["properties"].(map[string]any)["params"]
	var actual map[string]any
	require.NoError(t, json.Unmarshal([]byte(renderDirectlyCallParamsSchema(wrapped)), &actual))
	require.Equal(t, expected, actual, "cache schemas must retain all constraints and descriptions")
	require.NotContains(t, renderDirectlyCallParamsSchema(wrapped), "TOOL_PARAM_")
}

func TestRecentToolCacheSchemaUnwrapsOnlyToolEnvelopes(t *testing.T) {
	for _, raw := range []string{
		`{"type":"object","properties":{"params":{"type":"object","properties":{"x":{"type":"string"}}},"path":{"type":"string"}},"required":["path"],"additionalProperties":false}`,
		`{"type":"object","allOf":[{"required":["x"]}],"properties":{"x":{"type":"string"}},"maxProperties":1,"description":"preserve me"}`,
		`{"type":"object","additionalProperties":false}`,
	} {
		var expected, actual map[string]any
		require.NoError(t, json.Unmarshal([]byte(raw), &expected))
		require.NoError(t, json.Unmarshal([]byte(renderDirectlyCallParamsSchema(raw)), &actual))
		require.Equal(t, expected, actual)
	}
	require.Equal(t, "malformed", renderDirectlyCallParamsSchema("malformed"))
}
