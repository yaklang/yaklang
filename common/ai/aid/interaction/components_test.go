package interaction

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func testCatalog(t *testing.T) Catalog {
	t.Helper()
	raw, err := os.ReadFile("testdata/catalog.json")
	require.NoError(t, err)
	var catalog Catalog
	require.NoError(t, json.Unmarshal(raw, &catalog))
	require.NoError(t, catalog.Validate())
	return catalog
}

func TestUICatalogNormalization(t *testing.T) {
	catalog := testCatalog(t)
	doc := ComponentDocument{ID: "block", CatalogVersion: catalog.Version, Fallback: "比较结果", Root: "root",
		Nodes: []ComponentNode{
			{ID: "root", Component: "Stack", Props: map[string]any{"title": 123}, Children: []string{"text", "unknown", "invalid"}},
			{ID: "text", Component: "Text", Props: map[string]any{"text": "保留"}},
			{ID: "unknown", Component: "script", Props: map[string]any{"html": "<script>"}},
			{ID: "invalid", Component: "Text", Props: map[string]any{}},
		}}
	normalized, diagnostics := catalog.Normalize(doc)
	require.NotEmpty(t, diagnostics)
	require.Len(t, normalized.Nodes, 2, "invalid child must not remove valid siblings or the layout")
	require.Empty(t, normalized.Nodes[0].Props)
	require.Equal(t, []string{"text"}, normalized.Nodes[0].Children)
	require.Equal(t, []string{"Stack", "Text"}, normalized.RequiredComponents)
}

func TestUIRequiredDefaultsAndCycle(t *testing.T) {
	catalog := testCatalog(t)
	entry := catalog.Definition("Text")
	entry.Schema["properties"].(map[string]any)["text"].(map[string]any)["default"] = "default text"
	doc := ComponentDocument{ID: "block", CatalogVersion: catalog.Version, Fallback: "fallback", Root: "root",
		Nodes: []ComponentNode{
			{ID: "root", Component: "Stack", Props: map[string]any{}, Children: []string{"root", "text", "text"}},
			{ID: "text", Component: "Text"},
		}}
	normalized, diagnostics := catalog.Normalize(doc)
	require.Len(t, normalized.Nodes, 2)
	require.Len(t, diagnostics, 2)
	require.Equal(t, []string{"text"}, normalized.Nodes[0].Children)
	require.Equal(t, "default text", normalized.Nodes[1].Props["text"])
}

func TestUIRejectExternalReferences(t *testing.T) {
	require.Error(t, CheckSchema(map[string]any{"$ref": "https://example.com/schema"}, 0))
	require.Error(t, CheckSchema(map[string]any{"properties": map[string]any{"__proto__": true}}, 0))
}
