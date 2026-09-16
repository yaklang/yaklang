package mcp

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestToolInputSchemaCrossFieldConstraintsRoundTrip(t *testing.T) {
	raw := map[string]any{"type": "object", "properties": map[string]any{"url": map[string]any{"type": "string"}}, "allOf": []any{map[string]any{"required": []any{"url"}}}}
	var schema ToolInputSchema
	require.NoError(t, schema.FromMap(raw))
	encoded, err := json.Marshal(schema)
	require.NoError(t, err)
	var decoded ToolInputSchema
	require.NoError(t, json.Unmarshal(encoded, &decoded))
	require.Equal(t, schema.AllOf, decoded.AllOf)
	encoded, err = json.Marshal(decoded.ToMap())
	require.NoError(t, err)
	var roundTrip map[string]any
	require.NoError(t, json.Unmarshal(encoded, &roundTrip))
	require.Equal(t, raw, roundTrip)
	require.NoError(t, decoded.FromMap(map[string]any{"type": "object"}))
	require.Empty(t, decoded.AllOf, "reusing a schema must not retain stale constraints")
}
